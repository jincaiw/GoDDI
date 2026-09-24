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
	if _, err := db.Exec(`UPDATE dhcp_ipam_observation_event_dirty
		SET attempts=1, next_attempt_at='2026-09-24 10:00:00', last_error='control unavailable'
		WHERE event_id='replica-event-2'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE dhcp_ipam_observation_events SET status='done' WHERE sequence=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM dhcp_ipam_observation_event_dirty WHERE event_id='replica-event-1'`); err != nil {
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
	if first.Events[0].Status != ReplicaEventDone || first.Events[0].Delivery != nil {
		t.Fatalf("completed event retains a delivery marker: %+v", first.Events[0])
	}
	if first.Events[1].Delivery == nil || first.Events[1].Delivery.Attempts != 1 ||
		first.Events[1].Delivery.LastError != "control unavailable" || first.Events[1].Delivery.NextAttemptAt == nil ||
		*first.Events[1].Delivery.NextAttemptAt != "2026-09-24T10:00:00Z" {
		t.Fatalf("replicated delivery marker = %+v", first.Events[1].Delivery)
	}
	last, err := outbox.ReadReplicaPageTx(ctx, tx, first.NextAfter, 2)
	if err != nil {
		t.Fatal(err)
	}
	if last.LastSequence != 3 || last.NextAfter != 3 || !last.Complete || len(last.Events) != 1 {
		t.Fatalf("last replica page = %+v", last)
	}
	accumulator, err := NewReplicaSnapshotAccumulator(first.LastSequence)
	if err != nil {
		t.Fatal(err)
	}
	if err := accumulator.AddPage(first); err != nil {
		t.Fatal(err)
	}
	if _, err := accumulator.Manifest(); err == nil {
		t.Fatal("incomplete snapshot produced a manifest")
	}
	if err := accumulator.AddPage(last); err != nil {
		t.Fatal(err)
	}
	manifest, err := accumulator.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != ReplicaSnapshotVersion || manifest.LastSequence != 3 || manifest.EventCount != 3 || manifest.ChunkCount != 2 || len(manifest.Digest) != 64 {
		t.Fatalf("snapshot manifest = %+v", manifest)
	}
	verifier, err := NewReplicaSnapshotAccumulator(first.LastSequence)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.AddChunk(ReplicaSnapshotChunk{Index: 0, FirstSequence: 1, LastSequence: 2, Events: first.Events}); err != nil {
		t.Fatal(err)
	}
	if err := verifier.AddChunk(ReplicaSnapshotChunk{Index: 1, FirstSequence: 3, LastSequence: 3, Events: last.Events}); err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(manifest); err != nil {
		t.Fatalf("verify complete snapshot: %v", err)
	}
	rechunked, err := NewReplicaSnapshotAccumulator(first.LastSequence)
	if err != nil {
		t.Fatal(err)
	}
	allEvents := append(append([]ReplicaEvent(nil), first.Events...), last.Events...)
	if err := rechunked.AddChunk(ReplicaSnapshotChunk{Index: 0, FirstSequence: 1, LastSequence: 3, Events: allEvents}); err != nil {
		t.Fatal(err)
	}
	rechunkedManifest, err := rechunked.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if rechunkedManifest.Digest != manifest.Digest || rechunkedManifest.ChunkCount != 1 {
		t.Fatalf("digest changed with transport chunking: split=%+v combined=%+v", manifest, rechunkedManifest)
	}
	var streamedChunks []ReplicaSnapshotChunk
	streamedManifest, err := outbox.StreamReplicaSnapshotTx(ctx, tx, 1, func(chunk ReplicaSnapshotChunk) error {
		streamedChunks = append(streamedChunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if streamedManifest.Digest != manifest.Digest || streamedManifest.ChunkCount != 3 || len(streamedChunks) != 3 {
		t.Fatalf("streamed snapshot = %+v chunks=%d; want same digest and three one-event chunks", streamedManifest, len(streamedChunks))
	}
	var suffixChunks []ReplicaSnapshotChunk
	suffixManifest, err := outbox.StreamReplicaChangesTx(ctx, tx, 1, 1, func(chunk ReplicaSnapshotChunk) error {
		suffixChunks = append(suffixChunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if suffixManifest.AfterSequence != 1 || suffixManifest.LastSequence != 3 || suffixManifest.EventCount != 2 ||
		suffixManifest.ChunkCount != 2 || len(suffixChunks) != 2 || suffixChunks[0].FirstSequence != 2 ||
		suffixChunks[1].LastSequence != 3 {
		t.Fatalf("streamed facts suffix = manifest %+v chunks=%+v; want sequence 2..3", suffixManifest, suffixChunks)
	}
	stopStreaming := errors.New("stop stream")
	if _, err := outbox.StreamReplicaSnapshotTx(ctx, tx, 1, func(ReplicaSnapshotChunk) error {
		return stopStreaming
	}); !errors.Is(err, stopStreaming) {
		t.Fatalf("stream callback error = %v, want %v", err, stopStreaming)
	}
}

func TestReplicaSnapshotAccumulatorRejectsChangedHighWater(t *testing.T) {
	accumulator, err := NewReplicaSnapshotAccumulator(2)
	if err != nil {
		t.Fatal(err)
	}
	if err := accumulator.AddPage(ReplicaPage{LastSequence: 3, NextAfter: 0}); err == nil {
		t.Fatal("page with changed allocator high water was accepted")
	}
}

func TestReplicaSnapshotAccumulatorRejectsChunkBoundaryMismatch(t *testing.T) {
	accumulator, err := NewReplicaSnapshotAccumulator(2)
	if err != nil {
		t.Fatal(err)
	}
	chunk := ReplicaSnapshotChunk{
		Index: 0, FirstSequence: 2, LastSequence: 2,
		Events: []ReplicaEvent{{Envelope: Envelope{Sequence: 1}}},
	}
	if err := accumulator.AddChunk(chunk); err == nil {
		t.Fatal("chunk with a false declared boundary was accepted")
	}
	chunk.FirstSequence = 1
	chunk.LastSequence = 1
	chunk.Index = 1
	if err := accumulator.AddChunk(chunk); err == nil {
		t.Fatal("chunk with an out-of-order index was accepted")
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
