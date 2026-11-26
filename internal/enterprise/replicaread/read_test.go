package replicaread

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlogguard"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	"github.com/jincaiw/GoDDI/internal/enterprise/pullbudget"
	"github.com/jincaiw/GoDDI/internal/enterprise/pullresponse"
	replica "github.com/jincaiw/GoDDI/internal/gen/replica/v1"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
	"google.golang.org/protobuf/proto"
)

func parseResponse(t *testing.T, wire []byte, limit int) *replica.PullResponse {
	t.Helper()
	var r replica.PullResponse
	if err := proto.Unmarshal(wire, &r); err != nil {
		t.Fatal(err)
	}
	if len(wire) != proto.Size(&r) || len(wire) > limit {
		t.Fatalf("bad wire length %d size %d", len(wire), proto.Size(&r))
	}
	return &r
}
func TestPublicReadAlwaysClosed(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, context.Background(), canceled} {
		out, err := Read(ctx)
		if !errors.Is(err, ErrNoAuthority) || out != nil {
			t.Fatalf("out=%v err=%v", out, err)
		}
	}
}
func TestCandidateReadPrefixAndBudget(t *testing.T) {
	f := makeFixture(t, 3)
	_, tx := beginReader(t, f)
	ctx := context.Background()
	after := eventlog.Position{Epoch: 1}
	wire, err := candidate(ctx, tx, f, after, 2, pullbudget.MaxBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := parseResponse(t, wire, pullbudget.MaxBytes)
	if int32(r.Disposition) != pullbudget.Batch || len(r.Events) != 2 || r.ObservedHead.Seq != 3 {
		t.Fatalf("bad prefix %+v", r)
	}
	for i, e := range r.Events {
		want := f.events[i]
		if e.Position.Seq != want.Position.Seq || !bytes.Equal(e.Position.Hash, want.Position.Hash[:]) || !bytes.Equal(e.PrevHash, want.PrevHash[:]) || !bytes.Equal(e.Payload, want.Payload) {
			t.Fatalf("event %d altered", i)
		}
	}
	wire, err = candidate(ctx, tx, f, after, 2, pullbudget.MinBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	r = parseResponse(t, wire, pullbudget.MinBytes)
	if int32(r.Disposition) != pullbudget.BudgetTooSmall || len(r.Events) != 0 || r.RequiredBytes <= pullbudget.MinBytes {
		t.Fatalf("bad too-small response %+v", r)
	}
	_, err = candidate(ctx, tx, f, after, 2, pullbudget.MinBytes-1, nil)
	if !errors.Is(err, pullresponse.ErrInvalid) {
		t.Fatalf("small invalid limit %v", err)
	}
	wire, err = candidate(ctx, tx, f, f.events[2].Position, 2, pullbudget.MaxBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	r = parseResponse(t, wire, pullbudget.MaxBytes)
	if int32(r.Disposition) != pullbudget.AtHead || len(r.Events) != 0 {
		t.Fatalf("bad at-head %+v", r)
	}
	if public, e := Read(ctx); !errors.Is(e, ErrNoAuthority) || public != nil {
		t.Fatalf("fixture evidence opened public boundary: %v", e)
	}
	wire, err = candidate(ctx, tx, f, after, 0, 0, nil)
	if err != nil {
		t.Fatalf("zero defaults: %v", err)
	}
	r = parseResponse(t, wire, pullbudget.MaxBytes)
	if len(r.Events) != 3 || int32(r.Disposition) != pullbudget.Batch {
		t.Fatalf("zero defaults response: %+v", r)
	}
	wire, err = candidate(ctx, tx, f, after, 1, 0, nil)
	if err != nil {
		t.Fatalf("one event limit: %v", err)
	}
	r = parseResponse(t, wire, pullbudget.MaxBytes)
	if len(r.Events) != 1 {
		t.Fatalf("one event limit returned %d", len(r.Events))
	}
	if wire, err = candidate(ctx, tx, f, after, pullbudget.MaxEvents+1, 0, nil); !errors.Is(err, pullresponse.ErrInvalid) || wire != nil {
		t.Fatalf("oversized event limit: %v", err)
	}
	var n int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM main.replica_event`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("caller tx unusable: %d %v", n, err)
	}
}
func TestCandidateRejectsWrongAnchorAndSchema(t *testing.T) {
	f := makeFixture(t, 1)
	_, tx := beginReader(t, f)
	bad := f.events[0].Position
	bad.Hash[0] ^= 1
	wire, err := candidate(context.Background(), tx, f, bad, 1, pullbudget.MaxBytes, nil)
	if err == nil || len(wire) != 0 {
		t.Fatalf("accepted wrong anchor: %v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`DROP TRIGGER replica_head_rowid_insert_guard`); err != nil {
		t.Fatal(err)
	}
	_, tx2 := beginReader(t, f)
	wire, err = candidate(context.Background(), tx2, f, eventlog.Position{Epoch: 1}, 1, pullbudget.MaxBytes, nil)
	if !errors.Is(err, eventlogguard.ErrInvalid) || len(wire) != 0 {
		t.Fatalf("accepted broken schema: %v", err)
	}
}
func TestCandidateRejectsLegacyReference(t *testing.T) {
	ctx := context.Background()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "legacy.sqlite"))
	db.SetMaxOpenConns(2)
	defer db.Close()
	if err := ipamstore.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Init(ctx, db); err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, cluster: eventlog.ID{1}, leader: eventlog.ID{2}}
	f.log = eventlog.NewLog(db, f.cluster, f.leader)
	_, tx := beginReader(t, f)
	wire, err := candidate(ctx, tx, f, eventlog.Position{Epoch: 1}, 1, pullbudget.MaxBytes, nil)
	if !errors.Is(err, errTestLegacy) || len(wire) != 0 {
		t.Fatalf("accepted legacy: %v", err)
	}
}

func TestCandidateRejectsMarkerDrift(t *testing.T) {
	f := makeFixture(t, 1)
	if _, err := f.db.Exec(`UPDATE main.eventlog_reference_guard_migrations SET guard_digest='forged'`); err != nil {
		t.Fatal(err)
	}
	_, tx := beginReader(t, f)
	wire, err := candidate(context.Background(), tx, f, eventlog.Position{Epoch: 1}, 1, pullbudget.MaxBytes, nil)
	if !errors.Is(err, eventlogguard.ErrInvalid) || len(wire) != 0 {
		t.Fatalf("accepted forged marker: %v", err)
	}
}

func TestCandidateRejectsWrongLeaderAndEpoch(t *testing.T) {
	f := makeFixture(t, 1)
	_, tx := beginReader(t, f)
	f.leader = eventlog.ID{9}
	wire, err := candidate(context.Background(), tx, f, eventlog.Position{Epoch: 1}, 1, pullbudget.MaxBytes, nil)
	if !errors.Is(err, pullresponse.ErrInvalid) || len(wire) != 0 {
		t.Fatalf("accepted wrong leader: %v", err)
	}
	wire, err = candidate(context.Background(), tx, f, eventlog.Position{Epoch: 2}, 1, pullbudget.MaxBytes, nil)
	if err == nil || len(wire) != 0 {
		t.Fatalf("accepted wrong epoch: %v", err)
	}
}

// This deliberately violates InspectTx's caller isolation precondition to
// observe SQLite WAL snapshot behavior; it does not relax that precondition.
func TestCandidateSnapshotSurvivesLaterWALWriteAndDDL(t *testing.T) {
	f := makeFixture(t, 2)
	_, tx := beginReader(t, f)
	old := f.events[1]
	hook := func() error {
		appendFixtureEvent(t, f)
		_, err := f.db.Exec(`DROP TRIGGER replica_head_rowid_insert_guard`)
		return err
	}
	wire, err := candidate(context.Background(), tx, f, eventlog.Position{Epoch: 1}, 2, pullbudget.MaxBytes, hook)
	if err != nil {
		t.Fatal(err)
	}
	r := parseResponse(t, wire, pullbudget.MaxBytes)
	if len(r.Events) != 2 || r.ObservedHead.Seq != old.Position.Seq || !bytes.Equal(r.ObservedHead.Hash, old.Position.Hash[:]) {
		t.Fatalf("mixed snapshots %+v", r)
	}
	var exists int
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM main.sqlite_schema WHERE type='trigger' AND name='replica_head_rowid_insert_guard')`).Scan(&exists); err != nil || exists != 1 {
		t.Fatalf("schema snapshot changed %d %v", exists, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	_, newTx := beginReader(t, f)
	if v, err := eventlogguard.InspectTx(context.Background(), newTx); err == nil || v != eventlogguard.Unknown {
		t.Fatalf("new schema drift accepted v=%v err=%v", v, err)
	}
}
func TestCandidateCancellationAfterBuildDiscardsWire(t *testing.T) {
	f := makeFixture(t, 1)
	_, tx := beginReader(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	built := false
	wire, err := candidateWithHooks(ctx, tx, f, eventlog.Position{Epoch: 1}, 1, 0, nil, func(encoded []byte, buildErr error) {
		built = buildErr == nil && len(encoded) > 0
		cancel()
	})
	if !built || !errors.Is(err, context.Canceled) || wire != nil {
		t.Fatalf("built=%v wire=%v err=%v", built, wire, err)
	}
}

func TestCandidateCanceledDoesNotReturnWire(t *testing.T) {
	f := makeFixture(t, 1)
	_, tx := beginReader(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	wire, err := candidate(ctx, tx, f, eventlog.Position{Epoch: 1}, 1, pullbudget.MaxBytes, func() error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) || len(wire) != 0 {
		t.Fatalf("wire=%v err=%v", wire, err)
	}
}
