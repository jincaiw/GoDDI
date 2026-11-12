// Package dhcpingress implements a bounded exact-match scope map. It does not
// authenticate packets, authorize allocation, validate leases, or send replies.
// The receiving layer must verify the actual socket source and destination,
// interface, relay network, anti-spoofing and node ACL before calling Match.
// Option 82 is an untrusted claim, not authentication. A matching relay can
// carry a private copy for echo only after the external network checks.
package dhcpingress

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"

	"github.com/jasonwa/goddi/internal/enterprise/dhcpwire"
)

var ErrInvalid = errors.New("dhcpingress: invalid or ambiguous mapping")
var ErrRejected = errors.New("dhcpingress: ingress rejected")

const MaxEntries = 4096
const MaxIdentifierBytes = 128
const MaxInterfaceBytes = 64

type Target struct{ Tenant, Space, Scope string }

// Rule specifies a direct entry when Relay and GIAddr are both invalid; both
// are mandatory IPv4 unicast addresses for a relay entry. Destination is exact.
type Rule struct {
	Interface   string
	Destination netip.Addr
	Relay       netip.Addr
	GIAddr      netip.Addr
	Target      Target
	// Nil means no condition; non-nil must be 1..255 exact bytes.
	CircuitID, RemoteID []byte
}

// Packet contains metadata obtained from the socket and decoded GIAddr. Source
// must be the actual UDP source, not a client-supplied header. A valid zero
// GIAddr means direct; all other cases require an exact relay tuple.
type Packet struct {
	Interface   string
	Destination netip.Addr
	Source      netip.Addr
	GIAddr      netip.Addr
	// Option82 is the exact option payload, excluding code and length. Nil
	// means absent; a non-nil empty slice is invalid. Suboptions are parsed
	// here, never supplied as independently forgeable caller fields.
	Option82 []byte
}
type key struct {
	iface           string
	dest, relay, gi netip.Addr
}
type compiledRule struct {
	target          Target
	circuit, remote []byte
}
type Map struct{ rules map[key][]compiledRule }

// Result does not prove network identity or grant production authority. Its
// private echo is available only after an exact configured relay match.
type Result struct {
	Target Target
	echo   []byte
}

// EchoOption82 returns an independent copy of the exact accepted payload. Nil
// means no option was present; callers must separately authorize transmission.
func (r Result) EchoOption82() []byte { return bytes.Clone(r.echo) }

func identifier(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':') {
			return false
		}
	}
	return true
}
func destination(a netip.Addr) bool { return a.Is4() && !a.IsUnspecified() && !a.IsMulticast() }
func unicast(a netip.Addr) bool     { return destination(a) && a != netip.MustParseAddr("255.255.255.255") }
func ruleKey(r Rule) (key, error) {
	if !identifier(r.Interface, MaxInterfaceBytes) || !destination(r.Destination) || !identifier(r.Target.Tenant, MaxIdentifierBytes) || !identifier(r.Target.Space, MaxIdentifierBytes) || !identifier(r.Target.Scope, MaxIdentifierBytes) {
		return key{}, ErrInvalid
	}
	if (r.CircuitID != nil && (len(r.CircuitID) == 0 || len(r.CircuitID) > 255)) || (r.RemoteID != nil && (len(r.RemoteID) == 0 || len(r.RemoteID) > 255)) {
		return key{}, ErrInvalid
	}
	// Both suboptions, including their TLV headers, must fit option 82.
	if r.CircuitID != nil && r.RemoteID != nil && len(r.CircuitID)+len(r.RemoteID)+4 > 255 {
		return key{}, ErrInvalid
	}
	if r.CircuitID != nil && len(r.CircuitID)+2 > 255 || r.RemoteID != nil && len(r.RemoteID)+2 > 255 {
		return key{}, ErrInvalid
	}
	if !r.Relay.IsValid() && !r.GIAddr.IsValid() {
		if r.CircuitID != nil || r.RemoteID != nil {
			return key{}, ErrInvalid
		}
		return key{iface: r.Interface, dest: r.Destination}, nil
	}
	if !unicast(r.Relay) || !unicast(r.GIAddr) {
		return key{}, ErrInvalid
	}
	return key{iface: r.Interface, dest: r.Destination, relay: r.Relay, gi: r.GIAddr}, nil
}

// Compile copies each value into an unexported map; potentially overlapping
// conditions are rejected even for the same target. Direct and relay rules cannot share an
// interface and destination: a relay could otherwise set GIAddr to zero and
// match the direct rule. Network ACLs must still prevent source spoofing by
// unconfigured devices. No wildcard, prefix, or priority exists.
func Compile(rules []Rule) (*Map, error) {
	if len(rules) == 0 || len(rules) > MaxEntries {
		return nil, ErrInvalid
	}
	m := &Map{rules: make(map[key][]compiledRule, len(rules))}
	type location struct {
		iface string
		dest  netip.Addr
	}
	modes := make(map[location]bool, len(rules))
	for _, r := range rules {
		k, err := ruleKey(r)
		if err != nil {
			return nil, err
		}
		loc := location{iface: k.iface, dest: k.dest}
		isRelay := k.relay.IsValid()
		if previous, exists := modes[loc]; exists && previous != isRelay {
			return nil, fmt.Errorf("%w: direct and relay share an ingress location", ErrInvalid)
		}
		modes[loc] = isRelay
		candidate := compiledRule{target: r.Target, circuit: bytes.Clone(r.CircuitID), remote: bytes.Clone(r.RemoteID)}
		for _, old := range m.rules[k] {
			if constraintsOverlap(old.circuit, candidate.circuit) && constraintsOverlap(old.remote, candidate.remote) {
				return nil, fmt.Errorf("%w: overlapping ingress conditions", ErrInvalid)
			}
		}
		m.rules[k] = append(m.rules[k], candidate)
	}
	return m, nil
}
func constraintsOverlap(a, b []byte) bool { return a == nil || b == nil || bytes.Equal(a, b) }

// Match maps an externally verified socket tuple. The input bytes must not be
// mutated concurrently; no input backing array is retained after return.
func (m *Map) Match(p Packet) (Result, error) {
	if m == nil || !identifier(p.Interface, MaxInterfaceBytes) || !destination(p.Destination) || !p.Source.Is4() || !p.GIAddr.Is4() {
		return Result{}, ErrRejected
	}
	k := key{iface: p.Interface, dest: p.Destination}
	if p.GIAddr.IsUnspecified() {
		if !p.Source.IsUnspecified() && !unicast(p.Source) {
			return Result{}, ErrRejected
		}
		if p.Option82 != nil {
			return Result{}, ErrRejected
		}
	} else {
		if !unicast(p.GIAddr) || !unicast(p.Source) {
			return Result{}, ErrRejected
		}
		k.gi = p.GIAddr
		k.relay = p.Source
	}
	if len(p.Option82) > 255 {
		return Result{}, ErrRejected
	}
	raw := bytes.Clone(p.Option82)
	var circuit, remote []byte
	if raw != nil {
		parsed, err := dhcpwire.ParseRelayAgent(raw)
		if err != nil {
			return Result{}, ErrRejected
		}
		circuit, remote = parsed.CircuitID, parsed.RemoteID
	}
	for _, r := range m.rules[k] {
		if r.circuit != nil && !bytes.Equal(r.circuit, circuit) || r.remote != nil && !bytes.Equal(r.remote, remote) {
			continue
		}
		return Result{Target: r.target, echo: raw}, nil
	}
	return Result{}, ErrRejected
}
