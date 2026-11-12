package replicaread

// All database construction and composition here is test-only. The empty
// t.TempDir reference is not a migration, source authentication or authority.
import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/eventlogguard"
	"github.com/jasonwa/goddi/internal/enterprise/ipamstore"
	"github.com/jasonwa/goddi/internal/enterprise/pullbudget"
	"github.com/jasonwa/goddi/internal/enterprise/pullresponse"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

type fixture struct {
	db              *sql.DB
	log             *eventlog.Log
	cluster, leader eventlog.ID
	events          []eventlog.Event
}

func makeFixture(t *testing.T, n int) *fixture {
	t.Helper()
	ctx := context.Background()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "fixture.sqlite"))
	db.SetMaxOpenConns(3)
	t.Cleanup(func() { _ = db.Close() })
	if err := ipamstore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(ctx, db); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(fixtureGuardSQL))) != fixtureGuardDigest || fmt.Sprintf("%x", sha256.Sum256([]byte(eventlog.Schema))) != fixtureEventDigest {
		t.Fatal("reference digest mismatch")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.ipam_tenants UNION ALL SELECT 1 FROM main.ipam_spaces UNION ALL SELECT 1 FROM main.ipam_prefixes UNION ALL SELECT 1 FROM main.ipam_pools UNION ALL SELECT 1 FROM main.ipam_occupancies UNION ALL SELECT 1 FROM main.ipam_resource_ids UNION ALL SELECT 1 FROM main.writer_authority UNION ALL SELECT 1 FROM main.replica_head UNION ALL SELECT 1 FROM main.replica_event UNION ALL SELECT 1 FROM main.replica_checkpoint UNION ALL SELECT 1 FROM main.idempotency_receipt UNION ALL SELECT 1 FROM main.static_occupancy_audit)`).Scan(&exists); err != nil || exists != 0 {
		t.Fatalf("not empty: %d %v", exists, err)
	}
	if _, err = tx.ExecContext(ctx, fixtureGuardSQL); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, fixtureGuardMarker); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO main.eventlog_reference_guard_migrations VALUES(1,?,?)`, fixtureEventDigest, fixtureGuardDigest); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, cluster: eventlog.ID{1}, leader: eventlog.ID{2}}
	f.log = eventlog.NewLog(db, f.cluster, f.leader)
	genesis := eventlog.GenesisHash()
	if _, err = db.ExecContext(ctx, `INSERT INTO main.replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,0)`, f.cluster[:], genesis[:]); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		appendFixtureEvent(t, f)
	}
	return f
}
func appendFixtureEvent(t *testing.T, f *fixture) eventlog.Event {
	t.Helper()
	ctx := context.Background()
	seq := uint64(len(f.events) + 1)
	prev := eventlog.GenesisHash()
	if len(f.events) > 0 {
		prev = f.events[len(f.events)-1].Position.Hash
	}
	body := []byte(`{"message":"` + strings.Repeat("x", 1500) + `"}`)
	payload, err := eventlog.MarshalChanges(eventlog.Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	e := eventlog.Event{ClusterID: f.cluster, LeaderID: f.leader, Position: eventlog.Position{Epoch: 1, Seq: seq}, Schema: eventlog.SchemaVersionV1, CommitMs: int64(seq), PrevHash: prev, Payload: payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO main.replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, e.ClusterID[:], 1, seq, e.LeaderID[:], e.Schema, e.CommitMs, e.PrevHash[:], e.Position.Hash[:], e.Payload); err != nil {
		t.Fatal(err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE main.replica_head SET seq=?,hash=?,advanced_at_unix_ms=? WHERE cluster_id=? AND epoch=1 AND seq=? AND hash=?`, seq, e.Position.Hash[:], e.CommitMs, f.cluster[:], seq-1, prev[:])
	if err != nil {
		t.Fatal(err)
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		t.Fatalf("head CAS: %d %v", n, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	f.events = append(f.events, e)
	return e
}
func beginReader(t *testing.T, f *fixture) (*sql.Conn, *sql.Tx) {
	t.Helper()
	ctx := context.Background()
	c, err := f.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err = c.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	tx, err := c.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return c, tx
}

var errTestLegacy = errors.New("test: unguarded reference")

// candidate runs entirely in the caller's tx. The output is test data only;
// neither the schema version nor observed head authenticates a role or peer.
func candidate(ctx context.Context, tx *sql.Tx, f *fixture, after eventlog.Position, maxEvents, maxBytes uint32, hook func() error) ([]byte, error) {
	return candidateWithHooks(ctx, tx, f, after, maxEvents, maxBytes, hook, nil)
}

func candidateWithHooks(ctx context.Context, tx *sql.Tx, f *fixture, after eventlog.Position, maxEvents, maxBytes uint32, hook func() error, afterBuild func([]byte, error)) ([]byte, error) {
	if maxEvents == 0 {
		maxEvents = pullbudget.MaxEvents
	}
	if maxEvents > pullbudget.MaxEvents {
		return nil, pullresponse.ErrInvalid
	}
	if maxBytes == 0 {
		maxBytes = pullbudget.MaxBytes
	}
	v, err := eventlogguard.InspectTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if v != eventlogguard.HardenedReference {
		return nil, errTestLegacy
	}
	if hook != nil {
		if err = hook(); err != nil {
			return nil, err
		}
	}
	events, head, err := f.log.ReadSnapshotTx(ctx, tx, after, int(maxEvents))
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	wire, err := pullresponse.Build(pullresponse.Input{Cluster: f.cluster, Leader: f.leader, After: after, Head: head, Events: events, MaxEvents: maxEvents, MaxBytes: maxBytes, ReadableSchemas: []uint32{eventlog.SchemaVersionV1}})
	if afterBuild != nil {
		afterBuild(wire, err)
	}
	if ctx.Err() != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	if err != nil {
		return nil, err
	}
	return wire, nil
}
