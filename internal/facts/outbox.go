package facts

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrDuplicateEvent = errors.New("facts: duplicate event differs")
	ErrOutboxClosed   = errors.New("facts: nil outbox database")
)

// ObservationOutbox is the staged durable writer for DHCP-derived IPAM facts.
// It is intentionally separate from the legacy dhcp_dns_events table: callers
// can adopt the common envelope without changing the compatibility consumer.
type ObservationOutbox struct {
	db *sql.DB
}

// ObservationOutboxStatus is a durable, point-in-time backlog snapshot. A
// missing first outstanding sequence is represented by zero; callers must not
// treat zero as a healthy sequence because it means the outbox has no
// pending/failed event to inspect.
type ObservationOutboxStatus struct {
	Pending                  int64
	Failed                   int64
	HeadSequence             int64
	FirstOutstandingSequence int64
}

func NewObservationOutbox(db *sql.DB) (*ObservationOutbox, error) {
	if db == nil {
		return nil, ErrOutboxClosed
	}
	return &ObservationOutbox{db: db}, nil
}

// Enqueue records one validated envelope. The caller may use EnqueueTx to put
// this insert beside the authoritative lease mutation in one transaction.
func (o *ObservationOutbox) Enqueue(ctx context.Context, event Envelope) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("facts outbox: begin: %w", err)
	}
	if err := o.EnqueueTx(ctx, tx, event); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("facts outbox: commit: %w", err)
	}
	return nil
}

