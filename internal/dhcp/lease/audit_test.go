package lease

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/auditlog"
)

// leaseAuditRow is one audit entry, as read back.
type leaseAuditRow struct {
	Action     string
	Username   string
	ResourceID string
	Detail     string
	OldValue   sql.NullString
	NewValue   sql.NullString
}

// auditRows reads every lease entry, oldest first.
//
// Ordered by rowid rather than created_at: several transitions land in the same
// second, and an assertion that picks the wrong row reads exactly like a broken
// writer when it fails.
func auditRows(t *testing.T, db *sql.DB) []leaseAuditRow {
	t.Helper()

	rows, err := db.Query(`
		SELECT action, username, resource_id, detail, old_value, new_value
		FROM audit_logs
		WHERE resource_type = 'lease'
		ORDER BY rowid`)
	if err != nil {
		t.Fatalf("read audit rows: %v", err)
	}
	defer rows.Close()

	var out []leaseAuditRow
	for rows.Next() {
		var r leaseAuditRow
		if err := rows.Scan(&r.Action, &r.Username, &r.ResourceID, &r.Detail,
			&r.OldValue, &r.NewValue); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit rows: %v", err)
	}
	return out
}

func onlyAuditRow(t *testing.T, db *sql.DB) leaseAuditRow {
	t.Helper()
	rows := auditRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("got %d audit rows, want 1: %+v", len(rows), rows)
	}
	return rows[0]
}

// statusIn decodes the status field of one side of an entry.
func statusIn(t *testing.T, value sql.NullString) string {
	t.Helper()
	if !value.Valid {
		return ""
	}
	var decoded struct {
		Status  string `json:"status"`
		Address string `json:"ip_address"`
	}
	if err := json.Unmarshal([]byte(value.String), &decoded); err != nil {
		t.Fatalf("decode audit value %q: %v", value.String, err)
	}
	return decoded.Status
}

// TestAnOfferIsNotRecordedAndTheBindingThatFollowsIs is the offer/REQUEST
// distinction written down. A DISCOVER reserves an address for a client that
// may never come back; a REQUEST creates a binding. Only the second one is a
// fact about the network.
func TestAnOfferIsNotRecordedAndTheBindingThatFollowsIs(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.12")

	offer, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01", "host1")
	if err != nil {
		t.Fatalf("ReserveAddress: %v", err)
	}
	if offer.Status != LeaseStatusOffered {
		t.Fatalf("reserved lease status = %s, want offered", offer.Status)
	}
	if rows := auditRows(t, db); len(rows) != 0 {
		t.Fatalf("an offer was recorded as a binding: %+v", rows)
	}

	// A retransmitted DISCOVER refreshes the hold. Still not a binding.
	if _, err := m.ReserveAddress("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:01", "host1"); err != nil {
		t.Fatalf("ReserveAddress (retransmit): %v", err)
	}
	if rows := auditRows(t, db); len(rows) != 0 {
		t.Fatalf("refreshing an offer was recorded: %+v", rows)
	}

	bound, err := m.ActivateLease(offer.ID, time.Hour)
	if err != nil {
		t.Fatalf("ActivateLease: %v", err)
	}

	entry := onlyAuditRow(t, db)
	if entry.Action != auditlog.ActionLeaseBind {
		t.Errorf("action = %q, want %q", entry.Action, auditlog.ActionLeaseBind)
	}
	if entry.ResourceID != bound.ID {
		t.Errorf("resource = %q, want the lease %q", entry.ResourceID, bound.ID)
	}
	if got := statusIn(t, entry.OldValue); got != string(LeaseStatusOffered) {
		t.Errorf("old status = %q, want offered", got)
	}
	if got := statusIn(t, entry.NewValue); got != string(LeaseStatusActive) {
		t.Errorf("new status = %q, want active", got)
	}
	// The address has to be legible on both sides: "this binding was created"
	// answers nothing without it.
	for _, side := range []struct {
		name  string
		value sql.NullString
	}{{"old_value", entry.OldValue}, {"new_value", entry.NewValue}} {
		var decoded struct {
			IP string `json:"ip_address"`
		}
		if err := json.Unmarshal([]byte(side.value.String), &decoded); err != nil {
			t.Fatalf("decode %s: %v", side.name, err)
		}
		if decoded.IP != "192.0.2.10" {
			t.Errorf("%s names address %q, want 192.0.2.10", side.name, decoded.IP)
		}
	}
}

