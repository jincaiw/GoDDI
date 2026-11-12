package timeguard

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/timewater"
	"github.com/jasonwa/goddi/internal/enterprise/trustedtime"
)

type clock struct{ n atomic.Int64 }

func (c *clock) Now() int64 { return c.n.Load() }

type water struct {
	mu                 sync.Mutex
	high               int64
	readErr, errorNext error
	calls              int
}

func (w *water) Read() (int64, error) { w.mu.Lock(); defer w.mu.Unlock(); return w.high, w.readErr }
func (w *water) Advance(n int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	if w.errorNext != nil {
		return w.errorNext
	}
	if n < w.high {
		return timewater.ErrRollback
	}
	w.high = n
	return nil
}
func config() trustedtime.Config {
	return trustedtime.Config{MaxUncertainty: 10, MaxStep: 2, MaxHoldover: 20, StableWindow: 10, MaxSampleInterval: 10}
}
func newGuard(t *testing.T, w *water) (*Guard, *clock) {
	t.Helper()
	c := &clock{}
	g, e := New(c, config(), w)
	if e != nil {
		t.Fatal(e)
	}
	return g, c
}
func sample(g *Guard, c *clock, mono, utc int64) error {
	c.n.Store(mono)
	return g.Observe(&trustedtime.Sample{UTC: utc, Uncertainty: 1})
}
func healthy(t *testing.T, g *Guard, c *clock) {
	t.Helper()
	if e := sample(g, c, 0, 100); !errors.Is(e, trustedtime.ErrUnhealthy) {
		t.Fatal(e)
	}
	if e := sample(g, c, 10, 110); e != nil {
		t.Fatal(e)
	}
}
func TestInitializationAndWindowPersists(t *testing.T) {
	w := &water{readErr: os.ErrNotExist}
	if g, e := New(&clock{}, config(), w); g != nil || !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("%v %v", g, e)
	}
	w = &water{high: 90}
	g, c := newGuard(t, w)
	if e := sample(g, c, 0, 100); !errors.Is(e, trustedtime.ErrUnhealthy) {
		t.Fatal(e)
	}
	if w.high != 99 {
		t.Fatalf("first accepted sample not persisted: %d", w.high)
	}
	if b, e := g.Current(); !errors.Is(e, trustedtime.ErrUnhealthy) || b != (trustedtime.Bounds{}) {
		t.Fatalf("%v %v", b, e)
	}
	if e := sample(g, c, 10, 110); e != nil || w.high != 109 {
		t.Fatalf("%d %v", w.high, e)
	}
	c.n.Store(12)
	b, e := g.Current()
	if e != nil || b.Lower != 111 || w.high != 111 {
		t.Fatalf("bounds=%v high=%d err=%v", b, w.high, e)
	}
}
func TestFaultLocksAndNoBoundsLeak(t *testing.T) {
	for _, failure := range []error{errors.New("disk"), timewater.ErrIndeterminate, timewater.ErrRollback} {
		t.Run(failure.Error(), func(t *testing.T) {
			w := &water{high: 90}
			g, c := newGuard(t, w)
			healthy(t, g, c)
			w.errorNext = failure
			c.n.Store(11)
			b, e := g.Current()
			if b != (trustedtime.Bounds{}) || !errors.Is(e, ErrLocked) || !errors.Is(e, failure) {
				t.Fatalf("%v %v", b, e)
			}
			calls := w.calls
			w.errorNext = nil
			if e := sample(g, c, 12, 112); !errors.Is(e, ErrLocked) {
				t.Fatal(e)
			}
			if w.calls != calls {
				t.Fatal("locked guard touched store")
			}
			if b, e := g.Current(); b != (trustedtime.Bounds{}) || !errors.Is(e, ErrLocked) {
				t.Fatalf("%v %v", b, e)
			}
			g.Close()
			if _, e := g.Current(); !errors.Is(e, ErrClosed) {
				t.Fatal(e)
			}
		})
	}
	w := &water{high: 90, errorNext: timewater.ErrIndeterminate}
	g, c := newGuard(t, w)
	if e := sample(g, c, 0, 100); !errors.Is(e, ErrLocked) {
		t.Fatal(e)
	}
}
func TestRestartHoldoverAndRollback(t *testing.T) {
	w := &water{high: 90}
	g, c := newGuard(t, w)
	healthy(t, g, c)
	c.n.Store(25)
	if _, e := g.Current(); e != nil {
		t.Fatal(e)
	}
	g.Close()
	g, c = newGuard(t, w)
	if e := sample(g, c, 0, 100); !errors.Is(e, trustedtime.ErrUnhealthy) {
		t.Fatal(e)
	}
	if b, e := g.Current(); b != (trustedtime.Bounds{}) || !errors.Is(e, trustedtime.ErrUnhealthy) {
		t.Fatalf("%v %v", b, e)
	}
	if e := sample(g, c, 1, 126); !errors.Is(e, trustedtime.ErrUnhealthy) {
		t.Fatal(e)
	}
	if e := sample(g, c, 11, 136); e != nil {
		t.Fatal(e)
	}
	c.n.Store(31)
	if _, e := g.Current(); e != nil {
		t.Fatal(e)
	}
	c.n.Store(32)
	if b, e := g.Current(); b != (trustedtime.Bounds{}) || !errors.Is(e, trustedtime.ErrUnhealthy) {
		t.Fatalf("holdover %v %v", b, e)
	}
}
func TestConcurrentCurrent(t *testing.T) {
	w := &water{high: 90}
	g, c := newGuard(t, w)
	healthy(t, g, c)
	c.n.Store(11)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := g.Current(); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
}
func TestRealWater(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "time")
	if e = os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	w := timewater.NewStore(dir)
	if g, e := New(&clock{}, config(), w); g != nil || !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("%v %v", g, e)
	}
	if e = w.Bootstrap(90); e != nil {
		t.Fatal(e)
	}
	c := &clock{}
	g, e := New(c, config(), w)
	if e != nil {
		t.Fatal(e)
	}
	healthy(t, g, c)
	if n, e := timewater.NewStore(dir).Read(); e != nil || n != 109 {
		t.Fatalf("%d %v", n, e)
	}
	g.Close()
	c = &clock{}
	g, e = New(c, config(), timewater.NewStore(dir))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = g.Current(); !errors.Is(e, trustedtime.ErrUnhealthy) {
		t.Fatal(e)
	}
}
