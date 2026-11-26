package eventlog

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
)

// fixtureWriteTx bypasses the public, deliberately closed authority gate only
// for transaction tests. It is not role, fencing, or authorization evidence.
func fixtureWriteTx(t *testing.T, db *sql.DB, l *Log) *WriteTx {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	tx := &WriteTx{log: l, conn: conn, epoch: 1}
	if err = tx.readHead(ctx); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	return tx
}
func receiptTestSetup(t *testing.T) (*sql.DB, *Log, idempotency.Scope, idempotency.Receipt) {
	t.Helper()
	db, l := logFixture(t)
	if _, err := db.Exec(`CREATE TABLE business(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	genesis := GenesisHash()
	if _, err := db.Exec(`INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,1)`, l.cluster[:], genesis[:]); err != nil {
		t.Fatal(err)
	}
	return db, l, idempotency.Scope{Tenant: [16]byte{1}, Principal: "p", Method: "POST", Path: "/api/addresses", Key: "11111111-1111-4111-8111-111111111111"}, idempotency.Receipt{Response: idempotency.Response{Status: 201, ETag: `"v1"`, Body: []byte("ok")}, CreatedAtMS: 1, ExpiresAtMS: 2}
}
func addBusiness(t *testing.T, tx *WriteTx) {
	t.Helper()
	if _, err := tx.conn.ExecContext(context.Background(), `INSERT INTO business VALUES (1)`); err != nil {
		t.Fatal(err)
	}
}
func sampleChange() Change {
	return Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: []byte(`{"family":4}`)}
}
func assertCounts(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	for _, table := range []string{"business", "replica_event", "idempotency_receipt"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != want {
			t.Fatalf("%s: %d %v want %d", table, n, err, want)
		}
	}
	var seq int
	if err := db.QueryRow(`SELECT seq FROM replica_head`).Scan(&seq); err != nil || seq != want {
		t.Fatalf("head=%d err=%v", seq, err)
	}
}
func TestReadRejectsOversizedCorruptEventFields(t *testing.T) {
	for _, column := range []string{"leader_id", "prev_hash", "hash"} {
		t.Run(column, func(t *testing.T) {
			db, l := logFixture(t)
			seedEvent(t, db, l, 1, GenesisHash())
			// Deliberately corrupt the actual main database; a TEMP shadow is
			// no longer consulted by the reader.
			if _, err := db.Exec(`DROP TRIGGER replica_event_no_update`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE main.replica_event SET `+column+`=zeroblob(?)`, 10<<20); err != nil {
				t.Fatal(err)
			}
			if _, err := l.Read(context.Background(), Position{Epoch: 1}, 1); err == nil {
				t.Fatal("accepted oversized field")
			}
		})
	}
}

func TestWriteTxExposesNoArbitrarySQL(t *testing.T) {
	var tx any = &WriteTx{}
	if _, ok := tx.(interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	}); ok {
		t.Fatal("arbitrary SQL execution is exposed")
	}
	if _, ok := tx.(interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}); ok {
		t.Fatal("arbitrary SQL query is exposed")
	}
}

