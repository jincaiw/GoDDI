package address

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
)

// Address represents an IPAM address.
//
// Status and ObservedState answer different questions and are written by
// different actors; see status.go. The observed_* columns are the only ones a
// data-plane sync may touch on its own.
type Address struct {
	ID        string `json:"id"`
	SubnetID  string `json:"subnet_id"`
	SpaceID   string `json:"space_id"`
	IPAddress string `json:"ip_address"`
	Status    Status `json:"status"`

	MACAddress  string `json:"mac_address,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	DHCPLeaseID string `json:"dhcp_lease_id,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Device      string `json:"device,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`

	// DNSRecordID is deprecated. It held a single record id, so a host with
	// both an A and a PTR could only track one. Links now live in
	// ipam_dns_links; the column is retained so existing readers keep working
	// and is no longer written by any code path.
	DNSRecordID string `json:"dns_record_id,omitempty"`

	ObservedState    ObservedState `json:"observed_state"`
	ObservedAt       string        `json:"observed_at,omitempty"`
	ObservedSource   string        `json:"observed_source,omitempty"`
	ObservedMAC      string        `json:"observed_mac,omitempty"`
	ObservedHostname string        `json:"observed_hostname,omitempty"`

	AllocatedAt string `json:"allocated_at,omitempty"`
	AllocatedBy string `json:"allocated_by,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AddressOptions holds parameters for updating an address.
type AddressOptions struct {
	Status      *Status `json:"status,omitempty"`
	MACAddress  string  `json:"mac_address,omitempty"`
	Hostname    string  `json:"hostname,omitempty"`
	Owner       string  `json:"owner,omitempty"`
	Device      string  `json:"device,omitempty"`
	Location    string  `json:"location,omitempty"`
	Description string  `json:"description,omitempty"`
	// Actor is recorded in the history alongside the change.
	Actor string `json:"-"`
	// Source attributes the change to a subsystem. Defaults to SourceAdmin.
	Source string `json:"-"`
	// Reason explains the change and is stored verbatim in the history.
	Reason string `json:"-"`
}

// AddressFilter holds filter parameters for listing addresses.
type AddressFilter struct {
	SubnetID      string        `json:"subnet_id,omitempty"`
	SpaceID       string        `json:"space_id,omitempty"`
	Status        Status        `json:"status,omitempty"`
	ObservedState ObservedState `json:"observed_state,omitempty"`
	IPAddress     string        `json:"ip_address,omitempty"`
	MACAddress    string        `json:"mac_address,omitempty"`
	Hostname      string        `json:"hostname,omitempty"`
	Owner         string        `json:"owner,omitempty"`
	// Search is the console's single search box: one input standing for four
	// possible columns. It has to be honoured here, because the console has no
	// way to know which of them the operator meant, and a filter that quietly
	// returns everything reads as "these are the matches".
	Search   string `json:"search,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// AllocateRequest describes an allocation. An empty IPAddress asks the manager
// to pick one, which is why there is no separate "auto assign then allocate"
// pair: two calls let another writer take the address in between, and the
// second call then fails with a confusing "not available" for an address the
// caller never chose.
type AllocateRequest struct {
	SubnetID    string
	IPAddress   string
	Status      Status
	MACAddress  string
	Hostname    string
	Owner       string
	Device      string
	Location    string
	Description string
	// Actor is recorded as the author of the change. It is written as an empty
	// string when unset, never as NULL: allocated_by is NOT NULL, and an
	// explicit NULL in an INSERT or UPDATE fails the constraint rather than
	// falling back to the column default.
	Actor  string
	Source string
	Reason string
}

// Observation is what a data plane reported about an address.
type Observation struct {
	State    ObservedState
	MAC      string
	Hostname string
	Source   string
	LeaseID  string
	Actor    string
}

// MaxAutoAllocateAttempts bounds how many candidate addresses are examined
// when the caller does not name one. Each candidate is claimed with a
// conditional write, so a candidate that another writer won is skipped rather
// than failed. The bound keeps a nearly full subnet from turning one request
// into an unbounded scan.
const MaxAutoAllocateAttempts = 256

// Manager provides CRUD and allocation operations for IPAM addresses.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new address manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// querier is satisfied by both *sql.DB and *sql.Tx.
//
// Every operation that spans a read and a write takes one of these so the two
// halves run on the same connection. With SQLite capped at a single connection
// (see internal/database), a statement issued on the pool while a transaction
// is open would wait for a connection the transaction is holding.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

const addressColumns = `id, subnet_id, space_id, ip_address, status, mac_address, hostname,
	dns_record_id, dhcp_lease_id, owner, device, location, description, last_seen,
	observed_state, observed_at, observed_source, observed_mac, observed_hostname,
	allocated_at, allocated_by, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanAddress(row rowScanner) (*Address, error) {
	a := &Address{}
	var mac, host, dnsRecordID, leaseID, owner, device, location, description, lastSeen sql.NullString
	var observedAt, observedMAC, observedHostname, allocatedAt sql.NullString

	err := row.Scan(&a.ID, &a.SubnetID, &a.SpaceID, &a.IPAddress, &a.Status,
		&mac, &host, &dnsRecordID, &leaseID, &owner, &device, &location, &description, &lastSeen,
		&a.ObservedState, &observedAt, &a.ObservedSource, &observedMAC, &observedHostname,
		&allocatedAt, &a.AllocatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}

	a.MACAddress = mac.String
	a.Hostname = host.String
	a.DNSRecordID = dnsRecordID.String
	a.DHCPLeaseID = leaseID.String
	a.Owner = owner.String
	a.Device = device.String
	a.Location = location.String
	a.Description = description.String
	a.LastSeen = lastSeen.String
	a.ObservedAt = observedAt.String
	a.ObservedMAC = observedMAC.String
	a.ObservedHostname = observedHostname.String
	a.AllocatedAt = allocatedAt.String
	if !a.ObservedState.Valid() {
		a.ObservedState = ObservedUnknown
	}
	return a, nil
}

// GetAddress retrieves an address by ID.
func (m *Manager) GetAddress(id string) (*Address, error) {
	a, err := scanAddress(m.db.QueryRow(`SELECT `+addressColumns+` FROM ipam_addresses WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrAddressNotFound, id)
	}
	return a, err
}

// GetAddressByIP retrieves an address by subnet and IP. The IP is normalised
// before lookup so a caller that spells an address differently still finds the
// existing row instead of concluding the address is free.
//
// It returns (nil, nil) when no row matches: an unallocated address in a
// materialised subnet still has a row, so absence means the subnet was never
// materialised rather than that the address is in use.
func (m *Manager) GetAddressByIP(subnetID, ip string) (*Address, error) {
	canonical, err := NormalizeIP(ip)
	if err != nil {
		return nil, err
	}
	a, err := scanAddress(m.db.QueryRow(
		`SELECT `+addressColumns+` FROM ipam_addresses WHERE subnet_id = ? AND ip_address = ?`,
		subnetID, canonical))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// GetAddressBySpaceIP retrieves an address by space and IP. Because an address
// belongs to exactly one subnet per space and subnets in a space may not
// overlap, the (space, IP) pair identifies at most one row.
func (m *Manager) GetAddressBySpaceIP(spaceID, ip string) (*Address, error) {
	canonical, err := NormalizeIP(ip)
	if err != nil {
		return nil, err
	}
	a, err := scanAddress(m.db.QueryRow(
		`SELECT `+addressColumns+` FROM ipam_addresses WHERE space_id = ? AND ip_address = ?`,
		spaceID, canonical))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func getAddressBySpaceIPTx(tx querier, spaceID, ip string) (*Address, error) {
	a, err := scanAddress(tx.QueryRow(
		`SELECT `+addressColumns+` FROM ipam_addresses WHERE space_id = ? AND ip_address = ?`,
		spaceID, ip))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// ListAddresses lists addresses with filtering and pagination.
func (m *Manager) ListAddresses(filter AddressFilter) ([]Address, int64, error) {
	var conditions []string
	var args []any

	if filter.SubnetID != "" {
		conditions = append(conditions, "subnet_id = ?")
		args = append(args, filter.SubnetID)
	}
	if filter.SpaceID != "" {
		conditions = append(conditions, "space_id = ?")
		args = append(args, filter.SpaceID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.ObservedState != "" {
		conditions = append(conditions, "observed_state = ?")
		args = append(args, string(filter.ObservedState))
	}
	if filter.IPAddress != "" {
		canonical, err := NormalizeIP(filter.IPAddress)
		if err != nil {
			return nil, 0, err
		}
		conditions = append(conditions, "ip_address = ?")
		args = append(args, canonical)
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
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		// An address typed in full is matched exactly rather than by substring:
		// `LIKE '%192.0.2.20%'` also returns 192.0.2.200 through 192.0.2.209,
		// so "find this address" would come back with the whole neighbourhood
		// and the operator would have to pick their row out of it by eye.
		if canonical, err := NormalizeIP(filter.Search); err == nil {
			conditions = append(conditions, "(ip_address = ? OR hostname LIKE ? OR mac_address LIKE ? OR owner LIKE ?)")
			args = append(args, canonical, like, like, like)
		} else {
			conditions = append(conditions, "(ip_address LIKE ? OR hostname LIKE ? OR mac_address LIKE ? OR owner LIKE ?)")
			args = append(args, like, like, like, like)
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := m.db.QueryRow("SELECT COUNT(*) FROM ipam_addresses "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, pageSize, offset := normalizePage(filter.Page, filter.PageSize)

	querySQL := `SELECT ` + addressColumns + ` FROM ipam_addresses ` + whereClause + `
		ORDER BY ip_address ASC LIMIT ? OFFSET ?`
	rows, err := m.db.Query(querySQL, append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var addresses []Address
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, 0, err
		}
		addresses = append(addresses, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating addresses: %w", err)
	}
	_ = page
	return addresses, total, nil
}

func normalizePage(page, pageSize int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

// UpdateAddress updates the descriptive fields of an address and, when a
// status is supplied, routes the change through the state machine.
func (m *Manager) UpdateAddress(id string, opts AddressOptions) (*Address, error) {
	if opts.Status != nil {
		if _, err := m.TransitionAddress(id, *opts.Status, opts.Actor, opts.Reason, opts.Source); err != nil {
			return nil, err
		}
	}

	existing, err := m.GetAddress(id)
	if err != nil {
		return nil, err
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

	if _, err := m.db.Exec(`
		UPDATE ipam_addresses SET mac_address=?, hostname=?, owner=?,
			device=?, location=?, description=?, updated_at=datetime('now')
		WHERE id=?`,
		nullIfEmpty(existing.MACAddress), nullIfEmpty(existing.Hostname),
		nullIfEmpty(existing.Owner), nullIfEmpty(existing.Device), nullIfEmpty(existing.Location),
		nullIfEmpty(existing.Description), id); err != nil {
		return nil, fmt.Errorf("failed to update address: %w", err)
	}

	return m.GetAddress(id)
}

// TransitionAddress applies a status change, refusing illegal ones.
//
// The status update and its history row are written in one transaction. The
// previous implementation wrote the history outside any transaction and
// discarded the error, so a failed insert produced a status change with no
// record of who made it or why -- and the audit trail looked complete.
func (m *Manager) TransitionAddress(id string, to Status, actor, reason, source string) (*Address, error) {
	if !to.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, to)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := scanAddress(tx.QueryRow(`SELECT `+addressColumns+` FROM ipam_addresses WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrAddressNotFound, id)
	}
	if err != nil {
		return nil, err
	}

	if current.Status == to {
		// Nothing to change. Returning the row rather than an error keeps
		// callers that re-assert a desired state idempotent.
		return current, nil
	}
	if !CanTransition(current.Status, to) {
		return nil, transitionError(current.Status, to)
	}

	// The write is guarded on the observed status so a concurrent transition
	// cannot be silently overwritten. Two writers that both read "available"
	// and both write different targets must not both succeed.
	res, err := tx.Exec(`
		UPDATE ipam_addresses SET status=?, updated_at=datetime('now')
		WHERE id = ? AND status = ?`, string(to), id, string(current.Status))
	if err != nil {
		return nil, fmt.Errorf("update address status: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, fmt.Errorf("%w: %s changed concurrently", ErrAddressTaken, current.IPAddress)
	}

	if err := insertHistoryTx(tx, historyEntry{
		SpaceID:   current.SpaceID,
		SubnetID:  current.SubnetID,
		IPAddress: current.IPAddress,
		Action:    "transition",
		OldStatus: string(current.Status),
		NewStatus: string(to),
		Actor:     actor,
		Reason:    reason,
		Source:    defaultSource(source),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transition: %w", err)
	}
	return m.GetAddress(id)
}

// AllocateIP allocates an explicit address, or picks one when IPAddress is
// empty. Both paths claim the address with a conditional write, so two callers
// racing for the same address cannot both be told they succeeded.
func (m *Manager) AllocateIP(req AllocateRequest) (*Address, error) {
	if strings.TrimSpace(req.SubnetID) == "" {
		return nil, fmt.Errorf("subnet_id is required")
	}
	target := req.Status
	if target == "" {
		target = StatusUsed
	}
	if !target.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, target)
	}

	sub, err := loadSubnet(m.db, req.SubnetID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.IPAddress) != "" {
		canonical, _, _, err := ParseIP(req.IPAddress, sub.CIDR)
		if err != nil {
			return nil, err
		}
		return m.allocateSpecific(sub, canonical, target, req)
	}
	return m.allocateAuto(sub, target, req)
}

// AutoAllocateIP allocates the next free address in a subnet.
func (m *Manager) AutoAllocateIP(subnetID string, req AllocateRequest) (*Address, error) {
	req.SubnetID = subnetID
	req.IPAddress = ""
	if req.Status == "" {
		req.Status = StatusUsed
	}
	return m.AllocateIP(req)
}

func (m *Manager) allocateSpecific(sub *subnetRow, ip string, target Status, req AllocateRequest) (*Address, error) {
	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin allocate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := getAddressBySpaceIPTx(tx, sub.SpaceID, ip)
	if err != nil {
		return nil, err
	}

	id := ""
	from := StatusAvailable

	switch {
	case existing != nil:
		if !existing.Status.Allocatable() {
			return nil, fmt.Errorf("%w: %s is %s", ErrAddressTaken, ip, existing.Status)
		}
		if !CanTransition(existing.Status, target) {
			return nil, transitionError(existing.Status, target)
		}
		id, from = existing.ID, existing.Status
		res, err := tx.Exec(`
			UPDATE ipam_addresses SET status=?, mac_address=?, hostname=?, owner=?,
				device=?, location=?, description=?,
				allocated_at=datetime('now'), allocated_by=?, updated_at=datetime('now')
			WHERE id = ? AND status = ?`,
			string(target), nullIfEmpty(req.MACAddress), nullIfEmpty(req.Hostname),
			nullIfEmpty(req.Owner), nullIfEmpty(req.Device), nullIfEmpty(req.Location),
			nullIfEmpty(req.Description), req.Actor, existing.ID, string(existing.Status))
		if err != nil {
			return nil, fmt.Errorf("failed to allocate IP: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return nil, err
		} else if n == 0 {
			return nil, fmt.Errorf("%w: %s was taken concurrently", ErrAddressTaken, ip)
		}

	default:
		if !CanTransition(StatusAvailable, target) {
			return nil, transitionError(StatusAvailable, target)
		}
		id = uuid.New().String()
		if _, err := tx.Exec(`
			INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status,
				mac_address, hostname, owner, device, location, description,
				observed_state, allocated_at, allocated_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), ?, datetime('now'), datetime('now'))`,
			id, sub.ID, sub.SpaceID, ip, string(target),
			nullIfEmpty(req.MACAddress), nullIfEmpty(req.Hostname), nullIfEmpty(req.Owner),
			nullIfEmpty(req.Device), nullIfEmpty(req.Location), nullIfEmpty(req.Description),
			string(ObservedUnknown), req.Actor); err != nil {
			if isUniqueViolation(err) {
				return nil, fmt.Errorf("%w: %s", ErrAddressTaken, ip)
			}
			return nil, fmt.Errorf("failed to allocate IP: %w", err)
		}
	}

	reason := req.Reason
	if reason == "" {
		reason = "allocated"
	}
	if err := insertHistoryTx(tx, historyEntry{
		SpaceID: sub.SpaceID, SubnetID: sub.ID, IPAddress: ip,
		Action: "allocate", OldStatus: string(from), NewStatus: string(target),
		Actor: req.Actor, Reason: reason, Source: defaultSource(req.Source),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit allocate: %w", err)
	}
	return m.GetAddress(id)
}

func (m *Manager) allocateAuto(sub *subnetRow, target Status, req AllocateRequest) (*Address, error) {
	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin allocate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	taken, err := takenBySubnetTx(tx, sub.ID)
	if err != nil {
		return nil, err
	}

	_, ipNet, err := net.ParseCIDR(sub.CIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet CIDR %q: %w", sub.CIDR, err)
	}
	candidates := candidatesInSubnet(ipNet, taken, MaxAutoAllocateAttempts)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrPoolExhausted, sub.CIDR)
	}

	for _, ip := range candidates {
		a, err := m.claimCandidateTx(tx, sub, ip, target, req)
		if err != nil {
			if errors.Is(err, ErrAddressTaken) {
				// Another writer got there first. The next candidate is just
				// as good as this one; failing the request would make a
				// concurrent allocation look like a full pool.
				continue
			}
			return nil, err
		}
		if err := insertHistoryTx(tx, historyEntry{
			SpaceID: sub.SpaceID, SubnetID: sub.ID, IPAddress: ip,
			Action: "allocate", OldStatus: string(StatusAvailable), NewStatus: string(target),
			Actor: req.Actor, Reason: "auto-allocated", Source: defaultSource(req.Source),
		}); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit allocate: %w", err)
		}
		return m.GetAddress(a.ID)
	}

	return nil, fmt.Errorf("%w: %s (examined %d candidates)", ErrPoolExhausted, sub.CIDR, len(candidates))
}

// claimCandidateTx claims one candidate address, returning ErrAddressTaken when
// somebody else holds it. It never returns a success it did not verify: the
// state guard on the row and the conditional UPDATE's row count decide whether
// the caller won, and on the create path the unique index on
// (space_id, ip_address) does.
//
// Under the current SQLite configuration the read and the write happen inside
// one transaction held on the single pooled connection, so no writer can slip
// between them and the row count is a second line of defence rather than the
// arbiter -- which is why no test can make it fail on its own. It becomes the
// arbiter as soon as the connection limit is raised or the store is replaced,
// which is exactly the direction the data-plane separation takes, so it stays.
func (m *Manager) claimCandidateTx(tx *sql.Tx, sub *subnetRow, ip string, target Status, req AllocateRequest) (*Address, error) {
	existing, err := getAddressBySpaceIPTx(tx, sub.SpaceID, ip)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if !existing.Status.Allocatable() || !CanTransition(existing.Status, target) {
			return nil, fmt.Errorf("%w: %s", ErrAddressTaken, ip)
		}
		res, err := tx.Exec(`
			UPDATE ipam_addresses SET status=?, mac_address=?, hostname=?, owner=?,
				device=?, location=?, description=?,
				allocated_at=datetime('now'), allocated_by=?, updated_at=datetime('now')
			WHERE id = ? AND status = ?`,
			string(target), nullIfEmpty(req.MACAddress), nullIfEmpty(req.Hostname),
			nullIfEmpty(req.Owner), nullIfEmpty(req.Device), nullIfEmpty(req.Location),
			nullIfEmpty(req.Description), req.Actor, existing.ID, string(StatusAvailable))
		if err != nil {
			return nil, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return nil, err
		} else if n == 0 {
			return nil, fmt.Errorf("%w: %s", ErrAddressTaken, ip)
		}
		return existing, nil
	}

	if !CanTransition(StatusAvailable, target) {
		return nil, transitionError(StatusAvailable, target)
	}
	id := uuid.New().String()
	if _, err := tx.Exec(`
		INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status,
			mac_address, hostname, owner, device, location, description,
			observed_state, allocated_at, allocated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), ?, datetime('now'), datetime('now'))`,
		id, sub.ID, sub.SpaceID, ip, string(target),
		nullIfEmpty(req.MACAddress), nullIfEmpty(req.Hostname), nullIfEmpty(req.Owner),
		nullIfEmpty(req.Device), nullIfEmpty(req.Location), nullIfEmpty(req.Description),
		string(ObservedUnknown), req.Actor); err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: %s", ErrAddressTaken, ip)
		}
		return nil, err
	}
	return &Address{ID: id, IPAddress: ip}, nil
}

