package server

import (
	"context"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

type durableGateProbe struct{ err error }

func (g durableGateProbe) Durable(context.Context, *lease.Lease) error { return g.err }

func TestRequireDurableAllowsExistingPathWithoutGate(t *testing.T) {
	if err := RequireDurable(context.Background(), nil, nil); err != nil {
		t.Fatalf("nil gate error = %v", err)
	}
}

func TestRequireDurableFailsClosedWhenBoundaryFails(t *testing.T) {
	want := errors.New("fsync failed")
	value := &lease.Lease{ID: "l1"}
	if err := RequireDurable(context.Background(), durableGateProbe{err: want}, value); !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped fsync error", err)
	}
}

func TestRequireDurableRejectsNilLeaseWithConfiguredGate(t *testing.T) {
	if err := RequireDurable(context.Background(), durableGateProbe{}, nil); err == nil {
		t.Fatal("nil lease passed configured durable gate")
	}
}
