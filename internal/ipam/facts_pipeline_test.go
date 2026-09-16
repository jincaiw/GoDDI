package ipam

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/facts"
	"github.com/jasonwa/goddi/internal/ipam/address"
)

func TestFactsPipelineDisabledIsSafeNoOp(t *testing.T) {
	pipeline, err := NewFactsPipeline(nil, nil, FactsPipelineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pipeline.Enabled() {
		t.Fatal("disabled pipeline is enabled")
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatalf("disabled Start: %v", err)
	}
	pipeline.Wake()
	if err := pipeline.Stop(context.Background()); err != nil {
		t.Fatalf("disabled Stop: %v", err)
	}
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Consumer.State != FactsConsumerIdle || status.Consumer.Started {
		t.Fatalf("disabled status = %+v", status)
	}
}

func TestFactsPipelineStartFailsClosedBeforeConsumerStarts(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	gateErr := errors.New("producer recovery incomplete")
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled:     true,
		BeforeStart: func(context.Context) error { return gateErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); !errors.Is(err, gateErr) {
		t.Fatalf("Start error = %v, want %v", err, gateErr)
	}
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Consumer.Started || status.Consumer.Running || status.Consumer.State != FactsConsumerIdle {
		t.Fatalf("failed start status = %+v", status)
	}
}

func TestFactsPipelineRetriesAfterBeforeStartFailure(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	gateErr := errors.New("producer recovery incomplete")
	beforeStartCalls := 0
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled: true,
		ConsumerOptions: FactsConsumerOptions{
			PollInterval: time.Hour,
		},
		BeforeStart: func(context.Context) error {
			beforeStartCalls++
			if beforeStartCalls == 1 {
				return gateErr
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); !errors.Is(err, gateErr) {
		t.Fatalf("first Start error = %v, want %v", err, gateErr)
	}
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Consumer.Started || status.Consumer.Running || status.Consumer.State != FactsConsumerIdle {
		t.Fatalf("failed first start status = %+v", status)
	}
	if beforeStartCalls != 1 {
		t.Fatalf("BeforeStart calls after first attempt = %d, want 1", beforeStartCalls)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatalf("retry Start: %v", err)
	}
	status, err = pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Consumer.Started || !status.Consumer.Running || status.Consumer.State != FactsConsumerRunning {
		t.Fatalf("successful retry status = %+v", status)
	}
	if beforeStartCalls != 2 {
		t.Fatalf("BeforeStart calls after retry = %d, want 2", beforeStartCalls)
	}
	if err := pipeline.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFactsPipelineRejectsNilLifecycleContextsBeforeCallbacks(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	beforeStartCalls := 0
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled: true,
		BeforeStart: func(context.Context) error {
			beforeStartCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var nilContext context.Context
	if err := pipeline.Start(nilContext); err == nil {
		t.Fatal("nil Start context unexpectedly accepted")
	}
	if beforeStartCalls != 0 {
		t.Fatalf("BeforeStart calls = %d, want 0", beforeStartCalls)
	}
	if err := pipeline.Stop(nilContext); err == nil {
		t.Fatal("nil Stop context unexpectedly accepted")
	}
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Consumer.Started || status.Consumer.State != FactsConsumerIdle {
		t.Fatalf("nil context status = %+v", status)
	}
}

func TestFactsPipelineStartAndStopAreIdempotentAtWrapperBoundary(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); err == nil {
		t.Fatal("duplicate Start unexpectedly succeeded")
	}
	if err := pipeline.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Stop(context.Background()); err != nil {
		t.Fatalf("duplicate Stop: %v", err)
	}
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Consumer.State != FactsConsumerStopped || status.Consumer.Running {
		t.Fatalf("idempotent lifecycle status = %+v", status)
	}
}

func TestFactsPipelineConcurrentStartAllowsOnlyOneRunner(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var successes, failures int
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := pipeline.Start(context.Background())
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				failures++
			}
		}()
	}
	close(start)
	wg.Wait()
	if successes != 1 || failures != 7 {
		t.Fatalf("concurrent starts = successes:%d failures:%d", successes, failures)
	}
	if err := pipeline.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFactsPipelineConcurrentStopIsSafe(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := pipeline.Stop(context.Background())
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Stop: %v", err)
		}
	}
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Consumer.State != FactsConsumerStopped || status.Consumer.Running {
		t.Fatalf("concurrent stop status = %+v", status)
	}
}

