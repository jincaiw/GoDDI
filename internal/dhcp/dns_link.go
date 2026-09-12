package dhcp

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/miekg/dns"
)

// ZoneReloader is the slice of the authoritative DNS zone store the DHCP side
// needs. Records written here go straight into dns_records, but queries are
// answered from the in-memory zone store, so a write that is not followed by a
// reload is invisible: the row exists and the name still does not resolve.
type ZoneReloader interface {
	// ReloadNow reloads the zone store synchronously. The debounced Reload()
	// is not sufficient here — it only guarantees "eventually", and the whole
	// point of the linkage is that a confirmed binding becomes resolvable.
	ReloadNow()
}

// Stores names the two databases the DHCP-to-DNS linkage touches.
//
// They are separate fields because they belong to different planes. The lease
// store holds the data plane's own rows, and the DNS store holds zones and
// records, which belong to the DNS plane.
//
// Every construction that exists today passes Same(), and the two fields hold
// the same handle -- deliberately, and for the reason the type was introduced:
// after the split, the DNS plane reads the lease *replica* that the control
// plane pushes down to it, so the binding and the record really do live in one
// database. What the field names buy is that this is visible here rather than
// implied by a single handle, and that pointing a read at the other database is
// a one-line change in the caller instead of a hunt through the applier.
//
// The request path no longer goes through this type at all: server.New takes a
// lease database and nothing else, so a handler cannot reach the DNS store even
// by mistake. That is the guarantee, and it is the compiler's, not a test's.
type Stores struct {
	Lease *sql.DB
	DNS   *sql.DB
}

// Same returns stores that are a single database: the arrangement both the
// single-process installation and a split one end up with, because the lease a
// DNS plane reads is the replica it holds next to its zones. It is the only
// constructor used outside tests, which is why the doc comment above says what
// the two fields are for rather than pretending they currently differ.
func Same(db *sql.DB) Stores { return Stores{Lease: db, DNS: db} }

// DNSLink handles the DHCP to DNS half of the linkage: publishing and
// withdrawing the A/PTR records of a binding.
type DNSLink struct {
	stores   Stores
	reloader ZoneReloader
}

// NewDNSLink creates a new DNS link manager. reloader may be nil, in which case
// changes are written but no reload is triggered (only useful in tests).
func NewDNSLink(stores Stores, reloader ZoneReloader) *DNSLink {
	return &DNSLink{stores: stores, reloader: reloader}
}

// SetZoneReloader attaches the zone store to reload after each change.
func (dl *DNSLink) SetZoneReloader(r ZoneReloader) { dl.reloader = r }

func (dl *DNSLink) reload() {
	if dl.reloader != nil {
		dl.reloader.ReloadNow()
	}
}

// ApplyEvent applies one durable DNS change for a lease binding.
//
// It is idempotent and generation-guarded, which is what makes replay safe:
//
//   - create is applied only when the lease is still a confirmed binding at the
//     exact generation the event was recorded at. A create replayed after the
//     binding was torn down (or after a later renewal superseded it) is
//     dropped instead of resurrecting a name that is no longer owned.
//   - delete removes only rows owned by that lease at or below the recorded
//     generation. A delete replayed after a renewal therefore leaves the
//     renewed binding's record alone.
func (dl *DNSLink) ApplyEvent(e DNSEvent) error {
	changed, err := dl.ApplyOne(e)
	if err != nil {
		return err
	}
	if changed {
		dl.reload()
	}
	return nil
}

// ApplyOne applies a single event and reports whether anything was written. It
// does not reload, so a queue driver can apply a whole batch and reload once
// instead of paying a full zone re-read per event.
func (dl *DNSLink) ApplyOne(e DNSEvent) (bool, error) {
	switch e.Action {
	case DNSEventCreate:
		return dl.applyCreate(e)
	case DNSEventDelete:
		return dl.applyDelete(e)
	default:
		return false, fmt.Errorf("dhcp_dns_link: unknown action %q", e.Action)
	}
}

// ReloadZones republishes the in-memory zone data. Call it once after a batch
// of ApplyOne calls that reported a change.
func (dl *DNSLink) ReloadZones() { dl.reload() }

