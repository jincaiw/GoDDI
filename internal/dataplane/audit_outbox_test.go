package dataplane

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The audit trail this plane owes upward.
//
// These entries describe writes no one watched: an RFC 2136 update accepted
// from a client, and a name DHCP published for a binding it confirmed. The
// archive is in the control database; the only copy that exists at the moment
// those writes happen is the one here.
// ---------------------------------------------------------------------------

func insertLocalAudit(t *testing.T, s *Store, id, detail, oldValue, newValue string) {
	t.Helper()
	if _, err := s.Exec(`
		INSERT INTO audit_logs (id, username, action, resource_type, resource_id, detail, old_value, new_value)
		VALUES (?, 'ddns-key.', 'dns_dynamic_update', 'zone', 'zone-1', ?, ?, ?)`,
		id, detail, nullable(oldValue), nullable(newValue)); err != nil {
		t.Fatalf("insert local audit entry %s: %v", id, err)
	}
}

// nullable turns "" into NULL, which is how the writers spell "there was
// nothing on that side". An empty string would read the same in a report and
// mean something else.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestPushAuditLogsCopiesTheTrailUpAndClearsTheMarkers(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	before := ""
	after := `[{"name":"web.example.test.","type":"A","value":"192.0.2.30"}]`
	insertLocalAudit(t, store, "a1", "Dynamic update on zone example.test: 1 changes applied", before, after)

	if n, err := rep.PendingAuditLogs(); err != nil || n != 1 {
		t.Fatalf("pending = (%d, %v), want (1, nil)", n, err)
	}

	n, err := rep.PushAuditLogs(ctx, 100)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if n != 1 {
		t.Errorf("pushed = %d, want 1", n)
	}
	if n, err := rep.PendingAuditLogs(); err != nil || n != 0 {
		t.Errorf("pending after the push = (%d, %v), want (0, nil)", n, err)
	}

	// The control table refuses UPDATE outright, so an INSERT that lands at all
	// is the only way this entry can exist there.
	var username string
	var oldValue sql.NullString
	var newValue string
	if err := control.QueryRow(
		`SELECT username, old_value, new_value FROM audit_logs WHERE id = 'a1'`).
		Scan(&username, &oldValue, &newValue); err != nil {
		t.Fatalf("read the archived entry: %v", err)
	}
	if username != "ddns-key." {
		t.Errorf("username = %q, want the TSIG key the client signed with", username)
	}
	if oldValue.Valid {
		t.Errorf("old_value = %q, want NULL: the entry describes a create", oldValue.String)
	}
	if !strings.Contains(newValue, "192.0.2.30") {
		t.Errorf("new_value = %q, want the record that was added", newValue)
	}

	// An empty queue is a no-op, not a transaction against the control database.
	if n, err := rep.PushAuditLogs(ctx, 100); err != nil || n != 0 {
		t.Errorf("second push = (%d, %v), want (0, nil)", n, err)
	}
}

// TestPushAuditLogsNeverRewritesAnEntryTheControlPlaneAlreadyHas pins the
// insert-only write. An audit entry is a statement about something that already
// happened, so a re-push of an id the archive already holds has nothing to
// reconcile -- and overwriting it would be the one edit the archive's own
// triggers exist to forbid.
func TestPushAuditLogsNeverRewritesAnEntryTheControlPlaneAlreadyHas(t *testing.T) {
	control, store, rep := newPair(t)

	if _, err := control.Exec(`
		INSERT INTO audit_logs (id, username, action, resource_type, detail)
		VALUES ('a1', 'ddns-key.', 'dns_dynamic_update', 'zone', 'as first delivered')`); err != nil {
		t.Fatalf("seed the archived entry: %v", err)
	}
	// The local copy of the same id says something else: this store's row was
	// written before the previous push was acknowledged.
	insertLocalAudit(t, store, "a1", "rewritten before the push", "", "")

	if _, err := rep.PushAuditLogs(context.Background(), 100); err != nil {
		t.Fatalf("push: %v", err)
	}

	var detail string
	if err := control.QueryRow(`SELECT detail FROM audit_logs WHERE id = 'a1'`).Scan(&detail); err != nil {
		t.Fatalf("read the archived entry: %v", err)
	}
	if detail != "as first delivered" {
		t.Errorf("the push overwrote what the archive already held: %q", detail)
	}
	// The marker clears regardless: the archive holds a statement about that
	// id, and re-sending it would only spin.
	if n, err := rep.PendingAuditLogs(); err != nil || n != 0 {
		t.Errorf("pending after the push = (%d, %v), want (0, nil)", n, err)
	}
}

// TestTheAuditMirrorIsNotAColumnBehind is the guard for the failure this
// migration was written to fix. The mirror is hand-written, the archive is not,
// and a mirror that is one column short does not fail loudly on its own: the
// push statement names a column that does not exist, the error surfaces as
// "pushing is broken", and the field it was added for never travels. Comparing
// the two column sets turns that into a build-time failure.
func TestTheAuditMirrorIsNotAColumnBehind(t *testing.T) {
	control, store, _ := newPair(t)

	archived := columnNames(t, control, "audit_logs")
	mirrored := columnNames(t, store.DB, "audit_logs")

	for col := range archived {
		if !mirrored[col] {
			t.Errorf("the control archive has audit_logs.%s and this plane's mirror does not", col)
		}
	}
	for _, col := range auditPushColumns {
		if !mirrored[col] {
			t.Errorf("the push names audit_logs.%s, which the mirror does not have", col)
		}
		if !archived[col] {
			t.Errorf("the push names audit_logs.%s, which the archive does not have", col)
		}
	}
}

// TestTheLeasePlanePushesItsOwnAuditEntries guards the wiring rather than the
// writer. The two planes write different audit entries into different stores --
// lease transitions here, published records there -- so a push that runs on only
// one of them leaves the other's entries in a queue nobody drains. That failure
// is invisible from either side: the DHCP plane keeps working, and the archive
// simply never learns that an address changed hands.
func TestTheLeasePlanePushesItsOwnAuditEntries(t *testing.T) {
	control, store, rep := newPair(t)
	ctx := context.Background()

	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	if err := runner.Prime(ctx); err != nil {
		t.Fatalf("prime: %v", err)
	}

	if _, err := store.Exec(`
		INSERT INTO audit_logs (id, username, action, resource_type, resource_id, detail)
		VALUES ('a1', 'dhcp', 'dhcp_lease_release', 'lease', 'l1',
		        'DHCP lease active -> released for 10.0.0.10')`); err != nil {
		t.Fatalf("write the local entry: %v", err)
	}

	runner.Once(ctx)

	var action string
	if err := control.QueryRow(`SELECT action FROM audit_logs WHERE id = 'a1'`).Scan(&action); err != nil {
		t.Fatalf("the lease plane never pushed its audit entry: %v", err)
	}
	if action != "dhcp_lease_release" {
		t.Errorf("action = %q, want dhcp_lease_release", action)
	}
	if n, err := rep.PendingAuditLogs(); err != nil || n != 0 {
		t.Errorf("pending after the poll = (%d, %v), want (0, nil)", n, err)
	}
}

func columnNames(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	// PRAGMA over the single-connection pool: the cursor is drained and closed
	// before the caller issues anything else.
	rows, err := db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatalf("read the columns of %s: %v", table, err)
	}
	defer rows.Close()

	names := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan a column of %s: %v", table, err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate the columns of %s: %v", table, err)
	}
	return names
}
