package haregistry

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

var ctx = context.Background()

func sample() IssueRequest {
	var h [32]byte
	h[0] = 1
	return IssueRequest{ClusterID: "11111111-1111-4111-8111-111111111111", PredecessorID: "22222222-2222-4222-8222-222222222222", CandidateID: "33333333-3333-4333-8333-333333333333", Nonce: "44444444-4444-4444-8444-444444444444", ActiveEpoch: 1, NewEpoch: 2, PermitDigest: h, Requester: "operator"}
}
func open(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "registry.db")
	db, e := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(1000)")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = Init(ctx, tx); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	return db, path
}
func commitIssue(t *testing.T, db *sql.DB, r IssueRequest) error {
	t.Helper()
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = issue(ctx, tx, r); e != nil {
		return e
	}
	return tx.Commit()
}
func commitResolve(t *testing.T, db *sql.DB, nonce, kind string) {
	t.Helper()
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if e = resolve(ctx, tx, nonce, kind); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
}
func TestPublicDefaultClosed(t *testing.T) {
	db, _ := open(t)
	tx, _ := db.BeginTx(ctx, nil)
	defer tx.Rollback()
	for _, err := range []error{Issue(ctx, tx, sample()), Cancel(ctx, tx, sample().Nonce), Complete(ctx, tx, sample().Nonce)} {
		if !errors.Is(err, ErrNoAuthority) {
			t.Fatalf("got %v", err)
		}
	}
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM ha_issued`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows %d %v", n, err)
	}
}
func TestLifecycleAndReplay(t *testing.T) {
	db, _ := open(t)
	r := sample()
	if e := commitIssue(t, db, r); e != nil {
		t.Fatal(e)
	}
	r2 := r
	r2.Nonce = "55555555-5555-4555-8555-555555555555"
	r2.NewEpoch = 4
	if e := commitIssue(t, db, r2); e == nil {
		t.Fatal("two pending")
	}
	commitResolve(t, db, r.Nonce, "CANCELLED")
	if e := commitIssue(t, db, r); e == nil {
		t.Fatal("nonce reused")
	}
	r2.NewEpoch = 2
	if e := commitIssue(t, db, r2); e == nil {
		t.Fatal("epoch reused")
	}
	r2.NewEpoch = 4
	if e := commitIssue(t, db, r2); e != nil {
		t.Fatal(e)
	}
	commitResolve(t, db, r2.Nonce, "COMPLETED")
	r3 := r2
	r3.Nonce = "66666666-6666-4666-8666-666666666666"
	r3.NewEpoch = 3
	if e := commitIssue(t, db, r3); e == nil {
		t.Fatal("epoch rollback")
	}
	r3.NewEpoch = 5
	r3.ActiveEpoch = 0
	if e := commitIssue(t, db, r3); e == nil {
		t.Fatal("zero active")
	}
}
func TestCompletedEpochLowerBound(t *testing.T) {
	for _, outcome := range []string{"COMPLETED", "CANCELLED"} {
		t.Run(outcome, func(t *testing.T) {
			db, _ := open(t)
			first := sample()
			first.NewEpoch = 3
			if err := commitIssue(t, db, first); err != nil {
				t.Fatal(err)
			}
			commitResolve(t, db, first.Nonce, outcome)
			next := first
			next.Nonce = "55555555-5555-4555-8555-555555555555"
			next.NewEpoch = 4
			err := commitIssue(t, db, next)
			if outcome == "COMPLETED" {
				if err == nil {
					t.Fatal("accepted active epoch below completed permit")
				}
				next.ActiveEpoch = 3
				if err := commitIssue(t, db, next); err != nil {
					t.Fatalf("exact lower bound: %v", err)
				}
			} else if err != nil {
				t.Fatalf("cancelled permit incorrectly raised active bound: %v", err)
			}
		})
	}
}

func TestGlobalNonceAndActiveHistory(t *testing.T) {
	db, _ := open(t)
	r := sample()
	r.ActiveEpoch, r.NewEpoch = 3, 4
	if err := commitIssue(t, db, r); err != nil {
		t.Fatal(err)
	}
	commitResolve(t, db, r.Nonce, "CANCELLED")
	other := r
	other.ClusterID = "77777777-7777-4777-8777-777777777777"
	if err := commitIssue(t, db, other); err == nil {
		t.Fatal("nonce reused across clusters")
	}
	other = r
	other.Nonce = "55555555-5555-4555-8555-555555555555"
	other.ActiveEpoch, other.NewEpoch = 2, 5
	if err := commitIssue(t, db, other); err == nil {
		t.Fatal("active epoch regressed")
	}
}

func TestAtomicRollbackAndReopen(t *testing.T) {
	db, path := open(t)
	r := sample()
	tx, _ := db.BeginTx(ctx, nil)
	if e := issue(ctx, tx, r); e != nil {
		t.Fatal(e)
	}
	if e := tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	if e := commitIssue(t, db, r); e != nil {
		t.Fatal(e)
	}
	if e := db.Close(); e != nil {
		t.Fatal(e)
	}
	db2, e := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if e != nil {
		t.Fatal(e)
	}
	defer db2.Close()
	r.Nonce = "55555555-5555-4555-8555-555555555555"
	r.NewEpoch = 3
	if e = commitIssue(t, db2, r); e == nil {
		t.Fatal("lost pending after reopen")
	}
}
func TestSQLImmutability(t *testing.T) {
	db, _ := open(t)
	r := sample()
	if e := commitIssue(t, db, r); e != nil {
		t.Fatal(e)
	}
	tests := []string{`UPDATE ha_issued SET new_epoch=10`, `DELETE FROM ha_issued`, `INSERT OR REPLACE INTO ha_issued SELECT * FROM ha_issued`, `INSERT INTO ha_resolution(nonce,kind) VALUES('bogus','COMPLETED')`}
	for _, q := range tests {
		tx, _ := db.BeginTx(ctx, nil)
		if _, e := tx.Exec(q); e == nil {
			t.Fatalf("accepted %s", q)
		}
		tx.Rollback()
	}
	commitResolve(t, db, r.Nonce, "CANCELLED")
	for _, q := range []string{`UPDATE ha_resolution SET kind='COMPLETED'`, `DELETE FROM ha_resolution`, `INSERT OR REPLACE INTO ha_resolution SELECT * FROM ha_resolution`} {
		tx, _ := db.BeginTx(ctx, nil)
		if _, e := tx.Exec(q); e == nil {
			t.Fatalf("accepted %s", q)
		}
		tx.Rollback()
	}
}
func TestConcurrentIssue(t *testing.T) {
	db, _ := open(t)
	db.SetMaxOpenConns(4)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := sample()
			if i == 1 {
				r.Nonce = "55555555-5555-4555-8555-555555555555"
				r.NewEpoch = 3
			}
			tx, e := db.BeginTx(ctx, nil)
			if e == nil {
				defer tx.Rollback()
				e = issue(ctx, tx, r)
				if e == nil {
					e = tx.Commit()
				}
			}
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("wins=%d", wins)
	}
	var n int
	if e := db.QueryRow(`SELECT count(*) FROM ha_issued`).Scan(&n); e != nil || n != 1 {
		t.Fatalf("rows=%d err=%v", n, e)
	}
}

func TestExplicitRowidCannotReplaceRegistryHistory(t *testing.T) {
	db, _ := open(t)
	a := sample()
	if err := commitIssue(t, db, a); err != nil {
		t.Fatal(err)
	}
	b := a
	b.ClusterID = "77777777-7777-4777-8777-777777777777"
	b.Nonce = "55555555-5555-4555-8555-555555555555"
	if _, err := db.Exec(`INSERT OR REPLACE INTO ha_issued(rowid,cluster_id,predecessor_id,candidate_id,active_epoch,new_epoch,nonce,permit_digest,requester) VALUES(1,?,?,?,?,?,?,?,?)`, b.ClusterID, b.PredecessorID, b.CandidateID, b.ActiveEpoch, b.NewEpoch, b.Nonce, b.PermitDigest[:], b.Requester); err == nil {
		t.Fatal("accepted rowid replacement of issued history")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ha_issued WHERE nonce=?`, a.Nonce).Scan(&n); err != nil || n != 1 {
		t.Fatalf("original lost: %d %v", n, err)
	}
	if err := commitIssue(t, db, b); err != nil {
		t.Fatal(err)
	}
	commitResolve(t, db, a.Nonce, "COMPLETED")
	if _, err := db.Exec(`INSERT OR REPLACE INTO ha_resolution(rowid,nonce,kind) VALUES(1,?,'CANCELLED')`, b.Nonce); err == nil {
		t.Fatal("accepted rowid replacement of resolution")
	}
	var kind string
	if err := db.QueryRow(`SELECT kind FROM ha_resolution WHERE nonce=?`, a.Nonce).Scan(&kind); err != nil || kind != "COMPLETED" {
		t.Fatalf("resolution lost: %q %v", kind, err)
	}
	for _, table := range []string{"ha_issued", "ha_resolution"} {
		rows, err := db.Query(`PRAGMA index_list(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for rows.Next() {
			var seq, unique, partial int
			var name, origin string
			if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if unique == 1 {
				count++
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
		if count == 0 {
			t.Fatalf("%s lacks unique index", table)
		}
	}
}
