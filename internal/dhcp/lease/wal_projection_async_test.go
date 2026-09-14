package lease

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewSQLiteProjectionApplierCommitsLeaseAndWatermark(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	applier, err := NewSQLiteProjectionApplier(context.Background(), store.DB, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()

	event := WALEvent{
		Version: currentWALEventVersion, Seq: 1, Op: WALEventUpsert, Mutation: MutationOffer,
		Lease: &Lease{ID: "async-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusOffered},
	}
	if err := applier.Submit(event); err != nil {
		t.Fatal(err)
	}
	waitForApplied(t, applier, 1)

	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = ?`, "async-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("lease count = %d, want 1", count)
	}
	var applied int64
	if err := store.DB.QueryRow(`SELECT applied_seq FROM lease_projection_watermark WHERE domain = ?`, leaseProjectionWatermarkDomain).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("applied_seq = %d, want 1", applied)
	}
}

func TestNewSQLiteProjectionApplierRejectsGapAndStops(t *testing.T) {
	store := newLeaseStore(t)
	applier, err := NewSQLiteProjectionApplier(context.Background(), store.DB, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()

	if err := applier.Submit(WALEvent{
		Version: currentWALEventVersion, Seq: 2, Op: WALEventUpsert, Mutation: MutationOffer,
		Lease: &Lease{ID: "gap-async", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusOffered},
	}); err != nil {
		t.Fatal(err)
	}
	waitForFailure(t, applier)
	if !errors.Is(applier.Failed(), ErrWatermarkGap) {
		t.Fatalf("failed = %v, want ErrWatermarkGap", applier.Failed())
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = ?`, "gap-async").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("gap lease count = %d, want 0", count)
	}
}

func TestNewSQLiteProjectionApplierReplaysDuplicateWithoutRewrite(t *testing.T) {
	store := newLeaseStore(t)
	applier, err := NewSQLiteProjectionApplier(context.Background(), store.DB, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()

	event := WALEvent{
		Version: currentWALEventVersion, Seq: 1, Op: WALEventUpsert, Mutation: MutationOffer,
		Lease: &Lease{ID: "duplicate", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusOffered},
	}
	if err := applier.Submit(event); err != nil {
		t.Fatal(err)
	}
	waitForApplied(t, applier, 1)
	if err := applier.Submit(event); err != nil {
		t.Fatal(err)
	}
	// Duplicate is accepted by the bounded queue and ignored by the transaction
	// bridge because its sequence is already durable.
	deadline := time.Now().Add(time.Second)
	for applier.QueueDepth() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := applier.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applier.Applied() != 1 {
		t.Fatalf("applied = %d, want 1", applier.Applied())
	}
}
