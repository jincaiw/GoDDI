package lease

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LeaseStatus represents the status of a DHCP lease.
type LeaseStatus string

const (
	LeaseStatusActive  LeaseStatus = "active"
	LeaseStatusExpired LeaseStatus = "expired"
	// LeaseStatusOffered is an address reserved by a DISCOVER that the client
	// has not yet confirmed with a REQUEST (RFC 2131 §4.3.1). It holds the
	// address out of the pool for a short time, but it is not a binding.
	LeaseStatusOffered  LeaseStatus = "offered"
	LeaseStatusReleased LeaseStatus = "released"
	// LeaseStatusConflict marks an address a client reported as already in use
	// (DECLINE) or that failed a conflict probe. It stays out of the pool until
	// its quarantine expires.
	LeaseStatusConflict LeaseStatus = "conflict"
)

// heldStatuses are the lease states that make an address unavailable for
// allocation. Keeping this in one place means the availability query and the
// expiry sweep cannot drift apart: an address that is held but never swept
// would leak out of the pool permanently.
var heldStatuses = []LeaseStatus{
	LeaseStatusActive, LeaseStatusOffered, LeaseStatusConflict,
}

// offerHold bounds how long a DISCOVER reservation is kept. Reserving the
// offered address is required (RFC 2131 §4.3.1), but holding it for the full
// lease time would let a client that never sends REQUEST — or a scanner walking
// the range — exhaust the scope.
const offerHold = 2 * time.Minute

// declineQuarantine is how long an address stays out of the pool after a
// conflict report. Re-offering immediately would hand the same bad address to
// the next client, producing a decline loop across the whole scope.
const declineQuarantine = time.Hour

// ErrAddressTaken is returned when an address is already held by a different
// client and therefore cannot be reserved for the caller.
var ErrAddressTaken = errors.New("address already held by another client")

// heldStatusPlaceholders returns a "?,?,?" placeholder list and its arguments
// for the heldStatuses set, so the availability query cannot list a subset.
func heldStatusPlaceholders() (string, []interface{}) {
	placeholders := make([]string, len(heldStatuses))
	args := make([]interface{}, len(heldStatuses))
	for i, s := range heldStatuses {
		placeholders[i] = "?"
		args[i] = string(s)
	}
	return strings.Join(placeholders, ","), args
}

// Lease represents a DHCP lease.
//
// Generation counts how many times this row has entered a held state. DNS
// records written for the binding carry the generation they were written at, so
// a teardown that was decided at generation N cannot remove the record of a
// binding that has since been renewed to generation N+1. Without it, a delayed
// delete could undo a renewal that happened while it was queued.
type Lease struct {
	ID         string      `json:"id"`
	ScopeID    string      `json:"scope_id"`
	IPAddress  string      `json:"ip_address"`
	MACAddress string      `json:"mac_address"`
	Hostname   string      `json:"hostname,omitempty"`
	ClientID   string      `json:"client_id,omitempty"`
	LeaseStart string      `json:"lease_start"`
	LeaseEnd   string      `json:"lease_end"`
	Status     LeaseStatus `json:"status"`
	LastSeen   string      `json:"last_seen"`
	Generation int64       `json:"generation"`
}

// LeaseOptions holds parameters for updating a lease.
type LeaseOptions struct {
	Hostname string       `json:"hostname,omitempty"`
	Status   *LeaseStatus `json:"status,omitempty"`
}

