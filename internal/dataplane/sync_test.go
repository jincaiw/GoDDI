package dataplane

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

// newPair builds a control database and a lease store wired together.
func newPair(t *testing.T) (*sql.DB, *Store, *Replicator) {
	t.Helper()
	control := newControlDB(t)
	store := newStore(t, config.DataPlaneLease)
	return control, store, NewReplicator(control, store)
}

func insertControlScope(t *testing.T, db *sql.DB, id, name, cidr, start, end string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, domain_name, dns_updates)
		VALUES (?, ?, ?, ?, ?, 'example.test', 1)`, id, name, cidr, start, end); err != nil {
		t.Fatalf("insert control scope %s: %v", id, err)
	}
}

func countLocal(t *testing.T, s *Store, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := s.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

// ---------------------------------------------------------------------------
// Downward: configuration.
// ---------------------------------------------------------------------------

func TestSyncCopiesConfigurationAndThenDoesNothing(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := control.Exec(`INSERT INTO dhcp_reservations (id, scope_id, ip_address, mac_address, hostname)
		VALUES ('r1', 's1', '10.0.0.15', 'aa:bb:cc:dd:ee:01', 'printer')`); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	if _, err := control.Exec(`INSERT INTO dhcp_options (id, scope_id, code, value)
		VALUES ('o1', 's1', 6, '10.0.0.1')`); err != nil {
		t.Fatalf("insert option: %v", err)
	}

	res, err := rep.Sync(ctx, DomainDHCP)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if !res.Applied {
		t.Fatal("the first sync did not load anything")
	}
	if res.Rows != 3 {
		t.Errorf("rows copied = %d, want 3", res.Rows)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_scopes WHERE id = 's1'`); got != 1 {
		t.Errorf("local scopes = %d, want 1", got)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_reservations WHERE id = 'r1'`); got != 1 {
		t.Errorf("local reservations = %d, want 1", got)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_options WHERE id = 'o1'`); got != 1 {
		t.Errorf("local options = %d, want 1", got)
	}

	// An unchanged revision must cost nothing: no read of the tables and no
	// transaction on the store the request path is using.
	res, err = rep.Sync(ctx, DomainDHCP)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if res.Applied {
		t.Error("a sync with an unchanged revision rewrote the store")
	}
	if res.Rows != 0 {
		t.Errorf("rows read on an unchanged revision = %d, want 0", res.Rows)
	}
}

func TestSyncPicksUpAChange(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	if _, err := control.Exec(`UPDATE dhcp_scopes SET lease_time = 7200, name = 'lan-renamed' WHERE id = 's1'`); err != nil {
		t.Fatalf("update scope: %v", err)
	}

	res, err := rep.Sync(ctx, DomainDHCP)
	if err != nil {
		t.Fatalf("sync after change: %v", err)
	}
	if !res.Applied {
		t.Fatal("a changed revision was not applied")
	}

	var name string
	var leaseTime int
	if err := store.QueryRow(`SELECT name, lease_time FROM dhcp_scopes WHERE id = 's1'`).Scan(&name, &leaseTime); err != nil {
		t.Fatalf("read local scope: %v", err)
	}
	if name != "lan-renamed" || leaseTime != 7200 {
		t.Errorf("local scope = (%q, %d), want (lan-renamed, 7200)", name, leaseTime)
	}
}

func TestSyncRemovesConfigurationThatIsGone(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	insertControlScope(t, control, "s2", "lab", "10.0.1.0/24", "10.0.1.10", "10.0.1.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	if _, err := control.Exec(`DELETE FROM dhcp_scopes WHERE id = 's2'`); err != nil {
		t.Fatalf("delete scope: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("sync after delete: %v", err)
	}

	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_scopes WHERE id = 's2'`); got != 0 {
		t.Errorf("a deleted scope is still in the local copy (%d rows)", got)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_scopes WHERE id = 's1'`); got != 1 {
		t.Errorf("the surviving scope was lost (%d rows)", got)
	}
}

// ---------------------------------------------------------------------------
// Scope removal vs leases that live on the data plane.
// ---------------------------------------------------------------------------

