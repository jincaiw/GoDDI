package ipam

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"

	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

// Where a proposed router came from.
const (
	// RouterFromGateway means an address row marks this address as the
	// subnet's gateway. That is a statement somebody made, not a guess.
	RouterFromGateway = "gateway"
	// RouterFromConvention means no address is marked gateway and the plan
	// fell back to the first usable address. It says so, because a gateway
	// nobody chose is a gateway nobody verified.
	RouterFromConvention = "conventional"
)

// PlanAddress is an address the proposed pool reaches but must not hand out.
type PlanAddress struct {
	IP       string `json:"ip"`
	Status   string `json:"status"`
	Hostname string `json:"hostname,omitempty"`
	Owner    string `json:"owner,omitempty"`
}

// DHCPScopePlan is what would happen if a DHCP scope were created from an IPAM
// subnet: the range, the router, the addresses that must not be handed out,
// the reasons not to create it as proposed, and a fingerprint of the state the
// whole answer was derived from.
//
// The draft scope itself is deliberately not part of this. The API layer
// composes that from the subnet manager, so the defaults a generated scope
// carries keep one author and this type stays free of any dependency on the
// DHCP domain -- which is what lets a plan be computed and checked from
// anywhere the IPAM side is reachable.
type DHCPScopePlan struct {
	SubnetID   string `json:"subnet_id"`
	SubnetName string `json:"subnet_name"`
	SpaceID    string `json:"space_id,omitempty"`
	CIDR       string `json:"cidr"`
	StartIP    string `json:"start_ip"`
	EndIP      string `json:"end_ip"`

	// Router is the address the scope should hand out as the default gateway.
	// RouterSource says whether it was named or assumed; an empty source means
	// no candidate exists at all.
	Router       string `json:"router,omitempty"`
	RouterSource string `json:"router_source,omitempty"`

	// Excluded lists the addresses inside the proposed range that IPAM has
	// fenced off. A DHCP scope has no exclusion list, so this list is the
	// operator's, not the allocator's.
	Excluded []PlanAddress `json:"excluded"`

	// Conflicts are reasons the scope should not be created as proposed.
	Conflicts []string `json:"conflicts"`
	// Warnings are things the operator must know and may accept.
	Warnings []string `json:"warnings"`

	// Fingerprint identifies the state this plan was built from. It is passed
	// back when the scope is created, and a mismatch means the world moved in
	// between and the plan has to be looked at again.
	Fingerprint string `json:"fingerprint"`
}

