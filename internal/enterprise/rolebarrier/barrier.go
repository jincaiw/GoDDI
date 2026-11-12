// Package rolebarrier provides an in-process drain primitive. It does not
// establish authority, provide fencing, or persist role state.
package rolebarrier

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed indicates that no new operation may enter the barrier.
var ErrClosed = errors.New("rolebarrier: closed")

// Barrier is closed by default. There is deliberately no production API to
// open it: a future authority implementation must establish that protocol.
// A Barrier must not be copied after first use.
type Barrier struct {
	mu       sync.Mutex
	open     bool
	inflight uint64
	drained  chan struct{}
}

// Lease tracks one operation. The holder must retain it until the operation,
// including any externally visible effect, has completely finished.
type Lease struct{ state *leaseState }

type leaseState struct {
	barrier *Barrier
	once    sync.Once
}

// Acquire enters only while the barrier is open. A successful Acquire and
// Fence's transition to closed are ordered under the same mutex.
func (b *Barrier) Acquire() (*Lease, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.open {
		return nil, ErrClosed
	}
	b.inflight++
	return &Lease{state: &leaseState{barrier: b}}, nil
}

// Release ends an operation. Repeated or concurrent calls are harmless.
func (l *Lease) Release() {
	if l == nil || l.state == nil {
		return
	}
	l.state.once.Do(func() {
		b := l.state.barrier
		b.mu.Lock()
		defer b.mu.Unlock()
		b.inflight--
		if !b.open && b.inflight == 0 && b.drained != nil {
			close(b.drained)
			b.drained = nil
		}
	})
}

// Fence atomically rejects new entrants, then waits for all existing leases.
// Cancellation interrupts only the wait; the barrier remains closed and a
// later caller may wait again. Concurrent callers observe the same drain.
func (b *Barrier) Fence(ctx context.Context) error {
	b.mu.Lock()
	b.open = false
	if b.inflight == 0 {
		b.mu.Unlock()
		return nil
	}
	if b.drained == nil {
		b.drained = make(chan struct{})
	}
	drained := b.drained
	b.mu.Unlock()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
