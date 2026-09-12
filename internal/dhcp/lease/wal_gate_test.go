package lease

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestWALDurableGateAppendsAndSyncs(t *testing.T) {
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
	value := &Lease{ID: "l1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}
	if err := gate.Durable(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if wal.Sequence() != 1 {
		t.Fatalf("sequence = %d, want 1", wal.Sequence())
	}
	var replayed []WALEvent
	if _, err := wal.Replay(0, func(event WALEvent) error {
		replayed = append(replayed, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 1 || replayed[0].Lease == nil || replayed[0].Lease.ID != value.ID {
		t.Fatalf("replayed = %+v", replayed)
	}
}

func TestWALDurableGateRejectsInvalidConstructionAndContext(t *testing.T) {
	if _, err := NewWALDurableGate(nil, func(*Lease) WALEvent { return WALEvent{} }); !errors.Is(err, ErrWALGateClosed) {
		t.Fatalf("nil WAL error = %v", err)
	}
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "leases.wal"))
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	if _, err := NewWALDurableGate(wal, nil); err == nil {
		t.Fatal("nil event builder unexpectedly accepted")
	}
	gate, err := NewWALDurableGate(wal, func(value *Lease) WALEvent {
		return WALEvent{Op: WALEventUpsert, Lease: value}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.Durable(ctx, &Lease{ID: "l1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v", err)
	}
	if wal.Sequence() != 0 {
		t.Fatalf("canceled request appended sequence %d", wal.Sequence())
	}
}
