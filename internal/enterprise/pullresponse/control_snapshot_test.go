package pullresponse

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/pullbudget"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"google.golang.org/protobuf/proto"
)

// validEventAt builds a locally verifiable v1 event at (epoch 1, seq). It is
// evidence supplied by the caller, not proof of an authoritative tail.
func validEventAt(t *testing.T, cluster, leader eventlog.ID, seq uint64) *eventlog.Event {
	t.Helper()
	payload, err := eventlog.MarshalChanges(eventlog.Change{
		Kind: "dhcp.lease.committed", ObjectID: "b01dd6c0-81a0-4a8f-b66c-5f28c1f20001",
		Revision: "1", Body: []byte(`{"family":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	e := &eventlog.Event{
		ClusterID: cluster, LeaderID: leader,
		Position: eventlog.Position{Epoch: 1, Seq: seq},
		Schema:   eventlog.SchemaVersionV1, CommitMs: 10,
		PrevHash: eventlog.GenesisHash(), Payload: payload,
	}
	e.Position.Hash, err = e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func snapshotFixture(t *testing.T) ControlInput {
	t.Helper()
	cluster, leader := eventlog.ID{1}, eventlog.ID{2}
	earliest := validEventAt(t, cluster, leader, 5)
	return ControlInput{
		Cluster: cluster, Leader: leader,
		After:           eventlog.Position{Epoch: 1, Seq: 2, Hash: [32]byte{7}},
		Head:            eventlog.Position{Epoch: 1, Seq: 8, Hash: [32]byte{8}},
		ReadableSchemas: []uint32{1, 2},
		Earliest:        earliest,
	}
}

func divergedFixture(t *testing.T) ControlInput {
	t.Helper()
	cluster, leader := eventlog.ID{1}, eventlog.ID{2}
	conflict := validEventAt(t, cluster, leader, 3)
	// The follower claims a different digest at the same position.
	claimed := conflict.Position
	claimed.Hash[0] ^= 1
	return ControlInput{
		Cluster: cluster, Leader: leader,
		After:           claimed,
		Head:            eventlog.Position{Epoch: 1, Seq: 8, Hash: [32]byte{8}},
		ReadableSchemas: []uint32{1, 2},
		Conflict:        conflict,
	}
}

func decodeControl(t *testing.T, wire []byte) *replica.PullResponse {
	t.Helper()
	var r replica.PullResponse
	if err := proto.Unmarshal(wire, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

func TestSnapshotRequiredEncodesOfflineTelemetry(t *testing.T) {
	in := snapshotFixture(t)
	want := in.Head
	for _, budget := range []uint32{0, pullbudget.MinBytes, pullbudget.MaxBytes} {
		in.MaxBytes = budget
		wire, err := BuildControl(in, replica.PullDisposition_SNAPSHOT_REQUIRED)
		if err != nil {
			t.Fatalf("budget %d: %v", budget, err)
		}
		r := decodeControl(t, wire)
		if r.Disposition != replica.PullDisposition_SNAPSHOT_REQUIRED || len(r.Events) != 0 ||
			r.RequiredBytes != 0 || r.ObservedHead == nil ||
			r.ObservedHead.Epoch != want.Epoch || r.ObservedHead.Seq != want.Seq ||
			!bytes.Equal(r.ObservedHead.Hash, want.Hash[:]) ||
			len(wire) != proto.Size(r) || (budget != 0 && len(wire) > int(budget)) {
			t.Fatalf("bad SNAPSHOT_REQUIRED response %+v", r)
		}
		// The encoder must not alias or mutate caller-owned evidence.
		in.Head.Hash[0] ^= 1
		if !bytes.Equal(r.ObservedHead.Hash, want.Hash[:]) {
			t.Fatal("response aliases input")
		}
		in.Head = want
	}
}

func TestSnapshotRequiredRejectsInvalidEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ControlInput)
	}{
		{"evidence absent", func(x *ControlInput) { x.Earliest = nil }},
		{"no gap: earliest is next event", func(x *ControlInput) { x.Earliest = validEventAtForGap(t, x, 3) }},
		{"earliest at follower position", func(x *ControlInput) { x.Earliest = validEventAtForGap(t, x, 2) }},
		{"earliest beyond head", func(x *ControlInput) { x.Earliest = validEventAtForGap(t, x, 9) }},
		{"different epoch", func(x *ControlInput) { x.Earliest.Position.Epoch = 2 }},
		{"wrong cluster", func(x *ControlInput) { x.Earliest.ClusterID[0]++ }},
		{"wrong leader", func(x *ControlInput) { x.Earliest.LeaderID[0]++ }},
		{"wrong schema", func(x *ControlInput) { x.Earliest.Schema = 2 }},
		{"bad digest", func(x *ControlInput) { x.Earliest.Position.Hash[0] ^= 1 }},
		{"empty payload", func(x *ControlInput) { x.Earliest.Payload = nil }},
		{"negative commit", func(x *ControlInput) { x.Earliest.CommitMs = -1 }},
		{"next event also supplied", func(x *ControlInput) { x.Next = validEventAtForGap(t, x, 3) }},
		{"conflict also supplied", func(x *ControlInput) { x.Conflict = validEventAtForGap(t, x, 3) }},
		{"cross epoch request", func(x *ControlInput) { x.Head.Epoch = 2 }},
		{"follower already at head", func(x *ControlInput) { x.After = x.Head }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := snapshotFixture(t)
			tc.mutate(&in)
			wire, err := BuildControl(in, replica.PullDisposition_SNAPSHOT_REQUIRED)
			if !errors.Is(err, ErrInvalid) || wire != nil {
				t.Fatalf("wire=%v err=%v", wire, err)
			}
		})
	}
}

func TestDivergedEncodesOfflineTelemetry(t *testing.T) {
	in := divergedFixture(t)
	wire, err := BuildControl(in, replica.PullDisposition_DIVERGED)
	if err != nil {
		t.Fatal(err)
	}
	r := decodeControl(t, wire)
	if r.Disposition != replica.PullDisposition_DIVERGED || len(r.Events) != 0 ||
		r.RequiredBytes != 0 || r.ObservedHead == nil || r.ObservedHead.Seq != 8 {
		t.Fatalf("bad DIVERGED response %+v", r)
	}
}

func TestDivergedRejectsInvalidEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ControlInput)
	}{
		{"evidence absent", func(x *ControlInput) { x.Conflict = nil }},
		{"hashes agree: no fork", func(x *ControlInput) { x.After.Hash = x.Conflict.Position.Hash }},
		{"conflict at different seq", func(x *ControlInput) {
			x.Conflict.Position.Seq = 4
			x.Conflict.Position.Hash = mustRehash(t, x.Conflict)
		}},
		{"conflict at different epoch", func(x *ControlInput) {
			x.Conflict.Position.Epoch = 2
			x.Conflict.Position.Hash = mustRehash(t, x.Conflict)
		}},
		{"wrong cluster", func(x *ControlInput) { x.Conflict.ClusterID[0]++ }},
		{"wrong leader", func(x *ControlInput) { x.Conflict.LeaderID[0]++ }},
		{"wrong schema", func(x *ControlInput) { x.Conflict.Schema = 2 }},
		{"bad digest", func(x *ControlInput) { x.Conflict.Position.Hash[0] ^= 1 }},
		{"empty payload", func(x *ControlInput) { x.Conflict.Payload = nil }},
		{"anchor position", func(x *ControlInput) { x.After.Seq = 0; x.After.Hash = eventlog.GenesisHash() }},
		{"after beyond head", func(x *ControlInput) { x.After.Seq = 9 }},
		{"next event also supplied", func(x *ControlInput) { x.Next = validEventAtForGap(t, x, 4) }},
		{"earliest also supplied", func(x *ControlInput) { x.Earliest = validEventAtForGap(t, x, 5) }},
		{"cross epoch request", func(x *ControlInput) { x.Head.Epoch = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := divergedFixture(t)
			tc.mutate(&in)
			wire, err := BuildControl(in, replica.PullDisposition_DIVERGED)
			if !errors.Is(err, ErrInvalid) || wire != nil {
				t.Fatalf("wire=%v err=%v", wire, err)
			}
		})
	}
}

func TestEvidenceIsNeverInferredFromADatabaseError(t *testing.T) {
	// A plain control input with no evidence at all must be rejected for both
	// new dispositions. The encoder has no error parameter: a caller that hit
	// a storage error simply cannot construct a valid candidate.
	bare := ControlInput{
		Cluster: eventlog.ID{1}, Leader: eventlog.ID{2},
		After:           eventlog.Position{Epoch: 1, Seq: 2, Hash: [32]byte{7}},
		Head:            eventlog.Position{Epoch: 1, Seq: 8, Hash: [32]byte{8}},
		ReadableSchemas: []uint32{1},
	}
	for _, kind := range []replica.PullDisposition{
		replica.PullDisposition_SNAPSHOT_REQUIRED, replica.PullDisposition_DIVERGED,
	} {
		if out, err := BuildControl(bare, kind); out != nil || !errors.Is(err, ErrInvalid) {
			t.Fatalf("kind=%v: wire=%v err=%v", kind, out, err)
		}
	}
}

func TestNewControlCandidatesDoNotMutateInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   ControlInput
		kind replica.PullDisposition
	}{
		{"snapshot", snapshotFixture(t), replica.PullDisposition_SNAPSHOT_REQUIRED},
		{"diverged", divergedFixture(t), replica.PullDisposition_DIVERGED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			evidence := in.Earliest
			if evidence == nil {
				evidence = in.Conflict
			}
			before := *evidence
			before.Payload = bytes.Clone(evidence.Payload)
			after, head := in.After, in.Head
			if _, err := BuildControl(in, tc.kind); err != nil {
				t.Fatal(err)
			}
			if in.After != after || in.Head != head ||
				evidence.ClusterID != before.ClusterID || evidence.LeaderID != before.LeaderID ||
				evidence.Position != before.Position || evidence.Schema != before.Schema ||
				evidence.CommitMs != before.CommitMs || evidence.PrevHash != before.PrevHash ||
				!bytes.Equal(evidence.Payload, before.Payload) {
				t.Fatal("input evidence was mutated")
			}
		})
	}
}

// Helpers shared by the table-driven cases.

func validEventAtForGap(t *testing.T, in *ControlInput, seq uint64) *eventlog.Event {
	t.Helper()
	e := validEventAt(t, in.Cluster, in.Leader, seq)
	return e
}

func mustRehash(t *testing.T, e *eventlog.Event) [32]byte {
	t.Helper()
	h, err := e.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return h
}
