package facts

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestReadReplicaPagesPreserveEnvelopeAndDeliveryState(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	allocator, _ := NewSequenceAllocator(db)
	ctx := context.Background()
	for sequence := int64(1); sequence <= 3; sequence++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := allocator.NextTx(ctx, tx)
		if err != nil || got != sequence {
			t.Fatalf("allocated sequence = %d, %v; want %d", got, err, sequence)
		}
		event := testEnvelope(sequence)
		event.Sequence = sequence
		event.EventID = fmt.Sprintf("replica-event-%d", sequence)
		if err := outbox.EnqueueTx(ctx, tx, event); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE dhcp_ipam_observation_events
		SET attempts=2, next_attempt_at='2026-09-24 10:00:00', last_error='offline', status='failed'
		WHERE sequence=2`); err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	first, err := outbox.ReadReplicaPageTx(ctx, tx, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.LastSequence != 3 || first.NextAfter != 2 || first.Complete || len(first.Events) != 2 {
		t.Fatalf("first replica page = %+v", first)
	}
	if first.Events[1].Envelope.EventID != "replica-event-2" || first.Events[1].Status != ReplicaEventFailed ||
		first.Events[1].Attempts != 2 || first.Events[1].LastError != "offline" ||
		first.Events[1].NextAttemptAt != "2026-09-24T10:00:00Z" {
		t.Fatalf("replicated producer state = %+v", first.Events[1])
	}
	last, err := outbox.ReadReplicaPageTx(ctx, tx, first.NextAfter, 2)
	if err != nil {
		t.Fatal(err)
	}
	if last.LastSequence != 3 || last.NextAfter != 3 || !last.Complete || len(last.Events) != 1 {
		t.Fatalf("last replica page = %+v", last)
	}
}

func TestReadReplicaPageRejectsAllocatedSequenceGap(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	allocator, _ := NewSequenceAllocator(db)
	ctx := context.Background()
	for sequence := int64(1); sequence <= 3; sequence++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := allocator.NextTx(ctx, tx); err != nil {
			t.Fatal(err)
		}
		event := testEnvelope(sequence)
		event.Sequence = sequence
		event.EventID = fmt.Sprintf("gap-event-%d", sequence)
		if err := outbox.EnqueueTx(ctx, tx, event); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`DELETE FROM dhcp_ipam_observation_events WHERE sequence=2`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := outbox.ReadReplicaPageTx(ctx, tx, 0, 3); !errors.Is(err, ErrReplicaSequenceGap) {
		t.Fatalf("ReadReplicaPageTx() error = %v, want ErrReplicaSequenceGap", err)
	}
}
