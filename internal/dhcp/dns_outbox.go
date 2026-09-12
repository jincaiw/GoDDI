package dhcp

import (
	"database/sql"
	"fmt"
	"time"
)

// DNSEventAction is the kind of change a lease transition owes the DNS side.
type DNSEventAction string

const (
	// DNSEventCreate publishes (or refreshes) the A/PTR records for a binding.
	DNSEventCreate DNSEventAction = "create"
	// DNSEventDelete withdraws them because the binding no longer holds the
	// address.
	DNSEventDelete DNSEventAction = "delete"
)

// DNSOutboxRetryBase and DNSOutboxRetryMax bound the retry backoff of a failed
// DNS update. A DNS write can fail because the database is momentarily busy or
// the row is contended; retrying forever at full speed would turn that into a
// busy loop, while giving up after one attempt loses the update silently.
const (
	DNSOutboxRetryBase = 2 * time.Second
	DNSOutboxRetryMax  = 5 * time.Minute
	// DNSOutboxMaxAttempts is where an event is abandoned. Abandoned rows are
	// kept (status='failed') rather than deleted so the drift stays visible for
	// alerting instead of disappearing.
	DNSOutboxMaxAttempts = 8
)

// DNSEvent is one durable, replayable DNS change owed by a DHCP binding.
//
// The event carries the lease generation it was decided at. Applying it checks
// that generation against the lease's current value, so a create replayed
// after the binding was torn down does not resurrect the record, and a delete
// replayed after a renewal does not remove the record of the live binding.
type DNSEvent struct {
	ID         int64
	LeaseID    string
	Generation int64
	Action     DNSEventAction
	ScopeID    string
	IPAddress  string
	MACAddress string
	Hostname   string
	Attempts   int
}

// DNSOutbox is the durable queue of DNS changes derived from DHCP lease
// transitions.
//
// Dispatching the update straight from a goroutine meant a crash between the
// ACK and the write lost the change with no trace: the client had a working
// lease, the name was never published, and nothing recorded that it was owed.
// Persisting the intent first makes the change survive a restart, and lets a
// consumer replay it until it succeeds.
type DNSOutbox struct {
	db *sql.DB
}

// NewDNSOutbox creates an outbox backed by the given database.
func NewDNSOutbox(db *sql.DB) *DNSOutbox {
	return &DNSOutbox{db: db}
}

// Enqueue records a pending DNS change.
//
// Events are deliberately not coalesced: a create followed by the delete that
// supersedes it must both be replayable, in that order, so the consumer can
// reach the right end state from any starting point.
func (o *DNSOutbox) Enqueue(e DNSEvent) error {
	if e.LeaseID == "" || e.Action == "" {
		return fmt.Errorf("dns outbox: event needs a lease id and an action")
	}
	_, err := o.db.Exec(`
		INSERT INTO dhcp_dns_events
			(lease_id, generation, action, scope_id, ip_address, mac_address, hostname)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.LeaseID, e.Generation, string(e.Action),
		e.ScopeID, e.IPAddress, e.MACAddress, e.Hostname)
	if err != nil {
		return fmt.Errorf("dns outbox: enqueue %s for lease %s: %w", e.Action, e.LeaseID, err)
	}
	return nil
}

// PendingBatch returns up to limit events that are due, oldest first.
//
// Ordering by id is what keeps a create from being applied after the delete
// that superseded it: the log is append-only, so id order is the order the
// transitions actually happened.
func (o *DNSOutbox) PendingBatch(limit int) ([]DNSEvent, error) {
	if limit <= 0 {
		limit = 32
	}
	rows, err := o.db.Query(`
		SELECT id, lease_id, generation, action, scope_id, ip_address, mac_address, hostname, attempts
		FROM dhcp_dns_events
		WHERE status = 'pending' AND julianday(next_attempt_at) <= julianday('now')
		ORDER BY id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("dns outbox: selecting pending events: %w", err)
	}
	defer rows.Close()

	var events []DNSEvent
	for rows.Next() {
		var e DNSEvent
		var action string
		if err := rows.Scan(&e.ID, &e.LeaseID, &e.Generation, &action,
			&e.ScopeID, &e.IPAddress, &e.MACAddress, &e.Hostname, &e.Attempts); err != nil {
			return nil, fmt.Errorf("dns outbox: scanning event: %w", err)
		}
		e.Action = DNSEventAction(action)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dns outbox: iterating events: %w", err)
	}
	return events, nil
}

// MarkDone retires an event that has been applied.
func (o *DNSOutbox) MarkDone(id int64) error {
	_, err := o.db.Exec(`
		UPDATE dhcp_dns_events
		SET status='done', last_error='', updated_at=datetime('now')
		WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("dns outbox: marking event %d done: %w", id, err)
	}
	return nil
}

// MarkRetry schedules another attempt with exponential backoff, or abandons the
// event once it has exhausted its attempts. Either way the failure is recorded:
// a silently dropped DNS update is indistinguishable from one that was never
// owed.
func (o *DNSOutbox) MarkRetry(id int64, reason string) error {
	var attempts int
	if err := o.db.QueryRow(
		`SELECT attempts FROM dhcp_dns_events WHERE id=?`, id).Scan(&attempts); err != nil {
		return fmt.Errorf("dns outbox: reading attempts for event %d: %w", id, err)
	}
	attempts++

	if attempts >= DNSOutboxMaxAttempts {
		_, err := o.db.Exec(`
			UPDATE dhcp_dns_events
			SET status='failed', attempts=?, last_error=?, updated_at=datetime('now')
			WHERE id=?`, attempts, reason, id)
		if err != nil {
			return fmt.Errorf("dns outbox: abandoning event %d: %w", id, err)
		}
		return nil
	}

	backoff := DNSOutboxRetryBase << (attempts - 1)
	if backoff > DNSOutboxRetryMax || backoff <= 0 {
		backoff = DNSOutboxRetryMax
	}
	_, err := o.db.Exec(`
		UPDATE dhcp_dns_events
		SET attempts=?, last_error=?, next_attempt_at=datetime('now', ?), updated_at=datetime('now')
		WHERE id=?`, attempts, reason, fmt.Sprintf("+%d seconds", int(backoff.Seconds())), id)
	if err != nil {
		return fmt.Errorf("dns outbox: rescheduling event %d: %w", id, err)
	}
	return nil
}

// Stats reports how much work is outstanding, for metrics and alerting.
func (o *DNSOutbox) Stats() (pending, failed int64, err error) {
	err = o.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), 0)
		FROM dhcp_dns_events`).Scan(&pending, &failed)
	if err != nil {
		return 0, 0, fmt.Errorf("dns outbox: reading stats: %w", err)
	}
	return pending, failed, nil
}
