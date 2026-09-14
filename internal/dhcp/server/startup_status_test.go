package server

import "testing"

func TestServerStartupStatusIsReadyCompatibleWhenGateUnconfigured(t *testing.T) {
	s, db := newDHCPTestServer(t)
	defer db.Close()
	status := s.StartupStatus()
	if status.Configured || !status.Ready || status.State != "unconfigured" {
		t.Fatalf("status = %+v", status)
	}
}

func TestServerStartupStatusReportsConfiguredGate(t *testing.T) {
	s, db := newDHCPTestServer(t)
	defer db.Close()
	probe := &startupGateProbe{}
	s.SetStartupGate(probe)
	status := s.StartupStatus()
	if !status.Configured || status.Ready || status.State != "custom" {
		t.Fatalf("status = %+v", status)
	}
}
