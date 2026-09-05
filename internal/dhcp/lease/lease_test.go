package lease

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	schema := `
	CREATE TABLE dhcp_leases (
		id TEXT PRIMARY KEY,
		scope_id TEXT NOT NULL,
		ip_address TEXT NOT NULL,
		mac_address TEXT NOT NULL,
		hostname TEXT,
		client_id TEXT,
		lease_start TEXT NOT NULL,
		lease_end TEXT NOT NULL,
		status TEXT NOT NULL,
		last_seen TEXT
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// Regression: lease_end is stored in RFC3339 ("T" separator); ExpireLeases
// used to compare it lexicographically against datetime('now') output
// (" " separator), so leases expiring on the current day were never matched
// ('T' > ' '). The julianday() comparison must expire them.
func TestExpireLeasesRFC3339(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)

	lease, err := m.CreateLease("scope-1", "192.168.10.100", "aa:bb:cc:dd:ee:01", "host1", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	if lease.Status != LeaseStatusActive {
		t.Fatalf("expected active lease, got %s", lease.Status)
	}

	// Not expired yet: still active.
	if err := m.ExpireLeases(); err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	got, err := m.GetLease(lease.ID)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got.Status != LeaseStatusActive {
		t.Fatalf("lease should still be active, got %s", got.Status)
	}

	// Simulate a lease that expired half an hour ago.
	if _, err := db.Exec(`UPDATE dhcp_leases SET lease_end = ? WHERE id = ?`,
		time.Now().UTC().Add(-30*time.Minute).Format("2006-01-02T15:04:05Z"), lease.ID); err != nil {
		t.Fatalf("backdate lease_end: %v", err)
	}

	if err := m.ExpireLeases(); err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	got, err = m.GetLease(lease.ID)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got.Status != LeaseStatusExpired {
		t.Fatalf("lease should be expired, got %s (lease_end=%s)", got.Status, got.LeaseEnd)
	}
}

func TestExpireLeasesKeepsFutureLeaseWithOtherStatus(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)

	// A released lease must not be resurrected by ExpireLeases.
	lease, err := m.CreateLease("scope-1", "192.168.10.101", "aa:bb:cc:dd:ee:02", "host2", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	if err := m.ReleaseLease(lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := m.ExpireLeases(); err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	got, _ := m.GetLease(lease.ID)
	if got.Status != LeaseStatusReleased {
		t.Fatalf("released lease changed status to %s", got.Status)
	}
}
