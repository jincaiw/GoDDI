package lease

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMemoryWriteBridgeSubmitsWALCanonicalSequence(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "leases.wal"))
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()

	gate, err := NewWALDurableGate(wal, func(value *Lease) WALEvent {
		return WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}
	index := NewMemoryIndex()
	submitted := make(chan WALEvent, 1)
	applier := &recordingSubmitter{submit: func(event WALEvent) error {
		submitted <- event
		return nil
	}}
	bridge, err := NewMemoryWriteBridge(index, gate, applier, func(value *Lease) WALEvent {
		return WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}

	value := Lease{ID: "lease-1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}
	if err := bridge.Commit(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	got := <-submitted
	if got.Seq != 1 {
		t.Fatalf("submitted sequence = %d, want 1", got.Seq)
	}
	if wal.Sequence() != got.Seq {
		t.Fatalf("wal sequence = %d, submitted = %d", wal.Sequence(), got.Seq)
	}
	if got.Lease == nil || got.Lease.ID != value.ID {
		t.Fatalf("submitted lease = %+v", got.Lease)
	}
}

type recordingSubmitter struct {
	submit func(WALEvent) error
}

func (s *recordingSubmitter) Submit(event WALEvent) error {
	return s.submit(event)
}
