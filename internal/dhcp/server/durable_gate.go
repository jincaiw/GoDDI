package server

import (
	"context"
	"fmt"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// DurableLeaseGate is the final ACK prerequisite for the future WAL-backed
// store. It is intentionally independent of the current SQLite Manager: the
// current production path remains unchanged until a real implementation is
// injected.
type DurableLeaseGate interface {
	Durable(ctx context.Context, value *lease.Lease) error
}

// RequireDurable returns an ACK-safe error when the durable boundary cannot be
// confirmed. A nil gate is accepted for the existing SQLite-backed path, where
// the primary write has already completed before this hook is reached.
func RequireDurable(ctx context.Context, gate DurableLeaseGate, value *lease.Lease) error {
	if gate == nil {
		return nil
	}
	if value == nil {
		return fmt.Errorf("dhcp: durable gate requires a lease")
	}
	if err := gate.Durable(ctx, value); err != nil {
		return fmt.Errorf("dhcp: lease durable boundary: %w", err)
	}
	return nil
}
