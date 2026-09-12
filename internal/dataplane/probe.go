package dataplane

import (
	"log/slog"
	"time"
)

// Level is the tiered readiness of a process.
//
// Three levels, not two, because the interesting state of a data plane is not
// "up" or "down": after this batch a data plane is designed to keep serving
// its local copy while the control plane is away. Collapsing that into "down"
// is what makes an orchestrator kill the one process that is still handing
// out addresses.
type Level string

const (
	// LevelOK is everything this process is accountable for being current.
	LevelOK Level = "ok"
	// LevelDegraded is still doing its job, with a caveat an operator has to
	// see: the control database is unreachable, a quota has been crossed, or
	// an outbound queue is not draining.
	//
	// It does not mean "stop sending me traffic". A data plane in this state
	// is answering clients correctly, and taking it out of service would
	// turn a caveat into the outage it is warning about.
	LevelDegraded Level = "degraded"
	// LevelFailing is unable to do the job it was started for: a replica
	// state that cannot be read, a listener that never came up.
	LevelFailing Level = "failing"
)

// rank orders the levels so a process reporting several planes can be reduced
// to the worst one.
func (l Level) rank() int {
	switch l {
	case LevelFailing:
		return 2
	case LevelDegraded:
		return 1
	default:
		return 0
	}
}

// worse returns the more severe of two levels.
func worse(a, b Level) Level {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Readiness is the tiered probe view of one data plane.
type Readiness struct {
	// Level is the worst level across everything this plane reports.
	Level Level
	// Reasons are stable, coarse codes naming why the level is not ok. They
	// are for the authenticated detail endpoint and for log lines; the
	// unauthenticated probe never serves them.
	Reasons []string
	// Status is the underlying probe view, for callers that want the numbers.
	Status Status
	// Quota lists the bounds that are currently crossed.
	Quota []Breach
	// At is when this view was taken. The probe is answered from memory, so
	// the caller can see how stale it is.
	At time.Time
}

// Probe returns the readiness as of the last replication pass.
//
// It reads memory only. That is deliberate: /ready is unauthenticated by
// necessity -- a probe that needs a credential is a probe an orchestrator
// cannot use -- and a readiness endpoint that takes the store's single
// connection on every call is a way to starve the request path it is
// supposed to be reporting on. The numbers are therefore at most one
// interval old, which for a readiness decision is not a distinction that
// matters.
func (r *Runner) Probe() Readiness {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.probed {
		// Asked before the first pass finished. Reporting failing here would
		// be wrong in the other direction -- a process that has just started
		// is not failing -- so report degraded with the reason.
		return Readiness{
			Level:   LevelDegraded,
			Reasons: []string{"no_pass_yet"},
		}
	}
	return r.probe
}

// refreshProbe takes a fresh view and stores it for Probe to serve.
//
// It is called at the end of every pass, from the loop's own goroutine, so
// the store reads it performs are the same ones the pass already does and are
// never taken on a request path.
//
// A change of level is logged, a repeated one is not: the point of the line
// is that something changed, and a line per pass is a line nobody reads.
func (r *Runner) refreshProbe() {
	st := r.Snapshot()
	rd := r.evaluate(st)

	r.mu.Lock()
	var prev Level
	changed := !r.probed || r.probe.Level != rd.Level
	if r.probed {
		prev = r.probe.Level
	}
	r.probe = rd
	r.probed = true
	r.mu.Unlock()

	if !changed {
		return
	}
	// The attribute is named `readiness` rather than `level` because the logger
	// is a JSON handler, and a JSON handler already emits its own `level` key
	// for the severity. Two keys with the same name in one object is not an
	// error any parser reports: most keep the last one, so `level` would read
	// as "degraded" and the record would lose the fact that it is a warning --
	// exactly the field a log pipeline filters on.
	attrs := []any{"readiness", string(rd.Level), "previous", string(prev), "reasons", rd.Reasons}
	for _, b := range rd.Quota {
		attrs = append(attrs, "quota", b.String())
	}
	if rd.Level == LevelOK {
		slog.Info("dataplane: readiness ok", attrs...)
		return
	}
	slog.Warn("dataplane: readiness is below ok; the local copy stays in service", attrs...)
}

// evaluate turns a probe view into a level. It is a pure function so the
// rules can be tested without a store or a clock.
func (r *Runner) evaluate(st Status) Readiness {
	rd := Readiness{
		Level:  LevelOK,
		Status: st,
		At:     time.Now(),
	}

	// A domain whose replica state cannot be read is not a caveat: this
	// process cannot say what it is serving.
	for _, d := range r.cfg.Domains {
		if _, ok := st.Domains[d]; !ok {
			rd.Level = LevelFailing
			rd.Reasons = append(rd.Reasons, "replica_state_unreadable:"+string(d))
		}
	}

	// The control database being away is what this whole batch exists to
	// survive. The local copy is in service, so this is a caveat, not a
	// failure.
	if !st.ControlReachable {
		rd.Level = worse(rd.Level, LevelDegraded)
		rd.Reasons = append(rd.Reasons, "control_unreachable")
	}

	if breaches := r.cfg.Quota.Check(r.rep.store, st); len(breaches) > 0 {
		rd.Level = worse(rd.Level, LevelDegraded)
		rd.Reasons = append(rd.Reasons, "quota_exceeded")
		rd.Quota = breaches
	}

	// Rows the control database has declined. This is a caveat and not a
	// failure: the local copy is serving everything it holds, which is what
	// this process is for. What is wrong is that the console's view is drifting
	// away from the truth, and the row stays queued with a widening retry
	// delay rather than being dropped -- so the condition is real, persistent
	// and worth saying out loud. The usual cause is a downward sync that has
	// not landed yet; the rest are inconsistencies whose repair is an
	// operator's decision, which is exactly what they cannot make if the only
	// sign of it is a number in a metric.
	if st.RefusedRows > 0 {
		rd.Level = worse(rd.Level, LevelDegraded)
		rd.Reasons = append(rd.Reasons, "change_refused")
	}

	return rd
}
