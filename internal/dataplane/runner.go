package dataplane

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jasonwa/goddi/internal/metrics"
)

// RunnerConfig configures a replication loop.
type RunnerConfig struct {
	// Domains to replicate.
	Domains []Domain
	// Interval between polls. The poll is a single indexed read of the
	// control-plane counter.
	//
	// This interval is one of the terms bounding how long a client waits, not
	// only how long a configuration change takes to be seen. A record derived
	// from a binding has to cross the control database to reach the plane that
	// serves it, so the interval appears twice in that path -- once on the way
	// up, once on the way down -- plus the consumer's own drain. It is why the
	// default is short; see DefaultSyncIntervalSeconds.
	Interval time.Duration
	// PushLeases reports that this store owns lease state. It is what enables
	// the lease push, the one-off takeover of the control database's lease
	// rows, and the two queues that ride with them: the DHCP event log and the
	// binding-to-DNS outbox.
	//
	// It is false for a DNS store, which syncs the scopes -- it needs them to
	// qualify a client's short hostname -- but owns no lease and must not
	// claim the control database's rows.
	PushLeases bool
	// PushRecords reports that this store owns the DNS records derived from
	// DHCP bindings. It enables the record push and the zone-serial push.
	//
	// Separate from PushLeases because it is the other plane's obligation: the
	// records are written where they are served from, and the plane that
	// serves them is the one that has to report them.
	PushRecords bool
	// PushBatch caps how many changes one pass moves up.
	PushBatch int
	// Quota bounds the store's local footprint. Crossing a bound does not
	// stop anything; it raises the readiness level so that an operator sees
	// it before the disk does. See Quota.
	Quota Quota
	// OnApplied is called after a domain's replica is replaced, for the work
	// the store cannot do for itself (reloading an in-memory zone table).
	OnApplied func(d Domain)
}

// Runner keeps a data plane's copy of the control-plane configuration current.
type Runner struct {
	rep *Replicator
	cfg RunnerConfig

	// wake carries a hint that there is work to do now. Buffered with room for
	// one: a pass drains whatever has accumulated, so a hundred signals during
	// a pass are worth exactly one more pass, and none of them is worth making
	// the caller wait.
	wake chan struct{}

	mu               sync.Mutex
	controlReachable bool
	lastError        string
	lastSuccess      time.Time

	// probe is the tiered readiness as of the last pass, and probed says
	// whether one has happened. Guards the same mutex as the fields above:
	// they are written by the loop and read by the readiness endpoint.
	probe  Readiness
	probed bool
}

// NewRunner builds a replication loop.
func NewRunner(rep *Replicator, cfg RunnerConfig) *Runner {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Duration(0)
	}
	if cfg.PushBatch <= 0 {
		cfg.PushBatch = 500
	}
	return &Runner{rep: rep, cfg: cfg, wake: make(chan struct{}, 1)}
}

