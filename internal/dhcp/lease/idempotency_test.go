package lease

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func leaseRowsForIP(t *testing.T, m *Manager, ip string) int {
	t.Helper()
	var n int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE ip_address = ?`, ip).Scan(&n); err != nil {
		t.Fatalf("count leases for %s: %v", ip, err)
	}
	return n
}

// TestRetransmittedDiscoverIsIdempotent proves a retransmitted DISCOVER from the
// same client refreshes the existing hold instead of allocating a second row
// (the D-11 idempotency guarantee on the offer half of DORA).
func TestRetransmittedDiscoverIsIdempotent(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	m := NewManager(store.DB)

	l1, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc", "host")
	if err != nil {
		t.Fatalf("first offer: %v", err)
	}
	l2, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc", "host")
	if err != nil {
		t.Fatalf("retransmitted offer: %v", err)
	}
	if l1.ID != l2.ID {
		t.Fatalf("retransmit allocated a second hold: %s vs %s", l1.ID, l2.ID)
	}
	if n := leaseRowsForIP(t, m, "192.0.2.10"); n != 1 {
		t.Fatalf("rows for retransmitted DISCOVER = %d, want 1", n)
	}

	// A different client mining the same address must be refused, not served.
	if _, err := m.ReserveAddress("scope-1", "192.0.2.10", "de:ad:be", "other"); !errors.Is(err, ErrAddressTaken) {
		t.Fatalf("different MAC on a held IP = %v, want ErrAddressTaken", err)
	}
}

// TestRepeatedActivateAndReleaseAreIdempotent proves replaying REQUEST/release
// does not create rows or fail: renewal extends the one lease, release of a
// gone/released lease is a silent success.
func TestRepeatedActivateAndReleaseAreIdempotent(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	m := NewManager(store.DB)

	held, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc", "host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ActivateLease(held.ID, time.Hour); err != nil {
		t.Fatalf("activate: %v", err)
	}
	// A replayed REQUEST renews the same row (no second lease).
	if _, err := m.ActivateLease(held.ID, time.Hour); err != nil {
		t.Fatalf("re-activate/renew: %v", err)
	}
	if n := leaseRowsForIP(t, m, "192.0.2.10"); n != 1 {
		t.Fatalf("rows after replay = %d, want 1", n)
	}
	if err := m.ReleaseLease(held.ID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := m.ReleaseLease(held.ID); err != nil {
		t.Fatalf("repeated release must be a silent success, got %v", err)
	}
}

// TestConcurrentSameAddressHasOneWinner proves the unique (scope,ip) index keeps
// exactly one winner when two clients race for the same address.
func TestConcurrentSameAddressHasOneWinner(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	m := NewManager(store.DB)

	var wg sync.WaitGroup
	results := make([]error, 2)
	macs := []string{"aa:aa:aa", "bb:bb:bb"}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = m.ReserveAddress("scope-1", "192.0.2.10", macs[i], "host")
		}(i)
	}
	wg.Wait()

	okCount, takenCount := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			okCount++
		case errors.Is(err, ErrAddressTaken):
			takenCount++
		default:
			t.Fatalf("unexpected reserve error: %v", err)
		}
	}
	if okCount != 1 || takenCount != 1 {
		t.Fatalf("concurrent reserve: ok=%d taken=%d, want 1/1", okCount, takenCount)
	}
	if n := leaseRowsForIP(t, m, "192.0.2.10"); n != 1 {
		t.Fatalf("rows after concurrent race = %d, want 1", n)
	}
}