// TestSyncRetainsAScopeThatStillHoldsLeases is the case the control database
// used to handle with ON DELETE CASCADE and cannot any more, because the leases
// are no longer in that database. Dropping the scope would leave clients
// holding addresses the server can no longer answer for, so the scope stays and
// the retention is reported rather than swallowed.
func TestSyncRetainsAScopeThatStillHoldsLeases(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")

	// The console deletes the scope. The dependency check it runs reads the
	// control-plane replica, which lags by up to one tick, so this is exactly
	// the race the data plane has to survive.
	if _, err := control.Exec(`DELETE FROM dhcp_scopes WHERE id = 's1'`); err != nil {
		t.Fatalf("delete scope: %v", err)
	}

	res, err := rep.Sync(ctx, DomainDHCP)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(res.RetainedScopes) != 1 || res.RetainedScopes[0] != "s1" {
		t.Fatalf("retained scopes = %v, want [s1]", res.RetainedScopes)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_scopes WHERE id = 's1'`); got != 1 {
		t.Error("a scope with live leases was removed from the local copy")
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l1'`); got != 1 {
		t.Error("the lease was removed with its scope")
	}
}

// TestSyncDropsLeasesWhoseScopeIsReallyGone is the other half: once nothing
// holds the scope, its leftover rows must go, or every removed scope leaves
// permanent garbage behind.
func TestSyncDropsLeasesWhoseScopeIsReallyGone(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	// A lease nothing holds any more: expired, so it does not protect its scope.
	insertLease(t, store, "l-gone", "s1", "10.0.0.10", "expired")

	if _, err := control.Exec(`DELETE FROM dhcp_scopes WHERE id = 's1'`); err != nil {
		t.Fatalf("delete scope: %v", err)
	}

	res, err := rep.Sync(ctx, DomainDHCP)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(res.RetainedScopes) != 0 {
		t.Errorf("scopes retained for a lease nothing holds: %v", res.RetainedScopes)
	}
	if res.DroppedLeases != 1 {
		t.Errorf("dropped leases = %d, want 1", res.DroppedLeases)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l-gone'`); got != 0 {
		t.Error("a lease of a removed scope survived")
	}
}

// ---------------------------------------------------------------------------
// The control database being gone.
// ---------------------------------------------------------------------------

func TestSyncReportsAnUnreachableControlDatabase(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	// Something owed upward: a push with an empty queue is a no-op that never
	// touches the control database, so it can say nothing about reachability.
	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")

	if err := control.Close(); err != nil {
		t.Fatalf("close control: %v", err)
	}

	if _, err := rep.Sync(ctx, DomainDHCP); !errors.Is(err, ErrControlUnavailable) {
		t.Fatalf("sync against a closed control database = %v, want ErrControlUnavailable", err)
	}
	if _, err := rep.Push(ctx, 100); !errors.Is(err, ErrControlUnavailable) {
		t.Fatalf("push of an owed change against a closed control database = %v, want ErrControlUnavailable", err)
	}
	// The change is still owed: a failed push must not clear the marker.
	if n, err := rep.PendingLeases(); err != nil || n != 1 {
		t.Errorf("pending lease changes after a failed push = (%d, %v), want (1, nil)", n, err)
	}

	// The local copy has to be intact: that is the entire point.
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_scopes WHERE id = 's1'`); got != 1 {
		t.Error("the local copy did not survive the control database going away")
	}
}

func TestPrimeRefusesToStartWithNothingToServe(t *testing.T) {
	control, store, rep := newPair(t)
	if err := control.Close(); err != nil {
		t.Fatalf("close control: %v", err)
	}

	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}})
	err := runner.Prime(context.Background())
	if err == nil {
		t.Fatal("Prime accepted an empty store with an unreachable control database")
	}

	// A DHCP process that starts with no scopes answers nothing while
	// reporting healthy, which is the outcome being refused.
	if has, herr := store.HasReplica(DomainDHCP); herr != nil || has {
		t.Fatalf("HasReplica = (%v, %v), want (false, nil)", has, herr)
	}
}

func TestPrimeStartsFromTheLocalCopyWhenControlIsGone(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}})
	if err := runner.Prime(ctx); err != nil {
		t.Fatalf("first prime: %v", err)
	}

	if err := control.Close(); err != nil {
		t.Fatalf("close control: %v", err)
	}

	// New process, same store: yesterday's configuration is better than no DHCP.
	fresh := NewRunner(NewReplicator(control, store), RunnerConfig{Domains: []Domain{DomainDHCP}})
	if err := fresh.Prime(ctx); err != nil {
		t.Fatalf("restart with the control database down: %v", err)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_scopes WHERE id = 's1'`); got != 1 {
		t.Error("the scope was lost on restart with the control database down")
	}
	if st := fresh.Snapshot(); st.ControlReachable {
		t.Error("a failed replication attempt was reported as reachable")
	} else if st.LastError == "" {
		t.Error("the failure was reported without a reason")
	}
}

