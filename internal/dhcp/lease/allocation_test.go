package lease

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newAllocDB builds the minimum schema FindAvailableIP needs: the scope bounds,
// the lease table, and reservations.
func newAllocDB(t *testing.T) *sql.DB {
	t.Helper()

	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.12")
	return store.DB
}

func backdate(t *testing.T, db *sql.DB, id string, d time.Duration) {
	t.Helper()
	if _, err := db.Exec(`UPDATE dhcp_leases SET lease_end = ? WHERE id = ?`,
		time.Now().UTC().Add(d).Format("2006-01-02T15:04:05Z"), id); err != nil {
		t.Fatalf("backdate lease_end: %v", err)
	}
}

func statusOf(t *testing.T, db *sql.DB, id string) LeaseStatus {
	t.Helper()
	var s LeaseStatus
	if err := db.QueryRow("SELECT status FROM dhcp_leases WHERE id = ?", id).Scan(&s); err != nil {
		t.Fatalf("read status: %v", err)
	}
	return s
}

// Regression: a DISCOVER used to write a fully active lease with the whole
// lease time, so a client that never sent REQUEST — or anything walking the
// range — held addresses out of the pool for hours.
func TestReserveAddress_CreatesOfferedNotActive(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	l, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01", "host1")
	if err != nil {
		t.Fatalf("ReserveAddress: %v", err)
	}
	if l.Status != LeaseStatusOffered {
		t.Errorf("status = %s, want %s", l.Status, LeaseStatusOffered)
	}

	// The hold must be short: it exists to cover the OFFER/REQUEST window, not
	// to occupy the address for the lease duration.
	held := time.Until(mustParseTime(t, l.LeaseEnd))
	if held > offerHold+time.Minute {
		t.Errorf("offer held for %s, want at most %s", held, offerHold)
	}
}

func TestReserveAddress_RejectsAddressHeldByAnotherClient(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	if _, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01", "host1"); err != nil {
		t.Fatalf("first reserve: %v", err)
	}

	_, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:02", "host2")
	if !errors.Is(err, ErrAddressTaken) {
		t.Fatalf("err = %v, want ErrAddressTaken", err)
	}

	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM dhcp_leases WHERE ip_address = '192.0.2.10'").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("got %d lease rows for the address, want 1", n)
	}
}

func TestReserveAddress_RefreshesHoldOnRetransmit(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	first, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01", "host1")
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	backdate(t, db, first.ID, -offerHold+10*time.Second)

	// A retransmitted DISCOVER must refresh the hold in place instead of
	// racing the unique index with a second row.
	second, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01", "host1")
	if err != nil {
		t.Fatalf("retransmit reserve: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("retransmit created a new lease row (%s vs %s)", second.ID, first.ID)
	}
	if time.Until(mustParseTime(t, second.LeaseEnd)) < 30*time.Second {
		t.Errorf("hold was not refreshed: lease_end = %s", second.LeaseEnd)
	}
}

func TestActivateLease_PromotesOfferWithFullLeaseTime(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	offer, err := m.ReserveAddress("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:03", "host3")
	if err != nil {
		t.Fatalf("ReserveAddress: %v", err)
	}

	active, err := m.ActivateLease(offer.ID, 12*time.Hour)
	if err != nil {
		t.Fatalf("ActivateLease: %v", err)
	}
	if active.Status != LeaseStatusActive {
		t.Errorf("status = %s, want active", active.Status)
	}
	if got := time.Until(mustParseTime(t, active.LeaseEnd)); got < 11*time.Hour {
		t.Errorf("lease ends in %s, want ~12h", got)
	}

	// A leased row cannot be activated twice from a non-existent state.
	if _, err := m.ActivateLease("missing", time.Hour); err == nil {
		t.Error("ActivateLease on a missing lease should fail")
	}
}

// Regression: DECLINE only relabelled the lease, while the availability query
// looked at 'active' alone — so the declined address was handed straight to the
// next client.
func TestFindAvailableIP_SkipsDeclinedAddress(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	// The whole pool starts with one offered address; decline it.
	offer, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:04", "host4")
	if err != nil {
		t.Fatalf("ReserveAddress: %v", err)
	}
	if err := m.MarkLeaseConflict(offer.ID); err != nil {
		t.Fatalf("MarkLeaseConflict: %v", err)
	}

	got, err := m.FindAvailableIP("scope-1")
	if err != nil {
		t.Fatalf("FindAvailableIP: %v", err)
	}
	if got == "192.0.2.10" {
		t.Fatal("FindAvailableIP re-offered the declined address")
	}
	if got != "192.0.2.11" {
		t.Errorf("got %s, want the next address 192.0.2.11", got)
	}
}

func TestQuarantineIP_WritesTombstoneWhenNoLeaseExists(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	// A DECLINE can arrive for an address we never leased (statically
	// configured host, or a conflict observed by a peer).
	tombstone, err := m.QuarantineIP("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:05")
	if err != nil {
		t.Fatalf("QuarantineIP: %v", err)
	}
	if tombstone == nil || tombstone.Status != LeaseStatusConflict || tombstone.IPAddress != "192.0.2.10" {
		t.Fatalf("tombstone = %#v, want returned conflict lease", tombstone)
	}

	got, err := m.FindAvailableIP("scope-1")
	if err != nil {
		t.Fatalf("FindAvailableIP: %v", err)
	}
	if got == "192.0.2.10" {
		t.Fatal("quarantined address without a lease was offered again")
	}

	// Quarantining an address that already holds a lease reuses the row.
	leaseRow, err := m.ReserveAddress("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:06", "host6")
	if err != nil {
		t.Fatalf("ReserveAddress: %v", err)
	}
	if _, err := m.QuarantineIP("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:06"); err != nil {
		t.Fatalf("QuarantineIP on an existing lease: %v", err)
	}
	if got := statusOf(t, db, leaseRow.ID); got != LeaseStatusConflict {
		t.Errorf("status = %s, want conflict", got)
	}
	var rows int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM dhcp_leases WHERE ip_address = '192.0.2.11'").Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("got %d rows for the quarantined address, want 1", rows)
	}
}

