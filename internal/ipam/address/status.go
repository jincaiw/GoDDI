package address

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// Status is the allocation state of an address: the answer to "who is allowed
// to use this address", decided by an administrator or by an allocation
// request. It is authoritative and is never written as a side effect of
// observing traffic.
//
// This is deliberately separate from ObservedState. The previous version of
// this package had one `status` column written by both the IPAM API and the
// DHCP sync, so a DHCP renewal would overwrite an administrative "reserved"
// and the reservation would silently disappear. Two facts need two columns.
type Status string

const (
	// StatusAvailable is free for allocation. It is the only state from which
	// an address may be handed to a new owner.
	StatusAvailable Status = "available"
	// StatusUsed is allocated without a more specific owner type. It exists
	// for rows written before the more precise states below were used.
	StatusUsed Status = "used"
	// StatusDHCP is allocated to a DHCP client.
	StatusDHCP Status = "dhcp"
	// StatusStatic is allocated as a manually configured address.
	StatusStatic Status = "static"
	// StatusReserved is held back from automatic allocation but not assigned.
	StatusReserved Status = "reserved"
	// StatusGateway marks the subnet's gateway. Structural: it is not an
	// ordinary allocation and must not silently become one.
	StatusGateway Status = "gateway"
	// StatusExcluded is permanently outside the pool. Structural, like
	// StatusGateway.
	StatusExcluded Status = "excluded"
	// StatusConflict is claimed by more than one party. It requires an
	// explicit decision; nothing resolves it automatically.
	StatusConflict Status = "conflict"
	// StatusUnknown was discovered but never classified.
	StatusUnknown Status = "unknown"
)

// allStatuses is the closed set of values accepted from the API. Anything
// outside it is rejected at the boundary rather than stored and interpreted
// later by a switch statement that has no case for it.
var allStatuses = []Status{
	StatusAvailable, StatusUsed, StatusDHCP, StatusStatic,
	StatusReserved, StatusGateway, StatusExcluded, StatusConflict, StatusUnknown,
}

