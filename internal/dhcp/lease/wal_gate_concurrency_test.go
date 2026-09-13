package lease

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

func TestWALDurableGateAssignsUniqueSequencesConcurrently(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "leases.wal"))
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	gate, err := NewWALDurableGate(wal, func(value *Lease) WALEvent {
		return WALEvent{Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- gate.Durable(context.Background(), &Lease{ID: string(rune('a' + i))})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := wal.Sequence(); got != workers {
		t.Fatalf("sequence = %d, want %d", got, workers)
	}
}