// EnqueueTx inserts an event without committing. A repeated identical event is
// idempotent; a reused event ID or sequence with a different payload fails
// closed rather than silently discarding a fact.
func (o *ObservationOutbox) EnqueueTx(ctx context.Context, tx *sql.Tx, event Envelope) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	if tx == nil {
		return errors.New("facts outbox: nil transaction")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("facts outbox: encode payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO dhcp_ipam_observation_events
			(event_id, version, entity, action, generation, sequence, source,
			 occurred_at, payload_version, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.Version, event.Entity, event.Action, event.Generation,
		event.Sequence, event.Source, event.OccurredAt.UTC().Format(time.RFC3339Nano),
		event.PayloadVersion, string(payload))
	if err == nil {
		return nil
	}

	var existing Envelope
	var occurred string
	var payloadText string
	lookupErr := tx.QueryRowContext(ctx, `
		SELECT version, entity, action, generation, sequence, source,
		       occurred_at, payload_version, payload
		FROM dhcp_ipam_observation_events WHERE event_id = ?`, event.EventID).
		Scan(&existing.Version, &existing.Entity, &existing.Action, &existing.Generation,
			&existing.Sequence, &existing.Source, &occurred, &existing.PayloadVersion, &payloadText)
	if lookupErr != nil {
		return fmt.Errorf("facts outbox: insert event %s: %w", event.EventID, err)
	}
	existing.EventID = event.EventID
	existing.OccurredAt, lookupErr = time.Parse(time.RFC3339Nano, occurred)
	if lookupErr != nil {
		return fmt.Errorf("facts outbox: decode event %s time: %w", event.EventID, lookupErr)
	}
	existing.Payload = json.RawMessage(payloadText)
	if existing.Version == event.Version && existing.Entity == event.Entity &&
		existing.Action == event.Action && existing.Generation == event.Generation &&
		existing.Sequence == event.Sequence && existing.Source == event.Source &&
		existing.OccurredAt.Equal(event.OccurredAt) && existing.PayloadVersion == event.PayloadVersion &&
		bytes.Equal(existing.Payload, event.Payload) {
		return nil
	}
	return fmt.Errorf("%w: event_id=%s", ErrDuplicateEvent, event.EventID)
}

// MarkRetry records a failed delivery with bounded exponential backoff. Failed
// rows remain visible for reconciliation; they are never silently deleted.
func (o *ObservationOutbox) MarkRetry(ctx context.Context, eventID, reason string, maxAttempts int) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	if eventID == "" {
		return errors.New("facts outbox: empty event ID")
	}
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	var attempts int
	if err := o.db.QueryRowContext(ctx,
		`SELECT attempts FROM dhcp_ipam_observation_events WHERE event_id = ?`, eventID).Scan(&attempts); err != nil {
		return fmt.Errorf("facts outbox: read attempts %s: %w", eventID, err)
	}
	attempts++
	if attempts >= maxAttempts {
		_, err := o.db.ExecContext(ctx, `UPDATE dhcp_ipam_observation_events
			SET status = 'failed', attempts = ?, last_error = ?, updated_at = datetime('now')
			WHERE event_id = ?`, attempts, reason, eventID)
		if err != nil {
			return fmt.Errorf("facts outbox: fail %s: %w", eventID, err)
		}
		return nil
	}
	backoff := time.Second << minInt(attempts-1, 8)
	_, err := o.db.ExecContext(ctx, `UPDATE dhcp_ipam_observation_events
		SET attempts = ?, last_error = ?, next_attempt_at = datetime('now', ?), updated_at = datetime('now')
		WHERE event_id = ?`, attempts, reason, fmt.Sprintf("+%d seconds", int(backoff.Seconds())), eventID)
	if err != nil {
		return fmt.Errorf("facts outbox: retry %s: %w", eventID, err)
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Pending returns due events in the shared sequence order.
func (o *ObservationOutbox) Pending(ctx context.Context, limit int) ([]Envelope, error) {
	if o == nil || o.db == nil {
		return nil, ErrOutboxClosed
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := o.db.QueryContext(ctx, `
		SELECT event_id, version, entity, action, generation, sequence, source,
		       occurred_at, payload_version, payload
		FROM dhcp_ipam_observation_events
		WHERE status = 'pending' AND julianday(next_attempt_at) <= julianday('now')
		ORDER BY sequence ASC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("facts outbox: pending query: %w", err)
	}
	defer rows.Close()
	var events []Envelope
	for rows.Next() {
		var event Envelope
		var occurred, payload string
		if err := rows.Scan(&event.EventID, &event.Version, &event.Entity, &event.Action,
			&event.Generation, &event.Sequence, &event.Source, &occurred,
			&event.PayloadVersion, &payload); err != nil {
			return nil, fmt.Errorf("facts outbox: scan: %w", err)
		}
		var err error
		event.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return nil, fmt.Errorf("facts outbox: parse event %s time: %w", event.EventID, err)
		}
		event.Payload = json.RawMessage(payload)
		if err := event.Validate(); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("facts outbox: iterate: %w", err)
	}
	return events, nil
}

func (o *ObservationOutbox) MarkDone(ctx context.Context, eventID string) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("facts outbox: begin mark done: %w", err)
	}
	if err := o.MarkDoneTx(ctx, tx, eventID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("facts outbox: commit mark done: %w", err)
	}
	return nil
}

// MarkDoneTx marks an event complete without committing. Consumers use this in
// the same transaction as their projection and watermark update.
func (o *ObservationOutbox) MarkDoneTx(ctx context.Context, tx *sql.Tx, eventID string) error {
	if o == nil || o.db == nil {
		return ErrOutboxClosed
	}
	if tx == nil {
		return errors.New("facts outbox: nil transaction")
	}
	if eventID == "" {
		return errors.New("facts outbox: empty event ID")
	}
	res, err := tx.ExecContext(ctx, `UPDATE dhcp_ipam_observation_events
		SET status = 'done', last_error = '', updated_at = datetime('now')
		WHERE event_id = ? AND status <> 'done'`, eventID)
	if err != nil {
		return fmt.Errorf("facts outbox: mark %s done: %w", eventID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("facts outbox: mark %s done rows: %w", eventID, err)
	} else if n == 0 {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM dhcp_ipam_observation_events WHERE event_id = ?`, eventID).Scan(&status); err != nil {
			return fmt.Errorf("facts outbox: verify %s done: %w", eventID, err)
		}
		if status != "done" {
			return fmt.Errorf("facts outbox: event %s was not pending", eventID)
		}
	}
	return nil
}

func (o *ObservationOutbox) Stats(ctx context.Context) (pending, failed int64, err error) {
	status, err := o.Status(ctx)
	if err != nil {
		return 0, 0, err
	}
	return status.Pending, status.Failed, nil
}

// Status returns durable backlog counts and sequence coordinates. HeadSequence
// is the highest durable sequence, while FirstOutstandingSequence is the
// lowest pending or failed sequence. A failed event remains outstanding so a
// readiness consumer cannot mistake a retained failure for a drained stream.
func (o *ObservationOutbox) Status(ctx context.Context) (ObservationOutboxStatus, error) {
	if o == nil || o.db == nil {
		return ObservationOutboxStatus{}, ErrOutboxClosed
	}
	var status ObservationOutboxStatus
	err := o.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
		COALESCE(MAX(sequence), 0),
		COALESCE(MIN(CASE WHEN status IN ('pending', 'failed') THEN sequence END), 0)
		FROM dhcp_ipam_observation_events`).Scan(
		&status.Pending, &status.Failed, &status.HeadSequence, &status.FirstOutstandingSequence)
	if err != nil {
		return ObservationOutboxStatus{}, fmt.Errorf("facts outbox: status: %w", err)
	}
	return status, nil
}
