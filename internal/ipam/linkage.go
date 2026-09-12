package ipam

import (
	"bytes"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/space"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
	"github.com/jasonwa/goddi/pkg/dnsutil"
)

// Lease actions reported by the DHCP data plane.
const (
	LeaseActionBind    = "bind"
	LeaseActionRenew   = "renew"
	LeaseActionRelease = "release"
	LeaseActionExpire  = "expire"
	LeaseActionDecline = "decline"
)

// heldLeaseStatuses are the DHCP lease statuses that mean a client is holding
// an address. 'conflict' is included because a quarantined address is being
// kept away from clients deliberately; treating it as free would put it back
// in the pool from the IPAM side.
var heldLeaseStatuses = []string{"active", "offered", "conflict"}

// Linkage ties IPAM to the DHCP and DNS subsystems.
//
// It used to be dead code: nothing constructed it, so the DHCP server never
// updated IPAM and every address stayed "available" no matter how many clients
// held one. It is now the single bridge for lease observations, dependency
// checks and DNS record links, and it is constructed by the runtime.
type Linkage struct {
	db        *sql.DB
	addrMgr   *address.Manager
	subnetMgr *subnet.Manager
	spaceMgr  *space.Manager
}

// NewLinkage creates a new IPAM linkage manager.
func NewLinkage(db *sql.DB) *Linkage {
	return &Linkage{
		db:        db,
		addrMgr:   address.NewManager(db),
		subnetMgr: subnet.NewManager(db),
		spaceMgr:  space.NewManager(db),
	}
}

// Addresses exposes the address manager for callers that need allocation.
func (l *Linkage) Addresses() *address.Manager { return l.addrMgr }

// Subnets exposes the subnet manager.
func (l *Linkage) Subnets() *subnet.Manager { return l.subnetMgr }

// ObserveLease records what a DHCP lease says about an address.
//
// The signature is deliberately all primitives: the DHCP server declares the
// matching interface, so neither package imports the other and the data plane
// keeps working when the IPAM side is replaced. Fulfilling
// dhcp/server.LeaseObserver is structural, not by import.
//
// The observation never overwrites an administrative allocation. Whether the
// lease changes the allocation status is decided by
// address.PlanLeaseObservation; this method only makes sure the address exists
// and hands the report over.
func (l *Linkage) ObserveLease(action, leaseID, scopeID, ip, mac, hostname string) error {
	state, ok := observedStateFor(action)
	if !ok {
		return fmt.Errorf("unknown lease action %q", action)
	}
	if strings.TrimSpace(ip) == "" {
		return nil
	}

	sub, err := l.subnetForScope(scopeID, ip)
	if err != nil {
		return err
	}
	if sub == nil {
		// No IPAM subnet covers this scope. That is a configuration gap, not
		// a failure of the DHCP path: report it and let the lease stand.
		slog.Debug("ipam: no subnet covers DHCP scope; lease not tracked in IPAM",
			"scope_id", scopeID, "ip", ip)
		return nil
	}

	// A sparse subnet has no row until something touches an address in it, so
	// the report is allowed to create one. Without this the observation would
	// be dropped for exactly the subnets that need it most.
	if _, err := l.addrMgr.EnsureAddress(sub.ID, ip, "dhcp"); err != nil {
		return fmt.Errorf("ensure address %s: %w", ip, err)
	}

	_, err = l.addrMgr.ObserveBySpaceIP(sub.SpaceID, ip, address.Observation{
		State:    state,
		MAC:      mac,
		Hostname: hostname,
		Source:   address.SourceDHCP,
		LeaseID:  leaseID,
		Actor:    "dhcp",
	})
	return err
}

func observedStateFor(action string) (address.ObservedState, bool) {
	switch action {
	case LeaseActionBind, LeaseActionRenew:
		return address.ObservedInUse, true
	case LeaseActionRelease, LeaseActionExpire:
		return address.ObservedFree, true
	case LeaseActionDecline:
		// A DECLINE says the address is taken by somebody else, not that it is
		// free. Recording it as free would hand a known-conflicting address
		// straight back out and reproduce the decline loop the quarantine was
		// introduced to stop.
		return address.ObservedContested, true
	}
	return address.ObservedUnknown, false
}