// Statuses returns every valid allocation status.
func Statuses() []Status {
	out := make([]Status, len(allStatuses))
	copy(out, allStatuses)
	return out
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	for _, v := range allStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Allocatable reports whether the address may be handed to a new owner.
//
// Only StatusAvailable qualifies. `unknown` and `conflict` are excluded on
// purpose: an address nobody has classified, or one two parties both claim, is
// the last thing that should be handed out automatically.
func (s Status) Allocatable() bool { return s == StatusAvailable }

// Structural reports whether the status describes a role in the subnet rather
// than an allocation. Structural addresses are not released by the ordinary
// release path, because "give me back the gateway address" is almost never
// what the caller means.
func (s Status) Structural() bool {
	return s == StatusGateway || s == StatusExcluded
}

// fenced is the set of allocation states that express an administrative
// decision to keep an address out of the pool: do not hand this to a client.
//
// It is deliberately narrower than "not allocatable". A row marked dhcp or
// used inside a pool is the normal case -- the pool exists to be handed out,
// and the allocator's own lease table is what stops it handing an address out
// twice. What the allocator cannot see is a decision recorded only in this
// column, so these are the states where IPAM and DHCP disagree in a way that
// ends with a client holding the gateway address.
//
// One list, two readers: the 360° view's conflict report and the DHCP scope
// plan. A second copy of this set is a second chance to omit a state, and the
// omission shows up as an address that is silently handed out.
var fenced = []Status{
	StatusGateway, StatusExcluded, StatusReserved, StatusStatic, StatusConflict,
}

// FencedStatuses returns the states from which an address must not be handed
// to a client, in a stable order.
func FencedStatuses() []Status {
	out := make([]Status, len(fenced))
	copy(out, fenced)
	return out
}

// IsFenced reports whether s means "keep this address out of the pool".
func IsFenced(s Status) bool {
	for _, v := range fenced {
		if s == v {
			return true
		}
	}
	return false
}

// ObservedState is what the data plane reported about an address. It is
// advisory: it describes the world, not the intent, and a change here never
// rewrites Status.
type ObservedState string

const (
	// ObservedUnknown means nothing has reported on this address yet.
	ObservedUnknown ObservedState = "unknown"
	// ObservedInUse means a data plane reports the address as bound to a
	// client right now.
	ObservedInUse ObservedState = "in_use"
	// ObservedFree means the data plane reports the address as unbound. It is
	// weaker evidence than ObservedInUse: a client can be silent and still
	// hold an address, so a free observation never frees an allocation.
	ObservedFree ObservedState = "free"
	// ObservedContested means a client reported the address as already in use
	// by something else -- a DHCP DECLINE, or an address conflict probe. It is
	// evidence that two parties claim the address, which is a different fact
	// from "this client holds it" and must not be recorded as one.
	ObservedContested ObservedState = "contested"
)

// Valid reports whether o is a known observation.
func (o ObservedState) Valid() bool {
	switch o {
	case ObservedUnknown, ObservedInUse, ObservedFree, ObservedContested:
		return true
	}
	return false
}

// Observation sources, recorded so that a status change can be attributed to
// the subsystem that made it.
const (
	SourceAdmin      = "admin"
	SourceDHCP       = "dhcp"
	SourceDNS        = "dns"
	SourceImport     = "import"
	SourceReconciler = "reconciler"
	SourceAPI        = "api"
)

// Errors returned by this package. Callers match on these rather than on
// message text, so an API layer can map them to status codes without parsing.
var (
	// ErrAddressNotFound means no address row matched.
	ErrAddressNotFound = errors.New("address not found")
	// ErrInvalidIP means the supplied address is not a valid IP literal.
	ErrInvalidIP = errors.New("invalid IP address")
	// ErrOutOfSubnet means the address is not inside the subnet's CIDR.
	ErrOutOfSubnet = errors.New("address is not within the subnet")
	// ErrAddressTaken means another allocation already claims the address.
	ErrAddressTaken = errors.New("address is not available")
	// ErrIllegalTransition means a status change is not permitted.
	ErrIllegalTransition = errors.New("illegal status transition")
	// ErrInvalidStatus means a status value is not in the known set.
	ErrInvalidStatus = errors.New("invalid status")
	// ErrPoolExhausted means no allocatable address was found in the subnet.
	ErrPoolExhausted = errors.New("no available IP addresses in subnet")
	// ErrSubnetNotFound means no subnet matched.
	ErrSubnetNotFound = errors.New("subnet not found")
	// ErrSubnetInUse means the subnet still has dependencies.
	ErrSubnetInUse = errors.New("subnet has dependencies")
)

// NormalizeIP canonicalises an IP literal.
//
// This matters more than it looks. `net.IP.String()` collapses the many valid
// spellings of one address into one: "0:0:0:0:0:0:0:1" and "::1" become "::1",
// and the IPv4-mapped form "::ffff:192.0.2.1" becomes "192.0.2.1". Without it
// the unique index on (space_id, ip_address) does not stop the same host being
// allocated twice under two spellings of the same address.
//
// IPv4 with leading zeros ("192.168.001.001") is rejected outright rather than
// reinterpreted: Go treats it as invalid precisely because it was historically
// read as octal, and guessing which base the caller meant is how the same
// string allocates two different addresses on two systems.
func NormalizeIP(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidIP)
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidIP, s)
	}
	return ip.String(), nil
}

// ParseIP normalises s and additionally verifies it falls inside cidr.
// It returns the canonical text, the parsed address and the network.
func ParseIP(s, cidr string) (string, net.IP, *net.IPNet, error) {
	canonical, err := NormalizeIP(s)
	if err != nil {
		return "", nil, nil, err
	}
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return "", nil, nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	ip := net.ParseIP(canonical)
	if !ipNet.Contains(ip) {
		return "", nil, nil, fmt.Errorf("%w: %s is outside %s", ErrOutOfSubnet, canonical, cidr)
	}
	return canonical, ip, ipNet, nil
}

