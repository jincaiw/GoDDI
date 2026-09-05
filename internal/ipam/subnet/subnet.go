package subnet

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/dhcp/scope"
)

// Subnet represents an IPAM subnet.
type Subnet struct {
	ID          string `json:"id"`
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	CIDR        string `json:"cidr"`
	VLANID      int    `json:"vlan_id,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// SubnetOptions holds parameters for creating or updating a subnet.
type SubnetOptions struct {
	Name        string `json:"name"`
	CIDR        string `json:"cidr"`
	VLANID      *int   `json:"vlan_id,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
}

// SubnetFilter holds filter parameters for listing subnets.
type SubnetFilter struct {
	SpaceID  string `json:"space_id,omitempty"`
	Name     string `json:"name,omitempty"`
	CIDR     string `json:"cidr,omitempty"`
	VLANID   *int   `json:"vlan_id,omitempty"`
	Location string `json:"location,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// UsageStats represents IP usage statistics for a subnet.
type UsageStats struct {
	Total     int64 `json:"total"`
	Available int64 `json:"available"`
	Used      int64 `json:"used"`
	Reserved  int64 `json:"reserved"`
	DHCP      int64 `json:"dhcp"`
	Static    int64 `json:"static"`
	Gateway   int64 `json:"gateway"`
	Excluded  int64 `json:"excluded"`
	Conflict  int64 `json:"conflict"`
	Unknown   int64 `json:"unknown"`
}

// Manager provides CRUD operations for IPAM subnets.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new subnet manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateSubnet creates a new IPAM subnet and auto-creates IP address entries.
func (m *Manager) CreateSubnet(spaceID, name, cidr string, opts SubnetOptions) (*Subnet, error) {
	if name == "" || cidr == "" {
		return nil, fmt.Errorf("name and cidr are required")
	}

	// Validate CIDR.
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	// Check for overlap with existing subnets.
	if err := m.checkOverlap(cidr, ""); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	var vlanID interface{}
	if opts.VLANID != nil {
		vlanID = *opts.VLANID
	}

	_, err = m.db.Exec(`
		INSERT INTO ipam_subnets (id, space_id, name, cidr, vlan_id, location, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, spaceID, name, cidr, vlanID, opts.Location, opts.Description)
	if err != nil {
		return nil, fmt.Errorf("failed to create subnet: %w", err)
	}

	// Auto-create IP address entries for all IPs in subnet (synchronous).
	m.autoCreateAddresses(id, ipNet)

	return m.GetSubnet(id)
}

// GetSubnet retrieves a subnet by ID.
func (m *Manager) GetSubnet(id string) (*Subnet, error) {
	s := &Subnet{}
	var vlanID sql.NullInt64
	var location, description sql.NullString

	err := m.db.QueryRow(`
		SELECT id, space_id, name, cidr, vlan_id, location, description, created_at, updated_at
		FROM ipam_subnets WHERE id = ?`, id,
	).Scan(&s.ID, &s.SpaceID, &s.Name, &s.CIDR, &vlanID, &location, &description, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subnet not found")
	}
	if err != nil {
		return nil, err
	}

	if vlanID.Valid {
		s.VLANID = int(vlanID.Int64)
	}
	s.Location = location.String
	s.Description = description.String
	return s, nil
}

