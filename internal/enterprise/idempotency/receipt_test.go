package idempotency

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
)

func fixture(t *testing.T) (*sql.DB, Scope, Receipt) {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "idempotency.db") + "?_txlock=immediate")
	db.SetMaxOpenConns(2)
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE idempotency_receipt (
		tenant_id BLOB NOT NULL, principal TEXT NOT NULL, method TEXT NOT NULL, path TEXT NOT NULL,
		idempotency_key TEXT NOT NULL, request_digest BLOB NOT NULL, response BLOB NOT NULL,
		created_at_unix_ms INTEGER NOT NULL, expires_at_unix_ms INTEGER NOT NULL,
		PRIMARY KEY(tenant_id,principal,method,path,idempotency_key))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE business(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	return db, Scope{Tenant: [16]byte{1}, Principal: "user:one", Method: "POST", Path: "/api/v1/occupancies", Key: "11111111-1111-4111-8111-111111111111"}, Receipt{Response: Response{Status: 201, ETag: `"v1"`, Body: []byte{0, 1, 2, 255}}, CreatedAtMS: 1, ExpiresAtMS: 2}
}
func begin(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestIgnoredReceiptInsertRollsBackBusiness(t *testing.T) {
	db, scope, receipt := fixture(t)
	if _, err := db.Exec(`CREATE TRIGGER ignore_receipt BEFORE INSERT ON idempotency_receipt BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	tx := begin(t, db)
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO business(id) VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	if err := Insert(context.Background(), tx, scope, [32]byte{}, receipt); !errors.Is(err, ErrWrite) {
		t.Fatalf("ignored insert: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"business", "idempotency_receipt"} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s: count=%d err=%v", table, n, err)
		}
	}
}

type stubReceiptExec struct {
	result sql.Result
	err    error
}

func (s stubReceiptExec) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return s.result, s.err
}
func (s stubReceiptExec) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("unexpected query")
}

type stubReceiptResult struct {
	count int64
	err   error
}

func (r stubReceiptResult) LastInsertId() (int64, error) { return 0, nil }
func (r stubReceiptResult) RowsAffected() (int64, error) { return r.count, r.err }
func TestCreateStoredRequiresOneAffectedRow(t *testing.T) {
	failure := errors.New("rows affected unavailable")
	for _, tc := range []struct {
		name   string
		result sql.Result
		cause  error
	}{
		{"nil result", nil, nil}, {"zero rows", stubReceiptResult{}, nil}, {"two rows", stubReceiptResult{count: 2}, nil}, {"unavailable", stubReceiptResult{err: failure}, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := createStored(context.Background(), stubReceiptExec{result: tc.result}, Scope{}, Stored{})
			if !errors.Is(err, ErrWrite) || (tc.cause != nil && !errors.Is(err, tc.cause)) {
				t.Fatalf("error: %v", err)
			}
		})
	}
	if err := createStored(context.Background(), stubReceiptExec{result: stubReceiptResult{count: 1}}, Scope{}, Stored{}); err != nil {
		t.Fatal(err)
	}
}

func TestLookupRejectsCorruptDigestAndTimestampTypes(t *testing.T) {
	db, scope, receipt := fixture(t)
	tx := begin(t, db)
	defer tx.Rollback()
	// Shadow the constrained real schema to model a damaged or incompatible database.
	if _, err := tx.Exec(`CREATE TEMP TABLE idempotency_receipt (
		tenant_id, principal, method, path, idempotency_key, request_digest,
		response, created_at_unix_ms, expires_at_unix_ms)`); err != nil {
		t.Fatal(err)
	}
	encoded, err := encode(receipt.Response)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name                     string
		digest, created, expires any
	}{
		{"oversized digest", make([]byte, 10*MaxBodyBytes), int64(1), int64(2)},
		{"text digest", strings.Repeat("a", 32), int64(1), int64(2)},
		{"short digest", make([]byte, 31), int64(1), int64(2)},
		{"text created", make([]byte, 32), "1", int64(2)},
		{"blob expiry", make([]byte, 32), int64(1), []byte{2}},
		{"real expiry", make([]byte, 32), int64(1), 2.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tx.Exec(`DELETE FROM idempotency_receipt`); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO idempotency_receipt VALUES (?,?,?,?,?,?,?,?,?)`,
				scope.Tenant[:], scope.Principal, scope.Method, scope.Path, scope.Key,
				tc.digest, encoded, tc.created, tc.expires); err != nil {
				t.Fatal(err)
			}
			if _, found, err := Lookup(context.Background(), tx, scope, [32]byte{}); found || !errors.Is(err, ErrCorrupt) {
				t.Fatalf("found=%v err=%v", found, err)
			}
		})
	}
}

