package facts

import (
	"context"
	"database/sql"
	"testing"
)

func newRelayDB(t *testing.T) *sql.DB {
	t.Helper()
	db := newFactsOutboxDB(t)
	if _, err := db.Exec(`CREATE TABLE facts_projection_watermark (
		domain TEXT PRIMARY KEY,
		applied_seq INTEGER NOT NULL DEFAULT 0 CHECK (applied_seq >= 0)
	)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func countEvents(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events`).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func seedRelayEvents(t *testing.T, ob *ObservationOutbox, from, to int64) {
	t.Helper()
	for i := from; i <= to; i++ {
		if err := ob.Enqueue(context.Background(), mkEnv(t, i, `{"i":`+string(rune('0'+i))+`}`)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
}

// TestRelayDeliversAndResumesWithoutDuplicates proves the relay copies events
// in order, advances its watermark, is idempotent on re-tick, and resumes from
// the watermark after new events arrive (a restart/crash in the middle).
func TestRelayDeliversAndResumesWithoutDuplicates(t *testing.T) {
	src := newRelayDB(t)
	inbox := newRelayDB(t)
	sob, _ := NewObservationOutbox(src)
	iob, _ := NewObservationOutbox(inbox)
	relay, err := NewRelay(sob, iob, inbox)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedRelayEvents(t, sob, 1, 3)

	if n, err := relay.Tick(ctx); err != nil || n != 3 {
		t.Fatalf("first tick n=%d err=%v, want 3", n, err)
	}
	if c := countEvents(t, inbox); c != 3 {
		t.Fatalf("inbox count = %d, want 3", c)
	}
	// Re-tick is a no-op: the watermark already covers everything.
	if n, err := relay.Tick(ctx); err != nil || n != 0 {
		t.Fatalf("re-tick n=%d err=%v, want 0", n, err)
	}
	if c := countEvents(t, inbox); c != 3 {
		t.Fatalf("inbox count after re-tick = %d, want 3 (no duplicates)", c)
	}
	// "Restart": a fresh Relay over the same DBs resumes from the watermark.
	relay2, _ := NewRelay(sob, iob, inbox)
	seedRelayEvents(t, sob, 4, 5)
	if n, err := relay2.Tick(ctx); err != nil || n != 2 {
		t.Fatalf("resume tick n=%d err=%v, want 2", n, err)
	}
	if c := countEvents(t, inbox); c != 5 {
		t.Fatalf("inbox count after resume = %d, want 5", c)
	}
	// The relay is a copier, never an authority: it does not mark source rows.
	var status string
	if err := src.QueryRow(`SELECT status FROM dhcp_ipam_observation_events WHERE sequence=1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("source row status = %q, want pending (relay must not mark it)", status)
	}
}

// TestRelayOutageKeepsEventsAndRecovers proves an inbox outage fails the tick
// without advancing the watermark or losing source events, and that recovery
// delivers everything exactly once.
func TestRelayOutageKeepsEventsAndRecovers(t *testing.T) {
	src := newRelayDB(t)
	inbox := newRelayDB(t)
	sob, _ := NewObservationOutbox(src)
	iob, _ := NewObservationOutbox(inbox)
	relay, _ := NewRelay(sob, iob, inbox)
	ctx := context.Background()
	seedRelayEvents(t, sob, 1, 3)

	// Simulate the inbox being unavailable.
	if _, err := inbox.Exec(`ALTER TABLE dhcp_ipam_observation_events RENAME TO hidden_events`); err != nil {
		t.Fatal(err)
	}
	if _, err := relay.Tick(ctx); err == nil {
		t.Fatal("tick succeeded while inbox was unavailable")
	}
	if c := countEvents(t, src); c != 3 {
		t.Fatalf("source events lost during outage: %d, want 3", c)
	}
	var applied int64
	_ = inbox.QueryRow(`SELECT applied_seq FROM facts_projection_watermark WHERE domain=?`, RelayDomain).Scan(&applied)
	if applied != 0 {
		t.Fatalf("relay watermark advanced to %d during outage, want 0", applied)
	}

	// Recovery.
	if _, err := inbox.Exec(`ALTER TABLE hidden_events RENAME TO dhcp_ipam_observation_events`); err != nil {
		t.Fatal(err)
	}
	if n, err := relay.Tick(ctx); err != nil || n != 3 {
		t.Fatalf("recovery tick n=%d err=%v, want 3", n, err)
	}
	if c := countEvents(t, inbox); c != 3 {
		t.Fatalf("inbox count after recovery = %d, want 3 (exactly once)", c)
	}
}
