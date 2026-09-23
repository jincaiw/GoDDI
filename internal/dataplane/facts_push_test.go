package dataplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/facts"
)

func enqueueTestFact(t *testing.T, db *sql.DB, id string, seq int64) {
	t.Helper()
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"lease_id": "lease-1", "ip": "192.0.2.4"})
	err = outbox.Enqueue(context.Background(), facts.Envelope{
		EventID: id, Version: facts.CurrentEnvelopeVersion, Entity: "dhcp_lease", Action: "bind",
		Generation: 1, Sequence: seq, Source: "dhcp", OccurredAt: time.Now().UTC(), PayloadVersion: 1, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPushFactsCopiesImmutableEnvelopeAndReplaysWithoutResettingConsumerState(t *testing.T) {
	store := newStore(t, config.DataPlaneLease)
	control := newControlDB(t)
	rep := NewReplicator(control, store)
	enqueueTestFact(t, store.DB, "fact-1", 1)

	if n, err := rep.PendingFacts(); err != nil || n != 1 {
		t.Fatalf("initial pending = %d, %v; want 1", n, err)
	}
	if n, err := rep.PushFacts(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("first push = %d, %v; want 1", n, err)
	}
	var status string
	var sequence int64
	if err := control.QueryRow(`SELECT status, sequence FROM dhcp_ipam_observation_events WHERE event_id='fact-1'`).Scan(&status, &sequence); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || sequence != 1 {
		t.Fatalf("control inbox status/sequence = %q/%d", status, sequence)
	}
	if _, err := control.Exec(`UPDATE dhcp_ipam_observation_events SET status='done' WHERE event_id='fact-1'`); err != nil {
		t.Fatal(err)
	}
	// Simulate the crash window after remote commit but before producer ack.
	if _, err := store.Exec(`INSERT INTO dhcp_ipam_observation_event_dirty(event_id) VALUES ('fact-1')`); err != nil {
		t.Fatal(err)
	}
	if n, err := rep.PushFacts(context.Background(), 10); err != nil || n != 0 {
		t.Fatalf("replay = %d, %v; want idempotent no-op", n, err)
	}
	if err := control.QueryRow(`SELECT status FROM dhcp_ipam_observation_events WHERE event_id='fact-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "done" {
		t.Fatalf("replay reset consumer-owned status to %q", status)
	}
	if n, err := rep.PendingFacts(); err != nil || n != 0 {
		t.Fatalf("pending after replay = %d, %v", n, err)
	}
}

func TestPushFactsRetainsSequenceConflictForRetry(t *testing.T) {
	store := newStore(t, config.DataPlaneLease)
	control := newControlDB(t)
	rep := NewReplicator(control, store)
	_, err := control.Exec(`INSERT INTO dhcp_ipam_observation_events
		(event_id, version, entity, action, generation, sequence, source, occurred_at, payload_version, payload)
		VALUES ('different-event', 1, 'dhcp_lease', 'bind', 1, 1, 'dhcp', ?, 1, '{}')`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	enqueueTestFact(t, store.DB, "fact-1", 1)
	if n, err := rep.PushFacts(context.Background(), 10); err != nil || n != 0 {
		t.Fatalf("conflicting push = %d, %v; want refusal without fatal batch error", n, err)
	}
	if n, err := rep.PendingFacts(); err != nil || n != 1 {
		t.Fatalf("conflicted fact pending = %d, %v; want 1", n, err)
	}
	if n, err := rep.Refused(); err != nil || n != 1 {
		t.Fatalf("refused count = %d, %v; want 1", n, err)
	}
}
