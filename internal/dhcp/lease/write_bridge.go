package lease

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrWriteBridgeClosed = errors.New("lease write bridge: closed")
	ErrWriteBridgeFailed = errors.New("lease write bridge: durable write failed")
)

// DurableEventSink is the durable boundary used by MemoryWriteBridge. The
// event must be appended and synchronized before the method returns.
type DurableEventSink interface {
	DurableEvent(context.Context, WALEvent) error
}

// EventSubmitter hands an already durable event to the asynchronous SQLite
// projection. Submission is deliberately separate from the durable boundary.
type EventSubmitter interface {
	Submit(WALEvent) error
}

// MemoryWriteBridge is the staged write-path coordinator. It applies the new
// value to memory first, crosses the WAL durable boundary, then submits the
// same event to the asynchronous projection. If durability fails, the prior
// memory value is restored and no projection event is submitted.
//
// This is an explicit migration component; it does not replace Manager or
// change the default DHCP constructor.
type MemoryWriteBridge struct {
	index   *MemoryIndex
	durable DurableEventSink
	submit  EventSubmitter
	build   func(*Lease) WALEvent
}

func NewMemoryWriteBridge(index *MemoryIndex, durable DurableEventSink, submit EventSubmitter, build func(*Lease) WALEvent) (*MemoryWriteBridge, error) {
	if index == nil {
		return nil, errors.New("lease write bridge: memory index is nil")
	}
	if durable == nil {
		return nil, errors.New("lease write bridge: durable sink is nil")
	}
	if submit == nil {
		return nil, errors.New("lease write bridge: event submitter is nil")
	}
	if build == nil {
		return nil, errors.New("lease write bridge: event builder is nil")
	}
	return &MemoryWriteBridge{index: index, durable: durable, submit: submit, build: build}, nil
}

func (b *MemoryWriteBridge) Commit(ctx context.Context, value Lease) error {
	if b == nil || b.index == nil {
		return ErrWriteBridgeClosed
	}
	if value.ID == "" {
		return fmt.Errorf("%w: lease id is empty", ErrWALCorrupt)
	}
	previous, existed := b.index.Get(value.ID)
	b.index.Upsert(value)

	event := b.build(&value)
	if err := b.durable.DurableEvent(ctx, event); err != nil {
		b.restore(value.ID, previous, existed)
		return fmt.Errorf("%w: %v", ErrWriteBridgeFailed, err)
	}
	if err := b.submit.Submit(event); err != nil {
		// The WAL is already durable. Keep memory ahead of SQLite and return the
		// failure so readiness/ACK policy can stop further new bindings; replay
		// will recover this event after restart.
		return fmt.Errorf("%w: projection submit: %v", ErrWriteBridgeFailed, err)
	}
	return nil
}

func (b *MemoryWriteBridge) restore(id string, previous *Lease, existed bool) {
	if existed && previous != nil {
		b.index.Upsert(*previous)
		return
	}
	b.index.Remove(id)
}
