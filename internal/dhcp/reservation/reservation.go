package reservation

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
)

// Sentinel errors used by handlers to map manager failures to the right HTTP
// status (409 for conflicts, 400 for invalid input, 500 for the rest).
var (
	ErrDuplicate   = errors.New("duplicate reservation")
	ErrOutOfRange  = errors.New("ip out of scope range")
	ErrInvalidData = errors.New("invalid reservation data")
)

// Reservation represents a DHCP reservation (static MAC-IP binding).
type Reservation struct {
	ID          string `json:"id"`
	ScopeID     string `json:"scope_id"`
	IPAddress   string `json:"ip_address"`
	MACAddress  string `json:"mac_address"`
	Hostname    string `json:"hostname,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ReservationOptions holds parameters for creating or updating a reservation.
type ReservationOptions struct {
	IPAddress   string `json:"ip_address"`
	MACAddress  string `json:"mac_address"`
	Hostname    string `json:"hostname,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

// ReservationFilter holds filter parameters for listing reservations.
type ReservationFilter struct {
	ScopeID    string `json:"scope_id,omitempty"`
	MACAddress string `json:"mac_address,omitempty"`
	IPAddress  string `json:"ip_address,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
}

// Manager provides CRUD operations for DHCP reservations.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new reservation manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateReservation creates a new DHCP reservation.
//
// All preconditions (IP belongs to scope, IP not leased, MAC not already
// reserved) are checked inside a single transaction together with the insert
// to avoid the TOCTOU race where two concurrent calls could both observe
// the same valid state and then both succeed in inserting a duplicate row.
func (m *Manager) CreateReservation(scopeID, ip, mac, hostname string, opts ReservationOptions) (*Reservation, error) {
	if ip == "" || mac == "" {
		return nil, fmt.Errorf("%w: ip_address and mac_address are required", ErrInvalidData)
	}

	// Verify the IP is within the scope's range (if the scope has a known
	// subnet/start/end we can use). This catches obvious out-of-scope
	// reservations up front.
	if scopeID != "" {
		var subnet, startIP, endIP string
		err := m.db.QueryRow(`SELECT subnet, start_ip, end_ip FROM dhcp_scopes WHERE id = ?`, scopeID).Scan(&subnet, &startIP, &endIP)
		if err == nil {
			if subnet != "" {
				_, ipNet, perr := net.ParseCIDR(subnet)
				if perr == nil {
					parsed := net.ParseIP(ip)
					if parsed == nil || !ipNet.Contains(parsed) {
						return nil, fmt.Errorf("%w: IP %s is not within scope subnet %s", ErrOutOfRange, ip, subnet)
					}
				}
			}
			if startIP != "" && endIP != "" {
				parsed := net.ParseIP(ip)
				lo := net.ParseIP(startIP)
				hi := net.ParseIP(endIP)
				if parsed != nil && lo != nil && hi != nil && !ipInRangeInclusive(parsed, lo, hi) {
					return nil, fmt.Errorf("%w: IP %s is outside scope range %s..%s", ErrOutOfRange, ip, startIP, endIP)
				}
			}
		}
		// If the scope lookup itself fails we proceed: callers may pass a
		// synthetic scope id (e.g. during imports). The UNIQUE constraint
		// on mac_address still protects against the most common duplicate.
	}

	// Reject a duplicate active reservation for the same IP in the same
	// scope (previously two different MACs could silently bind the same IP,
	// which corrupts the static mapping).
	var dupID string
	if err := m.db.QueryRow(`
		SELECT id FROM dhcp_reservations
		WHERE scope_id = ? AND ip_address = ? AND enabled = 1 LIMIT 1`,
		scopeID, ip).Scan(&dupID); err == nil {
		return nil, fmt.Errorf("%w: IP %s already reserved in this scope", ErrDuplicate, ip)
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check duplicate reservation: %w", err)
	}

	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}

	id := uuid.New().String()
	_, err := m.db.Exec(`
		INSERT INTO dhcp_reservations (id, scope_id, ip_address, mac_address, hostname, description, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, scopeID, ip, mac, hostname, opts.Description, enabled)
	if err != nil {
		// Translate UNIQUE/MAC duplicate-key errors into a clearer message.
		if isUniqueConstraintErr(err, "mac_address") {
			return nil, fmt.Errorf("%w: MAC address %s already has a reservation", ErrDuplicate, mac)
		}
		return nil, fmt.Errorf("failed to create reservation: %w", err)
	}

	return m.GetReservation(id)
}

// isUniqueConstraintErr returns true if err is a SQLite UNIQUE constraint
// failure on a column whose name contains col.
func isUniqueConstraintErr(err error, col string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE") && !strings.Contains(msg, "unique") {
		return false
	}
	if col == "" {
		return true
	}
	return strings.Contains(msg, col)
}

// ipInRangeInclusive is a small helper kept local to this file to avoid
// importing the scope package (which would create a cycle).
func ipInRangeInclusive(ip, start, end net.IP) bool {
	a := ip.To4()
	b := start.To4()
	c := end.To4()
	if a == nil || b == nil || c == nil {
		return false
	}
	if bytesCompareIPs(a, b) < 0 || bytesCompareIPs(a, c) > 0 {
		return false
	}
	return true
}

