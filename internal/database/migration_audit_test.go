package database

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

// newMigratedDB brings up a database carrying every shipped migration.
func newMigratedDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { db.Close() })

	goose.SetBaseFS(goddiassets.Migrations())
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("up: %v", err)
	}
	return db
}

// TestMigration024MakesTheAuditTrailUneditable checks that append-only is a
// property of the database rather than of the current code. Nothing in the
// codebase updates or deletes an audit row today; the point of the trigger is
// that this stays true through the next refactor, which is exactly the kind of
// promise a comment cannot keep.
func TestMigration024MakesTheAuditTrailUneditable(t *testing.T) {
	db := newMigratedDB(t)

	// The two columns the data-plane writers fill in. A create has no before
	// and a delete has no after, which is what NULL means here.
	if _, err := db.Exec(`INSERT INTO audit_logs
		(id, username, action, resource_type, detail, old_value, new_value)
		VALUES ('a1', 'ddns-key.', 'dns_dynamic_update', 'zone', 'applied',
		        NULL, '[{"name":"web.example.test.","type":"A","value":"192.0.2.30"}]')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, err := db.Exec(`UPDATE audit_logs SET detail = 'edited' WHERE id = 'a1'`); err == nil {
		t.Error("an audit entry was edited; the trail is only a record if it cannot be")
	} else if !strings.Contains(err.Error(), "append-only") {
		t.Errorf("the refusal does not say why: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM audit_logs WHERE id = 'a1'`); err == nil {
		t.Error("an audit entry was deleted outside a maintenance window")
	} else if !strings.Contains(err.Error(), "maintenance window") {
		t.Errorf("the refusal does not name the remedy: %v", err)
	}

	// A shipped database must not arrive with a window open -- a window that is
	// simply there is a table that is simply deletable.
	var open int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_maintenance`).Scan(&open); err != nil {
		t.Fatalf("count maintenance windows: %v", err)
	}
	if open != 0 {
		t.Errorf("the shipped schema opens %d maintenance window(s)", open)
	}
}

// TestTheMaintenanceWindowIsNarrowAndLeavesItsOwnRecord covers both halves of
// the design: pruning is possible without dropping the trigger by hand, and the
// act of pruning is itself recorded. "The only way to prune is to edit the
// schema" is how a table ends up never pruned at all.
func TestTheMaintenanceWindowIsNarrowAndLeavesItsOwnRecord(t *testing.T) {
	db := newMigratedDB(t)

	if _, err := db.Exec(`INSERT INTO audit_logs (id, action, resource_type, detail)
		VALUES ('old', 'zone_create', 'zone', 'predates retention')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// The table holds one window, keyed by a fixed id, so a second attempt
	// cannot quietly widen it.
	if _, err := db.Exec(`INSERT INTO audit_maintenance (id, reason) VALUES (2, 'second')`); err == nil {
		t.Error("a second maintenance window was accepted")
	}

	if _, err := db.Exec(`INSERT INTO audit_maintenance (id, reason, opened_at)
		VALUES (1, 'retention: older than 365 days', datetime('now'))`); err != nil {
		t.Fatalf("open the window: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM audit_logs WHERE id = 'old'`); err != nil {
		t.Fatalf("the window did not permit pruning: %v", err)
	}

	// The window outlives the rows it was opened for, and says why it was
	// opened -- which is the entire difference between a window and an
	// always-deletable table.
	var reason string
	if err := db.QueryRow(`SELECT reason FROM audit_maintenance WHERE id = 1`).Scan(&reason); err != nil {
		t.Fatalf("the window left no record of itself: %v", err)
	}
	if reason == "" {
		t.Error("the window was opened without a reason")
	}

	// UPDATE stays refused even while the window is open: pruning is a
	// deletion, not a licence to rewrite history.
	if _, err := db.Exec(`INSERT INTO audit_logs (id, action, resource_type) VALUES ('live', 'zone_create', 'zone')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`UPDATE audit_logs SET detail = 'edited' WHERE id = 'live'`); err == nil {
		t.Error("an update was accepted while a maintenance window was open")
	}

	if _, err := db.Exec(`DELETE FROM audit_maintenance WHERE id = 1`); err != nil {
		t.Fatalf("close the window: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM audit_logs WHERE id = 'live'`); err == nil {
		t.Error("deletion stayed permitted after the window closed")
	}
}

// TestMigration024IsReversible runs 024 down and back up. The down direction
// names its target explicitly: goose.Down means "roll back one migration", so a
// bare Down would silently stop testing 024 the moment a later migration is
// added and go on passing while rolling back somebody else's work.
func TestMigration024IsReversible(t *testing.T) {
	const target = 23
	db := newMigratedDB(t)

	if err := goose.DownTo(db, ".", target); err != nil {
		t.Fatalf("down to %d: %v", target, err)
	}

	// Both halves of 024 must be gone: the columns and the table that opens the
	// pruning window. A successful probe here means the rollback left a schema
	// the older binary does not expect.
	for _, probe := range []string{
		`SELECT id, reason, opened_at FROM audit_maintenance LIMIT 1`,
		`SELECT old_value, new_value FROM audit_logs LIMIT 1`,
	} {
		rows, err := db.Query(probe)
		if err == nil {
			rows.Close()
			t.Errorf("probe survived the rollback: %s", probe)
		}
	}

	// With the triggers gone the table is editable again, which is what the
	// rollback has to mean: an operator who rolls back to a binary that prunes
	// must not be locked out of pruning.
	if _, err := db.Exec(`INSERT INTO audit_logs (id, action, resource_type) VALUES ('a1', 'zone_create', 'zone')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`UPDATE audit_logs SET detail = 'edited' WHERE id = 'a1'`); err != nil {
		t.Errorf("update still refused after the rollback: %v", err)
	}

	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if _, err := db.Exec(`UPDATE audit_logs SET detail = 'edited' WHERE id = 'a1'`); err == nil {
		t.Error("the append-only trigger did not come back")
	}
}