// subnetForScope maps a DHCP scope to the IPAM subnet that contains it.
//
// The scope's own `subnet` column is tried first, then containment. Both steps
// exist because the two subsystems can disagree: a scope created before its
// CIDR was canonicalised stores 10.0.0.5/24 where IPAM holds 10.0.0.0/24, and
// an exact-match-only lookup would conclude the subnet does not exist and drop
// every observation for the scope.
func (l *Linkage) subnetForScope(scopeID, ip string) (*subnetLookup, error) {
	var scopeCIDR, scopeName sql.NullString
	err := l.db.QueryRow("SELECT subnet, name FROM dhcp_scopes WHERE id = ?", scopeID).
		Scan(&scopeCIDR, &scopeName)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("load DHCP scope: %w", err)
	}

	if scopeCIDR.String != "" {
		if canonical, _, err := net.ParseCIDR(strings.TrimSpace(scopeCIDR.String)); err == nil {
			var id, spaceID, cidr string
			err := l.db.QueryRow(
				"SELECT id, space_id, cidr FROM ipam_subnets WHERE cidr = ?", canonical.String()).
				Scan(&id, &spaceID, &cidr)
			if err == nil {
				return &subnetLookup{ID: id, SpaceID: spaceID, CIDR: cidr}, nil
			}
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("load IPAM subnet: %w", err)
			}
		}
	}

	// Containment fallback: find the most specific subnet that contains the
	// address being reported.
	return l.mostSpecificSubnetForIP(ip)
}

// mostSpecificSubnetForIP finds the tightest IPAM subnet containing ip.
//
// The rows are materialised and the cursor closed before returning: SQLite
// runs on a single connection, and issuing another statement while a cursor is
// open waits for the connection that cursor holds -- no error, no timeout.
func (l *Linkage) mostSpecificSubnetForIP(ip string) (*subnetLookup, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return nil, fmt.Errorf("%w: %q", address.ErrInvalidIP, ip)
	}

	rows, err := l.db.Query("SELECT id, space_id, cidr FROM ipam_subnets")
	if err != nil {
		return nil, fmt.Errorf("list IPAM subnets: %w", err)
	}
	type candidate struct {
		id, spaceID, cidr string
		ones              int
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.spaceID, &c.cidr); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var best *candidate
	for i := range candidates {
		_, ipNet, err := net.ParseCIDR(strings.TrimSpace(candidates[i].cidr))
		if err != nil || !ipNet.Contains(parsed) {
			continue
		}
		ones, _ := ipNet.Mask.Size()
		if best == nil || ones > best.ones {
			candidates[i].ones = ones
			best = &candidates[i]
		}
	}
	if best == nil {
		return nil, nil
	}
	return &subnetLookup{ID: best.id, SpaceID: best.spaceID, CIDR: best.cidr}, nil
}

type subnetLookup struct {
	ID      string
	SpaceID string
	CIDR    string
}

// CheckDependencies reports what would break if a subnet were deleted.
func (l *Linkage) CheckDependencies(subnetID string) ([]subnet.Dependency, error) {
	return l.subnetMgr.CheckDependencies(subnetID)
}

// LinkDNSRecord records that a DNS record publishes an address.
//
// The address is located by its IP, so the caller does not have to know which
// subnet it belongs to. When the address has no IPAM row the link is skipped
// with a debug log: a name that resolves is not a reason to fail, and creating
// a row from the DNS side would invent an allocation nobody made.
func (l *Linkage) LinkDNSRecord(recordID, zoneID, name, recordType, value, ip, source string) error {
	a, err := l.addressByIP(ip)
	if err != nil {
		return err
	}
	if a == nil {
		slog.Debug("ipam: DNS record points at an untracked address", "ip", ip, "name", name)
		return nil
	}
	return l.addrMgr.LinkDNSRecord(address.DNSLink{
		AddressID:  a.ID,
		RecordID:   recordID,
		ZoneID:     zoneID,
		RecordName: name,
		RecordType: recordType,
		Value:      value,
		Source:     source,
	})
}

// UnlinkDNSRecord removes every link to a record.
func (l *Linkage) UnlinkDNSRecord(recordID string) error {
	return l.addrMgr.UnlinkDNSRecordByRecordID(recordID)
}

