package ipamstore

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "ipam.db"))
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrationIsAtomicAndRepeatable(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("repeated migration: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='ipam_occupancies'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected occupancy table, count=%d err=%v", n, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE enterprise_schema_migrations SET digest = 'bad'`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); !errors.Is(err, ErrMigrationDrifted) {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestMigrationRejectsMarkerWithoutMatchingSchema(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if _, err := db.ExecContext(ctx, `CREATE TABLE enterprise_schema_migrations (version INTEGER PRIMARY KEY, digest TEXT NOT NULL) STRICT`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO enterprise_schema_migrations VALUES (1, ?)`, migrationDigests[MigrationPaths()[0]]); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); !errors.Is(err, ErrMigrationDrifted) {
		t.Fatalf("forged marker was accepted: %v", err)
	}
}

func TestMigrationRejectsMissingTrigger(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER ipam_occupancies_insert_guard`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); !errors.Is(err, ErrMigrationDrifted) {
		t.Fatalf("missing guard was accepted: %v", err)
	}
}

func TestMigrationFailureRollsBackAllDDL(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	injected := errors.New("injected failure after reference DDL")
	err := migrate(ctx, db, func(ctx context.Context, conn *sql.Conn, path string) error {
		if err := applyMigration(ctx, conn, path); err != nil {
			return err
		}
		return injected
	})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected failure, got %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("failed migration left %d tables", n)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

func TestMigrationRefusesExistingSchema(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if _, err := db.ExecContext(ctx, `CREATE TABLE ipam_spaces (id TEXT PRIMARY KEY); INSERT INTO ipam_spaces VALUES ('existing')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); !errors.Is(err, ErrExistingSchema) {
		t.Fatalf("expected existing schema refusal, got %v", err)
	}
	var id string
	if err := db.QueryRowContext(ctx, `SELECT id FROM ipam_spaces`).Scan(&id); err != nil || id != "existing" {
		t.Fatalf("existing data changed: %q, %v", id, err)
	}
}
