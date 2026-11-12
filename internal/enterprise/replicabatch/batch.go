// Package replicabatch validates bounded candidate STATIC replication batches.
// It cannot authenticate peers, establish an anchor, apply a transaction or
// determine whether an observed position is the authoritative global tail.
package replicabatch

import (
	"errors"
	"fmt"
	"math"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/staticbundle"
)

const MaxEvents = 256

// MaxPayloadBytes bounds only event payloads, not the serialized protobuf
// response. Transport must independently enforce the whole-response limit.
const MaxPayloadBytes = 8 << 20

var ErrInvalid = errors.New("replicabatch: invalid batch")

// Expected is supplied from an independently established trusted binding.
// Constructing it does not authenticate a peer, authorize application, or
// prove that the anchor is on the authoritative chain.
type Expected struct {
	Cluster, Leader eventlog.ID
	Start           eventlog.Position
	Schema          uint32
}

// Batch has no exported fields. Its bundles return only independent copies of
// mutable payloads through staticbundle.Result's accessors.
type Batch struct {
	start Expected
	head  eventlog.Position
	items []staticbundle.Result
}

func (b *Batch) Len() int {
	if b == nil {
		return 0
	}
	return len(b.items)
}
func (b *Batch) Start() Expected {
	if b == nil {
		return Expected{}
	}
	return b.start
}
func (b *Batch) Head() eventlog.Position {
	if b == nil {
		return eventlog.Position{}
	}
	return b.head
}
func (b *Batch) At(i int) (staticbundle.Result, bool) {
	if b == nil || i < 0 || i >= len(b.items) {
		return staticbundle.Result{}, false
	}
	return b.items[i], true
}

// Verify accepts a bounded batch beginning strictly after the caller's exact
// externally trusted start position. A zero-length batch is valid and retains
// that start position, but says nothing about the authoritative tail. Duplicate
// positions are rejected; retained-history replay is a separate protocol.
// Callers must not modify the input slice or any Payload concurrently with this
// call. Original payload bytes are copied and hashed, never re-encoded for hash.
func Verify(expected Expected, events []eventlog.Event) (*Batch, error) {
	if expected.Cluster == (eventlog.ID{}) || expected.Leader == (eventlog.ID{}) || expected.Cluster == expected.Leader || expected.Schema != eventlog.SchemaVersionV1 || expected.Start.Epoch == 0 || expected.Start.Epoch > math.MaxInt64 || expected.Start.Seq > math.MaxInt64 || (expected.Start.Epoch == 1 && expected.Start.Seq == 0 && expected.Start.Hash != eventlog.GenesisHash()) || len(events) > MaxEvents {
		return nil, ErrInvalid
	}
	var total uint64
	for _, event := range events {
		n := uint64(len(event.Payload))
		if n == 0 || n > eventlog.MaxPayloadBytes || n > uint64(MaxPayloadBytes)-total {
			return nil, ErrInvalid
		}
		total += n
	}
	items := make([]staticbundle.Result, 0, len(events))
	head := expected.Start
	for i, incoming := range events {
		e := incoming
		e.Payload = append([]byte(nil), incoming.Payload...)
		if e.ClusterID != expected.Cluster || e.LeaderID != expected.Leader || e.Schema != expected.Schema || e.Position.Epoch != head.Epoch || head.Seq >= math.MaxInt64 || e.Position.Seq != head.Seq+1 || e.PrevHash != head.Hash || e.CommitMs <= 0 {
			return nil, fmt.Errorf("%w: envelope at index %d", ErrInvalid, i)
		}
		if err := e.Verify(); err != nil {
			return nil, fmt.Errorf("%w: event %d: %v", ErrInvalid, i, err)
		}
		value, err := staticbundle.Validate(e)
		if err != nil {
			return nil, fmt.Errorf("%w: candidate %d: %v", ErrInvalid, i, err)
		}
		items = append(items, value)
		head = e.Position
	}
	return &Batch{start: expected, head: head, items: items}, nil
}
