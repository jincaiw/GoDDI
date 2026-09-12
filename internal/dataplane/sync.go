package dataplane

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Domain is a replication group: the set of tables one data plane copies, and
// the control-plane counter that says when that copy is stale.
type Domain string

const (
	// DomainDHCP is the DHCP data plane's configuration.
	DomainDHCP Domain = "dhcp"
	// DomainDNS is the DNS data plane's zones and records.
	DomainDNS Domain = "dns"
	// DomainDDNS is the queue of DNS changes a DHCP process owes the DNS side.
	//
	// It is its own domain because it is not configuration: it is a log, it
	// grows at the rate clients arrive and leave, and its rows carry the
	// consumer's retry state. Riding along with either configuration domain
	// would make every lease renewal look like a zone edit.
	DomainDDNS Domain = "ddns"
	// DomainLeases is the lease replica, which the DNS side needs to answer
	// "does the binding behind this record still exist?".
	//
	// Separate from DomainDHCP because lease churn and scope configuration have
	// nothing to do with each other, and a DNS process has no use for scopes
	// changing at the rate clients renew.
	DomainLeases Domain = "leases"
)

// replaceMode says how a replica table is brought up to date.
type replaceMode int

const (
	// replaceWhole clears the table and loads the incoming rows. This is for
	// configuration, which the control plane authors in full: anything absent
	// from the incoming set was deleted there.
	replaceWhole replaceMode = iota

	// replaceControlAuthored replaces only the rows the control plane authored,
	// leaving the rows this plane authored exactly as they are. They are not in
	// the control plane's gift: they are this plane's answer to the leases it
	// served and the updates it accepted, and a configuration change elsewhere
	// must not withdraw them.
	replaceControlAuthored

	// appendNewer adds rows the store has not seen, keyed and ordered by key.
	//
	// This is for a log. Replacing a log wholesale would be wrong twice over:
	// it would discard the consumer's own state on rows already delivered, and
	// it would re-deliver the whole history on every pass.
	appendNewer
)

// table is one replicated table: its name, the columns carried across, and how
// the copy is applied.
//
// Parent tables come first. The copy applies them in slice order so that a
// child row never references a parent that has not landed yet, which is the
// constraint the foreign keys would otherwise have enforced.
type table struct {
	name    string
	columns []string
	mode    replaceMode
	// key is the column that identifies a row: the conflict target of an
	// upsert, the name used to keep protected rows, and the ordering column of
	// an append-only table.
	key string
}

var dhcpTables = []table{
	{name: "dhcp_scopes", key: "id", mode: replaceWhole, columns: []string{
		"id", "name", "interface", "subnet", "start_ip", "end_ip", "subnet_mask",
		"router", "dns_servers", "ntp_servers", "domain_name", "lease_time",
		"max_lease_time", "enabled", "ping_check_enabled", "dns_updates",
		"comment", "created_at", "updated_at",
	}},
	{name: "dhcp_reservations", key: "id", mode: replaceWhole, columns: []string{
		"id", "scope_id", "ip_address", "mac_address", "hostname", "description",
		"enabled", "created_at", "updated_at",
	}},
	{name: "dhcp_options", key: "id", mode: replaceWhole, columns: []string{
		"id", "scope_id", "reservation_id", "code", "value", "priority",
		"created_at", "updated_at",
	}},
}

var dnsTables = []table{
	{name: "dns_zones", key: "id", mode: replaceWhole, columns: []string{
		"id", "name", "type", "enabled", "dnssec_enabled", "default_ttl",
		"soa_mname", "soa_rname", "serial", "refresh", "retry", "expire",
		"minimum", "transfer_policy", "update_policy", "acl", "catalog",
		"nsec3_iterations", "nsec3_salt", "nsec3_optout", "last_sync_at",
		"last_sync_attempt_at", "sync_failure_count", "created_at", "updated_at",
	}},
	// Records are the one table with two authors, so they are the one table
	// that is not replaced wholesale. See replaceControlAuthored.
	{name: "dns_records", key: "id", mode: replaceControlAuthored, columns: []string{
		"id", "zone_id", "name", "type", "value", "ttl", "priority", "weight",
		"port", "enabled", "comment", "tags", "tag", "flag", "owner", "owner_ref",
		"owner_generation", "expires_at", "authored_locally", "created_at", "updated_at",
	}},
}

