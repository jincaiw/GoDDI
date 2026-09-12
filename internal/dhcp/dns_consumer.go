package dhcp

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// The far half of the DHCP-to-DNS linkage: the loop that drains the outbox and
// the slow sweep that repairs what the outbox could not see.
//
// It lives with the records rather than with the leases, because what it writes
// is dns_records and the plane that serves a record is the plane that has to
// write it. In a split deployment the queue it drains is a replica: the DHCP
// plane recorded the entries locally, pushed them to the control database, and
// this process pulled them down. Its only input is therefore its own store, and
// it keeps working while the DHCP plane is being restarted.
//
// Ownership is the point of the arrangement. A record's owner is the process
// that can answer for it, so a withdrawal decided here is decided by the same
// process that was serving the name -- no round trip to ask the DHCP side
// whether a binding still exists.

// How often the queue is polled, and how much of it one wake-up takes.
//
// The poll is deliberately not tuned for latency: an entry reaches this store
// only after the DHCP plane has pushed it up and this plane has pulled it down,
// so both of those polls bound the latency and polling this queue faster than
// once a second buys nothing an operator could observe.
const (
	dnsEventInterval  = time.Second
	dnsEventBatchSize = 64

	// The reconciler is deliberately slow. It exists only to repair drift the
	// queue could not observe: a crash between the lease commit and the event
	// insert, an entry abandoned after its retries, and a record published from
	// a lease replica that had not caught up yet.
	dnsReconcileEvery = 5 * time.Minute
	dnsReconcileLimit = 200
)

// DNSConsumer drains the durable DNS event queue of one store and applies each
// entry to that store's records.
type DNSConsumer struct {
	link   *DNSLink
	outbox *DNSOutbox
}

// NewDNSConsumer builds a consumer for the store the link writes to.
func NewDNSConsumer(db *sql.DB, link *DNSLink) *DNSConsumer {
	return &DNSConsumer{link: link, outbox: NewDNSOutbox(db)}
}

// Run drains the queue and sweeps for drift until the context is cancelled.
func (c *DNSConsumer) Run(ctx context.Context) {
	drain := time.NewTicker(dnsEventInterval)
	defer drain.Stop()
	sweep := time.NewTicker(dnsReconcileEvery)
	defer sweep.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-drain.C:
			c.Drain()
		case <-sweep.C:
			c.Sweep()
		}
	}
}

// Drain applies the due events in id order, retiring each one that succeeded
// and rescheduling or abandoning each one that failed.
//
// A failure is never swallowed: the entry stays in the queue with its error
// recorded, so the update is retried instead of being lost the way a bare
// fire-and-forget goroutine would lose it.
func (c *DNSConsumer) Drain() int {
	applied := 0

	// Bound the work per wake-up so a large backlog cannot monopolise the
	// connection, and so the sweep still gets a turn.
	for pass := 0; pass < 4; pass++ {
		events, err := c.outbox.PendingBatch(dnsEventBatchSize)
		if err != nil {
			slog.Error("dns consumer: failed to read the DNS event queue", "error", err)
			return applied
		}
		if len(events) == 0 {
			return applied
		}

		changed := false
		for _, e := range events {
			wrote, err := c.link.ApplyOne(e)
			if err != nil {
				if rerr := c.outbox.MarkRetry(e.ID, err.Error()); rerr != nil {
					slog.Error("dns consumer: failed to reschedule a DNS event",
						"event_id", e.ID, "error", rerr)
				}
				continue
			}
			changed = changed || wrote
			if wrote {
				applied++
			}
			if err := c.outbox.MarkDone(e.ID); err != nil {
				slog.Error("dns consumer: failed to retire a DNS event",
					"event_id", e.ID, "error", err)
			}
		}
		// One reload per batch rather than one per entry: the zone table is
		// re-read in full, and an entry that changed nothing does not need it.
		if changed {
			c.link.ReloadZones()
		}
		if len(events) < dnsEventBatchSize {
			return applied
		}
	}
	return applied
}

// Sweep repairs drift the queue could not observe and reports how many repairs
// it made.
func (c *DNSConsumer) Sweep() int {
	repaired, err := c.link.Reconcile(dnsReconcileLimit)
	if err != nil {
		slog.Error("dns consumer: reconciliation failed", "error", err)
		return 0
	}
	if repaired > 0 {
		slog.Info("dns consumer: reconciliation repaired records", "count", repaired)
	}
	if pending, failed, err := c.outbox.Stats(); err == nil && failed > 0 {
		// Surfaced on its own line rather than only in the log so an operator
		// has a standing signal that some names are still unpublished.
		slog.Warn("dns consumer: the DNS event queue has abandoned entries",
			"pending", pending, "failed", failed)
	}
	return repaired
}

// Stats reports the queue's depth, for the probe.
func (c *DNSConsumer) Stats() (pending, failed int64, err error) {
	return c.outbox.Stats()
}
