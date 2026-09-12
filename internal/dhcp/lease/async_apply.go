package lease

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	ErrApplierClosed  = errors.New("lease applier: closed")
	ErrApplierFailed  = errors.New("lease applier: failed")
	ErrApplyQueueFull = errors.New("lease applier: queue full")
)

// ApplyFunc applies one already durable WAL event to the SQLite projection.
// The function must make the event and its applied sequence watermark one
// transaction; this component only owns ordering and bounded backpressure.
type ApplyFunc func(context.Context, WALEvent) error

// AsyncApplier serializes WAL events into a bounded asynchronous projection
// queue. It is deliberately storage-agnostic so the transaction boundary stays
// in the caller that owns the SQLite schema and DNS outbox.
type AsyncApplier struct {
	ctx    context.Context
	cancel context.CancelFunc
	queue  chan WALEvent
	apply  ApplyFunc

	mu        sync.RWMutex
	failed    error
	closeErr  error
	closed    bool
	applied   atomic.Int64
	started   chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// NewAsyncApplier starts one ordered worker. applied is the last SQLite
// projection watermark; the first accepted event must be applied+1.
func NewAsyncApplier(parent context.Context, capacity int, applied int64, apply ApplyFunc) (*AsyncApplier, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("lease applier: capacity must be positive")
	}
	if apply == nil {
		return nil, errors.New("lease applier: apply function is nil")
	}
	ctx, cancel := context.WithCancel(parent)
	a := &AsyncApplier{
		ctx:     ctx,
		cancel:  cancel,
		queue:   make(chan WALEvent, capacity),
		apply:   apply,
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
	a.applied.Store(applied)
	go a.run()
	return a, nil
}

func (a *AsyncApplier) run() {
	close(a.started)
	defer close(a.done)
	for {
		select {
		case <-a.ctx.Done():
			return
		case event := <-a.queue:
			if err := a.applyOne(event); err != nil {
				a.mu.Lock()
				a.failed = err
				a.mu.Unlock()
				a.cancel()
				return
			}
		}
	}
}

func (a *AsyncApplier) applyOne(event WALEvent) error {
	last := a.applied.Load()
	if event.Seq <= last {
		return nil
	}
	if event.Seq != last+1 {
		return fmt.Errorf("%w: expected=%d got=%d", ErrWALGap, last+1, event.Seq)
	}
	if err := a.apply(a.ctx, event); err != nil {
		return fmt.Errorf("%w: seq=%d: %v", ErrApplierFailed, event.Seq, err)
	}
	a.applied.Store(event.Seq)
	return nil
}

// Submit never blocks the DHCP receive loop. The event must already have
// crossed the WAL durable boundary before submission.
func (a *AsyncApplier) Submit(event WALEvent) error {
	if err := validateWALEvent(event); err != nil {
		return err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return ErrApplierClosed
	}
	if a.failed != nil {
		return fmt.Errorf("%w: %v", ErrApplierFailed, a.failed)
	}
	select {
	case a.queue <- event:
		return nil
	default:
		return ErrApplyQueueFull
	}
}

func (a *AsyncApplier) Applied() int64 { return a.applied.Load() }

func (a *AsyncApplier) QueueDepth() int { return len(a.queue) }

func (a *AsyncApplier) Capacity() int { return cap(a.queue) }

func (a *AsyncApplier) Failed() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.failed
}

// Wait blocks until all currently queued work is applied or the applier fails.
func (a *AsyncApplier) Wait(ctx context.Context) error {
	for {
		if err := a.Failed(); err != nil {
			return err
		}
		if len(a.queue) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.done:
			if err := a.Failed(); err != nil {
				return err
			}
			return nil
		default:
		}
	}
}

// Close stops admission and waits for the worker. It does not discard already
// durable events from the WAL; un-applied events remain replayable at startup.
func (a *AsyncApplier) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed = true
		a.mu.Unlock()
		a.cancel()
		<-a.done
	})
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closeErr != nil {
		return a.closeErr
	}
	return a.failed
}
