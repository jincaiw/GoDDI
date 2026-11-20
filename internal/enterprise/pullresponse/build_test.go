package pullresponse

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/pullbudget"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
	"google.golang.org/protobuf/proto"
)

func fixture(t *testing.T) (*sql.DB, *eventlog.Log, eventlog.ID, eventlog.ID) {
	t.Helper()
	db := sqlitepolicy.OpenWriter(filepath.Join(t.TempDir(), "db.sqlite"))
	db.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = db.Close() })
	if err := eventlog.Init(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var cluster, leader eventlog.ID
	cluster[15] = 1
	leader[15] = 2
	return db, eventlog.NewLog(db, cluster, leader), cluster, leader
}
func seed(t *testing.T, c *sql.Conn, cl, leader eventlog.ID, seq uint64, prev [32]byte, payloadSize int) eventlog.Event {
	t.Helper()
	base, err := eventlog.MarshalChanges(eventlog.Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: []byte(`{"family":4}`)})
	if err != nil {
		t.Fatal(err)
	}
	if payloadSize < len(base) {
		payloadSize = len(base)
	}
	base = append(base, bytes.Repeat([]byte(" "), payloadSize-len(base))...)
	e := eventlog.Event{ClusterID: cl, LeaderID: leader, Position: eventlog.Position{Epoch: 1, Seq: seq}, Schema: 1, CommitMs: int64(seq), PrevHash: prev, Payload: base}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ExecContext(context.Background(), `INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, cl[:], 1, seq, leader[:], 1, seq, prev[:], e.Position.Hash[:], base); err != nil {
		t.Fatal(err)
	}
	if _, err = c.ExecContext(context.Background(), `INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,?,?,1,?) ON CONFLICT(cluster_id) DO UPDATE SET seq=excluded.seq,hash=excluded.hash,advanced_at_unix_ms=excluded.advanced_at_unix_ms`, cl[:], seq, e.Position.Hash[:], seq); err != nil {
		t.Fatal(err)
	}
	return e
}
func get(t *testing.T, l *eventlog.Log, cl, leader eventlog.ID, after eventlog.Position, max int) Input {
	t.Helper()
	events, head, err := l.ReadSnapshot(context.Background(), after, max)
	if err != nil {
		t.Fatal(err)
	}
	return Input{Cluster: cl, Leader: leader, After: after, Head: head, Events: events, ReadableSchemas: []uint32{1}}
}
func unmarshal(t *testing.T, b []byte) *replica.PullResponse {
	t.Helper()
	var msg replica.PullResponse
	if err := proto.Unmarshal(b, &msg); err != nil {
		t.Fatal(err)
	}
	if len(b) != proto.Size(&msg) {
		t.Fatal("size mismatch")
	}
	return &msg
}
func TestReadSnapshotBuildTwoPhysicalConnections(t *testing.T) {
	db, l, cl, leader := fixture(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	first := seed(t, c, cl, leader, 1, eventlog.GenesisHash(), 300)
	in := get(t, l, cl, leader, eventlog.Position{Epoch: 1}, 256)
	if db.Stats().OpenConnections < 2 {
		t.Fatal("not two physical connections")
	}
	_ = seed(t, c, cl, leader, 2, first.Position.Hash, 300)
	b, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	out := unmarshal(t, b)
	if int32(out.Disposition) != pullbudget.Batch || len(out.Events) != 1 || out.ObservedHead.Seq != 1 || !bytes.Equal(out.Events[0].Payload, first.Payload) {
		t.Fatalf("response %v", out)
	}
	if len(out.ProtoReflect().GetUnknown()) != 0 || len(out.ObservedHead.ProtoReflect().GetUnknown()) != 0 || len(out.Events[0].ProtoReflect().GetUnknown()) != 0 || len(out.Events[0].Position.ProtoReflect().GetUnknown()) != 0 {
		t.Fatal("unknown protobuf fields")
	}
	in.Events[0].Payload[0] ^= 1
	again := unmarshal(t, b)
	if !bytes.Equal(again.Events[0].Payload, first.Payload) {
		t.Fatal("output aliases caller")
	}
	b[0] ^= 1
	fresh := get(t, l, cl, leader, eventlog.Position{Epoch: 1}, 1)
	if _, err = Build(fresh); err != nil {
		t.Fatal(err)
	}
}
func TestBudgetsAndHead(t *testing.T) {
	db, l, cl, leader := fixture(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	first := seed(t, c, cl, leader, 1, eventlog.GenesisHash(), 1400)
	second := seed(t, c, cl, leader, 2, first.Position.Hash, 100)
	in := get(t, l, cl, leader, eventlog.Position{Epoch: 1}, 256)
	in.MaxBytes = 1024
	b, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	out := unmarshal(t, b)
	if int32(out.Disposition) != pullbudget.BudgetTooSmall || len(out.Events) != 0 || out.RequiredBytes <= 1024 || len(b) > 1024 {
		t.Fatalf("small: %v", out)
	}
	in.MaxBytes = out.RequiredBytes
	b, err = Build(in)
	if err != nil {
		t.Fatal(err)
	}
	out = unmarshal(t, b)
	if len(out.Events) != 1 || uint32(len(b)) != in.MaxBytes {
		t.Fatalf("boundary %d %v", len(b), out)
	}
	in.MaxBytes = 0
	b, err = Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := unmarshal(t, b); len(got.Events) != 2 {
		t.Fatal(got)
	} else if !bytes.Equal(got.Events[1].Position.Hash, second.Position.Hash[:]) || !bytes.Equal(got.Events[1].PrevHash, second.PrevHash[:]) || !bytes.Equal(got.Events[1].Payload, second.Payload) {
		t.Fatal("second event did not round-trip exactly")
	}
	head := get(t, l, cl, leader, second.Position, 256)
	b, err = Build(head)
	if err != nil || int32(unmarshal(t, b).Disposition) != pullbudget.AtHead {
		t.Fatalf("at head %v", err)
	}
}
func TestAtHeadRequiresReadableV1(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated=%v", populated), func(t *testing.T) {
			db, l, cl, leader := fixture(t)
			c, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			head := eventlog.Position{Epoch: 1}
			if populated {
				head = seed(t, c, cl, leader, 1, eventlog.GenesisHash(), 120).Position
			} else {
				genesis := eventlog.GenesisHash()
				if _, err := c.ExecContext(context.Background(), `INSERT INTO replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,0)`, cl[:], genesis[:]); err != nil {
					t.Fatal(err)
				}
			}
			in := get(t, l, cl, leader, head, 1)
			in.ReadableSchemas = []uint32{2}
			if out, err := Build(in); !errors.Is(err, ErrSchema) || out != nil {
				t.Fatalf("unsupported: output=%v err=%v", out, err)
			}
			in.ReadableSchemas = []uint32{2, 1}
			out, err := Build(in)
			if err != nil || int32(unmarshal(t, out).Disposition) != pullbudget.AtHead {
				t.Fatalf("supported: err=%v", err)
			}
		})
	}
}

func TestRejectInvalidCandidates(t *testing.T) {
	db, l, cl, leader := fixture(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	seed(t, c, cl, leader, 1, eventlog.GenesisHash(), 300)
	for _, tc := range []struct {
		name   string
		change func(*Input)
	}{
		{"cluster", func(i *Input) { i.Cluster[0] = 9 }}, {"leader", func(i *Input) { i.Leader[0] = 9 }},
		{"epoch", func(i *Input) { i.Head.Epoch++ }}, {"gap", func(i *Input) { i.Events[0].Position.Seq++ }},
		{"previous", func(i *Input) { i.Events[0].PrevHash[0]++ }}, {"hash", func(i *Input) { i.Events[0].Position.Hash[0]++ }},
		{"head hash", func(i *Input) { i.Head.Hash[0]++ }}, {"empty before head", func(i *Input) { i.Events = nil }},
		{"schema unsupported", func(i *Input) { i.ReadableSchemas = []uint32{2} }}, {"schema absent", func(i *Input) { i.ReadableSchemas = nil }},
		{"small cap", func(i *Input) { i.MaxBytes = 1023 }}, {"large cap", func(i *Input) { i.MaxBytes = pullbudget.MaxBytes + 1 }}, {"too many events", func(i *Input) { i.MaxEvents = 257 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := get(t, l, cl, leader, eventlog.Position{Epoch: 1}, 1)
			tc.change(&in)
			b, err := Build(in)
			if err == nil || len(b) != 0 {
				t.Fatalf("accepted %s: len %d err %v", tc.name, len(b), err)
			}
			if tc.name == "schema unsupported" && !errors.Is(err, ErrSchema) {
				t.Fatal(err)
			}
		})
	}
}
func TestVarintAndEightMiBBoundary(t *testing.T) {
	db, l, cl, leader := fixture(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var prev [32]byte
	for i := 1; i <= 130; i++ {
		e := seed(t, c, cl, leader, uint64(i), prev, 120)
		prev = e.Position.Hash
	}
	in := get(t, l, cl, leader, eventlog.Position{Epoch: 1}, 256)
	b, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	out := unmarshal(t, b)
	if len(out.Events) != 130 || out.ObservedHead.Seq != 130 {
		t.Fatalf("varint boundary %d", len(out.Events))
	}
	t.Run("max payload", func(t *testing.T) {
		db, l, cl, leader := fixture(t)
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		e := seed(t, c, cl, leader, 1, eventlog.GenesisHash(), eventlog.MaxPayloadBytes)
		seed(t, c, cl, leader, 2, e.Position.Hash, eventlog.MaxPayloadBytes)
		in := get(t, l, cl, leader, eventlog.Position{Epoch: 1}, 256)
		b, err := Build(in)
		if err != nil {
			t.Fatal(err)
		}
		out := unmarshal(t, b)
		if len(b) > pullbudget.MaxBytes || len(out.Events) != 1 {
			t.Fatalf("wire=%d events=%d", len(b), len(out.Events))
		}
	})
}
