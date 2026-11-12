package eventlog

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func TestReadSnapshotSameSQLiteSnapshotAcrossWriter(t *testing.T) {
	db, l := logFixture(t)
	db.SetMaxOpenConns(2)
	first := seedEvent(t, db, l, 1, GenesisHash())
	entered, resume := make(chan struct{}), make(chan struct{})
	var resumeOnce sync.Once
	unblock := func() { resumeOnce.Do(func() { close(resume) }) }
	defer unblock()
	type result struct {
		events []Event
		head   Position
		err    error
	}
	done := make(chan result, 1)
	go func() {
		e, h, err := l.readSnapshot(context.Background(), Position{Epoch: 1}, 10, func() { close(entered); <-resume })
		done <- result{e, h, err}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		unblock()
		t.Fatal("reader did not observe head")
	}
	second := seedEvent(t, db, l, 2, first.Position.Hash)
	if db.Stats().OpenConnections < 2 {
		t.Fatal("writer did not use a second physical connection")
	}
	unblock()
	select {
	case r := <-done:
		if r.err != nil || r.head != first.Position || len(r.events) != 1 || r.events[0].Position != first.Position {
			t.Fatalf("inconsistent snapshot: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reader stuck")
	}
	events, head, err := l.ReadSnapshot(context.Background(), first.Position, 10)
	if err != nil || head != second.Position || len(events) != 1 || events[0].Position != second.Position {
		t.Fatalf("new snapshot: %v %v %v", events, head, err)
	}
}
func appendSizedEvent(t *testing.T, db *sql.DB, l *Log, seq uint64, prev [32]byte, size int) Event {
	t.Helper()
	base, err := MarshalChanges(Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: []byte(`{"family":4}`)})
	if err != nil || len(base) > size {
		t.Fatal(err)
	}
	payload := append(base, bytes.Repeat([]byte(" "), size-len(base))...)
	e := Event{ClusterID: l.cluster, LeaderID: l.node, Position: Position{Epoch: 1, Seq: seq}, Schema: 1, CommitMs: 1, PrevHash: prev, Payload: payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, l.cluster[:], 1, seq, l.node[:], 1, 1, prev[:], e.Position.Hash[:], payload); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,?,?,?,?,?) ON CONFLICT(cluster_id) DO UPDATE SET seq=excluded.seq,hash=excluded.hash`, l.cluster[:], 1, seq, e.Position.Hash[:], 1, 1); err != nil {
		t.Fatal(err)
	}
	return e
}
func TestReadSnapshotPayloadBoundAndAtHead(t *testing.T) {
	db, l := logFixture(t)
	first := appendSizedEvent(t, db, l, 1, GenesisHash(), MaxPayloadBytes)
	second := appendSizedEvent(t, db, l, 2, first.Position.Hash, MaxPayloadBytes)
	third := seedEvent(t, db, l, 3, second.Position.Hash)
	events, head, err := l.ReadSnapshot(context.Background(), Position{Epoch: 1}, 10)
	if err != nil || len(events) != 2 || head != third.Position || len(events[0].Payload) != MaxPayloadBytes || len(events[1].Payload) != MaxPayloadBytes {
		t.Fatalf("bounded prefix: len=%d head=%+v err=%v", len(events), head, err)
	}
	events, head, err = l.ReadSnapshot(context.Background(), third.Position, 10)
	if err != nil || len(events) != 0 || head != third.Position {
		t.Fatalf("head: %v %+v %v", events, head, err)
	}
}
func TestReadSnapshotRejectsInconsistentHeadAndAnchors(t *testing.T) {
	for _, kind := range []string{"head hash", "head schema", "head time", "head type", "missing head event", "missing head", "gap", "epoch", "wrong anchor"} {
		t.Run(kind, func(t *testing.T) {
			db, l := logFixture(t)
			first := seedEvent(t, db, l, 1, GenesisHash())
			second := seedEvent(t, db, l, 2, first.Position.Hash)
			after := Position{Epoch: 1}
			exec := func(query string) {
				t.Helper()
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "head hash":
				exec(`UPDATE replica_head SET hash=zeroblob(32)`)
			case "head schema":
				exec(`UPDATE replica_head SET schema_version=2`)
			case "head time":
				exec(`UPDATE replica_head SET advanced_at_unix_ms=999`)
			case "head type":
				exec(`UPDATE replica_head SET advanced_at_unix_ms='not-an-integer'`)
			case "missing head event":
				exec(`DROP TRIGGER replica_event_no_delete`)
				exec(`DELETE FROM replica_event WHERE seq=2`)
			case "missing head":
				exec(`DELETE FROM replica_head`)
			case "gap":
				exec(`DROP TRIGGER replica_event_no_delete`)
				exec(`DELETE FROM replica_event WHERE seq=1`)
			case "epoch":
				after.Epoch = 2
			case "wrong anchor":
				after = second.Position
				after.Hash[0] ^= 1
			}
			events, head, err := l.ReadSnapshot(context.Background(), after, 1)
			if err == nil || len(events) != 0 || head != (Position{}) {
				t.Fatalf("accepted %s: %+v %+v %v", kind, events, head, err)
			}
		})
	}
}
func TestReadSnapshotPreservesReadEventCauses(t *testing.T) {
	for _, kind := range []string{"head", "anchor", "event"} {
		t.Run(kind, func(t *testing.T) {
			db, l := logFixture(t)
			first := seedEvent(t, db, l, 1, GenesisHash())
			second := seedEvent(t, db, l, 2, first.Position.Hash)
			after := Position{Epoch: 1}
			if _, err := db.Exec(`DROP TRIGGER replica_event_no_delete`); err != nil {
				t.Fatal(err)
			}
			if kind == "head" {
				if _, err := db.Exec(`DELETE FROM replica_event WHERE seq=2`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`DELETE FROM replica_event WHERE seq=1`); err != nil {
					t.Fatal(err)
				}
				if kind == "anchor" {
					after = first.Position
				}
				_ = second
			}
			events, head, err := l.ReadSnapshot(context.Background(), after, 10)
			if !errors.Is(err, ErrChainBreak) || !errors.Is(err, sql.ErrNoRows) || len(events) != 0 || head != (Position{}) {
				t.Fatalf("%s: events=%v head=%+v err=%v", kind, events, head, err)
			}
		})
	}
}
func TestReadSnapshotPreservesSQLiteError(t *testing.T) {
	db, l := logFixture(t)
	seedEvent(t, db, l, 1, GenesisHash())
	if _, err := db.Exec(`DROP TABLE replica_event`); err != nil {
		t.Fatal(err)
	}
	events, head, err := l.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1)
	var sqliteErr *sqlite.Error
	if !errors.Is(err, ErrChainBreak) || !errors.As(err, &sqliteErr) || len(events) != 0 || head != (Position{}) {
		t.Fatalf("events=%v head=%+v err=%v", events, head, err)
	}
}
func TestDeadlinePropagationIsNotChainBreak(t *testing.T) {
	err := wrapReadEventError("event", context.DeadlineExceeded)
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrChainBreak) {
		t.Fatalf("deadline: %v", err)
	}
}
func TestReadSnapshotCancellationIsNotChainBreak(t *testing.T) {
	db, l := logFixture(t)
	seedEvent(t, db, l, 1, GenesisHash())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, head, err := l.readSnapshot(ctx, Position{Epoch: 1}, 1, cancel)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrChainBreak) || len(events) != 0 || head != (Position{}) {
		t.Fatalf("events=%v head=%+v err=%v", events, head, err)
	}
}

func TestReadSnapshotEmptyAndInvalidLimits(t *testing.T) {
	db, l := logFixture(t)
	genesis := GenesisHash()
	if _, err := db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,0)`, l.cluster[:], genesis[:]); err != nil {
		t.Fatal(err)
	}
	events, head, err := l.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1)
	if err != nil || len(events) != 0 || head != (Position{Epoch: 1}) {
		t.Fatalf("empty %v %+v %v", events, head, err)
	}
	if _, _, err = l.ReadSnapshot(context.Background(), Position{Epoch: 1}, 0); err == nil {
		t.Fatal("zero max")
	}
	if _, _, err = l.ReadSnapshot(context.Background(), Position{Epoch: 1}, MaxBatchEvents+1); err == nil {
		t.Fatal("too many")
	}
	if _, _, err = l.ReadSnapshot(context.Background(), Position{Epoch: 2}, 1); !errors.Is(err, ErrChainBreak) {
		t.Fatalf("epoch: %v", err)
	}
}
