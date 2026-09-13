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

// Durable appends and syncs one lease event. Context cancellation is checked
// before touching the file; once file I/O begins, the WAL Sync result is the
// authoritative boundary and any error is propagated fail-closed. Sequence
// assignment and append+sync are one WAL critical section so concurrent DHCP
// workers cannot reserve the same sequence.
func (g *WALDurableGate) Durable(ctx context.Context, value *Lease) error {
	if g == nil || g.wal == nil {
		return ErrWALGateClosed
	}
	if value == nil {
		return fmt.Errorf("%w: lease is nil", ErrWALGateClosed)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	event := g.nextOp(value)
	if event.Version == 0 {
		event.Version = currentWALEventVersion
	}
	if event.Op == "" {
		event.Op = WALEventUpsert
	}
	if event.Lease == nil && event.Op == WALEventUpsert {
		copy := *value
		event.Lease = &copy
	}
	_, err := g.wal.AppendDurable(event)
	return err
}
