package dhcp

import (
	"testing"
)

func TestOutbox_EnqueueKeepsOrder(t *testing.T) {
	db := newLinkageDB(t)
	outbox := NewDNSOutbox(db)

	// A create followed by the delete that supersedes it must both be
	// replayable, in that order: the consumer has to be able to reach the right
	// end state from any starting point, which coalescing would prevent.
	if err := outbox.Enqueue(DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventCreate,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("enqueue create: %v", err)
	}
	if err := outbox.Enqueue(DNSEvent{
		LeaseID: "lease-1", Generation: 1, Action: DNSEventDelete,
		IPAddress: "192.0.2.10", Hostname: "host1",
	}); err != nil {
		t.Fatalf("enqueue delete: %v", err)
	}

	events, err := outbox.PendingBatch(10)
	if err != nil {
		t.Fatalf("PendingBatch: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Action != DNSEventCreate || events[1].Action != DNSEventDelete {
		t.Fatalf("order = (%s, %s), want (create, delete)", events[0].Action, events[1].Action)
	}
	if events[0].ID >= events[1].ID {
		t.Errorf("ids not increasing: %d then %d", events[0].ID, events[1].ID)
	}
	if events[0].IPAddress != "192.0.2.10" || events[0].Hostname != "host1" {
		t.Errorf("event payload lost: %+v", events[0])
	}
}

func TestOutbox_MarkDoneRetiresTheEvent(t *testing.T) {
	db := newLinkageDB(t)
	outbox := NewDNSOutbox(db)

	if err := outbox.Enqueue(DNSEvent{LeaseID: "l", Generation: 1, Action: DNSEventCreate}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	events, err := outbox.PendingBatch(10)
	if err != nil {
		t.Fatalf("PendingBatch: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if err := outbox.MarkDone(events[0].ID); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	again, err := outbox.PendingBatch(10)
	if err != nil {
		t.Fatalf("PendingBatch after done: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("pending after done = %d, want 0", len(again))
	}
}

func TestOutbox_RetryBacksOffThenAbandons(t *testing.T) {
	db := newLinkageDB(t)
	outbox := NewDNSOutbox(db)

	if err := outbox.Enqueue(DNSEvent{LeaseID: "l", Generation: 1, Action: DNSEventCreate}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	events, err := outbox.PendingBatch(10)
	if err != nil {
		t.Fatalf("PendingBatch: %v", err)
	}
	id := events[0].ID

	// The first failure must push the event into the future rather than leave
	// it immediately due: retrying a broken write at full speed turns a
	// transient failure into a busy loop.
	if err := outbox.MarkRetry(id, "boom"); err != nil {
		t.Fatalf("MarkRetry: %v", err)
	}
	due, err := outbox.PendingBatch(10)
	if err != nil {
		t.Fatalf("PendingBatch after retry: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("event still due immediately after a failure: %d", len(due))
	}

	var attempts int
	var lastError string
	if err := db.QueryRow(`SELECT attempts, last_error FROM dhcp_dns_events WHERE id = ?`, id).
		Scan(&attempts, &lastError); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if attempts != 1 || lastError != "boom" {
		t.Errorf("attempts/last_error = (%d, %q), want (1, %q)", attempts, lastError, "boom")
	}

	// Keep failing until the attempt budget is spent.
	for i := 1; i < DNSOutboxMaxAttempts; i++ {
		// Make the event due again so the retry is not blocked by backoff.
		if _, err := db.Exec(
			`UPDATE dhcp_dns_events SET next_attempt_at=datetime('now') WHERE id=?`, id); err != nil {
			t.Fatalf("force due: %v", err)
		}
		if err := outbox.MarkRetry(id, "boom"); err != nil {
			t.Fatalf("MarkRetry #%d: %v", i+1, err)
		}
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM dhcp_dns_events WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	// Abandoned, not deleted: the drift has to stay visible so it can be
	// alerted on instead of vanishing.
	if status != "failed" {
		t.Errorf("status = %q, want failed after %d attempts", status, DNSOutboxMaxAttempts)
	}

	pending, failed, err := outbox.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if pending != 0 || failed != 1 {
		t.Errorf("stats = (pending %d, failed %d), want (0, 1)", pending, failed)
	}
}

func TestOutbox_RetryBackoffIsBounded(t *testing.T) {
	db := newLinkageDB(t)
	outbox := NewDNSOutbox(db)

	if err := outbox.Enqueue(DNSEvent{LeaseID: "l", Generation: 1, Action: DNSEventCreate}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	events, _ := outbox.PendingBatch(1)
	id := events[0].ID

	// Walk the backoff and check it never exceeds the cap; an unbounded shift
	// would overflow the duration and could park the event forever.
	for i := 0; i < DNSOutboxMaxAttempts-1; i++ {
		if err := outbox.MarkRetry(id, "boom"); err != nil {
			t.Fatalf("MarkRetry #%d: %v", i+1, err)
		}
		var scheduled string
		if err := db.QueryRow(
			`SELECT next_attempt_at FROM dhcp_dns_events WHERE id = ?`, id).Scan(&scheduled); err != nil {
			t.Fatalf("read schedule: %v", err)
		}
		var delaySeconds float64
		if err := db.QueryRow(
			`SELECT (julianday(next_attempt_at) - julianday('now')) * 86400 FROM dhcp_dns_events WHERE id = ?`,
			id).Scan(&delaySeconds); err != nil {
			t.Fatalf("measure delay: %v", err)
		}
		if delaySeconds <= 0 {
			t.Fatalf("attempt %d scheduled in the past: %f", i+1, delaySeconds)
		}
		if delaySeconds > DNSOutboxRetryMax.Seconds()+5 {
			t.Fatalf("attempt %d delay %.0fs exceeds the %.0fs cap",
				i+1, delaySeconds, DNSOutboxRetryMax.Seconds())
		}
	}
}

func TestOutbox_RejectsIncompleteEvents(t *testing.T) {
	db := newLinkageDB(t)
	outbox := NewDNSOutbox(db)

	if err := outbox.Enqueue(DNSEvent{Action: DNSEventCreate}); err == nil {
		t.Error("expected an error when the lease id is missing")
	}
	if err := outbox.Enqueue(DNSEvent{LeaseID: "l"}); err == nil {
		t.Error("expected an error when the action is missing")
	}
}

func TestOutbox_EventsSurviveReopening(t *testing.T) {
	db := newLinkageDB(t)
	outbox := NewDNSOutbox(db)
	if err := outbox.Enqueue(DNSEvent{
		LeaseID: "l", Generation: 3, Action: DNSEventDelete,
		IPAddress: "192.0.2.7", Hostname: "h",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// A second handle on the same store is what a restarted process sees.
	reopened := NewDNSOutbox(db)
	events, err := reopened.PendingBatch(10)
	if err != nil {
		t.Fatalf("PendingBatch: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events after reopen = %d, want 1", len(events))
	}
	if events[0].Generation != 3 || events[0].Action != DNSEventDelete {
		t.Errorf("event = (%d, %s), want (3, delete)", events[0].Generation, events[0].Action)
	}
	if events[0].Attempts != 0 {
		t.Errorf("attempts = %d, want 0", events[0].Attempts)
	}
}
