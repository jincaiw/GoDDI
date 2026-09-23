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
	seedObservationEventWithID(t, outbox, "event-"+action, sequence, action)
}

func seedObservationEventWithID(t *testing.T, outbox *facts.ObservationOutbox, eventID string, sequence int64, action string) {
	t.Helper()
	payload, err := json.Marshal(LeaseObservationFact{Action: action, LeaseID: "lease-1", ScopeID: "scope-1", SpaceID: "sp1", IP: "192.0.2.10", MAC: "aa:bb", Hostname: "host"})
	if err != nil {
		t.Fatal(err)
	}
	event := facts.Envelope{EventID: eventID, Version: facts.CurrentEnvelopeVersion, Entity: "dhcp_lease", Action: action, Generation: 1, Sequence: sequence, Source: "dhcp", OccurredAt: time.Now().UTC(), PayloadVersion: 1, Payload: payload}
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

func TestFactsConsumerLifecycleStartsAndWakeDrainsWithoutPolling(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, _ := facts.NewObservationOutbox(db)
	consumer, err := NewFactsConsumerWithOptions(NewLinkage(db), outbox, FactsConsumerOptions{
		PollInterval: time.Hour,
		BatchSize:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := consumer.Status(context.Background())
	if err != nil || status.State != FactsConsumerIdle || status.Started || status.NextExpectedSequence != 1 || status.Lag != 0 || status.Gap || status.Readiness() != FactsReadinessUnconfigured {
		t.Fatalf("initial status = %+v, %v", status, err)
	}
	if err := consumer.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = consumer.Stop(context.Background()) }()
	seedObservationEvent(t, outbox, 1, LeaseActionBind)
	consumer.Wake()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, err = consumer.Status(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if status.LastApplied == 1 && status.Pending == 0 && status.Failed == 0 && status.Lag == 0 && !status.Gap && status.Running && status.Readiness() == FactsReadinessOK {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("consumer did not drain after wake: %+v", status)
}

func TestFactsConsumerReadinessReportsDegradedBacklog(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, _ := facts.NewObservationOutbox(db)
	consumer, err := NewFactsConsumer(NewLinkage(db), outbox)
	if err != nil {
		t.Fatal(err)
	}
	seedObservationEvent(t, outbox, 1, LeaseActionBind)
	status, err := consumer.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Pending != 1 || status.Failed != 0 || status.LastApplied != 0 || status.NextExpectedSequence != 1 || status.HeadSequence != 1 || status.Lag != 1 || status.Gap || status.Readiness() != FactsReadinessUnconfigured {
		t.Fatalf("backlog status = %+v", status)
	}
	// Readiness is a pure interpretation of a point-in-time status. Starting
	// the consumer here would race its immediate replay against this assertion.
	status.State = FactsConsumerRunning
	status.Started = true
	status.Running = true
	if status.Readiness() != FactsReadinessDegraded {
		t.Fatalf("running backlog readiness = %q, status=%+v", status.Readiness(), status)
	}
}

func TestFactsConsumerLifecycleRetainsFailureAndLeavesEventPending(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, _ := facts.NewObservationOutbox(db)
	consumer, err := NewFactsConsumerWithOptions(NewLinkage(db), outbox, FactsConsumerOptions{PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	seedObservationEvent(t, outbox, 2, LeaseActionBind)
	if err := consumer.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, statusErr := consumer.Status(context.Background())
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if status.State == FactsConsumerFailed {
			if !errors.Is(consumer.Start(context.Background()), ErrFactsConsumerFailed) {
				t.Fatal("second Start should remain fail-closed")
			}
			if status.LastError == "" || status.Pending != 1 || status.Failed != 0 || status.LastApplied != 0 || status.NextExpectedSequence != 1 || status.HeadSequence != 2 || status.Lag != 2 || !status.Gap || status.Readiness() != FactsReadinessFailing {
				t.Fatalf("failure status = %+v", status)
			}
			if err := consumer.Stop(context.Background()); !errors.Is(err, ErrFactsConsumerFailed) {
				t.Fatalf("Stop after failure = %v", err)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("consumer did not enter failed state")
}

func TestFactsConsumerLifecycleStopIsIdempotentAndWakeNeverBlocks(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, _ := facts.NewObservationOutbox(db)
	consumer, err := NewFactsConsumer(NewLinkage(db), outbox)
	if err != nil {
		t.Fatal(err)
	}
	consumer.Wake()
	if err := consumer.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	consumer.Wake()
	status, err := consumer.Status(context.Background())
	if err != nil || status.State != FactsConsumerStopped || status.Readiness() != FactsReadinessStopped {
		t.Fatalf("stopped status = %+v, %v", status, err)
	}
}