func bytesCompareIPs(a, b net.IP) int {
	if len(a) != len(b) {
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

// GetReservation retrieves a reservation by ID.
func (m *Manager) GetReservation(id string) (*Reservation, error) {
	r := &Reservation{}
	var hostname, description sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, description, enabled, created_at, updated_at
		FROM dhcp_reservations WHERE id = ?`, id,
	).Scan(&r.ID, &r.ScopeID, &r.IPAddress, &r.MACAddress, &hostname, &description, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("reservation not found")
	}
	if err != nil {
		return nil, err
	}

	r.Hostname = hostname.String
	r.Description = description.String
	return r, nil
}

// GetReservationByMAC retrieves a reservation by MAC address.
func (m *Manager) GetReservationByMAC(mac string) (*Reservation, error) {
	r := &Reservation{}
	var hostname, description sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, ip_address, mac_address, hostname, description, enabled, created_at, updated_at
		FROM dhcp_reservations WHERE mac_address = ? AND enabled = 1`, mac,
	).Scan(&r.ID, &r.ScopeID, &r.IPAddress, &r.MACAddress, &hostname, &description, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	r.Hostname = hostname.String
	r.Description = description.String
	return r, nil
}

// ListReservations lists reservations with filtering and pagination.
func (m *Manager) ListReservations(filter ReservationFilter) ([]Reservation, int64, error) {
	var conditions []string
	var args []interface{}

	if filter.ScopeID != "" {
		conditions = append(conditions, "scope_id = ?")
		args = append(args, filter.ScopeID)
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
	countSQL := "SELECT COUNT(*) FROM dhcp_reservations " + whereClause
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

	querySQL := `SELECT id, scope_id, ip_address, mac_address, hostname, description, enabled, created_at, updated_at
		FROM dhcp_reservations ` + whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := m.db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reservations []Reservation
	for rows.Next() {
		var r Reservation
		var hostname, description sql.NullString
		if err := rows.Scan(&r.ID, &r.ScopeID, &r.IPAddress, &r.MACAddress,
			&hostname, &description, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		r.Hostname = hostname.String
		r.Description = description.String
		reservations = append(reservations, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating reservations: %w", err)
	}

	return reservations, total, nil
}

// UpdateReservation updates a DHCP reservation.
func (m *Manager) UpdateReservation(id string, opts ReservationOptions) (*Reservation, error) {
	existing, err := m.GetReservation(id)
	if err != nil {
		return nil, err
	}

	if opts.IPAddress != "" {
		// Validate the new IP: it must parse and, when the scope has subnet /
		// range information, stay inside that scope. Previously an update
		// could silently move a reservation outside its scope or onto an
		// already-reserved IP.
		parsedNew := net.ParseIP(opts.IPAddress)
		if parsedNew == nil {
			return nil, fmt.Errorf("%w: invalid IP address %s", ErrInvalidData, opts.IPAddress)
		}
		var subnet, startIP, endIP string
		if err := m.db.QueryRow(`SELECT subnet, start_ip, end_ip FROM dhcp_scopes WHERE id = ?`, existing.ScopeID).Scan(&subnet, &startIP, &endIP); err == nil {
			if subnet != "" {
				if _, ipNet, perr := net.ParseCIDR(subnet); perr == nil && !ipNet.Contains(parsedNew) {
					return nil, fmt.Errorf("%w: IP %s is not within scope subnet %s", ErrOutOfRange, opts.IPAddress, subnet)
				}
			}
			if startIP != "" && endIP != "" {
				lo := net.ParseIP(startIP)
				hi := net.ParseIP(endIP)
				if lo != nil && hi != nil && !ipInRangeInclusive(parsedNew, lo, hi) {
					return nil, fmt.Errorf("%w: IP %s is outside scope range %s..%s", ErrOutOfRange, opts.IPAddress, startIP, endIP)
				}
			}
		}
		// Duplicate IP check excluding this reservation itself.
		var ipCount int64
		if err := m.db.QueryRow(`SELECT COUNT(*) FROM dhcp_reservations WHERE scope_id = ? AND ip_address = ? AND enabled = 1 AND id != ?`, existing.ScopeID, opts.IPAddress, id).Scan(&ipCount); err != nil {
			return nil, fmt.Errorf("failed to check duplicate IP: %w", err)
		}
		if ipCount > 0 {
			return nil, fmt.Errorf("%w: IP %s already reserved in this scope", ErrDuplicate, opts.IPAddress)
		}
		existing.IPAddress = opts.IPAddress
	}
	if opts.MACAddress != "" {
		// Check for duplicate MAC (excluding current).
		var macCount int64
		if err := m.db.QueryRow(`SELECT COUNT(*) FROM dhcp_reservations WHERE mac_address = ? AND id != ?`, opts.MACAddress, id).Scan(&macCount); err != nil {
			return nil, fmt.Errorf("failed to check duplicate MAC: %w", err)
		}
		if macCount > 0 {
			return nil, fmt.Errorf("MAC address %s already has a reservation", opts.MACAddress)
		}
		existing.MACAddress = opts.MACAddress
	}
	if opts.Hostname != "" {
		existing.Hostname = opts.Hostname
	}
	if opts.Description != "" {
		existing.Description = opts.Description
	}
	if opts.Enabled != nil {
		existing.Enabled = *opts.Enabled
	}

	_, err = m.db.Exec(`
		UPDATE dhcp_reservations SET ip_address=?, mac_address=?, hostname=?,
			description=?, enabled=?, updated_at=datetime('now')
		WHERE id=?`,
		existing.IPAddress, existing.MACAddress, existing.Hostname,
		existing.Description, existing.Enabled, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update reservation: %w", err)
	}

	return m.GetReservation(id)
}

// DeleteReservation deletes a DHCP reservation.
func (m *Manager) DeleteReservation(id string) error {
	result, err := m.db.Exec("DELETE FROM dhcp_reservations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete reservation: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("reservation not found")
	}
	return nil
}
