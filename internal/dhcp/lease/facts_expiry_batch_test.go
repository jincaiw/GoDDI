package lease

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestFactsMutationWriterExpireFactsBatchCommitsAllFactsTogether(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")
	first, err := manager.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:10", "host-10", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreateLease("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:11", "host-11", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE dhcp_leases SET lease_end=datetime('now', '-1 minute') WHERE id IN (?, ?)`, first.ID, second.ID); err != nil {
		t.Fatal(err)
	}

	got, err := writer.ExpireFactsBatch(context.Background(), "dhcp-node-a", func(l *Lease) (string, error) {
		if l.ScopeID != "scope-1" {
			t.Fatalf("scope = %q", l.ScopeID)
		}
		return "space-1", nil
	})
	if err != nil {
		t.Fatalf("ExpireFactsBatch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expired = %d, want 2", len(got))
	}
	var expired, facts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE status=?`, string(LeaseStatusExpired)).Scan(&expired); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events WHERE action=?`, string(MutationExpire)).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if expired != 2 || facts != 2 {
		t.Fatalf("expired=%d facts=%d, want 2/2", expired, facts)
	}
}

func TestFactsMutationWriterExpireFactsBatchRollsBackAllRowsAndFacts(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")
	first, err := manager.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:10", "host-10", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreateLease("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:11", "host-11", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE dhcp_leases SET lease_end=datetime('now', '-1 minute') WHERE id IN (?, ?)`, first.ID, second.ID); err != nil {
		t.Fatal(err)
	}

	_, err = writer.ExpireFactsBatch(context.Background(), "dhcp-node-a", func(l *Lease) (string, error) {
		if l.ID == second.ID {
			return "", sql.ErrNoRows
		}
		return "space-1", nil
	})
	if err == nil {
		t.Fatal("batch succeeded despite resolver failure")
	}
	var status string
	for _, id := range []string{first.ID, second.ID} {
		if err := db.QueryRow(`SELECT status FROM dhcp_leases WHERE id=?`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == string(LeaseStatusExpired) {
			t.Fatalf("lease %s remained expired after rollback", id)
		}
	}
	var facts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if facts != 0 {
		t.Fatalf("facts = %d, want 0 after rollback", facts)
	}
}

func TestFactsMutationWriterExpireFactsBatchRunsAuditAndWakeAfterCommit(t *testing.T) {
	writer, db, manager := newFactsMutationWriter(t)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.29")
	lease, err := manager.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:10", "host-10", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE dhcp_leases SET lease_end=datetime('now', '-1 minute') WHERE id=?`, lease.ID); err != nil {
		t.Fatal(err)
	}
	wakeCalls := 0
	writer.WithPostCommitWake(func() { wakeCalls++ })
	if _, err := writer.ExpireFactsBatch(context.Background(), "dhcp-node-a", func(*Lease) (string, error) { return "space-1", nil }); err != nil {
		t.Fatalf("ExpireFactsBatch: %v", err)
	}
	if wakeCalls != 1 {
		t.Fatalf("wake calls = %d, want 1", wakeCalls)
	}
	rows := auditRows(t, db)
	if len(rows) != 2 || rows[1].Action != "dhcp_lease_expire" || rows[1].ResourceID != lease.ID {
		t.Fatalf("audit rows = %+v", rows)
	}
}

func TestFactsMutationWriterExpireFactsBatchDoesNotWakeEmptyBatch(t *testing.T) {
	writer, _, _ := newFactsMutationWriter(t)
	wakeCalls := 0
	writer.WithPostCommitWake(func() { wakeCalls++ })
	got, err := writer.ExpireFactsBatch(context.Background(), "dhcp-node-a", func(*Lease) (string, error) { return "space-1", nil })
	if err != nil {
		t.Fatalf("ExpireFactsBatch: %v", err)
	}
	if got != nil || wakeCalls != 0 {
		t.Fatalf("empty batch result=%v wake calls=%d", got, wakeCalls)
	}
}

func TestFactsMutationWriterExpireFactsBatchRequiresResolverAndSource(t *testing.T) {
	writer, _, _ := newFactsMutationWriter(t)
	if _, err := writer.ExpireFactsBatch(context.Background(), "", func(*Lease) (string, error) { return "space", nil }); err == nil {
		t.Fatal("empty source accepted")
	}
	if _, err := writer.ExpireFactsBatch(context.Background(), "node", nil); err == nil {
		t.Fatal("nil resolver accepted")
	}
}
