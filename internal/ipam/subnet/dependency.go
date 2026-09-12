package subnet

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

// ErrSubnetHasDependencies means a subnet cannot be deleted yet.
var ErrSubnetHasDependencies = errors.New("subnet has dependencies")

// querier is satisfied by both *sql.DB and *sql.Tx, so the same dependency
// scan can run outside a transaction (to build the error message) and inside
// one (to make the decision). SQLite is capped at a single connection, so the
// in-transaction pass must use the transaction handle: issuing the same query
// on the pool while a transaction is open waits for the connection that
// transaction holds, with no error and no timeout.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Dependency is one thing that breaks if a subnet is deleted.
//
// It carries the id and the name so a refusal can say *what* is in the way
// instead of only that something is. The previous check returned a bare
// "cannot delete subnet with DHCP scope dependencies", which told an operator
// neither which scope nor that eight addresses were still allocated.
type Dependency struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (d Dependency) String() string {
	s := d.Kind + " " + d.Name
	if d.ID != "" {
		s += " (" + d.ID + ")"
	}
	if d.Detail != "" {
		s += ": " + d.Detail
	}
	return s
}

// DependencyError reports that deletion was refused, with the full list.
type DependencyError struct {
	SubnetID string       `json:"subnet_id"`
	CIDR     string       `json:"cidr"`
	Deps     []Dependency `json:"dependencies"`
}

func (e *DependencyError) Error() string {
	parts := make([]string, 0, len(e.Deps))
	for _, d := range e.Deps {
		parts = append(parts, d.String())
	}
	return fmt.Sprintf("%s: subnet %s has %d dependenc%s: %s",
		ErrSubnetHasDependencies, e.CIDR, len(e.Deps),
		plural(len(e.Deps), "y", "ies"), strings.Join(parts, "; "))
}

// Is lets errors.Is(err, ErrSubnetHasDependencies) succeed.
func (e *DependencyError) Is(target error) bool { return target == ErrSubnetHasDependencies }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// MaxListedDependencies caps how many entries of one kind are itemised. Beyond
// this the response carries a count instead, so a subnet with 4000 allocated
// addresses produces a usable error rather than a 4000-line one.
const MaxListedDependencies = 20

// CheckDependencies returns everything that would be broken by deleting the
// subnet. An empty result means the deletion is safe.
func (m *Manager) CheckDependencies(subnetID string) ([]Dependency, error) {
	return checkDependencies(m.db, subnetID)
}

// checkDependencies is the implementation, usable on a pool or a transaction.
func checkDependencies(q querier, subnetID string) ([]Dependency, error) {
	var cidr string
	if err := q.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", subnetID).Scan(&cidr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
		}
		return nil, err
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet CIDR %q: %w", cidr, err)
	}

	scopes, err := dependentScopes(q, ipNet)
	if err != nil {
		return nil, err
	}
	leases, err := dependentLeases(q, scopes)
	if err != nil {
		return nil, err
	}
	zones, err := dependentZones(q, ipNet)
	if err != nil {
		return nil, err
	}
	addresses, err := dependentAddresses(q, subnetID)
	if err != nil {
		return nil, err
	}

	deps := make([]Dependency, 0, len(scopes)+len(leases)+len(zones)+len(addresses))
	deps = append(deps, scopes...)
	deps = append(deps, leases...)
	deps = append(deps, zones...)
	deps = append(deps, addresses...)
	return deps, nil
}