func TestLookupRejectsOversizedStoredResponse(t *testing.T) {
	db, scope, _ := fixture(t)
	tx := begin(t, db)
	defer tx.Rollback()
	_, err := tx.Exec(`INSERT INTO idempotency_receipt VALUES (?,?,?,?,?,?,zeroblob(?),?,?)`,
		scope.Tenant[:], scope.Principal, scope.Method, scope.Path, scope.Key,
		make([]byte, 32), maxEncodedBytes+10*MaxBodyBytes, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, digest := range [][32]byte{{}, {1}} {
		if _, found, err := Lookup(context.Background(), tx, scope, digest); found || !errors.Is(err, ErrCorrupt) {
			t.Fatalf("oversized response: found=%v err=%v", found, err)
		}
	}
}

func TestLookupAcceptsMaximumEncodedResponse(t *testing.T) {
	db, scope, receipt := fixture(t)
	receipt.Response.ETag = strings.Repeat("e", MaxETagBytes)
	receipt.Response.Body = make([]byte, MaxBodyBytes)
	tx := begin(t, db)
	defer tx.Rollback()
	if err := Insert(context.Background(), tx, scope, [32]byte{}, receipt); err != nil {
		t.Fatal(err)
	}
	got, found, err := Lookup(context.Background(), tx, scope, [32]byte{})
	if err != nil || !found || len(got.Response.Body) != MaxBodyBytes || got.Response.ETag != receipt.Response.ETag {
		t.Fatalf("maximum response: found=%v err=%v body=%d", found, err, len(got.Response.Body))
	}
}

func TestReceiptCommitReplayAndScope(t *testing.T) {
	db, s, r := fixture(t)
	ctx := context.Background()
	digest := [32]byte{1}
	tx := begin(t, db)
	if _, err := tx.Exec(`INSERT INTO business VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if err := Insert(ctx, tx, s, digest, r); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx = begin(t, db)
	defer tx.Rollback()
	got, found, err := Lookup(ctx, tx, s, digest)
	if err != nil || !found {
		t.Fatalf("lookup %v %v", found, err)
	}
	if got.Response.Status != r.Response.Status || got.Response.ETag != r.Response.ETag || string(got.Response.Body) != string(r.Response.Body) || got.ExpiresAtMS != 2 {
		t.Fatalf("changed response: %+v", got)
	}
	if _, _, err := Lookup(ctx, tx, s, [32]byte{2}); !errors.Is(err, ErrConflict) {
		t.Fatalf("digest collision: %v", err)
	}
	for _, change := range []func(*Scope){func(s *Scope) { s.Tenant[0] = 2 }, func(s *Scope) { s.Principal = "user:two" }, func(s *Scope) { s.Method = "PUT" }, func(s *Scope) { s.Path = "/other" }} {
		other := s
		change(&other)
		if _, found, err := Lookup(ctx, tx, other, digest); err != nil || found {
			t.Fatalf("scope leak %v %v", found, err)
		}
	}
}

func TestReceiptRollbackAndDuplicateFailureRollbackBusiness(t *testing.T) {
	db, s, r := fixture(t)
	ctx := context.Background()
	digest := [32]byte{1}
	tx := begin(t, db)
	if _, err := tx.Exec(`INSERT INTO business VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	if err := Insert(ctx, tx, s, digest, r); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	tx = begin(t, db)
	if _, found, err := Lookup(ctx, tx, s, digest); err != nil || found {
		t.Fatalf("rolled back receipt: %v %v", found, err)
	}
	if err := Insert(ctx, tx, s, digest, r); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx = begin(t, db)
	if _, err := tx.Exec(`INSERT INTO business VALUES(2)`); err != nil {
		t.Fatal(err)
	}
	if err := Insert(ctx, tx, s, [32]byte{2}, r); err == nil {
		t.Fatal("duplicate inserted")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM business`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("business writes survived rollback: %d %v", n, err)
	}
}

func TestTwoConnectionsSerializeDuplicateKey(t *testing.T) {
	db, s, r := fixture(t)
	ctx := context.Background()
	const workers = 2
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	conns := make([]*sql.Conn, workers)
	for i := range conns {
		var err error
		conns[i], err = db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conns[i].Close()
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(conn *sql.Conn) {
			defer wg.Done()
			<-start
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback()
			_, found, err := Lookup(ctx, tx, s, [32]byte{1})
			if err == nil && !found {
				err = Insert(ctx, tx, s, [32]byte{1}, r)
			}
			if err == nil {
				err = tx.Commit()
			}
			errs <- err
		}(conns[i])
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM idempotency_receipt`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("receipts %d: %v", n, err)
	}
	if db.Stats().OpenConnections < 2 {
		t.Fatal("test did not open two physical connections")
	}
}

func TestRejectInvalidInputAndCorruptResponse(t *testing.T) {
	db, s, r := fixture(t)
	ctx := context.Background()
	tx := begin(t, db)
	defer tx.Rollback()
	for _, change := range []func(*Scope){func(s *Scope) { s.Key = "bad" }, func(s *Scope) { s.Path = "/a/../b" }, func(s *Scope) { s.Path = "/a%2fb" }, func(s *Scope) { s.Method = "post" }, func(s *Scope) { s.Principal = "" }} {
		v := s
		change(&v)
		if err := Insert(ctx, tx, v, [32]byte{}, r); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid scope: %v", err)
		}
	}
	r.Response.Body = make([]byte, MaxBodyBytes+1)
	if err := Insert(ctx, tx, s, [32]byte{}, r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized body: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO idempotency_receipt VALUES (?,?,?,?,?,?,?,?,?)`, s.Tenant[:], s.Principal, s.Method, s.Path, s.Key, make([]byte, 32), []byte("legacy"), 1, 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Lookup(ctx, tx, s, [32]byte{}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corrupt response: %v", err)
	}
}
