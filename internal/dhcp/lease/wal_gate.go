package lease

import (
	"context"
	"errors"
	"fmt"
)

var ErrWALGateClosed = errors.New("lease wal gate: closed")

// WALDurableGate appends one lease state event and synchronizes the WAL before
// returning. It is the concrete bridge for the server DurableLeaseGate seam;
// it deliberately does not submit to the asynchronous SQLite applier because
// ACK safety is established by WAL durability, not by projection lag.
type WALDurableGate struct {
	wal    *WALFile
	nextOp func(*Lease) WALEvent
}

// NewWALDurableGate constructs an explicit WAL-backed ACK prerequisite. The
// event sequence is taken from the WAL file and the caller supplies the event
// shape so this component cannot guess whether a mutation is an upsert or a
// removal.
func NewWALDurableGate(wal *WALFile, nextOp func(*Lease) WALEvent) (*WALDurableGate, error) {
	if wal == nil {
		return nil, ErrWALGateClosed
	}
	if nextOp == nil {
		return nil, errors.New("lease wal gate: event builder is nil")
	}
	return &WALDurableGate{wal: wal, nextOp: nextOp}, nil
}

// DurableEvent appends and syncs an already-built lease event, returning the
// canonical event with the sequence assigned by the WAL. The returned value is
// the only event that may be submitted to a projection: it prevents the old
// sequence-less builder event from diverging from the durable record.
func (g *WALDurableGate) DurableEvent(ctx context.Context, event WALEvent) (WALEvent, error) {
	if g == nil || g.wal == nil {
		return event, ErrWALGateClosed
	}
	if ctx == nil {
		return event, errors.New("lease wal gate: context is nil")
	}
	select {
	case <-ctx.Done():
		return event, ctx.Err()
	default:
	}
	if event.Version == 0 {
		event.Version = currentWALEventVersion
	}
	if event.Op == "" {
		event.Op = WALEventUpsert
	}
	return g.wal.AppendDurable(event)
}

// Durable builds one lease event, appends and syncs it, and discards no
// sequence information. It remains the compatibility implementation of the
// server DurableLeaseGate interface.
func (g *WALDurableGate) Durable(ctx context.Context, value *Lease) error {
	if value == nil {
		return fmt.Errorf("%w: lease is nil", ErrWALGateClosed)
	}
	event := g.nextOp(value)
	if event.Lease == nil && event.Op == WALEventUpsert {
		copy := *value
		event.Lease = &copy
	}
	_, err := g.DurableEvent(ctx, event)
	return err
}
