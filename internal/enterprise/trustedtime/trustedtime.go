// Package trustedtime implements a local, fail-closed time assessment. It does
// not authenticate time sources, persist watermarks, or grant any authority.
package trustedtime

import (
	"errors"
	"math/big"
)

var ErrUnhealthy = errors.New("trusted time unavailable")
var ErrInvalid = errors.New("invalid time configuration or sample")

// Clock returns monotonic nanoseconds since an arbitrary boot-local origin.
// It must include suspend time, never reset within an instance, and be read at
// the exact instant represented by each sample. Recreate State after reboot,
// resume, or any uncertainty about these properties.
type Clock interface{ Now() int64 }

type Config struct {
	MaxUncertainty    int64 // nanoseconds, including holdover drift
	MaxStep           int64 // maximum separation of predicted and sampled UTC intervals
	MaxHoldover       int64 // nanoseconds since last accepted sample
	StableWindow      int64 // nanoseconds of continuously sampled stable operation
	MaxSampleInterval int64 // maximum interval between samples during recovery
	DriftPPM          int64 // maximum holdover error growth, parts per million
}

// Sample is UTC Unix nanoseconds and its symmetric error bound at Clock.Now().
// The caller must obtain an authenticated source and account for sampling delay
// in Uncertainty. Sources reporting leap/smear semantics must match the caller's
// UTC and persisted lease semantics.
type Sample struct{ UTC, Uncertainty int64 }
type Bounds struct{ Lower, Upper int64 }

// State is not concurrency safe. Persist HighWater before relying on a result;
// after restart pass the durable value back to New and repeat the stable window.
type State struct {
	c                   Clock
	cfg                 Config
	high                int64
	hasSample, healthy  bool
	seenMono            bool
	lastReadMono        int64
	lastMono, startMono int64
	last                Bounds
}

func New(c Clock, cfg Config, persistedHighWater int64) (*State, error) {
	if c == nil || cfg.MaxUncertainty < 0 || cfg.MaxStep < 0 || cfg.MaxHoldover <= 0 || cfg.StableWindow <= 0 || cfg.MaxSampleInterval <= 0 || cfg.MaxSampleInterval > cfg.MaxHoldover || cfg.MaxSampleInterval > cfg.StableWindow || cfg.DriftPPM < 0 {
		return nil, ErrInvalid
	}
	return &State{c: c, cfg: cfg, high: persistedHighWater}, nil
}

// Observe receives a sample or nil when the trusted source is absent. Absence
// permits an already-healthy state to remain healthy only within holdover; it
// resets an unfinished stability window. Rejected observations reset recovery.
// Recovery also restarts if sample gaps exceed MaxSampleInterval. This bound
// cannot detect a discontinuity that the caller fails to report between samples.
func (s *State) Observe(sample *Sample) error {
	now, ok := s.readMonotonic()
	if !ok {
		return s.fail()
	}
	if sample == nil {
		if !s.healthy {
			s.hasSample = false
			return ErrUnhealthy
		}
		_, err := s.at(now)
		if err != nil {
			return s.fail()
		}
		return nil
	}
	if sample.Uncertainty < 0 || sample.Uncertainty > s.cfg.MaxUncertainty {
		return s.fail()
	}
	lower, ok := sub(sample.UTC, sample.Uncertainty)
	if !ok {
		return s.fail()
	}
	upper, ok := add(sample.UTC, sample.Uncertainty)
	if !ok || upper < s.high {
		return s.fail()
	}
	if s.hasSample && !s.healthy {
		gap, ok := sub(now, s.lastMono)
		if !ok {
			return s.fail()
		}
		if gap > s.cfg.MaxSampleInterval {
			// A sparse sequence cannot establish a continuous recovery window.
			s.hasSample = false
		}
	}
	if s.hasSample {
		predicted, err := s.at(now)
		if err != nil {
			return s.fail()
		}
		// The minimum distance between intervals must not exceed MaxStep.
		if (lower > predicted.Upper && greaterDifference(lower, predicted.Upper, s.cfg.MaxStep)) || (predicted.Lower > upper && greaterDifference(predicted.Lower, upper, s.cfg.MaxStep)) {
			return s.fail()
		}
	} else {
		s.startMono = now
	}
	s.last = Bounds{lower, upper}
	s.lastMono = now
	s.hasSample = true
	if lower > s.high {
		s.high = lower
	}
	elapsed, ok := sub(now, s.startMono)
	if !ok {
		return s.fail()
	}
	if elapsed >= s.cfg.StableWindow {
		s.healthy = true
	}
	if !s.healthy {
		return ErrUnhealthy
	}
	return nil
}