// applyCreate publishes the A and PTR records for a confirmed binding.
//
// The generation check here is deliberately tolerant, and the reason is the
// split deployment. In one process the lease row and the event are committed
// moments apart against the same database, so "the lease is not active at this
// generation" is a reliable statement. In a split deployment the DNS plane reads
// a replica that is filled by replication and can be behind: a binding confirmed
// a second ago may not be in it yet, and a binding whose delete event is still
// queued may already be released in it. Waiting for the replica to catch up
// would mean most creates were dropped and only the reconciler published names,
// minutes late.
//
// So the rule is: refuse only what the replica positively proves.
//
//   - A later generation means a later transition already owns this name;
//     publishing at the older generation would revive it.
//   - The same generation with a status that is no longer a binding means the
//     binding was released or expired after the event was recorded.
//   - Everything else -- a replica that is behind, or that has no row for this
//     lease at all -- publishes from the event's own snapshot. The event is a
//     durable record of a binding that was confirmed and acknowledged, and the
//     reconciler withdraws the name if the binding has since gone.
func (dl *DNSLink) applyCreate(e DNSEvent) (bool, error) {
	var status string
	var generation int64
	var hostname, leaseEnd, leaseScope sql.NullString
	var known bool

	err := dl.stores.Lease.QueryRow(`
		SELECT status, generation, hostname, lease_end, scope_id FROM dhcp_leases WHERE id = ?`,
		e.LeaseID).Scan(&status, &generation, &hostname, &leaseEnd, &leaseScope)
	switch {
	case err == sql.ErrNoRows:
		known = false
		slog.Debug("dhcp_dns_link: publishing from the event's snapshot; the lease is not in this store yet",
			"lease_id", e.LeaseID)
	case err != nil:
		return false, fmt.Errorf("dhcp_dns_link: reading lease %s: %w", e.LeaseID, err)
	default:
		known = true
	}

	if known && superseded(status, generation, e.Generation) {
		slog.Debug("dhcp_dns_link: skipping superseded create",
			"lease_id", e.LeaseID, "event_generation", e.Generation,
			"replica_generation", generation, "replica_status", status)
		return false, nil
	}

	// Prefer the hostname recorded on the lease: the event carries a snapshot,
	// but the lease is the source of truth if the client changed it.
	name := e.Hostname
	if hostname.Valid && hostname.String != "" {
		name = hostname.String
	}

	// The lease is the source of truth for the scope; the event's copy is a
	// snapshot taken before the write and may lag. Without a scope a short
	// client hostname cannot be qualified, and the name would silently never be
	// published.
	scopeID := e.ScopeID
	if leaseScope.Valid && leaseScope.String != "" {
		scopeID = leaseScope.String
	}

	cleaned := dl.qualifyName(scopeID, name)
	if cleaned == "" {
		slog.Debug("dhcp_dns_link: lease has no usable hostname, nothing to publish",
			"lease_id", e.LeaseID)
		return false, nil
	}

	zoneID, zoneName, err := dl.findForwardZone(cleaned)
	if err != nil {
		return false, fmt.Errorf("dhcp_dns_link: locating forward zone for %s: %w", cleaned, err)
	}
	if zoneID == "" {
		slog.Debug("dhcp_dns_link: no matching forward zone", "hostname", cleaned)
		return false, nil
	}

	// The zone name has to match at a label boundary: a plain suffix test would
	// accept "evil-example.test." for the zone "example.test.".
	if _, ok := relativeNameInZone(cleaned, zoneName); !ok {
		slog.Debug("dhcp_dns_link: hostname does not belong to zone",
			"hostname", cleaned, "zone", zoneName)
		return false, nil
	}

	// expires_at is only known when the replica has the lease. A record
	// published without it is not unbounded: the reconciler treats a missing
	// expiry as unfinished work and fills it in from the lease as soon as the
	// replica catches up.
	expiry := ""
	if known && leaseEnd.Valid {
		expiry = leaseEnd.String
	}

	if err := dl.upsertARecord(e, zoneID, dns.Fqdn(cleaned), expiry); err != nil {
		return false, err
	}
	if err := dl.upsertPTRRecord(e, dns.Fqdn(cleaned), expiry); err != nil {
		return false, err
	}

	slog.Info("dhcp_dns_link: published DNS records",
		"hostname", cleaned, "ip", e.IPAddress, "zone", zoneName, "lease_id", e.LeaseID,
		"from_event_snapshot", !known)

	// Read back what was actually written rather than describing what the
	// event asked for: a name that already had a row, or a missing reverse
	// zone, both make the two differ.
	published, err := dl.recordsOwnedBy(e.LeaseID)
	if err != nil {
		slog.Error("dhcp_dns_link: could not read back records for the audit entry",
			"lease_id", e.LeaseID, "error", err)
	} else {
		dl.auditRecordChange(e, AuditActionRecordCreate, "", encodeAuditRecords(published))
	}
	return true, nil
}

