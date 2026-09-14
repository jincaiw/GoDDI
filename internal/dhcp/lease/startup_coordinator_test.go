package lease

import (
	"context"
	"errors"
	"testing"
)

func TestStartupCoordinatorOnlyReadyAfterSuccessfulRecovery(t *testing.T) {
	c := NewStartupCoordinator()
	if c.State() != StartupCold || c.Ready() {
		t.Fatalf("initial state = %q ready=%v", c.State(), c.Ready())
	}
	called := false
	if err := c.Recover(context.Background(), func(context.Context) (int64, error) {
		called = true
		return 7, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called || !c.Ready() || c.State() != StartupReady || c.LastSequence() != 7 {
		t.Fatalf("state=%q ready=%v seq=%d called=%v", c.State(), c.Ready(), c.LastSequence(), called)
	}
	if !errors.Is(c.Recover(context.Background(), func(context.Context) (int64, error) { return 8, nil }), ErrStartupAlreadyReady) {
		t.Fatalf("second recovery did not return ErrStartupAlreadyReady")
	}
}

func TestStartupCoordinatorRecoveryFailureNeverBecomesReady(t *testing.T) {
	c := NewStartupCoordinator()
	want := errors.New("wal corrupt")
	if err := c.Recover(context.Background(), func(context.Context) (int64, error) { return 0, want }); !errors.Is(err, ErrStartupNotReady) {
		t.Fatalf("error = %v, want ErrStartupNotReady", err)
	}
	if c.Ready() || c.State() != StartupFailed || !errors.Is(c.Err(), want) {
		t.Fatalf("state=%q ready=%v err=%v", c.State(), c.Ready(), c.Err())
	}
	if err := c.Recover(context.Background(), func(context.Context) (int64, error) { return 1, nil }); !errors.Is(err, ErrStartupFailed) {
		t.Fatalf("retry error = %v, want ErrStartupFailed", err)
	}
}

func TestStartupCoordinatorCancelledContextStaysUnready(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewStartupCoordinator()
	if err := c.Recover(ctx, func(context.Context) (int64, error) { t.Fatal("recovery called after cancellation"); return 0, nil }); !errors.Is(err, ErrStartupNotReady) {
		t.Fatalf("error = %v, want ErrStartupNotReady", err)
	}
	if c.Ready() || c.State() != StartupFailed {
		t.Fatalf("state=%q ready=%v", c.State(), c.Ready())
	}
}

func TestStartupCoordinatorCloseBlocksRecovery(t *testing.T) {
	c := NewStartupCoordinator()
	c.Close()
	if c.State() != StartupClosed || c.Ready() {
		t.Fatalf("state=%q ready=%v", c.State(), c.Ready())
	}
	if err := c.Recover(context.Background(), func(context.Context) (int64, error) { return 1, nil }); !errors.Is(err, ErrStartupNotReady) {
		t.Fatalf("error = %v, want ErrStartupNotReady", err)
	}
}