// LeaseFilter holds filter parameters for listing leases.
type LeaseFilter struct {
	ScopeID    string      `json:"scope_id,omitempty"`
	Status     LeaseStatus `json:"status,omitempty"`
	MACAddress string      `json:"mac_address,omitempty"`
	IPAddress  string      `json:"ip_address,omitempty"`
	Hostname   string      `json:"hostname,omitempty"`
	// Search is the console's single search box, which on this page says
	// "IP/MAC": one input standing for three columns. Dropping it here makes
	// the box a no-op, and a filter that returns everything reads as "these
	// are the matches".
	Search   string `json:"search,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// Manager provides lease management operations.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new lease manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateLease creates a new, confirmed DHCP lease.
func (m *Manager) CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*Lease, error) {
	return m.createLeaseWithStatus(scopeID, ip, mac, hostname, duration, LeaseStatusActive)
}

// createLeaseWithStatus inserts a lease row with the given status and end time.
// It is the single write path for both the offered and the active states, so
// the unique index on (scope_id, ip_address) guards the DISCOVER reservation
// and the REQUEST confirmation alike.
func (m *Manager) createLeaseWithStatus(scopeID, ip, mac, hostname string, duration time.Duration, status LeaseStatus) (*Lease, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	leaseEnd := now.Add(duration)
	generation := int64(1)
	if status == LeaseStatusOffered {
		// An offer is not a binding yet; it only becomes generation 1 when the
		// REQUEST promotes it. Keeping offers at generation 0 means a create
		// event can never be emitted for an address the client never accepted.
		generation = 0
	}

	_, err := m.db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, scopeID, ip, mac, hostname, "",
		now.Format("2006-01-02T15:04:05Z"),
		leaseEnd.Format("2006-01-02T15:04:05Z"),
		string(status),
		now.Format("2006-01-02T15:04:05Z"),
		generation)
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}

	created, err := m.GetLease(id)
	if err != nil {
		return nil, err
	}
	// An offer is not a binding yet, so only a lease created already confirmed
	// is recorded as one. See ActivateLease for the DISCOVER-then-REQUEST path.
	if status == LeaseStatusActive {
		m.auditBind(nil, created)
	}
	return created, nil
}

// ReserveAddress reserves an address for a DISCOVER, returning ErrAddressTaken
// when another client holds it so the caller can retry with a fresh candidate.
//
// A retransmitted DISCOVER from the same client refreshes the existing hold
// rather than inserting a second row (which the unique index would reject), and
// an address the client already holds as a confirmed binding is returned as-is.
func (m *Manager) ReserveAddress(scopeID, ip, mac, hostname string) (*Lease, error) {
	held, err := m.GetHeldLeaseByIP(ip)
	if err != nil {
		return nil, err
	}
	if held != nil {
		if held.MACAddress != mac {
			return nil, ErrAddressTaken
		}
		if held.Status == LeaseStatusOffered {
			return m.refreshHold(held.ID)
		}
		return held, nil
	}
	return m.createLeaseWithStatus(scopeID, ip, mac, hostname, offerHold, LeaseStatusOffered)
}

// refreshHold extends an outstanding DISCOVER reservation.
func (m *Manager) refreshHold(id string) (*Lease, error) {
	now := time.Now().UTC()
	if _, err := m.db.Exec(`
		UPDATE dhcp_leases SET lease_end=?, last_seen=datetime('now')
		WHERE id=? AND status=?`,
		now.Add(offerHold).Format("2006-01-02T15:04:05Z"),
		id, string(LeaseStatusOffered)); err != nil {
		return nil, fmt.Errorf("failed to refresh offer: %w", err)
	}
	return m.GetLease(id)
}

// ActivateLease promotes an offered reservation (or renews an active lease) to
// a confirmed binding with the full lease time. This is the REQUEST half of the
// handshake: the ACK may only be sent after this returns successfully.
//
// The previous state is read first because the two cases are not the same
// event. Promoting an offer creates a binding and is recorded; renewing an
// active lease only extends one, and a trail of every renewal would be a trail
// of nothing.
func (m *Manager) ActivateLease(id string, duration time.Duration) (*Lease, error) {
	now := time.Now().UTC()

	previous, found, err := m.findLease(id)
	if err != nil {
		return nil, fmt.Errorf("reading lease %s: %w", id, err)
	}
	if !found {
		// Missing lease and wrong state are reported the same way, which is what
		// the single UPDATE's zero-row result used to do.
		return nil, fmt.Errorf("lease %s is not in an activatable state", id)
	}

	// Promoting an offer (or renewing an active lease) is a new binding
	// generation: any teardown queued before this point must not be able to
	// remove the record written from here on.
	res, err := m.db.Exec(`
		UPDATE dhcp_leases SET lease_start=?, lease_end=?, status=?, last_seen=datetime('now'),
			generation = generation + 1
		WHERE id=? AND status IN (?, ?)`,
		now.Format("2006-01-02T15:04:05Z"),
		now.Add(duration).Format("2006-01-02T15:04:05Z"),
		string(LeaseStatusActive), id,
		string(LeaseStatusOffered), string(LeaseStatusActive))
	if err != nil {
		return nil, fmt.Errorf("failed to activate lease: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil, fmt.Errorf("lease %s is not in an activatable state", id)
	}

	activated, err := m.GetLease(id)
	if err != nil {
		return nil, err
	}
	if previous.Status == LeaseStatusOffered {
		m.auditBind(previous, activated)
	}
	return activated, nil
}

// GetLease retrieves a lease by ID.
func (m *Manager) GetLease(id string) (*Lease, error) {
	l, found, err := m.findLease(id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("lease not found")
	}
	return l, nil
}

// findLease reads one lease row, reporting whether it was there at all.
//
// The callers that change state need the difference between "this lease is not
// here" and "this read failed": the first is a transition that did not happen
// and must not be recorded, the second is a database that needs attention. A
// single error value conflates them, and conflating them in an audit writer
// means either a false entry or a lost one.
func (m *Manager) findLease(id string) (*Lease, bool, error) {
	l := &Lease{}
	var hostname, clientID sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation
		FROM dhcp_leases WHERE id = ?`, id,
	).Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
		&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	l.Hostname = hostname.String
	l.ClientID = clientID.String
	return l, true, nil
}

