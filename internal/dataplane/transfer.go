package dataplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// metaMigratedLeases records that the control database's lease rows have been
// taken over by this store. Its presence is what stops the takeover from
// running a second time and resurrecting leases an operator has since removed.
const metaMigratedLeases = "leases_taken_over_at"

// pushBatchDefault caps how much one upward pass moves, so a large backlog
// cannot hold the store's single connection for an unbounded time.
const pushBatchDefault = 500

// leaseTransferColumns are the columns carried when a lease moves between the
// two databases. They are shared by the takeover and by the upward push so the
// two directions cannot disagree about what a lease is.
var leaseTransferColumns = []string{
	"id", "scope_id", "ip_address", "mac_address", "hostname", "client_id",
	"lease_start", "lease_end", "status", "last_seen", "generation",
}

// TakeOverLeases copies the control database's lease state into the store, once.
//
// Upgrading an existing installation must not lose leases: a scope whose
// addresses are all held would come back empty and hand the same addresses out
// again. This runs on the first start of the split build, moves the rows, and
// records that it happened.
//
// It refuses to run when the store already holds leases. That combination means
// the data plane was already authoritative and the control database's rows are
// the stale copy; copying them in would undo every lease change since the
// split. The marker is still written, so the question is not asked again.
func (r *Replicator) TakeOverLeases(ctx context.Context) (int, error) {
	if r.control == nil {
		return 0, ErrControlUnavailable
	}
	done, err := r.metaHas(metaMigratedLeases)
	if err != nil {
		return 0, err
	}
	if done {
		return 0, nil
	}

	local, err := r.store.hasRows("dhcp_leases")
	if err != nil {
		return 0, err
	}
	if local {
		slog.Info("dataplane: lease store already holds leases; control rows are the stale copy and are left alone")
		return 0, r.metaSet(metaMigratedLeases, time.Now().UTC().Format(time.RFC3339))
	}

	total := 0
	for _, spec := range []struct {
		table   string
		columns []string
	}{
		{"dhcp_leases", leaseTransferColumns},
		{"dhcp_dns_events", []string{
			"id", "lease_id", "generation", "action", "scope_id", "ip_address",
			"mac_address", "hostname", "attempts", "next_attempt_at", "last_error",
			"status", "created_at", "updated_at",
		}},
		{"dhcp_logs", []string{"id", "scope_id", "mac_address", "ip_address", "event_type", "message", "created_at"}},
	} {
		rows, err := readAll(ctx, r.control, table{name: spec.table, columns: spec.columns})
		if err != nil {
			return total, err
		}
		written, err := r.insertRows(spec.table, spec.columns, rows)
		if err != nil {
			return total, err
		}
		total += written
	}

	if err := r.metaSet(metaMigratedLeases, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return total, err
	}
	slog.Info("dataplane: took over lease state from the control database", "rows", total)
	return total, nil
}

