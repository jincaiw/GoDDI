package facts

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

func TestApplyStagedReplicaSnapshotReplacesOutboxAtomically(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	allocator, _ := NewSequenceAllocator(db)
	ctx := context.Background()

	seedActiveEvent(t, ctx, db, allocator, outbox, "old-event")
	events := stagedReplicaEvents(1, 2)
	chunk := ReplicaSnapshotChunk{Index: 0, FirstSequence: 1, LastSequence: 2, Events: events}
	manifest := manifestForChunks(t, 2, chunk)
	stageTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.StageReplicaChunkTx(ctx, stageTx, "snapshot-success", chunk); err != nil {
		t.Fatal(err)
	}
	if err := outbox.StageReplicaChunkTx(ctx, stageTx, "snapshot-success", chunk); err != nil {
		t.Fatalf("identical chunk replay: %v", err)
	}
	if err := stageTx.Commit(); err != nil {
		t.Fatal(err)
	}

	applyTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.ApplyStagedReplicaSnapshotTx(ctx, applyTx, "snapshot-success", manifest); err != nil {
		_ = applyTx.Rollback()
		t.Fatal(err)
	}
	if err := applyTx.Commit(); err != nil {
		t.Fatal(err)
	}
	var oldCount, eventCount, markerCount, stagedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events WHERE event_id='old-event'`).Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_event_dirty`).Scan(&markerCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM facts_replica_snapshot_chunks WHERE snapshot_id='snapshot-success'`).Scan(&stagedCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 || eventCount != 2 || markerCount != 2 || stagedCount != 0 {
		t.Fatalf("active/staged state = old=%d events=%d markers=%d staged=%d", oldCount, eventCount, markerCount, stagedCount)
	}
	if current, err := allocator.Current(ctx); err != nil || current != 2 {
		t.Fatalf("allocator = %d, %v; want 2", current, err)
	}
}

func TestApplyStagedReplicaSnapshotKeepsActiveStateOnChecksumFailure(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	allocator, _ := NewSequenceAllocator(db)
	ctx := context.Background()
	seedActiveEvent(t, ctx, db, allocator, outbox, "old-event")
	chunk := ReplicaSnapshotChunk{Index: 0, FirstSequence: 1, LastSequence: 1, Events: stagedReplicaEvents(1)}
	manifest := manifestForChunks(t, 1, chunk)
	stageTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.StageReplicaChunkTx(ctx, stageTx, "snapshot-corrupt", chunk); err != nil {
		t.Fatal(err)
	}
	if err := stageTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE facts_replica_snapshot_chunks SET chunk_json=replace(chunk_json, 'staged-event-1', 'tampered-event-1') WHERE snapshot_id='snapshot-corrupt'`); err != nil {
		t.Fatal(err)
	}

	applyTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.ApplyStagedReplicaSnapshotTx(ctx, applyTx, "snapshot-corrupt", manifest); err == nil {
		_ = applyTx.Rollback()
		t.Fatal("corrupted staged snapshot was applied")
	}
	_ = applyTx.Rollback()
	var count int
	var eventID string
	if err := db.QueryRow(`SELECT COUNT(*), MIN(event_id) FROM dhcp_ipam_observation_events`).Scan(&count, &eventID); err != nil {
		t.Fatal(err)
	}
	if count != 1 || eventID != "old-event" {
		t.Fatalf("active facts changed after rejected snapshot: count=%d event=%q", count, eventID)
	}
}

func TestApplyStagedReplicaChangesAppendsAtExactBase(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	allocator, _ := NewSequenceAllocator(db)
	ctx := context.Background()
	seedActiveEvent(t, ctx, db, allocator, outbox, "base-event")
	chunk := ReplicaSnapshotChunk{Index: 0, FirstSequence: 2, LastSequence: 2, Events: stagedReplicaEvents(2)}
	accumulator, err := NewReplicaSnapshotAccumulatorAfter(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := accumulator.AddChunk(chunk); err != nil {
		t.Fatal(err)
	}
	manifest, err := accumulator.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if err := stageChunk(t, ctx, db, outbox, "delta-2", chunk); err != nil {
		t.Fatal(err)
	}
	wrongBase, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	badManifest := manifest
	badManifest.AfterSequence = 0
	if err := outbox.ApplyStagedReplicaChangesTx(ctx, wrongBase, "delta-2", badManifest); err == nil {
		_ = wrongBase.Rollback()
		t.Fatal("delta with a stale base was applied")
	}
	_ = wrongBase.Rollback()

	applyTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.ApplyStagedReplicaChangesTx(ctx, applyTx, "delta-2", manifest); err != nil {
		_ = applyTx.Rollback()
		t.Fatal(err)
	}
	if err := applyTx.Commit(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("active facts event count = %d, want 2", count)
	}
	if current, err := allocator.Current(ctx); err != nil || current != 2 {
		t.Fatalf("allocator = %d, %v; want 2", current, err)
	}
}

func TestStageReplicaChunkRejectsConflictingReplay(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	ctx := context.Background()
	chunk := ReplicaSnapshotChunk{Index: 0, FirstSequence: 1, LastSequence: 1, Events: stagedReplicaEvents(1)}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.StageReplicaChunkTx(ctx, tx, "snapshot-replay", chunk); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	conflict := chunk
	conflict.Events = append([]ReplicaEvent(nil), chunk.Events...)
	conflict.Events[0].LastError = "changed"
	if err := stageChunk(t, ctx, db, outbox, "snapshot-replay", conflict); err == nil {
		t.Fatal("conflicting chunk replay was accepted")
	}
}

func stagedReplicaEvents(sequences ...int64) []ReplicaEvent {
	events := make([]ReplicaEvent, 0, len(sequences))
	for _, sequence := range sequences {
		envelope := testEnvelope(sequence)
		envelope.EventID = fmt.Sprintf("staged-event-%d", sequence)
		events = append(events, ReplicaEvent{
			Envelope: envelope, NextAttemptAt: "2026-09-24T10:00:00Z", Status: ReplicaEventPending,
			Delivery: &ReplicaDeliveryMarker{QueuedAt: "2026-09-24T09:00:00Z"},
		})
	}
	return events
}

func seedActiveEvent(t *testing.T, ctx context.Context, db *sql.DB, allocator *SequenceAllocator, outbox *ObservationOutbox, eventID string) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := allocator.NextTx(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	event := testEnvelope(sequence)
	event.EventID = eventID
	if err := outbox.EnqueueTx(ctx, tx, event); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func manifestForChunks(t *testing.T, highWater int64, chunks ...ReplicaSnapshotChunk) ReplicaSnapshotManifest {
	t.Helper()
	accumulator, err := NewReplicaSnapshotAccumulator(highWater)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range chunks {
		if err := accumulator.AddChunk(chunk); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := accumulator.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func stageChunk(t *testing.T, ctx context.Context, db *sql.DB, outbox *ObservationOutbox, snapshotID string, chunk ReplicaSnapshotChunk) error {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.StageReplicaChunkTx(ctx, tx, snapshotID, chunk); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