// GetLeaseByMAC retrieves an active lease by MAC address.
func (m *Manager) GetLeaseByMAC(mac string) (*Lease, error) {
	l := &Lease{}
	var hostname, clientID sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation
		FROM dhcp_leases WHERE mac_address = ? AND status = ?
		ORDER BY lease_end DESC LIMIT 1`, mac, string(LeaseStatusActive),
	).Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
		&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	l.Hostname = hostname.String
	l.ClientID = clientID.String
	return l, nil
}

// GetLeaseByIP retrieves an active lease by IP address.
func (m *Manager) GetLeaseByIP(ip string) (*Lease, error) {
	l := &Lease{}
	var hostname, clientID sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation
		FROM dhcp_leases WHERE ip_address = ? AND status = ?
		ORDER BY lease_end DESC LIMIT 1`, ip, string(LeaseStatusActive),
	).Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
		&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	l.Hostname = hostname.String
	l.ClientID = clientID.String
	return l, nil
}

// GetHeldLeaseByIP returns the lease currently holding an address, whether it is
// a confirmed binding or an outstanding DISCOVER reservation. Allocation and
// conflict decisions must use this rather than GetLeaseByIP: the latter only
// sees confirmed bindings, so an address with a pending offer would look free
// and could be handed to a second client.
func (m *Manager) GetHeldLeaseByIP(ip string) (*Lease, error) {
	l := &Lease{}
	var hostname, clientID sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation
		FROM dhcp_leases WHERE ip_address = ? AND status IN (?, ?)
		ORDER BY lease_end DESC LIMIT 1`, ip,
		string(LeaseStatusActive), string(LeaseStatusOffered),
	).Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
		&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	l.Hostname = hostname.String
	l.ClientID = clientID.String
	return l, nil
}

// ListLeases lists leases with filtering and pagination.
func (m *Manager) ListLeases(filter LeaseFilter) ([]Lease, int64, error) {
	var conditions []string
	var args []interface{}

	if filter.ScopeID != "" {
		conditions = append(conditions, "scope_id = ?")
		args = append(args, filter.ScopeID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.MACAddress != "" {
		conditions = append(conditions, "mac_address = ?")
		args = append(args, filter.MACAddress)
	}
	if filter.IPAddress != "" {
		conditions = append(conditions, "ip_address = ?")
		args = append(args, filter.IPAddress)
	}
	if filter.Hostname != "" {
		conditions = append(conditions, "hostname LIKE ?")
		args = append(args, "%"+filter.Hostname+"%")
	}
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		// A full address is matched exactly: `LIKE '%192.0.2.20%'` also returns
		// 192.0.2.200 through 192.0.2.209, and "which lease holds this address"
		// is the question the box is there to answer.
		if net.ParseIP(filter.Search) != nil {
			conditions = append(conditions, "(ip_address = ? OR mac_address LIKE ? OR hostname LIKE ?)")
			args = append(args, filter.Search, like, like)
		} else {
			conditions = append(conditions, "(ip_address LIKE ? OR mac_address LIKE ? OR hostname LIKE ?)")
			args = append(args, like, like, like)
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM dhcp_leases " + whereClause
	if err := m.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := filter.Page
	pageSize := filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	querySQL := `SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
		lease_start, lease_end, status, last_seen, generation
		FROM dhcp_leases ` + whereClause + ` ORDER BY lease_end DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := m.db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var leases []Lease
	for rows.Next() {
		var l Lease
		var hostname, clientID sql.NullString
		if err := rows.Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress,
			&hostname, &clientID, &l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation); err != nil {
			return nil, 0, err
		}
		l.Hostname = hostname.String
		l.ClientID = clientID.String
		leases = append(leases, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return leases, total, nil
}

// UpdateLease updates a lease.
func (m *Manager) UpdateLease(id string, opts LeaseOptions) (*Lease, error) {
	existing, err := m.GetLease(id)
	if err != nil {
		return nil, err
	}

	if opts.Hostname != "" {
		existing.Hostname = opts.Hostname
	}
	if opts.Status != nil {
		existing.Status = *opts.Status
	}

	_, err = m.db.Exec(`
		UPDATE dhcp_leases SET hostname=?, status=?, last_seen=datetime('now')
		WHERE id=?`, existing.Hostname, string(existing.Status), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update lease: %w", err)
	}

	return m.GetLease(id)
}

// RenewLease renews a lease by extending its end time.
func (m *Manager) RenewLease(id string, duration time.Duration) (*Lease, error) {
	now := time.Now().UTC()
	leaseEnd := now.Add(duration)

	res, err := m.db.Exec(`
		UPDATE dhcp_leases SET lease_start=?, lease_end=?, status=?, last_seen=datetime('now'),
			generation = generation + 1
		WHERE id=? AND status=?`,
		now.Format("2006-01-02T15:04:05Z"),
		leaseEnd.Format("2006-01-02T15:04:05Z"),
		string(LeaseStatusActive), id, string(LeaseStatusActive))
	if err != nil {
		return nil, fmt.Errorf("failed to renew lease: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, fmt.Errorf("lease %s not found or not active", id)
	}

	return m.GetLease(id)
}

// ReleaseLease releases a lease (marks it as released).
func (m *Manager) ReleaseLease(id string) error {
	before, found, err := m.findLease(id)
	if err != nil {
		return fmt.Errorf("reading lease %s: %w", id, err)
	}
	if !found {
		// Releasing a lease that is already gone stays a silent success: the
		// caller asked for the address to be free, and it is.
		return nil
	}

	res, err := m.db.Exec(`
		UPDATE dhcp_leases SET status=?, last_seen=datetime('now')
		WHERE id=?`, string(LeaseStatusReleased), id)
	if err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}
	// Only report a transition that happened. A release that matched no row --
	// the lease disappeared between the read and the write -- is not one.
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil
	}

	released, err := m.GetLease(id)
	if err != nil {
		return err
	}
	m.auditRelease(before, released)
	return nil
}

