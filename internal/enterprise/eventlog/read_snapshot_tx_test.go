package eventlog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"testing"
)

// readerFor uses a separate query-only connection to the fixture's WAL DB.
func readerFor(t *testing.T, db *sql.DB) *sql.DB {
	t.Helper()
	var seq int
	var name, filename string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &filename); err != nil {
		t.Fatal(err)
	}
	r, err := sql.Open("sqlite", "file:"+url.PathEscape(filename)+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	r.SetMaxOpenConns(2)
	if err = r.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}
func readTx(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback() })
	return tx
}

func TestReadSnapshotTxWALConsistentAndCallerOwnsTx(t *testing.T) {
	db, l := logFixture(t)
	first := seedEvent(t, db, l, 1, GenesisHash())
	reader := readerFor(t, db)
	tx := readTx(t, reader)
	events, head, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 10)
	if err != nil || len(events) != 1 || head != first.Position {
		t.Fatalf("first %v %+v %v", events, head, err)
	}
	second := seedEvent(t, db, l, 2, first.Position.Hash)
	events, head, err = l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 10)
	if err != nil || len(events) != 1 || head != first.Position {
		t.Fatalf("old view %v %+v %v", events, head, err)
	}
	var n int
	if err = tx.QueryRow(`SELECT count(*) FROM main.replica_event`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("transaction unusable %d %v", n, err)
	}
	newer := readTx(t, reader)
	events, head, err = l.ReadSnapshotTx(context.Background(), newer, first.Position, 10)
	if err != nil || len(events) != 1 || head != second.Position {
		t.Fatalf("new view %v %+v %v", events, head, err)
	}
}
func TestReadSnapshotTxPreconditionsAndErrors(t *testing.T) {
	db, l := logFixture(t)
	g := GenesisHash()
	if _, err := db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,0)`, l.cluster[:], g[:]); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.ReadSnapshotTx(context.Background(), nil, Position{Epoch: 1}, 1); err == nil {
		t.Fatal("nil tx")
	}
	tx := readTx(t, db)
	if _, _, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 1); err == nil {
		t.Fatal("accepted writable connection")
	}
	tx.Rollback()
	reader := readerFor(t, db)
	tx = readTx(t, reader)
	if events, head, err := l.ReadSnapshotTx(nil, tx, Position{Epoch: 1}, 1); err == nil || len(events) != 0 || head != (Position{}) { //nolint:staticcheck // deliberate nil-context negative test
		t.Fatalf("nil context: %v %v %v", events, head, err)
	}
	for _, max := range []int{0, MaxBatchEvents + 1} {
		if _, _, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, max); err == nil {
			t.Fatal("bad max")
		}
	}
	if e, h, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 1); err != nil || len(e) != 0 || h != (Position{Epoch: 1}) {
		t.Fatalf("empty %v %v %v", e, h, err)
	}
	if _, _, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1, Seq: math.MaxUint64}, 1); !errors.Is(err, ErrChainBreak) {
		t.Fatalf("anchor %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e, h, err := l.ReadSnapshotTx(ctx, tx, Position{Epoch: 1}, 1); !errors.Is(err, context.Canceled) || len(e) != 0 || h != (Position{}) {
		t.Fatalf("canceled %v %v %v", e, h, err)
	}
	tx.Rollback()
	if _, _, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 1); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("closed %v", err)
	}
}
func TestReadSnapshotTxTempAndAttach(t *testing.T) {
	for _, tc := range []struct {
		name, setup string
		valid       bool
	}{{"empty temp", `SELECT count(*) FROM temp.sqlite_schema`, true}, {"temp shadow", `CREATE TEMP TABLE replica_event(x)`, false}, {"attached", `ATTACH DATABASE ':memory:' AS other`, false}} {
		t.Run(tc.name, func(t *testing.T) {
			db, l := logFixture(t)
			g := GenesisHash()
			if _, e := db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,0)`, l.cluster[:], g[:]); e != nil {
				t.Fatal(e)
			}
			c, e := db.Conn(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			if _, e = c.ExecContext(context.Background(), tc.setup); e != nil {
				t.Fatal(e)
			}
			if _, e = c.ExecContext(context.Background(), `PRAGMA query_only=ON`); e != nil {
				t.Fatal(e)
			}
			defer c.ExecContext(context.Background(), `PRAGMA query_only=OFF`)
			tx, e := c.BeginTx(context.Background(), nil)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback()
			_, _, e = l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 1)
			if (e == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, e)
			}
		})
	}
}
func TestReadSnapshotTxUsesSuppliedDatabase(t *testing.T) {
	db1, l1 := logFixture(t)
	db2, l2 := logFixture(t)
	_ = db1
	e := seedEvent(t, db2, l2, 1, GenesisHash())
	reader := readerFor(t, db2)
	tx := readTx(t, reader)
	got, head, err := l1.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 1)
	if err != nil || len(got) != 1 || head != e.Position {
		t.Fatalf("caller database %v %v", head, err)
	}
}
func TestReadSnapshotTxSQLiteErrorIsPreserved(t *testing.T) {
	db, l := logFixture(t)
	if _, err := db.Exec(`DROP TABLE replica_head`); err != nil {
		t.Fatal(err)
	}
	reader := readerFor(t, db)
	tx := readTx(t, reader)
	_, _, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 1)
	if err == nil {
		t.Fatal("missing table")
	}
	var se interface{ Code() int }
	if !errors.As(err, &se) {
		t.Fatalf("driver error lost: %T %v", err, err)
	}
}
func TestReadSnapshotTxPayloadBudget(t *testing.T) {
	db, l := logFixture(t)
	first := appendSizedEvent(t, db, l, 1, GenesisHash(), MaxPayloadBytes)
	second := appendSizedEvent(t, db, l, 2, first.Position.Hash, MaxPayloadBytes)
	third := seedEvent(t, db, l, 3, second.Position.Hash)
	reader := readerFor(t, db)
	tx := readTx(t, reader)
	events, head, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, MaxBatchEvents)
	if err != nil || len(events) != 2 || head != third.Position || len(events[0].Payload) != MaxPayloadBytes || len(events[1].Payload) != MaxPayloadBytes {
		t.Fatalf("events=%d head=%+v err=%v", len(events), head, err)
	}
	events, head, err = l.ReadSnapshotTx(context.Background(), tx, third.Position, 1)
	if err != nil || len(events) != 0 || head != third.Position {
		t.Fatalf("head repeat %v %+v %v", events, head, err)
	}
}

