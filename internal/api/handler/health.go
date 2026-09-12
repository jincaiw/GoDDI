package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Readiness levels. They are declared here rather than borrowed from the data
// plane so that the API layer can describe an answer without importing the
// store machinery, and so that the JSON the probes serve has one definition.
const (
	// LevelOK is everything the process is accountable for being current.
	LevelOK = "ok"
	// LevelDegraded is still doing the job, with a caveat an operator has to
	// see. It is not "stop sending traffic"; see Ready.
	LevelDegraded = "degraded"
	// LevelFailing is unable to do the job the process was started for.
	LevelFailing = "failing"
)

// PlaneStatus is one plane's tiered status, as reported by the process that
// runs it.
type PlaneStatus struct {
	// Plane names what was checked: "control", "dns" or "dhcp".
	Plane string `json:"plane"`
	// Level is one of the Level constants.
	Level string `json:"level"`
	// Reasons are coarse, stable codes naming why the level is not ok.
	Reasons []string `json:"reasons,omitempty"`
	// Details carries the numbers. It is served on the authenticated
	// endpoint only; the unauthenticated probe drops it.
	Details any `json:"details,omitempty"`
}

// PlaneProbe answers the readiness question for one plane.
//
// It runs on an unauthenticated request path, so it must not block: a probe
// that takes the store's single connection on every call is a way to starve
// the request path it is supposed to be reporting on.
type PlaneProbe func() PlaneStatus

var (
	probeMu     sync.RWMutex
	planeOrder  []string
	planeProbes = map[string]PlaneProbe{}
)

// SetPlaneProbe registers a plane's readiness probe, replacing any previous
// one. Registration order decides the order statuses are reported in.
//
// Passing a nil probe removes the plane. A process that registers nothing is
// reported as failing rather than ok: an answer that says "healthy" because
// nothing was checked is the failure mode this whole package exists to avoid.
func SetPlaneProbe(plane string, fn PlaneProbe) {
	probeMu.Lock()
	defer probeMu.Unlock()

	if fn == nil {
		if _, ok := planeProbes[plane]; !ok {
			return
		}
		delete(planeProbes, plane)
		for i, p := range planeOrder {
			if p == plane {
				planeOrder = append(planeOrder[:i], planeOrder[i+1:]...)
				break
			}
		}
		return
	}

	if _, ok := planeProbes[plane]; !ok {
		planeOrder = append(planeOrder, plane)
	}
	planeProbes[plane] = fn
}

// ResetPlaneProbes removes every registered probe. It exists for tests that
// must not inherit another test's wiring.
func ResetPlaneProbes() {
	probeMu.Lock()
	defer probeMu.Unlock()
	planeOrder = nil
	planeProbes = map[string]PlaneProbe{}
}

// readiness reduces every registered plane to the worst level, plus the
// per-plane statuses in registration order.
func readiness() (string, []PlaneStatus) {
	probeMu.RLock()
	defer probeMu.RUnlock()

	if len(planeOrder) == 0 {
		return LevelFailing, []PlaneStatus{{
			Plane:   "probes",
			Level:   LevelFailing,
			Reasons: []string{"no_probes_registered"},
		}}
	}

	level := LevelOK
	out := make([]PlaneStatus, 0, len(planeOrder))
	for _, name := range planeOrder {
		fn := planeProbes[name]
		if fn == nil {
			continue
		}
		st := fn()
		st.Plane = name
		st.Level = normalize(st.Level)
		if rank(st.Level) > rank(level) {
			level = st.Level
		}
		out = append(out, st)
	}
	return level, out
}

// normalize maps a reported level onto one of the three this package knows.
//
// An unrecognised value becomes failing. Letting it through would be worse
// than it sounds: Ready decides 503 by comparing the aggregate against the
// failing constant, so an unknown string would take the 200 branch and report
// a state nobody can classify as healthy.
func normalize(level string) string {
	switch level {
	case LevelOK, LevelDegraded, LevelFailing:
		return level
	default:
		return LevelFailing
	}
}

// rank orders levels so several planes can be reduced to the worst one. An
// unknown level ranks as failing: a probe that reports something this package
// cannot classify must not be read as healthy.
func rank(level string) int {
	switch level {
	case LevelOK:
		return 0
	case LevelDegraded:
		return 1
	default:
		return 2
	}
}

// Worse returns the more severe of two levels.
//
// It exists so that a caller raising a plane's level uses the same ordering
// readiness() reduces planes with. A second copy of that ordering is how a
// level ends up sorting one way inside the aggregate and the other way in a
// caller -- and an unknown level, which is the case both functions have to get
// right, is exactly where the two would disagree.
func Worse(a, b string) string {
	if rank(a) >= rank(b) {
		return normalize(a)
	}
	return normalize(b)
}

// Health is the liveness endpoint.
//
// It answers whether the process is running, and nothing else. It used to
// answer 503 when a subsystem was degraded, which is backwards for a liveness
// probe: an orchestrator restarts an unhealthy container, so that behaviour
// asked for a restart of the one process still serving its local copy while
// the control plane was away -- the exact outage this split removes. The
// listener-failed-to-start case is no longer lost by dropping it here; it is
// reported by /ready as failing.
func Health(w http.ResponseWriter, r *http.Request) {
	writeStatus(w, http.StatusOK, map[string]any{"status": LevelOK})
}

// Ready is the readiness endpoint.
//
// It is unauthenticated, because a probe that needs a credential is a probe
// an orchestrator cannot use, and it is deliberately coarse: the level, and
// nothing else. Which plane is degraded, why, and against what number is
// reconnaissance -- /metrics is authenticated for the same reason -- and is
// served by DataPlaneStatus to an authenticated caller.
//
// Only failing is a 503. Degraded stays 200 on purpose: a data plane serving
// its local copy is doing its job, and removing it from service would turn
// the caveat it is reporting into the outage it is warning about. Alerting
// reads the body; load balancing reads the status code.
func Ready(w http.ResponseWriter, r *http.Request) {
	level, _ := readiness()
	code := http.StatusOK
	if level == LevelFailing {
		code = http.StatusServiceUnavailable
	}
	writeStatus(w, code, map[string]any{"status": level})
}

// DataPlaneStatus serves the detail behind Ready to an authenticated caller:
// the worst level, every plane that contributed to it, and the numbers.
func DataPlaneStatus(w http.ResponseWriter, r *http.Request) {
	level, planes := readiness()
	writeStatus(w, http.StatusOK, map[string]any{"status": level, "planes": planes})
}

// CachedProbe wraps a probe that costs something, so the cost is paid on a
// timer rather than on the unauthenticated request path.
//
// The first caller after the cache expires performs the check; everyone else
// reads the stored answer. Under a flood of probes that means one check per
// interval, not one per request.
type CachedProbe struct {
	fn       PlaneProbe
	interval time.Duration

	mu     sync.Mutex
	status PlaneStatus
	at     time.Time
}

// NewCachedProbe builds a probe that re-evaluates fn at most once per
// interval.
func NewCachedProbe(interval time.Duration, fn PlaneProbe) *CachedProbe {
	return &CachedProbe{fn: fn, interval: interval}
}

// Probe implements PlaneProbe.
func (c *CachedProbe) Probe() PlaneStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.at.IsZero() && time.Since(c.at) < c.interval {
		return c.status
	}
	c.status = c.fn()
	c.at = time.Now()
	return c.status
}

func writeStatus(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
