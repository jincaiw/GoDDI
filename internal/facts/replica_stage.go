package facts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// StageReplicaChunkTx durably stores one bounded, invisible snapshot chunk.
// Replaying an identical chunk is idempotent; reusing its index with different
// contents fails closed. The caller owns and commits the transaction.
func (o *ObservationOutbox) StageReplicaChunkTx(ctx context.Context, tx *sql.Tx, snapshotID string, chunk ReplicaSnapshotChunk) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	if tx == nil {
		return errors.New("facts: nil replica staging transaction")
	}
	if strings.TrimSpace(snapshotID) == "" {
		return errors.New("facts: empty replica snapshot ID")
	}
	if err := validateReplicaSnapshotChunk(chunk); err != nil {
		return err
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("facts: encode staged replica chunk: %w", err)
	}
	if len(encoded) > maxReplicaPageBytes {
		return errors.New("facts: replica snapshot chunk exceeds byte limit")
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO facts_replica_snapshot_chunks
		(snapshot_id, chunk_index, first_sequence, last_sequence, event_count, chunk_json)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(snapshot_id, chunk_index) DO NOTHING`,
		snapshotID, chunk.Index, chunk.FirstSequence, chunk.LastSequence, len(chunk.Events), string(encoded))
	if err != nil {
		return fmt.Errorf("facts: stage replica chunk %d: %w", chunk.Index, err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("facts: inspect staged replica chunk %d: %w", chunk.Index, err)
	}
	if inserted == 1 {
		return nil
	}
	var first, last int64
	var count int
	var existing string
	if err := tx.QueryRowContext(ctx, `SELECT first_sequence, last_sequence, event_count, chunk_json
		FROM facts_replica_snapshot_chunks WHERE snapshot_id=? AND chunk_index=?`, snapshotID, chunk.Index).
		Scan(&first, &last, &count, &existing); err != nil {
		return fmt.Errorf("facts: read staged replica chunk %d: %w", chunk.Index, err)
	}
	if first != chunk.FirstSequence || last != chunk.LastSequence || count != len(chunk.Events) || existing != string(encoded) {
		return fmt.Errorf("facts: staged replica chunk %d was replayed with different contents", chunk.Index)
	}
	return nil
}

// ApplyStagedReplicaSnapshotTx verifies every staged chunk before replacing the
// active outbox, delivery markers, and allocator in the caller's transaction.
// The caller can apply the corresponding lease snapshot in the same SQL
// transaction so lease and facts watermarks become visible together.
// The caller must roll back the entire transaction if this method returns an
// error; committing after a partial apply would violate that atomic boundary.
func (o *ObservationOutbox) ApplyStagedReplicaSnapshotTx(ctx context.Context, tx *sql.Tx, snapshotID string, manifest ReplicaSnapshotManifest) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	if tx == nil {
		return errors.New("facts: nil replica apply transaction")
	}
	if strings.TrimSpace(snapshotID) == "" {
		return errors.New("facts: empty replica snapshot ID")
	}
	verifier, err := NewReplicaSnapshotAccumulator(manifest.LastSequence)
	if err != nil {
		return err
	}
	if err := o.walkStagedChunks(ctx, tx, snapshotID, func(chunk ReplicaSnapshotChunk) error {
		return verifier.AddChunk(chunk)
	}); err != nil {
		return err
	}
	if err := verifier.Verify(manifest); err != nil {
		return fmt.Errorf("facts: verify staged replica snapshot: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM dhcp_ipam_observation_event_dirty`); err != nil {
		return fmt.Errorf("facts: clear active fact delivery markers: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM dhcp_ipam_observation_events`); err != nil {
		return fmt.Errorf("facts: replace active facts outbox: %w", err)
	}
	if err := o.walkStagedChunks(ctx, tx, snapshotID, func(chunk ReplicaSnapshotChunk) error {
		for _, event := range chunk.Events {
			if err := o.insertReplicaEventTx(ctx, tx, event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO facts_sequence_allocator(domain, last_sequence)
		VALUES (?, ?) ON CONFLICT(domain) DO UPDATE SET last_sequence=excluded.last_sequence`,
		sequenceAllocatorDomain, manifest.LastSequence); err != nil {
		return fmt.Errorf("facts: apply replica sequence watermark: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM facts_replica_snapshot_chunks WHERE snapshot_id=?`, snapshotID); err != nil {
		return fmt.Errorf("facts: remove applied replica staging rows: %w", err)
	}
	return nil
}

// DiscardReplicaSnapshotTx removes an interrupted or superseded invisible
// staging snapshot. It never modifies the active outbox or allocator.
func (o *ObservationOutbox) DiscardReplicaSnapshotTx(ctx context.Context, tx *sql.Tx, snapshotID string) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	if tx == nil {
		return errors.New("facts: nil replica staging transaction")
	}
	if strings.TrimSpace(snapshotID) == "" {
		return errors.New("facts: empty replica snapshot ID")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM facts_replica_snapshot_chunks WHERE snapshot_id=?`, snapshotID); err != nil {
		return fmt.Errorf("facts: discard staged replica snapshot: %w", err)
	}
	return nil
}

func validateReplicaSnapshotChunk(chunk ReplicaSnapshotChunk) error {
	if chunk.Index < 0 || len(chunk.Events) == 0 || len(chunk.Events) > maxReplicaPageEvents {
		return errors.New("facts: invalid replica snapshot chunk index or event count")
	}
	if chunk.Events[0].Envelope.Sequence != chunk.FirstSequence ||
		chunk.Events[len(chunk.Events)-1].Envelope.Sequence != chunk.LastSequence {
		return errors.New("facts: replica snapshot chunk bounds do not match its events")
	}
	expected := chunk.FirstSequence
	encodedBytes := 0
	for _, event := range chunk.Events {
		if event.Envelope.Sequence != expected {
			return fmt.Errorf("%w: expected=%d got=%d", ErrReplicaSequenceGap, expected, event.Envelope.Sequence)
		}
		if err := validateReplicaEvent(event); err != nil {
			return err
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("facts: encode replica event %s: %w", event.Envelope.EventID, err)
		}
		encodedBytes += len(encoded)
		if encodedBytes > maxReplicaPageBytes-2048 {
			return errors.New("facts: replica snapshot chunk exceeds byte limit")
		}
		expected++
	}
	return nil
}

func (o *ObservationOutbox) walkStagedChunks(ctx context.Context, tx *sql.Tx, snapshotID string, visit func(ReplicaSnapshotChunk) error) error {
	var count int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts_replica_snapshot_chunks WHERE snapshot_id=?`, snapshotID).Scan(&count); err != nil {
		return fmt.Errorf("facts: count staged replica chunks: %w", err)
	}
	for expectedIndex := int64(0); expectedIndex < count; expectedIndex++ {
		var first, last int64
		var eventCount int
		var encoded string
		if err := tx.QueryRowContext(ctx, `SELECT first_sequence, last_sequence, event_count, chunk_json
			FROM facts_replica_snapshot_chunks WHERE snapshot_id=? AND chunk_index=?`, snapshotID, expectedIndex).
			Scan(&first, &last, &eventCount, &encoded); err != nil {
			return fmt.Errorf("facts: read staged replica chunk %d: %w", expectedIndex, err)
		}
		if len(encoded) > maxReplicaPageBytes {
			return fmt.Errorf("facts: staged replica chunk %d exceeds byte limit", expectedIndex)
		}
		var chunk ReplicaSnapshotChunk
		if err := json.Unmarshal([]byte(encoded), &chunk); err != nil {
			return fmt.Errorf("facts: decode staged replica chunk %d: %w", expectedIndex, err)
		}
		if chunk.Index != expectedIndex || chunk.FirstSequence != first || chunk.LastSequence != last || len(chunk.Events) != eventCount {
			return fmt.Errorf("facts: staged replica chunk %d metadata does not match contents", expectedIndex)
		}
		if err := validateReplicaSnapshotChunk(chunk); err != nil {
			return fmt.Errorf("facts: invalid staged replica chunk %d: %w", expectedIndex, err)
		}
		if err := visit(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (o *ObservationOutbox) insertReplicaEventTx(ctx context.Context, tx *sql.Tx, item ReplicaEvent) error {
	event := item.Envelope
	if err := event.Validate(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dhcp_ipam_observation_events
		(event_id, version, entity, action, generation, sequence, source, occurred_at,
		 payload_version, payload, attempts, next_attempt_at, last_error, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.Version, event.Entity, event.Action, event.Generation, event.Sequence,
		event.Source, event.OccurredAt.UTC().Format(time.RFC3339Nano), event.PayloadVersion,
		string(event.Payload), item.Attempts, item.NextAttemptAt, item.LastError, string(item.Status)); err != nil {
		return fmt.Errorf("facts: apply replica event %s: %w", event.EventID, err)
	}
	if item.Delivery == nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM dhcp_ipam_observation_event_dirty WHERE event_id=?`, event.EventID); err != nil {
			return fmt.Errorf("facts: remove completed replica delivery marker %s: %w", event.EventID, err)
		}
		return nil
	}
	var next any
	if item.Delivery.NextAttemptAt != nil {
		next = *item.Delivery.NextAttemptAt
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dhcp_ipam_observation_event_dirty
		(event_id, queued_at, attempts, next_attempt_at, last_error) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(event_id) DO UPDATE SET queued_at=excluded.queued_at,
		attempts=excluded.attempts, next_attempt_at=excluded.next_attempt_at, last_error=excluded.last_error`,
		event.EventID, item.Delivery.QueuedAt, item.Delivery.Attempts, next, item.Delivery.LastError); err != nil {
		return fmt.Errorf("facts: apply replica delivery marker %s: %w", event.EventID, err)
	}
	return nil
}
