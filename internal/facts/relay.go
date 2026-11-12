package facts

// The data-plane relay: the bridge between the lease plane's producer outbox
// and the control plane's consumer inbox.
//
// The lease plane commits durable facts beside its lease mutations in the
// lease database. The control plane's consumer commits inbox completion,
// shared watermark, and IPAM projection in ONE transaction — which requires
// the inbox to live with the projection (the control database). The relay
// copies events across the two databases in strict sequence order, advancing
// a relay watermark in the same transaction as each insert, so a crash
// replays idempotently and never skips or duplicates a fact.
//
// The relay is a copier, not an authority: it changes no event payload, keeps
// the producer's event IDs and sequences, and never marks source rows done
// (the producer's own lifecycle — HA acknowledgement or retention — owns
// those). Its only durable state is the relay watermark in the inbox
// database, advanced exactly one sequence per relayed event.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RelayDomain is the watermark domain that tracks the relay's progress in the
// inbox database. It is distinct from the consumer's own domain so the relay
// and the projection can be restarted, repaired, and observed independently.
const RelayDomain = "facts_relay"

var ErrRelayInvalid = errors.New("facts relay: invalid wiring")

type Relay struct {
	source    *ObservationOutbox // the lease plane's producer outbox
	inbox     *ObservationOutbox // the control plane's consumer inbox
	watermark *WatermarkStore    // the relay progress, living in the inbox database
	batch     int
}

// NewRelay wires the relay. The inbox and the relay watermark must share a
// database (the projection's), because each relayed event commits its insert
// and its watermark advance in one transaction.
func NewRelay(source, inbox *ObservationOutbox, relayDB *sql.DB) (*Relay, error) {
	if source == nil || source.db == nil || inbox == nil || inbox.db == nil || relayDB == nil {
		return nil, ErrRelayInvalid
	}
	if inbox.db != relayDB {
		return nil, fmt.Errorf("%w: the inbox and the relay watermark must share a database", ErrRelayInvalid)
	}
	wm, err := NewWatermarkStore(relayDB)
	if err != nil {
		return nil, err
	}
	return &Relay{source: source, inbox: inbox, watermark: wm, batch: 100}, nil
}

// Tick relays every source event after the relay watermark, in strict
// sequence order. Each event commits its inbox insert and the watermark
// advance together; a failure rolls both back and leaves the fact for replay.
// Returns the number of events relayed.
func (r *Relay) Tick(ctx context.Context) (int, error) {
	if r == nil || r.source == nil || r.inbox == nil || r.watermark == nil {
		return 0, ErrRelayInvalid
	}
	applied, err := r.watermark.Applied(ctx, RelayDomain)
	if err != nil {
		return 0, fmt.Errorf("facts relay: watermark: %w", err)
	}
	relayed := 0
	for {
		events, err := r.source.After(ctx, applied, r.batch)
		if err != nil {
			return relayed, fmt.Errorf("facts relay: source read: %w", err)
		}
		if len(events) == 0 {
			return relayed, nil
		}
		for _, event := range events {
			if err := r.relayOne(ctx, event); err != nil {
				return relayed, err
			}
			applied = event.Sequence
			relayed++
		}
		if len(events) < r.batch {
			return relayed, nil
		}
	}
}

func (r *Relay) relayOne(ctx context.Context, event Envelope) error {
	tx, err := r.inbox.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("facts relay: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.inbox.EnqueueTx(ctx, tx, event); err != nil {
		return fmt.Errorf("facts relay: enqueue %s: %w", event.EventID, err)
	}
	advanced, err := r.watermark.AdvanceTx(ctx, tx, RelayDomain, event.Sequence)
	if err != nil {
		return fmt.Errorf("facts relay: watermark %d: %w", event.Sequence, err)
	}
	if !advanced {
		return fmt.Errorf("facts relay: watermark did not advance at %d", event.Sequence)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("facts relay: commit %d: %w", event.Sequence, err)
	}
	return nil
}

// Run ticks the relay on an interval until the context is cancelled. A tick
// error is durable-state-safe (each event commits atomically), so the loop
// logs and retries rather than failing the process; the caller's logger sees
// the failures through the returned error channel.
func (r *Relay) Run(ctx context.Context, interval time.Duration, onError func(error)) error {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := r.Tick(ctx); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}
