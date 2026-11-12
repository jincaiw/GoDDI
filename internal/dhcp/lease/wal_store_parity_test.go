package lease

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

type leaseStoreContract interface {
	CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*Lease, error)
	ReserveAddress(scopeID, ip, mac, hostname string) (*Lease, error)
	ActivateLease(id string, duration time.Duration) (*Lease, error)
	RenewLease(id string, duration time.Duration) (*Lease, error)
	GetLease(id string) (*Lease, error)
	GetLeaseByMAC(mac string) (*Lease, error)
	GetHeldLeaseByIP(ip string) (*Lease, error)
	FindAvailableIP(scopeID string) (string, error)
	ReleaseLease(id string) error
	QuarantineIP(scopeID, ip, mac string) (*Lease, error)
	ExpireLeases() ([]*Lease, error)
}

// runDHCPSequence drives the same realistic lease lifecycle through any store
// and returns the address FindAvailableIP reports at the end.
func runDHCPSequence(t *testing.T, s leaseStoreContract) string {
	t.Helper()
	ctx := context.Background()
	const scope = "scope-1"
	idByIP := func(ip string) string {
		l, err := s.GetHeldLeaseByIP(ip)
		if err != nil || l == nil {
			t.Fatalf("held lease at %s: %v %v", ip, l, err)
		}
		return l.ID
	}

	if _, err := s.ReserveAddress(scope, "192.0.2.10", "aa:01", "hostA"); err != nil {
		t.Fatalf("reserve .10: %v", err)
	}
	if _, err := s.ActivateLease(idByIP("192.0.2.10"), time.Hour); err != nil {
		t.Fatalf("activate .10: %v", err)
	}
	if _, err := s.RenewLease(idByIP("192.0.2.10"), 2*time.Hour); err != nil {
		t.Fatalf("renew .10: %v", err)
	}
	if _, err := s.ReserveAddress(scope, "192.0.2.11", "aa:02", "hostB"); err != nil {
		t.Fatalf("reserve .11: %v", err)
	}
	if err := s.ReleaseLease(idByIP("192.0.2.11")); err != nil {
		t.Fatalf("release .11: %v", err)
	}
	if _, err := s.QuarantineIP(scope, "192.0.2.12", "aa:03"); err != nil {
		t.Fatalf("quarantine .12: %v", err)
	}
	if _, err := s.CreateLease(scope, "192.0.2.13", "aa:04", "hostD", time.Hour); err != nil {
		t.Fatalf("create .13: %v", err)
	}
	if _, err := s.ExpireLeases(); err != nil {
		t.Fatalf("expire sweep: %v", err)
	}
	avail, err := s.FindAvailableIP(scope)
	if err != nil {
		t.Fatalf("find available: %v", err)
	}
	_ = ctx
	return avail
}

// leaseOutcome summarizes the durable rows by semantic fields only (ids and
// timestamps differ between a random-uuid store and a Manager run).
func leaseOutcome(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT ip_address, status, generation, mac_address, hostname FROM dhcp_leases`)
	if err != nil {
		t.Fatalf("read leases: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ip, status, mac, hostname string
		var gen int64
		if err := rows.Scan(&ip, &status, &gen, &mac, &hostname); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, ip+"|"+status+"|"+itoa64(gen)+"|"+mac+"|"+hostname)
	}
	sort.Strings(out)
	return out
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestWALLeaseStoreMatchesManagerSemantics(t *testing.T) {
	// SQLite-backed Manager run.
	mgrDB := newTestDB(t)
	seedScope(t, mgrDB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	mgr := NewManager(mgrDB)
	mgrAvail := runDHCPSequence(t, mgr)
	mgrOutcome := leaseOutcome(t, mgrDB)

	// Event-sourced store run.
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	walStore, err := OpenWALLeaseStore(context.Background(), WALLeaseStoreConfig{
		Path: filepath.Join(t.TempDir(), "leases.wal"), DB: store.DB, Capacity: 16,
	})
	if err != nil {
		t.Fatalf("OpenWALLeaseStore: %v", err)
	}
	walAvail := runDHCPSequence(t, walStore)
	if err := walStore.Drain(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if err := walStore.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	walOutcome := leaseOutcome(t, store.DB)

	if mgrAvail != walAvail {
		t.Fatalf("FindAvailableIP: manager=%s wal=%s", mgrAvail, walAvail)
	}
	if len(mgrOutcome) != len(walOutcome) {
		t.Fatalf("row count: manager=%d wal=%d\nmgr=%v\nwal=%v", len(mgrOutcome), len(walOutcome), mgrOutcome, walOutcome)
	}
	for i := range mgrOutcome {
		if mgrOutcome[i] != walOutcome[i] {
			t.Fatalf("row %d differs:\n manager=%v\n wal    =%v", i, mgrOutcome, walOutcome)
		}
	}
}

// TestWALLeaseStoreRestartRebuildsIndex proves a store reopened on the same WAL
// and database serves the same leases (recovery + snapshot load), and can keep
// mutating afterwards.
func TestWALLeaseStoreRestartRebuildsIndex(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	walPath := filepath.Join(t.TempDir(), "leases.wal")
	ctx := context.Background()

	first, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{Path: walPath, DB: store.DB, Capacity: 16})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.CreateLease("scope-1", "192.0.2.10", "aa:01", "hostA", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}

	second, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{Path: walPath, DB: store.DB, Capacity: 16})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer second.Close(ctx)
	held, err := second.GetHeldLeaseByIP("192.0.2.10")
	if err != nil || held == nil {
		t.Fatalf("reopened store lost the lease: %v %v", held, err)
	}
	if held.Status != LeaseStatusActive || held.Generation != 1 {
		t.Fatalf("reopened lease = %+v, want active gen1", held)
	}
	// A fresh mutation after reopen must still work and project.
	if _, err := second.RenewLease(held.ID, time.Hour); err != nil {
		t.Fatalf("renew after reopen: %v", err)
	}
	if err := second.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT generation FROM dhcp_leases WHERE ip_address='192.0.2.10'`).Scan(new(int64)); err != nil {
		t.Fatalf("projection after reopen+renew: %v", err)
	}
}