// ddnsTables carries the outbox. Only the payload travels: status, attempts and
// next_attempt_at are the consumer's, and overwriting them with another
// process's view of the queue is how a retry counter silently resets.
var ddnsTables = []table{
	{name: "dhcp_dns_events", key: "id", mode: appendNewer, columns: []string{
		"id", "lease_id", "generation", "action", "scope_id", "ip_address",
		"mac_address", "hostname", "created_at",
	}},
}

var leaseTables = []table{
	{name: "dhcp_leases", key: "id", mode: replaceWhole, columns: []string{
		"id", "scope_id", "ip_address", "mac_address", "hostname", "client_id",
		"lease_start", "lease_end", "status", "last_seen", "generation",
	}},
}

// TablesFor returns the replicated tables of a domain.
func TablesFor(d Domain) []table {
	switch d {
	case DomainDHCP:
		return dhcpTables
	case DomainDNS:
		return dnsTables
	case DomainDDNS:
		return ddnsTables
	case DomainLeases:
		return leaseTables
	default:
		return nil
	}
}

// Replicator copies control-plane configuration into a data-plane store.
//
// The control database is only ever read here, and it is held as an ordinary
// handle: when it is unreachable every method returns an error and the caller
// retries on the next tick, which is exactly what a data plane is supposed to
// do while its management process is down.
type Replicator struct {
	control *sql.DB
	store   *Store
}

// NewReplicator builds a replicator from a control-plane handle and a store.
// The control handle may be nil, in which case every sync reports
// ErrControlUnavailable.
func NewReplicator(control *sql.DB, store *Store) *Replicator {
	return &Replicator{control: control, store: store}
}

// ErrControlUnavailable is returned when the control database could not be
// reached. It is not a reason to stop serving: it is a reason to keep serving
// from the copy already in the store.
var ErrControlUnavailable = fmt.Errorf("control database unavailable")

// SyncResult reports what one sync pass did.
type SyncResult struct {
	// Applied is true when the store was rewritten. A sync that finds the
	// revision unchanged does no work at all: no read of the tables, no
	// transaction, no lock on the store the request path is using.
	Applied bool
	// Revision is the control revision the store now reflects.
	Revision int64
	// Rows copied across all tables, for logging and for the probe.
	Rows int
	// RetainedScopes names scopes that the control plane has removed but the
	// data plane is still serving because it holds leases in them. It is
	// reported rather than swallowed: an operator deleting a scope that is in
	// use needs to know the deletion did not take.
	RetainedScopes []string
	// DroppedLeases counts leases removed because their scope no longer
	// exists. It replaces the cascade the control database used to perform.
	DroppedLeases int
	// Duration of the pass.
	Duration time.Duration
}

// Sync copies a domain's configuration if the control-plane revision moved.
func (r *Replicator) Sync(ctx context.Context, d Domain) (SyncResult, error) {
	var res SyncResult
	if r.control == nil {
		return res, ErrControlUnavailable
	}
	started := time.Now()
	defer func() { res.Duration = time.Since(started) }()

	tables := TablesFor(d)
	if len(tables) == 0 {
		return res, fmt.Errorf("dataplane: unknown replication domain %q", d)
	}

	remote, err := r.controlRevision(d)
	if err != nil {
		return res, err
	}
	local, err := r.localRevision(d)
	if err != nil {
		return res, err
	}
	res.Revision = remote
	if remote == local {
		return res, nil
	}

	// Read everything before opening a transaction on the store. Holding the
	// store's single connection while waiting on the control database would
	// stall every DHCP request behind a management-plane read.
	rows := make(map[string][][]interface{}, len(tables))
	for _, t := range tables {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		got, err := readAll(ctx, r.control, t)
		if err != nil {
			return res, err
		}
		rows[t.name] = got
	}

	applied, retained, dropped, err := r.apply(d, tables, rows, remote)
	if err != nil {
		r.recordError(d, err)
		return res, err
	}
	res.Applied = applied
	res.RetainedScopes = retained
	res.DroppedLeases = dropped
	for _, got := range rows {
		res.Rows += len(got)
	}
	return res, nil
}

