package scope

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Scope represents a DHCP scope (subnet).
type Scope struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Interface        string `json:"interface,omitempty"`
	Subnet           string `json:"subnet"`
	StartIP          string `json:"start_ip"`
	EndIP            string `json:"end_ip"`
	SubnetMask       string `json:"subnet_mask,omitempty"`
	Router           string `json:"router,omitempty"`
	DNSServers       string `json:"dns_servers,omitempty"`
	NTPServers       string `json:"ntp_servers,omitempty"`
	DomainName       string `json:"domain_name,omitempty"`
	LeaseTime        int    `json:"lease_time"`
	MaxLeaseTime     int    `json:"max_lease_time,omitempty"`
	Enabled          bool   `json:"enabled"`
	PingCheckEnabled bool   `json:"ping_check_enabled"`
	DNSUpdates       bool   `json:"dns_updates"`
	Comment          string `json:"comment,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// ScopeOptions holds parameters for creating or updating a scope.
type ScopeOptions struct {
	Name             string  `json:"name"`
	Interface        *string `json:"interface,omitempty"`
	Subnet           string  `json:"subnet"`
	StartIP          string  `json:"start_ip"`
	EndIP            string  `json:"end_ip"`
	SubnetMask       *string `json:"subnet_mask,omitempty"`
	Router           *string `json:"router,omitempty"`
	DNSServers       *string `json:"dns_servers,omitempty"`
	NTPServers       *string `json:"ntp_servers,omitempty"`
	DomainName       *string `json:"domain_name,omitempty"`
	LeaseTime        *int    `json:"lease_time,omitempty"`
	MaxLeaseTime     *int    `json:"max_lease_time,omitempty"`
	Enabled          *bool   `json:"enabled,omitempty"`
	PingCheckEnabled *bool   `json:"ping_check_enabled,omitempty"`
	DNSUpdates       *bool   `json:"dns_updates,omitempty"`
	Comment          *string `json:"comment,omitempty"`
}

// ScopeFilter holds filter parameters for listing scopes.
type ScopeFilter struct {
	Name     string `json:"name,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Subnet   string `json:"subnet,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// Manager provides CRUD operations for DHCP scopes.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new scope manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateScope creates a new DHCP scope.
func (m *Manager) CreateScope(opts ScopeOptions) (*Scope, error) {
	if err := ValidateScopeOptions(opts); err != nil {
		return nil, err
	}

	leaseTime := 86400
	if opts.LeaseTime != nil {
		leaseTime = *opts.LeaseTime
	}
	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}
	pingCheck := false
	if opts.PingCheckEnabled != nil {
		pingCheck = *opts.PingCheckEnabled
	}
	dnsUpdates := false
	if opts.DNSUpdates != nil {
		dnsUpdates = *opts.DNSUpdates
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	// A comment of "" and no comment at all are stored the same way, which is
	// what UpdateScope already does. The two used to differ ('' here, NULL
	// there) because this call passed the pointer straight through: an omitted
	// comment became NULL and an empty one became '', and which one a scope had
	// depended on whether it had ever been edited.
	comment := ""
	if opts.Comment != nil {
		comment = *opts.Comment
	}

	_, err := m.db.Exec(`
		INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip, subnet_mask,
			router, dns_servers, ntp_servers, domain_name, lease_time, max_lease_time,
			enabled, ping_check_enabled, dns_updates, comment, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, opts.Name, opts.Interface, opts.Subnet, opts.StartIP, opts.EndIP,
		opts.SubnetMask, opts.Router, opts.DNSServers, opts.NTPServers,
		opts.DomainName, leaseTime, opts.MaxLeaseTime, enabled, pingCheck,
		dnsUpdates, nullIfEmpty(comment), now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create scope: %w", err)
	}

	return m.GetScope(id)
}

// GetScope retrieves a scope by ID.
func (m *Manager) GetScope(id string) (*Scope, error) {
	s := &Scope{}
	var maxLeaseTime sql.NullInt64
	var iface, subnetMask, router, dnsServers, ntpServers, domainName, comment sql.NullString

	err := m.db.QueryRow(`
		SELECT id, name, interface, subnet, start_ip, end_ip, subnet_mask,
			router, dns_servers, ntp_servers, domain_name, lease_time, max_lease_time,
			enabled, ping_check_enabled, dns_updates, comment, created_at, updated_at
		FROM dhcp_scopes WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &iface, &s.Subnet, &s.StartIP, &s.EndIP,
		&subnetMask, &router, &dnsServers, &ntpServers, &domainName,
		&s.LeaseTime, &maxLeaseTime, &s.Enabled, &s.PingCheckEnabled,
		&s.DNSUpdates, &comment, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scope not found")
	}
	if err != nil {
		return nil, err
	}

	s.Interface = iface.String
	s.SubnetMask = subnetMask.String
	s.Router = router.String
	s.DNSServers = dnsServers.String
	s.NTPServers = ntpServers.String
	s.DomainName = domainName.String
	s.Comment = comment.String
	if maxLeaseTime.Valid {
		s.MaxLeaseTime = int(maxLeaseTime.Int64)
	}

	return s, nil
}

