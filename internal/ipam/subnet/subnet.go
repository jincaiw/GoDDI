package subnet

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/dhcp/scope"
	"github.com/jasonwa/goddi/internal/ipam/address"
)

// ErrSubnetNotFound means the subnet does not exist.
//
// It is the address package's sentinel rather than a second one declared here:
// "the subnet named by this request does not exist" is one fact, and two
// sentinels for one fact is two chances for a caller to match only one of them
// and answer 500 where 404 was meant.
var ErrSubnetNotFound = address.ErrSubnetNotFound

// ErrDHCPv6Unsupported means a DHCP scope was requested for an IPv6 subnet.
//
// DHCPv6 is out of scope for this release, so the refusal is a fact about the
// request rather than a failure of the server. It is a sentinel so a handler
// can answer 400 instead of turning "we do not do that" into a 500 that reads
// like a bug.
var ErrDHCPv6Unsupported = errors.New("DHCPv6 is not supported")

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

	// Materialized is false when the subnet is too large to hold one row per
	// address; rows then exist only for addresses something has touched.
	Materialized bool `json:"materialized"`
	// AvailableDerived is true when Available was computed as
	// Total - (everything not available) instead of counted, because the
	// subnet is sparse. An operator comparing the two subsystems needs to know
	// which number came from a count.
	AvailableDerived bool `json:"available_derived"`
}