func (r *Replicator) controlRevision(d Domain) (int64, error) {
	var rev int64
	err := r.control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = ?`, string(d)).Scan(&rev)
	if err != nil {
		if isUnavailable(err) || err == sql.ErrNoRows {
			return 0, fmt.Errorf("%w: reading the %s revision: %v", ErrControlUnavailable, d, err)
		}
		return 0, fmt.Errorf("dataplane: reading the %s revision: %w", d, err)
	}
	return rev, nil
}

func (r *Replicator) localRevision(d Domain) (int64, error) {
	var rev int64
	err := r.store.QueryRow(
		`SELECT source_revision FROM dataplane_sync_state WHERE domain = ?`, string(d)).Scan(&rev)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("dataplane: reading local %s revision: %w", d, err)
	}
	return rev, nil
}

// readAll materialises a control-plane table. The cursor is fully drained and
// closed before the next statement runs: SQLite runs on a single connection per
// handle, and issuing another statement while a cursor is open waits on the
// connection that cursor holds -- with no error and no timeout.
func readAll(ctx context.Context, db *sql.DB, t table) ([][]interface{}, error) {
	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(t.columns, ", "), t.name)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: reading %s: %v", ErrControlUnavailable, t.name, err)
	}
	defer rows.Close()

	var out [][]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(t.columns))
		ptrs := make([]interface{}, len(t.columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("dataplane: scanning %s: %w", t.name, err)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: reading %s: %v", ErrControlUnavailable, t.name, err)
	}
	return out, nil
}

// apply rewrites the replica inside one transaction on the store.
func (r *Replicator) apply(d Domain, tables []table, rows map[string][][]interface{}, revision int64) (
	applied bool, retained []string, dropped int, err error,
) {
	tx, err := r.store.Begin()
	if err != nil {
		return false, nil, 0, fmt.Errorf("dataplane: begin %s sync: %w", d, err)
	}
	defer func() { _ = tx.Rollback() }()

	// A scope the control plane has removed, but which still has leases here,
	// is kept. Deleting it would leave clients holding addresses the server can
	// no longer answer for, and the cascade that used to make this safe lives
	// in the database the leases no longer reside in.
	var keepScopes = map[string]bool{}
	if d == DomainDHCP {
		keepScopes, err = r.protectedScopes(tx, rows["dhcp_scopes"])
		if err != nil {
			return false, nil, 0, err
		}
	}

	for _, t := range tables {
		if err := applyTable(tx, t, rows[t.name], keepScopes); err != nil {
			return false, nil, 0, err
		}
	}

	if d == DomainDHCP {
		dropped, err = dropOrphanLeases(tx)
		if err != nil {
			return false, nil, 0, err
		}
	}

	// Re-apply the serials this store has bumped and not yet published.
	//
	// The zone row is replaced wholesale, so a serial this store moved -- a
	// dynamic update, an inbound transfer -- comes back as the control plane's
	// older value. Left there, the store would advertise a number it has
	// already advertised, every secondary would conclude the zone had not
	// changed, and a real edit would never be transferred. The pending marker
	// holds the value to restore; this is where it is honoured.
	if d == DomainDNS {
		if err := reapplyLocalSerials(tx); err != nil {
			return false, nil, 0, err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO dataplane_sync_state (domain, source_revision, synced_at, last_error)
		VALUES (?, ?, datetime('now'), '')
		ON CONFLICT(domain) DO UPDATE SET
			source_revision = excluded.source_revision,
			synced_at       = excluded.synced_at,
			last_error      = ''`, string(d), revision); err != nil {
		return false, nil, 0, fmt.Errorf("dataplane: recording the %s revision: %w", d, err)
	}

	if err := tx.Commit(); err != nil {
		return false, nil, 0, fmt.Errorf("dataplane: commit %s sync: %w", d, err)
	}
	for id := range keepScopes {
		retained = append(retained, id)
	}
	sortStrings(retained)
	return true, retained, dropped, nil
}