// ListSubnets lists subnets with filtering and pagination.
func (m *Manager) ListSubnets(filter SubnetFilter) ([]Subnet, int64, error) {
	var conditions []string
	var args []interface{}

	if filter.SpaceID != "" {
		conditions = append(conditions, "space_id = ?")
		args = append(args, filter.SpaceID)
	}
	if filter.Name != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}
	if filter.CIDR != "" {
		conditions = append(conditions, "cidr = ?")
		args = append(args, filter.CIDR)
	}
	if filter.VLANID != nil {
		conditions = append(conditions, "vlan_id = ?")
		args = append(args, *filter.VLANID)
	}
	if filter.Location != "" {
		conditions = append(conditions, "location LIKE ?")
		args = append(args, "%"+filter.Location+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM ipam_subnets " + whereClause
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

	querySQL := `SELECT id, space_id, name, cidr, vlan_id, location, description, created_at, updated_at
		FROM ipam_subnets ` + whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := m.db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var subnets []Subnet
	for rows.Next() {
		var s Subnet
		var vlanID sql.NullInt64
		var location, description sql.NullString
		if err := rows.Scan(&s.ID, &s.SpaceID, &s.Name, &s.CIDR, &vlanID, &location, &description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if vlanID.Valid {
			s.VLANID = int(vlanID.Int64)
		}
		s.Location = location.String
		s.Description = description.String
		subnets = append(subnets, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating subnets: %w", err)
	}

	return subnets, total, nil
}

// UpdateSubnet updates a subnet.
func (m *Manager) UpdateSubnet(id string, opts SubnetOptions) (*Subnet, error) {
	existing, err := m.GetSubnet(id)
	if err != nil {
		return nil, err
	}

	if opts.Name != "" {
		existing.Name = opts.Name
	}
	if opts.CIDR != "" {
		if _, _, err := net.ParseCIDR(opts.CIDR); err != nil {
			return nil, fmt.Errorf("invalid CIDR: %w", err)
		}
		if err := m.checkOverlap(opts.CIDR, id); err != nil {
			return nil, err
		}
		existing.CIDR = opts.CIDR
	}
	if opts.VLANID != nil {
		existing.VLANID = *opts.VLANID
	}
	if opts.Location != "" {
		existing.Location = opts.Location
	}
	if opts.Description != "" {
		existing.Description = opts.Description
	}

	var vlanID interface{}
	if existing.VLANID != 0 {
		vlanID = existing.VLANID
	}

	_, err = m.db.Exec(`
		UPDATE ipam_subnets SET name=?, cidr=?, vlan_id=?, location=?, description=?, updated_at=datetime('now')
		WHERE id=?`, existing.Name, existing.CIDR, vlanID, existing.Location, existing.Description, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update subnet: %w", err)
	}

	return m.GetSubnet(id)
}

// DeleteSubnet deletes a subnet. Checks for DHCP/DNS dependencies.
func (m *Manager) DeleteSubnet(id string) error {
	// Use a transaction to ensure atomic deletion of all related records.
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get the subnet's CIDR first.
	var cidr string
	if err := tx.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", id).Scan(&cidr); err != nil {
		return fmt.Errorf("subnet not found")
	}

	// Check for DHCP scope dependency using the actual CIDR.
	var scopeCount int64
	if err := tx.QueryRow("SELECT COUNT(*) FROM dhcp_scopes WHERE subnet = ?", cidr).Scan(&scopeCount); err != nil {
		return fmt.Errorf("failed to check DHCP scope dependencies: %w", err)
	}
	if scopeCount > 0 {
		return fmt.Errorf("cannot delete subnet with DHCP scope dependencies")
	}

	// Delete addresses first (cascade should handle this, but be explicit).
	if _, err := tx.Exec("DELETE FROM ipam_addresses WHERE subnet_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete addresses: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM ipam_history WHERE subnet_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete history: %w", err)
	}

	result, err := tx.Exec("DELETE FROM ipam_subnets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete subnet: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("subnet not found")
	}
	return tx.Commit()
}

// GetUsageStats returns IP usage statistics for a subnet.
func (m *Manager) GetUsageStats(subnetID string) (*UsageStats, error) {
	stats := &UsageStats{}

	// Get total from CIDR.
	var cidr string
	err := m.db.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", subnetID).Scan(&cidr)
	if err != nil {
		return nil, fmt.Errorf("subnet not found")
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR")
	}

	ones, bits := ipNet.Mask.Size()
	// Compute total = 2^(bits-ones) without overflowing int64. We clamp the
	// shift to 62 to avoid shifting into the sign bit and treat anything
	// beyond that as "huge but unknown" (capped at a very large but
	// representable value).
	hostBits := bits - ones
	switch {
	case hostBits >= 63:
		stats.Total = 1 << 62 // cap at a very large but representable value
	case hostBits < 0:
		stats.Total = 0
	default:
		stats.Total = int64(1) << uint(hostBits)
	}
	// Subtract network and broadcast for IPv4.
	if bits == 32 {
		stats.Total -= 2
	}
	if stats.Total < 0 {
		stats.Total = 0
	}

	// Count by status.
	rows, err := m.db.Query(`
		SELECT status, COUNT(*) FROM ipam_addresses
		WHERE subnet_id = ? GROUP BY status`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int64
		if rows.Scan(&status, &count) != nil {
			continue
		}
		switch status {
		case "available":
			stats.Available = count
		case "used":
			stats.Used = count
		case "reserved":
			stats.Reserved = count
		case "dhcp":
			stats.DHCP = count
		case "static":
			stats.Static = count
		case "gateway":
			stats.Gateway = count
		case "excluded":
			stats.Excluded = count
		case "conflict":
			stats.Conflict = count
		case "unknown":
			stats.Unknown = count
		}
	}

	return stats, nil
}

// GenerateDHCPScope generates DHCP scope options from a subnet.
func (m *Manager) GenerateDHCPScope(subnetID string) (*scope.ScopeOptions, error) {
	s, err := m.GetSubnet(subnetID)
	if err != nil {
		return nil, err
	}

	_, ipNet, err := net.ParseCIDR(s.CIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR")
	}

	ones, _ := ipNet.Mask.Size()

	// Calculate IP range based on prefix length.
	startIP := make(net.IP, len(ipNet.IP))
	copy(startIP, ipNet.IP)

	endIP := make(net.IP, len(ipNet.IP))
	copy(endIP, ipNet.IP)

	// Calculate broadcast address.
	mask := ipNet.Mask
	for i := range endIP {
		endIP[i] = ipNet.IP[i] | ^mask[i]
	}

	switch {
	case ones >= 32:
		// /32: single host, start and end are the same.
		// No increment needed.
	case ones == 31:
		// /31: point-to-point link (RFC 3021), both addresses are usable.
		startIP[3]++ // skip network address (first address)
		// endIP is the second address (broadcast is not reserved for /31)
	default:
		// Normal subnet: skip network address and broadcast address.
		startIP[3]++ // skip network address
		// Subtract 1 from broadcast to get last usable address.
		for i := len(endIP) - 1; i >= 0; i-- {
			endIP[i]--
			if endIP[i] != 255 {
				break
			}
		}
	}

	subnetMask := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

	return &scope.ScopeOptions{
		Name:             s.Name,
		Subnet:           s.CIDR,
		StartIP:          startIP.String(),
		EndIP:            endIP.String(),
		SubnetMask:       &subnetMask,
		LeaseTime:        intPtr(86400),
		PingCheckEnabled: boolPtr(true),
		DNSUpdates:       boolPtr(false),
		Enabled:          boolPtr(true),
		Comment:          stringPtr("Generated from IPAM subnet " + s.Name),
	}, nil
}

// GenerateReverseZone generates reverse zone options from a subnet.
func (m *Manager) GenerateReverseZone(subnetID string) (string, error) {
	s, err := m.GetSubnet(subnetID)
	if err != nil {
		return "", err
	}

	_, ipNet, err := net.ParseCIDR(s.CIDR)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR")
	}

	// Generate reverse zone name.
	ones, _ := ipNet.Mask.Size()
	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("only IPv4 supported for reverse zones")
	}

	var zoneName string
	switch {
	case ones >= 24:
		zoneName = fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0])
	case ones >= 16:
		zoneName = fmt.Sprintf("%d.%d.in-addr.arpa", ip[1], ip[0])
	case ones >= 8:
		zoneName = fmt.Sprintf("%d.in-addr.arpa", ip[0])
	default:
		zoneName = "in-addr.arpa"
	}

	return zoneName, nil
}