// ReleaseIP returns an address to available.
//
// A structural address (gateway, excluded) is not released by this path: the
// caller asking to free the router address almost always means something else,
// and silently making it allocatable is how a host ends up configured with the
// gateway's address. Use TransitionAddress for that, which records the reason.
func (m *Manager) ReleaseIP(id string) error {
	_, err := m.ReleaseAddress(id, "", "", SourceAdmin)
	return err
}

// ReleaseAddress returns an address to available, attributing the change.
func (m *Manager) ReleaseAddress(id, actor, reason, source string) (*Address, error) {
	current, err := m.GetAddress(id)
	if err != nil {
		return nil, err
	}
	if current.Status.Structural() {
		return nil, fmt.Errorf(
			"%w: %s is %s and is not released by the allocation path; move it explicitly",
			ErrIllegalTransition, current.IPAddress, current.Status)
	}
	return m.TransitionAddress(id, StatusAvailable, actor, reason, source)
}

// ObserveBySpaceIP records what a data plane reported about an address.
//
// The observation columns are always refreshed. The allocation status is only
// changed when PlanLeaseObservation says the lease implies a change, so a
// DHCP renewal cannot overwrite an administrative reservation.
//
// It returns changed=false with no error when the address has no row: the
// caller decides whether the address is worth materialising.
func (m *Manager) ObserveBySpaceIP(spaceID, ip string, obs Observation) (bool, error) {
	canonical, err := NormalizeIP(ip)
	if err != nil {
		return false, err
	}
	if !obs.State.Valid() {
		return false, fmt.Errorf("%w: %q", ErrInvalidStatus, obs.State)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := getAddressBySpaceIPTx(tx, spaceID, canonical)
	if err != nil {
		return false, err
	}
	if current == nil {
		return false, nil
	}

	newStatus, changed, reason := PlanLeaseObservation(current.Status, obs.State)

	if _, err := tx.Exec(`
		UPDATE ipam_addresses SET observed_state=?, observed_at=datetime('now'),
			observed_source=?, observed_mac=?, observed_hostname=?, last_seen=datetime('now'),
			dhcp_lease_id=COALESCE(NULLIF(?, ''), dhcp_lease_id),
			updated_at=datetime('now')
		WHERE id = ?`,
		string(obs.State), obs.Source, nullIfEmpty(obs.MAC), nullIfEmpty(obs.Hostname),
		obs.LeaseID, current.ID); err != nil {
		return false, fmt.Errorf("record observation: %w", err)
	}

	if changed {
		res, err := tx.Exec(`UPDATE ipam_addresses SET status=?, updated_at=datetime('now')
			WHERE id = ? AND status = ?`, string(newStatus), current.ID, string(current.Status))
		if err != nil {
			return false, fmt.Errorf("apply observation status: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return false, err
		} else if n == 0 {
			// A concurrent writer moved the status. The observation itself is
			// recorded, which is the part that matters; leave the status alone
			// rather than forcing a decision onto a state we never read.
			return false, tx.Commit()
		}
		if err := insertHistoryTx(tx, historyEntry{
			SpaceID: current.SpaceID, SubnetID: current.SubnetID, IPAddress: canonical,
			Action: "observe", OldStatus: string(current.Status), NewStatus: string(newStatus),
			Actor: obs.Actor, Reason: reason, Source: defaultSource(obs.Source),
		}); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit observation: %w", err)
	}
	return changed, nil
}

// EnsureAddress creates the row for an address if it does not exist.
//
// Callers need this because a subnet larger than the materialisation threshold
// has no rows until something touches an address in it. A data plane that
// reports activity on such an address should be able to create the row rather
// than have its report dropped.
func (m *Manager) EnsureAddress(subnetID, ip, actor string) (*Address, error) {
	sub, err := loadSubnet(m.db, subnetID)
	if err != nil {
		return nil, err
	}
	canonical, _, _, err := ParseIP(ip, sub.CIDR)
	if err != nil {
		return nil, err
	}

	if _, err := m.db.Exec(`
		INSERT OR IGNORE INTO ipam_addresses
			(id, subnet_id, space_id, ip_address, status, observed_state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		uuid.New().String(), sub.ID, sub.SpaceID, canonical,
		string(StatusAvailable), string(ObservedUnknown)); err != nil {
		return nil, fmt.Errorf("ensure address: %w", err)
	}
	_ = actor
	return m.GetAddressBySpaceIP(sub.SpaceID, canonical)
}

// --- helpers ---------------------------------------------------------------

type subnetRow struct {
	ID      string
	SpaceID string
	CIDR    string
}

func loadSubnet(q querier, subnetID string) (*subnetRow, error) {
	s := &subnetRow{}
	err := q.QueryRow("SELECT id, space_id, cidr FROM ipam_subnets WHERE id = ?", subnetID).
		Scan(&s.ID, &s.SpaceID, &s.CIDR)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// takenBySubnetTx returns the addresses in a subnet that are not allocatable.
//
// The cursor is fully drained and closed before returning. With SQLite capped
// at one connection, issuing another statement while this cursor is open waits
// for the connection the cursor holds -- no error, no timeout, just a hang.
func takenBySubnetTx(tx querier, subnetID string) (map[string]bool, error) {
	rows, err := tx.Query(
		"SELECT ip_address FROM ipam_addresses WHERE subnet_id = ? AND status <> ?",
		subnetID, string(StatusAvailable))
	if err != nil {
		return nil, fmt.Errorf("query taken addresses: %w", err)
	}
	taken := make(map[string]bool)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			rows.Close()
			return nil, err
		}
		taken[ip] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterating taken addresses: %w", err)
	}
	rows.Close()
	return taken, nil
}

// candidatesInSubnet returns up to limit allocatable addresses in order.
//
// It never builds a bitmap or materialises the range. For a /8 or an IPv6 /64
// the first `limit` candidates are produced by incrementing from the network
// address, so the cost depends on `limit` rather than on the size of the
// subnet. That is the sparse representation W06 asks for: nothing in this
// package ever pre-creates one row, or one bit, per possible address.
func candidatesInSubnet(n *net.IPNet, taken map[string]bool, limit int) []string {
	if n == nil || len(n.IP) == 0 || limit <= 0 {
		return nil
	}
	ones, bits := n.Mask.Size()
	hostBits := bits - ones

	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)

	out := make([]string, 0, limit)
	add := func() bool {
		s := ip.String()
		if !taken[s] {
			out = append(out, s)
		}
		return len(out) < limit
	}

	switch {
	case hostBits == 0:
		// /32 (IPv4) or /128 (IPv6): the network address is the only address.
		add()
		return out

	case hostBits == 1 && bits == 32:
		// /31 point-to-point (RFC 3021): both addresses are usable, there is
		// no network or broadcast address to skip.
		for i := 0; i < 2 && add(); i++ {
			incIP(ip)
		}
		return out
	}

	// Skip the network address.
	incIP(ip)

	var broadcast net.IP
	if bits == 32 {
		broadcast = broadcastOf(n)
	}

	for n.Contains(ip) {
		if broadcast != nil && ip.Equal(broadcast) {
			break
		}
		if !add() {
			break
		}
		incIP(ip)
	}
	return out
}

func broadcastOf(n *net.IPNet) net.IP {
	if n == nil || len(n.IP) == 0 {
		return nil
	}
	out := make(net.IP, len(n.IP))
	for i := range out {
		out[i] = n.IP[i] | ^n.Mask[i]
	}
	return out
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			return
		}
	}
}

// historyEntry is one row of ipam_history.
type historyEntry struct {
	SpaceID   string
	SubnetID  string
	IPAddress string
	Action    string
	OldStatus string
	NewStatus string
	Actor     string
	Reason    string
	Source    string
}

// insertHistoryTx writes a history row inside the caller's transaction.
//
// The error is returned, not discarded. The previous version used
// `_, _ = m.db.Exec(...)`, which meant a history insert that failed produced no
// warning anywhere: the status changed and the trail said nothing happened.
func insertHistoryTx(tx querier, h historyEntry) error {
	if _, err := tx.Exec(`
		INSERT INTO ipam_history
			(id, space_id, subnet_id, ip_address, action, old_status, new_status,
			 changed_by, reason, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		uuid.New().String(), h.SpaceID, h.SubnetID, h.IPAddress, h.Action,
		nullIfEmpty(h.OldStatus), nullIfEmpty(h.NewStatus),
		nullIfEmpty(h.Actor), h.Reason, defaultSource(h.Source)); err != nil {
		return fmt.Errorf("write ipam history: %w", err)
	}
	return nil
}

// HistoryFor returns the change history of one address, newest first.
func (m *Manager) HistoryFor(spaceID, ip string, limit int) ([]HistoryEntry, error) {
	canonical, err := NormalizeIP(ip)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := m.db.Query(`
		SELECT action, old_status, new_status, changed_by, reason, source, created_at
		FROM ipam_history WHERE space_id = ? AND ip_address = ?
		ORDER BY created_at DESC, rowid DESC LIMIT ?`, spaceID, canonical, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		var oldStatus, newStatus, actor, reason, source sql.NullString
		if err := rows.Scan(&h.Action, &oldStatus, &newStatus, &actor, &reason, &source, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.OldStatus = oldStatus.String
		h.NewStatus = newStatus.String
		h.Actor = actor.String
		h.Reason = reason.String
		h.Source = source.String
		out = append(out, h)
	}
	return out, rows.Err()
}

// HistoryEntry is one recorded change.
type HistoryEntry struct {
	Action    string `json:"action"`
	OldStatus string `json:"old_status,omitempty"`
	NewStatus string `json:"new_status,omitempty"`
	Actor     string `json:"changed_by,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at"`
}

// NonCanonicalAddresses finds rows whose stored text is not the canonical
// spelling of the address it represents.
//
// SQLite cannot canonicalise an IP, so migration 020 does not rewrite existing
// values. Rows written before this change by a caller that supplied an
// unusual-but-valid spelling would therefore sit outside the (space, IP)
// unique index and could be joined by a second row for the same host. This
// method makes that state visible instead of leaving it to be discovered as a
// duplicate allocation.
func (m *Manager) NonCanonicalAddresses(limit int) ([]Address, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := m.db.Query(`SELECT `+addressColumns+` FROM ipam_addresses LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Address
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}
		canonical, err := NormalizeIP(a.IPAddress)
		if err != nil || canonical != a.IPAddress {
			out = append(out, *a)
		}
	}
	return out, rows.Err()
}

func defaultSource(s string) string {
	if s == "" {
		return SourceAdmin
	}
	return s
}

func transitionError(from, to Status) error {
	return fmt.Errorf("%w: %s -> %s (allowed from %s: %v)",
		ErrIllegalTransition, from, to, from, AllowedTransitions(from))
}

// isUniqueViolation reports whether err is a uniqueness conflict.
//
// This is how the (space_id, ip_address) index reports a lost race. It is
// matched on the driver's constraint code and, as a fallback, on SQLite's
// message, because the code path that produces it is the arbiter for
// concurrent allocation and must not be mistaken for a generic failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		switch coded.Code() {
		case 1555, 2067: // SQLITE_CONSTRAINT_PRIMARYKEY, SQLITE_CONSTRAINT_UNIQUE
			return true
		}
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