func TestReadLeaderPartialPrefixDoesNotAssumeUnseenMiddle(t *testing.T) {
	db, l := logFixture(t)
	a := seedEvent(t, db, l, 1, GenesisHash())
	foreign := *l
	foreign.node[0] = 9
	b := seedEvent(t, db, &foreign, 2, a.Position.Hash)
	seedEvent(t, db, l, 3, b.Position.Hash)
	if events, head, err := l.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1); err != nil || len(events) != 1 || head.Seq != 3 {
		t.Fatalf("unobserved middle %v %+v %v", events, head, err)
	}
	if events, err := l.Read(context.Background(), Position{Epoch: 1}, 1); err != nil || len(events) != 1 {
		t.Fatalf("legacy prefix %v %v", events, err)
	}
	if _, err := l.Read(context.Background(), Position{Epoch: 1}, 2); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("legacy mixed %v", err)
	}
}

func TestReadLeaderConsistencyAcrossAPIs(t *testing.T) {
	for _, foreignSeq := range []uint64{1, 2, 3} {
		t.Run(fmt.Sprint(foreignSeq), func(t *testing.T) {
			db, l := logFixture(t)
			var events []Event
			prev := GenesisHash()
			for i := uint64(1); i <= 3; i++ {
				node := *l
				if i == foreignSeq {
					node.node[0] = 7
				}
				e := seedEvent(t, db, &node, i, prev)
				events = append(events, e)
				prev = e.Position.Hash
			}
			for _, after := range []Position{{Epoch: 1}, events[0].Position, events[1].Position} {
				_, _, err := l.ReadSnapshot(context.Background(), after, 3)
				if foreignSeq < after.Seq {
					if err != nil {
						t.Errorf("unobserved foreign leader: %v", err)
					}
				} else if !errors.Is(err, ErrIdentityMismatch) {
					t.Errorf("after %d: %v", after.Seq, err)
				}
			}
			// With an at-head anchor only that event is observed, so no comparison to
			// an unobserved predecessor is possible or claimed.
			if e, _, err := l.ReadSnapshot(context.Background(), events[2].Position, 1); err != nil || len(e) != 0 {
				t.Fatalf("at head: %v", err)
			}
			reader := readerFor(t, db)
			tx := readTx(t, reader)
			if _, _, err := l.ReadSnapshotTx(context.Background(), tx, Position{Epoch: 1}, 3); !errors.Is(err, ErrIdentityMismatch) {
				t.Fatalf("tx mismatch %v", err)
			}
		})
	}
}
