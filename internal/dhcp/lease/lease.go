package lease

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LeaseStatus represents the status of a DHCP lease.
type LeaseStatus string

const (
	LeaseStatusActive   LeaseStatus = "active"
	LeaseStatusExpired  LeaseStatus = "expired"
	LeaseStatusReleased LeaseStatus = "released"
	LeaseStatusConflict LeaseStatus = "conflict"
)

// Lease represents a DHCP lease.
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
	Page       int         `json:"page,omitempty"`
	PageSize   int         `json:"page_size,omitempty"`
}

// Manager provides lease management operations.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new lease manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateLease creates a new DHCP lease.
func (m *Manager) CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*Lease, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	leaseEnd := now.Add(duration)

	_, err := m.db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, scopeID, ip, mac, hostname, "",
		now.Format("2006-01-02T15:04:05Z"),
		leaseEnd.Format("2006-01-02T15:04:05Z"),
		string(LeaseStatusActive),
		now.Format("2006-01-02T15:04:05Z"))
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}

	return m.GetLease(id)
}

// GetLease retrieves a lease by ID.
func (m *Manager) GetLease(id string) (*Lease, error) {
	l := &Lease{}
	var hostname, clientID sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen
		FROM dhcp_leases WHERE id = ?`, id,
	).Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
		&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("lease not found")
	}
	if err != nil {
		return nil, err
	}

	l.Hostname = hostname.String
	l.ClientID = clientID.String
	return l, nil
}

// GetLeaseByMAC retrieves an active lease by MAC address.
func (m *Manager) GetLeaseByMAC(mac string) (*Lease, error) {
	l := &Lease{}
	var hostname, clientID sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen
		FROM dhcp_leases WHERE mac_address = ? AND status = ?
		ORDER BY lease_end DESC LIMIT 1`, mac, string(LeaseStatusActive),
	).Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
		&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen)
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
			lease_start, lease_end, status, last_seen
		FROM dhcp_leases WHERE ip_address = ? AND status = ?
		ORDER BY lease_end DESC LIMIT 1`, ip, string(LeaseStatusActive),
	).Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
		&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen)
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
		lease_start, lease_end, status, last_seen
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
			&hostname, &clientID, &l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen); err != nil {
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
		UPDATE dhcp_leases SET lease_start=?, lease_end=?, status=?, last_seen=datetime('now')
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
	_, err := m.db.Exec(`
		UPDATE dhcp_leases SET status=?, last_seen=datetime('now')
		WHERE id=?`, string(LeaseStatusReleased), id)
	if err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}
	return nil
}

// MarkLeaseConflict marks a lease as conflict (IP declined by client).
func (m *Manager) MarkLeaseConflict(id string) error {
	_, err := m.db.Exec(`
		UPDATE dhcp_leases SET status=?, last_seen=datetime('now')
		WHERE id=?`, string(LeaseStatusConflict), id)
	if err != nil {
		return fmt.Errorf("failed to mark lease as conflict: %w", err)
	}
	return nil
}

// ExpireLeases marks all expired leases as expired.
// Uses julianday() for the comparison because lease_end is stored in RFC3339
// format ("2006-01-02T15:04:05Z", note the "T") which cannot be compared
// lexicographically against datetime('now') output ("2006-01-02 15:04:05").
// With a plain string compare, leases expiring on the current day never match
// (byte 'T' > ' ') and are only reclaimed a day late, exhausting pools.
// julianday() parses both ISO8601 variants; invalid values yield NULL and are
// skipped (fail-safe).
func (m *Manager) ExpireLeases() error {
	_, err := m.db.Exec(`
		UPDATE dhcp_leases SET status=?
		WHERE status=? AND julianday(lease_end) <= julianday('now')`,
		string(LeaseStatusExpired), string(LeaseStatusActive))
	if err != nil {
		return fmt.Errorf("failed to expire leases: %w", err)
	}
	return nil
}

// FindAvailableIP finds the next available IP in a scope.
// It skips IPs that have active leases or reservations.
//
// NOTE: This method is not atomic — two concurrent DISCOVER requests may obtain
// the same available IP. The actual atomicity is enforced by CreateLease, which
// relies on the UNIQUE constraint on (scope_id, ip_address) WHERE status='active'
// in dhcp_leases to detect and reject duplicate allocations. Callers should
// handle the resulting error from CreateLease and retry with a different IP if
// needed.
//
// The query streams the union of in-use IPs (active leases + active
// reservations) directly from the database and checks each candidate in the
// scope range with a NOT EXISTS subquery, avoiding loading the entire scope
// into memory.
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

	// Iterate through the range and find the first available IP. Each
	// candidate is checked against the database with a NOT EXISTS subquery
	// that combines active leases and active reservations for this scope,
	// so the entire scope is never loaded into memory.
	ip := make(net.IP, len(start4))
	copy(ip, start4)
	for {
		if ip == nil {
			break
		}
		ipStr := ip.String()
		var taken int
		err := m.db.QueryRow(`
			SELECT CASE WHEN EXISTS(
				SELECT 1 FROM dhcp_leases
				WHERE scope_id = ? AND ip_address = ? AND status = 'active'
			) OR EXISTS(
				SELECT 1 FROM dhcp_reservations
				WHERE scope_id = ? AND ip_address = ? AND enabled = 1
			) THEN 1 ELSE 0 END`,
			scopeID, ipStr, scopeID, ipStr,
		).Scan(&taken)
		if err != nil {
			return "", fmt.Errorf("checking IP %s availability: %w", ipStr, err)
		}
		if taken == 0 {
			return ipStr, nil
		}
		if ip.Equal(end4) {
			break
		}
		ip = nextIP(ip)
	}

	return "", fmt.Errorf("no available IP addresses in scope")
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

// nextIP returns the next IP address in sequence.
// Returns nil if the IP overflows (all bytes are 255) or if the IP is IPv6
// (this implementation only handles IPv4 leases).
func nextIP(ip net.IP) net.IP {
	if len(ip) == 16 {
		// IPv6 is not supported by the DHCPv4 lease manager.
		return nil
	}
	ip = ip.To4()
	if ip == nil {
		return nil
	}
	// Check for overflow: if all bytes are 255, return nil.
	allMax := true
	for _, b := range ip {
		if b != 255 {
			allMax = false
			break
		}
	}
	if allMax {
		return nil
	}
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] > 0 {
			break
		}
	}
	return next
}