// PlanDHCPScope builds the plan for creating a DHCP scope from a subnet.
//
// It reads; it never writes. Everything it reports is derived on the spot, so
// there is nothing to invalidate and no cached copy to go stale -- which is
// also why the fingerprint exists rather than a stored plan identifier.
func (l *Linkage) PlanDHCPScope(subnetID string) (*DHCPScopePlan, error) {
	sub, err := l.subnetMgr.GetSubnet(subnetID)
	if err != nil {
		return nil, err
	}

	start, end, err := subnet.UsableHostRange(sub.CIDR)
	if err != nil {
		return nil, err
	}

	plan := &DHCPScopePlan{
		SubnetID:   sub.ID,
		SubnetName: sub.Name,
		SpaceID:    sub.SpaceID,
		CIDR:       sub.CIDR,
		StartIP:    start,
		EndIP:      end,
		Excluded:   []PlanAddress{},
		Conflicts:  []string{},
		Warnings:   []string{},
	}

	if err := l.planRouter(plan); err != nil {
		return nil, err
	}

	excluded, err := l.fencedAddressesInRange(sub.ID, start, end)
	if err != nil {
		return nil, err
	}
	// Wrapped rather than assigned straight, because `fencedAddressesInRange`
	// returns nil when nothing is fenced and this line would then undo the
	// empty slice set at construction -- an initialiser that a later
	// assignment can quietly erase is not a guarantee.
	plan.Excluded = nonNil(excluded)
	if len(excluded) > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"the proposed range contains %d address(es) IPAM has fenced off. A DHCP scope "+
				"carries no exclusion list, so the allocator will still offer them; ping check "+
				"is the only thing that stops it in practice.",
			len(excluded)))
	}
	if plan.RouterSource == RouterFromGateway && ipStringInRange(plan.Router, start, end) {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"the gateway %s is inside the proposed range and the scope cannot exclude it",
			plan.Router))
	}

	all, truncated, err := l.readScopes()
	if err != nil {
		return nil, err
	}
	if truncated {
		// A bound read as an absence is how an overlapping scope goes
		// unnoticed, so this is reported where it changes the decision.
		plan.Conflicts = append(plan.Conflicts, fmt.Sprintf(
			"the DHCP scope table is larger than the %d rows a plan scans, so a scope that "+
				"overlaps this one may exist and would not appear here", maxScopesInView))
	}

	relevant := relevantScopes(all, sub.CIDR, start, end)
	for _, s := range relevant {
		switch {
		case rangesOverlap(s.StartIP, s.EndIP, start, end) && s.Enabled:
			plan.Conflicts = append(plan.Conflicts, fmt.Sprintf(
				"scope %q already hands out %s..%s, which overlaps the proposed %s..%s",
				s.Name, s.StartIP, s.EndIP, start, end))
		case rangesOverlap(s.StartIP, s.EndIP, start, end) && !s.Enabled:
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"scope %q is disabled but its pool %s..%s overlaps the proposed range; "+
					"enabling it later would put two scopes on the same addresses",
				s.Name, s.StartIP, s.EndIP))
		case cidrsEqual(s.Subnet, sub.CIDR):
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"scope %q already serves subnet %s with a pool that does not overlap this one "+
					"(%s..%s); two scopes on one subnet is unusual and worth checking",
				s.Name, s.Subnet, s.StartIP, s.EndIP))
		}
	}

	plan.Fingerprint = plan.fingerprint(relevant)
	return plan, nil
}

// CheckDHCPScopePlan verifies that a fingerprint still describes the state the
// plan was built from.
//
// It recomputes instead of comparing against a stored copy: the only way to
// know the world has not moved is to look at it again, and a stored plan is
// one more thing that can be missing when it is needed.
func (l *Linkage) CheckDHCPScopePlan(subnetID, fingerprint string) error {
	plan, err := l.PlanDHCPScope(subnetID)
	if err != nil {
		return err
	}
	if plan.Fingerprint != fingerprint {
		// Logged rather than audited: nothing happened, and an audit trail
		// full of "somebody was told their preview was old" buries the entries
		// that describe changes. A steady stream of these is nevertheless a
		// signal that the fingerprint is too sensitive.
		slog.Info("ipam: refused a DHCP scope create against a stale preview",
			"subnet_id", subnetID,
			"previewed", shortFingerprint(fingerprint),
			"current", shortFingerprint(plan.Fingerprint))
		return &PlanStaleError{
			SubnetID: subnetID,
			Expected: fingerprint,
			Actual:   plan.Fingerprint,
		}
	}
	return nil
}

// planRouter decides what to propose as the scope's router.
//
// "Named rather than inferred" is the whole point: a scope generated with no
// router hands clients no default route, and a scope whose router was guessed
// from "the first usable address" hands them the wrong one about as often as
// the right one. So the address row is consulted first and what was done is
// recorded either way.
func (l *Linkage) planRouter(plan *DHCPScopePlan) error {
	marked, err := l.gatewayAddresses(plan.SubnetID)
	if err != nil {
		return err
	}

	switch len(marked) {
	case 0:
		plan.Router = plan.StartIP
		plan.RouterSource = RouterFromConvention
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"no address is marked gateway, so the plan proposes %s (the first usable address) "+
				"as the router. Set the scope's router explicitly if that is not the gateway.",
			plan.StartIP))
	case 1:
		plan.Router = marked[0]
		plan.RouterSource = RouterFromGateway
	default:
		// Deterministic: the first of an ordered list, so the same state
		// always produces the same plan and the same fingerprint.
		plan.Router = marked[0]
		plan.RouterSource = RouterFromGateway
		plan.Conflicts = append(plan.Conflicts, fmt.Sprintf(
			"%d addresses are marked gateway (%s) and a subnet has one router, so which "+
				"address is the gateway is undecided",
			len(marked), strings.Join(marked, ", ")))
	}
	return nil
}

