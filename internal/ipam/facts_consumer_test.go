package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/facts"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func seedObservationEvent(t *testing.T, outbox *facts.ObservationOutbox, sequence int64, action string) {
	t.Helper()
	payload, err := json.Marshal(LeaseObservationFact{Action: action, LeaseID: "lease-1", ScopeID: "scope-1", SpaceID: "sp1", IP: "192.0.2.10", MAC: "aa:bb", Hostname: "host"})
	if err != nil {
		t.Fatal(err)
	}
	event := facts.Envelope{EventID: "event-" + action, Version: facts.CurrentEnvelopeVersion, Entity: "dhcp_lease", Action: action, Generation: 1, Sequence: sequence, Source: "dhcp", OccurredAt: time.Now().UTC(), PayloadVersion: 1, Payload: payload}
	if err := outbox.Enqueue(context.Background(), event); err != nil {
		t.Fatal(err)
	}
}

func newConsumerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE dhcp_ipam_observation_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE, version INTEGER NOT NULL, entity TEXT NOT NULL,
		action TEXT NOT NULL, generation INTEGER NOT NULL, sequence INTEGER NOT NULL UNIQUE,
		source TEXT NOT NULL, occurred_at DATETIME NOT NULL, payload_version INTEGER NOT NULL,
		payload TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0,
		next_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')), last_error TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending', created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	); CREATE TABLE facts_projection_watermark (domain TEXT PRIMARY KEY, applied_seq INTEGER NOT NULL DEFAULT 0);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_spaces (id, name, description) VALUES ('sp1', 'space', '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES ('sn1', 'sp1', 'subnet', '192.0.2.0/24')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status, observed_state) VALUES ('a1', 'sn1', 'sp1', '192.0.2.10', 'available', 'unknown')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestFactsConsumerProjectsAndAdvancesAtomically(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, _ := facts.NewObservationOutbox(db)
	consumer, err := NewFactsConsumer(NewLinkage(db), outbox)
	if err != nil {
		t.Fatal(err)
	}
	seedObservationEvent(t, outbox, 1, LeaseActionBind)
	if n, err := consumer.Replay(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("replay = %d, %v", n, err)
	}
	if got, err := consumer.Applied(context.Background()); err != nil || got != 1 {
		t.Fatalf("watermark = %d, %v", got, err)
	}
	a, err := NewLinkage(db).Addresses().GetAddressBySpaceIP("sp1", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if a.ObservedState != address.ObservedInUse || a.DHCPLeaseID != "lease-1" {
		t.Fatalf("address observation = %+v", a)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM dhcp_ipam_observation_events WHERE event_id = 'event-bind'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "done" {
		t.Fatalf("event status = %q", status)
	}
}

func TestFactsConsumerRejectsSequenceGapAndKeepsEventPending(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, _ := facts.NewObservationOutbox(db)
	consumer, _ := NewFactsConsumer(NewLinkage(db), outbox)
	seedObservationEvent(t, outbox, 2, LeaseActionBind)
	if _, err := consumer.ProcessOne(context.Background()); !errors.Is(err, facts.ErrWatermarkGap) {
		t.Fatalf("error = %v, want gap", err)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM dhcp_ipam_observation_events WHERE event_id = 'event-bind'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("event status after gap = %q", status)
	}
}
