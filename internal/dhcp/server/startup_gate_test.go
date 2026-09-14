package server

import (
	"context"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

type startupGateProbe struct {
	recoverErr error
	ready      bool
	calls      int
}

func (p *startupGateProbe) Recover(context.Context) error {
	p.calls++
	return p.recoverErr
}

func (p *startupGateProbe) Ready() bool { return p.ready }

func TestRequireStartupReadyAllowsUnconfiguredServer(t *testing.T) {
	if err := RequireStartupReady(nil); err != nil {
		t.Fatal(err)
	}
}

func TestRequireStartupReadyRejectsUnreadyGate(t *testing.T) {
	if err := RequireStartupReady(&startupGateProbe{}); err == nil {
		t.Fatal("unready gate unexpectedly accepted")
	}
	if err := RequireStartupReady(&startupGateProbe{ready: true}); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseStartupGateAdaptsCoordinator(t *testing.T) {
	coordinator := lease.NewStartupCoordinator()
	gate := LeaseStartupGate{
		Coordinator: coordinator,
		RecoverFn:   func(context.Context) (int64, error) { return 4, nil },
	}
	if err := gate.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !gate.Ready() || coordinator.LastSequence() != 4 {
		t.Fatalf("ready=%v sequence=%d", gate.Ready(), coordinator.LastSequence())
	}
	status := gate.Status()
	if !status.Configured || !status.Ready || status.State != "ready" || status.LastSeq != 4 || status.Error != "" {
		t.Fatalf("status = %+v", status)
	}
}

func TestLeaseStartupGateFailsClosedWhenRecoveryFails(t *testing.T) {
	want := errors.New("replay failed")
	gate := LeaseStartupGate{
		Coordinator: lease.NewStartupCoordinator(),
		RecoverFn:   func(context.Context) (int64, error) { return 0, want },
	}
	if err := gate.Recover(context.Background()); !errors.Is(err, lease.ErrStartupNotReady) {
		t.Fatalf("error = %v, want ErrStartupNotReady", err)
	}
	if gate.Ready() {
		t.Fatal("failed recovery became ready")
	}
}