// superseded reports whether the lease replica proves an event is stale: a
// later generation already owns the name, or the binding described by that
// generation is no longer held.
func superseded(replicaStatus string, replicaGeneration, eventGeneration int64) bool {
	if replicaGeneration > eventGeneration {
		return true
	}
	return replicaGeneration == eventGeneration && replicaStatus != string(lease.LeaseStatusActive)
}

// upsertARecord writes the forward record for a binding.
//
// Two rules matter here:
//
//   - A name has at most one DHCP-driven A row. If another lease (or a legacy
//     owner='dhcp' row with no owner_ref) currently holds the name, its row is
//     removed first, so the name follows the binding that was confirmed last
//     instead of accumulating one A per past client.
//   - The row is pinned to the lease and generation that wrote it, which is
//     what lets a later teardown delete only its own record.
//
// expires_at is set to the lease end. It is a second line of defence: even if
// every teardown path failed, the record stops being served once the binding
// that justified it has expired, instead of pointing at an address the pool may
// have since handed to somebody else.
func (dl *DNSLink) upsertARecord(e DNSEvent, zoneID, fqdn, leaseEnd string) error {
	tx, err := dl.stores.DNS.Begin()
	if err != nil {
		return fmt.Errorf("dhcp_dns_link: begin A record transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(`
		DELETE FROM dns_records
		WHERE zone_id = ? AND name = ? AND type = 'A' AND owner = 'dhcp'
		  AND (owner_ref IS NULL OR owner_ref <> ?)`,
		zoneID, fqdn, e.LeaseID); err != nil {
		return fmt.Errorf("dhcp_dns_link: releasing name %s from previous lease: %w", fqdn, err)
	}

	if _, err := tx.Exec(`
		INSERT INTO dns_records
			(id, zone_id, name, type, value, ttl, enabled, owner, owner_ref,
			 owner_generation, expires_at, authored_locally)
		VALUES (
			COALESCE((SELECT id FROM dns_records
			          WHERE zone_id = ? AND name = ? AND type = 'A' AND owner_ref = ?),
			         LOWER(HEX(RANDOMBLOB(16)))),
			?, ?, 'A', ?, 300, 1, 'dhcp', ?, ?, datetime(?), 1
		)
		ON CONFLICT(id) DO UPDATE SET
			value = excluded.value,
			enabled = 1,
			owner = 'dhcp',
			owner_ref = excluded.owner_ref,
			owner_generation = excluded.owner_generation,
			expires_at = excluded.expires_at,
			authored_locally = 1,
			updated_at = datetime('now')`,
		zoneID, fqdn, e.LeaseID,
		zoneID, fqdn, e.IPAddress, e.LeaseID, e.Generation, leaseEnd); err != nil {
		return fmt.Errorf("dhcp_dns_link: writing A record %s -> %s: %w", fqdn, e.IPAddress, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dhcp_dns_link: commit A record: %w", err)
	}
	return nil
}

// upsertPTRRecord writes the reverse record for a binding. PTR names are
// derived from the address, so two leases never contend for the same name the
// way they can for a hostname.
func (dl *DNSLink) upsertPTRRecord(e DNSEvent, fqdn, leaseEnd string) error {
	zoneID, zoneName, err := dl.findReverseZone(e.IPAddress)
	if err != nil || zoneID == "" {
		return nil // No reverse zone configured, nothing to publish.
	}

	reverseName := reverseIP(e.IPAddress) + ".in-addr.arpa"
	if _, ok := relativeNameInZone(reverseName, zoneName); !ok {
		return nil
	}
	// Zone membership only needs the label-boundary check; the name that gets
	// stored is the fully qualified one, matching how every other writer
	// records a name.
	owner := dns.Fqdn(reverseName)

	target := fqdn
	if !strings.HasSuffix(target, ".") {
		target += "."
	}

	tx, err := dl.stores.DNS.Begin()
	if err != nil {
		return fmt.Errorf("dhcp_dns_link: begin PTR transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(`
		DELETE FROM dns_records
		WHERE zone_id = ? AND name = ? AND type = 'PTR' AND owner = 'dhcp'
		  AND (owner_ref IS NULL OR owner_ref <> ?)`,
		zoneID, owner, e.LeaseID); err != nil {
		return fmt.Errorf("dhcp_dns_link: releasing PTR %s from previous lease: %w", owner, err)
	}

	if _, err := tx.Exec(`
		INSERT INTO dns_records
			(id, zone_id, name, type, value, ttl, enabled, owner, owner_ref,
			 owner_generation, expires_at, authored_locally)
		VALUES (
			COALESCE((SELECT id FROM dns_records
			          WHERE zone_id = ? AND name = ? AND type = 'PTR' AND owner_ref = ?),
			         LOWER(HEX(RANDOMBLOB(16)))),
			?, ?, 'PTR', ?, 300, 1, 'dhcp', ?, ?, datetime(?), 1
		)
		ON CONFLICT(id) DO UPDATE SET
			value = excluded.value,
			enabled = 1,
			owner = 'dhcp',
			owner_ref = excluded.owner_ref,
			owner_generation = excluded.owner_generation,
			expires_at = excluded.expires_at,
			authored_locally = 1,
			updated_at = datetime('now')`,
		zoneID, owner, e.LeaseID,
		zoneID, owner, target, e.LeaseID, e.Generation, leaseEnd); err != nil {
		return fmt.Errorf("dhcp_dns_link: writing PTR %s -> %s: %w", owner, target, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dhcp_dns_link: commit PTR record: %w", err)
	}
	return nil
}

// applyDelete withdraws the records written for a binding.
//
// Ownership is matched on the lease, not on owner='dhcp'. Matching only the
// string meant that two clients announcing the same hostname shared one name,
// so tearing down the older binding removed the newer client's live record.
// The generation bound adds the time dimension: a delete decided at generation
// N must not remove a record written at generation > N, which is exactly the
// case of a renewal that landed while the delete was queued.
//
// Rows written before migration 018 have no owner_ref. They are matched by
// owner='dhcp' plus the value, so a legacy record is still cleaned up without
// risking a record that belongs to somebody else.
func (dl *DNSLink) applyDelete(e DNSEvent) (bool, error) {
	fqdn := dl.qualifyName(e.ScopeID, e.Hostname)

	// Read the before state while it still exists. After the delete there is
	// nothing left to describe, and an audit entry that says only "a record was
	// withdrawn" cannot answer the question anyone actually asks.
	withdrawn, err := dl.recordsOwnedBy(e.LeaseID)
	if err != nil {
		slog.Error("dhcp_dns_link: could not read records for the audit entry",
			"lease_id", e.LeaseID, "error", err)
		withdrawn = nil
	}

	removed, err := dl.deleteOwnedRecords(e.LeaseID, e.Generation, e.IPAddress, fqdn)
	if err != nil {
		return false, err
	}

	if removed > 0 {
		slog.Info("dhcp_dns_link: withdrew DNS records",
			"hostname", e.Hostname, "ip", e.IPAddress, "removed", removed, "lease_id", e.LeaseID)
		dl.auditRecordChange(e, AuditActionRecordDelete, encodeAuditRecords(withdrawn), "")
		return true, nil
	}
	slog.Debug("dhcp_dns_link: delete matched no owned record",
		"hostname", e.Hostname, "ip", e.IPAddress, "lease_id", e.LeaseID)
	return false, nil
}

// qualifyName turns the name a client reports into a fully qualified name.
//
// DHCP clients commonly send option 12 — a short, NetBIOS-style label with no
// domain. Such a name matches no zone, so the linkage would silently publish
// nothing for the majority of clients. The scope's configured domain supplies
// the missing suffix.
func (dl *DNSLink) qualifyName(scopeID, hostname string) string {
	cleaned := sanitizeHostname(hostname)
	if cleaned == "" || scopeID == "" || strings.Contains(cleaned, ".") {
		return cleaned
	}

	var domain sql.NullString
	// The scope's domain name is configuration, and the lease store holds the
	// copy of it this process actually serves from.
	if err := dl.stores.Lease.QueryRow(
		`SELECT domain_name FROM dhcp_scopes WHERE id = ?`, scopeID).Scan(&domain); err != nil {
		return cleaned
	}
	suffix := strings.Trim(sanitizeHostname(domain.String), ".")
	if suffix == "" {
		return cleaned
	}
	return cleaned + "." + suffix
}

// deleteOwnedRecords removes every record this binding owns and returns how
// many rows were removed.
//
// The statement does not filter on zone or name. The lease id already
// identifies the rows exactly, and re-deriving the zone from the hostname would
// re-introduce the very failure mode this guards against: a client that reports
// a short, unqualified name would not resolve to a zone, so its records would
// never be withdrawn.
func (dl *DNSLink) deleteOwnedRecords(ownerRef string, generation int64, ip, fqdn string) (int, error) {
	if ownerRef == "" {
		return 0, nil
	}
	res, err := dl.stores.DNS.Exec(`
		DELETE FROM dns_records
		WHERE owner = 'dhcp'
		  AND (
		        (owner_ref = ? AND owner_generation <= ?)
		     OR (owner_ref IS NULL AND (
		             (type = 'A'   AND value = ?)
		          OR (type = 'PTR' AND value IN (?, ?))
		        ))
		      )`,
		ownerRef, generation,
		ip, fqdn, strings.TrimSuffix(fqdn, ".")+".")
	if err != nil {
		return 0, fmt.Errorf("dhcp_dns_link: deleting records owned by lease %s: %w", ownerRef, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Reconcile repairs drift between leases and the DHCP-driven DNS records. The
// outbox covers every transition it saw, but two situations escape it: a crash
// between the lease commit and the event insert leaves a binding with no record
// and no event, and an event abandoned after its retries leaves the same hole.
//
// The sweep is bounded (limit rows per pass) and only touches records that are
// provably orphaned, so it is safe to run on a timer:
//
//   - records whose owning lease is gone or no longer held are withdrawn;
//   - confirmed bindings in a dns_updates scope with a resolvable name and no
//     record at all are re-published.
//
// It returns how many repairs it made.
func (dl *DNSLink) Reconcile(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	repaired := 0
	changed := false

	orphans, err := dl.orphanedRecords(limit)
	if err != nil {
		return repaired, err
	}
	for _, o := range orphans {
		n, err := dl.deleteOwnedRecords(o.ownerRef, o.generation, "", "")
		if err != nil {
			return repaired, err
		}
		if n > 0 {
			repaired += n
			changed = true
			slog.Info("dhcp_dns_link: withdrew orphaned record",
				"name", o.name, "type", o.recordType, "lease_id", o.ownerRef)
		}
	}

	missing, err := dl.bindingsWithoutRecords(limit)
	if err != nil {
		return repaired, err
	}
	events := make([]DNSEvent, 0, len(missing))
	for _, m := range missing {
		events = append(events, DNSEvent{
			LeaseID:    m.LeaseID,
			Generation: m.Generation,
			Action:     DNSEventCreate,
			ScopeID:    m.ScopeID,
			IPAddress:  m.IPAddress,
			MACAddress: m.MACAddress,
			Hostname:   m.Hostname,
		})
	}
	if len(events) > 0 {
		for i := range events {
			c, err := dl.ApplyOne(events[i])
			if err != nil {
				// A single unusable binding must not abort the whole sweep; the
				// next pass retries it.
				slog.Warn("dhcp_dns_link: reconcile could not publish binding",
					"lease_id", events[i].LeaseID, "error", err)
				continue
			}
			if c {
				repaired++
				changed = true
			}
		}
	}

	// Both helpers materialize their rows before issuing further queries, so
	// the reload here runs with no cursor open.
	if changed {
		dl.reload()
	}

	return repaired, nil
}

type orphanRecord struct {
	zoneID     string
	name       string
	recordType string
	ownerRef   string
	generation int64
}

// orphanedRecords finds DHCP-driven records whose owning lease no longer holds
// the address.
//
// The row has to be *there* and no longer held. The earlier form of this query
// treated a missing lease row as a dead binding, which was right when the
// records and the leases were one database. In a split deployment the lease
// table here is a replica filled by replication, so a missing row means "not
// delivered yet" at least as often as it means "gone" -- and acting on that
// reading would withdraw the names of live clients every time replication
// lagged.
//
// What that gives up is the case of a lease row deleted outright: its records
// are no longer withdrawn by the sweep. They are still bounded, because
// expires_at stops them being served once the binding's lease end has passed,
// and a delete event is the normal way that row disappears. Bounding it by
// expiry rather than by replica lag is the trade worth making.
//
// This query joins the two views, so it runs against the DNS store, which is
// the only place both exist: it holds the DNS records and the lease replica the
// data plane pushes up. Reading the lease store instead would compare live DNS
// records against live leases in one statement, which two databases cannot do.
func (dl *DNSLink) orphanedRecords(limit int) ([]orphanRecord, error) {
	rows, err := dl.stores.DNS.Query(`
		SELECT r.zone_id, r.name, r.type, r.owner_ref, COALESCE(r.owner_generation, 0)
		FROM dns_records r
		JOIN dhcp_leases l ON l.id = r.owner_ref
		WHERE r.owner = 'dhcp' AND r.owner_ref IS NOT NULL
		  AND l.status NOT IN (?, ?)
		LIMIT ?`, string(lease.LeaseStatusActive), string(lease.LeaseStatusOffered), limit)
	if err != nil {
		return nil, fmt.Errorf("dhcp_dns_link: selecting orphaned records: %w", err)
	}
	// Materialize before any further query: a nested query while this cursor is
	// open would wait forever on the single SQLite connection.
	var out []orphanRecord
	for rows.Next() {
		var o orphanRecord
		if err := rows.Scan(&o.zoneID, &o.name, &o.recordType, &o.ownerRef,
			&o.generation); err != nil {
			rows.Close()
			return nil, fmt.Errorf("dhcp_dns_link: scanning orphaned record: %w", err)
		}
		out = append(out, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dhcp_dns_link: iterating orphaned records: %w", err)
	}
	return out, nil
}

type missingRecord struct {
	LeaseID    string
	ScopeID    string
	IPAddress  string
	MACAddress string
	Hostname   string
	Generation int64
}

// bindingsWithoutRecords finds confirmed bindings that should have a forward
// record but do not.
//
// Only bindings whose name actually lands in a zone are reported, so a lease in
// a scope that enables DNS updates but whose hostname does not match any zone
// does not produce a no-op every sweep.
//
// "Do not" includes a record whose expiry is unknown. A record published from a
// lagging lease replica has no expires_at, because the lease's end was not
// visible when it was written -- and expires_at is the second line of defence
// that stops a name outliving the binding it points at. Treating that as
// unfinished work is what makes the sweep fill it in, instead of leaving the
// name unbounded until somebody notices.
//
// Like orphanedRecords this joins the lease view to the record view, so it runs
// against the DNS store; see the note there.
func (dl *DNSLink) bindingsWithoutRecords(limit int) ([]missingRecord, error) {
	rows, err := dl.stores.DNS.Query(`
		SELECT l.id, l.scope_id, l.ip_address, l.mac_address, l.hostname, l.generation
		FROM dhcp_leases l
		JOIN dhcp_scopes s ON s.id = l.scope_id
		WHERE l.status = ?
		  AND s.dns_updates = 1
		  AND l.hostname IS NOT NULL AND l.hostname <> ''
		  AND NOT EXISTS (
		        SELECT 1 FROM dns_records r
		        WHERE r.owner_ref = l.id AND r.type = 'A' AND r.enabled = 1
		          AND r.expires_at IS NOT NULL
		      )
		LIMIT ?`, string(lease.LeaseStatusActive), limit)
	if err != nil {
		return nil, fmt.Errorf("dhcp_dns_link: selecting bindings without records: %w", err)
	}
	var candidates []missingRecord
	for rows.Next() {
		var m missingRecord
		if err := rows.Scan(&m.LeaseID, &m.ScopeID, &m.IPAddress, &m.MACAddress,
			&m.Hostname, &m.Generation); err != nil {
			rows.Close()
			return nil, fmt.Errorf("dhcp_dns_link: scanning binding: %w", err)
		}
		candidates = append(candidates, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dhcp_dns_link: iterating bindings: %w", err)
	}

	// findForwardZone issues its own query, so it must run only after the
	// cursor above is closed.
	var out []missingRecord
	for _, m := range candidates {
		// Qualify exactly the way applyCreate will, so the sweep does not
		// report a binding it would then decline to publish.
		cleaned := dl.qualifyName(m.ScopeID, m.Hostname)
		if cleaned == "" {
			continue
		}
		zoneID, _, err := dl.findForwardZone(cleaned)
		if err != nil {
			return nil, err
		}
		if zoneID == "" {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// findForwardZone finds the best matching forward DNS zone for a hostname.
func (dl *DNSLink) findForwardZone(hostname string) (string, string, error) {
	// Normalize hostname.
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))

	rows, err := dl.stores.DNS.Query(`
		SELECT id, name FROM dns_zones
		WHERE enabled = 1 AND name NOT LIKE '%in-addr.arpa%'
		ORDER BY LENGTH(name) DESC`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			slog.Warn("dns_link: failed to scan zone row", "error", err)
			continue
		}
		zoneName := strings.ToLower(strings.TrimSuffix(name, "."))
		if strings.HasSuffix(hostname, zoneName) || hostname == zoneName {
			return id, name, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("iterating zones: %w", err)
	}

	return "", "", nil
}

// findReverseZone finds the matching reverse DNS zone for an IP address.
func (dl *DNSLink) findReverseZone(ip string) (string, string, error) {
	reverseName := reverseIP(ip) + ".in-addr.arpa"

	rows, err := dl.stores.DNS.Query(`
		SELECT id, name FROM dns_zones
		WHERE enabled = 1 AND name LIKE '%in-addr.arpa%'
		ORDER BY LENGTH(name) DESC`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			slog.Warn("dns_link: failed to scan reverse zone row", "error", err)
			continue
		}
		zoneName := strings.ToLower(strings.TrimSuffix(name, "."))
		if strings.HasSuffix(reverseName, zoneName) {
			return id, name, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("iterating reverse zones: %w", err)
	}

	return "", "", nil
}

// HandleClientFQDN extracts the desired hostname from DHCP option 81 (Client FQDN).
func HandleClientFQDN(optionData []byte) string {
	if len(optionData) < 3 {
		return ""
	}

	// Option 81 format: flags(1) + RCODE(1) + domain-name in DNS wire format
	// Skip flags and RCODE, parse the domain name.
	domainName := decodeDNSWireFormat(optionData[2:])
	return domainName
}

// reverseIP converts an IP address to reverse notation (e.g., "1.2.3.4" -> "4.3.2.1").
func reverseIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	parsed = parsed.To4()
	if parsed == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", parsed[3], parsed[2], parsed[1], parsed[0])
}

// decodeDNSWireFormat decodes a DNS wire format domain name.
func decodeDNSWireFormat(data []byte) string {
	var labels []string
	i := 0
	for i < len(data) {
		length := int(data[i])
		if length == 0 {
			break
		}
		i++
		if i+length > len(data) {
			break
		}
		labels = append(labels, string(data[i:i+length]))
		i += length
	}
	return strings.Join(labels, ".")
}

// maxHostLength is the maximum total length of a DNS name per RFC 1035 §2.3.4.
const maxHostLength = 253

// sanitizeHostname normalizes a hostname and rejects inputs that are not safe
// to use in downstream SQL queries. It enforces the 253-character limit,
// strips control characters and quotes, and lowercases the result.
func sanitizeHostname(h string) string {
	h = strings.TrimSpace(h)
	if h == "" || len(h) > maxHostLength {
		return ""
	}
	// Strip trailing dot for normalization.
	h = strings.TrimSuffix(h, ".")
	h = strings.ToLower(h)
	// Filter to the printable ASCII subset allowed by RFC 1035 plus '-'.
	// This also strips quotes and backticks that could enable SQL injection.
	var b strings.Builder
	b.Grow(len(h))
	for i := 0; i < len(h); i++ {
		c := h[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c)
		case c >= '0' && c <= '9':
			b.WriteByte(c)
		case c == '-' || c == '.' || c == '_':
			b.WriteByte(c)
		}
	}
	return b.String()
}

// relativeNameInZone returns the part of hostname that is strictly outside
// the given zone, using a label-boundary check. It returns ok=false when
// hostname does not actually fall within zone.
//
// Examples:
//
//	"host1.example.com.", "example.com." -> "host1", true
//	"example.com.",      "example.com." -> "",     true
//	"evil.com.",         "example.com." -> "",     false  (suffix match but not at label boundary)
func relativeNameInZone(hostname, zone string) (string, bool) {
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	zone = strings.ToLower(strings.TrimSuffix(zone, "."))
	if hostname == "" || zone == "" {
		return "", false
	}
	if hostname == zone {
		return "", true
	}
	suffix := "." + zone
	if !strings.HasSuffix(hostname, suffix) {
		return "", false
	}
	return hostname[:len(hostname)-len(suffix)], true
}