// addressByIP finds the address row for an IP across all spaces.
func (l *Linkage) addressByIP(ip string) (*address.Address, error) {
	canonical, err := address.NormalizeIP(ip)
	if err != nil {
		return nil, err
	}
	rows, err := l.db.Query(`SELECT space_id FROM ipam_addresses WHERE ip_address = ? LIMIT 2`, canonical)
	if err != nil {
		return nil, fmt.Errorf("locate address: %w", err)
	}
	var spaceIDs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return nil, err
		}
		spaceIDs = append(spaceIDs, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(spaceIDs) == 0 {
		return nil, nil
	}
	return l.addrMgr.GetAddressBySpaceIP(spaceIDs[0], canonical)
}

// --- 360° view -------------------------------------------------------------

// LeaseSummary is a DHCP lease as it appears in an address view.
type LeaseSummary struct {
	ID         string `json:"id"`
	ScopeID    string `json:"scope_id"`
	MACAddress string `json:"mac_address"`
	Hostname   string `json:"hostname,omitempty"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
	LeaseStart string `json:"lease_start"`
	LeaseEnd   string `json:"lease_end"`
}

// ReservationSummary is a DHCP reservation as it appears in an address view.
type ReservationSummary struct {
	ID         string `json:"id"`
	ScopeID    string `json:"scope_id"`
	MACAddress string `json:"mac_address"`
	Hostname   string `json:"hostname,omitempty"`
	Enabled    bool   `json:"enabled"`
}

// PublishingRecord is a DNS record that points at one address.
//
// It carries the record's own columns rather than a link row's, because it is
// read from `dns_records` at the moment the question is asked. There is no
// stored link table behind it, and the difference is deliberate -- see
// publishingRecordsForIP.
type PublishingRecord struct {
	ID       string `json:"id"`
	ZoneID   string `json:"zone_id,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Enabled  bool   `json:"enabled"`
	Comment  string `json:"comment,omitempty"`
	OwnerRef string `json:"owner_ref,omitempty"`

	// AuthoredLocally is false for a record an operator configured and true for
	// one a data plane wrote (a dynamic update, or the DHCP-to-DNS bridge). The
	// distinction matters in this view: a locally authored row reaches the
	// control database by an upward push that lags, so a name that is listed
	// here may already have been withdrawn in the plane that answers.
	AuthoredLocally bool `json:"authored_locally"`

	// Direction is "forward" for an A/AAAA record whose value is this address,
	// and "reverse" for the PTR record whose name is its reverse name. A caller
	// that wants only one of the two no longer has to infer it from the type.
	Direction string `json:"direction"`
}

// DNS record types that publish an address, and the two directions they can do
// it in. A PTR holds the address in its *name*; an A/AAAA holds it in its
// value. That asymmetry is why this lookup is two queries rather than one.
const (
	publishingDirectionForward = "forward"
	publishingDirectionReverse = "reverse"
)

// maxPublishingRecords bounds one address's list.
//
// An address legitimately carries a handful of names -- an A record, maybe an
// alias or two, and its PTR. A hundred is far past anything an operator
// intended and keeps a pathological table from turning one detail request into
// an unbounded response.
const maxPublishingRecords = 100

// ScopeSummary is a DHCP scope as it appears in an address view.
//
// The three booleans are the reason this is in the view at all. "This address
// belongs to scope X" is not one question but three, and they disagree exactly
// when something is wrong:
//
//   - InSubnet: the address is inside the scope's declared subnet.
//   - InPool: the address falls between start_ip and end_ip, so the allocator
//     would hand it out.
//   - IsGateway: the address is the scope's router.
type ScopeSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subnet  string `json:"subnet"`
	StartIP string `json:"start_ip"`
	EndIP   string `json:"end_ip"`
	Router  string `json:"router,omitempty"`
	Enabled bool   `json:"enabled"`

	InSubnet  bool `json:"in_subnet"`
	InPool    bool `json:"in_pool"`
	IsGateway bool `json:"is_gateway"`
}

// maxScopesInView bounds the scopes one address view will consider.
//
// A deployment has as many scopes as it has segments -- tens, not thousands.
// The bound is here so a pathological table cannot turn a detail request into
// an unbounded response, and it is reported by omission rather than silently:
// `scopes_truncated` on the view says the list is partial.
const maxScopesInView = 200