// protectedScopes reports the incoming scopes that must not be removed because
// the store still holds leases in them.
func (r *Replicator) protectedScopes(tx *sql.Tx, incoming [][]interface{}) (map[string]bool, error) {
	// Column 0 of dhcp_scopes is id.
	keep := map[string]bool{}
	for _, row := range incoming {
		if len(row) == 0 {
			continue
		}
		id, _ := row[0].(string)
		if id != "" {
			keep[id] = true
		}
	}

	held, err := scopesWithHeldLeases(tx)
	if err != nil {
		return nil, err
	}
	protected := map[string]bool{}
	for _, id := range held {
		if !keep[id] {
			protected[id] = true
		}
	}
	return protected, nil
}

func scopesWithHeldLeases(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(
		`SELECT DISTINCT scope_id FROM dhcp_leases WHERE status IN ('active', 'offered', 'conflict')`)
	if err != nil {
		return nil, fmt.Errorf("dataplane: listing scopes with leases: %w", err)
	}
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}

// applyTable brings one replica table up to date according to its own rule.
func applyTable(tx *sql.Tx, t table, incoming [][]interface{}, keep map[string]bool) error {
	switch t.mode {
	case appendNewer:
		return appendNewerRows(tx, t, incoming)
	case replaceControlAuthored:
		return replaceControlAuthoredRows(tx, t, incoming)
	default:
		return replaceTable(tx, t, incoming, keep)
	}
}

// replaceTable clears a replica table and loads the incoming rows.
//
// keep names rows the caller has decided must survive even though they are
// absent from the incoming set; they are re-inserted from the row already in
// the store so a scope being served is not dropped and recreated under the
// leases pointing at it.
func replaceTable(tx *sql.Tx, t table, incoming [][]interface{}, keep map[string]bool) error {
	if err := deleteTable(tx, t, keep); err != nil {
		return err
	}
	return insertRowsInto(tx, t, incoming)
}

// replaceControlAuthoredRows brings the control plane's rows of a two-author
// table up to date and leaves this plane's own exactly as they are.
//
// Both filters are load-bearing, and they are the whole reason this mode
// exists:
//
//   - The delete must not touch a locally authored row. Without that, any
//     configuration change anywhere -- one operator renaming one zone -- would
//     withdraw every name this process is currently answering for.
//   - The insert must not carry a locally authored row back in. The control
//     database's copy of one is a reporting artifact, and it lags: a record
//     this process has already withdrawn on a teardown can still be sitting
//     there, and re-inserting it would publish a name no binding justifies.
//
// The invariant this leaves is worth stating plainly:
//
//	the table holds the control plane's rows, freshly copied, plus this
//	plane's own rows, untouched.
func replaceControlAuthoredRows(tx *sql.Tx, t table, incoming [][]interface{}) error {
	if _, err := tx.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE authored_locally = 0", t.name)); err != nil {
		return fmt.Errorf("dataplane: clearing the control-authored rows of %s: %w", t.name, err)
	}

	at, err := columnIndex(t, "authored_locally")
	if err != nil {
		return err
	}
	keep := make([][]interface{}, 0, len(incoming))
	for _, row := range incoming {
		if isLocallyAuthored(row[at]) {
			continue
		}
		keep = append(keep, row)
	}
	return insertRowsInto(tx, t, keep)
}

