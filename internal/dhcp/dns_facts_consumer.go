package dhcp

// The DNS side of the unified facts chain: a consumer that applies the
// unified lease facts to the DNS records. It is the only DNS producer.
//
// The chain is the same shape the IPAM projection uses: the lease plane
// commits durable facts beside its mutations, the data-plane relay copies
// them into this process's inbox, and this consumer applies each fact to the
// records and advances a watermark in one durable step. The fact payload
// carries everything ApplyOne needs (lease id, generation, scope, address,
// MAC, hostname), so no DNS-specific knowledge travels in the facts stream —
// the DNS derivation stays on the DNS side where the zones live.
//
// The apply is deliberately the same DNSLink.ApplyOne the former legacy
// queue used: idempotent, generation-tolerant, and reload-per-batch. A fact that fails to
// apply stays pending in the inbox for replay, exactly like a failed legacy
// event.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jincaiw/GoDDI/internal/facts"
)

// DNSFactsDomain is the watermark domain that tracks this consumer's
// progress in the inbox database, distinct from the IPAM projection's domain.
const DNSFactsDomain = "dns"

// dnsLeaseFact is the DNS side's view of the unified lease fact payload. It
// decodes only the fields the DNS records need; unknown fields are ignored so
// the payload can grow without touching this consumer.
type dnsLeaseFact struct {
	Action     string `json:"action"`
	LeaseID    string `json:"lease_id"`
	ScopeID    string `json:"scope_id"`
	IP         string `json:"ip"`
	MAC        string `json:"mac"`
	Hostname   string `json:"hostname"`
	Generation int64  `json:"generation"`
}

type DNSFactsConsumer struct {
	link      *DNSLink
	inbox     *facts.ObservationOutbox
	watermark *facts.WatermarkStore
	chain     *facts.ChainTipStore
	chainDom  string
}

// NewDNSFactsConsumer wires the consumer. The inbox must live in the same
// database as the records the link writes, so a fact's apply and its
// watermark advance stay orderable within one store.
func NewDNSFactsConsumer(link *DNSLink, inbox *facts.ObservationOutbox) (*DNSFactsConsumer, error) {
	if link == nil || inbox == nil || inbox.Database() == nil {
		return nil, fmt.Errorf("dns facts consumer: invalid dependencies")
	}
	wm, err := facts.NewWatermarkStore(inbox.Database())
	if err != nil {
		return nil, err
	}
	chain := facts.NewChainTipStore(inbox.Database())
	if err := chain.Ensure(context.Background()); err != nil {
		return nil, err
	}
	return &DNSFactsConsumer{link: link, inbox: inbox, watermark: wm, chain: chain, chainDom: DNSFactsDomain + ":hash_chain"}, nil
}

// toDNSEvent maps a unified lease fact onto the DNS record command it owes.
// A binding (bind/renew/activate) publishes the name; a release, expiry, or
// decline withdraws it — the same policy the legacy scope-aware sink applies
// to its queue.
func toDNSEvent(f dnsLeaseFact) (DNSEvent, error) {
	var action DNSEventAction
	switch f.Action {
	case "bind", "renew", "activate":
		action = DNSEventCreate
	case "release", "expire", "decline":
		action = DNSEventDelete
	default:
		return DNSEvent{}, fmt.Errorf("dns facts consumer: unknown lease action %q", f.Action)
	}
	if f.LeaseID == "" || f.IP == "" {
		return DNSEvent{}, fmt.Errorf("dns facts consumer: fact %s needs a lease id and an address", f.LeaseID)
	}
	return DNSEvent{
		LeaseID:    f.LeaseID,
		Generation: f.Generation,
		Action:     action,
		ScopeID:    f.ScopeID,
		IPAddress:  f.IP,
		MACAddress: f.MAC,
		Hostname:   f.Hostname,
	}, nil
}

