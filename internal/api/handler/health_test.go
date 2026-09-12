package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Liveness, readiness, and the difference between them.
//
// The process used to answer 503 on /health when a subsystem was degraded.
// An orchestrator restarts an unhealthy container, so that behaviour asked
// for a restart of the one process still serving its local copy while the
// control plane was away -- the exact outage the data-plane split removes.
// These tests pin the replacement: liveness never fails on degradation,
// readiness reports it, and only a genuine failure is a 503.

func withProbes(t *testing.T, probes map[string]PlaneProbe) {
	t.Helper()
	ResetPlaneProbes()
	t.Cleanup(ResetPlaneProbes)
	for name, fn := range probes {
		SetPlaneProbe(name, fn)
	}
}

func staticProbe(level string) PlaneProbe {
	return func() PlaneStatus {
		return PlaneStatus{Level: level, Reasons: []string{"reason_for_" + level}}
	}
}

func serve(t *testing.T, fn http.HandlerFunc) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	fn(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON (%q): %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

// TestLivenessIgnoresDegradation: a process whose data plane is degraded is
// still running, and must not be restarted for saying so.
func TestLivenessIgnoresDegradation(t *testing.T) {
	for _, level := range []string{LevelOK, LevelDegraded, LevelFailing} {
		withProbes(t, map[string]PlaneProbe{"dns": staticProbe(level)})

		code, body := serve(t, Health)
		if code != http.StatusOK {
			t.Errorf("with a %s plane, GET /health = %d, want 200: liveness "+
				"answers whether the process runs, not whether it is happy",
				level, code)
		}
		if body["status"] != LevelOK {
			t.Errorf("with a %s plane, /health status = %v, want %q", level, body["status"], LevelOK)
		}
	}
}

// TestReadinessReportsTheWorstPlane: one level per process, so the most
// severe answer has to survive the reduction.
func TestReadinessReportsTheWorstPlane(t *testing.T) {
	cases := []struct {
		name   string
		probes map[string]PlaneProbe
		want   string
		code   int
	}{
		{
			name:   "everything current",
			probes: map[string]PlaneProbe{"control": staticProbe(LevelOK), "dhcp": staticProbe(LevelOK)},
			want:   LevelOK,
			code:   http.StatusOK,
		},
		{
			name:   "one plane degraded",
			probes: map[string]PlaneProbe{"control": staticProbe(LevelOK), "dhcp": staticProbe(LevelDegraded)},
			want:   LevelDegraded,
			// 200 on purpose: a data plane serving its local copy is doing
			// its job, and evicting it would turn the caveat into the outage.
			code: http.StatusOK,
		},
		{
			name:   "one plane failing",
			probes: map[string]PlaneProbe{"control": staticProbe(LevelOK), "dhcp": staticProbe(LevelFailing)},
			want:   LevelFailing,
			code:   http.StatusServiceUnavailable,
		},
		{
			name:   "a failing plane outranks a degraded one",
			probes: map[string]PlaneProbe{"control": staticProbe(LevelDegraded), "dns": staticProbe(LevelFailing)},
			want:   LevelFailing,
			code:   http.StatusServiceUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withProbes(t, tc.probes)
			code, body := serve(t, Ready)
			if code != tc.code {
				t.Errorf("GET /ready = %d, want %d", code, tc.code)
			}
			if body["status"] != tc.want {
				t.Errorf("status = %v, want %q", body["status"], tc.want)
			}
		})
	}
}

// TestReadinessServesOnlyTheLevel: the endpoint is unauthenticated, so which
// plane is degraded, why, and against what figure is reconnaissance and must
// not be here. /metrics is authenticated for the same reason.
func TestReadinessServesOnlyTheLevel(t *testing.T) {
	withProbes(t, map[string]PlaneProbe{
		"dhcp": func() PlaneStatus {
			return PlaneStatus{
				Level:   LevelDegraded,
				Reasons: []string{"control_unreachable", "quota_exceeded"},
				Details: map[string]any{"held_leases": 12345},
			}
		},
	})

	_, body := serve(t, Ready)
	if len(body) != 1 {
		t.Fatalf("body = %v, want exactly one field (status)", body)
	}
	if _, leaked := body["planes"]; leaked {
		t.Error("the unauthenticated probe served the per-plane detail")
	}
	if _, leaked := body["reasons"]; leaked {
		t.Error("the unauthenticated probe served the reasons")
	}
}