// transitions is the allocation state machine. A pair that is absent is
// refused.
//
// The omissions carry the meaning:
//
//   - Nothing transitions *into* StatusGateway except from StatusUnknown. A
//     gateway is a role an address is created with, not something an address
//     becomes once a client holds it; "dhcp -> gateway" would quietly turn a
//     live lease into the router address.
//   - StatusGateway and StatusExcluded transition only to StatusAvailable,
//     StatusConflict and StatusUnknown. They cannot become StatusDHCP or
//     StatusStatic: that is the failure mode where the address the
//     administrator fenced off is handed to a client anyway.
//   - Everything may reach StatusConflict, because a conflict is something
//     that happens *to* an address rather than a decision an operator makes
//     about it, and refusing to record one would lose the fact.
//   - StatusConflict leaves only by an explicit decision (available,
//     reserved, excluded, used, static). It never lapses back to available on
//     its own, because "two parties claimed it, then one stopped" is not
//     evidence that nobody holds it.
var transitions = map[Status][]Status{
	StatusAvailable: {StatusUsed, StatusDHCP, StatusStatic, StatusReserved, StatusGateway, StatusExcluded, StatusConflict, StatusUnknown},
	StatusUsed:      {StatusAvailable, StatusStatic, StatusReserved, StatusConflict, StatusUnknown},
	StatusDHCP:      {StatusAvailable, StatusUsed, StatusStatic, StatusConflict, StatusUnknown},
	StatusStatic:    {StatusAvailable, StatusUsed, StatusDHCP, StatusConflict, StatusUnknown},
	StatusReserved:  {StatusAvailable, StatusUsed, StatusDHCP, StatusStatic, StatusConflict, StatusUnknown},
	StatusGateway:   {StatusAvailable, StatusConflict, StatusUnknown},
	StatusExcluded:  {StatusAvailable, StatusConflict, StatusUnknown},
	StatusConflict:  {StatusAvailable, StatusUsed, StatusStatic, StatusReserved, StatusExcluded},
	StatusUnknown:   {StatusAvailable, StatusUsed, StatusDHCP, StatusStatic, StatusReserved, StatusGateway, StatusExcluded, StatusConflict},
}

// CanTransition reports whether from -> to is a permitted change.
//
// A transition from a status to itself is not permitted: callers that have
// nothing to change should not be recording a transition at all, and accepting
// it would put no-op rows in the history that look like events.
func CanTransition(from, to Status) bool {
	if from == to {
		return false
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// AllowedTransitions returns the states reachable from s.
func AllowedTransitions(s Status) []Status {
	out := make([]Status, len(transitions[s]))
	copy(out, transitions[s])
	return out
}

// PlanLeaseObservation decides what a DHCP observation implies for the
// allocation status.
//
// The rule is narrow on purpose. A lease tells us the address is bound; it does
// not tell us the address was ours to give out.
//
//   - available / unknown -> dhcp: the address had no owner and now has one.
//   - dhcp / used / static -> unchanged: already allocated; the observation
//     column carries the new evidence.
//   - reserved / excluded / gateway -> conflict: the server handed out an
//     address the administrator had fenced off. Both facts are true and both
//     matter, so the status records the conflict and the history entry records
//     what it was before.
//   - conflict -> unchanged: an observation cannot resolve a conflict.
//
// A contested observation (a client reporting the address is already taken) is
// treated as a conflict for every status a lease could plausibly be handed out
// under, because that is exactly what it is: somebody else is using the
// address we thought we owned.
//
// It returns the status to store, whether the status changed, and a reason to
// write into the history when it did.
func PlanLeaseObservation(current Status, observed ObservedState) (Status, bool, string) {
	switch observed {
	case ObservedInUse:
		switch current {
		case StatusAvailable, StatusUnknown:
			return StatusDHCP, true, "address was unallocated and is bound to a DHCP client"
		case StatusReserved, StatusExcluded, StatusGateway:
			return StatusConflict, true,
				fmt.Sprintf("DHCP handed out an address held as %s", current)
		default:
			return current, false, ""
		}

	case ObservedContested:
		switch current {
		case StatusConflict:
			return current, false, ""
		case StatusExcluded:
			// Already fenced off. Raising it to conflict would lose the fact
			// that it is excluded, which is the stronger statement.
			return current, false, ""
		default:
			return StatusConflict, true,
				"a client reported the address is already in use by another device"
		}

	default:
		return current, false, ""
	}
}
