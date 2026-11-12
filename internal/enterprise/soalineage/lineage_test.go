package soalineage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/soaserial"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

func dbForTest(t *testing.T) *sql.DB {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "lineage.db") + "?_txlock=immediate")
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { db.Close() })
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = Init(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return db
}
func entry(release byte, serial uint32, digest byte, deleted bool) Entry {
	e := Entry{Key: Key{"tenant", "view", "example."}, Serial: serial, Deleted: deleted}
	e.Release[15] = release
	e.Digest[0] = digest
	return e
}
func record(db *sql.DB, e Entry, bootstrap bool) (bool, error) {
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	dup, err := Record(ctx, tx, e, bootstrap)
	if err != nil {
		return false, err
	}
	return dup, tx.Commit()
}

func TestIgnoredInsertIsNotSuccess(t *testing.T) {
	db := dbForTest(t)
	if _, err := db.Exec(`CREATE TRIGGER skip_lineage BEFORE INSERT ON soa_lineage BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := record(db, entry(1, 1, 1, false), true); err == nil {
		t.Fatal("ignored lineage insertion reported success")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM soa_lineage`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unexpected rows: %d, %v", count, err)
	}
}

func TestLineageWrapAndTombstone(t *testing.T) {
	db := dbForTest(t)
	a := entry(1, ^uint32(0), 1, false)
	if _, err := record(db, a, false); !errors.Is(err, ErrMissing) {
		t.Fatalf("missing: %v", err)
	}
	if dup, err := record(db, a, true); err != nil || dup {
		t.Fatalf("first: %v %v", dup, err)
	}
	if dup, err := record(db, a, false); err != nil || !dup {
		t.Fatalf("repeat: %v %v", dup, err)
	}
	conflict := a
	conflict.Digest[0] = 9
	if _, err := record(db, conflict, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("release conflict: %v", err)
	}
	b := entry(2, 0, 2, false)
	if _, err := record(db, b, false); err != nil {
		t.Fatal(err)
	}
	same := entry(3, 0, 2, false)
	if _, err := record(db, same, false); err != nil {
		t.Fatal(err)
	}
	rollback := entry(4, 1, 1, false)
	if _, err := record(db, rollback, false); err != nil {
		t.Fatalf("rollback new serial: %v", err)
	}
	tomb := entry(5, 2, 3, true)
	if _, err := record(db, tomb, false); err != nil {
		t.Fatal(err)
	}
	recreate := entry(6, 3, 1, false)
	if _, err := record(db, recreate, false); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM soa_lineage`).Scan(&n); err != nil || n != 6 {
		t.Fatalf("history=%d err=%v", n, err)
	}
}
func TestRejectAmbiguousAndReusedSerial(t *testing.T) {
	db := dbForTest(t)
	a := entry(1, 0, 1, false)
	if _, err := record(db, a, true); err != nil {
		t.Fatal(err)
	}
	if _, err := record(db, entry(2, 1<<31, 2, false), false); !errors.Is(err, soaserial.ErrAmbiguous) {
		t.Fatalf("half: %v", err)
	}
	if _, err := record(db, entry(2, 0, 2, false), false); !errors.Is(err, ErrOrder) {
		t.Fatalf("same serial changed: %v", err)
	}
	if _, err := record(db, entry(2, ^uint32(0), 2, false), false); !errors.Is(err, ErrOrder) {
		t.Fatalf("old: %v", err)
	}
}
func TestReusedSerialAgainstOlderHistory(t *testing.T) {
	db := dbForTest(t)
	for i, serial := range []uint32{0, 0x7fffffff, 0xfffffffe} {
		if _, err := record(db, entry(byte(i+1), serial, byte(i+1), false), i == 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := record(db, entry(4, 0, 9, false), false); !errors.Is(err, ErrConflict) {
		t.Fatalf("historical collision: %v", err)
	}
}

func TestSchemaForbidsReplaceAndMutation(t *testing.T) {
	db := dbForTest(t)
	a := entry(1, 1, 1, false)
	if _, err := record(db, a, true); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DELETE FROM soa_lineage`, `UPDATE soa_lineage SET serial=2`, `INSERT OR REPLACE INTO soa_lineage(tenant,view,zone,release,serial,digest,deleted) SELECT tenant,view,zone,release,serial,digest,deleted FROM soa_lineage`, `INSERT OR REPLACE INTO soa_lineage(ordinal,tenant,view,zone,release,serial,digest,deleted) SELECT ordinal,tenant,view,zone,randomblob(16),serial,digest,deleted FROM soa_lineage`, `INSERT INTO soa_lineage(tenant,view,zone,release,serial,digest,deleted) SELECT tenant,view,zone,randomblob(16),serial,randomblob(32),deleted FROM soa_lineage`} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("accepted %s", q)
		}
	}
}
func TestOrdinalBoundsAndReplace(t *testing.T) {
	db := dbForTest(t)
	if _, err := record(db, entry(1, 1, 1, false), true); err != nil {
		t.Fatal(err)
	}
	var ordinal int64
	if err := db.QueryRow(`SELECT ordinal FROM soa_lineage`).Scan(&ordinal); err != nil || ordinal <= 0 {
		t.Fatalf("automatic ordinal=%d err=%v", ordinal, err)
	}
	for _, id := range []int64{0, -1, ordinal} {
		for _, verb := range []string{"INSERT", "INSERT OR REPLACE"} {
			q := verb + ` INTO soa_lineage(ordinal,tenant,view,zone,release,serial,digest,deleted) VALUES(?, 'tenant','view','example.',?,1,?,0)`
			release, digest := make([]byte, 16), make([]byte, 32)
			release[0] = 7
			digest[0] = 1
			if _, err := db.Exec(q, id, release, digest); err == nil {
				t.Errorf("%s accepted ordinal %d", verb, id)
			}
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM soa_lineage`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("history count=%d err=%v", count, err)
	}
}

func TestCallerRollsBackEntireTransactionOnError(t *testing.T) {
	db := dbForTest(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Record(ctx, tx, entry(1, 1, 1, false), true); err != nil {
		t.Fatal(err)
	}
	if _, err = Record(ctx, tx, entry(2, 1, 2, false), false); !errors.Is(err, ErrOrder) {
		t.Fatalf("expected conflict: %v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM soa_lineage`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback rows=%d err=%v", count, err)
	}
}

func TestConcurrentConflictingReleases(t *testing.T) {
	db := dbForTest(t)
	a := entry(1, 100, 1, false)
	if _, err := record(db, a, true); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, e := range []Entry{entry(2, 101, 2, false), entry(3, 101, 3, false)} {
		wg.Add(1)
		go func(e Entry) { defer wg.Done(); <-start; _, err := record(db, e, false); results <- err }(e)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, failures := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("success=%d failures=%d", successes, failures)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM soa_lineage`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("rows=%d %v", n, err)
	}
}
func TestRollbackAndInputBounds(t *testing.T) {
	db := dbForTest(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Record(context.Background(), tx, entry(1, 1, 1, false), true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err = record(db, entry(2, 2, 2, false), false); !errors.Is(err, ErrMissing) {
		t.Fatalf("rollback retained history: %v", err)
	}
	bad := entry(1, 1, 1, false)
	bad.Key.Zone = ""
	if _, err = record(db, bad, true); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