// TestARenewalIsNotRecorded is the other half of the same rule, and the one
// most likely to be "fixed" by someone reading the state column alone: a
// renewal takes a lease from active to active. It changes lease_end and the
// generation, which is the row itself; it does not change the binding, and an
// entry per renewal would be an entry per client per half-life.
func TestARenewalIsNotRecorded(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.12")

	created, err := m.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:02", "host2", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	before := onlyAuditRow(t, db)
	if before.Action != auditlog.ActionLeaseBind {
		t.Fatalf("action = %q, want a bind", before.Action)
	}

	if _, err := m.RenewLease(created.ID, time.Hour); err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if _, err := m.ActivateLease(created.ID, time.Hour); err != nil {
		t.Fatalf("ActivateLease (renewal): %v", err)
	}

	if rows := auditRows(t, db); len(rows) != 1 {
		t.Fatalf("a renewal was recorded: %+v", rows)
	}

	// The renewal did happen -- the assertion above is about the trail, not
	// about a write that silently failed.
	var generation int64
	if err := db.QueryRow("SELECT generation FROM dhcp_leases WHERE id = ?", created.ID).
		Scan(&generation); err != nil {
		t.Fatalf("read generation: %v", err)
	}
	if generation <= created.Generation {
		t.Errorf("generation = %d, want more than %d: the renewal did not happen",
			generation, created.Generation)
	}
}