func TestPrivateFixtureAtomicReceiptAndReplay(t *testing.T) {
	db, l, s, r := receiptTestSetup(t)
	ctx := context.Background()
	digest := [32]byte{1}
	tx := fixtureWriteTx(t, db, l)
	if _, found, err := tx.LookupReceipt(ctx, s, digest); err != nil || found {
		t.Fatalf("lookup: %v %v", found, err)
	}
	addBusiness(t, tx)
	if _, err := tx.Append(ctx, 1, sampleChange()); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReceipt(ctx, s, digest, r); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertCounts(t, db, 1)
	tx = fixtureWriteTx(t, db, l)
	got, found, err := tx.LookupReceipt(ctx, s, digest)
	if err != nil || !found || string(got.Response.Body) != "ok" {
		t.Fatalf("replay: %+v %v %v", got, found, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertCounts(t, db, 1)
}
func TestReceiptBindingAndReplayPreventNewWrite(t *testing.T) {
	db, l, scope, receipt := receiptTestSetup(t)
	ctx := context.Background()
	digest := [32]byte{1}
	tx := fixtureWriteTx(t, db, l)
	if _, found, err := tx.LookupReceipt(ctx, scope, digest); err != nil || found {
		t.Fatalf("lookup: %v %v", found, err)
	}
	wrong := scope
	wrong.Principal = "other"
	if err := tx.InsertReceipt(ctx, wrong, digest, receipt); err == nil {
		t.Fatal("accepted a receipt for a different scope")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("scope mismatch committed")
	}
	assertCounts(t, db, 0)

	tx = fixtureWriteTx(t, db, l)
	if _, _, err := tx.LookupReceipt(ctx, scope, digest); err != nil {
		t.Fatal(err)
	}
	addBusiness(t, tx)
	if _, err := tx.Append(ctx, 1, sampleChange()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("committed after missing required receipt")
	}
	assertCounts(t, db, 0)

	tx = fixtureWriteTx(t, db, l)
	if _, _, err := tx.LookupReceipt(ctx, scope, digest); err != nil {
		t.Fatal(err)
	}
	addBusiness(t, tx)
	if _, err := tx.Append(ctx, 1, sampleChange()); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReceipt(ctx, scope, digest, receipt); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx = fixtureWriteTx(t, db, l)
	if _, found, err := tx.LookupReceipt(ctx, scope, digest); err != nil || !found {
		t.Fatalf("replay: %v %v", found, err)
	}
	if _, err := tx.Append(ctx, 2, sampleChange()); err == nil {
		t.Fatal("replay appended an event")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("replay committed a second event")
	}
	assertCounts(t, db, 1)
}

func TestCommittedEventOnlyAfterCommitAndIsCopied(t *testing.T) {
	db, l, _, _ := receiptTestSetup(t)
	ctx := context.Background()
	tx := fixtureWriteTx(t, db, l)
	pending, err := tx.Append(ctx, 1, sampleChange())
	if err != nil {
		t.Fatal(err)
	}
	if tx.Committed() != nil {
		t.Fatal("event reported committed before COMMIT")
	}
	pending.Payload[0] ^= 1
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	first := tx.Committed()
	if first == nil || first.Verify() != nil {
		t.Fatal("committed event affected by pending result mutation")
	}
	first.Payload[0] ^= 1
	if again := tx.Committed(); again == nil || again.Verify() != nil {
		t.Fatal("committed event affected by caller mutation")
	}
	tx = fixtureWriteTx(t, db, l)
	if _, err := tx.Append(ctx, 2, sampleChange()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if tx.Committed() != nil {
		t.Fatal("rolled-back event reported committed")
	}
}

func TestDuplicateAppendPoisonsTransaction(t *testing.T) {
	db, l, s, r := receiptTestSetup(t)
	ctx := context.Background()
	tx := fixtureWriteTx(t, db, l)
	if _, found, err := tx.LookupReceipt(ctx, s, [32]byte{1}); err != nil || found {
		t.Fatalf("lookup: %v %v", found, err)
	}
	addBusiness(t, tx)
	if _, err := tx.Append(ctx, 1, sampleChange()); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReceipt(ctx, s, [32]byte{1}, r); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Append(ctx, 1, sampleChange()); err != ErrAlreadyAppended {
		t.Fatalf("duplicate append: %v", err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("commit succeeded after failed append")
	}
	assertCounts(t, db, 0)
}

func TestPrivateFixtureAppendFailureRollsBack(t *testing.T) {
	db, l, s, r := receiptTestSetup(t)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TRIGGER block_event BEFORE INSERT ON replica_event BEGIN SELECT RAISE(ABORT,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	tx := fixtureWriteTx(t, db, l)
	if _, found, err := tx.LookupReceipt(ctx, s, [32]byte{}); err != nil || found {
		t.Fatalf("lookup: %v %v", found, err)
	}
	addBusiness(t, tx)
	if err := tx.InsertReceipt(ctx, s, [32]byte{}, r); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Append(ctx, 1, sampleChange()); err == nil {
		t.Fatal("append succeeded")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("dirty commit succeeded")
	}
	assertCounts(t, db, 0)
}
func TestPrivateFixtureReceiptFailureRollsBack(t *testing.T) {
	db, l, s, r := receiptTestSetup(t)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TRIGGER block_receipt BEFORE INSERT ON idempotency_receipt BEGIN SELECT RAISE(ABORT,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	tx := fixtureWriteTx(t, db, l)
	if _, found, err := tx.LookupReceipt(ctx, s, [32]byte{}); err != nil || found {
		t.Fatalf("lookup: %v %v", found, err)
	}
	addBusiness(t, tx)
	if _, err := tx.Append(ctx, 1, sampleChange()); err != nil {
		t.Fatal(err)
	}
	if err := tx.InsertReceipt(ctx, s, [32]byte{}, r); err == nil {
		t.Fatal("receipt insert succeeded")
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("dirty commit succeeded")
	}
	assertCounts(t, db, 0)
}
