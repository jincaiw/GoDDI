package eventlogguard

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
)

func connQueryOnly(t *testing.T, db *sql.DB) *sql.Conn {
	t.Helper()
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestVerifyGuardedOnlyReadOnly(t *testing.T) {
	db := legacy(t)
	if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatal(err)
	}
	c := connQueryOnly(t, db)
	var before, after int64
	if err := c.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := c.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&after); err != nil || before != after {
		t.Fatalf("verify changed db %d %d %v", before, after, err)
	}
	// The version-aware replicaschema.Verify now accepts both frozen
	// references, so it is no longer imported here (that would create a test
	// import cycle). Its legacy/hardened coverage lives in its own package.
	var value int
	if err := c.QueryRowContext(context.Background(), `SELECT count(*) FROM main.eventlog_reference_guard_migrations`).Scan(&value); err != nil || value != 1 {
		t.Fatal("connection unusable")
	}
}
func TestVerifyRejectsLegacyAndCancellation(t *testing.T) {
	db := legacy(t)
	c := connQueryOnly(t, db)
	if err := Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
		t.Fatalf("legacy accepted %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Verify(ctx, c)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrInvalid) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if e := c.Close(); e != nil && !errors.Is(e, sql.ErrConnDone) {
		t.Fatalf("close canceled connection: %v", e)
	}
	fresh := connQueryOnly(t, db)
	var n int
	if err = fresh.QueryRowContext(context.Background(), `SELECT count(*) FROM main.sqlite_schema`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("fresh connection failed %v", err)
	}
	if !errors.Is(Verify(nil, fresh), ErrInvalid) || !errors.Is(Verify(context.Background(), nil), ErrInvalid) {
		t.Fatal("nil accepted")
	}
}
func TestVerifyRejectsSchemaAndMarkerDrift(t *testing.T) {
	for name, q := range map[string]string{"extra table": `CREATE TABLE unknown(x)`, "extra trigger": `CREATE TRIGGER unknown BEFORE INSERT ON replica_head BEGIN SELECT 1; END`, "missing guard": `DROP TRIGGER replica_head_rowid_update_guard`, "wrong guard": `DROP TRIGGER replica_head_rowid_update_guard; CREATE TRIGGER replica_head_rowid_update_guard BEFORE UPDATE ON replica_head BEGIN SELECT 1; END`, "guard digest": `UPDATE eventlog_reference_guard_migrations SET guard_digest='wrong'`, "guard omitted": `DELETE FROM eventlog_reference_guard_migrations`, "guard extra": `INSERT INTO eventlog_reference_guard_migrations VALUES(2,'x','y')`, "ipam digest": `UPDATE enterprise_schema_migrations SET digest='wrong' WHERE version=2`} {
		t.Run(name, func(t *testing.T) {
			db := legacy(t)
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
			c := connQueryOnly(t, db)
			if err := Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
				t.Fatalf("drift accepted %v", err)
			}
		})
	}
}
func TestVerifyEnvironmentAndTransaction(t *testing.T) {
	for _, kind := range []string{"query_only", "foreign_keys", "sync", "busy", "journal", "defer", "temp", "attach", "transaction"} {
		t.Run(kind, func(t *testing.T) {
			db := legacy(t)
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatal(err)
			}
			c, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			q := map[string]string{"foreign_keys": "PRAGMA foreign_keys=OFF", "sync": "PRAGMA synchronous=NORMAL", "busy": "PRAGMA busy_timeout=1", "journal": "PRAGMA journal_mode=DELETE", "defer": "PRAGMA defer_foreign_keys=ON", "temp": "CREATE TEMP TABLE replica_head(x)", "attach": "ATTACH DATABASE ':memory:' AS foreign_db"}[kind]
			if q != "" {
				if _, err = c.ExecContext(context.Background(), q); err != nil {
					t.Fatal(err)
				}
			}
			if kind != "query_only" {
				if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "transaction" {
				if _, err = c.ExecContext(context.Background(), `BEGIN`); err != nil {
					t.Fatal(err)
				}
				defer c.ExecContext(context.Background(), `ROLLBACK`)
			}
			if err = Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
				t.Fatalf("environment accepted %s: %v", kind, err)
			}
		})
	}
}
func TestBoundedSchemaObjectsRejectHugeDDL(t *testing.T) {
	db := legacy(t)
	if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE VIEW oversized AS SELECT '` + strings.Repeat("x", maxSchemaFieldBytes+1) + `'`); err != nil {
		t.Fatal(err)
	}
	c := connQueryOnly(t, db)
	if err := Verify(context.Background(), c); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize accepted: %v", err)
	}
}
func TestVerifyConcurrentReadOnlyConnections(t *testing.T) {
	db := legacy(t)
	db.SetMaxOpenConns(3)
	if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() { defer wg.Done(); defer c.Close(); errs <- Verify(context.Background(), c) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
