package facts

// The relay's round trip: events the producer commits into the lease plane's
// outbox appear in the control plane's inbox in strict sequence order, the
// relay watermark advances exactly one sequence per event, and a crash
// between the two sides replays idempotently (the inbox insert and the
// watermark advance commit together).

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// outboxDDL is the shared observation-events table (the producer outbox and
// the consumer inbox carry the same shape; the migrations add their own index
// names and the watermark table on the control side).
const outboxDDL = `CREATE TABLE IF NOT EXISTS dhcp_ipam_observation_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id        TEXT    NOT NULL UNIQUE,
    version         INTEGER NOT NULL CHECK (version > 0),
    entity          TEXT    NOT NULL,
    action          TEXT    NOT NULL,
    generation      INTEGER NOT NULL CHECK (generation >= 0),
    sequence        INTEGER NOT NULL UNIQUE CHECK (sequence > 0),
    source          TEXT    NOT NULL,
    occurred_at     DATETIME NOT NULL,
    payload_version INTEGER NOT NULL CHECK (payload_version > 0),
    payload         TEXT    NOT NULL CHECK (json_valid(payload)),
    attempts        INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_error      TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'done', 'failed')),
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);`

const watermarkDDL = `CREATE TABLE IF NOT EXISTS facts_projection_watermark (
    domain      TEXT PRIMARY KEY,
    applied_seq INTEGER NOT NULL DEFAULT 0 CHECK (applied_seq >= 0)
);`

func testTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func relayFixture(t *testing.T) (source, inbox *ObservationOutbox, relay *Relay) {
	t.Helper()
	leaseDB, err := sql.Open("sqlite", t.TempDir()+"/lease.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = leaseDB.Close() })
	controlDB, err := sql.Open("sqlite", t.TempDir()+"/control.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controlDB.Close() })
	// The two tables install through the respective migration chains in
	// production; this fixture applies the same DDL directly.
	if _, err := leaseDB.Exec(outboxDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := controlDB.Exec(outboxDDL + watermarkDDL); err != nil {
		t.Fatal(err)
	}
	source, err = NewObservationOutbox(leaseDB)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err = NewObservationOutbox(controlDB)
	if err != nil {
		t.Fatal(err)
	}
	relay, err = NewRelay(source, inbox, controlDB)
	if err != nil {
		t.Fatal(err)
	}
	return source, inbox, relay
}

func TestRelayCopiesEventsInStrictSequenceOrder(t *testing.T) {
	source, inbox, relay := relayFixture(t)
	for i := int64(1); i <= 5; i++ {
		if err := source.Enqueue(context.Background(), Envelope{
			EventID: "evt-" + string(rune('a'+i-1)), Version: 1, Entity: "ipam",
			Action: "activate", Sequence: i, Source: "dhcp",
			OccurredAt: testTime(), PayloadVersion: 1,
			Payload: []byte(`{"action":"activate","space_id":"s","ip":"192.0.2.1"}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := relay.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if n != 5 {
		t.Fatalf("relayed %d events, want 5", n)
	}
	// The inbox holds every event in order; the watermark reached the head.
	pending, err := inbox.Pending(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 5 {
		t.Fatalf("the inbox holds %d events, want 5", len(pending))
	}
	if pending[0].Sequence != 1 || pending[4].Sequence != 5 {
		t.Fatalf("the inbox order is wrong: %d..%d", pending[0].Sequence, pending[4].Sequence)
	}
	applied, err := relay.watermark.Applied(context.Background(), RelayDomain)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 5 {
		t.Fatalf("the relay watermark = %d, want 5", applied)
	}
	// A second tick is a no-op: everything is relayed.
	n, err = relay.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a second tick relayed %d events, want 0", n)
	}
}

func TestRelayReplaysIdempotentlyAfterAPartialTick(t *testing.T) {
	source, inbox, relay := relayFixture(t)
	for i := int64(1); i <= 3; i++ {
		if err := source.Enqueue(context.Background(), Envelope{
			EventID: "evt-" + string(rune('a'+i-1)), Version: 1, Entity: "ipam",
			Action: "activate", Sequence: i, Source: "dhcp",
			OccurredAt: testTime(), PayloadVersion: 1,
			Payload: []byte(`{"action":"activate","space_id":"s","ip":"192.0.2.1"}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Relay the first two, then a fresh relay continues from the watermark
	// (the crash-replay shape: a new process resumes without duplicates).
	if n, err := relay.Tick(context.Background()); err != nil || n != 3 {
		t.Fatalf("first tick: n=%d err=%v", n, err)
	}
	// The watermark is at 3; a repeated relay of the same events must be
	// refused by the strict watermark (no duplicates), and new events relay.
	if err := source.Enqueue(context.Background(), Envelope{
		EventID: "evt-d", Version: 1, Entity: "ipam", Action: "release",
		Sequence: 4, Source: "dhcp", OccurredAt: testTime(), PayloadVersion: 1,
		Payload: []byte(`{"action":"release","space_id":"s","ip":"192.0.2.1"}`),
	}); err != nil {
		t.Fatal(err)
	}
	n, err := relay.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("the second tick relayed %d events, want 1", n)
	}
	if _, err := inbox.Pending(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
}

func TestRelayRejectsMismatchedDatabases(t *testing.T) {
	leaseDB, err := sql.Open("sqlite", t.TempDir()+"/lease.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = leaseDB.Close() })
	otherDB, err := sql.Open("sqlite", t.TempDir()+"/other.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherDB.Close() })
	source, err := NewObservationOutbox(leaseDB)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := NewObservationOutbox(otherDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRelay(source, inbox, leaseDB); !errors.Is(err, ErrRelayInvalid) {
		t.Fatalf("the inbox and the watermark must share a database, got %v", err)
	}
}
