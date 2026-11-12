package pullresponse

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/pullbudget"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"google.golang.org/protobuf/proto"
)

func TestSnapshotLargeEpochAndSecondEventRoundTrip(t *testing.T) {
	db, l, cluster, leader := fixture(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var prev [32]byte
	var events []eventlog.Event
	for seq := uint64(1); seq <= 3; seq++ {
		payload, err := eventlog.MarshalChanges(eventlog.Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: []byte(`{"family":4}`)})
		if err != nil {
			t.Fatal(err)
		}
		payload = append(payload, bytes.Repeat([]byte(" "), int(seq))...)
		e := eventlog.Event{ClusterID: cluster, LeaderID: leader, Position: eventlog.Position{Epoch: 128, Seq: seq}, Schema: eventlog.SchemaVersionV1, CommitMs: math.MaxInt64, PrevHash: prev, Payload: payload}
		e.Position.Hash, err = e.Hash()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.ExecContext(context.Background(), `INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, cluster[:], 128, seq, leader[:], 1, int64(math.MaxInt64), prev[:], e.Position.Hash[:], payload); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
		prev = e.Position.Hash
	}
	if _, err = c.ExecContext(context.Background(), `INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,128,3,?,1,?)`, cluster[:], prev[:], int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	in := get(t, l, cluster, leader, events[0].Position, 10)
	raw, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	out := unmarshal(t, raw)
	if len(out.Events) != 2 || out.ObservedHead.Epoch != 128 {
		t.Fatalf("events/head: %v", out)
	}
	second := out.Events[1]
	original := events[2]
	if second.Position.Epoch != 128 || second.Position.Seq != original.Position.Seq || second.CommittedUnixMs != math.MaxInt64 || !bytes.Equal(second.Position.Hash, original.Position.Hash[:]) || !bytes.Equal(second.PrevHash, original.PrevHash[:]) || !bytes.Equal(second.Payload, original.Payload) {
		t.Fatal("second event fields differ")
	}
	// Values beyond SQLite's signed range are rejected by the builder even
	// though protobuf itself could encode them.
	in.Head.Seq = uint64(math.MaxInt64) + 1
	if out, err := Build(in); err == nil || out != nil {
		t.Fatal("accepted out-of-range SQLite sequence")
	}
}
func TestWireSizeUint32AndInt64Maximum(t *testing.T) {
	// This exercises protobuf sizing only. Build accepts schema v1 and bounded
	// required bytes, not arbitrary uint32 schema values as supported data.
	p := &replica.PullResponse{Disposition: replica.PullDisposition(7), RequiredBytes: math.MaxUint32, ObservedHead: &replica.Position{Epoch: 128, Seq: math.MaxInt64, Hash: make([]byte, 32)}, Events: []*replica.Event{{SchemaVersion: math.MaxUint32, CommittedUnixMs: math.MaxInt64}}}
	got, err := pullbudget.Size(pullbudget.Response{Disposition: 7, RequiredBytes: math.MaxUint32, ObservedHead: &pullbudget.Position{Epoch: 128, Seq: math.MaxInt64, Hash: make([]byte, 32)}, Events: []pullbudget.Event{{SchemaVersion: math.MaxUint32, CommittedUnixMS: math.MaxInt64}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(p)
	if err != nil || got != len(raw) || got != proto.Size(p) {
		t.Fatalf("size=%d marshaled=%d proto=%d err=%v", got, len(raw), proto.Size(p), err)
	}
}
