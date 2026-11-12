package trustedtime

import (
	"errors"
	"math"
	"testing"
)

type manualClock struct{ now int64 }

func (c *manualClock) Now() int64 { return c.now }
func config() Config {
	return Config{MaxUncertainty: 10, MaxStep: 2, MaxHoldover: 20, StableWindow: 10, MaxSampleInterval: 10, DriftPPM: 0}
}
func state(t *testing.T) (*State, *manualClock) {
	t.Helper()
	c := &manualClock{}
	s, e := New(c, config(), 0)
	if e != nil {
		t.Fatal(e)
	}
	return s, c
}
func observe(t *testing.T, s *State, c *manualClock, mono, utc, unc int64, want error) {
	t.Helper()
	c.now = mono
	e := s.Observe(&Sample{utc, unc})
	if !errors.Is(e, want) {
		t.Fatalf("at %d: %v want %v", mono, e, want)
	}
}
func healthy(t *testing.T, s *State, c *manualClock) {
	t.Helper()
	observe(t, s, c, 0, 100, 1, ErrUnhealthy)
	observe(t, s, c, 10, 110, 1, nil)
}

func TestStabilityAndJumps(t *testing.T) {
	for _, tc := range []struct {
		name string
		jump int64
	}{{"forward", 30}, {"backward", -30}} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := state(t)
			healthy(t, s, c)
			observe(t, s, c, 11, 111+tc.jump, 1, ErrUnhealthy)
			if _, e := s.Current(); !errors.Is(e, ErrUnhealthy) {
				t.Fatal(e)
			}
			observe(t, s, c, 12, 112, 1, ErrUnhealthy)
			observe(t, s, c, 21, 121, 1, ErrUnhealthy)
			observe(t, s, c, 22, 122, 1, nil)
		})
	}
}
func TestRecoveryRequiresBoundedSampleGaps(t *testing.T) {
	cfg := config()
	cfg.MaxSampleInterval = 4
	c := &manualClock{}
	s, err := New(c, cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	observe(t, s, c, 0, 100, 0, ErrUnhealthy)
	// A ten-unit gap is inside holdover but cannot prove continuous recovery.
	observe(t, s, c, 10, 110, 0, ErrUnhealthy)
	observe(t, s, c, 14, 114, 0, ErrUnhealthy)
	observe(t, s, c, 18, 118, 0, ErrUnhealthy)
	observe(t, s, c, 20, 120, 0, nil)
}

func TestCurrentAdvancesWatermarkAndDetectsClockRegression(t *testing.T) {
	s, c := state(t)
	healthy(t, s, c)
	c.now = 25
	bounds, err := s.Current()
	if err != nil || s.HighWater() != bounds.Lower || bounds.Lower != 124 {
		t.Fatalf("current watermark: bounds=%+v high=%d err=%v", bounds, s.HighWater(), err)
	}
	// This is still later than the last sample at 10, but earlier than the
	// previously observed clock reading at 25 and must close the gate.
	c.now = 20
	if _, err := s.Current(); !errors.Is(err, ErrUnhealthy) {
		t.Fatalf("monotonic regression accepted: %v", err)
	}
}

func TestSourceHoldoverAndUncertainty(t *testing.T) {
	s, c := state(t)
	healthy(t, s, c)
	c.now = 30
	if e := s.Observe(nil); e != nil {
		t.Fatal(e)
	}
	b, e := s.Current()
	if e != nil || b != (Bounds{129, 131}) {
		t.Fatalf("%+v %v", b, e)
	}
	c.now = 31
	if e := s.Observe(nil); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	observe(t, s, c, 32, 132, 1, ErrUnhealthy)
	c.now = 33
	if e := s.Observe(nil); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	observe(t, s, c, 34, 134, 1, ErrUnhealthy)
	observe(t, s, c, 44, 144, 1, nil)
	observe(t, s, c, 45, 145, 11, ErrUnhealthy)
}
func TestRestartWatermarkAndExpiry(t *testing.T) {
	s, c := state(t)
	healthy(t, s, c)
	h := s.HighWater()
	if h != 109 {
		t.Fatal(h)
	}
	s, e := New(c, config(), h)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Current(); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	observe(t, s, c, 11, 100, 1, ErrUnhealthy)
	observe(t, s, c, 12, 112, 1, ErrUnhealthy)
	observe(t, s, c, 22, 122, 1, nil)
	b, e := s.Current()
	if e != nil {
		t.Fatal(e)
	}
	if Expired(b, b.Lower) || !Expired(b, b.Lower-1) || Expired(Bounds{2, 1}, 0) {
		t.Fatal("expiry must be strict and bounds valid")
	}
}
func TestOverflowAndRegression(t *testing.T) {
	cfg := config()
	cfg.StableWindow, cfg.MaxSampleInterval = 1, 1
	c := &manualClock{}
	s, _ := New(c, cfg, math.MinInt64)
	if e := s.Observe(&Sample{math.MaxInt64, 1}); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	if e := s.Observe(&Sample{math.MinInt64, 1}); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	if e := s.Observe(&Sample{math.MaxInt64 - 2, 0}); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	c.now = 1
	if e := s.Observe(&Sample{math.MaxInt64 - 1, 0}); e != nil {
		t.Fatal(e)
	}
	c.now = 3
	if _, e := s.Current(); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	c.now = 5
	if e := s.Observe(&Sample{10, 0}); !errors.Is(e, ErrUnhealthy) {
		t.Fatal("watermark must survive errors", e)
	}
	s, _ = New(c, cfg, 0)
	if e := s.Observe(&Sample{10, 0}); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	c.now = 6
	if e := s.Observe(&Sample{11, 0}); e != nil {
		t.Fatal(e)
	}
	c.now = 4
	if _, e := s.Current(); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
}
func TestDriftRoundingAndLimit(t *testing.T) {
	cfg := config()
	cfg.StableWindow, cfg.MaxSampleInterval = 1, 1
	cfg.DriftPPM = 1_000_000
	cfg.MaxUncertainty = 2
	c := &manualClock{}
	s, _ := New(c, cfg, 0)
	if e := s.Observe(&Sample{100, 0}); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
	c.now = 1
	if e := s.Observe(&Sample{101, 0}); e != nil {
		t.Fatal(e)
	}
	c.now = 3
	b, e := s.Current()
	if e != nil || b != (Bounds{101, 105}) {
		t.Fatalf("%v %v", b, e)
	}
	c.now = 4
	if _, e = s.Current(); !errors.Is(e, ErrUnhealthy) {
		t.Fatal(e)
	}
}
func TestInvalidConfig(t *testing.T) {
	cfg := config()
	cfg.MaxStep = -1
	if _, e := New(&manualClock{}, cfg, 0); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := New(nil, config(), 0); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
