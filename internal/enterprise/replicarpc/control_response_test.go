package replicarpc

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func controlOK(t *testing.T, r *replica.PullResponse, d replica.PullDisposition, epoch, seq uint64, limit int) {
	t.Helper()
	if r == nil || r.Disposition != d || len(r.Events) != 0 || r.RequiredBytes != 0 || r.ObservedHead == nil || r.ObservedHead.Epoch != epoch || r.ObservedHead.Seq != seq {
		t.Fatalf("control response %+v", r)
	}
	b, err := proto.Marshal(r)
	if err != nil || len(b) != proto.Size(r) || len(b) > limit {
		t.Fatalf("budget %d %v", len(b), err)
	}
}
func TestObservedEpochAndReadableSchemaControls(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	conn := connect(t, sv, f, &f.client, f.ca)
	// The epoch comes from SQLite, not the peer's claimed epoch.
	r := request()
	r.After.Epoch = 2
	r.After.Hash = bytes.Repeat([]byte{9}, 32)
	r.MaxBytes = 1024
	out, err := pull(conn, r)
	if err != nil {
		t.Fatal(err)
	}
	controlOK(t, out, replica.PullDisposition_EPOCH_REBASE_REQUIRED, 1, 1, 1024)
	// Only an actual verified next event can justify schema incompatibility.
	r = request()
	r.MaxBytes = 1024
	r.ReadableSchemaVersions = []uint32{2}
	out, err = pull(conn, r)
	if err != nil {
		t.Fatal(err)
	}
	controlOK(t, out, replica.PullDisposition_SCHEMA_INCOMPATIBLE, 1, 1, 1024)
	r.ReadableSchemaVersions = []uint32{2, 1}
	out, err = pull(conn, r)
	if err != nil || out.Disposition != replica.PullDisposition_REQUEST_BUDGET_TOO_SMALL {
		t.Fatalf("readable v1: %v %v", out, err)
	}
	r = request()
	r.After.Seq = 1
	r.After.Hash = bytes.Repeat([]byte{5}, 32)
	r.ReadableSchemaVersions = []uint32{2}
	if out, err = pull(conn, r); err == nil || out != nil {
		t.Fatalf("bad anchor inferred schema: %v %v", out, err)
	}
	r = request()
	normal, err := pull(conn, r)
	if err != nil {
		t.Fatal(err)
	}
	r.After = normal.ObservedHead
	r.ReadableSchemaVersions = []uint32{2}
	if out, err = pull(conn, r); status.Code(err) != codes.FailedPrecondition || out != nil {
		t.Fatalf("at head schema policy: %v %v", out, err)
	}
}
func TestEpochTwoObservedFromSQLite(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	conn := connect(t, sv, f, &f.client, f.ca)
	first, err := pull(conn, request())
	if err != nil {
		t.Fatal(err)
	}
	var prev [32]byte
	copy(prev[:], first.Events[0].Position.Hash)
	e := eventlog.Event{ClusterID: sv.service.cluster, LeaderID: sv.service.leader, Position: eventlog.Position{Epoch: 2, Seq: 1}, Schema: 1, CommitMs: 101, PrevHash: prev, Payload: first.Events[0].Payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`INSERT INTO main.replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,2,1,?,1,101,?,?,?)`, e.ClusterID[:], e.LeaderID[:], e.PrevHash[:], e.Position.Hash[:], e.Payload); err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`UPDATE main.replica_head SET epoch=2,seq=1,hash=?,advanced_at_unix_ms=101 WHERE cluster_id=?`, e.Position.Hash[:], e.ClusterID[:]); err != nil {
		t.Fatal(err)
	}
	out, err := pull(conn, request())
	if err != nil {
		t.Fatal(err)
	}
	controlOK(t, out, replica.PullDisposition_EPOCH_REBASE_REQUIRED, 2, 1, 8<<20)
	r := request()
	r.After.Epoch = 2
	r.After.Seq = 0
	r.After.Hash = prev[:]
	if out, err = pull(conn, r); err == nil || out != nil {
		t.Fatalf("untrusted handover accepted: %v %v", out, err)
	}
	r.After.Seq = 1
	r.After.Hash = e.Position.Hash[:]
	out, err = pull(conn, r)
	if err != nil || out.Disposition != replica.PullDisposition_AT_HEAD {
		t.Fatalf("epoch 2 at head: %v %v", out, err)
	}
}
func TestForeignHeadLeaderNeverReturnsDisposition(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	conn := connect(t, sv, f, &f.client, f.ca)
	first, err := pull(conn, request())
	if err != nil {
		t.Fatal(err)
	}
	var prev [32]byte
	copy(prev[:], first.Events[0].Position.Hash)
	foreign := sv.service.leader
	foreign[0] = 7
	e := eventlog.Event{ClusterID: sv.service.cluster, LeaderID: foreign, Position: eventlog.Position{Epoch: 1, Seq: 2}, Schema: 1, CommitMs: 101, PrevHash: prev, Payload: first.Events[0].Payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`INSERT INTO main.replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,1,2,?,1,101,?,?,?)`, e.ClusterID[:], e.LeaderID[:], e.PrevHash[:], e.Position.Hash[:], e.Payload); err != nil {
		t.Fatal(err)
	}
	if _, err = sv.service.db.Exec(`UPDATE main.replica_head SET seq=2,hash=?,advanced_at_unix_ms=101 WHERE cluster_id=?`, e.Position.Hash[:], e.ClusterID[:]); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		epoch, seq uint64
		hash       []byte
		schemas    []uint32
	}{{"at-head", 1, 2, e.Position.Hash[:], []uint32{1}}, {"at-head-no-v1", 1, 2, e.Position.Hash[:], []uint32{2}}, {"schema", 1, 1, prev[:], []uint32{2}}, {"rebase", 2, 0, make([]byte, 32), []uint32{1}}, {"prefix", 1, 0, make([]byte, 32), []uint32{1}}} {
		t.Run(tc.name, func(t *testing.T) {
			r := request()
			r.After.Epoch = tc.epoch
			r.After.Seq = tc.seq
			r.After.Hash = tc.hash
			r.ReadableSchemaVersions = tc.schemas
			if out, err := pull(conn, r); status.Code(err) != codes.FailedPrecondition || out != nil {
				t.Fatalf("foreign leader accepted: %v %v", out, err)
			}
		})
	}
}