// Manager provides CRUD operations for IPAM subnets.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new subnet manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateSubnet creates a new IPAM subnet and materialises its address rows
// when the subnet is small enough for that to be reasonable.
func (m *Manager) CreateSubnet(spaceID, name, cidr string, opts SubnetOptions) (*Subnet, error) {
	if name == "" || cidr == "" {
		return nil, fmt.Errorf("name and cidr are required")
	}
	if spaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	canonical, ipNet, err := canonicalCIDR(cidr)
	if err != nil {
		return nil, err
	}

	// Check for overlap with existing subnets.
	if err := m.checkOverlap(canonical, "", spaceID); err != nil {
		return nil, err
	}

	var spaceExists int
	if err := m.db.QueryRow("SELECT COUNT(*) FROM ipam_spaces WHERE id = ?", spaceID).Scan(&spaceExists); err != nil {
		return nil, fmt.Errorf("failed to verify space: %w", err)
	}
	if spaceExists == 0 {
		return nil, fmt.Errorf("space not found: %s", spaceID)
	}

	id := uuid.New().String()
	var vlanID interface{}
	if opts.VLANID != nil {
		vlanID = *opts.VLANID
	}

	_, err = m.db.Exec(`
		INSERT INTO ipam_subnets (id, space_id, name, cidr, vlan_id, location, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, spaceID, name, canonical, vlanID, opts.Location, opts.Description)
	if err != nil {
		return nil, fmt.Errorf("failed to create subnet: %w", err)
	}

	m.autoCreateAddresses(id, spaceID, ipNet)

	return m.GetSubnet(id)
}

// canonicalCIDR validates a CIDR and returns its canonical text.
//
// net.ParseCIDR masks the address it is given, so "10.0.0.5/24" yields the
// network 10.0.0.0/24 -- but the previous code stored the caller's original
// string. A subnet therefore existed under two spellings depending on where it
// was read from, and any equality comparison against it (the DHCP scope
// dependency check, a generated scope's `subnet`) silently failed to match.
func canonicalCIDR(cidr string) (string, *net.IPNet, error) {
	cidr = strings.TrimSpace(cidr)
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", nil, fmt.Errorf("invalid CIDR: %w", err)
	}
	canonical := ipNet.String()
	if ipNet.IP.To4() == nil && ipNet.IP.To16() == nil {
		return "", nil, fmt.Errorf("invalid CIDR: %q is neither IPv4 nor IPv6", cidr)
	}
	return canonical, ipNet, nil
}

// autoCreateMaxAddresses is the largest subnet whose individual addresses are
// materialised as rows. Above it the subnet is represented sparsely: rows are
// created only for addresses something actually touches.
//
// The threshold is on address count, not on prefix length, so it applies the
// same way to IPv4 and IPv6. Applying it by prefix length is how an IPv6 /120
// (256 addresses, trivially materialisable) ends up skipped while an IPv4 /15
// (131072 addresses) is attempted.
const autoCreateMaxAddresses = 1 << 16

// materialize reports whether a subnet's addresses should be pre-created.
func materialize(ipNet *net.IPNet) bool {
	if ipNet == nil {
		return false
	}
	ones, bits := ipNet.Mask.Size()
	hostBits := bits - ones
	if hostBits >= 63 {
		return false
	}
	return int64(1)<<uint(hostBits) <= autoCreateMaxAddresses
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
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, id)
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
	if strings.TrimSpace(opts.CIDR) != "" {
		canonical, ipNet, err := canonicalCIDR(opts.CIDR)
		if err != nil {
			return nil, err
		}
		if canonical != existing.CIDR {
			if err := m.checkOverlap(canonical, id, existing.SpaceID); err != nil {
				return nil, err
			}
			if err := m.resizeAddresses(id, existing.SpaceID, existing.CIDR, canonical, ipNet); err != nil {
				return nil, err
			}
			existing.CIDR = canonical
		}
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

// resizeAddresses reconciles materialised address rows with a new CIDR.
//
// Changing the prefix changes which addresses exist. Re-materialising without
// checking first would delete the row for an address that is currently in use
// -- and with it the record of who holds it -- so a subnet with any allocation
// refuses to shrink or move. A subnet whose rows are all available can be
// rebuilt freely, which is the common case for a subnet whose range was
// entered one octet too wide.
func (m *Manager) resizeAddresses(subnetID, spaceID, oldCIDR, newCIDR string, newNet *net.IPNet) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin resize: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var occupied int64
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = ? AND status <> 'available'",
		subnetID).Scan(&occupied); err != nil {
		return fmt.Errorf("count allocated addresses: %w", err)
	}
	if occupied > 0 {
		return fmt.Errorf(
			"cannot change CIDR from %s to %s: %d address(es) are not available; release them first",
			oldCIDR, newCIDR, occupied)
	}

	if _, err := tx.Exec("DELETE FROM ipam_addresses WHERE subnet_id = ?", subnetID); err != nil {
		return fmt.Errorf("clear addresses: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit resize: %w", err)
	}

	m.autoCreateAddresses(subnetID, spaceID, newNet)
	return nil
}

// DeleteSubnet deletes a subnet, refusing when anything still depends on it.
//
// The dependency scan runs a second time inside the deletion transaction. The
// first scan produces the error message; the second one is the authoritative
// decision, because between the two a scope can be created, and a check that
// runs only outside the transaction is exactly the check that misses it.
func (m *Manager) DeleteSubnet(id string) error {
	deps, err := m.CheckDependencies(id)
	if err != nil {
		return err
	}
	if len(deps) > 0 {
		var cidr string
		_ = m.db.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", id).Scan(&cidr)
		return &DependencyError{SubnetID: id, CIDR: cidr, Deps: deps}
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	deps, err = checkDependencies(tx, id)
	if err != nil {
		return err
	}
	if len(deps) > 0 {
		var cidr string
		_ = tx.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", id).Scan(&cidr)
		return &DependencyError{SubnetID: id, CIDR: cidr, Deps: deps}
	}

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
		return fmt.Errorf("%w: %s", ErrSubnetNotFound, id)
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
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
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
	// Reserve network and broadcast for IPv4, but only where they exist:
	// /31 is a point-to-point link (RFC 3021) and /32 is a host route, so
	// neither has a reserved pair to subtract.
	if bits == 32 && hostBits >= 2 {
		stats.Total -= 2
	}
	if stats.Total < 0 {
		stats.Total = 0
	}

	stats.Materialized = materialize(ipNet)

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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// A sparse subnet has no row for an address nothing has touched, so
	// counting rows would report zero free addresses for a /8 that is almost
	// entirely free. Derive the number instead, and say that it is derived:
	// the two figures are computed differently and an operator comparing them
	// against the allocation path needs to know that.
	if !stats.Materialized {
		occupied := stats.Used + stats.Reserved + stats.DHCP + stats.Static +
			stats.Gateway + stats.Excluded + stats.Conflict + stats.Unknown
		stats.Available = stats.Total - occupied
		if stats.Available < 0 {
			stats.Available = 0
		}
		stats.AvailableDerived = true
	}

	return stats, nil
}

// GenerateDHCPScope generates DHCP scope options from a subnet.
//
// The range comes from UsableHostRange, which also refuses IPv6 subnets; the
// defaults applied here (lease time, ping check, DNS updates off) are the ones
// a generated scope has always carried.
func (m *Manager) GenerateDHCPScope(subnetID string) (*scope.ScopeOptions, error) {
	s, err := m.GetSubnet(subnetID)
	if err != nil {
		return nil, err
	}

	startIP, endIP, err := UsableHostRange(s.CIDR)
	if err != nil {
		return nil, err
	}

	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(s.CIDR))
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR")
	}
	subnetMask := net.IP(ipNet.Mask).String()

	return &scope.ScopeOptions{
		Name:             s.Name,
		Subnet:           s.CIDR,
		StartIP:          startIP,
		EndIP:            endIP,
		SubnetMask:       &subnetMask,
		LeaseTime:        intPtr(86400),
		PingCheckEnabled: boolPtr(true),
		DNSUpdates:       boolPtr(false),
		Enabled:          boolPtr(true),
		Comment:          stringPtr("Generated from IPAM subnet " + s.Name),
	}, nil
}

// UsableHostRange returns the first and last address a DHCP pool built from
// cidr would use.
//
// It is exported because two callers need the same answer: the generated scope
// draft and the scope plan that a console previews before creating it. "Which
// addresses may this subnet hand out" must have one author -- two
// implementations of this arithmetic drift, and the drift shows up as a
// preview that does not match what is created.
//
// IPv4 only. An IPv6 subnet is refused rather than converted: indexing the
// address as bytes 0..3 is only meaningful for a 4-byte address, and on a
// 16-byte network it produces a range unrelated to the subnet while still
// reporting success. DHCPv6 is out of scope for this release, so saying so is
// the honest answer.
func UsableHostRange(cidr string) (string, string, error) {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return "", "", fmt.Errorf("invalid CIDR")
	}
	if ipNet.IP.To4() == nil {
		return "", "", fmt.Errorf("cannot generate an IPv4 DHCP scope from %s: %w", cidr, ErrDHCPv6Unsupported)
	}

	ones, _ := ipNet.Mask.Size()
	startIP, endIP := networkRange(ipNet)
	if startIP == nil || endIP == nil {
		return "", "", fmt.Errorf("invalid CIDR")
	}

	switch {
	case ones >= 32:
		// /32: a single host; start and end are the same address.
	case ones == 31:
		// /31 point-to-point (RFC 3021). Both addresses on a /31 are usable,
		// but the lower one is skipped so the pool does not include the
		// network address; a one-address pool is the conservative answer.
		incIP(startIP)
	default:
		// Skip the network address and the broadcast address, which are the
		// first and last addresses of the range.
		incIP(startIP)
		decIP(endIP)
	}
	return startIP.String(), endIP.String(), nil
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return
		}
	}
}

func decIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]--
		if ip[i] != 255 {
			return
		}
	}
}

// ReverseZonePlan is the reverse delegation a subnet could be published in.
//
// It carries the whole ordered list rather than one answer because the choice
// belongs to the operator: a /24 belongs in its own /24 zone, but a site that
// delegates 10.0.0.0/16 wants to be able to see that option too. ZoneName is
// the most specific candidate, which is what a caller that does not want to
// choose should use.
type ReverseZonePlan struct {
	SubnetID   string   `json:"subnet_id"`
	CIDR       string   `json:"cidr"`
	ZoneName   string   `json:"zone_name"`
	Candidates []string `json:"candidates"`
}

// PlanReverseZone describes where a subnet's addresses would be published in
// reverse DNS, most specific candidate first.
//
// This computes and writes nothing. Creating the zone is a DNS write with its
// own permission, so the two steps are separate on purpose; a caller that
// treats this as the action reports a write that never happened.
func (m *Manager) PlanReverseZone(subnetID string) (*ReverseZonePlan, error) {
	s, err := m.GetSubnet(subnetID)
	if err != nil {
		return nil, err
	}

	// The bare roots are dropped on purpose. ReverseZoneCandidates answers
	// "which zones could contain these addresses", which is the question the
	// dependency check asks, and the root is a legitimate answer there. This
	// function answers "which zone should this subnet be published in", and the
	// root never is: creating an in-addr.arpa or ip6.arpa zone locally takes
	// over reverse resolution for the whole tree instead of delegating a
	// subtree, and offering it as a choice invites exactly that mistake.
	all := ReverseZoneCandidates(s.CIDR)
	candidates := make([]string, 0, len(all))
	for _, c := range all {
		if c == "in-addr.arpa" || c == "ip6.arpa" {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("subnet %s has no reverse zone: %q is not a CIDR", subnetID, s.CIDR)
	}

	return &ReverseZonePlan{
		SubnetID:   s.ID,
		CIDR:       s.CIDR,
		ZoneName:   candidates[0],
		Candidates: candidates,
	}, nil
}

// GenerateReverseZone returns the single reverse zone a subnet belongs in: the
// most specific candidate, which is the one whose delegation matches the
// subnet's own prefix. Callers that want to show the alternatives use
// PlanReverseZone instead.
func (m *Manager) GenerateReverseZone(subnetID string) (string, error) {
	plan, err := m.PlanReverseZone(subnetID)
	if err != nil {
		return "", err
	}
	return plan.ZoneName, nil
}

// checkOverlap checks whether a CIDR overlaps another subnet *in the same
// space*.
//
// Scoping to the space is the whole point of having spaces. Two sites both
// using 10.0.0.0/24 is the normal case for an RFC1918 plan, and a global
// overlap check makes the second site impossible to add. Subnets in different
// spaces are separate address plans; only within one space must they be
// disjoint, which is also what makes (space_id, ip_address) a valid identity
// for an address.
func (m *Manager) checkOverlap(cidr, excludeID, spaceID string) error {
	_, newNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	rows, err := m.db.Query("SELECT id, cidr FROM ipam_subnets WHERE space_id = ?", spaceID)
	if err != nil {
		return fmt.Errorf("check overlap: %w", err)
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
	return rows.Err()
}

// networksOverlap returns true when two IP networks share at least one
// address. It compares the start / end of the address ranges produced by
// NetworkRange (the first and last IP in each network) so that it works
// correctly for subnets that do not begin on a byte boundary (e.g. 10.0.0.5/24).
//
// Subnets of different address families never overlap. Without this guard a
// byte-wise comparison of a 4-byte and a 16-byte range has no ordering, and
// the comparison falls through to "equal", reporting every IPv6 subnet as
// colliding with every IPv4 subnet in the same space.
func networksOverlap(a, b *net.IPNet) bool {
	aStart, aEnd := networkRange(a)
	bStart, bEnd := networkRange(b)
	if aStart == nil || bStart == nil {
		return false
	}
	// Normalise v4-mapped forms so a 16-byte 192.0.2.0/24 and a 4-byte one are
	// still recognised as the same family.
	a4, b4 := aStart.To4(), bStart.To4()
	switch {
	case a4 != nil && b4 != nil:
		aStart, aEnd = a4, aEnd.To4()
		bStart, bEnd = b4, bEnd.To4()
	case a4 != nil || b4 != nil:
		// Exactly one of them is IPv4.
		return false
	}
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
//
// Only subnets at or below autoCreateMaxAddresses are materialised. Larger
// subnets stay sparse: their rows are created on demand by the allocation
// path, so a /8 or an IPv6 /64 costs nothing until something touches an
// address in it. Pre-creating one row per address -- or, worse, one bit per
// address -- does not survive contact with a real IPv6 prefix.
//
// The work runs in batches inside one transaction per batch to keep the
// per-insert cost low. It is safe to call after the subnet row has been
// committed, and safe to call twice: existing rows are left untouched.
func (m *Manager) autoCreateAddresses(subnetID, spaceID string, ipNet *net.IPNet) {
	if !materialize(ipNet) {
		return
	}

	ones, bits := ipNet.Mask.Size()
	hostBits := bits - ones
	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)

	// Skip network address (and, for IPv4, stop before the broadcast).
	if hostBits > 0 {
		incIP(ip)
	}

	var broadcast net.IP
	if bits == 32 && hostBits >= 2 {
		_, end := networkRange(ipNet)
		broadcast = end
	}

	const autoCreateBatchSize = 500
	batch := make([]string, 0, autoCreateBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := m.insertAddressBatch(subnetID, spaceID, batch); err != nil {
			slog.Warn("ipam: autoCreateAddresses batch insert failed",
				"subnet_id", subnetID, "batch_size", len(batch), "error", err)
		}
		batch = batch[:0]
	}

	for ipNet.Contains(ip) {
		if broadcast != nil && ip.Equal(broadcast) {
			break
		}
		batch = append(batch, ip.String())
		if len(batch) >= autoCreateBatchSize {
			flush()
		}
		incIP(ip)
	}
	flush()
}

// insertAddressBatch inserts a batch of IP address rows in a single
// transaction. Existing rows (space_id, ip_address) are left untouched via
// INSERT OR IGNORE so the operation is idempotent.
func (m *Manager) insertAddressBatch(subnetID, spaceID string, ips []string) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO ipam_addresses
			(id, subnet_id, space_id, ip_address, status, observed_state, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'available', 'unknown', datetime('now'), datetime('now'))`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, ip := range ips {
		if _, err := stmt.Exec(uuid.New().String(), subnetID, spaceID, ip); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func intPtr(v int) *int          { return &v }
func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }
