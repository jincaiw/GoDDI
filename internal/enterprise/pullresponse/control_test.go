package pullresponse

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/pullbudget"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"google.golang.org/protobuf/proto"
)

func schemaControlFixture(t *testing.T) ControlInput {
	t.Helper()
	payload, err := eventlog.MarshalChanges(eventlog.Change{Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001", Revision: "1", Body: []byte(`{"family":4}`)})
	if err != nil {
		t.Fatal(err)
	}
	cluster, leader := eventlog.ID{1}, eventlog.ID{2}
	e := eventlog.Event{ClusterID: cluster, LeaderID: leader, Position: eventlog.Position{Epoch: 1, Seq: 1}, Schema: eventlog.SchemaVersionV1, CommitMs: 10, PrevHash: eventlog.GenesisHash(), Payload: payload}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return ControlInput{Cluster: cluster, Leader: leader, After: eventlog.Position{Epoch: 1}, Head: e.Position, ReadableSchemas: []uint32{2}, Next: &e}
}
func epochControlFixture() ControlInput {
	h := [32]byte{9}
	return ControlInput{Cluster: eventlog.ID{1}, Leader: eventlog.ID{2}, After: eventlog.Position{Epoch: 3, Seq: 0, Hash: h}, Head: eventlog.Position{Epoch: 2, Seq: 1, Hash: h}, ReadableSchemas: []uint32{1, 2}}
}
func TestBuildControlEncodesOnlyOfflineTelemetry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input func() ControlInput
		kind  replica.PullDisposition
	}{{"schema", func() ControlInput { return schemaControlFixture(t) }, replica.PullDisposition_SCHEMA_INCOMPATIBLE}, {"epoch", epochControlFixture, replica.PullDisposition_EPOCH_REBASE_REQUIRED}} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.input()
			want := in.Head
			for _, budget := range []uint32{0, pullbudget.MinBytes, pullbudget.MaxBytes} {
				in.MaxBytes = budget
				wire, err := BuildControl(in, tc.kind)
				if err != nil {
					t.Fatal(err)
				}
				var r replica.PullResponse
				if err = proto.Unmarshal(wire, &r); err != nil {
					t.Fatal(err)
				}
				if r.Disposition != tc.kind || len(r.Events) != 0 || r.RequiredBytes != 0 || r.ObservedHead == nil || r.ObservedHead.Epoch != want.Epoch || r.ObservedHead.Seq != want.Seq || !bytes.Equal(r.ObservedHead.Hash, want.Hash[:]) || len(wire) != proto.Size(&r) || budget != 0 && len(wire) > int(budget) {
					t.Fatalf("bad control response %+v", &r)
				}
				in.Head.Hash[0] ^= 1
				if !bytes.Equal(r.ObservedHead.Hash, want.Hash[:]) {
					t.Fatal("response aliases input")
				}
				in.Head = want
			}
		})
	}
}
func TestControlRejectsCommonInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ControlInput)
	}{
		{"zero cluster", func(x *ControlInput) { x.Cluster = eventlog.ID{} }}, {"zero leader", func(x *ControlInput) { x.Leader = eventlog.ID{} }}, {"same identities", func(x *ControlInput) { x.Leader = x.Cluster }}, {"nil schemas", func(x *ControlInput) { x.ReadableSchemas = nil }}, {"zero schema", func(x *ControlInput) { x.ReadableSchemas = []uint32{0} }}, {"duplicate schema", func(x *ControlInput) { x.ReadableSchemas = []uint32{2, 2} }}, {"too many schemas", func(x *ControlInput) { x.ReadableSchemas = make([]uint32, 257) }}, {"too many events", func(x *ControlInput) { x.MaxEvents = pullbudget.MaxEvents + 1 }}, {"budget too small", func(x *ControlInput) { x.MaxBytes = pullbudget.MinBytes - 1 }}, {"budget too large", func(x *ControlInput) { x.MaxBytes = pullbudget.MaxBytes + 1 }}, {"after epoch zero", func(x *ControlInput) { x.After.Epoch = 0 }}, {"after epoch overflow", func(x *ControlInput) { x.After.Epoch = math.MaxUint64 }}, {"after seq overflow", func(x *ControlInput) { x.After.Seq = math.MaxUint64 }}, {"head epoch zero", func(x *ControlInput) { x.Head.Epoch = 0 }}, {"head epoch overflow", func(x *ControlInput) { x.Head.Epoch = math.MaxUint64 }}, {"head seq overflow", func(x *ControlInput) { x.Head.Seq = math.MaxUint64 }}, {"bad genesis request", func(x *ControlInput) { x.After = eventlog.Position{Epoch: 1, Hash: [32]byte{1}} }}, {"unverified later empty head", func(x *ControlInput) { x.Head = eventlog.Position{Epoch: 2} }}, {"bad genesis head", func(x *ControlInput) { x.Head = eventlog.Position{Epoch: 1, Hash: [32]byte{1}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := epochControlFixture()
			tc.mutate(&in)
			wire, err := BuildControl(in, replica.PullDisposition_EPOCH_REBASE_REQUIRED)
			if !errors.Is(err, ErrInvalid) || wire != nil {
				t.Fatalf("wire=%v err=%v", wire, err)
			}
		})
	}
}
func TestControlEpochRequiresDifferentEpoch(t *testing.T) {
	in := epochControlFixture()
	in.Head.Epoch = in.After.Epoch
	if out, err := BuildControl(in, replica.PullDisposition_EPOCH_REBASE_REQUIRED); !errors.Is(err, ErrInvalid) || out != nil {
		t.Fatal("same epoch accepted")
	}
	in = epochControlFixture()
	in.Next = &eventlog.Event{}
	if out, err := BuildControl(in, replica.PullDisposition_EPOCH_REBASE_REQUIRED); !errors.Is(err, ErrInvalid) || out != nil {
		t.Fatal("unexpected event accepted")
	}
	in = epochControlFixture()
	in.ReadableSchemas = []uint32{2}
	if _, err := BuildControl(in, replica.PullDisposition_EPOCH_REBASE_REQUIRED); err != nil {
		t.Fatalf("epoch priority with unsupported schema: %v", err)
	}
	in = epochControlFixture()
	in.Head = eventlog.Position{Epoch: 1}
	if _, err := BuildControl(in, replica.PullDisposition_EPOCH_REBASE_REQUIRED); err != nil {
		t.Fatalf("genesis head: %v", err)
	}
}
func TestSchemaControlRejectsInvalidNext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ControlInput)
	}{
		{"next absent", func(x *ControlInput) { x.Next = nil }}, {"client supports v1", func(x *ControlInput) { x.ReadableSchemas = []uint32{1, 2} }}, {"at head", func(x *ControlInput) { x.After = x.Head }}, {"epoch differs", func(x *ControlInput) { x.Head.Epoch++ }}, {"after beyond head", func(x *ControlInput) { x.After.Seq = 2 }}, {"later epoch zero anchor", func(x *ControlInput) { x.After.Epoch = 2; x.After.Seq = 0; x.Head.Epoch = 2; x.Next.Position.Epoch = 2 }}, {"wrong cluster", func(x *ControlInput) { x.Next.ClusterID[0]++ }}, {"wrong leader", func(x *ControlInput) { x.Next.LeaderID[0]++ }}, {"wrong event schema", func(x *ControlInput) { x.Next.Schema = 2 }}, {"wrong epoch", func(x *ControlInput) { x.Next.Position.Epoch++ }}, {"sequence gap", func(x *ControlInput) { x.Next.Position.Seq++ }}, {"wrong previous hash", func(x *ControlInput) { x.Next.PrevHash[0]++ }}, {"next differs from head", func(x *ControlInput) { x.Head.Hash[0]++ }}, {"negative commit", func(x *ControlInput) { x.Next.CommitMs = -1 }}, {"nil payload", func(x *ControlInput) { x.Next.Payload = nil }}, {"oversize payload", func(x *ControlInput) { x.Next.Payload = make([]byte, eventlog.MaxPayloadBytes+1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := schemaControlFixture(t)
			tc.mutate(&in)
			wire, err := BuildControl(in, replica.PullDisposition_SCHEMA_INCOMPATIBLE)
			if !errors.Is(err, ErrInvalid) || wire != nil {
				t.Fatalf("wire=%v err=%v", wire, err)
			}
		})
	}
}
func TestSchemaControlDoesNotChangeInput(t *testing.T) {
	in := schemaControlFixture(t)
	before := *in.Next
	before.Payload = bytes.Clone(in.Next.Payload)
	schemas := append([]uint32(nil), in.ReadableSchemas...)
	after, head := in.After, in.Head
	if _, err := BuildControl(in, replica.PullDisposition_SCHEMA_INCOMPATIBLE); err != nil {
		t.Fatal(err)
	}
	if in.After != after || in.Head != head || in.Next.ClusterID != before.ClusterID || in.Next.LeaderID != before.LeaderID || in.Next.Position != before.Position || in.Next.Schema != before.Schema || in.Next.CommitMs != before.CommitMs || in.Next.PrevHash != before.PrevHash || !bytes.Equal(in.Next.Payload, before.Payload) || len(schemas) != len(in.ReadableSchemas) || schemas[0] != in.ReadableSchemas[0] {
		t.Fatal("input changed")
	}
}

func TestSchemaControlRetainsDigestError(t *testing.T) {
	in := schemaControlFixture(t)
	in.Next.Position.Hash[0] ^= 1
	in.Head = in.Next.Position
	wire, err := BuildControl(in, replica.PullDisposition_SCHEMA_INCOMPATIBLE)
	if wire != nil || !errors.Is(err, ErrInvalid) || !errors.Is(err, eventlog.ErrDigestMismatch) {
		t.Fatalf("lost digest error %v", err)
	}
}
func TestSchemaControlAllowsPartialPrefixTelemetry(t *testing.T) {
	in := schemaControlFixture(t)
	in.Head.Seq = 2
	in.Head.Hash = [32]byte{99}
	wire, err := BuildControl(in, replica.PullDisposition_SCHEMA_INCOMPATIBLE)
	if err != nil {
		t.Fatal(err)
	}
	var r replica.PullResponse
	if err = proto.Unmarshal(wire, &r); err != nil || r.ObservedHead.Seq != 2 {
		t.Fatalf("head %v %v", r.ObservedHead, err)
	}
}
func TestControlRejectsOtherDispositions(t *testing.T) {
	// SNAPSHOT_REQUIRED and DIVERGED are now supported, but only with their
	// own explicit evidence; they are covered in control_snapshot_test.go.
	for _, kind := range []replica.PullDisposition{replica.PullDisposition_PULL_DISPOSITION_UNSPECIFIED, replica.PullDisposition_BATCH, replica.PullDisposition_AT_HEAD, replica.PullDisposition_REQUEST_BUDGET_TOO_SMALL, replica.PullDisposition(99)} {
		if out, err := BuildControl(epochControlFixture(), kind); out != nil || !errors.Is(err, ErrInvalid) {
			t.Fatalf("kind=%v err=%v", kind, err)
		}
	}
}