func TestControlNeverEscapesFailedCleanupOrCancellation(t *testing.T) {
	for _, kind := range []string{"epoch", "schema"} {
		t.Run(kind+"-rollback", func(t *testing.T) {
			sv, f := testServer(t, nil, time.Time{})
			sv.service.rollback = func(tx *sql.Tx) error {
				if err := tx.Rollback(); err != nil {
					return err
				}
				return errors.New("simulated cleanup failure")
			}
			conn := connect(t, sv, f, &f.client, f.ca)
			r := request()
			if kind == "epoch" {
				r.After.Epoch = 2
			} else {
				r.ReadableSchemaVersions = []uint32{2}
			}
			if out, err := pull(conn, r); err == nil || out != nil {
				t.Fatalf("control escaped cleanup: %v %v", out, err)
			}
		})
		t.Run(kind+"-cancel", func(t *testing.T) {
			sv, f := testServer(t, nil, time.Time{})
			entered := make(chan struct{})
			finished := make(chan struct{})
			sv.service.hook = func(stage string, ctx context.Context) {
				if stage == "after-build" {
					close(entered)
					<-ctx.Done()
				}
			}
			sv.service.rollback = func(tx *sql.Tx) error { defer close(finished); return tx.Rollback() }
			conn := connect(t, sv, f, &f.client, f.ca)
			r := request()
			if kind == "epoch" {
				r.After.Epoch = 2
			} else {
				r.ReadableSchemaVersions = []uint32{2}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				out, err := replica.NewReplicationClient(conn).Pull(ctx, r)
				if out != nil {
					done <- errors.New("partial control")
					return
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("control build not reached")
			}
			cancel()
			select {
			case err := <-done:
				if status.Code(err) != codes.Canceled {
					t.Fatalf("cancel %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancel stuck")
			}
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("cleanup stuck")
			}
		})
	}
}

func TestUnverifiedEpochHeadNeverReturnsControl(t *testing.T) {
	for _, kind := range []string{"epoch2-genesis", "wrong-leader", "bad-hash", "unknown-schema"} {
		t.Run(kind, func(t *testing.T) {
			sv, f := testServer(t, nil, time.Time{})
			conn := connect(t, sv, f, &f.client, f.ca)
			if kind == "epoch2-genesis" {
				if _, err := sv.service.db.Exec(`UPDATE main.replica_head SET epoch=2,seq=0,hash=zeroblob(32),advanced_at_unix_ms=0`); err != nil {
					t.Fatal(err)
				}
			} else if kind == "bad-hash" {
				if _, err := sv.service.db.Exec(`UPDATE main.replica_head SET hash=zeroblob(32)`); err != nil {
					t.Fatal(err)
				}
			} else if kind == "unknown-schema" {
				if _, err := sv.service.db.Exec(`UPDATE main.replica_head SET schema_version=2`); err != nil {
					t.Fatal(err)
				}
			} else {
				// Configuration is independent from the structurally valid local head.
				sv.service.leader[0] = 99
			}
			r := request()
			r.After.Epoch = 3
			if out, err := pull(conn, r); err == nil || out != nil {
				t.Fatalf("unverified head %v %v", out, err)
			}
		})
	}
}
