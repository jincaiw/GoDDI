package lease

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAsyncApplierAppliesInOrderAndTracksWatermark(t *testing.T) {
	var mu sync.Mutex
	var got []int64
	applier, err := NewAsyncApplier(context.Background(), 4, 0, func(context.Context, WALEvent) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, 1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()
	for seq := int64(1); seq <= 3; seq++ {
		if err := applier.Submit(WALEvent{Version: 1, Seq: seq, Op: WALEventRemove, LeaseID: "l"}); err != nil {
			t.Fatal(err)
		}
	}
	waitForApplied(t, applier, 3)
	if applier.Applied() != 3 {
		t.Fatalf("applied = %d, want 3", applier.Applied())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("applied events = %d, want 3", len(got))
	}
}

func TestAsyncApplierRejectsQueueOverflowAndPreservesDurableBoundary(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	applier, err := NewAsyncApplier(context.Background(), 1, 0, func(context.Context, WALEvent) error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()
	first := WALEvent{Version: 1, Seq: 1, Op: WALEventRemove, LeaseID: "l"}
	second := WALEvent{Version: 1, Seq: 2, Op: WALEventRemove, LeaseID: "l"}
	third := WALEvent{Version: 1, Seq: 3, Op: WALEventRemove, LeaseID: "l"}
	if err := applier.Submit(first); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := applier.Submit(second); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(applier.Submit(third), ErrApplyQueueFull) {
		t.Fatalf("overflow error = %v, want ErrApplyQueueFull", applier.Submit(third))
	}
	close(release)
	waitForApplied(t, applier, 2)
}

func TestAsyncApplierStopsOnApplyErrorAndFailsClosed(t *testing.T) {
	want := errors.New("sqlite unavailable")
	applier, err := NewAsyncApplier(context.Background(), 2, 0, func(context.Context, WALEvent) error {
		return want
	})
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()
	if err := applier.Submit(WALEvent{Version: 1, Seq: 1, Op: WALEventRemove, LeaseID: "l"}); err != nil {
		t.Fatal(err)
	}
	waitForFailure(t, applier)
	if !errors.Is(applier.Failed(), ErrApplierFailed) {
		t.Fatalf("failed = %v, want ErrApplierFailed", applier.Failed())
	}
	if err := applier.Submit(WALEvent{Version: 1, Seq: 2, Op: WALEventRemove, LeaseID: "l"}); !errors.Is(err, ErrApplierFailed) {
		t.Fatalf("submit after failure = %v, want ErrApplierFailed", err)
	}
}

func TestAsyncApplierRejectsInvalidConfigurationAndClosedSubmit(t *testing.T) {
	if _, err := NewAsyncApplier(context.Background(), 0, 0, func(context.Context, WALEvent) error { return nil }); err == nil {
		t.Fatal("capacity zero unexpectedly accepted")
	}
	if _, err := NewAsyncApplier(context.Background(), 1, 0, nil); err == nil {
		t.Fatal("nil apply unexpectedly accepted")
	}
	applier, err := NewAsyncApplier(context.Background(), 1, 0, func(context.Context, WALEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := applier.Close(); err != nil {
		t.Fatal(err)
	}
	if err := applier.Submit(WALEvent{Version: 1, Seq: 1, Op: WALEventRemove, LeaseID: "l"}); !errors.Is(err, ErrApplierClosed) {
		t.Fatalf("closed submit = %v, want ErrApplierClosed", err)
	}
}

func waitForApplied(t *testing.T, applier *AsyncApplier, want int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if applier.Applied() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("applied = %d, want at least %d", applier.Applied(), want)
}

func waitForFailure(t *testing.T, applier *AsyncApplier) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if applier.Failed() != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("applier did not fail")
}