// ---------------------------------------------------------------------------
// Upward: taking over leases, then keeping the replica current.
// ---------------------------------------------------------------------------

func controlLease(t *testing.T, db *sql.DB, id, scopeID, ip string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO dhcp_leases
		(id, scope_id, ip_address, mac_address, hostname, client_id,
		 lease_start, lease_end, status, last_seen, generation)
		VALUES (?, ?, ?, 'aa:bb:cc:dd:ee:09', 'old-host', '',
		        datetime('now'), datetime('now', '+1 hour'), 'active', datetime('now'), 3)`,
		id, scopeID, ip); err != nil {
		t.Fatalf("insert control lease %s: %v", id, err)
	}
}

func TestTakeOverLeasesMovesTheRowsOnceAndOnlyOnce(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	controlLease(t, control, "l1", "s1", "10.0.0.10")
	if _, err := control.Exec(`INSERT INTO dhcp_dns_events (lease_id, generation, action)
		VALUES ('l1', 3, 'create')`); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	moved, err := rep.TakeOverLeases(ctx)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if moved != 2 {
		t.Errorf("rows taken over = %d, want 2 (one lease, one outbox event)", moved)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l1'`); got != 1 {
		t.Errorf("local leases = %d, want 1", got)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_dns_events WHERE lease_id = 'l1'`); got != 1 {
		t.Errorf("local outbox events = %d, want 1", got)
	}

	// Running it again must not resurrect anything an operator has removed.
	if _, err := store.Exec(`DELETE FROM dhcp_leases WHERE id = 'l1'`); err != nil {
		t.Fatalf("delete local lease: %v", err)
	}
	moved, err = rep.TakeOverLeases(ctx)
	if err != nil {
		t.Fatalf("second takeover: %v", err)
	}
	if moved != 0 {
		t.Errorf("the second takeover moved %d rows, want 0", moved)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l1'`); got != 0 {
		t.Error("a lease removed after the takeover came back")
	}
}

// TestTakeOverLeasesLeavesAnOwnedStoreAlone guards the case where the control
// database's rows are the stale copy: copying them in would undo every lease
// change made since the split.
func TestTakeOverLeasesLeavesAnOwnedStoreAlone(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	controlLease(t, control, "l-stale", "s1", "10.0.0.10")

	// The store already owns a lease, so it has been authoritative for a while.
	insertLease(t, store, "l-live", "s1", "10.0.0.11", "active")

	moved, err := rep.TakeOverLeases(ctx)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if moved != 0 {
		t.Errorf("rows taken over = %d, want 0", moved)
	}
	if got := countLocal(t, store, `SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l-stale'`); got != 0 {
		t.Error("the stale control row was copied over a store that already owns its leases")
	}
}

