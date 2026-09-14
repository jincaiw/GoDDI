package lease

import (
	"context"
	"errors"
	"testing"
)

func TestApplyWALEventTxUpdatesLeaseAndWatermarkTogether(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	if err := EnsureAppliedWatermark(ctx, store.DB); err != nil {
		t.Fatal(err)
	}
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")

	event := WALEvent{
		Version:  currentWALEventVersion,
		Seq:      1,
		Op:       WALEventUpsert,
		Mutation: MutationActivate,
		Lease: &Lease{
			ID: "lease-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa",
			LeaseStart: "2026-09-13T20:00:00Z", LeaseEnd: "2026-09-13T21:00:00Z",
			LastSeen: "2026-09-13T20:00:00Z", Status: LeaseStatusActive, Generation: 1,
		},
	}
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ApplyWALEventTx(ctx, tx, event)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := store.DB.QueryRow(`SELECT status FROM dhcp_leases WHERE id = ?`, "lease-1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(LeaseStatusActive) {
		t.Fatalf("status = %q, want active", status)
	}
	var applied int64
	if err := store.DB.QueryRow(`SELECT applied_seq FROM lease_projection_watermark WHERE domain = ?`, leaseProjectionWatermarkDomain).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("applied_seq = %d, want 1", applied)
	}
}

func TestApplyWALEventTxGapDoesNotWriteLeaseOrWatermark(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	if err := EnsureAppliedWatermark(ctx, store.DB); err != nil {
		t.Fatal(err)
	}
	event := WALEvent{
		Version: currentWALEventVersion, Seq: 2, Op: WALEventUpsert, Mutation: MutationOffer,
		Lease: &Lease{ID: "gap", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusOffered},
	}
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ApplyWALEventTx(ctx, tx, event)
	if !errors.Is(err, ErrWatermarkGap) || changed {
		t.Fatalf("result = changed=%v err=%v, want false ErrWatermarkGap", changed, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = ?`, "gap").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("gap lease count = %d, want 0", count)
	}
}

func TestApplyWALEventTxRollbackRevertsLeaseAndWatermark(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	if err := EnsureAppliedWatermark(ctx, store.DB); err != nil {
		t.Fatal(err)
	}
	event := WALEvent{
		Version: currentWALEventVersion, Seq: 1, Op: WALEventUpsert, Mutation: MutationOffer,
		Lease: &Lease{ID: "rollback", ScopeID: "scope-1", IPAddress: "192.0.2.11", MACAddress: "aa", Status: LeaseStatusOffered},
	}
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyWALEventTx(ctx, tx, event); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = ?`, "rollback").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback lease count = %d, want 0", count)
	}
	var applied int64
	if err := store.DB.QueryRow(`SELECT applied_seq FROM lease_projection_watermark WHERE domain = ?`, leaseProjectionWatermarkDomain).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("applied_seq = %d, want 0", applied)
	}
}
