package main

import (
	"testing"

	"github.com/jasonwa/goddi/internal/api/handler"
	dhcpserver "github.com/jasonwa/goddi/internal/dhcp/server"
)

func TestMergeStartupStatusKeepsUnconfiguredDHCPReady(t *testing.T) {
	status := handler.PlaneStatus{Level: handler.LevelOK}
	mergeStartupStatus(&status, dhcpserver.StartupGateStatus{
		State: "unconfigured",
		Ready: true,
	})

	if status.Level != handler.LevelOK {
		t.Fatalf("level = %q, want %q", status.Level, handler.LevelOK)
	}
	if len(status.Reasons) != 0 {
		t.Fatalf("reasons = %v, want none", status.Reasons)
	}
	assertStartupDetails(t, status.Details, false, true, "unconfigured", 0)
}

func TestMergeStartupStatusReportsRecoveryFailureWithoutErrorDetails(t *testing.T) {
	status := handler.PlaneStatus{Level: handler.LevelOK}
	mergeStartupStatus(&status, dhcpserver.StartupGateStatus{
		Configured: true,
		Ready:      false,
		State:      "failed",
		LastSeq:    41,
		Error:      "open /secret/lease.wal: permission denied",
	})

	if status.Level != handler.LevelFailing {
		t.Fatalf("level = %q, want %q", status.Level, handler.LevelFailing)
	}
	if len(status.Reasons) != 1 || status.Reasons[0] != "startup_recovery_failed" {
		t.Fatalf("reasons = %v, want [startup_recovery_failed]", status.Reasons)
	}
	assertStartupDetails(t, status.Details, true, false, "failed", 41)
	if details := status.Details.(map[string]any); details["error"] != nil || details["startup_recovery_error"] != nil {
		t.Fatalf("startup error leaked into details: %v", details)
	}
}

func TestMergeStartupStatusReportsRecoveryNotReady(t *testing.T) {
	status := handler.PlaneStatus{Level: handler.LevelDegraded}
	mergeStartupStatus(&status, dhcpserver.StartupGateStatus{
		Configured: true,
		Ready:      false,
		State:      "recovering",
		LastSeq:    7,
	})

	if status.Level != handler.LevelFailing {
		t.Fatalf("level = %q, want %q", status.Level, handler.LevelFailing)
	}
	if len(status.Reasons) != 1 || status.Reasons[0] != "startup_recovery_not_ready" {
		t.Fatalf("reasons = %v, want [startup_recovery_not_ready]", status.Reasons)
	}
	assertStartupDetails(t, status.Details, true, false, "recovering", 7)
}

func TestMergeStartupStatusPreservesExistingDetails(t *testing.T) {
	status := handler.PlaneStatus{
		Level:   handler.LevelOK,
		Details: map[string]any{"existing": "value"},
	}
	mergeStartupStatus(&status, dhcpserver.StartupGateStatus{
		Configured: true,
		Ready:      true,
		State:      "ready",
		LastSeq:    9,
	})

	details, ok := status.Details.(map[string]any)
	if !ok {
		t.Fatalf("details type = %T, want map[string]any", status.Details)
	}
	if details["existing"] != "value" {
		t.Fatalf("existing detail was lost: %v", details)
	}
	assertStartupDetails(t, details, true, true, "ready", 9)
}

func assertStartupDetails(t *testing.T, raw any, configured, ready bool, state string, lastSeq int64) {
	t.Helper()
	details, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("details type = %T, want map[string]any", raw)
	}
	checks := map[string]any{
		"startup_recovery_configured": configured,
		"startup_recovery_ready":      ready,
		"startup_recovery_state":      state,
		"startup_recovery_last_seq":   lastSeq,
	}
	for key, want := range checks {
		if got := details[key]; got != want {
			t.Errorf("details[%q] = %#v, want %#v", key, got, want)
		}
	}
}