func TestPushCopiesLeaseChangesUpAndClearsTheMarkers(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}

	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")
	if n, err := rep.PendingLeases(); err != nil || n != 1 {
		t.Fatalf("pending = (%d, %v), want (1, nil)", n, err)
	}

	res, err := rep.Push(ctx, 100)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if res.Upserted != 1 || res.Deleted != 0 {
		t.Errorf("push = %+v, want one upsert", res)
	}
	if res.Pending != 0 {
		t.Errorf("pending after the push = %d, want 0", res.Pending)
	}

	var status, hostname string
	var generation int64
	if err := control.QueryRow(
		`SELECT status, hostname, generation FROM dhcp_leases WHERE id = 'l1'`).
		Scan(&status, &hostname, &generation); err != nil {
		t.Fatalf("read the replica: %v", err)
	}
	if status != "active" || generation != 1 {
		t.Errorf("replica = (%q, %d), want (active, 1)", status, generation)
	}
}

func TestPushRemovesReplicaRowsForDeletedLeases(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}
	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")
	if _, err := rep.Push(ctx, 100); err != nil {
		t.Fatalf("first push: %v", err)
	}

	if _, err := store.Exec(`DELETE FROM dhcp_leases WHERE id = 'l1'`); err != nil {
		t.Fatalf("delete local lease: %v", err)
	}
	res, err := rep.Push(ctx, 100)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if res.Deleted != 1 {
		t.Errorf("deleted = %d, want 1", res.Deleted)
	}

	var n int
	if err := control.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l1'`).Scan(&n); err != nil {
		t.Fatalf("count replica: %v", err)
	}
	if n != 0 {
		t.Error("a deleted lease is still in the control-plane replica")
	}
}

// TestPushKeepsItsMarkersWhenTheReplicaCannotBeWritten is the durability rule
// for the upward direction: a change that did not arrive must still be owed.
func TestPushKeepsItsMarkersWhenTheReplicaCannotBeWritten(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}
	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")

	if err := control.Close(); err != nil {
		t.Fatalf("close control: %v", err)
	}
	if _, err := rep.Push(ctx, 100); err == nil {
		t.Fatal("push succeeded against a closed control database")
	}
	if n, err := rep.PendingLeases(); err != nil || n != 1 {
		t.Fatalf("pending after a failed push = (%d, %v), want (1, nil)", n, err)
	}
}

func TestRunnerSnapshotReportsReplicationState(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	if err := runner.Prime(ctx); err != nil {
		t.Fatalf("prime: %v", err)
	}
	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")

	st := runner.Snapshot()
	if !st.ControlReachable {
		t.Error("a successful replication was reported as unreachable")
	}
	if st.LastError != "" {
		t.Errorf("LastError = %q, want empty", st.LastError)
	}
	if st.HeldLeases != 1 {
		t.Errorf("HeldLeases = %d, want 1", st.HeldLeases)
	}
	if st.PendingLeaseChanges != 1 {
		t.Errorf("PendingLeaseChanges = %d, want 1", st.PendingLeaseChanges)
	}
	state, ok := st.Domains[DomainDHCP]
	if !ok {
		t.Fatal("no replication state for the dhcp domain")
	}
	if state.Behind {
		t.Error("the store reports itself behind after a successful prime")
	}
	if state.SourceRevision == 0 {
		t.Error("the recorded revision is zero after a successful sync")
	}
}

func TestRunnerOncePushesWhatIsOwed(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	if err := runner.Prime(ctx); err != nil {
		t.Fatalf("prime: %v", err)
	}
	insertLease(t, store, "l1", "s1", "10.0.0.10", "active")

	runner.Once(ctx)

	var n int
	if err := control.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l1'`).Scan(&n); err != nil {
		t.Fatalf("count replica: %v", err)
	}
	if n != 1 {
		t.Error("the poll did not push the outstanding lease change")
	}
	if pending, err := rep.PendingLeases(); err != nil || pending != 0 {
		t.Errorf("pending after the poll = (%d, %v), want (0, nil)", pending, err)
	}
}

func TestStoreMetaIsAbsentBeforeTheTakeover(t *testing.T) {
	store := newStore(t, config.DataPlaneLease)
	v, err := store.Meta(metaMigratedLeases)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if v != "" {
		t.Errorf("meta %s = %q before any takeover, want empty", metaMigratedLeases, v)
	}
}
