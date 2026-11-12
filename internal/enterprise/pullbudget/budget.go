// Package pullbudget plans protobuf PullResponse wire sizes for the explicit
// fields represented below. Unknown protobuf fields are not represented: a
// transport must construct a fresh response without them or size the final
// generated message before sending. It must also obtain events and observed head
// from one consistent storage view. This package does not authenticate peers,
// read storage, verify event hashes or grant authority.
package pullbudget

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	MaxEvents       = 256
	MaxBytes        = 8 << 20
	MinBytes        = 1024
	MaxPayloadBytes = 4 << 20
	Batch           = 1
	AtHead          = 2
	BudgetTooSmall  = 7
)

var ErrInvalid = errors.New("pullbudget: invalid request or input")
var ErrBudgetTooSmall = errors.New("pullbudget: next event does not fit")

type Position struct {
	Epoch, Seq uint64
	Hash       []byte
}
type Event struct {
	ClusterID         []byte
	Position          *Position
	LeaderID          []byte
	SchemaVersion     uint32
	CommittedUnixMS   int64
	PrevHash, Payload []byte
}
type Response struct {
	Disposition   int32
	Events        []Event
	ObservedHead  *Position
	RequiredBytes uint32
}
type PlanResult struct {
	Count                        int
	Disposition                  int32
	ResponseBytes, RequiredBytes uint32
}

// Size computes the uncompressed proto3 wire size for the represented fields
// of the supplied response, including presence of non-nil embedded messages.
// Unknown fields on actual protobuf messages are not included. Bytes with zero
// length and zero scalar fields are omitted. Negative commit times, negative
// dispositions, and excessively large fields are rejected by this subset.
func Size(r Response) (int, error) {
	if r.Disposition < 0 {
		return 0, ErrInvalid
	}
	n := scalarSize(1, uint64(r.Disposition)) + scalarSize(4, uint64(r.RequiredBytes))
	if r.ObservedHead != nil {
		p, e := positionSize(r.ObservedHead)
		if e != nil {
			return 0, e
		}
		n += messageSize(3, p)
	}
	for _, v := range r.Events {
		s, e := eventSize(v)
		if e != nil {
			return 0, e
		}
		n += messageSize(2, s)
	}
	return n, nil
}
func scalarSize(field protowire.Number, v uint64) int {
	if v == 0 {
		return 0
	}
	return protowire.SizeTag(field) + protowire.SizeVarint(v)
}
func bytesSize(field protowire.Number, v []byte) int {
	if len(v) == 0 {
		return 0
	}
	return messageSize(field, len(v))
}
func messageSize(field protowire.Number, n int) int {
	return protowire.SizeTag(field) + protowire.SizeBytes(n)
}
func positionSize(p *Position) (int, error) {
	if len(p.Hash) > MaxBytes {
		return 0, ErrInvalid
	}
	return scalarSize(1, p.Epoch) + scalarSize(2, p.Seq) + bytesSize(3, p.Hash), nil
}
func eventSize(e Event) (int, error) {
	if e.CommittedUnixMS < 0 || len(e.ClusterID) > MaxBytes || len(e.LeaderID) > MaxBytes || len(e.PrevHash) > MaxBytes || len(e.Payload) > MaxPayloadBytes {
		return 0, ErrInvalid
	}
	n := bytesSize(1, e.ClusterID) + bytesSize(3, e.LeaderID) + scalarSize(4, uint64(e.SchemaVersion)) + scalarSize(5, uint64(e.CommittedUnixMS)) + bytesSize(6, e.PrevHash) + bytesSize(7, e.Payload)
	if e.Position != nil {
		s, err := positionSize(e.Position)
		if err != nil {
			return 0, err
		}
		n += messageSize(2, s)
	}
	return n, nil
}

// Plan selects a contiguous prefix. Its caller MUST already have validated
// authenticated identity, authority, epoch, schemas, content hashes, and chain
// continuity from its requested anchor through the returned candidate prefix.
// ObservedHead is telemetry from the same read snapshot; an unreturned suffix
// is not proven by this planner. Events must be a consecutive prefix, not a
// filtered list; empty requires an anchor exactly equal to the validated head.
// Input and slices must not
// be mutated concurrently or changed before constructing the actual response.
// maxEvents/maxBytes zero select defaults. Nonzero bounds are 1..256 events and
// 1024..8MiB. On ErrBudgetTooSmall RequiredBytes is the size of a BATCH response
// with just the first event and observed head; ResponseBytes is the separate
// REQUEST_BUDGET_TOO_SMALL response size. No event is skipped.
func Plan(head Position, events []Event, maxEvents, maxBytes uint32) (PlanResult, error) {
	var out PlanResult
	if maxEvents == 0 {
		maxEvents = MaxEvents
	}
	if maxBytes == 0 {
		maxBytes = MaxBytes
	}
	if maxEvents > MaxEvents || maxBytes < MinBytes || maxBytes > MaxBytes || len(head.Hash) != 32 {
		return out, ErrInvalid
	}
	base, err := Size(Response{Disposition: Batch, ObservedHead: &head})
	if err != nil {
		return out, err
	}
	if len(events) == 0 {
		n, e := Size(Response{Disposition: AtHead, ObservedHead: &head})
		if e != nil || n > int(maxBytes) {
			return out, ErrInvalid
		}
		return PlanResult{Disposition: AtHead, ResponseBytes: uint32(n)}, nil
	}
	total := base
	for i, e := range events {
		if i >= int(maxEvents) {
			break
		}
		if len(e.ClusterID) != 16 || len(e.LeaderID) != 16 || len(e.PrevHash) != 32 || e.Position == nil || len(e.Position.Hash) != 32 {
			return PlanResult{}, fmt.Errorf("%w: event %d shape", ErrInvalid, i)
		}
		size, err := eventSize(e)
		if err != nil {
			return PlanResult{}, fmt.Errorf("%w: event %d", err, i)
		}
		contribution := messageSize(2, size)
		if total+contribution > int(maxBytes) {
			if i == 0 {
				required := base + contribution
				errSize, e := Size(Response{Disposition: BudgetTooSmall, ObservedHead: &head, RequiredBytes: uint32(required)})
				if e != nil || errSize > int(maxBytes) {
					return PlanResult{}, ErrInvalid
				}
				return PlanResult{Disposition: BudgetTooSmall, RequiredBytes: uint32(required), ResponseBytes: uint32(errSize)}, ErrBudgetTooSmall
			}
			break
		}
		total += contribution
		out.Count++
	}
	out.Disposition = Batch
	out.ResponseBytes = uint32(total)
	return out, nil
}
