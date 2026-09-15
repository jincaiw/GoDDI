package facts

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func newFactsOutboxDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE dhcp_ipam_observation_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		version INTEGER NOT NULL,
		entity TEXT NOT NULL,
		action TEXT NOT NULL,
		generation INTEGER NOT NULL,
		sequence INTEGER NOT NULL UNIQUE,
		source TEXT NOT NULL,
		occurred_at DATETIME NOT NULL,
		payload_version INTEGER NOT NULL,
		payload TEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
		last_error TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestObservationOutboxEnqueueIsIdempotentButRejectsConflicts(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, err := NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	event := testEnvelope(1)
	ctx := context.Background()
	if err := outbox.Enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Enqueue(ctx, event); err != nil {
		t.Fatalf("identical replay: %v", err)
	}
	changed := event
	changed.Action = "release"
	if err := outbox.Enqueue(ctx, changed); !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("conflicting replay error = %v, want ErrDuplicateEvent", err)
	}
}

func TestObservationOutboxEnqueueTxRollsBackWithCaller(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.EnqueueTx(context.Background(), tx, testEnvelope(1)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("events after rollback = %d, want 0", count)
	}
}

func TestObservationOutboxStatusReportsBacklogCoordinates(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	ctx := context.Background()
	if err := outbox.Enqueue(ctx, testEnvelope(1)); err != nil {
		t.Fatal(err)
	}
	second := testEnvelope(2)
	second.EventID = "event-2"
	if err := outbox.Enqueue(ctx, second); err != nil {
		t.Fatal(err)
	}
	status, err := outbox.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Pending != 2 || status.Failed != 0 || status.HeadSequence != 2 || status.FirstOutstandingSequence != 1 {
		t.Fatalf("status = %+v, want pending=2 failed=0 head=2 first=1", status)
	}
	if err := outbox.MarkDone(ctx, "event-1"); err != nil {
		t.Fatal(err)
	}
	if err := outbox.MarkRetry(ctx, "event-2", "projection unavailable", 1); err != nil {
		t.Fatal(err)
	}
	status, err = outbox.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Pending != 0 || status.Failed != 1 || status.HeadSequence != 2 || status.FirstOutstandingSequence != 2 {
		t.Fatalf("failed status = %+v, want pending=0 failed=1 head=2 first=2", status)
	}
}

func TestObservationOutboxPendingAndRetryKeepFailedFactsVisible(t *testing.T) {
	db := newFactsOutboxDB(t)
	outbox, _ := NewObservationOutbox(db)
	ctx := context.Background()
	if err := outbox.Enqueue(ctx, testEnvelope(1)); err != nil {
		t.Fatal(err)
	}
	pending, err := outbox.Pending(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].Sequence != 1 {
		t.Fatalf("pending = %+v, err=%v", pending, err)
	}
	if err := outbox.MarkRetry(ctx, "event-1", "consumer unavailable", 1); err != nil {
		t.Fatal(err)
	}
	pendingCount, failedCount, err := outbox.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pendingCount != 0 || failedCount != 1 {
		t.Fatalf("stats = pending=%d failed=%d, want 0/1", pendingCount, failedCount)
	}
	if err := outbox.MarkDone(ctx, "event-1"); err != nil {
		t.Fatal(err)
	}
	pendingCount, failedCount, err = outbox.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pendingCount != 0 || failedCount != 0 {
		t.Fatalf("stats after done = pending=%d failed=%d, want 0/0", pendingCount, failedCount)
	}
}
