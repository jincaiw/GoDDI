package rolebarrier

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Tests exercise an open barrier without exposing a production opening API.
func activeForTest() *Barrier { return &Barrier{open: true} }

func TestDefaultClosed(t *testing.T) {
	var b Barrier
	if _, err := b.Acquire(); !errors.Is(err, ErrClosed) {
		t.Fatalf("zero value accepted entry: %v", err)
	}
	if err := b.Fence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Acquire(); !errors.Is(err, ErrClosed) {
		t.Fatal("fencing opened barrier")
	}
}

func TestFenceWaitsAndStopsNewEntries(t *testing.T) {
	b := activeForTest()
	first, err := b.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Fence(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled wait: %v", err)
	}
	if _, err := b.Acquire(); !errors.Is(err, ErrClosed) {
		t.Fatal("entry permitted after cancellation")
	}
	done := make(chan error, 1)
	go func() { done <- b.Fence(context.Background()) }()
	first.Release()
	select {
	case <-done:
		t.Fatal("fence completed with lease outstanding")
	default:
	}
	second.Release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fence did not drain")
	}
	if _, err := b.Acquire(); !errors.Is(err, ErrClosed) {
		t.Fatal("entry permitted after drain")
	}
}

func TestConcurrentFenceAndCopiedLeaseRelease(t *testing.T) {
	b := activeForTest()
	lease, err := b.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	other, err := b.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	copied := *lease
	const count = 32
	var wg sync.WaitGroup
	done := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); done <- b.Fence(context.Background()) }()
	}
	// Establish closed before concurrent releases.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Fence(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("fence: %v", err)
	}
	var releases sync.WaitGroup
	for i := 0; i < count; i++ {
		releases.Add(1)
		go func() { defer releases.Done(); lease.Release(); copied.Release() }()
	}
	releases.Wait()
	select {
	case <-done:
		t.Fatal("copied lease released a different operation")
	default:
	}
	other.Release()
	wg.Wait()
	close(done)
	for err := range done {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAcquireRacingFenceDrainsEveryAdmittedOperation(t *testing.T) {
	for round := 0; round < 100; round++ {
		b := activeForTest()
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				lease, err := b.Acquire()
				if err == nil {
					lease.Release()
				} else if !errors.Is(err, ErrClosed) {
					t.Errorf("acquire: %v", err)
				}
			}()
		}
		close(start)
		if err := b.Fence(context.Background()); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		if _, err := b.Acquire(); !errors.Is(err, ErrClosed) {
			t.Fatal("fenced barrier admitted entry")
		}
	}
}
