package audit

// The control plane's audit trail.
//
// Two properties are worth more than the rest of the surface put together, and
// neither is visible in a green run of the others:
//
//   - an audit write that cannot happen must be reported. A trail that fails
//     silently is worse than no trail, because the table then reads as "nothing
//     happened" rather than "nobody was watching".
//   - the filtered total must be the count of the filtered set. A total taken
//     from the whole table looks right on page one of an unfiltered request and
//     silently mis-reports every filter and every later page.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"

	"github.com/jasonwa/goddi/internal/auditlog"
)

func newAuditStore(t *testing.T) *AuditManager {
	t.Helper()

	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(t.TempDir(), "audit.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	return NewAuditManager(dbh.DB)
}

func TestTheTrailRecordsEveryColumnAndBothOutcomes(t *testing.T) {
	am := newAuditStore(t)

	for _, success := range []bool{true, false} {
		entry := LogEntry{
			UserID: "user-" + boolName(success), Username: "operator",
			Action: "create", ResourceType: "zone",
			ResourceID: "zone-1", Detail: "name=example.test",
			SourceIP: "192.0.2.10", UserAgent: "test-agent",
			Success: success,
		}
		if err := am.Log(entry); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	logs, total, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("total=%d len=%d, want 2 and 2", total, len(logs))
	}

	byAction := map[bool]AuditLog{}
	for _, l := range logs {
		byAction[l.Success] = l
	}
	// The failure has to be stored as a failure. A trail in which every entry
	// is a success is a trail nobody can act on.
	if byAction[false].Success {
		t.Error("the failed attempt was recorded as a success")
	}
	for success, l := range byAction {
		if l.Username != "operator" || l.ResourceType != "zone" || l.ResourceID != "zone-1" ||
			l.Detail != "name=example.test" || l.SourceIP != "192.0.2.10" || l.UserAgent != "test-agent" {
			t.Errorf("success=%v entry lost a column: %+v", success, l)
		}
		if l.CreatedAt.IsZero() {
			t.Errorf("success=%v entry has no timestamp", success)
		}
	}
}

func TestATrailThatCannotBeWrittenSaysSo(t *testing.T) {
	am := newAuditStore(t)

	// A trail that fails silently is worse than no trail: the table then reads
	// as "nothing happened" rather than "nobody was watching". So the failure
	// has to come back to the caller.
	if err := am.Log(LogEntry{Action: "create", ResourceType: "zone"}); err != nil {
		t.Fatalf("the first write failed: %v", err)
	}
	dropAuditLogs(t, am)

	if err := am.Log(LogEntry{Action: "create", ResourceType: "zone"}); err == nil {
		t.Error("writing to a table that does not exist reported success")
	}
}

func TestTheTotalIsTheFilteredCountAndTheFiltersCombine(t *testing.T) {
	am := newAuditStore(t)

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	seed := []LogEntry{
		{UserID: "u1", Action: "create", ResourceType: "zone", ResourceID: "z1"},
		{UserID: "u1", Action: "delete", ResourceType: "zone", ResourceID: "z2"},
		{UserID: "u2", Action: "create", ResourceType: "record", ResourceID: "r1"},
	}
	for i, e := range seed {
		if err := am.Log(e); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// The timestamps Log writes are "now", so a window filter has to be built
	// around them rather than around a literal date.
	before := time.Now().Add(-time.Minute)
	after := time.Now().Add(time.Minute)

	for _, tc := range []struct {
		name   string
		filter AuditFilter
		want   int
	}{
		{"no filter", AuditFilter{}, 3},
		{"one user", AuditFilter{UserID: "u1"}, 2},
		{"one resource type", AuditFilter{ResourceType: "zone"}, 2},
		{"one action", AuditFilter{Action: "create"}, 2},
		{"user and action", AuditFilter{UserID: "u1", Action: "create"}, 1},
		{"all three", AuditFilter{UserID: "u2", ResourceType: "record", Action: "create"}, 1},
		{"a window that contains everything", AuditFilter{StartTime: &before, EndTime: &after}, 3},
		{"a window that has already closed", AuditFilter{StartTime: &base, EndTime: timePtr(base.Add(time.Hour))}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs, total, err := am.QueryLogs(tc.filter)
			if err != nil {
				t.Fatalf("QueryLogs: %v", err)
			}
			if int(total) != tc.want {
				t.Errorf("total = %d, want %d", total, tc.want)
			}
			if len(logs) != tc.want {
				t.Errorf("returned %d rows, want %d", len(logs), tc.want)
			}
			// The two must agree: a total counted over the unfiltered table
			// looks right only on page one of an unfiltered request.
			if int(total) != len(logs) && len(logs) < tc.want {
				t.Errorf("total=%d but only %d rows came back", total, len(logs))
			}
		})
	}
}

func TestPaginationIsBoundedFromBothEnds(t *testing.T) {
	am := newAuditStore(t)
	for i := 0; i < 5; i++ {
		if err := am.Log(LogEntry{Action: "create", ResourceType: "zone", ResourceID: "z"}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	// A page size beyond the cap is clamped rather than honoured, so one
	// request cannot ask for the whole table.
	logs, _, err := am.QueryLogs(AuditFilter{PageSize: 5000})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(logs) != 5 {
		t.Fatalf("returned %d rows, want 5", len(logs))
	}

	// A page number beyond the cap is clamped too, so a large OFFSET -- which
	// SQLite scans linearly -- cannot be asked for.
	logs, total, err := am.QueryLogs(AuditFilter{Page: 4000, PageSize: 100})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(logs) != 0 {
		t.Errorf("page 4000 of 5 rows returned %d rows", len(logs))
	}

	// Page 0 and page 1 are the same page.
	zero, _, err := am.QueryLogs(AuditFilter{Page: 0})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	one, _, err := am.QueryLogs(AuditFilter{Page: 1})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(zero) != len(one) {
		t.Errorf("page 0 returned %d rows and page 1 returned %d", len(zero), len(one))
	}
}

func TestARowWithAnUnreadableTimestampIsStillReported(t *testing.T) {
	am := newAuditStore(t)
	if err := am.Log(LogEntry{Action: "create", ResourceType: "zone", ResourceID: "good"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	exec(t, am, `INSERT INTO audit_logs (id, action, resource_type, resource_id, success, created_at)
		VALUES ('bad-row', 'create', 'zone', 'bad', 1, 'not a timestamp')`)

	logs, _, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	// A row the reader cannot date must not disappear: an audit gap that looks
	// like "there was nothing" is the one outcome an audit trail must not have.
	if len(logs) != 2 {
		t.Fatalf("returned %d rows, want 2: an unreadable timestamp dropped its row", len(logs))
	}
	for _, l := range logs {
		if l.ResourceID == "bad" && !l.CreatedAt.IsZero() {
			t.Errorf("the row with the unreadable timestamp reported %v", l.CreatedAt)
		}
	}
}

// TestBothWritersProduceATimestampThisReaderUnderstands covers the seam between
// the two things that write an audit row. The control plane's Log writes an
// RFC3339 string; the data plane's auditlog.Append lets SQLite write
// datetime('now'). If the reader only understands one of them, every entry the
// data plane wrote shows up undated -- and an undated entry is one an operator
// cannot place in a sequence.
func TestBothWritersProduceATimestampThisReaderUnderstands(t *testing.T) {
	am := newAuditStore(t)

	if _, err := auditlog.Append(am.db, auditlog.Entry{
		Action: auditlog.ActionLeaseBind, ResourceType: auditlog.ResourceLease, ResourceID: "lease-1",
	}); err != nil {
		t.Fatalf("data-plane append: %v", err)
	}
	if err := am.Log(LogEntry{Action: "create", ResourceType: "zone", ResourceID: "z1"}); err != nil {
		t.Fatalf("control-plane log: %v", err)
	}

	logs, _, err := am.QueryLogs(AuditFilter{})
	if err != nil {
		t.Fatalf("QueryLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("returned %d rows, want 2", len(logs))
	}
	for _, l := range logs {
		if l.CreatedAt.IsZero() {
			t.Errorf("the %s entry written for %s has no readable timestamp", l.Action, l.ResourceID)
		}
	}
}

func TestALoginByAnUnknownUserIsStillRecorded(t *testing.T) {
	am := newAuditStore(t)
	// login_history.user_id carries a foreign key, so the known user has to
	// exist for the successful row to be written at all.
	exec(t, am, `INSERT INTO users (id, username, password_hash) VALUES ('user-1', 'operator', 'x')`)

	// A failed login names nobody, so its user_id is NULL. The entry has to
	// survive that: it is exactly the entry an operator is looking for.
	if err := am.LogLogin("", "attacker", "password", "203.0.113.9", "agent", false, "invalid credentials"); err != nil {
		t.Fatalf("LogLogin: %v", err)
	}
	if err := am.LogLogin("user-1", "operator", "password", "192.0.2.10", "agent", true, ""); err != nil {
		t.Fatalf("LogLogin: %v", err)
	}

	all, total, err := am.QueryLoginHistory("", 1, 20)
	if err != nil {
		t.Fatalf("QueryLoginHistory: %v", err)
	}
	if total != 2 || len(all) != 2 {
		t.Fatalf("total=%d len=%d, want 2 and 2", total, len(all))
	}
	for _, entry := range all {
		if entry["username"] == "attacker" {
			if _, present := entry["user_id"]; present {
				t.Error("the failed login reported a user_id; it has none")
			}
			if entry["success"] != false {
				t.Errorf("the failed login reported success=%v", entry["success"])
			}
		}
	}

	only := func(userID string) int {
		t.Helper()
		rows, _, err := am.QueryLoginHistory(userID, 1, 20)
		if err != nil {
			t.Fatalf("QueryLoginHistory(%q): %v", userID, err)
		}
		return len(rows)
	}
	if n := only("user-1"); n != 1 {
		t.Errorf("filtering by a known user returned %d rows, want 1", n)
	}
	if n := only("nobody"); n != 0 {
		t.Errorf("filtering by an unknown user returned %d rows, want 0", n)
	}
}

// ---- helpers ---------------------------------------------------------------

func boolName(b bool) string {
	if b {
		return "ok"
	}
	return "fail"
}

func timePtr(t time.Time) *time.Time { return &t }

func exec(t *testing.T, am *AuditManager, query string, args ...any) {
	t.Helper()
	if _, err := am.db.Exec(query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}

// dropAuditLogs removes the table to make every subsequent write fail. The
// migration installs a trigger that refuses UPDATE and DELETE on audit_logs
// unless a maintenance window is open, which is why the table has to go rather
// than its rows.
func dropAuditLogs(t *testing.T, am *AuditManager) {
	t.Helper()
	exec(t, am, `DROP TABLE audit_logs`)
}
