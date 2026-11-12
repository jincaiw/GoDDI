package dataplane

import (
	"context"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

// TestSyncWritesAtomicLKG proves the LKG stamp is written in the same sync as
// the configuration and reflects the control revision, and that a no-op sync
// leaves it unchanged.
func TestSyncWritesAtomicLKG(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	res, err := rep.Sync(ctx, DomainDHCP)
	if err != nil || !res.Applied {
		t.Fatalf("sync: applied=%v err=%v", res.Applied, err)
	}
	var controlRev int64
	if err := control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = ?`, string(DomainDHCP)).Scan(&controlRev); err != nil {
		t.Fatalf("control revision: %v", err)
	}
	rev, at, ok, err := LKG(store.DB, DomainDHCP)
	if err != nil || !ok {
		t.Fatalf("LKG ok=%v err=%v", ok, err)
	}
	if rev != controlRev {
		t.Fatalf("LKG revision = %d, want control %d", rev, controlRev)
	}
	if at.IsZero() {
		t.Fatal("LKG applied_at is zero")
	}
	if store.DSN() == "" {
		t.Fatal("store DSN empty")
	}
	_ = config.DataPlaneLease

	// A no-op sync must not change the stamp revision.
	before := rev
	if _, err := rep.Sync(ctx, DomainDHCP); err != nil {
		t.Fatal(err)
	}
	rev2, _, _, _ := LKG(store.DB, DomainDHCP)
	if rev2 != before {
		t.Fatalf("LKG changed on a no-op sync: %d -> %d", before, rev2)
	}
}