// AddressView is everything the system knows about one address.
//
// The point of it is that the three subsystems can be compared without
// cross-referencing three screens: what IPAM decided, what DNS publishes, what
// DHCP is doing, and where those disagree.
type AddressView struct {
	Address *address.Address `json:"address"`
	Subnet  *subnet.Subnet   `json:"subnet,omitempty"`
	Space   *space.Space     `json:"space,omitempty"`

	DNSRecords       []PublishingRecord     `json:"dns_records"`
	DHCPScopes       []ScopeSummary         `json:"dhcp_scopes"`
	DHCPLeases       []LeaseSummary         `json:"dhcp_leases"`
	DHCPReservations []ReservationSummary   `json:"dhcp_reservations"`
	History          []address.HistoryEntry `json:"history"`

	// ScopesTruncated says DHCPScopes is a partial list because the scope table
	// is larger than the view will read in one request. A caller that treats an
	// absence from DHCPScopes as "no scope claims this address" would otherwise
	// read a bound as a fact.
	ScopesTruncated bool `json:"scopes_truncated"`

	// Conflicts lists places where the subsystems disagree. It is computed
	// rather than stored, so it cannot go stale.
	Conflicts []string `json:"conflicts"`
}

// ViewAddress builds the 360° view for an address identified by space and IP.
func (l *Linkage) ViewAddress(spaceID, ip string) (*AddressView, error) {
	canonical, err := address.NormalizeIP(ip)
	if err != nil {
		return nil, err
	}

	a, err := l.addrMgr.GetAddressBySpaceIP(spaceID, canonical)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("%w: %s", address.ErrAddressNotFound, canonical)
	}

	view := &AddressView{Address: a}

	if sp, err := l.spaceMgr.GetSpace(a.SpaceID); err == nil {
		view.Space = sp
	}
	if sn, err := l.subnetMgr.GetSubnet(a.SubnetID); err == nil {
		view.Subnet = sn
	}

	links, err := l.publishingRecordsForIP(canonical)
	if err != nil {
		return nil, err
	}
	view.DNSRecords = nonNil(links)

	scopes, truncated, err := l.scopesForIP(canonical)
	if err != nil {
		return nil, err
	}
	view.DHCPScopes = nonNil(scopes)
	view.ScopesTruncated = truncated

	leases, err := l.leasesForIP(canonical)
	if err != nil {
		return nil, err
	}
	view.DHCPLeases = nonNil(leases)

	reservations, err := l.reservationsForIP(canonical)
	if err != nil {
		return nil, err
	}
	view.DHCPReservations = nonNil(reservations)

	history, err := l.addrMgr.HistoryFor(a.SpaceID, canonical, 100)
	if err != nil {
		return nil, err
	}
	view.History = nonNil(history)

	view.Conflicts = nonNil(detectConflicts(a, links, scopes, leases, reservations))
	return view, nil
}

// publishingRecordsForIP finds the DNS records that point at one address.
//
// It reads `dns_records` directly instead of an `ipam_dns_links` row, and that
// is the whole design. The link table was populated by exactly one writer --
// `Linkage.LinkDNSRecord` -- which no runtime path ever called, so in a real
// deployment it held nothing and this view's answer to "which names publish
// this address" was always the empty list. It was not merely stale: a table
// with no writer cannot be kept in step by fixing its readers.
//
// Deriving at read time has no writer to forget, no invalidator to lose, and no
// drift class to test for. What it costs is one index (migration 025) and the
// fact that it reports the control database's copy of a record rather than the
// data plane's; that copy can lag, which is why each record carries
// AuthoredLocally and why the view's other halves already accept the same lag
// (the leases below are a replica too).
//
// The two directions are separate queries because a name can hold the address
// in two different places: an A/AAAA record carries it in `value`, while the
// PTR that reverses it carries it in `name`. A single `WHERE value = ?` would
// silently omit every reverse record.
func (l *Linkage) publishingRecordsForIP(ip string) ([]PublishingRecord, error) {
	out := make([]PublishingRecord, 0, 4)

	forward, err := l.publishingRecords(
		`WHERE r.type IN ('A', 'AAAA') AND r.value = ?`, ip, publishingDirectionForward)
	if err != nil {
		return nil, err
	}
	out = append(out, forward...)

	// An IPv4 address has a reverse name; an IPv6 one would need the nibble
	// form, which ReverseIP refuses to invent (DHCPv6 is out of scope this
	// release). An empty reverse name means "ask nothing", not "no PTR": a
	// query with an empty name would match records whose name is empty, which
	// is a different and wrong answer.
	if reverse := dnsutil.ReverseIP(ip); reverse != "" {
		backward, err := l.publishingRecords(
			`WHERE r.type = 'PTR' AND r.name = ?`, reverse, publishingDirectionReverse)
		if err != nil {
			return nil, err
		}
		out = append(out, backward...)
	}

	return out, nil
}

