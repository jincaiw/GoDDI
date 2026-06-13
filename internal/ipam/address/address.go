package address

import (
	"database/sql"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
)

// IPAddressStatus represents the status of an IP address.
type IPAddressStatus string

const (
	StatusAvailable IPAddressStatus = "available"
	StatusUsed      IPAddressStatus = "used"
	StatusReserved  IPAddressStatus = "reserved"
	StatusDHCP      IPAddressStatus = "dhcp"
	StatusStatic    IPAddressStatus = "static"
	StatusGateway   IPAddressStatus = "gateway"
	StatusExcluded  IPAddressStatus = "excluded"
	StatusConflict  IPAddressStatus = "conflict"
	StatusUnknown   IPAddressStatus = "unknown"
)

// Address represents an IPAM address.
type Address struct {
	ID          string          `json:"id"`
	SubnetID    string          `json:"subnet_id"`
	IPAddress   string          `json:"ip_address"`
	Status      IPAddressStatus `json:"status"`
	MACAddress  string          `json:"mac_address,omitempty"`
	Hostname    string          `json:"hostname,omitempty"`
	DNSRecordID string          `json:"dns_record_id,omitempty"`
	DHCPLeaseID string          `json:"dhcp_lease_id,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Device      string          `json:"device,omitempty"`
	Location    string          `json:"location,omitempty"`
	Description string          `json:"description,omitempty"`
	LastSeen    string          `json:"last_seen,omitempty"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// AddressOptions holds parameters for updating an address.
type AddressOptions struct {
	Status      *IPAddressStatus `json:"status,omitempty"`
	MACAddress  string           `json:"mac_address,omitempty"`
	Hostname    string           `json:"hostname,omitempty"`
	Owner       string           `json:"owner,omitempty"`
	Device      string           `json:"device,omitempty"`
	Location    string           `json:"location,omitempty"`
	Description string           `json:"description,omitempty"`
}

// AddressFilter holds filter parameters for listing addresses.
type AddressFilter struct {
	SubnetID   string          `json:"subnet_id,omitempty"`
	Status     IPAddressStatus `json:"status,omitempty"`
	IPAddress  string          `json:"ip_address,omitempty"`
	MACAddress string          `json:"mac_address,omitempty"`
	Hostname   string          `json:"hostname,omitempty"`
	Owner      string          `json:"owner,omitempty"`
	Page       int             `json:"page,omitempty"`
	PageSize   int             `json:"page_size,omitempty"`
}

// Manager provides CRUD operations for IPAM addresses.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new address manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// GetAddress retrieves an address by ID.
func (m *Manager) GetAddress(id string) (*Address, error) {
	a := &Address{}
	var macAddress, hostname, dnsRecordID, dhcpLeaseID, owner, device, location, description, lastSeen sql.NullString

	err := m.db.QueryRow(`
		SELECT id, subnet_id, ip_address, status, mac_address, hostname, dns_record_id,
			dhcp_lease_id, owner, device, location, description, last_seen, created_at, updated_at
		FROM ipam_addresses WHERE id = ?`, id,
	).Scan(&a.ID, &a.SubnetID, &a.IPAddress, &a.Status, &macAddress, &hostname, &dnsRecordID,
		&dhcpLeaseID, &owner, &device, &location, &description, &lastSeen, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("address not found")
	}
	if err != nil {
		return nil, err
	}

	a.MACAddress = macAddress.String
	a.Hostname = hostname.String
	a.DNSRecordID = dnsRecordID.String
	a.DHCPLeaseID = dhcpLeaseID.String
	a.Owner = owner.String
	a.Device = device.String
	a.Location = location.String
	a.Description = description.String
	a.LastSeen = lastSeen.String
	return a, nil
}

// GetAddressByIP retrieves an address by subnet ID and IP.
func (m *Manager) GetAddressByIP(subnetID, ip string) (*Address, error) {
	a := &Address{}
	var macAddress, hostname, dnsRecordID, dhcpLeaseID, owner, device, location, description, lastSeen sql.NullString

	err := m.db.QueryRow(`
		SELECT id, subnet_id, ip_address, status, mac_address, hostname, dns_record_id,
			dhcp_lease_id, owner, device, location, description, last_seen, created_at, updated_at
		FROM ipam_addresses WHERE subnet_id = ? AND ip_address = ?`, subnetID, ip,
	).Scan(&a.ID, &a.SubnetID, &a.IPAddress, &a.Status, &macAddress, &hostname, &dnsRecordID,
		&dhcpLeaseID, &owner, &device, &location, &description, &lastSeen, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	a.MACAddress = macAddress.String
	a.Hostname = hostname.String
	a.DNSRecordID = dnsRecordID.String
	a.DHCPLeaseID = dhcpLeaseID.String
	a.Owner = owner.String
	a.Device = device.String
	a.Location = location.String
	a.Description = description.String
	a.LastSeen = lastSeen.String
	return a, nil
}

// ListAddresses lists addresses with filtering and pagination.
func (m *Manager) ListAddresses(filter AddressFilter) ([]Address, int64, error) {
	var conditions []string
	var args []interface{}

	if filter.SubnetID != "" {
		conditions = append(conditions, "subnet_id = ?")
		args = append(args, filter.SubnetID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.IPAddress != "" {
		conditions = append(conditions, "ip_address = ?")
		args = append(args, filter.IPAddress)
	}
	if filter.MACAddress != "" {
		conditions = append(conditions, "mac_address = ?")
		args = append(args, filter.MACAddress)
	}
	if filter.Hostname != "" {
		conditions = append(conditions, "hostname LIKE ?")
		args = append(args, "%"+filter.Hostname+"%")
	}
	if filter.Owner != "" {
		conditions = append(conditions, "owner = ?")
		args = append(args, filter.Owner)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM ipam_addresses " + whereClause
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

	querySQL := `SELECT id, subnet_id, ip_address, status, mac_address, hostname, dns_record_id,
		dhcp_lease_id, owner, device, location, description, last_seen, created_at, updated_at
		FROM ipam_addresses ` + whereClause + ` ORDER BY ip_address ASC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := m.db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var addresses []Address
	for rows.Next() {
		var a Address
		var macAddress, hostname, dnsRecordID, dhcpLeaseID, owner, device, location, description, lastSeen sql.NullString
		if err := rows.Scan(&a.ID, &a.SubnetID, &a.IPAddress, &a.Status, &macAddress, &hostname,
			&dnsRecordID, &dhcpLeaseID, &owner, &device, &location, &description, &lastSeen,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		a.MACAddress = macAddress.String
		a.Hostname = hostname.String
		a.DNSRecordID = dnsRecordID.String
		a.DHCPLeaseID = dhcpLeaseID.String
		a.Owner = owner.String
		a.Device = device.String
		a.Location = location.String
		a.Description = description.String
		a.LastSeen = lastSeen.String
		addresses = append(addresses, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating addresses: %w", err)
	}

	return addresses, total, nil
}

// UpdateAddress updates an address.
func (m *Manager) UpdateAddress(id string, opts AddressOptions) (*Address, error) {
	existing, err := m.GetAddress(id)
	if err != nil {
		return nil, err
	}

	if opts.Status != nil {
		existing.Status = *opts.Status
	}
	if opts.MACAddress != "" {
		existing.MACAddress = opts.MACAddress
	}
	if opts.Hostname != "" {
		existing.Hostname = opts.Hostname
	}
	if opts.Owner != "" {
		existing.Owner = opts.Owner
	}
	if opts.Device != "" {
		existing.Device = opts.Device
	}
	if opts.Location != "" {
		existing.Location = opts.Location
	}
	if opts.Description != "" {
		existing.Description = opts.Description
	}

	_, err = m.db.Exec(`
		UPDATE ipam_addresses SET status=?, mac_address=?, hostname=?, owner=?,
			device=?, location=?, description=?, updated_at=datetime('now')
		WHERE id=?`,
		string(existing.Status), nullIfEmpty(existing.MACAddress), nullIfEmpty(existing.Hostname),
		nullIfEmpty(existing.Owner), nullIfEmpty(existing.Device), nullIfEmpty(existing.Location),
		nullIfEmpty(existing.Description), id)
	if err != nil {
		return nil, fmt.Errorf("failed to update address: %w", err)
	}

	return m.GetAddress(id)
}

// AllocateIP manually allocates an IP address.
func (m *Manager) AllocateIP(subnetID, ip, owner string, opts AddressOptions) (*Address, error) {
	// Verify the IP belongs to the subnet's CIDR range.
	var cidr string
	if err := m.db.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", subnetID).Scan(&cidr); err != nil {
		return nil, fmt.Errorf("subnet not found: %w", err)
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet CIDR: %w", err)
	}
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}
	if !ipNet.Contains(parsedIP) {
		return nil, fmt.Errorf("IP %s is not within subnet %s", ip, cidr)
	}

	// Check if address already exists.
	existing, err := m.GetAddressByIP(subnetID, ip)
	if err != nil {
		return nil, err
	}

	status := StatusUsed
	if opts.Status != nil {
		status = *opts.Status
	}

	if existing != nil {
		// Update existing address.
		if existing.Status != StatusAvailable {
			return nil, fmt.Errorf("IP %s is not available (status: %s)", ip, existing.Status)
		}
		_, err = m.db.Exec(`
			UPDATE ipam_addresses SET status=?, mac_address=?, hostname=?, owner=?,
				device=?, location=?, description=?, updated_at=datetime('now')
			WHERE id=?`,
			string(status), nullIfEmpty(opts.MACAddress), nullIfEmpty(opts.Hostname),
			nullIfEmpty(owner), nullIfEmpty(opts.Device), nullIfEmpty(opts.Location),
			nullIfEmpty(opts.Description), existing.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to allocate IP: %w", err)
		}

		// Log history.
		m.logHistory(subnetID, ip, "allocate", string(StatusAvailable), string(status), owner)
		return m.GetAddress(existing.ID)
	}

	// Create new address entry.
	id := uuid.New().String()
	_, err = m.db.Exec(`
		INSERT INTO ipam_addresses (id, subnet_id, ip_address, status, mac_address, hostname,
			owner, device, location, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, subnetID, ip, string(status), nullIfEmpty(opts.MACAddress), nullIfEmpty(opts.Hostname),
		nullIfEmpty(owner), nullIfEmpty(opts.Device), nullIfEmpty(opts.Location),
		nullIfEmpty(opts.Description))
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP: %w", err)
	}

	m.logHistory(subnetID, ip, "allocate", string(StatusAvailable), string(status), owner)
	return m.GetAddress(id)
}

// ReleaseIP releases an IP address back to available.
func (m *Manager) ReleaseIP(id string) error {
	existing, err := m.GetAddress(id)
	if err != nil {
		return err
	}

	oldStatus := string(existing.Status)
	_, err = m.db.Exec(`
		UPDATE ipam_addresses SET status=?, mac_address=NULL, hostname=NULL,
			owner=NULL, device=NULL, updated_at=datetime('now')
		WHERE id=?`, string(StatusAvailable), id)
	if err != nil {
		return fmt.Errorf("failed to release IP: %w", err)
	}

	m.logHistory(existing.SubnetID, existing.IPAddress, "release", oldStatus, string(StatusAvailable), "")
	return nil
}

// logHistory records an IP address status change in the history table.
func (m *Manager) logHistory(subnetID, ip, action, oldStatus, newStatus, changedBy string) {
	id := uuid.New().String()
	_, _ = m.db.Exec(`
		INSERT INTO ipam_history (id, subnet_id, ip_address, action, old_status, new_status, changed_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		id, subnetID, ip, action, oldStatus, newStatus, changedBy)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// AutoAssignIP finds the first available IP in a subnet and returns it.
func (m *Manager) AutoAssignIP(subnetID string) (string, error) {
	var cidr string
	if err := m.db.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", subnetID).Scan(&cidr); err != nil {
		return "", fmt.Errorf("subnet not found: %w", err)
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid subnet CIDR: %w", err)
	}

	// Collect all used/reserved IPs from the subnet.
	rows, err := m.db.Query(
		"SELECT ip_address FROM ipam_addresses WHERE subnet_id = ? AND status != ?",
		subnetID, string(StatusAvailable),
	)
	if err != nil {
		return "", fmt.Errorf("query existing addresses: %w", err)
	}
	defer rows.Close()

	used := make(map[string]bool)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			continue
		}
		used[ip] = true
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating addresses: %w", err)
	}

	// Iterate through the subnet range and find the first unused IP.
	// Skip network address (first) and broadcast address (last).
	networkAddr := ipNet.IP.Mask(ipNet.Mask)
	first := make(net.IP, len(networkAddr))
	copy(first, networkAddr)
	incrementIP(first) // skip network address

	for ip := first; ipNet.Contains(ip); incrementIP(ip) {
		// Check if this is the broadcast address (next IP would be outside the network)
		next := make(net.IP, len(ip))
		copy(next, ip)
		incrementIP(next)
		if !ipNet.Contains(next) {
			break // skip broadcast address
		}
		str := ip.String()
		if !used[str] {
			return str, nil
		}
	}
	return "", fmt.Errorf("no available IP addresses in subnet %s", cidr)
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			return
		}
	}
}
