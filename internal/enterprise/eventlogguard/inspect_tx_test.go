package eventlogguard

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestInspectTxPreservesCallerTransaction(t *testing.T) {
	for _, want := range []Version{LegacyReference, HardenedReference} {
		t.Run(map[Version]string{LegacyReference: "legacy", HardenedReference: "guarded"}[want], func(t *testing.T) {
			db := legacy(t)
			if want == HardenedReference {
				if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
					t.Fatal(err)
				}
			}
			c := connQueryOnly(t, db)
			tx, err := c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			var before, after int
			if err = tx.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			got, err := InspectTx(context.Background(), tx)
			if err != nil || got != want {
				t.Fatalf("version=%v err=%v", got, err)
			}
			if err = tx.QueryRowContext(context.Background(), `SELECT total_changes()`).Scan(&after); err != nil || after != before {
				t.Fatalf("transaction closed or changed: %d %d %v", before, after, err)
			}
			var rows int
			if err = tx.QueryRowContext(context.Background(), `SELECT count(*) FROM main.ipam_tenants`).Scan(&rows); err != nil {
				t.Fatalf("caller cannot keep reading: %v", err)
			}
			if err = tx.Rollback(); err != nil {
				t.Fatalf("caller cannot end transaction: %v", err)
			}
		})
	}
}
func TestInspectTxInvalidInputsAndPolicy(t *testing.T) {
	db := legacy(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tx, err := c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	v, err := InspectTx(context.Background(), tx)
	if v != Unknown || !errors.Is(err, ErrInvalid) {
		t.Fatalf("query_only off: version=%v err=%v", v, err)
	}
	var n int
	if err = tx.QueryRowContext(context.Background(), `SELECT 1`).Scan(&n); err != nil || n != 1 {
		t.Fatal("inspector ended caller transaction")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	v, err = InspectTx(context.Background(), tx)
	if v != Unknown || !errors.Is(err, ErrInvalid) || !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("closed tx: %v %v", v, err)
	}
	if v, err = InspectTx(nil, tx); v != Unknown || !errors.Is(err, ErrInvalid) {
		t.Fatal("nil context accepted")
	}
	if v, err = InspectTx(context.Background(), nil); v != Unknown || !errors.Is(err, ErrInvalid) {
		t.Fatal("nil tx accepted")
	}
}
func TestInspectTxCanceledDoesNotOwnTransaction(t *testing.T) {
	db := legacy(t)
	c := connQueryOnly(t, db)
	tx, err := c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v, err := InspectTx(ctx, tx)
	if v != Unknown || !errors.Is(err, ErrInvalid) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel version=%v err=%v", v, err)
	}
	var n int
	if err = tx.QueryRowContext(context.Background(), `SELECT 1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("caller transaction unexpectedly ended %v", err)
	}
}
func TestInspectTxRejectsAdditionalSchemas(t *testing.T) {
	for _, kind := range []string{"temp", "attach", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			db := legacy(t)
			c, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			q := map[string]string{"temp": `CREATE TEMP TABLE ipam_tenants(x)`, "attach": `ATTACH DATABASE ':memory:' AS other`, "unknown": `CREATE TABLE unrelated(x)`}[kind]
			if _, err = c.ExecContext(context.Background(), q); err != nil {
				t.Fatal(err)
			}
			if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
				t.Fatal(err)
			}
			tx, err := c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			v, err := InspectTx(context.Background(), tx)
			if v != Unknown || !errors.Is(err, ErrInvalid) {
				t.Fatalf("%s version=%v err=%v", kind, v, err)
			}
			var n int
			if err = tx.QueryRowContext(context.Background(), `SELECT 1`).Scan(&n); err != nil || n != 1 {
				t.Fatalf("transaction unexpectedly ended %v", err)
			}
		})
	}
}

// This intentionally violates the clean-query-only-at-BEGIN precondition. It
// documents that a current PRAGMA cannot attest to earlier transaction writes.
func TestInspectTxCannotProveTransactionWasAlwaysReadOnly(t *testing.T) {
	db := legacy(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tx, err := c.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(context.Background(), `INSERT INTO main.ipam_tenants(id,slug,display_name,revision) VALUES('t','tenant','Tenant',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	v, err := InspectTx(context.Background(), tx)
	if err != nil || v != LegacyReference {
		t.Fatalf("version=%v err=%v", v, err)
	}
	var count int
	if err = tx.QueryRowContext(context.Background(), `SELECT count(*) FROM main.ipam_tenants`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("uncommitted state count=%d err=%v", count, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = c.QueryRowContext(context.Background(), `SELECT count(*) FROM main.ipam_tenants`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback count=%d err=%v", count, err)
	}
}

// This deliberately violates the documented no-concurrent-writer precondition
// only to demonstrate that SQLite WAL keeps an already pinned read snapshot.
func TestInspectTxReadsPinnedWALSnapshot(t *testing.T) {
	db := legacy(t)
	db.SetMaxOpenConns(2)
	c := connQueryOnly(t, db)
	tx, err := c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err = tx.QueryRowContext(context.Background(), `SELECT count(*) FROM main.sqlite_schema`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE VIEW later_schema_drift AS SELECT 1`); err != nil {
		t.Fatal(err)
	}
	v, err := InspectTx(context.Background(), tx)
	if err != nil || v != LegacyReference {
		t.Fatalf("pinned snapshot version=%v err=%v", v, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	tx, err = c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	v, err = InspectTx(context.Background(), tx)
	if v != Unknown || !errors.Is(err, ErrInvalid) {
		t.Fatalf("new snapshot failed to reject drift version=%v err=%v", v, err)
	}
}