// PublishingRecordsForAddressID answers the same question as
// publishingRecordsForIP for a caller holding an address id.
func (l *Linkage) PublishingRecordsForAddressID(addressID string) ([]PublishingRecord, error) {
	a, err := l.addrMgr.GetAddress(addressID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("%w: %s", address.ErrAddressNotFound, addressID)
	}
	return l.publishingRecordsForIP(a.IPAddress)
}

// scopesForIP lists the DHCP scopes that claim an address, and says how.
//
// The comparison is done in Go rather than SQL, and that is not a preference:
// `start_ip` and `end_ip` are text, so `WHERE '192.0.2.9' BETWEEN start_ip AND
// end_ip` compares strings, where "9" sorts after "10". SQLite has no address
// type to cast to. The scope table is configuration scale, so reading it and
// deciding numerically costs nothing and cannot be wrong in that particular way.
//
// The returned flag is true when the table held more rows than the view reads;
// a caller must not read the absence of a scope from a truncated list as proof
// that none claims the address.
func (l *Linkage) scopesForIP(ip string) ([]ScopeSummary, bool, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, false, nil
	}

	all, truncated, err := l.readScopes()
	if err != nil {
		return nil, false, err
	}

	var out []ScopeSummary
	for _, s := range all {
		s.InSubnet = cidrContains(s.Subnet, parsed)
		s.InPool = ipInRange(parsed, s.StartIP, s.EndIP)
		s.IsGateway = sameIP(parsed, s.Router)

		// A scope is listed when either question is true, not only when both
		// are. A pool that reaches outside its own subnet is a misconfiguration
		// worth seeing, and requiring both would hide exactly that case.
		if s.InSubnet || s.InPool || s.IsGateway {
			out = append(out, s)
		}
	}
	return out, truncated, nil
}

// readScopes reads the scope table, bounded.
//
// The whole table is materialised before it is returned. SQLite is capped at
// one connection, so a caller that issued another query while this cursor was
// open would wait for the connection this query holds -- with no error and no
// timeout. Returning a slice rather than rows is what makes that impossible to
// get wrong.
//
// The version-in-view booleans (InSubnet/InPool/IsGateway) are left false: they
// are a question asked by a caller with a particular address in mind.
func (l *Linkage) readScopes() ([]ScopeSummary, bool, error) {
	rows, err := l.db.Query(`
		SELECT id, name, subnet, start_ip, end_ip, COALESCE(router, ''), COALESCE(enabled, 1)
		FROM dhcp_scopes
		ORDER BY name
		LIMIT ?`, maxScopesInView+1)
	if err != nil {
		return nil, false, fmt.Errorf("query dhcp scopes: %w", err)
	}
	defer rows.Close()

	var out []ScopeSummary
	total := 0
	for rows.Next() {
		total++
		var s ScopeSummary
		var enabled any
		if err := rows.Scan(&s.ID, &s.Name, &s.Subnet, &s.StartIP, &s.EndIP,
			&s.Router, &enabled); err != nil {
			return nil, false, err
		}
		s.Enabled = truthy(enabled)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return out, total > maxScopesInView, nil
}

// cidrContains reports whether an address falls inside a CIDR. A malformed CIDR
// contains nothing, which is the right answer for a scope nobody can serve.
func cidrContains(cidr string, ip net.IP) bool {
	_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return false
	}
	return network.Contains(ip)
}