// gatewayAddresses returns the addresses the subnet marks as its gateway.
//
// More than one is possible -- nothing in the schema forbids it -- so the
// caller is handed all of them rather than the first one found, and the plan
// says the choice is ambiguous instead of silently picking one.
func (l *Linkage) gatewayAddresses(subnetID string) ([]string, error) {
	rows, err := l.db.Query(`
		SELECT ip_address FROM ipam_addresses
		WHERE subnet_id = ? AND status = ?
		ORDER BY ip_address
		LIMIT ?`, subnetID, string(address.StatusGateway), maxGatewaysPerSubnet+1)
	if err != nil {
		return nil, fmt.Errorf("query gateway addresses: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// ORDER BY on a text column orders 192.0.2.9 after 192.0.2.10, which
	// decides a different plan on a different day. Sort by address.
	sortIPs(out)
	return out, nil
}

// maxGatewaysPerSubnet bounds the gateway scan. A subnet has one gateway; the
// bound exists so a mislabelled table cannot turn a preview into an unbounded
// read.
const maxGatewaysPerSubnet = 16

// fencedAddressesInRange returns the addresses in the subnet that IPAM has
// fenced off and that the proposed range would reach.
func (l *Linkage) fencedAddressesInRange(subnetID, start, end string) ([]PlanAddress, error) {
	statuses := address.FencedStatuses()
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")
	args := make([]any, 0, len(statuses)+1)
	args = append(args, subnetID)
	for _, s := range statuses {
		args = append(args, string(s))
	}

	rows, err := l.db.Query(`
		SELECT ip_address, status, COALESCE(hostname, ''), COALESCE(owner, '')
		FROM ipam_addresses
		WHERE subnet_id = ? AND status IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("query fenced addresses: %w", err)
	}
	defer rows.Close()

	var out []PlanAddress
	for rows.Next() {
		var a PlanAddress
		if err := rows.Scan(&a.IP, &a.Status, &a.Hostname, &a.Owner); err != nil {
			return nil, err
		}
		// The range test happens here rather than in the query: start_ip and
		// end_ip are text, SQLite has no address type, and a BETWEEN over text
		// puts 192.0.2.9 after 192.0.2.10.
		if !ipStringInRange(a.IP, start, end) {
			continue
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return ipLess(out[i].IP, out[j].IP)
	})
	return out, nil
}

// relevantScopes keeps the scopes a plan for this subnet has any reason to
// care about.
//
// It is a separate function because two things depend on the same answer: what
// the plan reports, and what the fingerprint covers. A scope outside this
// filter is one whose changes cannot alter the plan, and counting it would make
// the fingerprint move for reasons nobody reviewed.
func relevantScopes(scopes []ScopeSummary, cidr, start, end string) []ScopeSummary {
	out := make([]ScopeSummary, 0, len(scopes))
	for _, s := range scopes {
		if rangesOverlap(s.StartIP, s.EndIP, start, end) || cidrsEqual(s.Subnet, cidr) {
			out = append(out, s)
		}
	}
	return out
}

// planFingerprintVersion changes whenever the fingerprinted facts change, so a
// fingerprint produced by an older build cannot be mistaken for a current one.
const planFingerprintVersion = "dhcpscopeplan/1"

// fingerprint identifies the state a plan was derived from.
//
// What it covers is the proposal and the objections to it: identity, range,
// router, the fenced addresses inside the range, and every relevant scope. It
// deliberately does not cover counts that move on their own -- how many
// addresses in the range are currently leased, for instance -- because a check
// that refuses for changes nobody reviewed is a check operators learn to
// bypass.
func (p *DHCPScopePlan) fingerprint(scopes []ScopeSummary) string {
	h := sha256.New()
	line := func(format string, args ...any) {
		fmt.Fprintf(h, format+"\n", args...)
	}

	line("version\t%s", planFingerprintVersion)
	line("subnet\t%s", p.SubnetID)
	line("name\t%s", p.SubnetName)
	line("cidr\t%s", p.CIDR)
	line("range\t%s\t%s", p.StartIP, p.EndIP)
	line("router\t%s\t%s", p.Router, p.RouterSource)

	excluded := make([]string, 0, len(p.Excluded))
	for _, a := range p.Excluded {
		excluded = append(excluded, a.IP+"\t"+a.Status)
	}
	sort.Strings(excluded)
	for _, e := range excluded {
		line("excluded\t%s", e)
	}

	// Every relevant scope, not only the ones the plan objected to: a scope
	// that becomes a conflict, and one that stops being one, both have to
	// invalidate the plan.
	lines := make([]string, 0, len(scopes))
	for _, s := range scopes {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%t",
			s.ID, s.Name, s.StartIP, s.EndIP, s.Enabled))
	}
	sort.Strings(lines)
	for _, s := range lines {
		line("scope\t%s", s)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ErrDHCPScopePlanStale means the plan a caller approved no longer describes
// the current state.
var ErrDHCPScopePlanStale = errors.New("DHCP scope plan is stale")

// PlanStaleError carries both fingerprints so the refusal can say what
// happened without another round trip. It does not carry the fresh plan: the
// caller re-reads it, which is the action the refusal asks for.
type PlanStaleError struct {
	SubnetID string
	Expected string
	Actual   string
}

func (e *PlanStaleError) Error() string {
	return fmt.Sprintf(
		"%s: subnet %s was previewed against %s but the current state is %s; "+
			"preview the plan again and confirm the new one",
		ErrDHCPScopePlanStale, e.SubnetID, shortFingerprint(e.Expected), shortFingerprint(e.Actual))
}

// Is lets errors.Is(err, ErrDHCPScopePlanStale) succeed.
func (e *PlanStaleError) Is(target error) bool { return target == ErrDHCPScopePlanStale }

// shortFingerprint abbreviates a fingerprint for a message a person reads.
func shortFingerprint(fp string) string {
	if len(fp) <= 12 {
		return fp
	}
	return fp[:12]
}

// rangesOverlap reports whether two inclusive address ranges share an address.
//
// Expressed through ipInRange rather than comparing four endpoints here, so
// the numeric comparison and the refusal to compare across address families
// have one author. For closed intervals it is enough to ask whether either
// range's start lies inside the other; if the ranges intersect at all, one of
// those two questions is true.
func rangesOverlap(aStart, aEnd, bStart, bEnd string) bool {
	return ipStringInRange(aStart, bStart, bEnd) || ipStringInRange(bStart, aStart, aEnd)
}

// ipStringInRange is ipInRange for a textual address, which is how addresses
// arrive from the database.
func ipStringInRange(ip, start, end string) bool {
	return ipInRange(net.ParseIP(strings.TrimSpace(ip)), start, end)
}

// cidrsEqual reports whether two CIDRs name the same network, accepting the
// same network written differently (host bits set, different spacing).
func cidrsEqual(a, b string) bool {
	_, an, aerr := net.ParseCIDR(strings.TrimSpace(a))
	_, bn, berr := net.ParseCIDR(strings.TrimSpace(b))
	if aerr != nil || berr != nil {
		return false
	}
	return an.String() == bn.String()
}

// sortIPs orders addresses numerically, falling back to text order for
// anything that does not parse.
func sortIPs(ips []string) {
	sort.Slice(ips, func(i, j int) bool { return ipLess(ips[i], ips[j]) })
}

func ipLess(a, b string) bool {
	pa, pb := net.ParseIP(strings.TrimSpace(a)), net.ParseIP(strings.TrimSpace(b))
	if pa == nil || pb == nil || (pa.To4() == nil) != (pb.To4() == nil) {
		return a < b
	}
	for k := range pa {
		if pa[k] != pb[k] {
			return pa[k] < pb[k]
		}
	}
	return false
}
