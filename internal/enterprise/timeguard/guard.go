// Package timeguard combines a local time assessment with a durable watermark.
// It does not authenticate sources, authorize roles, or detect disk rollback.
// Samples and the suspend-aware monotonic clock must satisfy trustedtime's
// contract. Recreate the guard after reboot, resume or clock uncertainty.
package timeguard

import (
	"errors"
	"fmt"
	"sync"

	"github.com/jincaiw/GoDDI/internal/enterprise/trustedtime"
)

var ErrLocked = errors.New("timeguard: permanently unavailable; recreate guard")
var ErrClosed = errors.New("timeguard: closed")

// Water must provide serialized, durable high-water reads and monotonic advances.
// An error, including an uncertain commit, is treated as a permanent fault.
type Water interface {
	Read() (int64, error)
	Advance(int64) error
}

type Guard struct {
	mu     sync.Mutex
	state  *trustedtime.State
	water  Water
	fault  error
	closed bool
}

// New reads existing durable state; it never initializes absent state. Each new
// guard must complete a new stability window before reporting usable bounds.
func New(clock trustedtime.Clock, cfg trustedtime.Config, water Water) (*Guard, error) {
	if water == nil {
		return nil, fmt.Errorf("timeguard: missing watermark store")
	}
	high, err := water.Read()
	if err != nil {
		return nil, fmt.Errorf("timeguard: read watermark: %w", err)
	}
	if high <= 0 {
		return nil, fmt.Errorf("timeguard: invalid durable watermark")
	}
	state, err := trustedtime.New(clock, cfg, high)
	if err != nil {
		return nil, err
	}
	return &Guard{state: state, water: water}, nil
}
func (g *Guard) ready() error {
	if g.closed {
		return ErrClosed
	}
	if g.fault != nil {
		return errors.Join(ErrLocked, g.fault)
	}
	return nil
}
func (g *Guard) persist() error {
	if err := g.water.Advance(g.state.HighWater()); err != nil {
		g.fault = fmt.Errorf("timeguard: persist watermark: %w", err)
		return errors.Join(ErrLocked, g.fault)
	}
	return nil
}

// Observe accepts an authenticated sample supplied by the caller, or nil on
// source loss. Even during the stability window accepted samples can raise the
// watermark, so it is persisted before returning either success or an error.
func (g *Guard) Observe(sample *trustedtime.Sample) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.ready(); err != nil {
		return err
	}
	err := g.state.Observe(sample)
	if e := g.persist(); e != nil {
		return e
	}
	return err
}

// Current returns zero bounds on every error and only returns bounds after the
// corresponding watermark has been durably confirmed under the same lock.
func (g *Guard) Current() (trustedtime.Bounds, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.ready(); err != nil {
		return trustedtime.Bounds{}, err
	}
	b, err := g.state.Current()
	if err != nil {
		return trustedtime.Bounds{}, err
	}
	if err := g.persist(); err != nil {
		return trustedtime.Bounds{}, err
	}
	return b, nil
}

// Close permanently disables this guard; it does not close the supplied store.
func (g *Guard) Close() { g.mu.Lock(); defer g.mu.Unlock(); g.closed = true }