// dependentScopes finds DHCP scopes that overlap the subnet.
//
// Overlap, not string equality. A scope for 10.0.0.0/25 sits inside an IPAM
// subnet 10.0.0.0/24 and depends on it just as much as an exact match does;
// the previous equality check let it through, so the subnet could be deleted
// while a live scope was still handing out addresses from it.
func dependentScopes(q querier, ipNet *net.IPNet) ([]Dependency, error) {
	rows, err := q.Query("SELECT id, name, subnet, start_ip, end_ip FROM dhcp_scopes")
	if err != nil {
		return nil, fmt.Errorf("query DHCP scopes: %w", err)
	}
	type scopeRow struct{ id, name, subnet, startIP, endIP string }
	var all []scopeRow
	for rows.Next() {
		var s scopeRow
		if err := rows.Scan(&s.id, &s.name, &s.subnet, &s.startIP, &s.endIP); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var deps []Dependency
	for _, s := range all {
		reason := ""
		if _, scopeNet, err := net.ParseCIDR(strings.TrimSpace(s.subnet)); err == nil {
			if networksOverlap(ipNet, scopeNet) {
				reason = "scope subnet overlaps"
			}
		} else {
			reason = "scope subnet is not a valid CIDR"
		}
		// A scope whose declared subnet sits outside the IPAM subnet can still
		// hand out addresses inside it when its pool was configured loosely.
		if reason == "" {
			if ipInside(ipNet, s.startIP) || ipInside(ipNet, s.endIP) {
				reason = "scope pool reaches into the subnet"
			}
		}
		if reason == "" {
			continue
		}
		deps = append(deps, Dependency{
			Kind: "dhcp_scope", ID: s.id, Name: s.name,
			Detail: fmt.Sprintf("%s (%s..%s, %s)", reason, s.startIP, s.endIP, s.subnet),
		})
	}
	return deps, nil
}

// dependentLeases counts the leases the dependent scopes are currently holding.
//
// They are reported against the scope rather than as individual entries: a
// subnet with a live scope almost always has leases, and listing them would
// bury the reason to refuse under the consequence of it.
func dependentLeases(q querier, scopes []Dependency) ([]Dependency, error) {
	ids := make([]string, 0, len(scopes))
	for _, d := range scopes {
		if d.Kind == "dhcp_scope" {
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, "active", "offered")

	rows, err := q.Query(
		`SELECT scope_id, COUNT(*) FROM dhcp_leases
		 WHERE scope_id IN (`+placeholders+`) AND status IN (?, ?)
		 GROUP BY scope_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("query leases: %w", err)
	}
	var out []Dependency
	for rows.Next() {
		var scopeID string
		var n int64
		if err := rows.Scan(&scopeID, &n); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, Dependency{
			Kind: "dhcp_lease", ID: scopeID, Name: scopeID,
			Detail: fmt.Sprintf("%d address(es) currently leased", n),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}

// dependentZones finds DNS reverse zones that would be left answering for
// addresses that no longer exist.
//
// All enclosing zones are checked, not only the one matching the subnet's own
// prefix length. A /24 subnet is served by name by a /16 zone just as much as
// by a /24 zone, and only checking the exact prefix missed that.
func dependentZones(q querier, ipNet *net.IPNet) ([]Dependency, error) {
	names := reverseZoneCandidates(ipNet)
	if len(names) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	args := make([]any, 0, len(names))
	for _, n := range names {
		args = append(args, n)
	}

	rows, err := q.Query(`SELECT id, name FROM dns_zones WHERE name IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("query DNS zones: %w", err)
	}
	var out []Dependency
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, Dependency{Kind: "dns_zone", ID: id, Name: name, Detail: "reverse zone"})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return out, nil
}

// dependentAddresses lists the addresses that are not merely available.
func dependentAddresses(q querier, subnetID string) ([]Dependency, error) {
	rows, err := q.Query(`
		SELECT status, COUNT(*) FROM ipam_addresses
		WHERE subnet_id = ? AND status <> 'available'
		GROUP BY status ORDER BY status`, subnetID)
	if err != nil {
		return nil, fmt.Errorf("query allocated addresses: %w", err)
	}
	type counted struct {
		status string
		n      int64
	}
	var counts []counted
	for rows.Next() {
		var c counted
		if err := rows.Scan(&c.status, &c.n); err != nil {
			rows.Close()
			return nil, err
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	out := make([]Dependency, 0, len(counts)+MaxListedDependencies)
	for _, c := range counts {
		out = append(out, Dependency{
			Kind: "address", Name: c.status,
			Detail: fmt.Sprintf("%d address(es) are %s, not available", c.n, c.status),
		})
	}

	// A bounded sample of the addresses themselves, so the operator can see
	// which hosts are at stake without opening a second query.
	rows, err = q.Query(`
		SELECT ip_address, status, COALESCE(hostname, ''), COALESCE(owner, '')
		FROM ipam_addresses
		WHERE subnet_id = ? AND status <> 'available'
		ORDER BY ip_address LIMIT ?`, subnetID, MaxListedDependencies)
	if err != nil {
		return nil, fmt.Errorf("query allocated addresses: %w", err)
	}
	var sample []Dependency
	for rows.Next() {
		var ip, status, hostname, owner string
		if err := rows.Scan(&ip, &status, &hostname, &owner); err != nil {
			rows.Close()
			return nil, err
		}
		detail := status
		if hostname != "" {
			detail += ", " + hostname
		}
		if owner != "" {
			detail += ", owner " + owner
		}
		sample = append(sample, Dependency{Kind: "address_detail", Name: ip, Detail: detail})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	sort.Slice(sample, func(i, j int) bool { return sample[i].Name < sample[j].Name })
	return append(out, sample...), nil
}

// ReverseZoneCandidates returns every in-addr.arpa / ip6.arpa zone name that
// could contain addresses from ipNet, from most specific to least.
func ReverseZoneCandidates(cidr string) []string {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return nil
	}
	return reverseZoneCandidates(ipNet)
}

func reverseZoneCandidates(ipNet *net.IPNet) []string {
	if ipNet == nil || len(ipNet.IP) == 0 || len(ipNet.Mask) == 0 {
		return nil
	}
	ones, bits := ipNet.Mask.Size()
	if bits == 128 {
		return ipv6ReverseCandidates(ipNet, ones)
	}
	if bits != 32 {
		return nil
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return nil
	}
	var out []string
	switch {
	case ones >= 24:
		out = append(out, fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0]))
		fallthrough
	case ones >= 16:
		out = append(out, fmt.Sprintf("%d.%d.in-addr.arpa", ip[1], ip[0]))
		fallthrough
	case ones >= 8:
		out = append(out, fmt.Sprintf("%d.in-addr.arpa", ip[0]))
		fallthrough
	default:
		out = append(out, "in-addr.arpa")
	}
	return out
}

// ipv6ReverseCandidates returns the nibble-aligned ancestors of an IPv6 subnet.
//
// Reverse IPv6 delegation happens on nibble boundaries, so a prefix that is not
// a multiple of 4 rounds up to the next nibble before the name is built.
//
// Each label is one hex character, because one nibble is one hex character of
// the 32-character representation -- not four. Treating them as four-character
// groups produces a name that looks plausible and resolves nothing.
func ipv6ReverseCandidates(ipNet *net.IPNet, ones int) []string {
	if ones < 0 || ones > 128 {
		return nil
	}
	nibbleCount := (ones + 3) / 4
	if nibbleCount == 0 {
		return []string{"ip6.arpa"}
	}
	ip := ipNet.IP.To16()
	if ip == nil {
		return nil
	}
	// %x on a net.IP invokes its String method, so the "hex" would be the
	// ASCII of the textual address rather than its bytes. Converting to a byte
	// slice first is what makes this the address.
	addr := fmt.Sprintf("%032x", []byte(ip))

	var out []string
	for n := nibbleCount; n >= 1; n-- {
		labels := make([]string, 0, n)
		// Most specific first: the least significant nibble of the prefix
		// leads the name.
		for k := n - 1; k >= 0; k-- {
			labels = append(labels, string(addr[k]))
		}
		out = append(out, strings.Join(labels, ".")+".ip6.arpa")
	}
	return append(out, "ip6.arpa")
}

func ipInside(ipNet *net.IPNet, s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	return ip != nil && ipNet.Contains(ip)
}
