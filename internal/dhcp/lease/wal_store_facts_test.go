package lease

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func factRows(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT action, source, sequence FROM dhcp_ipam_observation_events ORDER BY sequence`)
	if err != nil {
		t.Fatalf("read facts: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var action, source string
		var seq int64
		if err := rows.Scan(&action, &source, &seq); err != nil {
			t.Fatal(err)
		}
		out = append(out, action+"@"+itoa64(seq)+"/"+source)
	}
	return out
}

// TestWALLeaseStoreDerivesFactsFromTheSameWALEvent proves the store feeds the
// unified facts chain from the WAL: bind/renew/release each produce exactly one
// fact with a contiguous sequence, and an OFFER produces none.
func TestWALLeaseStoreDerivesFactsFromTheSameWALEvent(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	ctx := context.Background()
	s, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{
		Path: filepath.Join(t.TempDir(), "leases.wal"), DB: store.DB, Capacity: 16,
		Source: "node-a", ResolveSpaceID: func(scopeID, ip string) (string, error) { return "space-1", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	// Offer must produce no fact.
	if _, err := s.ReserveAddress("scope-1", "192.0.2.10", "aa:01", "hostA"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	held, _ := s.GetHeldLeaseByIP("192.0.2.10")
	if _, err := s.ActivateLease(held.ID, time.Hour); err != nil {
		t.Fatalf("activate (bind): %v", err)
	}
	if _, err := s.RenewLease(held.ID, time.Hour); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := s.ReleaseLease(held.ID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	got := factRows(t, store.DB)
	want := []string{"request_activate@1/node-a", "request_renew@2/node-a", "release@3/node-a"}
	if len(got) != len(want) {
		t.Fatalf("facts = %v, want %v (offer must not emit)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fact %d = %s, want %s (all=%v)", i, got[i], want[i], got)
		}
	}
}

// TestWALLeaseStoreOmitsFactWithoutSpaceMapping proves a missing IPAM space
// mapping omits the fact (like the synchronous seam) rather than failing the
// lease write.
func TestWALLeaseStoreOmitsFactWithoutSpaceMapping(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	ctx := context.Background()
	s, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{
		Path: filepath.Join(t.TempDir(), "leases.wal"), DB: store.DB, Capacity: 8,
		Source: "node-a", ResolveSpaceID: func(scopeID, ip string) (string, error) { return "", sql.ErrNoRows },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLease("scope-1", "192.0.2.10", "aa:01", "hostA", time.Hour); err != nil {
		t.Fatalf("create without space: %v", err)
	}
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if got := factRows(t, store.DB); len(got) != 0 {
		t.Fatalf("facts = %v, want none without a space mapping", got)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE ip_address='192.0.2.10'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("lease not projected: count=%d err=%v", count, err)
	}
}

// TestWALLeaseStoreFactsAreIdempotentAcrossRestart proves reopening the store
// on the same WAL does not duplicate already-projected facts.
func TestWALLeaseStoreFactsAreIdempotentAcrossRestart(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	ctx := context.Background()
	cfg := WALLeaseStoreConfig{Path: filepath.Join(t.TempDir(), "leases.wal"), DB: store.DB, Capacity: 8,
		Source: "node-a", ResolveSpaceID: func(scopeID, ip string) (string, error) { return "space-1", nil }}
	first, err := OpenWALLeaseStore(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.CreateLease("scope-1", "192.0.2.10", "aa:01", "hostA", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	before := factRows(t, store.DB)
	if len(before) != 1 || before[0] != "request_bind@1/node-a" {
		t.Fatalf("after first run facts=%v", before)
	}
	second, err := OpenWALLeaseStore(ctx, cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer second.Close(ctx)
	if err := second.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	after := factRows(t, store.DB)
	if len(after) != 1 {
		t.Fatalf("restart duplicated facts: before=%v after=%v", before, after)
	}
	// A new mutation after restart continues the sequence.
	held, _ := second.GetHeldLeaseByIP("192.0.2.10")
	if held == nil {
		t.Fatal("reopened store lost the lease")
	}
	if _, err := second.RenewLease(held.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := second.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if rows := factRows(t, store.DB); len(rows) != 2 || rows[1] != "request_renew@2/node-a" {
		t.Fatalf("post-restart facts=%v", rows)
	}
	_ = errors.Is
}
