package lease

import (
	"context"
	"errors"
	"sync"
)

var ErrWALLifecycleClosed = errors.New("lease wal lifecycle: closed")

// WALLifecycle owns the opt-in ordering between the durable WAL and its
// asynchronous SQLite projection. It is intentionally separate from Server:
// the default DHCP process does not construct this component.
//
// AppendDurable holds the read side of the admission lock through both the WAL
// durable append and applier submission. Close takes the write side, so its
// admission cutoff is fixed before Sync and drain begin.
type WALLifecycle struct {
	mu              sync.RWMutex
	closeMu         sync.Mutex
	wal             *WALFile
	applier         *AsyncApplier
	closing         bool
	admissionClosed bool
	closed          bool
}

func NewWALLifecycle(wal *WALFile, applier *AsyncApplier) (*WALLifecycle, error) {
	if wal == nil {
		return nil, errors.New("lease wal lifecycle: WAL is nil")
	}
	if applier == nil {
		return nil, errors.New("lease wal lifecycle: applier is nil")
	}
	return &WALLifecycle{wal: wal, applier: applier}, nil
}

// AppendDurable appends and syncs the event before making it visible to the
// asynchronous projection. If submission is rejected after the durable append,
// the event remains in the WAL and will be replayed during the next startup.
func (l *WALLifecycle) AppendDurable(event WALEvent) (WALEvent, error) {
	if l == nil {
		return event, ErrWALLifecycleClosed
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed || l.admissionClosed {
		return event, ErrWALLifecycleClosed
	}
	appended, err := l.wal.AppendDurable(event)
	if err != nil {
		return event, err
	}
	if err := l.applier.Submit(appended); err != nil {
		return appended, err
	}
	return appended, nil
}

func (l *WALLifecycle) WAL() *WALFile {
	if l == nil {
		return nil
	}
	return l.wal
}

func (l *WALLifecycle) Applier() *AsyncApplier {
	if l == nil {
		return nil
	}
	return l.applier
}

// Close performs graceful opt-in shutdown. New admissions are rejected first;
// then the WAL is synced, accepted projection work is drained, the WAL is
// synced again, and finally both resources are closed. A cancelled or timed
// out drain leaves durable resources open and retryable; queued WAL events are
// therefore still available for startup replay.
func (l *WALLifecycle) Close(ctx context.Context) error {
	if l == nil {
		return ErrWALLifecycleClosed
	}
	if ctx == nil {
		return errors.New("lease wal lifecycle: close context is nil")
	}

	l.closeMu.Lock()
	defer l.closeMu.Unlock()

	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return ErrWALLifecycleClosed
	}
	l.closing = true
	l.admissionClosed = true
	l.mu.Unlock()
	fail := func(err error) error {
		l.mu.Lock()
		l.closing = false
		l.mu.Unlock()
		return err
	}

	if err := l.wal.Sync(); err != nil {
		return fail(err)
	}
	if err := l.applier.DrainAndClose(ctx); err != nil {
		return fail(err)
	}
	if err := l.wal.Sync(); err != nil {
		return fail(err)
	}
	if err := l.wal.Close(); err != nil && !errors.Is(err, ErrWALClosed) {
		return fail(err)
	}
	l.mu.Lock()
	l.closing = false
	l.closed = true
	l.mu.Unlock()
	return nil
}
