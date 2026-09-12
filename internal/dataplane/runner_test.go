package dataplane

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// The wake-up, and what an idle replication pass costs.
//
// A data plane has two kinds of change to move. One is made somewhere it cannot
// see -- another plane's push, a console edit -- and the only way it learns is
// to ask the control database on a timer. The other is made here, in a store
// this process owns, and making it wait for that same timer is a choice, not a
// constraint: it is one of the two intervals between a DHCP client being
// answered and its own name resolving.
//
// These tests pin the second kind. They are written with an interval of an hour
// so that a poll cannot be what passes them: if the wake does nothing, the work
// is still queued when the test gives up.

// insertLeaseForPush puts one confirmed binding in the store and leaves the
// dirty marker the trigger writes, so a pass has one thing to move.
func insertLeaseForPush(t *testing.T, control *sql.DB, store *Store, rep *Replicator) {
	t.Helper()
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	// The control database enforces its foreign keys, so the scope has to be
	// there before the lease that points at it can be pushed into it.
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync the scope down: %v", err)
	}
	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")
}

// controlLeaseCount reports how many leases the control database has, which is
// the only thing the push is for.
func controlLeaseCount(t *testing.T, control *sql.DB) int {
	t.Helper()
	var n int
	if err := control.QueryRow(`SELECT COUNT(*) FROM dhcp_leases`).Scan(&n); err != nil {
		t.Fatalf("count the control database's leases: %v", err)
	}
	return n
}

// awaitControlLease waits for the push to land and says how long it took.
func awaitControlLease(t *testing.T, control *sql.DB, limit time.Duration) time.Duration {
	t.Helper()
	started := time.Now()
	for time.Since(started) < limit {
		if controlLeaseCount(t, control) > 0 {
			return time.Since(started)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the lease was never pushed: after %s it is still queued, waiting "+
		"for a poll that is an hour away", limit)
	return 0
}

// TestAWokenLoopPushesWithoutWaitingForTheInterval is the mechanism: a change
// this process just made does not wait for the poll.
func TestAWokenLoopPushesWithoutWaitingForTheInterval(t *testing.T) {
	control, store, rep := newPair(t)
	insertLeaseForPush(t, control, store, rep)

	runner := NewRunner(rep, RunnerConfig{
		Domains:    []Domain{DomainDHCP},
		PushLeases: true,
		Interval:   time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	// Let the loop reach its select before waking it, so this measures the
	// wake and not the channel's buffer.
	time.Sleep(50 * time.Millisecond)
	runner.Wake()

	if took := awaitControlLease(t, control, 5*time.Second); took > 2*time.Second {
		t.Fatalf("the push took %s after a wake; it should not be bounded by the poll", took)
	}
}

// TestASignalSentBeforeTheLoopStartsIsNotLost covers the other ordering. The
// request path signals a loop that another goroutine is starting, and a signal
// dropped because "nobody is listening yet" would reintroduce the wait it
// exists to remove. It is why the channel is buffered.
func TestASignalSentBeforeTheLoopStartsIsNotLost(t *testing.T) {
	control, store, rep := newPair(t)
	insertLeaseForPush(t, control, store, rep)

	runner := NewRunner(rep, RunnerConfig{
		Domains:    []Domain{DomainDHCP},
		PushLeases: true,
		Interval:   time.Hour,
	})

	runner.Wake() // before Run, deliberately

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	awaitControlLease(t, control, 5*time.Second)
}

// TestWakeNeverBlocksTheRequestPath: Wake is called from the goroutine that
// builds a DHCP reply. Blocking there would trade one wait for a worse one.
func TestWakeNeverBlocksTheRequestPath(t *testing.T) {
	_, store, rep := newPair(t)
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, Interval: time.Hour})
	_ = store

	done := make(chan struct{})
	go func() {
		// No loop is draining it, and the buffer holds one. Every send after
		// the first has to be a no-op rather than a wait.
		for i := 0; i < 1000; i++ {
			runner.Wake()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wake blocked; the DHCP request path would stall behind a replication signal")
	}
}

// TestTheCostOfAnIdlePassAtTheDocumentedLeaseCeiling is why the poll interval
// can be one second.
//
// ADR 0001 fixes the sizing target at twenty thousand leases, so this measures
// a pass at that size with nothing to move -- the steady state, and the only
// one that repeats. The pass reads a counter per domain and drives its queue
// queries from the marker tables, which are empty when everything has been
// pushed; it does not walk the lease table.
//
// The bound is loose on purpose. It is here to catch a change that makes an
// idle pass cost something proportional to the lease table, not to measure a
// microsecond count on a shared machine; the measured value is logged so a
// regression is visible even while the bound holds.
//
// It is also an absolute figure, so it is only meaningful on a binary that is
// doing the work and nothing else. The race detector is not that binary. On the
// machine the bound was set on, the same pass measures 1.1ms without it and
// 46ms with it -- forty times -- and on a shared four-vCPU CI runner, where the
// suite runs as `go test -race ./...`, 179ms. Comparing an instrumented number
// against an uninstrumented bound says nothing about the product, so under
// -race the bound is widened rather than removed. The tight one is still
// enforced by plain `go test ./...`, which release.yml runs on every tag and
// which is what a local run uses; the measured value is logged either way, with
// which bound was in force.
func TestTheCostOfAnIdlePassAtTheDocumentedLeaseCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("seeding twenty thousand leases is not a short-mode test")
	}
	control, store, rep := newPair(t)
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")

	seedLeasesAtTheCeiling(t, store, 20000)

	runner := NewRunner(rep, RunnerConfig{
		Domains:    []Domain{DomainDHCP},
		PushLeases: true,
		Interval:   time.Second,
	})

	// One pass to settle, then measure. A pass after a change would be doing
	// the work rather than paying for the poll.
	runner.Once(context.Background())
	started := time.Now()
	runner.Once(context.Background())
	took := time.Since(started)

	bound := 100 * time.Millisecond
	if raceDetectorEnabled {
		bound = 500 * time.Millisecond
	}

	t.Logf("an idle pass with 20000 leases took %s, against a %s interval (bound %s, race %t)",
		took, time.Second, bound, raceDetectorEnabled)
	if took > bound {
		t.Fatalf("an idle pass took %s, more than the %s this test allows: at that "+
			"price polling every second costs real work per pass", took, bound)
	}
}

// seedLeasesAtTheCeiling fills the lease table in one transaction -- twenty
// thousand separate commits would make this test about fsync rather than about
// the poll -- and then clears the markers, which is the state a store is in
// once everything has been pushed once.
func seedLeasesAtTheCeiling(t *testing.T, store *Store, n int) {
	t.Helper()
	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO dhcp_leases
		(id, scope_id, ip_address, mac_address, hostname, client_id,
		 lease_start, lease_end, status, last_seen, generation)
		VALUES (?, 's1', ?, 'aa:bb:cc:dd:ee:01', 'h', '',
		        datetime('now'), datetime('now', '+1 hour'), 'active', datetime('now'), 1)`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for i := 0; i < n; i++ {
		ip := fmt.Sprintf("10.%d.%d.%d", i/(256*256)%256, i/256%256, i%256)
		if _, err := stmt.Exec(fmt.Sprintf("l%05d", i), ip); err != nil {
			t.Fatalf("insert lease %d: %v", i, err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, table := range []string{"dhcp_lease_dirty", "dhcp_dns_event_dirty", "dhcp_log_dirty"} {
		if _, err := store.Exec("DELETE FROM " + table); err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}
}
