package eventlog

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

func logFixture(t *testing.T) (*sql.DB, *Log) {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "events.db"))
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var cluster, node ID
	cluster[15] = 1
	node[15] = 2
	return db, NewLog(db, cluster, node)
}

func seedEvent(t *testing.T, db *sql.DB, l *Log, seq uint64, prev [32]byte) Event {
	t.Helper()
	payload, err := MarshalChanges(Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: []byte(`{"family":4}`)})
	if err != nil {
		t.Fatal(err)
	}
	e := Event{ClusterID: l.cluster, LeaderID: l.node, Position: Position{Epoch: 1, Seq: seq}, Schema: SchemaVersionV1, CommitMs: 1, PrevHash: prev, Payload: payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, l.cluster[:], 1, seq, l.node[:], e.Schema, e.CommitMs, e.PrevHash[:], e.Position.Hash[:], e.Payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,?,?,?,?,?) ON CONFLICT(cluster_id) DO UPDATE SET seq=excluded.seq,hash=excluded.hash`, l.cluster[:], 1, seq, e.Position.Hash[:], 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAuthorityDatabaseRowNeverGrantsWrite(t *testing.T) {
	db, l := logFixture(t)
	_, err := db.Exec(`INSERT INTO writer_authority VALUES(?,?,?,?,?,?)`, l.cluster[:], l.node[:], 1, "gen", make([]byte, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.Begin(context.Background()); !errors.Is(err, ErrNoAuthority) {
		t.Fatalf("database row authorized protected write: %v", err)
	}
}

func TestCommitWithoutEventRollsBack(t *testing.T) {
	db, l := logFixture(t)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `CREATE TABLE unlogged(id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	tx := &WriteTx{log: l, conn: conn}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("unlogged transaction committed")
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='unlogged'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unlogged write survived: %d, %v", n, err)
	}
}

func TestDirtyCommitReturnsErrorAndRollsBack(t *testing.T) {
	db, l := logFixture(t)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE dirty_write(id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	tx := &WriteTx{log: l, conn: conn, dirty: true, appended: &Event{}}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("dirty transaction reported success")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='dirty_write'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dirty transaction persisted: %d %v", n, err)
	}
}

func TestReceiptCannotBeReplaced(t *testing.T) {
	db, _ := logFixture(t)
	tenant := make([]byte, 16)
	digest := make([]byte, 32)
	insert := `INSERT INTO idempotency_receipt VALUES (?, 'principal', 'POST', '/path', 'key', ?, ?, 1, 2)`
	if _, err := db.Exec(insert, tenant, digest, []byte("original")); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO idempotency_receipt VALUES (?, 'principal', 'POST', '/path', 'key', ?, ?, 1, 2)`, tenant, digest, []byte("replacement"))
	if err == nil {
		t.Fatal("REPLACE overwrote an idempotency receipt")
	}
	var response []byte
	if err := db.QueryRow(`SELECT response FROM idempotency_receipt`).Scan(&response); err != nil || string(response) != "original" {
		t.Fatalf("original receipt changed: %q, %v", response, err)
	}
}

func TestEventPositionCannotBeReplaced(t *testing.T) {
	db, l := logFixture(t)
	e := seedEvent(t, db, l, 1, GenesisHash())
	_, err := db.Exec(`INSERT OR REPLACE INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, l.cluster[:], 1, 1, l.node[:], 1, 2, e.PrevHash[:], e.Position.Hash[:], []byte(`{}`))
	if err == nil {
		t.Fatal("REPLACE rewrote an immutable event")
	}
	var stored []byte
	if err := db.QueryRow(`SELECT payload FROM replica_event WHERE cluster_id=? AND epoch=1 AND seq=1`, l.cluster[:]).Scan(&stored); err != nil || string(stored) != string(e.Payload) {
		t.Fatalf("original event changed: %s, %v", stored, err)
	}
}

