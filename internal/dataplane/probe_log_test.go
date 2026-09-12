package dataplane

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// The readiness line is the only place a level change reaches an operator who
// is not scraping. /ready answers a probe, and /metrics is a number; both are
// pull-shaped, so a level that changes while nobody is looking leaves no trace
// in the log. That is why refreshProbe logs on change -- and why the line has
// to be pinned: a log statement is exactly the kind of thing that keeps
// compiling after it stops being reached.
//
// The rules asserted here are the ones the comment on refreshProbe claims:
// a change is logged, a repeat is not, and the line says both what the level
// became and what it was.
func TestALevelChangeIsLoggedAndARepeatedOneIsNot(t *testing.T) {
	control, _, rep := newPair(t)
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}

	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	capture := captureLogs(t)

	// 1. The first pass is a change by construction: there was no previous
	//    level, and a process whose readiness is unknown is not ok.
	runner.recordSuccess()
	runner.refreshProbe()

	records := capture.all()
	if len(records) != 1 {
		t.Fatalf("first pass logged %d lines, want 1: a level that has just been "+
			"established is news", len(records))
	}
	first := records[0]
	if first.Level != slog.LevelInfo {
		t.Errorf("first line has level %v, want %v: it reports ok", first.Level, slog.LevelInfo)
	}
	if !strings.Contains(first.Message, "readiness ok") {
		t.Errorf("first line message = %q, want the ok message the restore drill "+
			"greps for", first.Message)
	}
	if got := attr(first, "readiness"); got != string(LevelOK) {
		t.Errorf("first line readiness = %q, want %q", got, LevelOK)
	}

	// 2. The same level again says nothing. A line per pass is a line nobody
	//    reads, and at one pass per second there would be 86400 of them a day.
	runner.refreshProbe()
	if got := len(capture.all()); got != 1 {
		t.Fatalf("a repeated pass logged %d lines, want none: an unchanged level "+
			"must stay quiet", got-1)
	}

	// 3. The level drops. Exactly one new line, and it is a warning.
	runner.recordFailure(errors.New("control database is unreachable"))
	runner.refreshProbe()

	records = capture.all()
	if len(records) != 2 {
		t.Fatalf("the level change logged %d lines in total, want 2", len(records))
	}
	second := records[1]
	if second.Level != slog.LevelWarn {
		t.Errorf("the change to %q was logged at %v, want %v: a level below ok is "+
			"what an operator has to see", LevelDegraded, second.Level, slog.LevelWarn)
	}
	if got := attr(second, "readiness"); got != string(LevelDegraded) {
		t.Errorf("readiness = %q, want %q", got, LevelDegraded)
	}
	if got := attr(second, "previous"); got != string(LevelOK) {
		t.Errorf("previous = %q, want %q: a line that does not say what it changed "+
			"from cannot be read as a transition", got, LevelOK)
	}
	if got := attr(second, "reasons"); !strings.Contains(got, "control_unreachable") {
		t.Errorf("reasons = %q, want control_unreachable: the level alone does not "+
			"say what to look at", got)
	}

	// 4. Still degraded: still quiet.
	runner.refreshProbe()
	if got := len(capture.all()); got != 2 {
		t.Fatalf("a repeated degraded pass logged %d extra lines, want none", got-2)
	}

	// 5. And back up again, which is the transition an operator most needs to
	//    see: the caveat has cleared.
	runner.recordSuccess()
	runner.refreshProbe()

	records = capture.all()
	if len(records) != 3 {
		t.Fatalf("the recovery logged %d lines in total, want 3", len(records))
	}
	third := records[2]
	if third.Level != slog.LevelInfo {
		t.Errorf("the recovery was logged at %v, want %v", third.Level, slog.LevelInfo)
	}
	if got := attr(third, "readiness"); got != string(LevelOK) {
		t.Errorf("recovery readiness = %q, want %q", got, LevelOK)
	}
	if got := attr(third, "previous"); got != string(LevelDegraded) {
		t.Errorf("recovery previous = %q, want %q", got, LevelDegraded)
	}
	runner.refreshProbe()
	if got := len(capture.all()); got != 3 {
		t.Fatalf("a repeated ok pass logged %d extra lines, want none", got-3)
	}
}

// The attribute that carries the level must not be called `level`.
//
// The logger is a JSON handler, which writes its own `level` key for the
// severity. Two keys with one name in a JSON object is not an error any parser
// reports -- most keep the last one -- so the record's severity would read as
// "degraded" and the warning would arrive looking like a status field. Log
// pipelines filter on that field.
func TestTheReadinessLineDoesNotShadowTheSeverity(t *testing.T) {
	control, _, rep := newPair(t)
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}

	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	capture := captureLogs(t)

	runner.recordFailure(errors.New("control database is unreachable"))
	runner.refreshProbe()

	records := capture.all()
	if len(records) != 1 {
		t.Fatalf("logged %d lines, want 1", len(records))
	}
	keys := attrKeys(records[0])
	for _, key := range keys {
		if key == "level" {
			t.Fatalf("the readiness line carries an attribute named %q, which shadows "+
				"the severity the JSON handler writes under the same name; keys = %v",
				key, keys)
		}
	}
	// The severity is still reachable as the record's own level, which is what
	// the handler serialises into that key.
	if records[0].Level != slog.LevelWarn {
		t.Errorf("severity = %v, want %v", records[0].Level, slog.LevelWarn)
	}
}

// captureLogs installs a handler that keeps the records instead of formatting
// them, so the assertions read the level and the attributes rather than the
// text a handler happens to produce.
type logCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	capture := &logCapture{}
	previous := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return capture
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }

func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r.Clone())
	return nil
}

func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }

func (c *logCapture) all() []slog.Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]slog.Record(nil), c.records...)
}

func attr(r slog.Record, key string) string {
	var out string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			out = a.Value.String()
			return false
		}
		return true
	})
	return out
}

func attrKeys(r slog.Record) []string {
	var out []string
	r.Attrs(func(a slog.Attr) bool {
		out = append(out, a.Key)
		return true
	})
	return out
}