// appendNewerRows adds the log entries the store has not seen.
//
// The high-water mark is read from the table itself rather than kept in
// dataplane_meta, so it cannot fall out of step with the rows it describes: a
// store restored from a backup brings back both or neither.
//
// Note what is deliberately absent: there is no delete. An entry the control
// database no longer has is one this store has already applied, and "the row is
// gone" must not be replayed as "undo the change".
func appendNewerRows(tx *sql.Tx, t table, incoming [][]interface{}) error {
	if len(incoming) == 0 {
		return nil
	}
	at, err := columnIndex(t, t.key)
	if err != nil {
		return err
	}
	var highest int64
	if err := tx.QueryRow(fmt.Sprintf("SELECT COALESCE(MAX(%s), 0) FROM %s", t.key, t.name)).
		Scan(&highest); err != nil {
		return fmt.Errorf("dataplane: reading the high-water mark of %s: %w", t.name, err)
	}

	fresh := make([][]interface{}, 0, len(incoming))
	for _, row := range incoming {
		key, ok := asInt64(row[at])
		if !ok || key <= highest {
			continue
		}
		fresh = append(fresh, row)
	}
	return insertRowsInto(tx, t, fresh)
}

// insertRowsInto loads rows that have already been filtered.
//
// Plain INSERT, not INSERT OR IGNORE: by the time a row reaches here the caller
// has decided it belongs, so a key collision is a fault in that decision and
// should surface as an error rather than be swallowed.
func insertRowsInto(tx *sql.Tx, t table, rows [][]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	cols := strings.Join(t.columns, ", ")
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(t.columns)), ", ")
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", t.name, cols, placeholders)

	prepared, err := tx.Prepare(stmt)
	if err != nil {
		return fmt.Errorf("dataplane: preparing insert into %s: %w", t.name, err)
	}
	defer prepared.Close()

	for _, row := range rows {
		if _, err := prepared.Exec(row...); err != nil {
			return fmt.Errorf("dataplane: inserting into %s: %w", t.name, err)
		}
	}
	return nil
}

func columnIndex(t table, column string) (int, error) {
	for i, c := range t.columns {
		if c == column {
			return i, nil
		}
	}
	return 0, fmt.Errorf("dataplane: %s is replicated without its %q column", t.name, column)
}

// isLocallyAuthored reports whether a scanned authored_locally value says the
// row belongs to this plane. The column is INTEGER, but a driver is free to
// hand back a string or a byte slice for it, and reading the wrong one of those
// as "not authored here" would delete a row that must survive.
func isLocallyAuthored(v interface{}) bool {
	n, ok := asInt64(v)
	return ok && n != 0
}

func asInt64(v interface{}) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case []byte:
		var n int64
		if _, err := fmt.Sscan(string(x), &n); err == nil {
			return n, true
		}
	case string:
		var n int64
		if _, err := fmt.Sscan(x, &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

func deleteTable(tx *sql.Tx, t table, keep map[string]bool) error {
	if len(keep) == 0 {
		if _, err := tx.Exec("DELETE FROM " + t.name); err != nil {
			return fmt.Errorf("dataplane: clearing %s: %w", t.name, err)
		}
		return nil
	}

	// Keep the protected rows in place rather than deleting and re-inserting
	// them: a delete followed by an insert is two chances to lose the row.
	args := make([]interface{}, 0, len(keep))
	holders := make([]string, 0, len(keep))
	for id := range keep {
		args = append(args, id)
		holders = append(holders, "?")
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE %s NOT IN (%s)",
		t.name, t.key, strings.Join(holders, ", "))
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("dataplane: clearing %s: %w", t.name, err)
	}
	return nil
}