// checkOverlap checks if a CIDR overlaps with existing subnets.
func (m *Manager) checkOverlap(cidr, excludeID string) error {
	_, newNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	rows, err := m.db.Query("SELECT id, cidr FROM ipam_subnets")
	if err != nil {
		return nil // If we can't check, allow it.
	}
	defer rows.Close()

	for rows.Next() {
		var id, existingCIDR string
		if rows.Scan(&id, &existingCIDR) != nil {
			continue
		}
		if id == excludeID {
			continue
		}

		_, existingNet, err := net.ParseCIDR(existingCIDR)
		if err != nil {
			continue
		}

		if networksOverlap(newNet, existingNet) {
			return fmt.Errorf("CIDR %s overlaps with existing subnet %s (%s)", cidr, existingCIDR, id)
		}
	}

	return nil
}

// networksOverlap returns true when two IP networks share at least one
// address. It compares the start / end of the address ranges produced by
// NetworkRange (the first and last IP in each network) so that it works
// correctly for subnets that do not begin on a byte boundary (e.g. 10.0.0.5/24).
func networksOverlap(a, b *net.IPNet) bool {
	aStart, aEnd := networkRange(a)
	bStart, bEnd := networkRange(b)
	// Two closed intervals [aStart, aEnd] and [bStart, bEnd] overlap iff
	// aStart <= bEnd && bStart <= aEnd.
	return bytesCompare(aStart, bEnd) <= 0 && bytesCompare(bStart, aEnd) <= 0
}