func TestMissingHeadIsNotRecreated(t *testing.T) {
	db, l := logFixture(t)
	seedEvent(t, db, l, 1, GenesisHash())
	if _, err := db.Exec(`DELETE FROM replica_head`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Head(context.Background()); err == nil {
		t.Fatal("missing head accepted")
	}
	if _, err := l.Read(context.Background(), Position{Epoch: 1}, 1); err == nil {
		t.Fatal("missing head accepted for read")
	}
}

func TestRollbackFailureRetiresPhysicalConnection(t *testing.T) {
	db, l := logFixture(t)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `CREATE TEMP TABLE connection_marker(id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	tx := &WriteTx{log: l, conn: conn}
	if err = tx.Rollback(ctx); err == nil {
		t.Fatal("rollback without transaction unexpectedly succeeded")
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_temp_master WHERE name='connection_marker'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("failed connection reused: %d, %v", n, err)
	}
}

func TestReadVerifiesChainAndDigest(t *testing.T) {
	db, l := logFixture(t)
	ctx := context.Background()
	first := seedEvent(t, db, l, 1, GenesisHash())
	second := seedEvent(t, db, l, 2, first.Position.Hash)
	got, err := l.Read(ctx, Position{Epoch: 1}, 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("read: %d %v", len(got), err)
	}
	if _, err = l.Read(ctx, Position{Epoch: 1, Seq: 2, Hash: [32]byte{9}}, 10); err == nil {
		t.Fatal("accepted false head anchor")
	}
	if _, err = db.Exec(`DROP TRIGGER replica_event_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE replica_event SET payload=? WHERE seq=2`, []byte(`{"changes":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Read(ctx, first.Position, 10); err == nil {
		t.Fatal("accepted corrupt payload")
	}
	_ = second
}

func TestReadRefusesSequenceGapAndHeadMismatch(t *testing.T) {
	db, l := logFixture(t)
	ctx := context.Background()
	first := seedEvent(t, db, l, 1, GenesisHash())
	seedEvent(t, db, l, 3, first.Position.Hash)
	if _, err := l.Read(ctx, Position{Epoch: 1}, 10); err == nil {
		t.Fatal("accepted gap")
	}
	if _, err := db.Exec(`UPDATE replica_head SET hash=?`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Read(ctx, Position{Epoch: 1, Seq: 3, Hash: [32]byte{}}, 10); err == nil {
		t.Fatal("accepted head without matching event hash")
	}
}

func TestRecordAppliedRefusesUnverifiedPeer(t *testing.T) {
	db, l := logFixture(t)
	var peer ID
	peer[0] = 1
	if err := l.RecordApplied(context.Background(), peer, Position{Epoch: 1}, time.Now()); !errors.Is(err, ErrNoAuthority) {
		t.Fatalf("accepted unverified confirmation: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM replica_checkpoint`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("checkpoint changed: %d %v", n, err)
	}
}

func TestReadRejectsCorruptAnchorAtHead(t *testing.T) {
	db, l := logFixture(t)
	e := seedEvent(t, db, l, 1, GenesisHash())
	if _, err := db.Exec(`DROP TRIGGER replica_event_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE replica_event SET payload=? WHERE seq=1`, []byte(`{"changes":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Read(context.Background(), e.Position, 1); err == nil {
		t.Fatal("corrupt anchor accepted as empty result")
	}
}

func TestReadPayloadBudgetDoesNotSkip(t *testing.T) {
	db, l := logFixture(t)
	first := seedEvent(t, db, l, 1, GenesisHash())
	second := seedEvent(t, db, l, 2, first.Position.Hash)
	ctx := context.Background()
	if _, err := l.ReadWithPayloadBudget(ctx, Position{Epoch: 1}, 2, len(first.Payload)-1); !errors.Is(err, ErrPayloadBudgetTooSmall) {
		t.Fatalf("expected budget error: %v", err)
	}
	got, err := l.ReadWithPayloadBudget(ctx, Position{Epoch: 1}, 2, len(first.Payload))
	if err != nil || len(got) != 1 || got[0].Position != first.Position {
		t.Fatalf("first page: %v %v", got, err)
	}
	got, err = l.ReadWithPayloadBudget(ctx, first.Position, 2, len(second.Payload))
	if err != nil || len(got) != 1 || got[0].Position != second.Position {
		t.Fatalf("second page: %v %v", got, err)
	}
	if _, err := l.ReadWithPayloadBudget(ctx, Position{Epoch: 1}, 2, MaxBatchPayloadBytes+1); err == nil {
		t.Fatal("accepted excessive budget")
	}
}

func TestReadOversizedPayloadIsNotLoaded(t *testing.T) {
	db, l := logFixture(t)
	e := seedEvent(t, db, l, 1, GenesisHash())
	if _, err := db.Exec(`DROP TRIGGER replica_event_no_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE replica_event SET payload=zeroblob(?) WHERE seq=1`, MaxPayloadBytes+1); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	row, size, err := l.readEvent(ctx, tx, 1, 1, MaxBatchPayloadBytes)
	if err != nil {
		t.Fatal(err)
	}
	if size != MaxPayloadBytes+1 || row.Payload != nil {
		t.Fatalf("oversized payload loaded: size=%d bytes=%d", size, len(row.Payload))
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Read(ctx, Position{Epoch: 1}, 1); !errors.Is(err, ErrPayloadSize) {
		t.Fatalf("oversized event: %v", err)
	}
	if _, err := l.Read(ctx, e.Position, 1); !errors.Is(err, ErrPayloadSize) {
		t.Fatalf("oversized anchor: %v", err)
	}
}
