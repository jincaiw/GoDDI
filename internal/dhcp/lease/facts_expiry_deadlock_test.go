package lease

import (
	"context"
	"testing"
	"time"
)

// TestExpireFactsBatchResolvesSpaceWithoutDeadlock reproduces the data-plane
// store shape: a single-connection lease store whose space resolver reads that
// same store. Resolving the space while the expiry transaction held the only
// connection deadlocked the whole data plane (DHCP receive loop, reconcile and
// sync all blocked). The resolver must run before the transaction opens. A
// context deadline keeps a regression failing fast instead of hanging.
func TestExpireFactsBatchResolvesSpaceWithoutDeadlock(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	// The data-plane lease store is capped at one connection (dataplane.Store).
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")
	l, err := manager.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:10", "host-10", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE dhcp_leases SET lease_end=datetime('now','-1 minute') WHERE id=?`, l.ID); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := writer.ExpireFactsBatch(ctx, "dhcp-node-a", func(*Lease) (string, error) {
		// Read the same single-connection store, exactly as the real
		// dataplane.ResolveIPAMSpaceByIP does.
		var one int
		if err := db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
			return "", err
		}
		return "space-1", nil
	})
	if err != nil {
		t.Fatalf("ExpireFactsBatch: %v", err)
	}
	if len(got) != 1 || got[0].ID != l.ID {
		t.Fatalf("expired = %+v, want [%s]", got, l.ID)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM dhcp_leases WHERE id=?`, l.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(LeaseStatusExpired) {
		t.Fatalf("status = %q, want expired", status)
	}
	var facts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events WHERE action=?`, string(MutationExpire)).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if facts != 1 {
		t.Fatalf("facts = %d, want 1", facts)
	}
}
