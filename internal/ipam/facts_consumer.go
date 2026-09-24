package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jasonwa/goddi/internal/facts"
	"github.com/jasonwa/goddi/internal/ipam/address"
)

const IPAMFactsConsumerDomain = "ipam"

var (
	ErrFactsConsumerClosed = errors.New("ipam facts consumer: closed")
	ErrFactsConsumerFailed = errors.New("ipam facts consumer: failed")
)

// LeaseObservationFact is the versioned payload carried by a DHCP-derived
// facts.Envelope. SpaceID is resolved by the authoritative producer; keeping it
// in the payload lets the consumer project without querying DHCP tables.
type LeaseObservationFact struct {
	Action    string `json:"action"`
	LeaseID   string `json:"lease_id"`
	ScopeID   string `json:"scope_id"`
	SpaceID   string `json:"space_id"`
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Tombstone bool   `json:"tombstone,omitempty"`
}

// FactsConsumer is a restartable IPAM projection consumer. Callers explicitly
// own its lifecycle and it projects only durable facts from the control inbox.
type FactsConsumer struct {
	linkage   *Linkage
	outbox    *facts.ObservationOutbox
	watermark *facts.WatermarkStore

	lifecycleOnce sync.Once
	lifecycleMu   sync.Mutex
	wake          chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	started       bool
	state         FactsConsumerState
	lifecycleErr  error
	options       FactsConsumerOptions
}

func NewFactsConsumer(linkage *Linkage, outbox *facts.ObservationOutbox) (*FactsConsumer, error) {
	return NewFactsConsumerWithOptions(linkage, outbox, FactsConsumerOptions{})
}

// NewFactsConsumerWithOptions constructs an opt-in consumer with an explicit
// polling and batch policy. It does not start a goroutine.
func NewFactsConsumerWithOptions(linkage *Linkage, outbox *facts.ObservationOutbox, options FactsConsumerOptions) (*FactsConsumer, error) {
	if linkage == nil || linkage.db == nil || outbox == nil || outbox.Database() == nil {
		return nil, errors.New("ipam facts consumer: invalid dependencies")
	}
	// The supplied outbox is the consumer-side inbox. It must live with the
	// projection because ProcessOne commits inbox completion, watermark, and
	// IPAM writes in one transaction. Producer databases are separate and reach
	// this inbox through the data-plane relay.
	if linkage.db != outbox.Database() {
		return nil, errors.New("ipam facts consumer: control inbox and projection must share a database")
	}
	wm, err := facts.NewWatermarkStore(linkage.db)
	if err != nil {
		return nil, err
	}
	return &FactsConsumer{linkage: linkage, outbox: outbox, watermark: wm, options: options}, nil
}

// ProcessOne applies the oldest due event in one transaction. Projection,
// shared watermark, and outbox completion commit together; any error rolls all
// three back and leaves the durable fact pending for replay.
func (c *FactsConsumer) ProcessOne(ctx context.Context) (bool, error) {
	if c == nil || c.linkage == nil || c.outbox == nil {
		return false, ErrFactsConsumerClosed
	}
	events, err := c.outbox.Pending(ctx, 1)
	if err != nil {
		return false, err
	}
	if len(events) == 0 {
		return false, nil
	}
	event := events[0]
	var payload LeaseObservationFact
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return false, fmt.Errorf("ipam facts consumer: decode %s: %w", event.EventID, err)
	}
	if payload.Action == "" || payload.SpaceID == "" || payload.IP == "" {
		return false, fmt.Errorf("ipam facts consumer: invalid payload %s", event.EventID)
	}

	tx, err := c.linkage.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("ipam facts consumer: begin %s: %w", event.EventID, err)
	}
	defer func() { _ = tx.Rollback() }()
	advanced, err := c.watermark.AdvanceTx(ctx, tx, IPAMFactsConsumerDomain, event.Sequence)
	if err != nil {
		return false, err
	}
	if advanced {
		state, ok := observedStateFor(payload.Action)
		if !ok {
			return false, fmt.Errorf("ipam facts consumer: unknown lease action %q", payload.Action)
		}
		if _, err := c.linkage.addrMgr.ObserveBySpaceIPTx(tx, payload.SpaceID, payload.IP, address.Observation{
			State: state, MAC: payload.MAC, Hostname: payload.Hostname,
			Source: address.SourceDHCP, LeaseID: payload.LeaseID, Actor: "facts-consumer",
		}); err != nil {
			return false, fmt.Errorf("ipam facts consumer: project %s: %w", event.EventID, err)
		}
	}
	if err := c.outbox.MarkDoneTx(ctx, tx, event.EventID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("ipam facts consumer: commit %s: %w", event.EventID, err)
	}
	return true, nil
}

// Replay drains due events until the outbox is empty or the first fail-closed
// error. A failed projection remains durable and is not silently skipped.
func (c *FactsConsumer) Replay(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 10000
	}
	applied := 0
	for applied < limit {
		ok, err := c.ProcessOne(ctx)
		if err != nil {
			return applied, err
		}
		if !ok {
			return applied, nil
		}
		applied++
	}
	return applied, nil
}

func (c *FactsConsumer) Applied(ctx context.Context) (int64, error) {
	if c == nil || c.watermark == nil {
		return 0, ErrFactsConsumerClosed
	}
	return c.watermark.Applied(ctx, IPAMFactsConsumerDomain)
}

func (c *FactsConsumer) DB() *sql.DB {
	if c == nil || c.linkage == nil {
		return nil
	}
	return c.linkage.db
}
