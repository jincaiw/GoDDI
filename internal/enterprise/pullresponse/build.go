// Package pullresponse constructs offline candidate protobuf responses. It does
// not read a database, authenticate a peer, authorize replication, send packets,
// or prove that the supplied events and head came from the same read snapshot.
package pullresponse

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/pullbudget"
	replica "github.com/jincaiw/GoDDI/internal/gen/replica/v1"
	"google.golang.org/protobuf/proto"
)

var ErrInvalid = errors.New("pullresponse: invalid offline candidate")
var ErrSchema = errors.New("pullresponse: unsupported event schema")

// Input is caller-supplied, untrusted data, never a permission or evidence of
// authority. Head and Events must originate from the same validated read view.
// Slices must not be mutated concurrently with Build.
type Input struct {
	Cluster, Leader     eventlog.ID
	After, Head         eventlog.Position
	Events              []eventlog.Event
	MaxEvents, MaxBytes uint32
	ReadableSchemas     []uint32
}

// Build returns independent serialized bytes. Successful output is only an
// offline encoding; it must not be used to infer authorization or valid head.
func Build(in Input) ([]byte, error) {
	if in.Cluster == (eventlog.ID{}) || in.Leader == (eventlog.ID{}) || in.Cluster == in.Leader || len(in.Events) > pullbudget.MaxEvents || len(in.ReadableSchemas) == 0 || len(in.ReadableSchemas) > 256 {
		return nil, ErrInvalid
	}
	maxEvents, maxBytes := in.MaxEvents, in.MaxBytes
	if maxEvents == 0 {
		maxEvents = pullbudget.MaxEvents
	}
	if maxBytes == 0 {
		maxBytes = pullbudget.MaxBytes
	}
	if maxEvents > pullbudget.MaxEvents || maxBytes < pullbudget.MinBytes || maxBytes > pullbudget.MaxBytes || len(in.Events) > int(maxEvents) {
		return nil, ErrInvalid
	}
	if in.After.Epoch == 0 || in.After.Epoch > math.MaxInt64 || in.After.Seq > math.MaxInt64 || in.Head.Epoch != in.After.Epoch || in.Head.Seq > math.MaxInt64 || in.After.Seq > in.Head.Seq {
		return nil, ErrInvalid
	}
	if in.After.Seq == 0 && (in.After.Epoch != 1 || in.After.Hash != eventlog.GenesisHash()) {
		return nil, ErrInvalid
	}
	if in.Head.Seq == 0 && (in.Head.Epoch != 1 || in.Head.Hash != eventlog.GenesisHash()) {
		return nil, ErrInvalid
	}
	schemas := map[uint32]bool{}
	for _, v := range in.ReadableSchemas {
		if v == 0 || schemas[v] {
			return nil, ErrInvalid
		}
		schemas[v] = true
	}
	// ReadSnapshot currently accepts only v1 heads, including genesis. A later
	// protocol must negotiate against the actual observed head schema instead.
	if !schemas[eventlog.SchemaVersionV1] {
		return nil, ErrSchema
	}
	events := make([]eventlog.Event, len(in.Events))
	total := 0
	for i, e := range in.Events {
		if len(e.Payload) == 0 || len(e.Payload) > eventlog.MaxPayloadBytes || len(e.Payload) > eventlog.MaxBatchPayloadBytes-total {
			return nil, ErrInvalid
		}
		total += len(e.Payload)
		events[i] = e
		events[i].Payload = bytes.Clone(e.Payload)
	}
	if len(events) == 0 {
		if in.After != in.Head {
			return nil, ErrInvalid
		}
	} else if in.After == in.Head {
		return nil, ErrInvalid
	}
	candidates := make([]pullbudget.Event, len(events))
	prev := in.After
	for i, e := range events {
		if prev.Seq == math.MaxInt64 || e.ClusterID != in.Cluster || e.LeaderID != in.Leader || e.Position.Epoch != in.After.Epoch || e.Position.Seq != prev.Seq+1 || e.Position.Seq > in.Head.Seq || e.PrevHash != prev.Hash || e.CommitMs < 0 {
			return nil, ErrInvalid
		}
		if !schemas[e.Schema] || e.Schema != eventlog.SchemaVersionV1 {
			return nil, ErrSchema
		}
		if err := e.Verify(); err != nil {
			return nil, fmt.Errorf("%w: event %d: %v", ErrInvalid, i, err)
		}
		if e.Position.Seq == in.Head.Seq && e.Position != in.Head {
			return nil, ErrInvalid
		}
		candidates[i] = pullbudget.Event{ClusterID: e.ClusterID.Bytes(), Position: &pullbudget.Position{Epoch: e.Position.Epoch, Seq: e.Position.Seq, Hash: bytes.Clone(e.Position.Hash[:])}, LeaderID: e.LeaderID.Bytes(), SchemaVersion: e.Schema, CommittedUnixMS: e.CommitMs, PrevHash: bytes.Clone(e.PrevHash[:]), Payload: e.Payload}
		prev = e.Position
	}
	head := pullbudget.Position{Epoch: in.Head.Epoch, Seq: in.Head.Seq, Hash: bytes.Clone(in.Head.Hash[:])}
	plan, err := pullbudget.Plan(head, candidates, maxEvents, maxBytes)
	if err != nil && !errors.Is(err, pullbudget.ErrBudgetTooSmall) {
		return nil, err
	}
	response := &replica.PullResponse{Disposition: replica.PullDisposition(plan.Disposition), ObservedHead: position(in.Head), RequiredBytes: plan.RequiredBytes}
	for _, e := range events[:plan.Count] {
		response.Events = append(response.Events, &replica.Event{ClusterId: e.ClusterID.Bytes(), Position: position(e.Position), LeaderId: e.LeaderID.Bytes(), SchemaVersion: e.Schema, CommittedUnixMs: e.CommitMs, PrevHash: bytes.Clone(e.PrevHash[:]), Payload: bytes.Clone(e.Payload)})
	}
	size := proto.Size(response)
	if size != int(plan.ResponseBytes) || size > int(maxBytes) {
		return nil, ErrInvalid
	}
	out, err := proto.Marshal(response)
	if err != nil || len(out) != size || len(out) > int(maxBytes) {
		return nil, ErrInvalid
	}
	return out, nil
}
func position(p eventlog.Position) *replica.Position {
	return &replica.Position{Epoch: p.Epoch, Seq: p.Seq, Hash: bytes.Clone(p.Hash[:])}
}
