package facts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ReplicaEventStatus is the producer-side delivery state that must travel
// with an envelope when a DHCP lease store is mirrored for HA.
type ReplicaEventStatus string

const (
	ReplicaEventPending ReplicaEventStatus = "pending"
	ReplicaEventDone    ReplicaEventStatus = "done"
	ReplicaEventFailed  ReplicaEventStatus = "failed"
)

// ReplicaEvent is one durable producer row. Consumer state is deliberately
// absent: projection status belongs to the control database, while these
// fields describe whether the producer has delivered the envelope there.
type ReplicaEvent struct {
	Envelope      Envelope               `json:"envelope"`
	Attempts      int                    `json:"attempts"`
	NextAttemptAt string                 `json:"next_attempt_at"`
	LastError     string                 `json:"last_error,omitempty"`
	Status        ReplicaEventStatus     `json:"status"`
	Delivery      *ReplicaDeliveryMarker `json:"delivery,omitempty"`
}

// ReplicaDeliveryMarker is the durable producer-to-control retry marker. It
// is distinct from event status so a mirror can preserve the exact next push
// attempt after promotion. Done events must not have a marker.
type ReplicaDeliveryMarker struct {
	QueuedAt      string  `json:"queued_at"`
	Attempts      int     `json:"attempts"`
	NextAttemptAt *string `json:"next_attempt_at,omitempty"`
	LastError     string  `json:"last_error,omitempty"`
}

// ReplicaPage is a bounded page from a stable SQL read transaction. The
// caller keeps that transaction open across pages so LastSequence and the
// returned rows describe one snapshot even while the primary keeps writing.
type ReplicaPage struct {
	LastSequence int64          `json:"last_sequence"`
	NextAfter    int64          `json:"next_after"`
	Complete     bool           `json:"complete"`
	Events       []ReplicaEvent `json:"events"`
}

var ErrReplicaSequenceGap = errors.New("facts: replica snapshot sequence gap")

// ReadReplicaPageTx reads a bounded page from a caller-owned read transaction.
// Every allocated sequence must have exactly one durable envelope; a missing
// row makes the snapshot unusable and is returned as ErrReplicaSequenceGap.
func (o *ObservationOutbox) ReadReplicaPageTx(ctx context.Context, tx *sql.Tx, after int64, limit int) (ReplicaPage, error) {
	if o == nil || o.db == nil {
		return ReplicaPage{}, ErrOutboxClosed
	}
	if tx == nil {
		return ReplicaPage{}, errors.New("facts: nil replica snapshot transaction")
	}
	if after < 0 {
		return ReplicaPage{}, fmt.Errorf("facts: invalid replica cursor %d", after)
	}
	if limit <= 0 || limit > 1000 {
		limit = 256
	}
	var page ReplicaPage
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((
		SELECT last_sequence FROM facts_sequence_allocator WHERE domain = ?), 0)`, sequenceAllocatorDomain).
		Scan(&page.LastSequence); err != nil {
		return ReplicaPage{}, fmt.Errorf("facts: read replica sequence: %w", err)
	}
	if after > page.LastSequence {
		return ReplicaPage{}, fmt.Errorf("facts: replica cursor %d exceeds sequence %d", after, page.LastSequence)
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.event_id, e.version, e.entity, e.action, e.generation,
		e.sequence, e.source, e.occurred_at, e.payload_version, e.payload, e.attempts,
		e.next_attempt_at, e.last_error, e.status,
		d.event_id, d.queued_at, d.attempts, d.next_attempt_at, d.last_error
		FROM dhcp_ipam_observation_events e
		LEFT JOIN dhcp_ipam_observation_event_dirty d ON d.event_id = e.event_id
		WHERE e.sequence > ? ORDER BY e.sequence ASC LIMIT ?`, after, limit)
	if err != nil {
		return ReplicaPage{}, fmt.Errorf("facts: query replica page: %w", err)
	}
	defer rows.Close()
	expected := after + 1
	for rows.Next() {
		var item ReplicaEvent
		var occurred, payload, nextAttempt, status string
		var deliveryID, queuedAt, deliveryError sql.NullString
		var deliveryAttempts sql.NullInt64
		var deliveryNextAttempt sql.NullString
		if err := rows.Scan(&item.Envelope.EventID, &item.Envelope.Version,
			&item.Envelope.Entity, &item.Envelope.Action, &item.Envelope.Generation,
			&item.Envelope.Sequence, &item.Envelope.Source, &occurred,
			&item.Envelope.PayloadVersion, &payload, &item.Attempts,
			&nextAttempt, &item.LastError, &status,
			&deliveryID, &queuedAt, &deliveryAttempts, &deliveryNextAttempt, &deliveryError); err != nil {
			return ReplicaPage{}, fmt.Errorf("facts: scan replica page: %w", err)
		}
		item.Envelope.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			// The SQL writer may use SQLite's canonical UTC datetime format.
			item.Envelope.OccurredAt, err = time.Parse("2006-01-02 15:04:05", occurred)
		}
		if err != nil {
			return ReplicaPage{}, fmt.Errorf("facts: parse replica event %s time: %w", item.Envelope.EventID, err)
		}
		item.Envelope.Payload = json.RawMessage(payload)
		item.NextAttemptAt = nextAttempt
		item.Status = ReplicaEventStatus(status)
		if deliveryID.Valid {
			if !queuedAt.Valid || !deliveryAttempts.Valid {
				return ReplicaPage{}, fmt.Errorf("facts: incomplete replica delivery marker for event %s", item.Envelope.EventID)
			}
			item.Delivery = &ReplicaDeliveryMarker{
				QueuedAt: queuedAt.String, Attempts: int(deliveryAttempts.Int64), LastError: deliveryError.String,
			}
			if deliveryNextAttempt.Valid {
				next := deliveryNextAttempt.String
				item.Delivery.NextAttemptAt = &next
			}
		}
		if err := item.Envelope.Validate(); err != nil {
			return ReplicaPage{}, err
		}
		if item.Envelope.Sequence != expected {
			return ReplicaPage{}, fmt.Errorf("%w: expected=%d got=%d", ErrReplicaSequenceGap, expected, item.Envelope.Sequence)
		}
		if item.Attempts < 0 || item.NextAttemptAt == "" || !validReplicaEventStatus(item.Status) ||
			(item.Status == ReplicaEventDone && item.Delivery != nil) ||
			(item.Status != ReplicaEventDone && item.Delivery == nil) ||
			(item.Delivery != nil && (item.Delivery.QueuedAt == "" || item.Delivery.Attempts < 0)) {
			return ReplicaPage{}, fmt.Errorf("facts: invalid replica delivery state for event %s", item.Envelope.EventID)
		}
		page.Events = append(page.Events, item)
		page.NextAfter = item.Envelope.Sequence
		expected++
	}
	if err := rows.Err(); err != nil {
		return ReplicaPage{}, fmt.Errorf("facts: iterate replica page: %w", err)
	}
	if page.NextAfter < after {
		page.NextAfter = after
	}
	if page.NextAfter < page.LastSequence && len(page.Events) < limit {
		return ReplicaPage{}, fmt.Errorf("%w: expected=%d got=end (head=%d)", ErrReplicaSequenceGap, page.NextAfter+1, page.LastSequence)
	}
	page.Complete = page.NextAfter == page.LastSequence
	return page, nil
}

func validReplicaEventStatus(status ReplicaEventStatus) bool {
	switch status {
	case ReplicaEventPending, ReplicaEventDone, ReplicaEventFailed:
		return true
	default:
		return false
	}
}