// MarkLeaseConflict marks a lease as conflict (IP declined by client) and sets
// the start of its quarantine: the address must stay out of the pool until the
// quarantine expires, not merely be relabelled.
func (m *Manager) MarkLeaseConflict(id string) error {
	now := time.Now().UTC()

	before, found, err := m.findLease(id)
	if err != nil {
		return fmt.Errorf("reading lease %s: %w", id, err)
	}
	if !found {
		// Marking a lease that is not there stays a silent success, as it was
		// when the update's zero-row result was the only signal.
		return nil
	}

	res, err := m.db.Exec(`
		UPDATE dhcp_leases SET status=?, lease_end=?, last_seen=datetime('now')
		WHERE id=?`,
		string(LeaseStatusConflict),
		now.Add(declineQuarantine).Format("2006-01-02T15:04:05Z"),
		id)
	if err != nil {
		return fmt.Errorf("failed to mark lease as conflict: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil
	}

	conflicted, err := m.GetLease(id)
	if err != nil {
		return err
	}
	m.auditDecline(before, conflicted)
	return nil
}

// QuarantineIP takes an address out of allocation for declineQuarantine. If a
// lease already holds the address its quarantine is set in place; otherwise a
// tombstone row is written, because a DECLINE can arrive for an address this
// server never recorded (a statically configured host, or a conflict seen by a
// peer) and without the tombstone the next DISCOVER would offer it again.
//
// It returns the lease the address was bound to, or nil when only a tombstone
// was written. A declined address must also lose its DNS record: the client
// just told us the binding is wrong, so leaving the name published would keep
// handing out an address that is already in use by something else.
func (m *Manager) QuarantineIP(scopeID, ip, mac string) (*Lease, error) {
	if scopeID == "" {
		return nil, fmt.Errorf("cannot quarantine %s without a scope", ip)
	}

	var id string
	err := m.db.QueryRow(`
		SELECT id FROM dhcp_leases
		WHERE ip_address = ? AND status IN (?, ?, ?)
		ORDER BY lease_end DESC LIMIT 1`, ip,
		string(LeaseStatusActive), string(LeaseStatusOffered), string(LeaseStatusConflict),
	).Scan(&id)
	switch {
	case err == nil:
		if err := m.MarkLeaseConflict(id); err != nil {
			return nil, err
		}
		return m.GetLease(id)
	case !errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("failed to look up lease for IP %s: %w", ip, err)
	}

	now := time.Now().UTC()
	tombstoneID := uuid.New().String()
	if _, err := m.db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation)
		VALUES (?, ?, ?, ?, '', '', ?, ?, ?, ?, 0)`,
		tombstoneID, scopeID, ip, mac,
		now.Format("2006-01-02T15:04:05Z"),
		now.Add(declineQuarantine).Format("2006-01-02T15:04:05Z"),
		string(LeaseStatusConflict),
		now.Format("2006-01-02T15:04:05Z")); err != nil {
		return nil, fmt.Errorf("failed to write conflict tombstone for %s: %w", ip, err)
	}

	// A decline for an address this server never recorded is still a decline,
	// and it is the one a reader is most likely to need: the address is being
	// quarantined on nothing but the client's word.
	if tombstone, found, err := m.findLease(tombstoneID); err != nil {
		slog.Error("dhcp: could not read back the conflict tombstone for auditing",
			"lease", tombstoneID, "ip", ip, "error", err)
	} else if found {
		m.auditDecline(nil, tombstone)
	}
	return nil, nil
}

// ExpireLeases moves every held lease whose time is up to expired, which
// releases the address back to the pool.
// Uses julianday() for the comparison because lease_end is stored in RFC3339
// format ("2006-01-02T15:04:05Z", note the "T") which cannot be compared
// lexicographically against datetime('now') output ("2006-01-02 15:04:05").
// With a plain string compare, leases expiring on the current day never match
// (byte 'T' > ' ') and are only reclaimed a day late, exhausting pools.
// julianday() parses both ISO8601 variants; invalid values yield NULL and are
// skipped (fail-safe).
//
// Offered reservations and conflict quarantines are swept by the same rule, so
// an address can never become permanently unreachable through a state that no
// sweep covers.
//
// The return value is the list of leases the sweep actually moved, so the
// caller can tear down the DNS records written for those bindings: a client
// that simply leaves — the common case, since most clients never send RELEASE —
// must not leave its name pointing at an address the server is free to hand to
// somebody else.
//
// The read and the update run in one transaction, which is what makes that list
// exact. They used to be two statements, and a lease renewed in between would
// appear in the returned list while its status never changed; the DNS teardown
// tolerated that because it carries a generation, and the audit entry cannot --
// "this binding expired" is a statement about something that did not happen.
// One transaction removes the gap instead of teaching each reader to live with
// it. The sweep is serialized against every other lease write either way,
// because the pool holds a single connection.
func (m *Manager) ExpireLeases() ([]*Lease, error) {
	placeholder, statusArgs := heldStatusPlaceholders()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning the expiry sweep: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Materialize the candidates first: the cursor must be fully drained and
	// closed before the UPDATE runs. With SetMaxOpenConns(1) a query issued
	// while a cursor is still open waits forever on the connection that cursor
	// is holding, with no error and no timeout.
	selectSQL := `SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
		lease_start, lease_end, status, last_seen, generation
		FROM dhcp_leases
		WHERE status IN (` + placeholder + `) AND julianday(lease_end) <= julianday('now')`
	rows, err := tx.Query(selectSQL, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("selecting expired leases: %w", err)
	}
	var swept []*Lease
	for rows.Next() {
		l := &Lease{}
		var hostname, clientID sql.NullString
		if err := rows.Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress,
			&hostname, &clientID, &l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen,
			&l.Generation); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning expired lease: %w", err)
		}
		l.Hostname = hostname.String
		l.ClientID = clientID.String
		swept = append(swept, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating expired leases: %w", err)
	}

	updateSQL := `UPDATE dhcp_leases SET status=?
		WHERE status IN (` + placeholder + `) AND julianday(lease_end) <= julianday('now')`
	args := append([]interface{}{string(LeaseStatusExpired)}, statusArgs...)
	if _, err := tx.Exec(updateSQL, args...); err != nil {
		return nil, fmt.Errorf("failed to expire leases: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing the expiry sweep: %w", err)
	}

	// Audited after the commit, and one entry per address: an expiry is the
	// only transition where the address changes hands without anything being
	// said, and "when did 192.0.2.5 become free" is the question this answers.
	// The sweep runs on a timer, so a failure here is logged and dropped like
	// every other audit failure -- the addresses are already back in the pool.
	for _, l := range swept {
		after := *l
		after.Status = LeaseStatusExpired
		m.auditExpire(l, &after)
	}

	return swept, nil
}

// FindAvailableIP finds the next available IP in a scope.
// It skips IPs that are held by a lease in any of heldStatuses (confirmed
// bindings, outstanding DISCOVER offers, and conflict quarantines) or that have
// an enabled reservation.
//
// NOTE: This method is not atomic — two concurrent DISCOVER requests may obtain
// the same available IP. The actual atomicity is enforced by the write that
// follows, which relies on the UNIQUE index on (scope_id, ip_address) covering
// both 'active' and 'offered' (migration 017) to reject duplicate allocations.
// Callers must handle that error and retry with a different IP; see
// Server.reserveAddress.
//
// The query streams the union of in-use IPs directly from the database and
// checks each candidate in the scope range with a NOT EXISTS subquery, avoiding
// loading the entire scope into memory.
// The held set is read once and the first gap is found in memory, rather than
// probing each candidate address with its own query. The probing version was
// correct and was measured at the documented ceiling: with 20,000 of a /16
// pool handed out, finding the next free address cost **310 ms** and issued two
// queries per candidate, on the DISCOVER path, with up to five retries per
// client. The cost grew with the number of addresses already in use, which is
// exactly backwards -- the busier the pool, the slower the next client. Reading
// the held set is one query whose cost is proportional to what is held, and the
// scan for a gap is a sort and a walk over that same set.
//
// The answer is unchanged, address for address: the first address in
// [start_ip, end_ip] that neither a held lease nor an enabled reservation
// occupies. TestTheFirstFreeAddressIsTheOneBruteForceWouldFind pins that
// against a reference implementation so an optimisation cannot quietly change
// which address a client is given.
func (m *Manager) FindAvailableIP(scopeID string) (string, error) {
	// Get scope range.
	var startIP, endIP string
	err := m.db.QueryRow(`
		SELECT start_ip, end_ip FROM dhcp_scopes WHERE id = ? AND enabled = 1`, scopeID,
	).Scan(&startIP, &endIP)
	if err != nil {
		return "", fmt.Errorf("scope not found or disabled: %w", err)
	}

	start := net.ParseIP(startIP)
	end := net.ParseIP(endIP)
	if start == nil || end == nil {
		return "", fmt.Errorf("invalid scope IP range")
	}
	start4 := start.To4()
	end4 := end.To4()
	if start4 == nil || end4 == nil {
		return "", fmt.Errorf("only IPv4 scopes are supported")
	}
	from := uint64(start4[0])<<24 | uint64(start4[1])<<16 | uint64(start4[2])<<8 | uint64(start4[3])
	to := uint64(end4[0])<<24 | uint64(end4[1])<<16 | uint64(end4[2])<<8 | uint64(end4[3])
	if from > to {
		return "", fmt.Errorf("no available IP addresses in scope")
	}

	// The held-status placeholder list comes from heldStatuses so a status can
	// never be excluded from allocation without also being excluded here.
	statusPlaceholders, statusArgs := heldStatusPlaceholders()
	heldQuery := `SELECT ip_address FROM dhcp_leases
			WHERE scope_id = ? AND status IN (` + statusPlaceholders + `)
		UNION
		SELECT ip_address FROM dhcp_reservations
			WHERE scope_id = ? AND enabled = 1`

	args := append([]interface{}{scopeID}, statusArgs...)
	args = append(args, scopeID)
	rows, err := m.db.Query(heldQuery, args...)
	if err != nil {
		return "", fmt.Errorf("reading the addresses held in scope %s: %w", scopeID, err)
	}

	// Materialise before anything else: the store is capped at one connection,
	// so issuing a query while this cursor is open would block forever rather
	// than fail. Nothing else is queried on this path, and closing here keeps
	// it that way.
	taken := make([]uint64, 0, 256)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			rows.Close()
			return "", fmt.Errorf("scanning a held address in scope %s: %w", scopeID, err)
		}
		// A row the address column cannot parse is skipped rather than fatal,
		// matching the old per-candidate query: such a row could never have
		// matched a candidate either, so treating it as held would refuse
		// addresses the previous behaviour handed out.
		if v, ok := ipv4ToUint64(ip); ok {
			taken = append(taken, v)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", fmt.Errorf("reading the addresses held in scope %s: %w", scopeID, err)
	}
	rows.Close()

	sort.Slice(taken, func(i, j int) bool { return taken[i] < taken[j] })

	// Walk the held set looking for the first gap. `next` only ever moves
	// forward, so the walk is one pass and the answer is the same address the
	// per-candidate probe would have returned.
	next := from
	for _, v := range taken {
		if v < next || v > to {
			continue
		}
		if v > next {
			break
		}
		next = v + 1
	}
	if next > to {
		return "", fmt.Errorf("no available IP addresses in scope")
	}
	return uint64ToIPv4(next), nil
}

// ipv4ToUint64 converts a dotted quad to its numeric value, or reports that the
// string is not one.
func ipv4ToUint64(ip string) (uint64, bool) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0, false
	}
	v4 := parsed.To4()
	if v4 == nil {
		return 0, false
	}
	return uint64(v4[0])<<24 | uint64(v4[1])<<16 | uint64(v4[2])<<8 | uint64(v4[3]), true
}

func uint64ToIPv4(v uint64) string {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String()
}

// PingCheckEnabled controls whether PingCheck is performed.
// Default is false because net.DialTimeout("ip4:icmp", ...) only establishes a
// raw ICMP socket — it does NOT actually send an ICMP Echo Request or wait for
// a reply. The result is therefore unreliable and may produce false positives
// (reporting an IP as in use when it is not) or false negatives.
// Enable only if you have verified that the underlying OS behaves as expected.
var PingCheckEnabled = false

// PingCheck performs a simple ICMP ping check for conflict detection.
// Returns true if the IP is in use (responds to ping).
//
// WARNING: This implementation uses net.DialTimeout("ip4:icmp", ...) which
// only opens a raw ICMP socket connection — it does NOT send an ICMP Echo
// Request or wait for a reply. The result is unreliable. Disabled by default;
// controlled by the PingCheckEnabled variable.
func PingCheck(ip string) bool {
	if !PingCheckEnabled {
		return false
	}
	c, err := net.DialTimeout("ip4:icmp", ip, 1*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