func TestFactsPipelineEnabledStartsAndStopsConsumer(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled: true,
		ConsumerOptions: FactsConsumerOptions{
			PollInterval: time.Hour,
			BatchSize:    1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Consumer.Started || status.Consumer.Readiness() != FactsReadinessUnconfigured {
		t.Fatalf("pre-start status = %+v", status)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err = pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Consumer.Started || !status.Consumer.Running {
		t.Fatalf("running status = %+v", status)
	}
	if err := pipeline.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err = pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Consumer.State != FactsConsumerStopped || status.Consumer.Readiness() != FactsReadinessStopped {
		t.Fatalf("stopped status = %+v", status)
	}
}

func TestFactsPipelineReportsLagAsDegradedWhileRunning(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled:         true,
		ConsumerOptions: FactsConsumerOptions{PollInterval: time.Hour},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedObservationEvent(t, outbox, 1, LeaseActionBind)
	if _, err := db.Exec(`UPDATE dhcp_ipam_observation_events SET next_attempt_at = datetime('now', '+1 hour')`); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pipeline.Stop(context.Background()) }()
	status, err := pipeline.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Consumer.Pending != 1 || status.Consumer.Lag != 1 || status.Consumer.Readiness() != FactsReadinessDegraded {
		t.Fatalf("lag status = %+v", status)
	}
}

func TestFactsPipelineReportsGapAndFailedBacklogAsFailing(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	seedObservationEvent(t, outbox, 2, LeaseActionBind)
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled:         true,
		ConsumerOptions: FactsConsumerOptions{PollInterval: time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, statusErr := pipeline.Status(context.Background())
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if status.Consumer.State == FactsConsumerFailed {
			if status.Consumer.Readiness() != FactsReadinessFailing || !status.Consumer.Gap || status.Consumer.Pending != 1 {
				t.Fatalf("gap status = %+v", status)
			}
			if err := pipeline.Stop(context.Background()); !errors.Is(err, ErrFactsConsumerFailed) {
				t.Fatalf("stop after gap = %v", err)
			}
			if err := outbox.MarkRetry(context.Background(), "event-bind", "gap retained", 1); err != nil {
				t.Fatal(err)
			}
			failedStatus, err := pipeline.Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if failedStatus.Consumer.Failed != 1 || failedStatus.Consumer.Readiness() != FactsReadinessFailing {
				t.Fatalf("failed backlog status = %+v", failedStatus)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("pipeline did not fail closed on a sequence gap")
}

func TestFactsPipelineWakeDrainsCommittedProducerFact(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled:         true,
		ConsumerOptions: FactsConsumerOptions{PollInterval: time.Hour, BatchSize: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(LeaseObservationFact{Action: LeaseActionBind, LeaseID: "lease-1", ScopeID: "scope-1", SpaceID: "sp1", IP: "192.0.2.10", MAC: "aa:bb", Hostname: "host"})
	if err != nil {
		t.Fatal(err)
	}
	event := facts.Envelope{EventID: "event-pipeline-bind", Version: facts.CurrentEnvelopeVersion, Entity: "dhcp_lease", Action: LeaseActionBind, Generation: 1, Sequence: 1, Source: "dhcp", OccurredAt: time.Now().UTC(), PayloadVersion: 1, Payload: payload}
	if err := outbox.Enqueue(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	pipeline.Wake()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, statusErr := pipeline.Status(context.Background())
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if status.Consumer.LastApplied == 1 && status.Consumer.Pending == 0 && status.Consumer.Lag == 0 {
			observed, err := NewLinkage(db).Addresses().GetAddressBySpaceIP("sp1", "192.0.2.10")
			if err != nil {
				t.Fatal(err)
			}
			if observed.ObservedState != address.ObservedInUse || observed.DHCPLeaseID != "lease-1" {
				t.Fatalf("projected address = %+v", observed)
			}
			if err := pipeline.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("pipeline did not drain committed fact after wake")
}

func TestFactsPipelineWakeDrainsFactsMutationAfterCommit(t *testing.T) {
	db := newConsumerDB(t)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, enabled) VALUES ('scope-writer', 'writer', '192.0.2.0/24', '192.0.2.20', '192.0.2.29', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE facts_sequence_allocator (domain TEXT PRIMARY KEY, last_sequence INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status, observed_state) VALUES ('a2', 'sn1', 'sp1', '192.0.2.20', 'available', 'unknown')`); err != nil {
		t.Fatal(err)
	}
	manager := lease.NewManager(db)
	offered, err := manager.ReserveAddress("scope-writer", "192.0.2.20", "aa:bb:cc:dd:ee:20", "writer-host")
	if err != nil {
		t.Fatal(err)
	}
	allocator, err := facts.NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := facts.NewObservationOutbox(db)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := lease.NewFactsMutationWriter(manager, allocator, outbox)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := NewFactsPipeline(NewLinkage(db), outbox, FactsPipelineOptions{
		Enabled:         true,
		ConsumerOptions: FactsConsumerOptions{PollInterval: time.Hour, BatchSize: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	writer.WithPostCommitWake(pipeline.Wake)
	if err := pipeline.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ActivateLease(context.Background(), "", "dhcp-node-a", "sp1", offered.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, statusErr := pipeline.Status(context.Background())
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if status.Consumer.LastApplied == 1 && status.Consumer.Pending == 0 {
			observed, err := NewLinkage(db).Addresses().GetAddressBySpaceIP("sp1", "192.0.2.20")
			if err != nil {
				t.Fatal(err)
			}
			if observed.ObservedState != address.ObservedInUse {
				t.Fatalf("projected address = %+v", observed)
			}
			if err := pipeline.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("pipeline did not drain facts mutation after commit wake")
}

func TestFactsPipelineRejectsEnabledSplitStorageBeforeStart(t *testing.T) {
	producer := newConsumerDB(t)
	t.Cleanup(func() { _ = producer.Close() })
	control := newConsumerDB(t)
	t.Cleanup(func() { _ = control.Close() })
	outbox, err := facts.NewObservationOutbox(producer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFactsPipeline(NewLinkage(control), outbox, FactsPipelineOptions{Enabled: true}); err == nil {
		t.Fatal("enabled split-storage pipeline unexpectedly accepted")
	}
}
