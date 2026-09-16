package lease

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWALLifecycleClosesInDurabilityOrder(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "leases.wal"))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	applier, err := NewAsyncApplier(context.Background(), 1, 0, func(context.Context, WALEvent) error {
		close(started)
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewWALLifecycle(wal, applier)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l1"}); err != nil {
		t.Fatal(err)
	}
	<-started

	closeDone := make(chan error, 1)
	go func() { closeDone <- lifecycle.Close(context.Background()) }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before applier drained: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if _, err := lifecycle.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l2"}); !errors.Is(err, ErrWALLifecycleClosed) {
		t.Fatalf("append during close = %v, want ErrWALLifecycleClosed", err)
	}
	close(release)
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if _, err := wal.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l2"}); !errors.Is(err, ErrWALClosed) {
		t.Fatalf("WAL after lifecycle close = %v, want ErrWALClosed", err)
	}
	if err := applier.Submit(WALEvent{Version: 1, Seq: 2, Op: WALEventRemove, LeaseID: "l2"}); !errors.Is(err, ErrApplierClosed) {
		t.Fatalf("applier after lifecycle close = %v, want ErrApplierClosed", err)
	}
}

func TestWALLifecycleTimeoutKeepsDurableResourcesAvailableForReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.wal")
	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	applier, err := NewAsyncApplier(context.Background(), 1, 0, func(context.Context, WALEvent) error {
		close(started)
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewWALLifecycle(wal, applier)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l1"}); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := lifecycle.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want deadline exceeded", err)
	}
	if _, err := lifecycle.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l2"}); !errors.Is(err, ErrWALLifecycleClosed) {
		t.Fatalf("append after timed out close = %v, want ErrWALLifecycleClosed", err)
	}

	close(release)
	if err := applier.Close(); err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var replayed []WALEvent
	last, err := reopened.Replay(0, func(event WALEvent) error {
		replayed = append(replayed, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if last != 1 || len(replayed) != 1 || replayed[0].LeaseID != "l1" {
		t.Fatalf("replay last=%d events=%+v, want durable l1 event", last, replayed)
	}
}

func TestWALLifecycleRejectsNilDependenciesAndContext(t *testing.T) {
	if _, err := NewWALLifecycle(nil, nil); err == nil {
		t.Fatal("nil dependencies unexpectedly accepted")
	}
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "leases.wal"))
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	applier, err := NewAsyncApplier(context.Background(), 1, 0, func(context.Context, WALEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer applier.Close()
	lifecycle, err := NewWALLifecycle(wal, applier)
	if err != nil {
		t.Fatal(err)
	}
	var nilContext context.Context
	if err := lifecycle.Close(nilContext); err == nil {
		t.Fatal("nil close context unexpectedly accepted")
	}
}

func TestWALLifecycleConcurrentCloseHasSingleCutoff(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "leases.wal"))
	if err != nil {
		t.Fatal(err)
	}
	applier, err := NewAsyncApplier(context.Background(), 2, 0, func(context.Context, WALEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewWALLifecycle(wal, applier)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- lifecycle.Close(context.Background())
		}()
	}
	wg.Wait()
	close(results)
	var success, closed int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrWALLifecycleClosed):
			closed++
		default:
			t.Fatalf("concurrent close error = %v", err)
		}
	}
	if success != 1 || closed != 1 {
		t.Fatalf("close results success=%d closed=%d, want 1/1", success, closed)
	}
}
