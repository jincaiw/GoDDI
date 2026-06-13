package option

import (
	"database/sql"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
)

// Option represents a DHCP option.
type Option struct {
	ID            string `json:"id"`
	ScopeID       string `json:"scope_id"`
	ReservationID string `json:"reservation_id,omitempty"`
	Code          int    `json:"code"`
	Value         string `json:"value"`
	Priority      string `json:"priority"` // "reservation" > "client_class" > "scope" > "global"
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// OptionOptions holds parameters for creating or updating a DHCP option.
type OptionOptions struct {
	ScopeID       string `json:"scope_id"`
	ReservationID string `json:"reservation_id,omitempty"`
	Code          int    `json:"code"`
	Value         string `json:"value"`
	Priority      string `json:"priority,omitempty"`
}

// Supported DHCP option codes.
const (
	OptionSubnetMask     = 1
	OptionRouter         = 3
	OptionDNSServers     = 6
	OptionHostName       = 12
	OptionDomainName     = 15
	OptionNTPServers     = 42
	OptionWINSServers    = 44
	OptionLeaseTime      = 51
	OptionServerID       = 54
	OptionRenewalTime    = 58
	OptionRebindingTime  = 59
	OptionTFTPServer     = 66
	OptionBootfileName   = 67
	OptionClientFQDN     = 81
	OptionDomainSearch   = 119
	OptionClasslessRoute = 121
	OptionCAPWAP         = 138
	OptionTFTPAddress    = 150
)

// SupportedOptionCodes contains all supported DHCP option codes.
var SupportedOptionCodes = map[int]string{
	OptionSubnetMask:     "Subnet Mask",
	OptionRouter:         "Router",
	OptionDNSServers:     "DNS Servers",
	OptionHostName:       "Host Name",
	OptionDomainName:     "Domain Name",
	OptionNTPServers:     "NTP Servers",
	OptionWINSServers:    "WINS Servers",
	OptionLeaseTime:      "Lease Time",
	OptionServerID:       "Server ID",
	OptionRenewalTime:    "Renewal Time (T1)",
	OptionRebindingTime:  "Rebinding Time (T2)",
	OptionTFTPServer:     "TFTP Server Name",
	OptionBootfileName:   "Bootfile Name",
	OptionClientFQDN:     "Client FQDN",
	OptionDomainSearch:   "Domain Search List",
	OptionClasslessRoute: "Classless Static Route",
	OptionCAPWAP:         "CAPWAP AC Address",
	OptionTFTPAddress:    "TFTP Server Address",
}

// Manager provides CRUD operations for DHCP options.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new option manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateOption creates a new DHCP option.
func (m *Manager) CreateOption(scopeID string, opts OptionOptions) (*Option, error) {
	if opts.Code == 0 {
		return nil, fmt.Errorf("option code is required")
	}
	if _, ok := SupportedOptionCodes[opts.Code]; !ok {
		return nil, fmt.Errorf("unsupported DHCP option code: %d", opts.Code)
	}
	if opts.Value == "" {
		return nil, fmt.Errorf("option value is required")
	}

	priority := "scope"
	if opts.Priority != "" {
		priority = opts.Priority
	}

	id := uuid.New().String()
	var reservationID interface{}
	if opts.ReservationID != "" {
		reservationID = opts.ReservationID
	}

	_, err := m.db.Exec(`
		INSERT INTO dhcp_options (id, scope_id, reservation_id, code, value, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, scopeID, reservationID, opts.Code, opts.Value, priority)
	if err != nil {
		return nil, fmt.Errorf("failed to create option: %w", err)
	}

	return m.GetOption(id)
}

// GetOption retrieves an option by ID.
func (m *Manager) GetOption(id string) (*Option, error) {
	o := &Option{}
	var reservationID sql.NullString

	err := m.db.QueryRow(`
		SELECT id, scope_id, reservation_id, code, value, priority, created_at, updated_at
		FROM dhcp_options WHERE id = ?`, id,
	).Scan(&o.ID, &o.ScopeID, &reservationID, &o.Code, &o.Value, &o.Priority, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("option not found")
	}
	if err != nil {
		return nil, err
	}

	o.ReservationID = reservationID.String
	return o, nil
}

// ListOptions lists DHCP options, optionally filtered by scope_id.
func (m *Manager) ListOptions(scopeID string) ([]Option, error) {
	var args []interface{}
	query := `SELECT id, scope_id, reservation_id, code, value, priority, created_at, updated_at
		FROM dhcp_options`

	if scopeID != "" {
		query += " WHERE scope_id = ?"
		args = append(args, scopeID)
	}
	query += " ORDER BY priority DESC, code ASC"

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var options []Option
	for rows.Next() {
		var o Option
		var reservationID sql.NullString
		if err := rows.Scan(&o.ID, &o.ScopeID, &reservationID, &o.Code, &o.Value, &o.Priority, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.ReservationID = reservationID.String
		options = append(options, o)
	}

	return options, nil
}

// UpdateOption updates a DHCP option.
func (m *Manager) UpdateOption(id string, opts OptionOptions) (*Option, error) {
	existing, err := m.GetOption(id)
	if err != nil {
		return nil, err
	}

	if opts.Code != 0 {
		if _, ok := SupportedOptionCodes[opts.Code]; !ok {
			return nil, fmt.Errorf("unsupported DHCP option code: %d", opts.Code)
		}
		existing.Code = opts.Code
	}
	if opts.Value != "" {
		existing.Value = opts.Value
	}
	if opts.Priority != "" {
		existing.Priority = opts.Priority
	}
	if opts.ReservationID != "" {
		existing.ReservationID = opts.ReservationID
	}

	var reservationID interface{}
	if existing.ReservationID != "" {
		reservationID = existing.ReservationID
	}

	_, err = m.db.Exec(`
		UPDATE dhcp_options SET code=?, value=?, priority=?, reservation_id=?, updated_at=datetime('now')
		WHERE id=?`, existing.Code, existing.Value, existing.Priority, reservationID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update option: %w", err)
	}

	return m.GetOption(id)
}

// DeleteOption deletes a DHCP option.
func (m *Manager) DeleteOption(id string) error {
	result, err := m.db.Exec("DELETE FROM dhcp_options WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete option: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("option not found")
	}
	return nil
}

// GetOptionsByScope retrieves all options for a scope.
func (m *Manager) GetOptionsByScope(scopeID string) ([]Option, error) {
	return m.ListOptions(scopeID)
}

// GetOptionsByReservation retrieves all options for a reservation.
func (m *Manager) GetOptionsByReservation(reservationID string) ([]Option, error) {
	rows, err := m.db.Query(`
		SELECT id, scope_id, reservation_id, code, value, priority, created_at, updated_at
		FROM dhcp_options WHERE reservation_id = ?
		ORDER BY priority DESC, code ASC`, reservationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var options []Option
	for rows.Next() {
		var o Option
		var resID sql.NullString
		if err := rows.Scan(&o.ID, &o.ScopeID, &resID, &o.Code, &o.Value, &o.Priority, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.ReservationID = resID.String
		options = append(options, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating DHCP options: %w", err)
	}

	return options, nil
}

// GetGlobalOptions retrieves all global options (no scope or reservation).
func (m *Manager) GetGlobalOptions() ([]Option, error) {
	rows, err := m.db.Query(`
		SELECT id, scope_id, reservation_id, code, value, priority, created_at, updated_at
		FROM dhcp_options WHERE priority = 'global'
		ORDER BY code ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var options []Option
	for rows.Next() {
		var o Option
		var reservationID sql.NullString
		if err := rows.Scan(&o.ID, &o.ScopeID, &reservationID, &o.Code, &o.Value, &o.Priority, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.ReservationID = reservationID.String
		options = append(options, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating DHCP options: %w", err)
	}

	return options, nil
}

// BuildOptionMapForScope builds a priority-ordered map of options for a scope.
// Priority: reservation > client_class > scope > global
//
// Errors from the underlying lookups are returned to the caller instead of
// being silently swallowed. Callers can decide whether to fall back to a
// partial map (the legacy behaviour) or to abort option construction
// entirely when the database is unreachable.
func (m *Manager) BuildOptionMapForScope(scopeID, reservationID string) (map[int]string, error) {
	optionMap := make(map[int]string)

	// Start with global options (lowest priority).
	globalOpts, err := m.GetGlobalOptions()
	if err != nil {
		return nil, fmt.Errorf("build option map: load global options: %w", err)
	}
	for _, o := range globalOpts {
		optionMap[o.Code] = o.Value
	}

	// Apply scope options (overrides global).
	scopeOpts, err := m.GetOptionsByScope(scopeID)
	if err != nil {
		return nil, fmt.Errorf("build option map: load scope options: %w", err)
	}
	for _, o := range scopeOpts {
		if o.ReservationID == "" {
			if o.Priority == "scope" || o.Priority == "client_class" {
				optionMap[o.Code] = o.Value
			}
		}
	}

	// Apply reservation options (highest priority).
	if reservationID != "" {
		resOpts, err := m.GetOptionsByReservation(reservationID)
		if err != nil {
			return nil, fmt.Errorf("build option map: load reservation options: %w", err)
		}
		for _, o := range resOpts {
			optionMap[o.Code] = o.Value
		}
	}

	return optionMap, nil
}

// ListAllOptionCodes returns all supported DHCP option codes.
func ListAllOptionCodes() map[int]string {
	return SupportedOptionCodes
}

// FormatOptionValue formats an option value based on its code for display.
func FormatOptionValue(code int, value string) string {
	name, ok := SupportedOptionCodes[code]
	if !ok {
		return fmt.Sprintf("Option %d: %s", code, value)
	}
	return fmt.Sprintf("%s: %s", name, value)
}

// ValidateOptionValue validates an option value for the given code.
func ValidateOptionValue(code int, value string) error {
	if value == "" {
		return fmt.Errorf("option value cannot be empty")
	}

	switch code {
	case OptionSubnetMask, OptionRouter, OptionDNSServers, OptionNTPServers,
		OptionWINSServers, OptionServerID, OptionTFTPAddress, OptionCAPWAP:
		// IP address(es) - comma-separated
		parts := strings.Split(value, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if net := parseIP(p); net == nil {
				return fmt.Errorf("invalid IP address: %s", p)
			}
		}
	case OptionLeaseTime, OptionRenewalTime, OptionRebindingTime:
		// Integer (seconds)
		for _, c := range value {
			if c < '0' || c > '9' {
				return fmt.Errorf("option %d requires a numeric value (seconds)", code)
			}
		}
	case OptionHostName, OptionDomainName, OptionTFTPServer, OptionBootfileName:
		// String value - no special validation
	}
	return nil
}

func parseIP(s string) interface{} {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil
	}
	if ip.To4() == nil {
		return nil
	}
	return s
}