// TestAProcessWithNoProbesIsNotReady: an answer of "healthy" because nothing
// was checked is the failure mode this package exists to avoid -- it is the
// same shape as a DHCP server with no scopes looking fine.
func TestAProcessWithNoProbesIsNotReady(t *testing.T) {
	withProbes(t, nil)

	code, body := serve(t, Ready)
	if code != http.StatusServiceUnavailable {
		t.Errorf("GET /ready with no probes = %d, want 503", code)
	}
	if body["status"] != LevelFailing {
		t.Errorf("status = %v, want %q", body["status"], LevelFailing)
	}
}

// TestAnUnclassifiableLevelIsTreatedAsFailing: a probe that reports something
// this package does not know must not be read as healthy by a comparison that
// silently falls through.
func TestAnUnclassifiableLevelIsTreatedAsFailing(t *testing.T) {
	withProbes(t, map[string]PlaneProbe{"dns": staticProbe("probably-fine")})

	code, _ := serve(t, Ready)
	if code != http.StatusServiceUnavailable {
		t.Errorf("GET /ready = %d for an unknown level, want 503", code)
	}
}

// TestTheDetailEndpointCarriesTheNumbers is the other half of the split: the
// figures the probe withholds are served, in full, to an authenticated caller.
func TestTheDetailEndpointCarriesTheNumbers(t *testing.T) {
	withProbes(t, map[string]PlaneProbe{
		"dhcp": func() PlaneStatus {
			return PlaneStatus{
				Level:   LevelDegraded,
				Reasons: []string{"control_unreachable"},
				Details: map[string]any{"held_leases": float64(12345)},
			}
		},
	})

	code, body := serve(t, DataPlaneStatus)
	if code != http.StatusOK {
		t.Fatalf("GET detail = %d, want 200: the detail endpoint reports, it "+
			"does not gate", code)
	}
	if body["status"] != LevelDegraded {
		t.Errorf("status = %v, want %q", body["status"], LevelDegraded)
	}
	planes, ok := body["planes"].([]any)
	if !ok || len(planes) != 1 {
		t.Fatalf("planes = %v, want one entry", body["planes"])
	}
	entry, _ := planes[0].(map[string]any)
	if entry["plane"] != "dhcp" {
		t.Errorf("plane = %v, want dhcp", entry["plane"])
	}
	if entry["reasons"] == nil {
		t.Error("the detail endpoint dropped the reasons")
	}
	if entry["details"] == nil {
		t.Error("the detail endpoint dropped the numbers")
	}
}

// TestCachedProbeEvaluatesAtMostOncePerInterval: /ready is unauthenticated,
// so a probe that costs something has to pay for it on a timer rather than
// once per request. Without this, a flood of probes takes the store's single
// connection away from the request path it is reporting on.
func TestCachedProbeEvaluatesAtMostOncePerInterval(t *testing.T) {
	var calls atomic.Int64
	cached := NewCachedProbe(time.Hour, func() PlaneStatus {
		calls.Add(1)
		return PlaneStatus{Level: LevelOK}
	})

	for i := 0; i < 50; i++ {
		if got := cached.Probe(); got.Level != LevelOK {
			t.Fatalf("call %d returned %q, want %q", i, got.Level, LevelOK)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("the probe ran %d times across 50 calls, want 1: it is meant to "+
			"be paid for on a timer, not per request", n)
	}
}

// TestCachedProbeReEvaluatesOnceTheIntervalPasses: the cache must expire, or
// a process that recovered would keep reporting the failure forever.
func TestCachedProbeReEvaluatesOnceTheIntervalPasses(t *testing.T) {
	var calls atomic.Int64
	cached := NewCachedProbe(0, func() PlaneStatus {
		calls.Add(1)
		return PlaneStatus{Level: LevelOK}
	})

	cached.Probe()
	cached.Probe()
	if n := calls.Load(); n != 2 {
		t.Errorf("the probe ran %d times with a zero interval, want 2: a zero "+
			"interval means no caching", n)
	}
}
