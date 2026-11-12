package ipamstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

// legacyFixture explicitly constructs the reviewed 001 reference, never an
// upstream deployment. The test runner is the only consumer of this helper.
func legacyFixture(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	text, err := ContentsOf(initialMigrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(text); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE enterprise_schema_migrations (version INTEGER PRIMARY KEY, digest TEXT NOT NULL) STRICT`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO enterprise_schema_migrations VALUES(1,?)`, migrationDigests[initialMigrationPath]); err != nil {
		t.Fatal(err)
	}
	return db
}
func markers(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM enterprise_schema_migrations`).Scan(&count); err != nil || count != want {
		t.Fatalf("markers=%d want=%d err=%v", count, want, err)
	}
}
func TestV1UpgradePreservesRowsAndIsRepeatable(t *testing.T) {
	db := legacyFixture(t)
	if _, err := db.Exec(`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('t','slug','Tenant')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	markers(t, db, 2)
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ipam_tenants WHERE id='t'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("lost data %d %v", n, err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}
func TestV1UpgradeAndV2Reopen(t *testing.T) {
	db := legacyFixture(t)
	var index int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&index, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := sqlitepolicy.OpenWriter(path)
	if err := Migrate(context.Background(), reopened); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	again := sqlitepolicy.OpenWriter(path)
	defer again.Close()
	if err := Migrate(context.Background(), again); err != nil {
		t.Fatal(err)
	}
	markers(t, again, 2)
}

func TestV1UpgradeRejectsNonPositiveAndDrift(t *testing.T) {
	for _, kind := range []string{"negative", "zero", "foreign key", "extra attached trigger", "extra ipam view", "attached database", "temp trigger", "forged marker"} {
		t.Run(kind, func(t *testing.T) {
			db := legacyFixture(t)
			var q string
			switch kind {
			case "negative":
				q = `INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES(-1,'bad','bad','bad')`
			case "zero":
				q = `INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES(0,'bad','bad','bad')`
			case "extra attached trigger":
				q = `CREATE TRIGGER sneaky BEFORE INSERT ON ipam_tenants BEGIN SELECT RAISE(IGNORE); END`
			case "temp trigger":
				q = `CREATE TEMP TRIGGER sneaky BEFORE INSERT ON main.ipam_tenants BEGIN SELECT RAISE(IGNORE); END`
			case "extra ipam view":
				q = `CREATE VIEW ipam_fake AS SELECT id FROM ipam_tenants`
			case "attached database":
				q = `ATTACH DATABASE ':memory:' AS extra`
			case "forged marker":
				q = `UPDATE enterprise_schema_migrations SET digest='wrong'`
			case "foreign key":
				if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES('missing','s','s','Space')`); err != nil {
					t.Fatal(err)
				}
				q = `PRAGMA foreign_keys=ON`
			}
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(context.Background(), db); err == nil {
				t.Fatal("accepted invalid v1")
			}
			markers(t, db, 1)
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='trigger' AND name LIKE '%_rowid_insert_guard'`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("partial guards %d %v", count, err)
			}
		})
	}
}
func TestV1UpgradeFailureRollsBackDDLAndMarker(t *testing.T) {
	db := legacyFixture(t)
	injected := errors.New("injected after v2 ddl")
	err := migrate(context.Background(), db, func(ctx context.Context, c *sql.Conn, path string) error {
		if err := applyMigration(ctx, c, path); err != nil {
			return err
		}
		if path == rowidMigrationPath {
			return injected
		}
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("failure %v", err)
	}
	markers(t, db, 1)
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='trigger' AND name LIKE '%_rowid_insert_guard'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("guards escaped rollback %d %v", n, err)
	}
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	markers(t, db, 2)
}
func TestV2RejectsMissingOrUnreviewedGuard(t *testing.T) {
	for _, q := range []string{`DROP TRIGGER ipam_tenants_rowid_insert_guard`, `CREATE TRIGGER unreviewed BEFORE INSERT ON ipam_tenants BEGIN SELECT RAISE(IGNORE); END`, `DELETE FROM enterprise_schema_migrations WHERE version=2`, `UPDATE enterprise_schema_migrations SET digest='bad' WHERE version=2`, `INSERT INTO enterprise_schema_migrations VALUES(3,'bad')`} {
		t.Run(q, func(t *testing.T) {
			db := openTestDB(t)
			if err := Migrate(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(context.Background(), db); err == nil {
				t.Fatal("accepted damaged schema")
			}
		})
	}
}
func TestRowIDCollisionAliasesAndAutoAvailability(t *testing.T) {
	db := openTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('old','old','Old')`); err != nil {
		t.Fatal(err)
	}
	for _, recursive := range []string{"OFF", "ON"} {
		if _, err := db.Exec(`PRAGMA recursive_triggers=` + recursive); err != nil {
			t.Fatal(err)
		}
		for _, alias := range []string{"rowid", "_rowid_", "oid"} {
			q := `INSERT OR REPLACE INTO ipam_tenants(` + alias + `,id,slug,display_name) SELECT rowid,'replacement','replacement','Replacement' FROM ipam_tenants WHERE id='old'`
			if _, err := db.Exec(q); err == nil || !strings.Contains(err.Error(), "IPAM_ROWID_CONFLICT") {
				t.Fatalf("%s/%s %v", recursive, alias, err)
			}
			if _, err := db.Exec(`UPDATE OR REPLACE ipam_tenants SET ` + alias + `=2000,revision=revision+1 WHERE id='old'`); err == nil {
				t.Fatalf("update %s succeeded", alias)
			}
		}
	}
	for _, id := range []int64{-42, -1, 0} {
		if _, err := db.Exec(`INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES(?,'negative','negative','Negative')`, id); err == nil {
			t.Fatalf("rowid %d accepted", id)
		}
	}
	if _, err := db.Exec(`INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES(2000,'first','first','First'),(-1,'second','second','Second')`); err == nil {
		t.Fatal("bad batch accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM ipam_tenants WHERE id='first'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial batch %d %v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('auto1','auto1','Auto'),('auto2','auto2','Auto')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES(9223372036854775807,'max','max','Max')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('auto3','auto3','Auto')`); err != nil {
		t.Fatalf("automatic after max: %v", err)
	}
}
