package pullresponse

import (
	"bytes"
	"fmt"
	"math"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/pullbudget"
	replica "github.com/jasonwa/goddi/internal/gen/replica/v1"
	"google.golang.org/protobuf/proto"
)

// ControlInput holds caller-supplied, unauthenticated observations. No field
// proves source identity, role, an authoritative head, or that an event was
// read from storage. The caller must independently validate the anchor and
// obtain Next/Earliest/Conflict and Head from the same appropriate view.
// The caller must not mutate Next, Earliest or Conflict or ReadableSchemas
// concurrently with BuildControl.
type ControlInput struct {
	Cluster, Leader     eventlog.ID
	After, Head         eventlog.Position
	MaxEvents, MaxBytes uint32
	ReadableSchemas     []uint32
	// Next is the actual event following After (SCHEMA_INCOMPATIBLE).
	Next *eventlog.Event
	// Earliest is the oldest event still retained by the leader's log
	// (SNAPSHOT_REQUIRED). It proves a gap that a batch cannot bridge.
	Earliest *eventlog.Event
	// Conflict is the leader's event at After's position (DIVERGED). Its
	// digest differs from After.Hash, which is the fork being reported.
	Conflict *eventlog.Event
}

// BuildControl only serializes offline control candidates
// (EPOCH_REBASE_REQUIRED, SCHEMA_INCOMPATIBLE, SNAPSHOT_REQUIRED, DIVERGED).
// It does not infer any state from an SQL error, authenticate any input,
// authorize a role, or prove history. Every disposition requires the caller to
// supply explicit, locally verifiable evidence; a storage error, a timeout or
// a cancellation is never a reason to classify a chain as diverged.
func BuildControl(in ControlInput, disposition replica.PullDisposition) ([]byte, error) {
	if in.Cluster == (eventlog.ID{}) || in.Leader == (eventlog.ID{}) || in.Cluster == in.Leader || len(in.ReadableSchemas) == 0 || len(in.ReadableSchemas) > 256 {
		return nil, ErrInvalid
	}
	maxEvents, maxBytes := in.MaxEvents, in.MaxBytes
	if maxEvents == 0 {
		maxEvents = pullbudget.MaxEvents
	}
	if maxBytes == 0 {
		maxBytes = pullbudget.MaxBytes
	}
	if maxEvents > pullbudget.MaxEvents || maxBytes < pullbudget.MinBytes || maxBytes > pullbudget.MaxBytes {
		return nil, ErrInvalid
	}
	schemas := make(map[uint32]bool, len(in.ReadableSchemas))
	for _, v := range in.ReadableSchemas {
		if v == 0 || schemas[v] {
			return nil, ErrInvalid
		}
		schemas[v] = true
	}
	if in.After.Epoch == 0 || in.After.Epoch > math.MaxInt64 || in.After.Seq > math.MaxInt64 || in.Head.Epoch == 0 || in.Head.Epoch > math.MaxInt64 || in.Head.Seq > math.MaxInt64 {
		return nil, ErrInvalid
	}
	if in.After.Seq == 0 && in.After.Epoch == 1 && in.After.Hash != eventlog.GenesisHash() {
		return nil, ErrInvalid
	}
	if in.Head.Seq == 0 && (in.Head.Epoch != 1 || in.Head.Hash != eventlog.GenesisHash()) {
		return nil, ErrInvalid
	}
	switch disposition {
	case replica.PullDisposition_EPOCH_REBASE_REQUIRED:
		if in.After.Epoch == in.Head.Epoch || in.Next != nil {
			return nil, ErrInvalid
		}
	case replica.PullDisposition_SNAPSHOT_REQUIRED:
		// The follower sits behind the leader's earliest retained event, so
		// no batch can bridge the gap and a snapshot is required. The caller
		// must supply that earliest event from the same trusted read view.
		if in.After.Epoch != in.Head.Epoch || in.After.Seq >= in.Head.Seq ||
			in.Next != nil || in.Conflict != nil || in.Earliest == nil {
			return nil, ErrInvalid
		}
		if err := verifyEvidenceEvent(in.Earliest, in.Cluster, in.Leader); err != nil {
			return nil, fmt.Errorf("%w: earliest event: %w", ErrInvalid, err)
		}
		e := in.Earliest
		if e.Position.Epoch != in.After.Epoch || e.Position.Seq <= in.After.Seq+1 ||
			e.Position.Seq > in.Head.Seq {
			return nil, ErrInvalid
		}
	case replica.PullDisposition_DIVERGED:
		// The follower's hash at (epoch, seq) disagrees with the leader's
		// event at that exact position. The caller must supply that event;
		// a missing read or a query error is never evidence of a fork.
		if in.After.Epoch != in.Head.Epoch || in.After.Seq == 0 ||
			in.After.Seq > in.Head.Seq || in.Next != nil ||
			in.Earliest != nil || in.Conflict == nil {
			return nil, ErrInvalid
		}
		if err := verifyEvidenceEvent(in.Conflict, in.Cluster, in.Leader); err != nil {
			return nil, fmt.Errorf("%w: conflicting event: %w", ErrInvalid, err)
		}
		e := in.Conflict
		if e.Position.Epoch != in.After.Epoch || e.Position.Seq != in.After.Seq ||
			e.Position.Hash == in.After.Hash {
			return nil, ErrInvalid
		}
	case replica.PullDisposition_SCHEMA_INCOMPATIBLE:
		if in.After.Epoch != in.Head.Epoch || in.After.Seq >= in.Head.Seq || in.After.Seq == 0 && in.After.Epoch != 1 || in.Next == nil || schemas[eventlog.SchemaVersionV1] {
			return nil, ErrInvalid
		}
		e := *in.Next
		if e.Schema != eventlog.SchemaVersionV1 || e.ClusterID != in.Cluster || e.LeaderID != in.Leader || e.Position.Epoch != in.After.Epoch || e.Position.Seq != in.After.Seq+1 || e.Position.Seq > in.Head.Seq || e.PrevHash != in.After.Hash || e.CommitMs < 0 || len(e.Payload) == 0 || len(e.Payload) > eventlog.MaxPayloadBytes {
			return nil, ErrInvalid
		}
		if e.Position.Seq == in.Head.Seq && e.Position != in.Head {
			return nil, ErrInvalid
		}
		e.Payload = bytes.Clone(e.Payload)
		if err := e.Verify(); err != nil {
			return nil, fmt.Errorf("%w: next event: %w", ErrInvalid, err)
		}
	default:
		return nil, ErrInvalid
	}
	out := &replica.PullResponse{Disposition: disposition, ObservedHead: position(in.Head)}
	expected, err := pullbudget.Size(pullbudget.Response{Disposition: int32(disposition), ObservedHead: &pullbudget.Position{Epoch: in.Head.Epoch, Seq: in.Head.Seq, Hash: in.Head.Hash[:]}})
	if err != nil {
		return nil, fmt.Errorf("%w: size: %w", ErrInvalid, err)
	}
	size := proto.Size(out)
	if size != expected || size > int(maxBytes) {
		return nil, ErrInvalid
	}
	wire, err := proto.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %w", ErrInvalid, err)
	}
	if len(wire) != size || len(wire) > int(maxBytes) {
		return nil, ErrInvalid
	}
	return wire, nil
}

// verifyEvidenceEvent checks that e is a locally well-formed v1 event from the
// expected cluster and leader. It proves nothing about where the event came
// from, whether the head is authoritative, or which role may act on it: those
// remain entirely the caller's responsibility.
func verifyEvidenceEvent(e *eventlog.Event, cluster, leader eventlog.ID) error {
	if e == nil {
		return ErrInvalid
	}
	if e.ClusterID != cluster || e.LeaderID != leader || e.Schema != eventlog.SchemaVersionV1 {
		return ErrInvalid
	}
	if e.CommitMs < 0 || len(e.Payload) == 0 || len(e.Payload) > eventlog.MaxPayloadBytes {
		return ErrInvalid
	}
	cp := *e
	cp.Payload = bytes.Clone(e.Payload)
	return cp.Verify()
}