// ListScopes lists scopes with filtering and pagination.
func (m *Manager) ListScopes(filter ScopeFilter) ([]Scope, int64, error) {
	var conditions []string
	var args []interface{}

	if filter.Name != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}
	if filter.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, *filter.Enabled)
	}
	if filter.Subnet != "" {
		conditions = append(conditions, "subnet = ?")
		args = append(args, filter.Subnet)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM dhcp_scopes " + whereClause
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

	querySQL := `SELECT id, name, interface, subnet, start_ip, end_ip, subnet_mask,
		router, dns_servers, ntp_servers, domain_name, lease_time, max_lease_time,
		enabled, ping_check_enabled, dns_updates, comment, created_at, updated_at
		FROM dhcp_scopes ` + whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := m.db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var scopes []Scope
	for rows.Next() {
		var s Scope
		var maxLeaseTime sql.NullInt64
		var iface, subnetMask, router, dnsServers, ntpServers, domainName, comment sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &iface, &s.Subnet, &s.StartIP, &s.EndIP,
			&subnetMask, &router, &dnsServers, &ntpServers, &domainName,
			&s.LeaseTime, &maxLeaseTime, &s.Enabled, &s.PingCheckEnabled,
			&s.DNSUpdates, &comment, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		s.Interface = iface.String
		s.SubnetMask = subnetMask.String
		s.Router = router.String
		s.DNSServers = dnsServers.String
		s.NTPServers = ntpServers.String
		s.DomainName = domainName.String
		s.Comment = comment.String
		if maxLeaseTime.Valid {
			s.MaxLeaseTime = int(maxLeaseTime.Int64)
		}
		scopes = append(scopes, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return scopes, total, nil
}

// UpdateScope updates a DHCP scope.
//
// The optional string fields in ScopeOptions use *string so that callers can
// distinguish "not provided" (nil) from "explicitly cleared" (pointer to "").
// This is needed to support clearing fields such as router, comment, etc.
func (m *Manager) UpdateScope(id string, opts ScopeOptions) (*Scope, error) {
	existing, err := m.GetScope(id)
	if err != nil {
		return nil, err
	}

	// Apply updates.
	if opts.Name != "" {
		existing.Name = opts.Name
	}
	if opts.Interface != nil {
		existing.Interface = *opts.Interface
	}
	if opts.Subnet != "" {
		existing.Subnet = opts.Subnet
	}
	if opts.StartIP != "" {
		existing.StartIP = opts.StartIP
	}
	if opts.EndIP != "" {
		existing.EndIP = opts.EndIP
	}
	if opts.SubnetMask != nil {
		existing.SubnetMask = *opts.SubnetMask
	}
	if opts.Router != nil {
		existing.Router = *opts.Router
	}
	if opts.DNSServers != nil {
		existing.DNSServers = *opts.DNSServers
	}
	if opts.NTPServers != nil {
		existing.NTPServers = *opts.NTPServers
	}
	if opts.DomainName != nil {
		existing.DomainName = *opts.DomainName
	}
	if opts.LeaseTime != nil {
		existing.LeaseTime = *opts.LeaseTime
	}
	if opts.MaxLeaseTime != nil {
		existing.MaxLeaseTime = *opts.MaxLeaseTime
	}
	if opts.Enabled != nil {
		existing.Enabled = *opts.Enabled
	}
	if opts.PingCheckEnabled != nil {
		existing.PingCheckEnabled = *opts.PingCheckEnabled
	}
	if opts.DNSUpdates != nil {
		existing.DNSUpdates = *opts.DNSUpdates
	}
	if opts.Comment != nil {
		existing.Comment = *opts.Comment
	}

	// Validate updated values.
	validateOpts := ScopeOptions{
		Subnet:  existing.Subnet,
		StartIP: existing.StartIP,
		EndIP:   existing.EndIP,
	}
	if err := ValidateScopeOptions(validateOpts); err != nil {
		return nil, err
	}

	_, err = m.db.Exec(`
		UPDATE dhcp_scopes SET name=?, interface=?, subnet=?, start_ip=?, end_ip=?,
			subnet_mask=?, router=?, dns_servers=?, ntp_servers=?, domain_name=?,
			lease_time=?, max_lease_time=?, enabled=?, ping_check_enabled=?,
			dns_updates=?, comment=?, updated_at=datetime('now')
		WHERE id=?`,
		existing.Name, nullIfEmpty(existing.Interface), existing.Subnet,
		existing.StartIP, existing.EndIP, nullIfEmpty(existing.SubnetMask),
		nullIfEmpty(existing.Router), nullIfEmpty(existing.DNSServers),
		nullIfEmpty(existing.NTPServers), nullIfEmpty(existing.DomainName),
		existing.LeaseTime, nullIfZero(existing.MaxLeaseTime),
		existing.Enabled, existing.PingCheckEnabled, existing.DNSUpdates,
		nullIfEmpty(existing.Comment), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update scope: %w", err)
	}

	return m.GetScope(id)
}

// DeleteScope deletes a DHCP scope and everything that hangs off it.
//
// It refuses while a client is still bound, and that refusal has to mean the
// same thing as the cleanup it guards. The two used to disagree: the check
// blocked on any row still marked 'active', while the cleanup removed the rows
// that had expired by time or been released. A row left at 'active' past its
// lease_end is exactly a row the cleanup intends to remove, so blocking on it
// made the scope undeletable. The sweep normally moves those rows to
// 'expired', which is what hid the disagreement -- but a node that runs no
// DHCP data plane has no sweep, so there the scope could never be deleted.
//
// Check and cleanup now read one predicate: live means status = 'active' and
// the end is still in the future. Everything else goes with the scope, because
// a lease row pointing at a scope that no longer exists is not a record of
// anything -- an expired binding, a released one, a lapsed offer or a lapsed
// quarantine all describe a scope that is being removed.
func (m *Manager) DeleteScope(id string) error {
	// Use a transaction to clean up associations atomically.
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// julianday() because lease_end is stored as RFC3339 with a "T", which is
	// not lexicographically comparable to datetime('now') output ("byte 'T' >
	// ' '" would make a lease expiring today look live until tomorrow). An
	// unparseable lease_end yields NULL and so is not counted as live, which is
	// the same fail-safe reading the sweep takes.
	const liveBinding = `scope_id = ? AND status = 'active' AND julianday(lease_end) > julianday('now')`

	// Check for live leases inside the transaction to avoid a TOCTOU race.
	var leaseCount int64
	if err := tx.QueryRow("SELECT COUNT(*) FROM dhcp_leases WHERE "+liveBinding, id).Scan(&leaseCount); err != nil {
		return fmt.Errorf("failed to check active leases: %w", err)
	}
	if leaseCount > 0 {
		return fmt.Errorf("cannot delete scope with active leases (%d leases exist)", leaseCount)
	}

	// Delete DHCP options for this scope.
	if _, err := tx.Exec("DELETE FROM dhcp_options WHERE scope_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete scope options: %w", err)
	}

	// Delete DHCP reservations for this scope.
	if _, err := tx.Exec("DELETE FROM dhcp_reservations WHERE scope_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete scope reservations: %w", err)
	}

	// Every lease row for this scope, not just the expired ones. The guard
	// above already established that none of them is a live binding, so the
	// remainder are history -- and history attached to a scope that is about to
	// stop existing is the orphan the previous partial delete could leave
	// behind (a lapsed offer or quarantine was never in its WHERE clause).
	if _, err := tx.Exec("DELETE FROM dhcp_leases WHERE scope_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete leases: %w", err)
	}

	// Delete the scope itself.
	result, err := tx.Exec("DELETE FROM dhcp_scopes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete scope: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("scope not found")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// FindScopeByIP finds the scope whose subnet contains the given IP address.
// The address may be a relay agent's giaddr or a client address, so membership
// is decided by the subnet, not by the allocation pool.
func (m *Manager) FindScopeByIP(ip string) (*Scope, error) {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	rows, err := m.db.Query(`
		SELECT id, name, interface, subnet, start_ip, end_ip, subnet_mask,
			router, dns_servers, ntp_servers, domain_name, lease_time, max_lease_time,
			enabled, ping_check_enabled, dns_updates, comment, created_at, updated_at
		FROM dhcp_scopes WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s Scope
		var maxLeaseTime sql.NullInt64
		var iface, subnetMask, router, dnsServers, ntpServers, domainName, comment sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &iface, &s.Subnet, &s.StartIP, &s.EndIP,
			&subnetMask, &router, &dnsServers, &ntpServers, &domainName,
			&s.LeaseTime, &maxLeaseTime, &s.Enabled, &s.PingCheckEnabled,
			&s.DNSUpdates, &comment, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		s.Interface = iface.String
		s.SubnetMask = subnetMask.String
		s.Router = router.String
		s.DNSServers = dnsServers.String
		s.NTPServers = ntpServers.String
		s.DomainName = domainName.String
		s.Comment = comment.String
		if maxLeaseTime.Valid {
			s.MaxLeaseTime = int(maxLeaseTime.Int64)
		}

		if scopeContainsIP(&s, parsedIP) {
			return &s, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("no scope found for IP %s", ip)
}

// scopeContainsIP reports whether an address belongs to the scope's subnet.
//
// Matching on the subnet rather than on the allocation pool is what makes relayed
// traffic work. Behind a relay the identifying address is the relay's giaddr,
// which is a router address inside the subnet but normally outside the pool; a
// pool-bounds test therefore finds no scope at all and the client is silently
// ignored. The pool bounds still govern which addresses may be handed out.
func scopeContainsIP(s *Scope, ip net.IP) bool {
	if _, ipNet, err := net.ParseCIDR(s.Subnet); err == nil {
		return ipNet.Contains(ip)
	}

	// Not a valid CIDR: fall back to the pool bounds so a scope with a legacy
	// or malformed subnet value keeps behaving as before.
	startIP := net.ParseIP(s.StartIP)
	endIP := net.ParseIP(s.EndIP)
	if startIP == nil || endIP == nil {
		return false
	}
	return ipInRange(ip, startIP, endIP)
}

// ValidateScopeOptions validates scope creation/update parameters.
//
// Exported so that other write paths (notably configuration publishing in
// internal/configver) enforce exactly the same rules as CreateScope and
// UpdateScope. A second, slightly different copy of these rules is how a
// configuration passes validation at publish time and is then rejected -- or
// worse, silently accepted -- by the storage layer.
//
// An options value with every field empty is treated as "nothing to validate"
// and passes; callers that require a complete scope must check the required
// fields themselves.
func ValidateScopeOptions(opts ScopeOptions) error {
	if opts.Name == "" && opts.Subnet == "" && opts.StartIP == "" && opts.EndIP == "" {
		// Update with no fields to validate - skip
		return nil
	}

	// Validate numeric fields.
	if opts.LeaseTime != nil && *opts.LeaseTime < 0 {
		return fmt.Errorf("lease_time must be non-negative, got %d", *opts.LeaseTime)
	}
	if opts.MaxLeaseTime != nil && *opts.MaxLeaseTime < 0 {
		return fmt.Errorf("max_lease_time must be non-negative, got %d", *opts.MaxLeaseTime)
	}
	if opts.LeaseTime != nil && opts.MaxLeaseTime != nil && *opts.MaxLeaseTime > 0 && *opts.LeaseTime > *opts.MaxLeaseTime {
		return fmt.Errorf("lease_time (%d) cannot exceed max_lease_time (%d)", *opts.LeaseTime, *opts.MaxLeaseTime)
	}

	startIP := net.ParseIP(opts.StartIP)
	endIP := net.ParseIP(opts.EndIP)
	if startIP == nil {
		return fmt.Errorf("invalid start IP: %s", opts.StartIP)
	}
	if endIP == nil {
		return fmt.Errorf("invalid end IP: %s", opts.EndIP)
	}

	// start_ip must be less than or equal to end_ip. Equality is permitted
	// to support /32 (and /128) subnets where a single host has only one
	// usable address.
	for i := range startIP {
		if startIP[i] < endIP[i] {
			break
		}
		if startIP[i] > endIP[i] {
			return fmt.Errorf("start_ip (%s) must be less than or equal to end_ip (%s)", opts.StartIP, opts.EndIP)
		}
	}

	// Validate subnet is valid CIDR
	if opts.Subnet != "" {
		_, ipNet, err := net.ParseCIDR(opts.Subnet)
		if err != nil {
			return fmt.Errorf("invalid subnet CIDR: %s", opts.Subnet)
		}
		// Check IP range is within subnet
		if !ipNet.Contains(startIP) {
			return fmt.Errorf("start_ip (%s) is not within subnet (%s)", opts.StartIP, opts.Subnet)
		}
		if !ipNet.Contains(endIP) {
			return fmt.Errorf("end_ip (%s) is not within subnet (%s)", opts.EndIP, opts.Subnet)
		}
	}

	return nil
}

// ipInRange checks if an IP is within the range [start, end].
func ipInRange(ip, start, end net.IP) bool {
	return bytesCompare(ip, start) >= 0 && bytesCompare(ip, end) <= 0
}

// bytesCompare compares two IP addresses. The comparison is performed
// lexicographically on the underlying byte slice, which gives the correct
// ordering for both IPv4 and IPv6 addresses as long as the inputs are in the
// same address family (4-byte or 16-byte). It returns 0 when the two
// addresses cannot be compared (different lengths).
func bytesCompare(a, b net.IP) int {
	if len(a) != len(b) {
		// Cannot compare across families / different representations.
		return 0
	}
	// Prefer net.IP.Equal for the equality short-circuit to also collapse
	// representations like 4in6-mapped IPv4 addresses.
	if a.Equal(b) {
		return 0
	}
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