func TestReleaseAndDeclineAreRecordedWithBothSides(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.12")

	released, err := m.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:03", "host3", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	if err := m.ReleaseLease(released.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}

	declined, err := m.CreateLease("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:04", "host4", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	if err := m.MarkLeaseConflict(declined.ID); err != nil {
		t.Fatalf("MarkLeaseConflict: %v", err)
	}

	rows := auditRows(t, db)
	if len(rows) != 4 {
		t.Fatalf("got %d audit rows, want 4 (two binds, a release, a decline): %+v", len(rows), rows)
	}

	want := []struct {
		action string
		lease  string
		before string
		after  string
	}{
		{auditlog.ActionLeaseBind, released.ID, "", string(LeaseStatusActive)},
		{auditlog.ActionLeaseRelease, released.ID, string(LeaseStatusActive), string(LeaseStatusReleased)},
		{auditlog.ActionLeaseBind, declined.ID, "", string(LeaseStatusActive)},
		{auditlog.ActionLeaseDecline, declined.ID, string(LeaseStatusActive), string(LeaseStatusConflict)},
	}
	for i, w := range want {
		got := rows[i]
		if got.Action != w.action || got.ResourceID != w.lease {
			t.Errorf("row %d = (%s, %s), want (%s, %s)", i, got.Action, got.ResourceID, w.action, w.lease)
			continue
		}
		if before := statusIn(t, got.OldValue); before != w.before {
			t.Errorf("row %d (%s) old status = %q, want %q", i, w.action, before, w.before)
		}
		if after := statusIn(t, got.NewValue); after != w.after {
			t.Errorf("row %d (%s) new status = %q, want %q", i, w.action, after, w.after)
		}
	}
}

// TestADeclineForAnUnknownAddressIsRecorded covers the tombstone path. A
// DECLINE can arrive for an address this server never recorded -- a statically
// configured host, or a conflict a peer saw first -- and the quarantine then
// rests on nothing but the client's word, which is exactly the case a reader
// needs the entry for.
func TestADeclineForAnUnknownAddressIsRecorded(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)
	seedScope(t, db, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.12")

	held, err := m.QuarantineIP("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:05")
	if err != nil {
		t.Fatalf("QuarantineIP: %v", err)
	}
	if held != nil {
		t.Fatalf("QuarantineIP returned a lease for an address nothing held: %+v", held)
	}

	entry := onlyAuditRow(t, db)
	if entry.Action != auditlog.ActionLeaseDecline {
		t.Errorf("action = %q, want a decline", entry.Action)
	}
	if entry.OldValue.Valid {
		t.Errorf("old_value = %q, want NULL: nothing held the address", entry.OldValue.String)
	}
	if got := statusIn(t, entry.NewValue); got != string(LeaseStatusConflict) {
		t.Errorf("new status = %q, want conflict", got)
	}

	// And when a lease does hold the address, the decline names what it took
	// the address away from.
	m2 := NewManager(db)
	bound, err := m2.CreateLease("scope-1", "192.0.2.12", "aa:bb:cc:dd:ee:06", "host6", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	if _, err := m2.QuarantineIP("scope-1", "192.0.2.12", "aa:bb:cc:dd:ee:06"); err != nil {
		t.Fatalf("QuarantineIP (held): %v", err)
	}

	rows := auditRows(t, db)
	last := rows[len(rows)-1]
	if last.Action != auditlog.ActionLeaseDecline || last.ResourceID != bound.ID {
		t.Fatalf("last row = (%s, %s), want a decline of %s", last.Action, last.ResourceID, bound.ID)
	}
	if got := statusIn(t, last.OldValue); got != string(LeaseStatusActive) {
		t.Errorf("old status = %q, want active", got)
	}
}

// TestTheExpirySweepRecordsWhatItActuallyMoved checks the sweep's entries
// against its own return value. The audit is what makes the two statements
// inseparable: "this address went back to the pool" has to be true of every row
// the sweep reports, because the caller withdraws the DNS names of exactly
// those rows on the strength of it.
func TestTheExpirySweepRecordsWhatItActuallyMoved(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "lan", "192.0.2.0/24", "192.0.2.10", "192.0.2.12")
	m := NewManager(store.DB)

	expiring, err := m.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:07", "leaving", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	staying, err := m.CreateLease("scope-1", "192.0.2.11", "aa:bb:cc:dd:ee:08", "staying", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}

	// Backdate one binding past its end. The client that stays gets a renewal.
	if _, err := store.Exec(`UPDATE dhcp_leases SET lease_end = ? WHERE id = ?`,
		time.Now().UTC().Add(-time.Minute).Format("2006-01-02T15:04:05Z"), expiring.ID); err != nil {
		t.Fatalf("backdate lease: %v", err)
	}
	if _, err := m.RenewLease(staying.ID, time.Hour); err != nil {
		t.Fatalf("RenewLease: %v", err)
	}

	before := len(auditRows(t, store.DB))
	swept, err := m.ExpireLeases()
	if err != nil {
		t.Fatalf("ExpireLeases: %v", err)
	}

	if len(swept) != 1 || swept[0].ID != expiring.ID {
		t.Fatalf("swept = %+v, want only the lease that ran out", swept)
	}

	rows := auditRows(t, store.DB)
	if len(rows) != before+1 {
		t.Fatalf("got %d audit rows, want %d", len(rows), before+1)
	}
	entry := rows[len(rows)-1]
	if entry.Action != auditlog.ActionLeaseExpire {
		t.Errorf("action = %q, want %q", entry.Action, auditlog.ActionLeaseExpire)
	}
	if entry.ResourceID != expiring.ID {
		t.Errorf("resource = %q, want %q", entry.ResourceID, expiring.ID)
	}
	if got := statusIn(t, entry.OldValue); got != string(LeaseStatusActive) {
		t.Errorf("old status = %q, want active", got)
	}
	if got := statusIn(t, entry.NewValue); got != string(LeaseStatusExpired) {
		t.Errorf("new status = %q, want expired", got)
	}

	// A second sweep finds nothing and therefore records nothing: an entry for
	// a sweep that moved no rows would be a claim about nothing happening.
	if swept, err := m.ExpireLeases(); err != nil || len(swept) != 0 {
		t.Fatalf("second sweep = (%v, %v), want nothing", swept, err)
	}
	if got := len(auditRows(t, store.DB)); got != len(rows) {
		t.Errorf("the empty sweep recorded %d entries", got-len(rows))
	}
}

// TestAuditSurvivesTheStoreRefusingTheWrite is the contract every writer in
// this package shares: the address has already been handed out, so a trail that
// cannot be written must not turn a working DHCP server into a failing one.
func TestAuditSurvivesTheStoreRefusingTheWrite(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)

	// A writer with nothing to write into: the lease exists, the archive does
	// not. This is the shape of a schema that is a migration behind.
	if _, err := db.Exec(`DROP TABLE audit_logs`); err != nil {
		t.Fatalf("drop the audit table: %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS trg_dp_audit_log_dirty_insert`); err != nil {
		t.Fatalf("drop the dirty marker trigger: %v", err)
	}

	created, err := m.CreateLease("scope-1", "192.0.2.10", "aa:bb:cc:dd:ee:09", "host9", time.Hour)
	if err != nil {
		t.Fatalf("CreateLease failed because auditing failed: %v", err)
	}
	if created.Status != LeaseStatusActive {
		t.Errorf("status = %s, want active", created.Status)
	}
	if err := m.ReleaseLease(created.ID); err != nil {
		t.Fatalf("ReleaseLease failed because auditing failed: %v", err)
	}
	if err := m.MarkLeaseConflict(created.ID); err != nil {
		t.Fatalf("MarkLeaseConflict failed because auditing failed: %v", err)
	}
}
