package server

import (
	"context"
	"fmt"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// StartupGate is an optional admission prerequisite. Implementations must
// complete durable recovery before returning nil. A nil gate preserves the
// existing SQLite server behavior.
type StartupGate interface {
	Recover(context.Context) error
	Ready() bool
}

// StartupGateStatus is a stable, read-only view for authenticated readiness
// detail and metrics adapters. It intentionally does not alter the anonymous
// /ready response contract.
type StartupGateStatus struct {
	Configured bool
	Ready      bool
	State      string
	LastSeq    int64
	Error      string
}

// StartupGateStatusReader is optional so existing test and deployment gates
// only implementing Recover/Ready remain source-compatible.
type StartupGateStatusReader interface {
	Status() StartupGateStatus
}

// LeaseStartupGate adapts the lease package's recovery coordinator without
// making Server own WAL/database wiring. It is intentionally explicit and does
// not change New's default construction.
type LeaseStartupGate struct {
	Coordinator *lease.StartupCoordinator
	RecoverFn   func(context.Context) (int64, error)
}

func (g LeaseStartupGate) Recover(ctx context.Context) error {
	if g.Coordinator == nil {
		return fmt.Errorf("dhcp: startup coordinator is nil")
	}
	return g.Coordinator.Recover(ctx, g.RecoverFn)
}

func (g LeaseStartupGate) Ready() bool {
	return g.Coordinator != nil && g.Coordinator.Ready()
}

func (g LeaseStartupGate) Status() StartupGateStatus {
	status := StartupGateStatus{Configured: true, Ready: g.Ready()}
	if g.Coordinator == nil {
		status.State = "failed"
		status.Error = "startup coordinator is nil"
		return status
	}
	status.State = string(g.Coordinator.State())
	status.LastSeq = g.Coordinator.LastSequence()
	if err := g.Coordinator.Err(); err != nil {
		status.Error = err.Error()
	}
	return status
}

func RequireStartupReady(gate StartupGate) error {
	if gate == nil {
		return nil
	}
	if !gate.Ready() {
		return fmt.Errorf("dhcp: startup recovery is not ready")
	}
	return nil
}

// StartupStatus returns the current optional recovery-gate state without
// performing I/O. Unconfigured servers report a ready-compatible status so
// existing SQLite deployments are not downgraded by the observer itself.
func (s *Server) StartupStatus() StartupGateStatus {
	if s == nil {
		return StartupGateStatus{State: "failed", Error: "server is nil"}
	}
	s.admissionMu.Lock()
	gate := s.startupGate
	s.admissionMu.Unlock()
	if gate == nil {
		return StartupGateStatus{Ready: true, State: "unconfigured"}
	}
	if reader, ok := gate.(StartupGateStatusReader); ok {
		return reader.Status()
	}
	return StartupGateStatus{Configured: true, Ready: gate.Ready(), State: "custom"}
}