// Tick applies the oldest due fact in the inbox. The apply, the inbox
// completion, and the watermark advance are orderable within the store: the
// apply runs first (it is idempotent, so a crash between the apply and the
// commit replays the same fact to the same end state), then the completion
// and the watermark commit together.
// Tick consumes one due fact. The first return says a fact was consumed
// (regardless of whether the record changed) and the second says the record
// actually changed. Keeping them apart matters: an idempotent no-op fact must
// not make Replay think the inbox is empty and abandon the facts behind it.
func (c *DNSFactsConsumer) Tick(ctx context.Context) (consumed bool, changed bool, err error) {
	events, err := c.inbox.Pending(ctx, 1)
	if err != nil {
		return false, false, err
	}
	if len(events) == 0 {
		return false, false, nil
	}
	event := events[0]
	var fact dnsLeaseFact
	if err := json.Unmarshal(event.Payload, &fact); err != nil {
		return false, false, fmt.Errorf("dns facts consumer: decode %s: %w", event.EventID, err)
	}
	dnsEvent, err := toDNSEvent(fact)
	if err != nil {
		return false, false, fmt.Errorf("dns facts consumer: %s: %w", event.EventID, err)
	}
	// D-03: verify the producer hash chain before applying; a fork/tamper leaves
	// the fact pending (blocked) rather than publishing a wrong record.
	var chainPrev, chainNext facts.ChainTip
	chainOn := event.HasChain()
	if chainOn {
		chainPrev, err = c.chain.Load(ctx, c.chainDom)
		if err != nil {
			return false, false, err
		}
		chainNext, err = facts.VerifyEnvelope(chainPrev, event)
		if err != nil {
			slog.Error("dns facts consumer: hash chain verification failed; fact stays pending",
				"event_id", event.EventID, "error", err)
			return false, false, fmt.Errorf("dns facts consumer: hash chain verification failed for %s: %w", event.EventID, err)
		}
	}
	wrote, err := c.link.ApplyOne(dnsEvent)
	if err != nil {
		// The fact stays pending for replay; the error is logged here because
		// a failed apply is otherwise invisible until someone reads the queue.
		slog.Error("dns facts consumer: apply failed; the fact stays pending for replay",
			"event_id", event.EventID, "sequence", event.Sequence, "error", err)
		return false, false, fmt.Errorf("dns facts consumer: apply %s: %w", event.EventID, err)
	}
	tx, err := c.inbox.Database().BeginTx(ctx, nil)
	if err != nil {
		return false, false, fmt.Errorf("dns facts consumer: begin %s: %w", event.EventID, err)
	}
	defer func() { _ = tx.Rollback() }()
	advanced, err := c.watermark.AdvanceTx(ctx, tx, DNSFactsDomain, event.Sequence)
	if err != nil {
		return false, false, err
	}
	_ = advanced
	if err := c.inbox.MarkDoneTx(ctx, tx, event.EventID); err != nil {
		return false, false, err
	}
	if chainOn {
		if err := c.chain.AdvanceTx(ctx, tx, c.chainDom, chainPrev.Tip, chainNext); err != nil {
			return false, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, false, fmt.Errorf("dns facts consumer: commit %s: %w", event.EventID, err)
	}
	return true, wrote, nil
}

// Replay drains due facts until the inbox is empty or the first fail-closed
// error. Returns the number of facts that changed records.
func (c *DNSFactsConsumer) Replay(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 10000
	}
	applied := 0
	changedAny := false
	for applied < limit {
		consumed, changed, err := c.Tick(ctx)
		if err != nil {
			// Records written before the failure are still durable; publish
			// them to the in-memory zone even when a later fact failed.
			if changedAny {
				c.link.ReloadZones()
			}
			return applied, err
		}
		if !consumed {
			break
		}
		if changed {
			changedAny = true
			applied++
		}
	}
	// The DNS server answers from memory. A row written by ApplyOne is not
	// resolvable until the zone store is reloaded; doing it once per batch
	// keeps the reload cost proportional to the batch, not to each fact.
	if changedAny {
		c.link.ReloadZones()
	}
	return applied, nil
}
