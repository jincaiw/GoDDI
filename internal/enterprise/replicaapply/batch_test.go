package replicaapply

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
)

func pair(t *testing.T, a *testApplier, first eventlog.Event, address string) []eventlog.Event {
	t.Helper()
	second := eventFixtureValues(t, a.cluster, a.leader, "99999999-9999-4999-8999-999999999999", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", address)
	second.Position.Seq = 2
	second.PrevHash = first.Position.Hash
	var err error
	second.Position.Hash, err = second.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return []eventlog.Event{first, second}
}
func TestPrivateBatchCommitAndReplayClosed(t *testing.T) {
	db, a, e, _ := setup(t)
	events := pair(t, a, e, "192.0.2.11")
	if !errors.Is(Batch(context.Background(), events), ErrNoAuthority) {
		t.Fatal("public batch open")
	}
	if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events); err != nil {
		t.Fatal(err)
	}
	counts(t, db, 2)
	if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events[1:]); err == nil {
		t.Fatal("suffix replay accepted")
	}
	counts(t, db, 2)
	if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events); !errors.Is(err, ErrStopped) {
		t.Fatalf("not stopped %v", err)
	}
}
func TestBatchRollbackCases(t *testing.T) {
	for _, kind := range []string{"second invalid", "duplicate", "ignore", "head", "same address", "before commit"} {
		t.Run(kind, func(t *testing.T) {
			db, a, e, _ := setup(t)
			events := pair(t, a, e, "192.0.2.11")
			switch kind {
			case "second invalid":
				events[1].Payload[0] ^= 1
			case "duplicate":
				events[1] = events[0]
			case "ignore":
				if _, err := db.Exec(`CREATE TRIGGER ignore_second BEFORE INSERT ON ipam_occupancies WHEN NEW.id='99999999-9999-4999-8999-999999999999' BEGIN SELECT RAISE(IGNORE); END`); err != nil {
					t.Fatal(err)
				}
			case "head":
				if _, err := db.Exec(`UPDATE replica_head SET hash=zeroblob(32),seq=1`); err != nil {
					t.Fatal(err)
				}
			case "same address":
				events = pair(t, a, e, "192.0.2.10")
			case "before commit":
				a.beforeCommit = func() error { return errors.New("injected") }
			}
			if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events); err == nil {
				t.Fatal("accepted failed batch")
			}
			if kind != "head" {
				counts(t, db, 0)
			} else {
				for _, table := range []string{"ipam_occupancies", "static_occupancy_audit", "idempotency_receipt", "replica_event"} {
					var n int
					if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
						t.Fatalf("%s %d %v", table, n, err)
					}
				}
			}
			if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events); !errors.Is(err, ErrStopped) {
				t.Fatalf("not stopped %v", err)
			}
		})
	}
}
func TestBatchRequiresExactIndependentAnchor(t *testing.T) {
	db, a, first, _ := setup(t)
	events := pair(t, a, first, "192.0.2.11")
	if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events[:1]); err != nil {
		t.Fatal(err)
	}
	if err := a.applyBatchFixture(context.Background(), first.Position, events[1:]); err != nil {
		t.Fatal(err)
	}
	counts(t, db, 2)
}

func TestBatchBoundsRejectBeforeWrites(t *testing.T) {
	db, a, first, _ := setup(t)
	items := make([]eventlog.Event, fixtureMaxBatchEvents+1)
	for i := range items {
		items[i] = first
	}
	if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, items); err == nil {
		t.Fatal("oversized batch accepted")
	}
	counts(t, db, 0)
}

func TestBatchCommitUncertaintyStops(t *testing.T) {
	db, a, e, _ := setup(t)
	events := pair(t, a, e, "192.0.2.11")
	a.afterCommit = func() error { return errors.New("injected after commit") }
	if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events); err == nil {
		t.Fatal("accepted uncertain result")
	}
	counts(t, db, 2)
	if err := a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events); !errors.Is(err, ErrStopped) {
		t.Fatal(err)
	}
}
func TestBatchConcurrentCallsSerialize(t *testing.T) {
	db, a, e, _ := setup(t)
	events := pair(t, a, e, "192.0.2.11")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- a.applyBatchFixture(context.Background(), eventlog.Position{Epoch: 1}, events)
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("successes %d", wins)
	}
	counts(t, db, 2)
}