// networkRange returns the first and last IP of an IPNet (inclusive).
// Returns (nil, nil) if the network cannot be processed.
func networkRange(n *net.IPNet) (net.IP, net.IP) {
	if n == nil {
		return nil, nil
	}
	ipLen := len(n.IP)
	if ipLen == 0 {
		return nil, nil
	}
	start := make(net.IP, ipLen)
	copy(start, n.IP)
	end := make(net.IP, ipLen)
	copy(end, n.IP)
	for i := 0; i < ipLen; i++ {
		end[i] = start[i] | ^n.Mask[i]
	}
	return start, end
}

// bytesCompare compares two IP addresses. It returns 0 for nil arguments or
// for addresses of different lengths, -1 if a<b, +1 if a>b.
func bytesCompare(a, b net.IP) int {
	if a == nil || b == nil || len(a) != len(b) {
		return 0
	}
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

// autoCreateAddresses creates IP address entries for all IPs in a subnet.
// For large subnets (> /16, more than 65536 addresses) the function does
// nothing: pre-allocating one row per IP is too expensive in both time and
// storage. For smaller subnets the work is performed in batches of
// autoCreateBatchSize rows inside a single transaction to keep the
// per-insert cost low. The function is safe to call after the subnet row
// has been committed.
func (m *Manager) autoCreateAddresses(subnetID string, ipNet *net.IPNet) {
	ones, bits := ipNet.Mask.Size()
	// Only auto-create for subnets /16 or smaller (max 65536 IPs).
	// Larger subnets (e.g. /8) would generate an unreasonable number of
	// rows; defer allocation until the addresses are actually used.
	if bits-ones > 16 {
		return
	}

	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)

	// Skip network address.
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}

	// Calculate broadcast address once so we can stop at it.
	broadcast := make(net.IP, len(ipNet.IP))
	copy(broadcast, ipNet.IP)
	for i := range broadcast {
		broadcast[i] |= ^ipNet.Mask[i]
	}

	// Build batches of addresses and flush them transactionally.
	const autoCreateBatchSize = 500
	batch := make([]string, 0, autoCreateBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := m.insertAddressBatch(subnetID, batch); err != nil {
			slog.Warn("ipam: autoCreateAddresses batch insert failed",
				"subnet_id", subnetID, "batch_size", len(batch), "error", err)
		}
		batch = batch[:0]
	}

	for {
		if !ipNet.Contains(ip) {
			break
		}
		// Never create an address row for the broadcast address: appending
		// before the check made it allocatable via AllocateIP.
		if ip.Equal(broadcast) {
			break
		}
		batch = append(batch, ip.String())
		if len(batch) >= autoCreateBatchSize {
			flush()
		}
		// Increment IP.
		for i := len(ip) - 1; i >= 0; i-- {
			ip[i]++
			if ip[i] != 0 {
				break
			}
		}
	}
	flush()
}

// insertAddressBatch inserts a batch of IP address rows in a single
// transaction. Existing rows (ip_address, subnet_id) are left untouched via
// INSERT OR IGNORE so the operation is idempotent.
func (m *Manager) insertAddressBatch(subnetID string, ips []string) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO ipam_addresses (id, subnet_id, ip_address, status, created_at, updated_at)
		VALUES (?, ?, ?, 'available', datetime('now'), datetime('now'))`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, ip := range ips {
		if _, err := stmt.Exec(uuid.New().String(), subnetID, ip); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func intPtr(v int) *int          { return &v }
func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }
