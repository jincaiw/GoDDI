package lease

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAsyncApplierWaitWaitsForInFlightApply pins the drain contract that the
// WAL pipeline depends on: Wait must not return "idle" while an accepted event
// is still being applied. The old implementation judged idleness from
// len(queue)==0 && !inflight, which had a window between the worker receiving
// an event and marking it in flight -- under load that let a drain report
// success while the event was still being applied. The accounting is now based
// on accepted/applied sequence numbers.
func TestAsyncApplierWaitWaitsForInFlightApply(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	applier, err := NewAsyncApplier(context.Background(), 4, 0, func(context.Context, WALEvent) error {
		close(started)
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()

	if err := applier.Submit(WALEvent{Version: currentWALEventVersion, Seq: 1, Op: WALEventRemove, LeaseID: "l1"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-started

	// The event is being applied and cannot finish until we release it. A
	// bounded Wait must therefore time out rather than report an empty queue.
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if err := applier.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait during in-flight apply = %v, want context deadline exceeded", err)
	}

	close(release)
	if err := applier.Wait(context.Background()); err != nil {
		t.Fatalf("Wait after apply finished = %v, want nil", err)
	}
	if got := applier.Applied(); got != 1 {
		t.Fatalf("applied = %d, want 1", got)
	}
}
