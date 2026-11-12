package facts

import (
	"context"
	"testing"
)

// TestStampedOutboxProducesVerifiableChain proves the opt-in producer stamps a
// verifiable hash chain and that replaying the same event is idempotent (does
// not advance the chain or fail as a conflict).
func TestStampedOutboxProducesVerifiableChain(t *testing.T) {
	db := newFactsOutboxDB(t)
	ob, err := NewObservationOutboxStamped(db, "dhcp_lease_facts")
	if err != nil {
		t.Fatalf("stamped outbox: %v", err)
	}
	ctx := context.Background()
	e1 := mkEnv(t, 1, `{"i":1}`)
	if err := ob.Enqueue(ctx, e1); err != nil {
		t.Fatalf("enqueue 1: %v", err)
	}
	// Replay of the same event is a no-op (payload identical).
	if err := ob.Enqueue(ctx, e1); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if err := ob.Enqueue(ctx, mkEnv(t, 2, `{"i":2}`)); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}

	events, err := ob.After(ctx, 0, 10)
	if err != nil {
		t.Fatalf("after: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	for i, e := range events {
		if !e.HasChain() {
			t.Fatalf("event %d has no chain fields", i)
		}
	}
	if err := VerifyChain(events); err != nil {
		t.Fatalf("chain did not verify: %v", err)
	}
}