// ipInRange reports whether an address falls between two inclusive bounds.
//
// It refuses to compare across address families: a byte comparison between an
// IPv4 address (which ParseIP stores as a 4-in-6 form) and a real IPv6 address
// would produce an answer, and the answer would be meaningless. An empty or
// unparseable bound is likewise not "everything".
func ipInRange(ip net.IP, start, end string) bool {
	lo := net.ParseIP(strings.TrimSpace(start))
	hi := net.ParseIP(strings.TrimSpace(end))
	if lo == nil || hi == nil {
		return false
	}
	if (ip.To4() == nil) != (lo.To4() == nil) || (ip.To4() == nil) != (hi.To4() == nil) {
		return false
	}
	return bytes.Compare(ip, lo) >= 0 && bytes.Compare(ip, hi) <= 0
}

// sameIP reports whether two addresses are the same one.
func sameIP(a net.IP, b string) bool {
	other := net.ParseIP(strings.TrimSpace(b))
	return other != nil && a.Equal(other)
}

// publishingRecords runs one direction of the lookup. `filter` is a SQL
// fragment with exactly one placeholder, chosen from the constants inside
// publishingRecordsForIP -- it is never built from caller input.
func (l *Linkage) publishingRecords(filter, arg, direction string) ([]PublishingRecord, error) {
	rows, err := l.db.Query(`
		SELECT r.id, COALESCE(r.zone_id, ''), r.name, r.type, r.value,
		       COALESCE(r.ttl, 0), COALESCE(r.enabled, 1),
		       COALESCE(r.comment, ''), COALESCE(r.owner_ref, ''),
		       COALESCE(r.authored_locally, 0)
		FROM dns_records r `+filter+`
		ORDER BY r.name
		LIMIT ?`, arg, maxPublishingRecords)
	if err != nil {
		return nil, fmt.Errorf("query publishing records: %w", err)
	}
	defer rows.Close()

	var out []PublishingRecord
	for rows.Next() {
		var p PublishingRecord
		var enabled, authoredLocally any
		if err := rows.Scan(&p.ID, &p.ZoneID, &p.Name, &p.Type, &p.Value,
			&p.TTL, &enabled, &p.Comment, &p.OwnerRef, &authoredLocally); err != nil {
			return nil, err
		}
		p.Enabled = truthy(enabled)
		p.AuthoredLocally = truthy(authoredLocally)
		p.Direction = direction
		out = append(out, p)
	}
	return out, rows.Err()
}