// dropOrphanLeases removes leases whose scope no longer exists.
//
// The control database used to do this with ON DELETE CASCADE. Leases live here
// now, so the equivalent has to be written out, and writing it out has the
// advantage that it can be counted and reported instead of happening silently.
func dropOrphanLeases(tx *sql.Tx) (int, error) {
	res, err := tx.Exec(`
		DELETE FROM dhcp_leases
		WHERE scope_id <> '' AND scope_id NOT IN (SELECT id FROM dhcp_scopes)`)
	if err != nil {
		return 0, fmt.Errorf("dataplane: dropping leases of removed scopes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// reapplyLocalSerials restores the zone serials this store has bumped and not
// yet pushed, after the sync has replaced the zone rows.
//
// The guard mirrors the one on the far side: this only ever raises a serial, so
// a replace that happens to be ahead is left alone. Both ends of the serial
// therefore move in one direction only, which is the property a secondary's
// serial comparison depends on.
func reapplyLocalSerials(tx *sql.Tx) error {
	_, err := tx.Exec(`
		UPDATE dns_zones
		SET serial = (
			SELECT s.serial FROM dns_zone_serial_dirty s WHERE s.zone_id = dns_zones.id
		)
		WHERE id IN (SELECT zone_id FROM dns_zone_serial_dirty)
		  AND serial < (
			SELECT s.serial FROM dns_zone_serial_dirty s WHERE s.zone_id = dns_zones.id
		)`)
	if err != nil {
		return fmt.Errorf("dataplane: restoring locally bumped zone serials: %w", err)
	}
	return nil
}

func (r *Replicator) recordError(d Domain, cause error) {
	if _, err := r.store.Exec(`
		INSERT INTO dataplane_sync_state (domain, source_revision, synced_at, last_error)
		VALUES (?, 0, NULL, ?)
		ON CONFLICT(domain) DO UPDATE SET last_error = excluded.last_error`,
		string(d), cause.Error()); err != nil {
		slog.Warn("dataplane: recording a sync failure failed", "domain", d, "error", err)
	}
}

// SyncState is the probe view of one domain's replication.
type SyncState struct {
	Domain         Domain
	SourceRevision int64
	SyncedAt       string
	LastError      string
	// Behind is true when the store records a failure or has never synced.
	Behind bool
}

// State reports what the store knows about a domain's replication.
func (s *Store) State(d Domain) (SyncState, error) {
	out := SyncState{Domain: d}
	var syncedAt sql.NullString
	err := s.QueryRow(
		`SELECT source_revision, synced_at, last_error FROM dataplane_sync_state WHERE domain = ?`,
		string(d)).Scan(&out.SourceRevision, &syncedAt, &out.LastError)
	if err == sql.ErrNoRows {
		out.Behind = true
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("dataplane: reading %s sync state: %w", d, err)
	}
	out.SyncedAt = syncedAt.String
	out.Behind = out.LastError != "" || !syncedAt.Valid
	return out, nil
}

// HasReplica reports whether the store holds any configuration for a domain.
//
// This is what separates "the control plane is down and I am serving yesterday's
// configuration" from "the control plane is down and I have nothing". The first
// starts; the second must refuse to, because a DHCP server that starts with no
// scopes looks healthy while answering nothing.
func (s *Store) HasReplica(d Domain) (bool, error) {
	switch d {
	case DomainDHCP:
		return s.hasRows("dhcp_scopes")
	case DomainDNS:
		return s.hasRows("dns_zones")
	case DomainDDNS:
		return s.hasRows("dhcp_dns_events")
	case DomainLeases:
		return s.hasRows("dhcp_leases")
	default:
		return false, fmt.Errorf("dataplane: unknown domain %q", d)
	}
}

// HasAnyReplica reports whether the store holds anything for any of the given
// domains.
//
// This is the question a process actually has to answer at startup: not "was
// this particular domain ever synced" but "do I have anything at all to serve".
// A DNS process whose zones are present must start while the control database
// is down even if it has no DHCP scopes, and asking per domain would refuse it
// for the empty one.
func (s *Store) HasAnyReplica(domains []Domain) (bool, error) {
	for _, d := range domains {
		has, err := s.HasReplica(d)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) hasRows(table string) (bool, error) {
	var n int
	if err := s.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		return false, fmt.Errorf("dataplane: counting %s: %w", table, err)
	}
	return n > 0, nil
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// isUnavailable reports whether an error means "the other database is not
// reachable" as opposed to "the query was wrong".
func isUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"unable to open database file",
		"no such table",
		"database is locked",
		"disk i/o error",
		"connection refused",
		"bad connection",
		"closed",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
