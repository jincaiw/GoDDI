package configver

import (
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

// TestMigration019UpDownUp verifies that migration 019 is reversible and that
// the constraints it adds are really enforced.
//
// The down direction names its target version explicitly. "goose.Down" means
// "roll back one migration", so it silently stopped testing 019 the moment a
// later migration was added -- the test would have gone on passing while
// exercising somebody else's rollback.
func TestMigration019UpDownUp(t *testing.T) {
	const target = 18
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("up: %v", err)
	}

	// Tables and columns must exist with the shipped shapes.
	for _, probe := range []string{
		`SELECT id, resource_type, resource_id, revision, status, content, content_hash, base_revision, idempotency_key, actor, note, error, applied_generation, created_at, updated_at, applied_at FROM config_revisions LIMIT 1`,
		`SELECT id, revision_id, resource_type, resource_id, revision, status, attempts, resource_applied, applied_generation, last_error, next_attempt_at, created_at, applied_at FROM config_release_outbox LIMIT 1`,
	} {
		rows, err := db.Query(probe)
		if err != nil {
			t.Fatalf("probe failed: %v", err)
		}
		rows.Close()
	}

	// The unique indexes must actually be enforced.
	if _, err := db.Exec(`INSERT INTO config_revisions
		(id, resource_type, resource_id, revision, status, content, content_hash)
		VALUES ('a','dhcp_scope','s1',1,'staged','{}','h')`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO config_revisions
		(id, resource_type, resource_id, revision, status, content, content_hash)
		VALUES ('b','dhcp_scope','s1',1,'staged','{}','h')`); err == nil {
		t.Fatalf("duplicate revision number was accepted")
	}
	if _, err := db.Exec(`INSERT INTO config_revisions
		(id, resource_type, resource_id, revision, status, content, content_hash, idempotency_key)
		VALUES ('c','dhcp_scope','s2',1,'staged','{}','h','dup')`); err != nil {
		t.Fatalf("insert with key: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO config_revisions
		(id, resource_type, resource_id, revision, status, content, content_hash, idempotency_key)
		VALUES ('d','dhcp_scope','s3',1,'staged','{}','h','dup')`); err == nil {
		t.Fatalf("duplicate idempotency key was accepted")
	}
	// A NULL key must not collide with another NULL key.
	if _, err := db.Exec(`INSERT INTO config_revisions
		(id, resource_type, resource_id, revision, status, content, content_hash)
		VALUES ('e','dhcp_scope','s4',1,'staged','{}','h')`); err != nil {
		t.Fatalf("insert without key: %v", err)
	}

	if err := goose.DownTo(db, ".", target); err != nil {
		t.Fatalf("down to %d: %v", target, err)
	}
	var table string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='config_revisions'`).Scan(&table)
	if err != sql.ErrNoRows {
		t.Fatalf("config_revisions still present after down: %v", err)
	}

	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	if err := goose.DownTo(db, ".", target); err != nil {
		t.Fatalf("down to %d after re-up: %v", target, err)
	}
}
