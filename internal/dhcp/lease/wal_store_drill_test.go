package lease

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestWALLeaseStoreReplaysUnprojectedMutationsAfterCrash reproduces an
// ungraceful crash: the mutation is durable in the WAL but the process dies
// before the SQLite projection (and its fact) commit. Reopening the store must
// replay the durable event, project the lease, and enqueue the owed fact
// exactly once — a crash must not lose a lease or a fact.
func TestWALLeaseStoreReplaysUnprojectedMutationsAfterCrash(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	path := filepath.Join(t.TempDir(), "leases.wal")

	// Durable event appended, then the "process" dies before projecting.
	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	leaseValue := Lease{
		ID: "crash-lease", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa:01",
		LeaseStart: time.Now().UTC().Format(leaseTimeLayout), LeaseEnd: time.Now().UTC().Add(time.Hour).Format(leaseTimeLayout),
		Status: LeaseStatusActive, LastSeen: time.Now().UTC().Format(leaseTimeLayout), Generation: 1,
	}
	if _, err := wal.AppendDurable(WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert,
		Mutation: MutationBind, SpaceID: "space-1", Lease: &leaseValue}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil { // crash: nothing projected
		t.Fatal(err)
	}

	ctx := context.Background()
	s, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{Path: path, DB: store.DB, Capacity: 8,
		Source: "node-a", ResolveSpaceID: func(scopeID, ip string) (string, error) { return "space-1", nil }})
	if err != nil {
		t.Fatalf("reopen after crash: %v", err)
	}
	held, err := s.GetHeldLeaseByIP("192.0.2.10")
	if err != nil || held == nil || held.Status != LeaseStatusActive || held.Generation != 1 {
		t.Fatalf("replayed lease = %+v err=%v, want active gen1", held, err)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id='crash-lease'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("projected lease count=%d err=%v, want 1", count, err)
	}
	facts := factRows(t, store.DB)
	if len(facts) != 1 || facts[0] != "request_bind@1/node-a" {
		t.Fatalf("recovered facts = %v, want [request_bind@1/node-a]", facts)
	}
	// Reopening again must not duplicate the recovered fact.
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{Path: path, DB: store.DB, Capacity: 8,
		Source: "node-a", ResolveSpaceID: func(scopeID, ip string) (string, error) { return "space-1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close(ctx)
	if facts := factRows(t, store.DB); len(facts) != 1 {
		t.Fatalf("second reopen duplicated facts: %v", facts)
	}
}

// TestWALLeaseStoreRollbackToSQLiteManagerSeesTheSameData proves the rollback
// path: after the WAL store has projected its writes, a plain SQLite *Manager on
// the same database reads exactly the same leases. Rolling the config back to
// sqlite therefore does not lose data.
func TestWALLeaseStoreRollbackToSQLiteManagerSeesTheSameData(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	ctx := context.Background()
	s, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{Path: filepath.Join(t.TempDir(), "leases.wal"),
		DB: store.DB, Capacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLease("scope-1", "192.0.2.10", "aa:01", "hostA", time.Hour); err != nil {
		t.Fatal(err)
	}
	held, _ := s.GetHeldLeaseByIP("192.0.2.10")
	if _, err := s.RenewLease(held.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}

	// Rollback: the SQLite manager reads the projected rows unchanged.
	mgr := NewManager(store.DB)
	byMAC, err := mgr.GetLeaseByMAC("aa:01")
	if err != nil || byMAC == nil {
		t.Fatalf("rollback manager GetLeaseByMAC: %+v err=%v", byMAC, err)
	}
	if byMAC.Status != LeaseStatusActive || byMAC.Generation != 2 {
		t.Fatalf("rollback manager lease = %+v, want active gen2", byMAC)
	}
}

// TestWALLeaseStoreReloadFromDBServesMirrorPopulatedRows proves standby-wal
// promotion readiness: a standby's SQLite lease DB is filled by the HA mirror
// while no DHCP server runs on it. When the node is promoted, a WAL store that
// reloads from that DB must serve the mirrored leases (and allocate around
// them) before taking new traffic.
func TestWALLeaseStoreReloadFromDBServesMirrorPopulatedRows(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	ctx := context.Background()
	s, err := OpenWALLeaseStore(ctx, WALLeaseStoreConfig{Path: filepath.Join(t.TempDir(), "leases.wal"),
		DB: store.DB, Capacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)

	// Simulate the HA mirror applying a primary snapshot directly to SQLite.
	for _, row := range [][2]string{{"mirror-1", "192.0.2.10"}, {"mirror-2", "192.0.2.11"}} {
		if _, err := store.DB.Exec(`INSERT INTO dhcp_leases
			(id, scope_id, ip_address, mac_address, hostname, client_id, lease_start, lease_end, status, last_seen, generation)
			VALUES (?, 'scope-1', ?, 'aa:bb', 'mirrored', '', ?, ?, 'active', ?, 1)`,
			row[0], row[1], time.Now().UTC().Format(leaseTimeLayout),
			time.Now().UTC().Add(time.Hour).Format(leaseTimeLayout), time.Now().UTC().Format(leaseTimeLayout)); err != nil {
			t.Fatalf("mirror insert: %v", err)
		}
	}
	if _, ok := s.Index().Get("mirror-1"); ok {
		t.Fatal("index already saw a row it never loaded (mirror bypasses the store)")
	}
	if err := s.ReloadFromDB(ctx); err != nil {
		t.Fatalf("ReloadFromDB: %v", err)
	}
	for _, ip := range []string{"192.0.2.10", "192.0.2.11"} {
		held, err := s.GetHeldLeaseByIP(ip)
		if err != nil || held == nil {
			t.Fatalf("mirrored lease at %s not served after reload: %v %v", ip, held, err)
		}
	}
	// Allocation must skip the mirrored addresses.
	avail, err := s.FindAvailableIP("scope-1")
	if err != nil {
		t.Fatal(err)
	}
	if avail == "192.0.2.10" || avail == "192.0.2.11" {
		t.Fatalf("FindAvailableIP returned a mirrored address: %s", avail)
	}
	if _, err := s.CreateLease("scope-1", avail, "cc:dd", "new", time.Hour); err != nil {
		t.Fatalf("create after reload: %v", err)
	}
}