func (r *Replicator) insertRows(tableName string, columns []string, rows [][]interface{}) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := r.store.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	stmt := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)",
		tableName, strings.Join(columns, ", "),
		strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", "))
	prepared, err := tx.Prepare(stmt)
	if err != nil {
		return 0, fmt.Errorf("dataplane: preparing takeover insert into %s: %w", tableName, err)
	}
	defer prepared.Close()

	n := 0
	for _, row := range rows {
		res, err := prepared.Exec(row...)
		if err != nil {
			return n, fmt.Errorf("dataplane: taking over %s: %w", tableName, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return n, fmt.Errorf("dataplane: counting the takeover into %s: %w", tableName, err)
		}
		// INSERT OR IGNORE reports 0 for a row that was already present, so
		// what this returns is how many rows actually moved rather than how many
		// were offered. The number ends up in a log line an operator reads when
		// deciding whether an upgrade lost anything.
		n += int(affected)
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

func (r *Replicator) metaHas(key string) (bool, error) {
	var v string
	err := r.store.QueryRow(`SELECT value FROM dataplane_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("dataplane: reading meta %s: %w", key, err)
	}
	return true, nil
}

func (r *Replicator) metaSet(key, value string) error {
	_, err := r.store.Exec(`
		INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Meta returns a stored fact about the store, or "" when it is unset.
func (s *Store) Meta(key string) (string, error) {
	var v string
	err := s.QueryRow(`SELECT value FROM dataplane_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// PushResult reports one upward replication pass.
type PushResult struct {
	Upserted int
	Deleted  int
	// Refused is how many rows in this pass the control database would not
	// take. They are still owed and are paced by the retry state migration 005
	// adds; they are counted here so the pass can say so in one line rather
	// than one line per row per tick.
	Refused int
	// Pending is how many changes are still owed after the pass.
	Pending int
}

// Push copies lease changes up to the control-plane replica.
//
// The control database's copy of a lease exists for the console, the scope and
// IPAM dependency checks, and backups. None of those are on the DHCP request
// path, so this runs on the poll tick and a failure is recorded and retried
// rather than being reported to a client: a client must never be refused its
// address because a reporting copy could not be written.
//
// A refusal is recorded per row. The transaction carries every row the control
// database accepted, and the rows it did not are left queued with a widening
// retry delay -- see recordPushFailures and migration 005. Rows are independent
// obligations, so one of them being unacceptable must not decide the fate of
// the others, which is what returning on the first error used to do.
//
// With an empty queue there is nothing to report and nothing to retry, so Push
// returns without touching the control database. Reachability is Sync's to
// report, not a no-op's.
func (r *Replicator) Push(ctx context.Context, limit int) (PushResult, error) {
	var res PushResult
	if r.control == nil {
		return res, ErrControlUnavailable
	}
	if limit <= 0 {
		limit = 500
	}

	pending, err := r.pendingLeases(ctx, limit)
	if err != nil {
		return res, err
	}
	if len(pending) == 0 {
		return res, nil
	}

	tx, err := r.control.Begin()
	if err != nil {
		return res, fmt.Errorf("%w: beginning the lease push: %v", ErrControlUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	deleteStmt, err := tx.Prepare(`DELETE FROM dhcp_leases WHERE id = ?`)
	if err != nil {
		return res, fmt.Errorf("dataplane: preparing the replica delete: %w", err)
	}
	defer deleteStmt.Close()

	upsertStmt, err := tx.Prepare(fmt.Sprintf(`
		INSERT INTO dhcp_leases (%s) VALUES (%s)
		ON CONFLICT(id) DO UPDATE SET %s`,
		strings.Join(leaseTransferColumns, ", "),
		strings.TrimSuffix(strings.Repeat("?, ", len(leaseTransferColumns)), ", "),
		upsertAssignments(leaseTransferColumns, "id")))
	if err != nil {
		return res, fmt.Errorf("dataplane: preparing the replica upsert: %w", err)
	}
	defer upsertStmt.Close()

	// A row the control database will not take is recorded and skipped, not
	// returned on. Returning here is what made one unacceptable lease stall the
	// entire queue: every marker in the batch stayed, so the next pass read the
	// same batch and stopped at the same row, once a second, forever.
	var done []string
	var refused []pushFailure
	for _, p := range pending {
		if p.gone {
			if _, err := deleteStmt.Exec(p.id); err != nil {
				refused = append(refused, pushFailure{key: p.id, err: err})
				continue
			}
			res.Deleted++
		} else {
			if _, err := upsertStmt.Exec(p.values...); err != nil {
				refused = append(refused, pushFailure{key: p.id, err: err})
				continue
			}
			res.Upserted++
		}
		done = append(done, p.id)
	}
	res.Refused = len(refused)

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("dataplane: committing the lease push: %w", err)
	}

	// The markers are cleared only after the control database has accepted the
	// rows. Clearing them first would drop a change that never arrived.
	if err := r.clearDirty(done); err != nil {
		return res, err
	}
	// Bookkept after the clear, so that a row which is still owed is never the
	// one that loses its record of being refused.
	if err := r.recordPushFailures("dhcp_lease_dirty", "lease_id", refused); err != nil {
		return res, err
	}
	res.Pending, err = r.PendingLeases()
	return res, err
}

func upsertAssignments(columns []string, key string) string {
	var sets []string
	for _, c := range columns {
		if c == key {
			continue
		}
		sets = append(sets, c+" = excluded."+c)
	}
	return strings.Join(sets, ", ")
}

type pendingLease struct {
	id     string
	gone   bool
	values []interface{}
}

// pendingRow is one queued upward change. key is the marker's key -- the id of
// the row it describes.
type pendingRow struct {
	key    string
	gone   bool
	values []interface{}
}

// pendingQuery describes one upward queue.
//
// The four queues this store owes are shaped the same way and are read by one
// implementation on purpose: a second reader written by hand is a second place
// for the "materialise before the next query" rule to be forgotten, and that
// mistake does not raise an error, it waits forever.
type pendingQuery struct {
	markerTable string
	// markerKey is the column in markerTable naming the row it describes.
	markerKey   string
	sourceTable string
	// sourceKey is the column in sourceTable that markerKey names.
	sourceKey string
	columns   []string
	// hasDeleted is true for a marker table that distinguishes "the row
	// changed" from "the row is gone". Without it a missing row is the only
	// sign of a deletion, which is enough for a table whose rows are never
	// removed for any other reason.
	hasDeleted bool
	limit      int
}

// pendingRows materialises a queue. The cursor is fully drained and closed
// before returning: SQLite runs on one connection per handle and issuing
// another statement while a cursor is open waits on the connection that cursor
// holds, with no error and no timeout.
//
// Rows that are still waiting out a refusal are left out. That filter has to be
// here rather than in the caller: the read is ordered by queued_at and capped,
// so the oldest rows -- exactly the ones that have been refused -- would fill
// every batch and the rows behind them would never be read.
//
// `next_attempt_at IS NULL` is spelled out. NULL does not compare: for a row
// that has never been refused, `julianday(NULL) <= julianday('now')` is NULL
// rather than true, and a filter that relied on it would skip every row in the
// queue while reporting an empty backlog. julianday() rather than a text
// comparison because it holds for either representation of the timestamp.
func (r *Replicator) pendingRows(ctx context.Context, q pendingQuery) ([]pendingRow, error) {
	selectCols := make([]string, 0, len(q.columns))
	for _, c := range q.columns {
		selectCols = append(selectCols, "s."+c)
	}
	deleted := ""
	if q.hasDeleted {
		deleted = ", d.deleted"
	}
	query := fmt.Sprintf(`
		SELECT d.%s%s, %s
		FROM %s d
		LEFT JOIN %s s ON s.%s = d.%s
		WHERE d.next_attempt_at IS NULL OR julianday(d.next_attempt_at) <= julianday('now')
		ORDER BY d.queued_at ASC, d.%s ASC
		LIMIT ?`,
		q.markerKey, deleted, strings.Join(selectCols, ", "),
		q.markerTable, q.sourceTable, q.sourceKey, q.markerKey, q.markerKey)

	rows, err := r.store.QueryContext(ctx, query, q.limit)
	if err != nil {
		return nil, fmt.Errorf("dataplane: reading pending %s: %w", q.markerTable, err)
	}
	defer rows.Close()

	var out []pendingRow
	for rows.Next() {
		var key string
		var wasDeleted int
		vals := make([]interface{}, len(q.columns))
		ptrs := make([]interface{}, 0, len(q.columns)+2)
		ptrs = append(ptrs, &key)
		if q.hasDeleted {
			ptrs = append(ptrs, &wasDeleted)
		}
		for i := range vals {
			ptrs = append(ptrs, &vals[i])
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("dataplane: scanning pending %s: %w", q.markerTable, err)
		}

		// A marker for a row that is not there means the row was deleted after
		// the marker was written, or written by a delete trigger and then
		// removed entirely. Either way the replica must not keep it.
		gone := (q.hasDeleted && wasDeleted == 1) || vals[0] == nil
		out = append(out, pendingRow{key: key, gone: gone, values: vals})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dataplane: reading pending %s: %w", q.markerTable, err)
	}
	return out, nil
}

// pendingLeases materialises the changed leases, cursor first and closed.
func (r *Replicator) pendingLeases(ctx context.Context, limit int) ([]pendingLease, error) {
	got, err := r.pendingRows(ctx, pendingQuery{
		markerTable: "dhcp_lease_dirty",
		markerKey:   "lease_id",
		sourceTable: "dhcp_leases",
		sourceKey:   "id",
		columns:     leaseTransferColumns,
		hasDeleted:  true,
		limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]pendingLease, 0, len(got))
	for _, p := range got {
		out = append(out, pendingLease{id: p.key, gone: p.gone, values: p.values})
	}
	return out, nil
}

// upwardQueues names every marker table this store owes upward, with the
// column that names the row each marker describes.
//
// One list, because the count of rows waiting out a refusal is a sum over it
// and a list that misses a queue would report a healthy system while that queue
// filled. TestEveryMarkerTableIsInTheUpwardQueueList reads the schema and
// fails if a seventh queue is ever added without landing here.
var upwardQueues = []struct{ table, key string }{
	{"dhcp_lease_dirty", "lease_id"},
	{"dhcp_dns_event_dirty", "event_id"},
	{"dhcp_log_dirty", "log_id"},
	{"dns_record_dirty", "record_id"},
	{"dns_zone_serial_dirty", "zone_id"},
	{"audit_log_dirty", "log_id"},
}

// pushBackoff is how long a row waits after its nth consecutive refusal.
//
// The widening interval is what keeps a row the control database will not
// accept from turning the one-second poll into a one-second retry of the same
// doomed statement. The ceiling is deliberate and is the whole reason there is
// no terminal state: see migration 005. A row whose cause is repaired waits at
// most this long before it travels.
//
// The steps are a literal ladder rather than an exponent so the schedule can be
// read off the source and asserted in a test.
func pushBackoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Second
	case attempts == 2:
		return 5 * time.Second
	case attempts == 3:
		return 15 * time.Second
	case attempts == 4:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}

// pushFailure is one row the control database refused, and what it said.
type pushFailure struct {
	key string
	err error
}

// recordPushFailures writes down what the control database said about the rows
// this pass could not deliver, and pushes each of them out of the way for a
// while.
//
// This is the other half of per-row isolation: without it a refused row would
// be retried on every tick forever, which is the same statement at one hertz.
// The marker itself is deliberately kept -- the change is still owed, and the
// count of what is owed is a number an operator reads and must not be flattered
// by writing off work that has not been done.
//
// A failure whose marker has vanished in the meantime is dropped rather than
// recorded: the marker was cleared, so the row was dealt with and there is
// nothing left to pace.
func (r *Replicator) recordPushFailures(markerTable, markerKey string, failures []pushFailure) error {
	if len(failures) == 0 {
		return nil
	}
	tx, err := r.store.Begin()
	if err != nil {
		return fmt.Errorf("dataplane: beginning the refusal bookkeeping: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	read, err := tx.Prepare(fmt.Sprintf(
		`SELECT attempts FROM %s WHERE %s = ?`, markerTable, markerKey))
	if err != nil {
		return fmt.Errorf("dataplane: preparing the refusal read: %w", err)
	}
	defer read.Close()

	write, err := tx.Prepare(fmt.Sprintf(`
		UPDATE %s
		   SET attempts = ?, last_error = ?, next_attempt_at = datetime('now', ?)
		 WHERE %s = ?`, markerTable, markerKey))
	if err != nil {
		return fmt.Errorf("dataplane: preparing the refusal write: %w", err)
	}
	defer write.Close()

	for _, f := range failures {
		var attempts int
		switch err := read.QueryRow(f.key).Scan(&attempts); {
		case errors.Is(err, sql.ErrNoRows):
			continue
		case err != nil:
			return fmt.Errorf("dataplane: reading the refusal state of %s: %w", f.key, err)
		}
		attempts++
		backoff := pushBackoff(attempts)
		// A first refusal is a transition and is reported once at Warn; the
		// rest are the same standing condition repeating, and the queue depth
		// and the refusal count are what carry it from then on. Logging every
		// retry at Warn would bury the transition in its own derivatives.
		level := slog.LevelDebug
		if attempts == 1 {
			level = slog.LevelWarn
		}
		slog.Log(context.Background(), level,
			"dataplane: the control database refused a change; it stays queued",
			"queue", markerTable, "key", f.key, "attempts", attempts,
			"retry_in", backoff.String(), "error", f.err)
		if _, err := write.Exec(attempts, f.err.Error(),
			fmt.Sprintf("+%d seconds", int(backoff.Seconds())), f.key); err != nil {
			return fmt.Errorf("dataplane: recording the refusal of %s: %w", f.key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dataplane: committing the refusal bookkeeping: %w", err)
	}
	return nil
}

// Refused counts the rows across every upward queue that are waiting out a
// refusal from the control database.
//
// It is a number apart from the queue depths on purpose. A backlog says work is
// owed; this says work is owed and being declined, which is the difference
// between a control plane that is behind and one that will not take the rows.
func (r *Replicator) Refused() (int, error) {
	parts := make([]string, 0, len(upwardQueues))
	for _, q := range upwardQueues {
		parts = append(parts, fmt.Sprintf(
			`(SELECT COUNT(*) FROM %s
			   WHERE next_attempt_at IS NOT NULL AND julianday(next_attempt_at) > julianday('now'))`,
			q.table))
	}
	var n int
	if err := r.store.QueryRow("SELECT " + strings.Join(parts, " + ")).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// clearMarkers removes the queued markers a completed pass has dealt with.
func (r *Replicator) clearMarkers(markerTable, markerKey string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := r.store.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", markerTable, markerKey))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, key := range keys {
		if _, err := stmt.Exec(key); err != nil {
			return fmt.Errorf("dataplane: clearing a %s marker: %w", markerTable, err)
		}
	}
	return tx.Commit()
}

// insertOnlyStatement builds a statement whose conflict clause does nothing.
//
// It exists beside upsertStatement because the two say different things about
// the same situation: an upsert means "this row's content is the truth and
// should replace what is there", while DO NOTHING means "this row's content is
// a statement about the past, and a second copy of it carries no new
// information". Using an upsert for the second case would let a retry rewrite a
// record of something that already happened.
func insertOnlyStatement(tableName string, columns []string, key string) string {
	return fmt.Sprintf(`
		INSERT INTO %s (%s) VALUES (%s)
		ON CONFLICT(%s) DO NOTHING`,
		tableName,
		strings.Join(columns, ", "),
		strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", "),
		key)
}

// upsertStatement builds the statement the upward queues share. Written once so
// the column list, the placeholders and the update assignments cannot drift
// apart.
func upsertStatement(tableName string, columns []string, key string) string {
	return fmt.Sprintf(`
		INSERT INTO %s (%s) VALUES (%s)
		ON CONFLICT(%s) DO UPDATE SET %s`,
		tableName,
		strings.Join(columns, ", "),
		strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", "),
		key, upsertAssignments(columns, key))
}

// eventPushColumns are the columns an outbox row travels with: the payload the
// producer decided, and nothing else.
//
// The consumer's own columns -- status, attempts, next_attempt_at, last_error
// -- are deliberately absent, and they are absent from the push rather than
// from the table because two planes now hold a copy of this queue. The DHCP
// plane authors the payload; each DNS plane keeps its own progress through it.
// Copying an attempt counter made in one file system over another's is how a
// retry silently resets or an event is silently re-delivered.
//
// The consequence is worth stating plainly rather than hiding: the control
// database's copy of this queue records what was owed, not what was done. It is
// the payload's rendezvous, and completion is per-consumer state held in the
// store that did the work.
var eventPushColumns = []string{
	"id", "lease_id", "generation", "action", "scope_id", "ip_address",
	"mac_address", "hostname", "created_at",
}

var logPushColumns = []string{
	"id", "scope_id", "mac_address", "ip_address", "event_type", "message", "created_at",
}

// recordPushColumns are the columns of a record this plane authored.
var recordPushColumns = []string{
	"id", "zone_id", "name", "type", "value", "ttl", "priority", "weight",
	"port", "enabled", "comment", "tags", "tag", "flag", "owner", "owner_ref",
	"owner_generation", "expires_at", "authored_locally", "created_at", "updated_at",
}

// PushEvents copies queued outbox entries up to the control database.
//
// This is what makes the split deployment have DDNS at all: the DNS process in
// another file system cannot see this store, so an entry that is not copied up
// is a name that will never be published. Like the lease push it is off the
// request path, and an empty queue is a no-op that touches nothing.
//
// A row that is already there is left exactly as it is. This is the producer's
// rule and it is the whole reason the conflict clause is a no-op rather than an
// update: the producer cannot observe the consumer, so anything it wrote would
// be a guess about state it does not own. In particular it must not report an
// event back to pending because its own local copy has never been consumed.
func (r *Replicator) PushEvents(ctx context.Context, limit int) (int, error) {
	if r.control == nil {
		return 0, ErrControlUnavailable
	}
	if limit <= 0 {
		limit = pushBatchDefault
	}

	pending, err := r.pendingRows(ctx, pendingQuery{
		markerTable: "dhcp_dns_event_dirty",
		markerKey:   "event_id",
		sourceTable: "dhcp_dns_events",
		sourceKey:   "id",
		columns:     eventPushColumns,
		limit:       limit,
	})
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}

	tx, err := r.control.Begin()
	if err != nil {
		return 0, fmt.Errorf("%w: beginning the event push: %v", ErrControlUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(fmt.Sprintf(`
		INSERT INTO dhcp_dns_events (%s) VALUES (%s)
		ON CONFLICT(id) DO NOTHING`,
		strings.Join(eventPushColumns, ", "),
		strings.TrimSuffix(strings.Repeat("?, ", len(eventPushColumns)), ", ")))
	if err != nil {
		return 0, fmt.Errorf("dataplane: preparing the event insert: %w", err)
	}
	defer stmt.Close()

	done := make([]string, 0, len(pending))
	var refused []pushFailure
	n := 0
	for _, p := range pending {
		// An entry is never deleted -- the log is append-only -- so a marker
		// whose row is missing is a fault, not a deletion. Skip it rather than
		// write a null row, and let the marker be cleared so it cannot block
		// the queue.
		if p.gone {
			done = append(done, p.key)
			continue
		}
		res, err := stmt.Exec(p.values...)
		if err != nil {
			refused = append(refused, pushFailure{key: p.key, err: err})
			continue
		}
		done = append(done, p.key)
		// Counted from the rows the control database actually took: a re-push
		// of an entry that is already there moves nothing, and reporting it as
		// moved would make the log line a fiction.
		if affected, err := res.RowsAffected(); err == nil {
			n += int(affected)
		}
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("dataplane: committing the event push: %w", err)
	}
	if err := r.clearMarkers("dhcp_dns_event_dirty", "event_id", done); err != nil {
		return n, err
	}
	if err := r.recordPushFailures("dhcp_dns_event_dirty", "event_id", refused); err != nil {
		return n, err
	}
	return n, nil
}

// zoneSerialColumns are the columns a serial push needs.
var zoneSerialColumns = []string{"zone_id", "serial"}

// PushZoneSerials raises the control database's copy of a zone's SOA serial to
// the value this store has already advertised.
//
// The guard is the statement's whole point: `serial < ?` makes the push
// monotonic on the far side, so a delayed push -- a control plane that was down
// while this store bumped the serial several times -- cannot move the control
// database's serial backwards past a value it has already published.
//
// Nothing else about the zone row travels. The control plane remains the author
// of the zone; this is one number, and it is reported, not asserted.
func (r *Replicator) PushZoneSerials(ctx context.Context, limit int) (int, error) {
	if r.control == nil {
		return 0, ErrControlUnavailable
	}
	if limit <= 0 {
		limit = pushBatchDefault
	}

	pending, err := r.pendingRows(ctx, pendingQuery{
		markerTable: "dns_zone_serial_dirty",
		markerKey:   "zone_id",
		// The value to publish lives in the marker, not in dns_zones: the
		// downward sync replaces the zone row and would have taken a locally
		// bumped serial with it. Reading the marker as its own source is what
		// keeps the push independent of what the sync last wrote.
		sourceTable: "dns_zone_serial_dirty",
		sourceKey:   "zone_id",
		columns:     zoneSerialColumns,
		limit:       limit,
	})
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}

	tx, err := r.control.Begin()
	if err != nil {
		return 0, fmt.Errorf("%w: beginning the serial push: %v", ErrControlUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(
		`UPDATE dns_zones SET serial = ?, updated_at = datetime('now')
		 WHERE id = ? AND serial < ?`)
	if err != nil {
		return 0, fmt.Errorf("dataplane: preparing the serial update: %w", err)
	}
	defer stmt.Close()

	done := make([]string, 0, len(pending))
	var refused []pushFailure
	n := 0
	for _, p := range pending {
		if p.gone {
			done = append(done, p.key)
			continue
		}
		serial, ok := asInt64(p.values[1])
		if !ok {
			// A marker whose serial cannot be read is a fault rather than work,
			// and it is recorded as one. It used to be cleared -- the marker is
			// unreadable, so clearing it "cannot lose anything". That is true
			// and beside the point: the row says this store's serial and the
			// control database's have diverged, and a queue whose backlog
			// silently drops that is a queue that cannot be trusted about the
			// rows it does report.
			refused = append(refused, pushFailure{key: p.key,
				err: fmt.Errorf("the queued serial is not a number")})
			continue
		}
		res, err := stmt.Exec(serial, p.key, serial)
		if err != nil {
			refused = append(refused, pushFailure{key: p.key, err: err})
			continue
		}
		done = append(done, p.key)
		if affected, err := res.RowsAffected(); err == nil {
			n += int(affected)
		}
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("dataplane: committing the serial push: %w", err)
	}
	if err := r.clearMarkers("dns_zone_serial_dirty", "zone_id", done); err != nil {
		return n, err
	}
	if err := r.recordPushFailures("dns_zone_serial_dirty", "zone_id", refused); err != nil {
		return n, err
	}
	return n, nil
}

// PendingZoneSerials counts the serial bumps still owed to the control
// database.
func (r *Replicator) PendingZoneSerials() (int, error) {
	var n int
	if err := r.store.QueryRow(`SELECT COUNT(*) FROM dns_zone_serial_dirty`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// PushLogs copies the DHCP event log up so the console's view of DHCP activity
// does not stop at the moment the split was introduced.
func (r *Replicator) PushLogs(ctx context.Context, limit int) (int, error) {
	if r.control == nil {
		return 0, ErrControlUnavailable
	}
	if limit <= 0 {
		limit = pushBatchDefault
	}

	pending, err := r.pendingRows(ctx, pendingQuery{
		markerTable: "dhcp_log_dirty",
		markerKey:   "log_id",
		sourceTable: "dhcp_logs",
		sourceKey:   "id",
		columns:     logPushColumns,
		limit:       limit,
	})
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}

	tx, err := r.control.Begin()
	if err != nil {
		return 0, fmt.Errorf("%w: beginning the log push: %v", ErrControlUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(upsertStatement("dhcp_logs", logPushColumns, "id"))
	if err != nil {
		return 0, fmt.Errorf("dataplane: preparing the log upsert: %w", err)
	}
	defer stmt.Close()

	done := make([]string, 0, len(pending))
	var refused []pushFailure
	n := 0
	for _, p := range pending {
		if p.gone {
			done = append(done, p.key)
			continue
		}
		if _, err := stmt.Exec(p.values...); err != nil {
			refused = append(refused, pushFailure{key: p.key, err: err})
			continue
		}
		done = append(done, p.key)
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("dataplane: committing the log push: %w", err)
	}
	if err := r.clearMarkers("dhcp_log_dirty", "log_id", done); err != nil {
		return n, err
	}
	if err := r.recordPushFailures("dhcp_log_dirty", "log_id", refused); err != nil {
		return n, err
	}
	return n, nil
}

// auditPushColumns are the columns of an audit entry written by this plane.
//
// The archive lives in the control database; this store holds the copy the
// writers could reach. old_value and new_value are part of the list because
// migration 024 added them for exactly these writers -- a push that omitted
// them would ship every entry without the field it exists to carry.
var auditPushColumns = []string{
	"id", "user_id", "username", "action", "resource_type", "resource_id",
	"detail", "old_value", "new_value", "source_ip", "user_agent", "success", "created_at",
}

// PendingAuditLogs counts the audit entries still owed to the control database.
func (r *Replicator) PendingAuditLogs() (int, error) {
	var n int
	if err := r.store.QueryRow(`SELECT COUNT(*) FROM audit_log_dirty`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// PushAuditLogs copies the audit entries this plane wrote up to the control
// database.
//
// The write is insert-only. An audit entry is a statement about something that
// already happened, and it is immutable everywhere else -- the control table
// refuses UPDATE outright -- so a re-push of an entry that is already there has
// nothing to reconcile. DO NOTHING is what makes the retry safe rather than
// merely harmless.
func (r *Replicator) PushAuditLogs(ctx context.Context, limit int) (int, error) {
	if r.control == nil {
		return 0, ErrControlUnavailable
	}
	if limit <= 0 {
		limit = pushBatchDefault
	}

	pending, err := r.pendingRows(ctx, pendingQuery{
		markerTable: "audit_log_dirty",
		markerKey:   "log_id",
		sourceTable: "audit_logs",
		sourceKey:   "id",
		columns:     auditPushColumns,
		limit:       limit,
	})
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}

	tx, err := r.control.Begin()
	if err != nil {
		return 0, fmt.Errorf("%w: beginning the audit push: %v", ErrControlUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(insertOnlyStatement("audit_logs", auditPushColumns, "id"))
	if err != nil {
		return 0, fmt.Errorf("dataplane: preparing the audit insert: %w", err)
	}
	defer stmt.Close()

	done := make([]string, 0, len(pending))
	var refused []pushFailure
	n := 0
	for _, p := range pending {
		if p.gone {
			done = append(done, p.key)
			continue
		}
		if _, err := stmt.Exec(p.values...); err != nil {
			refused = append(refused, pushFailure{key: p.key, err: err})
			continue
		}
		done = append(done, p.key)
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("dataplane: committing the audit push: %w", err)
	}
	if err := r.clearMarkers("audit_log_dirty", "log_id", done); err != nil {
		return n, err
	}
	if err := r.recordPushFailures("audit_log_dirty", "log_id", refused); err != nil {
		return n, err
	}
	return n, nil
}

// PushRecords copies the records this plane authored up to the control
// database.
//
// Without it the console simply stops showing the names DHCP publishes. That is
// worse than it sounds: an operator looking at the zone sees names missing
// rather than a process that is not reporting, and the natural conclusion is
// that DDNS is broken. Deletes travel too, and are restricted to
// authored_locally = 1 on the far side, so a withdrawal decided here can never
// remove a record an operator typed in.
func (r *Replicator) PushRecords(ctx context.Context, limit int) (PushResult, error) {
	var res PushResult
	if r.control == nil {
		return res, ErrControlUnavailable
	}
	if limit <= 0 {
		limit = pushBatchDefault
	}

	pending, err := r.pendingRows(ctx, pendingQuery{
		markerTable: "dns_record_dirty",
		markerKey:   "record_id",
		sourceTable: "dns_records",
		sourceKey:   "id",
		columns:     recordPushColumns,
		hasDeleted:  true,
		limit:       limit,
	})
	if err != nil {
		return res, err
	}
	if len(pending) == 0 {
		return res, nil
	}

	tx, err := r.control.Begin()
	if err != nil {
		return res, fmt.Errorf("%w: beginning the record push: %v", ErrControlUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	deleteStmt, err := tx.Prepare(
		`DELETE FROM dns_records WHERE id = ? AND authored_locally = 1`)
	if err != nil {
		return res, fmt.Errorf("dataplane: preparing the record delete: %w", err)
	}
	defer deleteStmt.Close()

	upsertStmt, err := tx.Prepare(upsertStatement("dns_records", recordPushColumns, "id"))
	if err != nil {
		return res, fmt.Errorf("dataplane: preparing the record upsert: %w", err)
	}
	defer upsertStmt.Close()

	done := make([]string, 0, len(pending))
	var refused []pushFailure
	for _, p := range pending {
		if p.gone {
			if _, err := deleteStmt.Exec(p.key); err != nil {
				refused = append(refused, pushFailure{key: p.key, err: err})
				continue
			}
			res.Deleted++
		} else {
			if _, err := upsertStmt.Exec(p.values...); err != nil {
				refused = append(refused, pushFailure{key: p.key, err: err})
				continue
			}
			res.Upserted++
		}
		done = append(done, p.key)
	}
	res.Refused = len(refused)

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("dataplane: committing the record push: %w", err)
	}
	if err := r.clearMarkers("dns_record_dirty", "record_id", done); err != nil {
		return res, err
	}
	if err := r.recordPushFailures("dns_record_dirty", "record_id", refused); err != nil {
		return res, err
	}
	res.Pending, err = r.PendingRecords()
	return res, err
}

// PendingRecords counts the locally authored records owed to the control
// database.
func (r *Replicator) PendingRecords() (int, error) {
	var n int
	if err := r.store.QueryRow(`SELECT COUNT(*) FROM dns_record_dirty`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// PendingEvents counts the outbox entries owed to the DNS side.
func (r *Replicator) PendingEvents() (int, error) {
	var n int
	if err := r.store.QueryRow(`SELECT COUNT(*) FROM dhcp_dns_event_dirty`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// PendingLogs counts the event log entries owed to the control database.
func (r *Replicator) PendingLogs() (int, error) {
	var n int
	if err := r.store.QueryRow(`SELECT COUNT(*) FROM dhcp_log_dirty`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *Replicator) clearDirty(ids []string) error {
	return r.clearMarkers("dhcp_lease_dirty", "lease_id", ids)
}

// PendingLeases counts the lease changes still owed to the control database.
func (r *Replicator) PendingLeases() (int, error) {
	var n int
	if err := r.store.QueryRow(`SELECT COUNT(*) FROM dhcp_lease_dirty`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// HeldLeases counts the leases this store is currently holding.
func (s *Store) HeldLeases() (int, error) {
	var n int
	err := s.QueryRow(
		`SELECT COUNT(*) FROM dhcp_leases WHERE status IN ('active', 'offered', 'conflict')`).Scan(&n)
	return n, err
}
