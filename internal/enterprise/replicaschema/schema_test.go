package replicaschema

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
)

func fixture(t *testing.T) (*sql.DB, *sql.Conn) {
	t.Helper()
	ctx := context.Background()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "test.db"))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := ipamstore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(ctx, db); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return db, conn
}
func exec(t *testing.T, c *sql.Conn, q string) {
	t.Helper()
	if _, err := c.ExecContext(context.Background(), q); err != nil {
		t.Fatal(err)
	}
}
func verify(t *testing.T, c *sql.Conn, want bool) {
	t.Helper()
	exec(t, c, "PRAGMA query_only=ON")
	err := Verify(context.Background(), c)
	if (err == nil) != want {
		t.Fatalf("Verify = %v, want success %v", err, want)
	}
}
func TestReferenceAcceptedReadOnly(t *testing.T) {
	_, c := fixture(t)
	verify(t, c, true)
	if _, err := c.ExecContext(context.Background(), "DELETE FROM enterprise_schema_migrations"); err == nil {
		t.Fatal("connection no longer read-only")
	}
}
func TestRejectDriftAndUnexpectedObjects(t *testing.T) {
	cases := map[string]string{
		"missing trigger":    `DROP TRIGGER replica_event_no_update`,
		"missing v2 guard":   `DROP TRIGGER ipam_occupancies_rowid_insert_guard`,
		"missing v2 marker":  `DELETE FROM enterprise_schema_migrations WHERE version=2`,
		"extra trigger":      `CREATE TRIGGER bad BEFORE INSERT ON ipam_occupancies BEGIN SELECT RAISE(IGNORE); END`,
		"extra index":        `CREATE INDEX bad_idx ON replica_event(seq)`,
		"forged marker":      `UPDATE enterprise_schema_migrations SET digest='forged'`,
		"oversized marker":   `UPDATE enterprise_schema_migrations SET digest=printf('%.*c',2097152,'x') WHERE version=1`,
		"negative marker":    `UPDATE enterprise_schema_migrations SET version=-1 WHERE version=1`,
		"blob marker schema": `DROP TABLE enterprise_schema_migrations; CREATE TABLE enterprise_schema_migrations(version,digest); INSERT INTO enterprise_schema_migrations VALUES(1,zeroblob(64))`,
		"extra version":      `INSERT INTO enterprise_schema_migrations VALUES(3,'x')`,
		"missing marker":     `DELETE FROM enterprise_schema_migrations`,
		"unknown table":      `CREATE TABLE unrelated(x INTEGER)`,
		"temp trigger":       `CREATE TEMP TRIGGER evil BEFORE INSERT ON main.ipam_occupancies BEGIN SELECT RAISE(IGNORE); END`,
		"temp shadow":        `CREATE TEMP TABLE replica_event(x INTEGER)`,
		"changed table":      `DROP TABLE replica_checkpoint; CREATE TABLE replica_checkpoint(x INTEGER)`,
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) { _, c := fixture(t); exec(t, c, q); verify(t, c, false) })
	}
}
func TestRejectPolicyMismatch(t *testing.T) {
	for name, q := range map[string]string{"foreign keys": "PRAGMA foreign_keys=OFF", "synchronous": "PRAGMA synchronous=NORMAL", "timeout": "PRAGMA busy_timeout=1"} {
		t.Run(name, func(t *testing.T) { _, c := fixture(t); exec(t, c, q); verify(t, c, false) })
	}
	_, c := fixture(t)
	if err := Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
		t.Fatalf("writable connection accepted: %v", err)
	}
}

func TestMigrationPathsAreIndependentCopies(t *testing.T) {
	paths := ipamstore.MigrationPaths()
	if len(paths) != 2 {
		t.Fatalf("paths: %v", paths)
	}
	paths[0] = "migrations/forged.sql"
	_ = append(paths, "extra.sql")
	if got := ipamstore.MigrationPaths(); len(got) != 2 || got[0] != "migrations/001_ipam.sql" || got[1] != "migrations/002_rowid_guards.sql" {
		t.Fatalf("paths changed: %v", got)
	}
	if err := ipamstore.VerifyDigests(); err != nil {
		t.Fatalf("verify digests: %v", err)
	}
	_, conn := fixture(t) // Migrate must still install the pinned original.
	verify(t, conn, true)
}