func TestQuarantineIP_RequiresScope(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	if _, err := m.QuarantineIP("", "192.0.2.10", "aa:bb:cc:dd:ee:07"); err == nil {
		t.Error("QuarantineIP without a scope should fail (scope_id is a foreign key)")
	}
}

// An address held by a state that no sweep covers would leak out of the pool
// forever, so the expiry sweep must cover every held status.
func TestExpireLeases_ReleasesOfferedAndConflict(t *testing.T) {
	db := newAllocDB(t)
	m := NewManager(db)

	offer, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:08", "host8")
	if err != nil {
		t.Fatalf("ReserveAddress: %v", err)
	}
	conflict, err := m.ReserveAddress("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:09", "host9")
	if err != nil {
		t.Fatalf("ReserveAddress: %v", err)
	}
	if err := m.MarkLeaseConflict(conflict.ID); err != nil {
		t.Fatalf("MarkLeaseConflict: %v", err)
	}

	// The quarantine is in the future, so nothing should expire yet.
	if _, err := m.ExpireLeases(); err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	if got := statusOf(t, db, offer.ID); got != LeaseStatusOffered {
		t.Errorf("offer expired early: status = %s", got)
	}
	if got := statusOf(t, db, conflict.ID); got != LeaseStatusConflict {
		t.Errorf("quarantine expired early: status = %s", got)
	}

	backdate(t, db, offer.ID, -time.Second)
	backdate(t, db, conflict.ID, -time.Second)
	if _, err := m.ExpireLeases(); err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}
	if got := statusOf(t, db, offer.ID); got != LeaseStatusExpired {
		t.Errorf("offer status = %s, want expired", got)
	}
	if got := statusOf(t, db, conflict.ID); got != LeaseStatusExpired {
		t.Errorf("quarantine status = %s, want expired", got)
	}

	// Both addresses must be back in the pool.
	got, err := m.FindAvailableIP("scope-1")
	if err != nil {
		t.Fatalf("FindAvailableIP after expiry: %v", err)
	}
	if got != "192.0.2.10" {
		t.Errorf("got %s, want 192.0.2.10 to be reusable", got)
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	for _, layout := range []string{"2006-01-02T15:04:05Z", time.RFC3339} {
		if ts, err := time.Parse(layout, s); err == nil {
			return ts
		}
	}
	t.Fatalf("unparseable timestamp %q", s)
	return time.Time{}
}
