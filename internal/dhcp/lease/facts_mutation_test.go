package lease

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/facts"
)

func newFactsMutationWriter(t *testing.T) (*FactsMutationWriter, *sql.DB, *Manager) {
	t.Helper()
	db := newTestDB(t)
	manager := NewManager(db)
	allocator, err := facts.NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := NewFactsMutationWriter(manager, allocator, outbox)
	if err != nil {
		t.Fatal(err)
	}
	return writer, db, manager
}

func TestNewFactsMutationWriterRejectsSplitDatabases(t *testing.T) {
	managerDB := newTestDB(t)
	t.Cleanup(func() { _ = managerDB.Close() })
	allocatorDB := newTestDB(t)
	t.Cleanup(func() { _ = allocatorDB.Close() })
	outboxDB := newTestDB(t)
	t.Cleanup(func() { _ = outboxDB.Close() })
	allocator, err := facts.NewSequenceAllocator(allocatorDB)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := facts.NewObservationOutbox(outboxDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFactsMutationWriter(NewManager(managerDB), allocator, outbox); err == nil {
		t.Fatal("split manager/allocator/outbox databases unexpectedly accepted")
	}
	allocator, err = facts.NewSequenceAllocator(managerDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFactsMutationWriter(NewManager(managerDB), allocator, outbox); err == nil {
		t.Fatal("split manager/outbox databases unexpectedly accepted")
	}
}

func seedOfferedLease(t *testing.T, db *sql.DB, manager *Manager) *Lease {
	t.Helper()
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")
	created, err := manager.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:10", "host-10")
	if err != nil {
		t.Fatal(err)
	}
	return created
}

type dnsSinkProbe struct {
	calls int
	fail  bool
}

func (p *dnsSinkProbe) EnqueueTx(tx *sql.Tx, l *Lease, action DNSMutationAction) error {
	if p.fail {
		p.calls++
		return errors.New("dns sink unavailable")
	}
	var wireAction string
	switch action {
	case DNSActionUpsert:
		wireAction = "create"
	case DNSActionDelete:
		wireAction = "delete"
	default:
		return errors.New("unsupported dns action")
	}
	_, err := tx.Exec(`INSERT INTO dhcp_dns_events
		(lease_id, generation, action, scope_id, ip_address, mac_address, hostname)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, l.ID, l.Generation, wireAction,
		l.ScopeID, l.IPAddress, l.MACAddress, l.Hostname)
	if err == nil {
		p.calls++
	}
	return err
}

func TestFactsMutationWriterPostCommitWakeRunsOnlyAfterCommit(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	offered := seedOfferedLease(t, db, manager)
	wakeCalls := 0
	writer.WithPostCommitWake(func() { wakeCalls++ })
	if _, err := writer.ActivateLease(context.Background(), "event-wake", "dhcp-node-a", "space-1", offered.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if wakeCalls != 1 {
		t.Fatalf("wake calls after commit = %d, want 1", wakeCalls)
	}
	writer.WithDNSSink(&dnsSinkProbe{fail: true})
	seedScope(t, db, "scope-2", "lan-2", "192.0.2.0/24", "192.0.2.30", "192.0.2.39")
	offered, err := manager.ReserveAddress("scope-2", "192.0.2.30", "aa:bb:cc:dd:ee:30", "host-30")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ActivateLease(context.Background(), "event-wake-fail", "dhcp-node-a", "space-1", offered.ID, time.Hour); err == nil {
		t.Fatal("failed mutation unexpectedly succeeded")
	}
	if wakeCalls != 1 {
		t.Fatalf("wake calls after rollback = %d, want 1", wakeCalls)
	}
}

func TestFactsMutationWriterDerivesEventIDWhenOmitted(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	offered := seedOfferedLease(t, db, manager)
	if _, err := writer.ActivateLease(context.Background(), "", "dhcp-node-a", "space-1", offered.ID, time.Hour); err != nil {
		t.Fatalf("ActivateLease: %v", err)
	}
	var eventID string
	if err := db.QueryRow(`SELECT event_id FROM dhcp_ipam_observation_events`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	identity, err := NewMutationFactIdentity("dhcp-node-a", "space-1", MutationActivate, offered.ID, offered.Generation+1)
	if err != nil {
		t.Fatal(err)
	}
	if eventID != identity.EventID {
		t.Fatalf("event ID = %q, want %q", eventID, identity.EventID)
	}
}

func TestFactsMutationWriterActivateRejectsGenerationOverflow(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	offered := seedOfferedLease(t, db, manager)
	if _, err := db.Exec(`UPDATE dhcp_leases SET generation=9223372036854775807 WHERE id=?`, offered.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ActivateLease(context.Background(), "event-activate-overflow", "dhcp-node-a", "space-1", offered.ID, time.Hour); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("ActivateLease() error = %v, want overflow rejection", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id=? AND status=?`, offered.ID, string(LeaseStatusOffered)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("offered lease changed after rejected activation: rows=%d", count)
	}
}

func TestFactsMutationWriterActivateCommitsLeaseAndFactTogether(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	offered := seedOfferedLease(t, db, manager)

	active, err := writer.ActivateLease(context.Background(), "event-activate-1", "dhcp-node-a", "space-1", offered.ID, time.Hour)
	if err != nil {
		t.Fatalf("ActivateLease: %v", err)
	}
	if active.Status != LeaseStatusActive || active.Generation != offered.Generation+1 {
		t.Fatalf("active lease = %+v", active)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM dhcp_leases WHERE id=?`, offered.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(LeaseStatusActive) {
		t.Fatalf("stored status = %s", status)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events WHERE event_id=? AND sequence=1`, "event-activate-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("fact count = %d, want 1", count)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload FROM dhcp_ipam_observation_events WHERE event_id=?`, "event-activate-1").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload == "" || !containsAll(payload, "space-1", offered.ID, "192.0.2.10") {
		t.Fatalf("fact payload = %s", payload)
	}
}

func TestFactsMutationWriterWithLegacyDNSSinkUsesSameCommit(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	offered := seedOfferedLease(t, db, manager)
	dns := &dnsSinkProbe{}
	writer.WithDNSSink(dns)
	if _, err := writer.ActivateLease(context.Background(), "event-dns-1", "dhcp-node-a", "space-1", offered.ID, time.Hour); err != nil {
		t.Fatalf("ActivateLease: %v", err)
	}
	if dns.calls != 1 {
		t.Fatalf("dns sink calls = %d, want 1", dns.calls)
	}

	writer, db, manager = newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	dns = &dnsSinkProbe{}
	writer.WithDNSSink(dns)
	if _, err := writer.ReleaseLease(context.Background(), "event-dns-2", "dhcp-node-a", "space-1", active.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if dns.calls != 1 {
		t.Fatalf("dns sink calls = %d, want 1", dns.calls)
	}
}

func TestFactsMutationWriterRenewCommitsGenerationAndFact(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	beforeGeneration := active.Generation
	renewed, err := writer.RenewLease(context.Background(), "event-renew-1", "dhcp-node-a", "space-1", active.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if renewed.Status != LeaseStatusActive || renewed.Generation != beforeGeneration+1 {
		t.Fatalf("renewed lease = %+v", renewed)
	}
	var action string
	if err := db.QueryRow(`SELECT action FROM dhcp_ipam_observation_events WHERE event_id=?`, "event-renew-1").Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != string(MutationRenew) {
		t.Fatalf("fact action = %s", action)
	}
}

func TestFactsMutationWriterRenewRejectsGenerationOverflow(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")
	lease, err := manager.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:10", "host-10", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE dhcp_leases SET generation=? WHERE id=?`, maxLeaseGeneration, lease.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.RenewLease(context.Background(), "event-overflow", "dhcp-node-a", "space-1", lease.ID, time.Hour); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("RenewLease() error = %v, want overflow rejection", err)
	}
	var generation int64
	if err := db.QueryRow(`SELECT generation FROM dhcp_leases WHERE id=?`, lease.ID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation != maxLeaseGeneration {
		t.Fatalf("generation = %d, want unchanged max generation", generation)
	}
}

func TestFactsMutationWriterRenewRejectsReleasedLease(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	if err := manager.ReleaseLease(active.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.RenewLease(context.Background(), "event-renew-invalid", "dhcp-node-a", "space-1", active.ID, time.Hour); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("renew released lease error = %v", err)
	}
}

func TestFactsMutationWriterDeclineCommitsConflictFact(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	conflicted, err := writer.DeclineLease(context.Background(), "event-decline-1", "dhcp-node-a", "space-1", active.ID, time.Hour)
	if err != nil {
		t.Fatalf("DeclineLease: %v", err)
	}
	if conflicted.Status != LeaseStatusConflict || conflicted.Generation != active.Generation {
		t.Fatalf("conflicted lease = %+v", conflicted)
	}
	var action string
	if err := db.QueryRow(`SELECT action FROM dhcp_ipam_observation_events WHERE event_id=?`, "event-decline-1").Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != string(MutationDecline) {
		t.Fatalf("fact action = %s", action)
	}
}

func TestFactsMutationWriterDeclineRejectsInvalidQuarantine(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	if _, err := writer.DeclineLease(context.Background(), "event-decline-invalid", "dhcp-node-a", "space-1", active.ID, 0); err == nil {
		t.Fatal("zero quarantine unexpectedly accepted")
	}
	stored, err := manager.GetLease(active.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != LeaseStatusActive {
		t.Fatalf("lease after invalid decline = %+v", stored)
	}
}

func TestFactsMutationWriterExpireCommitsDeleteFact(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	expired, err := writer.ExpireLease(context.Background(), "event-expire-1", "dhcp-node-a", "space-1", active.ID)
	if err != nil {
		t.Fatalf("ExpireLease: %v", err)
	}
	if expired.Status != LeaseStatusExpired || expired.Generation != active.Generation {
		t.Fatalf("expired lease = %+v", expired)
	}
	var action string
	if err := db.QueryRow(`SELECT action FROM dhcp_ipam_observation_events WHERE event_id=?`, "event-expire-1").Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != string(MutationExpire) {
		t.Fatalf("fact action = %s", action)
	}
}

func TestFactsMutationWriterExpireRejectsReleasedLease(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	if err := manager.ReleaseLease(active.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExpireLease(context.Background(), "event-expire-invalid", "dhcp-node-a", "space-1", active.ID); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("expire released lease error = %v", err)
	}
}

func TestFactsMutationWriterReleaseCommitsFactWithBeforeGeneration(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	beforeGeneration := active.Generation

	released, err := writer.ReleaseLease(context.Background(), "event-release-1", "dhcp-node-a", "space-1", active.ID)
	if err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if released.Status != LeaseStatusReleased || released.Generation != beforeGeneration {
		t.Fatalf("released lease = %+v", released)
	}
	var action string
	if err := db.QueryRow(`SELECT action FROM dhcp_ipam_observation_events WHERE event_id=?`, "event-release-1").Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != string(MutationRelease) {
		t.Fatalf("fact action = %s", action)
	}
}

func TestFactsMutationWriterRollbackRestoresLeaseAndSequence(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	offered := seedOfferedLease(t, db, manager)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = writer.ActivateLeaseTx(context.Background(), tx, "event-rollback-1", "dhcp-node-a", "space-1", offered.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	stored, err := manager.GetLease(offered.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != LeaseStatusOffered || stored.Generation != offered.Generation {
		t.Fatalf("lease after rollback = %+v", stored)
	}
	allocator, err := facts.NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocator.Current(context.Background()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("sequence after rollback = %v, want sql.ErrNoRows", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events WHERE event_id=?`, "event-rollback-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback fact count = %d", count)
	}
}

func TestFactsMutationWriterRejectsInvalidStateBeforeSideEffects(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	if _, err := writer.ActivateLease(context.Background(), "event-invalid-1", "dhcp-node-a", "space-1", active.ID, time.Hour); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("invalid activate error = %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid transition wrote %d facts", count)
	}
}

func TestFactsMutationWriterDNSFailureRollsBackAllWrites(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	offered := seedOfferedLease(t, db, manager)
	writer.WithDNSSink(&dnsSinkProbe{fail: true})
	if _, err := writer.ActivateLease(context.Background(), "event-dns-fail", "dhcp-node-a", "space-1", offered.ID, time.Hour); err == nil {
		t.Fatal("DNS failure unexpectedly committed mutation")
	}
	stored, err := manager.GetLease(offered.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != LeaseStatusOffered || stored.Generation != offered.Generation {
		t.Fatalf("lease after DNS failure = %+v", stored)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("facts after DNS failure = %d", count)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_dns_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("DNS outbox after DNS failure = %d", count)
	}
	allocator, err := facts.NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocator.Current(context.Background()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("sequence after DNS failure = %v, want sql.ErrNoRows", err)
	}
}

func TestFactsMutationWriterTransactionSeamAllowsCallerRollback(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	active := seedActiveLease(t, db, manager)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = writer.ReleaseLeaseTx(context.Background(), tx, "event-release-rollback", "dhcp-node-a", "space-1", active.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	stored, err := manager.GetLease(active.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != LeaseStatusActive {
		t.Fatalf("lease after caller rollback = %+v", stored)
	}
}

func seedActiveLease(t *testing.T, db *sql.DB, manager *Manager) *Lease {
	t.Helper()
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")
	active, err := manager.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:11", "host-11", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return active
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
