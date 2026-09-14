package server

import (
	"context"
	"errors"
	"testing"
)

func TestServerStartRunsStartupGateBeforeSocketBinding(t *testing.T) {
	s, db := newDHCPTestServer(t)
	defer db.Close()
	probe := &startupGateProbe{ready: true}
	s.SetStartupGate(probe)

	// The test environment intentionally supplies no usable interface. The
	// gate must nevertheless run before Start reaches interface/socket setup.
	err := s.Start(context.Background())
	if probe.calls != 1 {
		t.Fatalf("startup gate calls = %d, want 1", probe.calls)
	}
	if err == nil {
		t.Fatal("Start unexpectedly succeeded without a listening interface")
	}
}

func TestServerStartFailsClosedWhenStartupRecoveryFails(t *testing.T) {
	s, db := newDHCPTestServer(t)
	defer db.Close()
	want := errors.New("wal recovery failed")
	probe := &startupGateProbe{recoverErr: want}
	s.SetStartupGate(probe)

	if err := s.Start(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Start error = %v, want %v", err, want)
	}
	if probe.calls != 1 {
		t.Fatalf("startup gate calls = %d, want 1", probe.calls)
	}
	if len(s.listeners) != 0 {
		t.Fatalf("listeners = %d, want 0 after failed recovery", len(s.listeners))
	}
}
