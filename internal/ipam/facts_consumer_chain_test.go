package ipam

import (
	"context"
	"testing"

	"github.com/jasonwa/goddi/internal/facts"
)

// TestFactsConsumerVerifiesProducerChain proves the consumer accepts a clean
// producer-stamped chain while rejecting a tampered payload (leaving the event
// pending, i.e. blocked rather than silently applied).
func TestFactsConsumerVerifiesProducerChain(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutboxStamped(db, "dhcp_lease_facts")
	if err != nil {
		t.Fatal(err)
	}
	seedObservationEvent(t, outbox, 1, LeaseActionBind)
	consumer, err := NewFactsConsumer(NewLinkage(db), outbox)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if n, err := consumer.Replay(ctx, 10); err != nil || n != 1 {
		t.Fatalf("clean chain replay n=%d err=%v, want 1", n, err)
	}

	// Second event, then tamper its payload after stamping -> chain break.
	seedObservationEventWithID(t, outbox, "event-tamper", 2, LeaseActionRenew)
	if _, err := db.Exec(`UPDATE dhcp_ipam_observation_events SET payload = ? WHERE event_id = 'event-tamper'`,
		`{"action":"release","lease_id":"lease-1","scope_id":"scope-1","space_id":"sp1","ip":"192.0.2.10","mac":"aa:bb","hostname":"host"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := consumer.ProcessOne(ctx); err == nil {
		t.Fatal("tampered event was applied; chain verification did not fire")
	}
	// The event must remain pending (not marked done, watermark not advanced).
	var status string
	if err := db.QueryRow(`SELECT status FROM dhcp_ipam_observation_events WHERE event_id='event-tamper'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("tampered event status = %q, want pending (blocked)", status)
	}
}