// Wake asks the loop to run a pass now instead of waiting for the tick.
//
// It is how a change this process just made stops being hostage to a timer.
// The work is already durable -- a DHCP binding is committed and its DNS event
// queued before the client is answered -- and the only thing standing between
// that row and the control database is a poll interval. Waking the loop removes
// that interval from the path, which is one of the two terms in the delay a
// client sees before its own name resolves.
//
// It is safe to call from the request path: the send never blocks, and the
// push it triggers happens in the loop's goroutine, so the caller never waits
// on the control database. It is also safe to call when the loop is not
// running or is already busy -- the signal is dropped or coalesced, and the
// next tick picks the work up as before.
func (r *Runner) Wake() {
	if r.wake == nil {
		return
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Store exposes the store the runner replicates into.
func (r *Runner) Store() *Store { return r.rep.store }

// Quota reports the bounds this runner's store is measured against.
func (r *Runner) Quota() Quota { return r.cfg.Quota }

// Replicator exposes the replicator.
func (r *Runner) Replicator() *Replicator { return r.rep }

// Prime performs the startup pass and decides whether this process may serve.
//
// The rule it enforces is the difference between two outages that look alike
// from the outside:
//
//   - The control plane is unreachable and the store holds a configuration:
//     start, serve it, and say so. Yesterday's scopes are better than no DHCP.
//   - The control plane is unreachable and the store is empty: refuse to start.
//     A DHCP server with no scopes accepts no clients while looking healthy,
//     which is worse than not starting because nothing will page anybody.
func (r *Runner) Prime(ctx context.Context) error {
	var firstErr error

	for _, d := range r.cfg.Domains {
		// The takeover is for the store that owns leases. A DNS process also
		// syncs DomainDHCP -- it needs the scopes to qualify a client's short
		// hostname -- but it does not own lease state, and copying the control
		// database's lease rows into it would both duplicate what DomainLeases
		// already brings across and mark the outbox entries it should still
		// consume as already seen.
		if d == DomainDHCP && r.cfg.PushLeases {
			if _, err := r.rep.TakeOverLeases(ctx); err != nil {
				// A takeover failure is only fatal when it would have had
				// something to move; not being able to tell is not a reason to
				// refuse to serve leases that are already here.
				slog.Warn("dataplane: could not take over lease state from the control database",
					"error", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}

		res, err := r.rep.Sync(ctx, d)
		if err != nil {
			r.recordFailure(err)
			// One question for the whole store, not one per domain: a DNS
			// process that holds its zones must start while the control
			// database is down even though it holds no DHCP scopes.
			has, hasErr := r.rep.store.HasAnyReplica(r.cfg.Domains)
			if hasErr != nil {
				return hasErr
			}
			if !has {
				return errors.Join(
					errors.New("dataplane: no local configuration to serve and the control database is unreachable"),
					err)
			}
			slog.Warn("dataplane: serving the local configuration; the control database is unreachable",
				"domain", d, "error", err)
			continue
		}
		r.recordSuccess()
		if res.Applied {
			slog.Info("dataplane: configuration loaded from the control database",
				"domain", d, "revision", res.Revision, "rows", res.Rows,
				"retained_scopes", len(res.RetainedScopes), "dropped_leases", res.DroppedLeases)
			r.notify(d)
		} else {
			slog.Info("dataplane: local configuration already current",
				"domain", d, "revision", res.Revision)
		}
	}

	r.pushUp(ctx)
	r.refreshProbe()
	return nil
}

// pushUp moves everything this store owes the control database.
//
// Every one of these is off the request path and every failure has the same
// shape: the change stays queued and the next pass retries it. None of them is
// reported to a client, because none of them is a reason to refuse an address
// or a name.
func (r *Runner) pushUp(ctx context.Context) {
	// Every push below fails the same way: the work stays queued and the next
	// pass retries it, so there is nothing to hand back to a caller -- only a
	// line in the log and a count.
	//
	// The count is the part an operator needs. A control database that refuses
	// writes is otherwise invisible on this plane: DNS keeps resolving and
	// clients keep getting addresses, exactly as the split intends, while the
	// console quietly stops reflecting anything. Without a number here that
	// looks like everything is working.
	onPushFailure := func(what string, err error) {
		slog.Warn("dataplane: pushing "+what+" up failed; it stays queued", "error", err)
		metrics.RecordDBError()
	}

	if r.cfg.PushLeases {
		res, err := r.rep.Push(ctx, r.cfg.PushBatch)
		if err != nil {
			onPushFailure("lease changes", err)
		} else if res.Upserted > 0 || res.Deleted > 0 || res.Refused > 0 {
			slog.Debug("dataplane: pushed lease changes",
				"upserted", res.Upserted, "deleted", res.Deleted,
				"refused", res.Refused, "pending", res.Pending)
		}

		// The outbox and the event log ride with the lease push: they are
		// written from the same request, and losing them costs the two things
		// a lease row alone cannot carry -- the DNS side's work, and the
		// console's record that anything happened at all.
		if n, err := r.rep.PushEvents(ctx, r.cfg.PushBatch); err != nil {
			onPushFailure("queued DNS work", err)
		} else if n > 0 {
			slog.Debug("dataplane: pushed queued DNS work", "entries", n)
		}
		if n, err := r.rep.PushLogs(ctx, r.cfg.PushBatch); err != nil {
			onPushFailure("the DHCP event log", err)
		} else if n > 0 {
			slog.Debug("dataplane: pushed DHCP event log entries", "entries", n)
		}

		// A lease state change is audited in this store, so its entries are
		// owed upward from here. They used to ride with the record push alone,
		// which is the DNS plane's -- so on a split deployment every lease
		// entry the DHCP plane wrote stayed in its store and never reached the
		// archive, which is the one place it is read from.
		r.pushAuditLogs(ctx)
	}

	if r.cfg.PushRecords {
		res, err := r.rep.PushRecords(ctx, r.cfg.PushBatch)
		if err != nil {
			onPushFailure("DNS records", err)
		} else if res.Upserted > 0 || res.Deleted > 0 || res.Refused > 0 {
			slog.Debug("dataplane: pushed DNS records",
				"upserted", res.Upserted, "deleted", res.Deleted,
				"refused", res.Refused, "pending", res.Pending)
		}
		// A serial this store has already advertised has to reach the control
		// plane before the console computes the next one from a value that
		// predates it. See PushZoneSerials.
		if n, err := r.rep.PushZoneSerials(ctx, r.cfg.PushBatch); err != nil {
			onPushFailure("zone serials", err)
		} else if n > 0 {
			slog.Debug("dataplane: pushed zone serials", "zones", n)
		}

		// The other writer of audit entries on this plane: the RFC 2136 handler
		// and the DHCP-to-DNS consumer both run where the records do, and both
		// record what they did while they do it.
		r.pushAuditLogs(ctx)
	}
}

// pushAuditLogs carries the entries this store wrote up to the archive.
//
// Called from both planes because both write them: the lease store records
// lease transitions, the zone store records the records it published. Each
// store keeps its own queue, so this is per-runner state, not a shared one.
func (r *Runner) pushAuditLogs(ctx context.Context) {
	n, err := r.rep.PushAuditLogs(ctx, r.cfg.PushBatch)
	if err != nil {
		slog.Warn("dataplane: pushing audit entries up failed; they stay queued", "error", err)
		metrics.RecordDBError()
	} else if n > 0 {
		slog.Debug("dataplane: pushed audit entries", "entries", n)
	}
}

// Run polls until the context is cancelled.
//
// The tick is the fallback, not the mechanism: it is what covers changes this
// process cannot observe directly -- another plane's push, a console edit, a
// configuration published while this one was down. A change this process made
// itself calls Wake instead, so the interval does not have to be short enough
// for the worst case on both sides of the control database.
func (r *Runner) Run(ctx context.Context) {
	if r.cfg.Interval <= 0 {
		slog.Info("dataplane: replication loop disabled (no interval)")
		return
	}
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Once(ctx)
		case <-r.wake:
			r.Once(ctx)
		}
	}
}

// Once performs one replication pass and reports what it did.
func (r *Runner) Once(ctx context.Context) {
	for _, d := range r.cfg.Domains {
		res, err := r.rep.Sync(ctx, d)
		if err != nil {
			r.recordFailure(err)
			slog.Warn("dataplane: configuration sync failed; the local copy stays in service",
				"domain", d, "error", err)
			// Counted as well as logged. This is the read side of "the control
			// database is not answering", and on this plane it is the only
			// place that failure surfaces at all: the local copy stays in
			// service by design, so nothing else in the process goes wrong.
			metrics.RecordDBError()
			continue
		}
		r.recordSuccess()
		if !res.Applied {
			continue
		}
		if len(res.RetainedScopes) > 0 {
			// The console said this scope was deleted; the data plane is still
			// serving it because clients hold addresses in it. Saying nothing
			// would leave an operator believing the deletion took effect.
			slog.Warn("dataplane: retaining scopes that still hold leases",
				"scope_ids", res.RetainedScopes)
		}
		if res.DroppedLeases > 0 {
			slog.Warn("dataplane: removed leases whose scope no longer exists",
				"count", res.DroppedLeases)
		}
		slog.Info("dataplane: configuration updated", "domain", d,
			"revision", res.Revision, "rows", res.Rows)
		r.notify(d)
	}

	r.pushUp(ctx)
	r.refreshProbe()
}

func (r *Runner) notify(d Domain) {
	if r.cfg.OnApplied != nil {
		r.cfg.OnApplied(d)
	}
}

func (r *Runner) recordFailure(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.controlReachable = false
	r.lastError = err.Error()
}

func (r *Runner) recordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.controlReachable = true
	r.lastError = ""
	r.lastSuccess = time.Now()
}

// Status is the probe view of the replication loop.
type Status struct {
	// ControlReachable is the outcome of the last replication attempt.
	ControlReachable bool
	// LastError is why it failed, empty when it did not.
	LastError string
	// LastSuccess is when replication last completed.
	LastSuccess time.Time
	// Domains is the per-domain replication state recorded in the store.
	Domains map[Domain]SyncState
	// PendingLeaseChanges are lease changes owed to the control database.
	PendingLeaseChanges int
	// HeldLeases are the leases this store is currently holding.
	HeldLeases int
	// PendingDNSEvents are binding-to-DNS changes owed to the DNS side.
	PendingDNSEvents int
	// PendingDHCPLogs are event log entries owed to the control database.
	PendingDHCPLogs int
	// PendingRecords are DNS records owed to the control database.
	PendingRecords int
	// PendingZoneSerials are zone serial bumps owed to the control database.
	PendingZoneSerials int
	// PendingAuditLogs are audit entries owed to the control database.
	PendingAuditLogs int
	// RefusedRows are queued changes across every upward queue that the control
	// database has declined and that are waiting out a retry delay.
	//
	// It is one number rather than one per queue, and it is the number that
	// distinguishes a backlog from a refusal: PendingLeaseChanges says work is
	// owed, this says the far side will not take it. Which queue a refusal came
	// from is in the log line recordPushFailures writes, and in the marker
	// table itself.
	RefusedRows int
}

// QueueDepth is one upward queue and how much is waiting in it.
type QueueDepth struct {
	Name    string
	Pending int
}

// PendingByQueue names every upward queue, in a fixed order.
//
// One function rather than a list at each call site. The readiness endpoint,
// the Prometheus sample and the quota bounds all need this set, and three
// copies of it are three chances for a queue to be visible in one place and
// missing in the others -- which is how a backlog that nothing bounds goes
// unnoticed until the disk does.
//
// The order is fixed because it is what the breach list, the log line and the
// metric series are compared against.
func (s Status) PendingByQueue() []QueueDepth {
	return []QueueDepth{
		{Name: "lease_changes", Pending: s.PendingLeaseChanges},
		{Name: "dns_events", Pending: s.PendingDNSEvents},
		{Name: "dhcp_logs", Pending: s.PendingDHCPLogs},
		{Name: "records", Pending: s.PendingRecords},
		{Name: "zone_serials", Pending: s.PendingZoneSerials},
		{Name: "audit_logs", Pending: s.PendingAuditLogs},
	}
}

// Snapshot reads the probe view. Every field is best effort: a probe that
// cannot read its own store should still answer.
func (r *Runner) Snapshot() Status {
	r.mu.Lock()
	st := Status{
		ControlReachable: r.controlReachable,
		LastError:        r.lastError,
		LastSuccess:      r.lastSuccess,
		Domains:          map[Domain]SyncState{},
	}
	r.mu.Unlock()

	for _, d := range r.cfg.Domains {
		if s, err := r.rep.store.State(d); err == nil {
			st.Domains[d] = s
		}
	}
	if r.cfg.PushLeases {
		if n, err := r.rep.PendingLeases(); err == nil {
			st.PendingLeaseChanges = n
		}
		if n, err := r.rep.store.HeldLeases(); err == nil {
			st.HeldLeases = n
		}
		if n, err := r.rep.PendingEvents(); err == nil {
			st.PendingDNSEvents = n
		}
		if n, err := r.rep.PendingLogs(); err == nil {
			st.PendingDHCPLogs = n
		}
	}
	if r.cfg.PushRecords {
		if n, err := r.rep.PendingRecords(); err == nil {
			st.PendingRecords = n
		}
		if n, err := r.rep.PendingZoneSerials(); err == nil {
			st.PendingZoneSerials = n
		}
		if n, err := r.rep.PendingAuditLogs(); err == nil {
			st.PendingAuditLogs = n
		}
	}
	// Every queue, not only the ones this role pushes: a refused row is a fact
	// about the queue, and the role that is not pushing it today is the one
	// that will inherit it tomorrow.
	if n, err := r.rep.Refused(); err == nil {
		st.RefusedRows = n
	}
	return st
}
