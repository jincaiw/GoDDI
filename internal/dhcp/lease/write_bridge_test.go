package lease

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type durableEventProbe struct {
	mu    sync.Mutex
	err   error
	event WALEvent
	calls int
}

func (p *durableEventProbe) DurableEvent(_ context.Context, event WALEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.event = event
	return p.err
}

type submitEventProbe struct {
	mu    sync.Mutex
	err   error
	event WALEvent
	calls int
}

func (p *submitEventProbe) Submit(event WALEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.event = event
	return p.err
}

func TestMemoryWriteBridgeRejectsCancelledContextWithoutMutation(t *testing.T) {
	index := NewMemoryIndex()
	durable := &durableEventProbe{}
	submit := &submitEventProbe{}
	bridge, err := NewMemoryWriteBridge(index, durable, submit, func(value *Lease) WALEvent {
		return WALEvent{Version: 1, Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bridge.Commit(ctx, Lease{ID: "l1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("commit error = %v, want context.Canceled", err)
	}
	if durable.calls != 0 || submit.calls != 0 {
		t.Fatalf("durable calls=%d submit calls=%d, want zero", durable.calls, submit.calls)
	}
	if _, ok := index.Get("l1"); ok {
		t.Fatal("cancelled commit mutated memory")
	}
}

func TestMemoryWriteBridgeSerializesConcurrentCommits(t *testing.T) {
	index := NewMemoryIndex()
	var orderMu sync.Mutex
	var order []string
	durable := &durableEventProbe{}
	submit := &submitEventProbe{}
	bridge, err := NewMemoryWriteBridge(index, durable, submit, func(value *Lease) WALEvent {
		orderMu.Lock()
		order = append(order, "build:"+value.ID)
		orderMu.Unlock()
		return WALEvent{Version: 1, Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"l1", "l2", "l3"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := bridge.Commit(context.Background(), Lease{ID: id}); err != nil {
				t.Errorf("commit %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()
	durable.mu.Lock()
	durableCalls := durable.calls
	durable.mu.Unlock()
	submit.mu.Lock()
	submitCalls := submit.calls
	submit.mu.Unlock()
	orderMu.Lock()
	orderLen := len(order)
	orderMu.Unlock()
	if durableCalls != 3 || submitCalls != 3 || orderLen != 3 {
		t.Fatalf("durable=%d submit=%d build=%d, want 3", durableCalls, submitCalls, orderLen)
	}
}

func TestMemoryWriteBridgeCommitsMemoryDurabilityThenProjection(t *testing.T) {
	index := NewMemoryIndex()
	durable := &durableEventProbe{}
	submit := &submitEventProbe{}
	bridge, err := NewMemoryWriteBridge(index, durable, submit, func(value *Lease) WALEvent {
		return WALEvent{Version: 1, Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}
	value := Lease{ID: "l1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}
	if err := bridge.Commit(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if got, ok := index.Get(value.ID); !ok || got.IPAddress != value.IPAddress {
		t.Fatalf("memory value = %+v, %v", got, ok)
	}
	if durable.calls != 1 || submit.calls != 1 {
		t.Fatalf("durable calls=%d submit calls=%d", durable.calls, submit.calls)
	}
	if submit.event.Lease == nil || submit.event.Lease.ID != value.ID {
		t.Fatalf("submitted event = %+v", submit.event)
	}
}

func TestMemoryWriteBridgeRestoresMemoryWhenDurabilityFails(t *testing.T) {
	index := NewMemoryIndex()
	old := Lease{ID: "l1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}
	index.Upsert(old)
	want := errors.New("sync failed")
	durable := &durableEventProbe{err: want}
	submit := &submitEventProbe{}
	bridge, err := NewMemoryWriteBridge(index, durable, submit, func(value *Lease) WALEvent {
		return WALEvent{Version: 1, Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.Commit(context.Background(), Lease{ID: old.ID, IPAddress: "192.0.2.11", Status: LeaseStatusActive}); !errors.Is(err, ErrWriteBridgeFailed) {
		t.Fatalf("commit error = %v", err)
	}
	got, ok := index.Get(old.ID)
	if !ok || got.IPAddress != old.IPAddress {
		t.Fatalf("old value not restored: %+v, %v", got, ok)
	}
	if submit.calls != 0 {
		t.Fatalf("submit calls = %d, want 0", submit.calls)
	}
}

func TestMemoryWriteBridgeKeepsDurableMemoryWhenProjectionSubmitFails(t *testing.T) {
	index := NewMemoryIndex()
	durable := &durableEventProbe{}
	submit := &submitEventProbe{err: errors.New("queue full")}
	bridge, err := NewMemoryWriteBridge(index, durable, submit, func(value *Lease) WALEvent {
		return WALEvent{Version: 1, Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}
	value := Lease{ID: "l1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}
	if err := bridge.Commit(context.Background(), value); !errors.Is(err, ErrWriteBridgeFailed) {
		t.Fatalf("commit error = %v", err)
	}
	if got, ok := index.Get(value.ID); !ok || got.IPAddress != value.IPAddress {
		t.Fatalf("durable memory value lost: %+v, %v", got, ok)
	}
}
