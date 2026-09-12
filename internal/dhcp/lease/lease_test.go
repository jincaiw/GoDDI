package lease

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return newLeaseStore(t).DB
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
	if _, err := m.ExpireLeases(); err != nil {
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

	if _, err := m.ExpireLeases(); err != nil {
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
	if _, err := m.ExpireLeases(); err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	got, _ := m.GetLease(lease.ID)
	if got.Status != LeaseStatusReleased {
		t.Fatalf("released lease changed status to %s", got.Status)
	}
}

// TestTheSearchBoxMatchesTheColumnsItStandsFor.
//
// The lease list has one search input, labelled "IP/MAC", and it sends the term
// as `search`. Nothing read that parameter, so the box answered every query
// with the whole table -- and a table of everything reads as a table of
// matches. This pins the two things that make it a filter: it matches all three
// columns a lease has, and it does not match an address it was not given.
func TestTheSearchBoxMatchesTheColumnsItStandsFor(t *testing.T) {
	db := newTestDB(t)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")

	for _, row := range []struct{ id, ip, mac, hostname string }{
		{"l-20", "192.0.2.20", "aa:bb:cc:dd:ee:20", "host-a"},
		{"l-200", "192.0.2.200", "aa:bb:cc:dd:ee:21", "host-b"},
	} {
		if _, err := db.Exec(`
			INSERT INTO dhcp_leases
				(id, scope_id, ip_address, mac_address, hostname, status, lease_start, lease_end, last_seen)
			VALUES (?, 'scope-1', ?, ?, ?, 'active', datetime('now'), datetime('now', '+1 hour'), datetime('now'))`,
			row.id, row.ip, row.mac, row.hostname); err != nil {
			t.Fatalf("seed lease %s: %v", row.ip, err)
		}
	}

	manager := NewManager(db)
	search := func(term string) string {
		t.Helper()
		list, total, err := manager.ListLeases(LeaseFilter{Search: term})
		if err != nil {
			t.Fatalf("searching %q: %v", term, err)
		}
		// total comes from a second query with the same WHERE clause, so a
		// mismatch means the two disagree about what matched.
		if int(total) != len(list) {
			t.Fatalf("searching %q: total = %d but %d row(s) came back", term, total, len(list))
		}
		found := make([]string, 0, len(list))
		for _, l := range list {
			found = append(found, l.IPAddress)
		}
		return strings.Join(found, ",")
	}

	cases := map[string]string{
		// A full address is exact. As a substring it would also return
		// 192.0.2.200, and "which lease holds this address" would come back
		// with its neighbours.
		"192.0.2.20": "192.0.2.20",
		"ee:21":      "192.0.2.200",
		"host-b":     "192.0.2.200",
		// A term nothing carries returns nothing, which is the property the
		// old behaviour could not have: it returned everything.
		"no-such-client": "",
	}
	for term, want := range cases {
		if got := search(term); got != want {
			t.Errorf("searching %q matched %q, want %q", term, got, want)
		}
	}
}