func (l *Linkage) leasesForIP(ip string) ([]LeaseSummary, error) {
	rows, err := l.db.Query(`
		SELECT id, scope_id, mac_address, COALESCE(hostname, ''), status,
		       COALESCE(generation, 0), lease_start, lease_end
		FROM dhcp_leases WHERE ip_address = ?
		ORDER BY lease_end DESC LIMIT 20`, ip)
	if err != nil {
		return nil, fmt.Errorf("query leases: %w", err)
	}
	defer rows.Close()

	var out []LeaseSummary
	for rows.Next() {
		var s LeaseSummary
		if err := rows.Scan(&s.ID, &s.ScopeID, &s.MACAddress, &s.Hostname, &s.Status,
			&s.Generation, &s.LeaseStart, &s.LeaseEnd); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (l *Linkage) reservationsForIP(ip string) ([]ReservationSummary, error) {
	rows, err := l.db.Query(`
		SELECT id, scope_id, mac_address, COALESCE(hostname, ''), COALESCE(enabled, 1)
		FROM dhcp_reservations WHERE ip_address = ? LIMIT 20`, ip)
	if err != nil {
		return nil, fmt.Errorf("query reservations: %w", err)
	}
	defer rows.Close()

	var out []ReservationSummary
	for rows.Next() {
		var s ReservationSummary
		var enabled any
		if err := rows.Scan(&s.ID, &s.ScopeID, &s.MACAddress, &s.Hostname, &enabled); err != nil {
			return nil, err
		}
		s.Enabled = truthy(enabled)
		out = append(out, s)
	}
	return out, rows.Err()
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case int64:
		return t != 0
	case int:
		return t != 0
	case []byte:
		return len(t) > 0 && t[0] != '0' && t[0] != 'f'
	case string:
		return t != "" && t != "0" && strings.ToLower(t) != "false"
	case nil:
		return false
	default:
		return false
	}
}

// detectConflicts compares the three views of one address.
//
// Every check here is a real disagreement, not a heuristic: the address is
// fenced off yet handed out, or two subsystems name it differently.
//
// `publishers` is what DNS actually has for this address, derived rather than
// looked up in a link table. Until that derivation existed, the two checks
// below it saw an always-empty list and reported "allocated address has no DNS
// name" for every allocated address in the system -- a conflict that was a
// property of the view, not of the deployment.
func detectConflicts(a *address.Address, publishers []PublishingRecord,
	scopes []ScopeSummary, leases []LeaseSummary, reservations []ReservationSummary) []string {

	var conflicts []string

	held := make([]LeaseSummary, 0, len(leases))
	for _, lz := range leases {
		for _, s := range heldLeaseStatuses {
			if lz.Status == s {
				held = append(held, lz)
				break
			}
		}
	}

	if a.Status == address.StatusConflict {
		conflicts = append(conflicts, "status is conflict: two parties claim this address")
	}
	if a.ObservedState == address.ObservedContested {
		conflicts = append(conflicts, "a client reported this address was already in use by another device")
	}
	if len(held) > 1 {
		conflicts = append(conflicts, fmt.Sprintf(
			"%d DHCP leases hold this address at once (%s)",
			len(held), leaseIDs(held)))
	}
	if len(held) > 0 {
		switch a.Status {
		case address.StatusReserved, address.StatusExcluded, address.StatusGateway:
			conflicts = append(conflicts, fmt.Sprintf(
				"address is %s but DHCP is holding a lease on it", a.Status))
		case address.StatusAvailable:
			conflicts = append(conflicts, "address is available but DHCP is holding a lease on it")
		}
	}
	if a.Status == address.StatusDHCP && len(held) == 0 {
		conflicts = append(conflicts, "address is allocated to DHCP but no lease holds it")
	}
	if a.ObservedState == address.ObservedInUse && len(held) == 0 &&
		a.ObservedSource == address.SourceDHCP {
		conflicts = append(conflicts, "last DHCP observation says in use but no lease remains")
	}
	if len(reservations) > 0 && a.Status == address.StatusAvailable {
		conflicts = append(conflicts, "address is available but a DHCP reservation claims it")
	}
	if len(reservations) > 0 && len(held) > 0 {
		for _, lz := range held {
			for _, r := range reservations {
				if r.MACAddress != "" && lz.MACAddress != "" && r.MACAddress != lz.MACAddress {
					conflicts = append(conflicts, fmt.Sprintf(
						"reservation is for %s but the lease is held by %s", r.MACAddress, lz.MACAddress))
				}
			}
		}
	}
	if a.Status == address.StatusStatic && len(held) > 0 {
		conflicts = append(conflicts, "address is configured static but a DHCP lease holds it")
	}

	// A pool that reaches an address IPAM has fenced off.
	//
	// This is a real disagreement rather than a warning because the two sides
	// never speak: the allocator's availability query looks at dhcp_leases and
	// dhcp_reservations and at nothing else -- a scope carries no exclusion
	// list, and the query does not read ipam_addresses. So a gateway, an
	// excluded or reserved address, or a statically configured one that happens
	// to sit inside the range will be handed to a client, and IPAM's status
	// column is the only place that fact is written down.
	//
	// A disabled scope is skipped: it hands out nothing, so agreeing with it is
	// not required.
	for _, s := range scopes {
		if !s.Enabled || !s.InPool {
			continue
		}
		if address.IsFenced(a.Status) {
			conflicts = append(conflicts, fmt.Sprintf(
				"address is %s but DHCP scope %q would hand it out", a.Status, s.Name))
		}
	}

	// Only names that are actually being answered count. A disabled record
	// publishes nothing, so its presence must not suppress the "no DNS name"
	// finding -- otherwise an operator who disables the last A record would see
	// the address as named right up until they needed the name to resolve.
	enabled := make([]PublishingRecord, 0, len(publishers))
	for _, p := range publishers {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}

	if a.Status != address.StatusAvailable && len(enabled) == 0 {
		conflicts = append(conflicts, "allocated address has no DNS name")
	}
	if len(enabled) > 0 && len(held) == 0 && a.Status == address.StatusDHCP {
		conflicts = append(conflicts, "DHCP allocation has a DNS name but no live lease")
	}

	return conflicts
}

func leaseIDs(leases []LeaseSummary) string {
	ids := make([]string, 0, len(leases))
	for _, l := range leases {
		ids = append(ids, l.ID+"@"+l.ScopeID)
	}
	return strings.Join(ids, ", ")
}

// --- reconciler ------------------------------------------------------------

// ReconcileResult reports what a reconciliation pass changed.
type ReconcileResult struct {
	// Observed counts leases pushed into IPAM that were missing there.
	Observed int
	// Released counts addresses marked free because no lease holds them.
	Released int
	// Scanned counts leases examined.
	Scanned int
}

// Reconcile replays DHCP state into IPAM.
//
// Observations made in the DHCP request path are best effort: they are
// deliberately unable to fail a DHCP reply, so a transient database problem
// leaves IPAM behind. This pass closes that gap the same way the DNS outbox
// reconciler does, by recomputing from the authoritative side rather than
// trusting that every in-line write succeeded.
func (l *Linkage) Reconcile(limit int) (ReconcileResult, error) {
	if limit <= 0 || limit > 10000 {
		limit = 500
	}
	var out ReconcileResult

	rows, err := l.db.Query(`
		SELECT l.id, l.scope_id, l.ip_address, l.mac_address, COALESCE(l.hostname, ''), l.status
		FROM dhcp_leases l
		LEFT JOIN ipam_addresses a ON a.ip_address = l.ip_address
		WHERE l.status IN ('active', 'offered')
		  AND l.ip_address <> ''
		  AND (a.id IS NULL OR a.observed_state <> 'in_use' OR a.dhcp_lease_id IS NULL)
		ORDER BY l.ip_address
		LIMIT ?`, limit)
	if err != nil {
		return out, fmt.Errorf("query leases needing observation: %w", err)
	}
	type pendingLease struct {
		id, scopeID, ip, mac, hostname, status string
	}
	var pending []pendingLease
	for rows.Next() {
		var p pendingLease
		if err := rows.Scan(&p.id, &p.scopeID, &p.ip, &p.mac, &p.hostname, &p.status); err != nil {
			rows.Close()
			return out, err
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()

	out.Scanned = len(pending)
	for _, p := range pending {
		action := LeaseActionBind
		if p.status == "offered" {
			action = LeaseActionRenew
		}
		if err := l.ObserveLease(action, p.id, p.scopeID, p.ip, p.mac, p.hostname); err != nil {
			slog.Warn("ipam: reconcile observation failed", "ip", p.ip, "error", err)
			continue
		}
		out.Observed++
	}

	// Addresses IPAM believes are DHCP-bound but no lease holds them. Only
	// DHCP-sourced observations are cleared: an administrator's manual
	// observation is not this pass's to undo.
	released, err := l.releaseStaleObservations(limit)
	if err != nil {
		return out, err
	}
	out.Released = released
	return out, nil
}

func (l *Linkage) releaseStaleObservations(limit int) (int, error) {
	rows, err := l.db.Query(`
		SELECT a.space_id, a.ip_address
		FROM ipam_addresses a
		WHERE a.observed_state = 'in_use'
		  AND a.observed_source = 'dhcp'
		  AND a.dhcp_lease_id IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM dhcp_leases l
			WHERE l.ip_address = a.ip_address
			  AND l.status IN ('active', 'offered', 'conflict')
		  )
		LIMIT ?`, limit)
	if err != nil {
		return 0, fmt.Errorf("query stale observations: %w", err)
	}
	type stale struct{ spaceID, ip string }
	var stales []stale
	for rows.Next() {
		var s stale
		if err := rows.Scan(&s.spaceID, &s.ip); err != nil {
			rows.Close()
			return 0, err
		}
		stales = append(stales, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	n := 0
	for _, s := range stales {
		if _, err := l.addrMgr.ObserveBySpaceIP(s.spaceID, s.ip, address.Observation{
			State:  address.ObservedFree,
			Source: address.SourceReconciler,
			Actor:  "reconciler",
		}); err != nil {
			slog.Warn("ipam: reconcile release failed", "ip", s.ip, "error", err)
			continue
		}
		n++
	}
	return n, nil
}
