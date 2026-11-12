package snapshotauditcheck

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

func fixture(t *testing.T) *sql.DB {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "audit.sqlite"))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := eventlog.Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}
func reader(t *testing.T, db *sql.DB) *sql.Conn {
	t.Helper()
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	return c
}
func limits(c Counts) Limits { return Limits{Rows: 100, Bytes: 4 << 20, Expected: c} }
func addReceipt(t *testing.T, db *sql.DB, tenant byte, body []byte, etag string) {
	t.Helper()
	var id [16]byte
	id[0] = tenant
	scope := idempotency.Scope{Tenant: id, Principal: "user:one", Method: "POST", Path: "/api/addresses", Key: "11111111-1111-4111-8111-111111111111"}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = idempotency.Insert(context.Background(), tx, scope, [32]byte{3}, idempotency.Receipt{Response: idempotency.Response{Status: 201, ETag: etag, Body: body}, CreatedAtMS: 1, ExpiresAtMS: 2}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
func addAudit(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO static_occupancy_audit(cluster_id,epoch,tenant_id,space_id,resource_id,principal,action,request_id,prior_revision,new_revision,result,occurred_at_ms,request_digest) VALUES(?,1,?,?,?,?, 'ipam.occupancy.create',?,0,1,'SUCCESS',1,?)`, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333", "user:one", "44444444-4444-4444-8444-444444444444", make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
}
func TestEmptyAndHistoricalExpiredRows(t *testing.T) {
	db := fixture(t)
	c := reader(t, db)
	out, err := Check(context.Background(), c, limits(Counts{}))
	if err != nil || !out.AuditRows || !out.ReceiptRows || out.Counts != (Counts{}) {
		t.Fatalf("empty %+v %v", out, err)
	}
	if _, err = c.ExecContext(context.Background(), `INSERT INTO idempotency_receipt DEFAULT VALUES`); err == nil {
		t.Fatal("query_only allowed write")
	}
	if _, err = c.ExecContext(context.Background(), `PRAGMA query_only=OFF`); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	addAudit(t, db)
	addReceipt(t, db, 0xfb, []byte("past"), `"v1"`)
	addReceipt(t, db, 0x03, []byte("old"), `"v1"`)
	c = reader(t, db)
	expected := Counts{Audit: 1, Receipts: 2}
	out, err = Check(context.Background(), c, limits(expected))
	if err != nil || out.Counts != expected {
		t.Fatalf("historical %+v %v", out, err)
	}
}
func TestMaximumReceipt(t *testing.T) {
	db := fixture(t)
	addReceipt(t, db, 1, make([]byte, idempotency.MaxBodyBytes), strings.Repeat("x", idempotency.MaxETagBytes))
	c := reader(t, db)
	out, err := Check(context.Background(), c, limits(Counts{Receipts: 1}))
	if err != nil || !out.ReceiptRows {
		t.Fatalf("maximum %+v %v", out, err)
	}
}
func TestRejectMalformedAndBudget(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		lim       Limits
	}{
		{"bad magic", `UPDATE idempotency_receipt SET response=x'0000000000000000'`, limits(Counts{Receipts: 1})},
		{"bad storage type", `UPDATE idempotency_receipt SET response='not blob'`, limits(Counts{Receipts: 1})},
		{"oversized blob", `UPDATE idempotency_receipt SET response=zeroblob(1049097)`, limits(Counts{Receipts: 1})},
		{"oversized text", `UPDATE idempotency_receipt SET principal=printf('%.*c',4097,'x')`, limits(Counts{Receipts: 1})},
		{"invalid times", `UPDATE idempotency_receipt SET expires_at_unix_ms=1`, limits(Counts{Receipts: 1})},
		{"wrong timestamp type", `UPDATE idempotency_receipt SET expires_at_unix_ms='invalid'`, limits(Counts{Receipts: 1})},
		{"wrong digest type", `UPDATE idempotency_receipt SET request_digest=printf('%032d',0)`, limits(Counts{Receipts: 1})},
		{"invalid status", `UPDATE idempotency_receipt SET response=x'4749523100000000'`, limits(Counts{Receipts: 1})},
		{"invalid etag framing", `UPDATE idempotency_receipt SET response=x'4749523100c80002'`, limits(Counts{Receipts: 1})},
		{"row budget", "", Limits{Rows: 1, Bytes: 100000, Expected: Counts{Receipts: 2}}},
		{"byte budget", "", Limits{Rows: 100, Bytes: 1, Expected: Counts{Receipts: 1}}},
		{"wrong count", "", limits(Counts{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := fixture(t)
			addReceipt(t, db, 1, []byte("payload"), "")
			if tc.name == "row budget" {
				addReceipt(t, db, 2, []byte("payload"), "")
			}
			if tc.sql != "" {
				if _, err := db.Exec(`DROP TRIGGER idempotency_receipt_immutable`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			out, err := Check(context.Background(), reader(t, db), tc.lim)
			if err == nil || out != (Evidence{}) {
				t.Fatalf("accepted %+v %v", out, err)
			}
		})
	}
}
func TestRejectUTF16(t *testing.T) {
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "utf16.db"))
	db.SetMaxOpenConns(1)
	defer db.Close()
	if _, err := db.Exec(`PRAGMA encoding='UTF-16'`); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	got, err := Check(context.Background(), reader(t, db), limits(Counts{}))
	if err == nil || got != (Evidence{}) {
		t.Fatalf("UTF-16 accepted: %+v %v", got, err)
	}
}

func TestConnectionIsolationAndCancellation(t *testing.T) {
	for _, kind := range []string{"not query only", "foreign keys off", "temp", "attached", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			db := fixture(t)
			c, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			ctx := context.Background()
			if kind != "not query only" {
				if _, err = c.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "foreign keys off":
				_, err = c.ExecContext(ctx, `PRAGMA foreign_keys=OFF`)
			case "temp":
				_, err = c.ExecContext(ctx, `PRAGMA query_only=OFF`)
				if err == nil {
					_, err = c.ExecContext(ctx, `CREATE TEMP TABLE idempotency_receipt(x)`)
				}
				if err == nil {
					_, err = c.ExecContext(ctx, `PRAGMA query_only=ON`)
				}
			case "attached":
				_, err = c.ExecContext(ctx, `ATTACH DATABASE ':memory:' AS other`)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			got, e := Check(ctx, c, limits(Counts{}))
			if e == nil || got != (Evidence{}) {
				t.Fatalf("accepted %+v %v", got, e)
			}
			if kind == "cancelled" && !errors.Is(e, context.Canceled) {
				t.Fatalf("cancel: %v", e)
			}
		})
	}
}
