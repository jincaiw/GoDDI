package lease

import (
	"context"
	"errors"
	"testing"
)

func TestRecoverSQLiteProjectionReplaysAfterPersistedWatermark(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	wal, err := OpenWAL(t.TempDir() + "/leases.wal")
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()

	first := WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert, Mutation: MutationOffer, Lease: &Lease{
		ID: "recover-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusOffered,
	}}
	second := WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert, Mutation: MutationActivate, Lease: &Lease{
		ID: "recover-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusActive, Generation: 1,
	}}
	if first, err = wal.AppendDurable(first); err != nil {
		t.Fatal(err)
	}
	if second, err = wal.AppendDurable(second); err != nil {
		t.Fatal(err)
	}

	if err := EnsureAppliedWatermark(context.Background(), store.DB); err != nil {
		t.Fatal(err)
	}
	tx, err := store.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyWALEventTx(context.Background(), tx, first); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	last, err := RecoverSQLiteProjection(context.Background(), wal, store.DB)
	if err != nil {
		t.Fatal(err)
	}
	if last != second.Seq {
		t.Fatalf("last = %d, want %d", last, second.Seq)
	}
	var status string
	if err := store.DB.QueryRow(`SELECT status FROM dhcp_leases WHERE id = ?`, "recover-1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(LeaseStatusActive) {
		t.Fatalf("status = %q, want active", status)
	}
}

func TestRecoverSQLiteProjectionApplyFailureLeavesProjectionUnchanged(t *testing.T) {
	store := newLeaseStore(t)
	wal, err := OpenWAL(t.TempDir() + "/leases.wal")
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	event := WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert, Mutation: MutationOffer, Lease: &Lease{
		ID: "recover-invalid", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusOffered,
	}}
	if _, err := wal.AppendDurable(event); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverSQLiteProjection(context.Background(), wal, store.DB); !errors.Is(err, ErrStartupNotReady) {
		t.Fatalf("error = %v, want ErrStartupNotReady", err)
	}
	var applied int64
	if err := store.DB.QueryRow(`SELECT applied_seq FROM lease_projection_watermark WHERE domain = ?`, leaseProjectionWatermarkDomain).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("applied_seq = %d, want unchanged 0", applied)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = ?`, "recover-invalid").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid lease count = %d, want 0", count)
	}
}

func TestRecoverSQLiteProjectionRejectsNilDependencies(t *testing.T) {
	if _, err := RecoverSQLiteProjection(context.Background(), nil, nil); !errors.Is(err, ErrStartupNotReady) {
		t.Fatalf("error = %v, want ErrStartupNotReady", err)
	}
}