// Current computes conservative bounds using monotonic elapsed time and maximum
// drift. Expired holdover, arithmetic overflow, or monotonic regression fail
// closed and restart the stability window.
func (s *State) Current() (Bounds, error) {
	if !s.healthy {
		return Bounds{}, ErrUnhealthy
	}
	now, ok := s.readMonotonic()
	if !ok {
		s.fail()
		return Bounds{}, ErrUnhealthy
	}
	b, err := s.at(now)
	if err != nil {
		s.fail()
		return Bounds{}, ErrUnhealthy
	}
	if b.Lower > s.high {
		s.high = b.Lower
	}
	return b, nil
}

// HighWater returns the maximum accepted conservative lower bound. This value
// is not durable until the caller persists it, and is not an authorization.
func (s *State) HighWater() int64 { return s.high }

// Expired reports true only when the conservative current lower bound is
// strictly later than the conservative upper bound of the promised expiry.
func Expired(current Bounds, expiryUpper int64) bool {
	return current.Lower <= current.Upper && current.Lower > expiryUpper
}

func (s *State) at(now int64) (Bounds, error) {
	if !s.hasSample || now < 0 || now < s.lastMono {
		return Bounds{}, ErrUnhealthy
	}
	elapsed, ok := sub(now, s.lastMono)
	if !ok || elapsed > s.cfg.MaxHoldover {
		return Bounds{}, ErrUnhealthy
	}
	// Round error growth up, using unbounded intermediate arithmetic.
	growth := new(big.Int).Mul(big.NewInt(elapsed), big.NewInt(s.cfg.DriftPPM))
	growth.Add(growth, big.NewInt(999999))
	growth.Div(growth, big.NewInt(1000000))
	if !growth.IsInt64() {
		return Bounds{}, ErrUnhealthy
	}
	g := growth.Int64()
	width := new(big.Int).Sub(big.NewInt(s.last.Upper), big.NewInt(s.last.Lower))
	width.Add(width, new(big.Int).Mul(big.NewInt(g), big.NewInt(2)))
	limit := new(big.Int).Mul(big.NewInt(s.cfg.MaxUncertainty), big.NewInt(2))
	if width.Cmp(limit) > 0 {
		return Bounds{}, ErrUnhealthy
	}
	lo, ok := add(s.last.Lower, elapsed)
	if !ok {
		return Bounds{}, ErrUnhealthy
	}
	lo, ok = sub(lo, g)
	if !ok {
		return Bounds{}, ErrUnhealthy
	}
	hi, ok := add(s.last.Upper, elapsed)
	if !ok {
		return Bounds{}, ErrUnhealthy
	}
	hi, ok = add(hi, g)
	if !ok {
		return Bounds{}, ErrUnhealthy
	}
	return Bounds{lo, hi}, nil
}
func (s *State) readMonotonic() (int64, bool) {
	now := s.c.Now()
	if now < 0 || (s.seenMono && now < s.lastReadMono) {
		return 0, false
	}
	s.lastReadMono, s.seenMono = now, true
	return now, true
}

func (s *State) fail() error { s.healthy = false; s.hasSample = false; return ErrUnhealthy }
func add(a, b int64) (int64, bool) {
	x := new(big.Int).Add(big.NewInt(a), big.NewInt(b))
	return x.Int64(), x.IsInt64()
}
func sub(a, b int64) (int64, bool) {
	x := new(big.Int).Sub(big.NewInt(a), big.NewInt(b))
	return x.Int64(), x.IsInt64()
}
func greaterDifference(a, b, limit int64) bool {
	return new(big.Int).Sub(big.NewInt(a), big.NewInt(b)).Cmp(big.NewInt(limit)) > 0
}
