package lease

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrStartupAlreadyReady = errors.New("lease startup: already ready")
	ErrStartupFailed       = errors.New("lease startup: recovery failed")
)

type StartupState string

const (
	StartupCold       StartupState = "cold"
	StartupRecovering StartupState = "recovering"
	StartupReady      StartupState = "ready"
	StartupFailed     StartupState = "failed"
	StartupClosed     StartupState = "closed"
)

// StartupCoordinator is a small fail-closed gate for the staged WAL recovery
// path. It does not open sockets or alter the default DHCP constructor; the
// caller must check Ready before enabling packet admission.
type StartupCoordinator struct {
	mu      sync.RWMutex
	state   StartupState
	lastSeq int64
	err     error
}

func NewStartupCoordinator() *StartupCoordinator {
	return &StartupCoordinator{state: StartupCold}
}

func (c *StartupCoordinator) State() StartupState {
	if c == nil {
		return StartupFailed
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *StartupCoordinator) LastSequence() int64 {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSeq
}

func (c *StartupCoordinator) Err() error {
	if c == nil {
		return ErrStartupNotReady
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.err
}

func (c *StartupCoordinator) Ready() bool {
	return c != nil && c.State() == StartupReady
}

// Recover runs the supplied synchronous recovery function exactly once per
// coordinator lifecycle. Recovery failure leaves the coordinator failed and
// never ready. A successful recovery is the only path to StartupReady.
func (c *StartupCoordinator) Recover(ctx context.Context, recoverFn func(context.Context) (int64, error)) error {
	if c == nil {
		return ErrStartupNotReady
	}
	if recoverFn == nil {
		return c.fail(errors.New("lease startup: recovery function is nil"))
	}
	c.mu.Lock()
	switch c.state {
	case StartupCold:
		c.state = StartupRecovering
	case StartupReady:
		c.mu.Unlock()
		return ErrStartupAlreadyReady
	case StartupFailed:
		err := c.err
		c.mu.Unlock()
		return fmt.Errorf("%w: %v", ErrStartupFailed, err)
	case StartupRecovering:
		c.mu.Unlock()
		return fmt.Errorf("%w: recovery already in progress", ErrStartupNotReady)
	case StartupClosed:
		c.mu.Unlock()
		return ErrStartupNotReady
	}
	c.mu.Unlock()

	if ctx == nil {
		return c.fail(errors.New("lease startup: context is nil"))
	}
	select {
	case <-ctx.Done():
		return c.fail(ctx.Err())
	default:
	}
	lastSeq, err := recoverFn(ctx)
	if err != nil {
		return c.fail(err)
	}
	c.mu.Lock()
	c.lastSeq = lastSeq
	c.state = StartupReady
	c.err = nil
	c.mu.Unlock()
	return nil
}

func (c *StartupCoordinator) fail(err error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
	c.state = StartupFailed
	return fmt.Errorf("%w: %v", ErrStartupNotReady, err)
}

// Close prevents a not-yet-started coordinator from becoming ready. Closing a
// ready coordinator records that admission must be stopped by the caller.
func (c *StartupCoordinator) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = StartupClosed
}
