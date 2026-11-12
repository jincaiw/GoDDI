// Package dhcpwire performs bounded, structural DHCPv4 client-message decoding.
// Success confers no address, relay, identity or sending authority. GIAddr and
// CIAddr are claims from the packet, not authenticated ingress information.
// Callers independently bind the client key to a trusted tenant/space/scope,
// enforce protocol and role policy, and authenticate the actual network path.
// Option overload is rejected. Option 82 and every suboption are untrusted
// claims, never sufficient to select a scope, authenticate ingress or send.
package dhcpwire

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/netip"
)

var ErrMalformed = errors.New("dhcpwire: invalid or unsupported client datagram")

const MaxPayload = 4096
const MaxHops = 16
const (
	Discover byte = 1
	Request  byte = 3
	Decline  byte = 4
	Release  byte = 7
	Inform   byte = 8
)
const (
	keyClientID byte = 1
	keyHardware byte = 2
)

// RelayOption is an untrusted claim. Raw is the exact option payload, and
// Suboptions preserve wire order, including unknown codes. Each byte slice is
// independently owned; the caller decides whether ingress permits Option 82.
type RelayOption struct {
	Raw                           []byte
	Suboptions                    []RelaySuboption
	CircuitID, RemoteID           []byte
	CircuitPresent, RemotePresent bool
}
type RelaySuboption struct {
	Code  byte
	Value []byte
}

type Message struct {
	Type byte
	XID  uint32
	// Flags and Broadcast are packet claims, not authorization. The caller
	// must revalidate trusted network, role and binding before sending.
	Flags           uint16
	Broadcast       bool
	HardwareType    byte
	HardwareAddress []byte
	HardwareHex     string
	ClientID        []byte
	RelayInfo       *RelayOption // nil when Option 82 is absent; never authenticated
	// ClientKey uses a leading namespace byte; hardware keys additionally contain
	// hardware type and length. It is not globally unique without an external scope.
	ClientKey             []byte
	CIAddr, GIAddr        netip.Addr
	RequestedIP, ServerID netip.Addr // invalid when absent
	LeaseSeconds          uint32
	HasLeaseSeconds       bool
}

// ParseRelayAgent validates an Option 82 payload without authenticating it.
// Suboptions are nonempty TLVs; circuit/remote codes may each appear once.
// Every returned byte slice is copied independently from the input and peers.
func ParseRelayAgent(raw []byte) (RelayOption, error) {
	if len(raw) == 0 || len(raw) > 255 {
		return RelayOption{}, ErrMalformed
	}
	out := RelayOption{Raw: append([]byte(nil), raw...)}
	for i := 0; i < len(raw); {
		if len(raw)-i < 2 {
			return RelayOption{}, ErrMalformed
		}
		code, n := raw[i], int(raw[i+1])
		i += 2
		if n == 0 || n > len(raw)-i {
			return RelayOption{}, ErrMalformed
		}
		v := raw[i : i+n]
		i += n
		switch code {
		case 1:
			if out.CircuitPresent {
				return RelayOption{}, ErrMalformed
			}
			out.CircuitPresent = true
			out.CircuitID = append([]byte(nil), v...)
		case 2:
			if out.RemotePresent {
				return RelayOption{}, ErrMalformed
			}
			out.RemotePresent = true
			out.RemoteID = append([]byte(nil), v...)
		}
		out.Suboptions = append(out.Suboptions, RelaySuboption{Code: code, Value: append([]byte(nil), v...)})
	}
	return out, nil
}

func addr(raw []byte) netip.Addr { return netip.AddrFrom4([4]byte{raw[0], raw[1], raw[2], raw[3]}) }
func usable(a netip.Addr) bool {
	return a.Is4() && !a.IsUnspecified() && !a.IsMulticast() && a != netip.MustParseAddr("255.255.255.255")
}

// Decode copies all returned byte slices; it retains no reference to raw. It
// accepts only the five supported client types, exact critical option lengths,
// one instance of each critical option and zero padding after a mandatory END.
// A zero XID, invalid Ethernet hlen or unsupported overload fails closed.
// Option 82 is parsed only as an untrusted declaration. Unknown top-level
// options are bounded and skipped, including their duplicates.
func Decode(raw []byte) (Message, error) {
	var m Message
	if len(raw) < 241 || len(raw) > MaxPayload || raw[0] != 1 || raw[1] == 0 || raw[2] == 0 || raw[2] > 16 || (raw[1] == 1 && raw[2] != 6) || raw[3] > MaxHops || binary.BigEndian.Uint32(raw[4:8]) == 0 || raw[236] != 99 || raw[237] != 130 || raw[238] != 83 || raw[239] != 99 {
		return Message{}, ErrMalformed
	}
	m.XID = binary.BigEndian.Uint32(raw[4:8])
	m.Flags = binary.BigEndian.Uint16(raw[10:12])
	m.Broadcast = m.Flags&0x8000 != 0
	m.HardwareType = raw[1]
	m.HardwareAddress = append([]byte(nil), raw[28:28+int(raw[2])]...)
	m.HardwareHex = hex.EncodeToString(m.HardwareAddress)
	m.CIAddr = addr(raw[12:16])
	m.GIAddr = addr(raw[24:28])
	seen := [256]bool{}
	end := false
	for i := 240; i < len(raw); {
		code := raw[i]
		i++
		if code == 0 {
			continue
		}
		if code == 255 {
			for _, b := range raw[i:] {
				if b != 0 {
					return Message{}, ErrMalformed
				}
			}
			end = true
			break
		}
		if i >= len(raw) {
			return Message{}, ErrMalformed
		}
		n := int(raw[i])
		i++
		if n > len(raw)-i {
			return Message{}, ErrMalformed
		}
		v := raw[i : i+n]
		i += n
		switch code {
		case 52:
			return Message{}, ErrMalformed
		case 82:
			if seen[code] {
				return Message{}, ErrMalformed
			}
			seen[code] = true
			relay, err := ParseRelayAgent(v)
			if err != nil {
				return Message{}, err
			}
			m.RelayInfo = &relay
		case 53, 50, 54, 51, 61:
			if seen[code] {
				return Message{}, ErrMalformed
			}
			seen[code] = true
			switch code {
			case 53:
				if n != 1 {
					return Message{}, ErrMalformed
				}
				m.Type = v[0]
			case 50:
				if n != 4 {
					return Message{}, ErrMalformed
				}
				m.RequestedIP = addr(v)
				if !usable(m.RequestedIP) {
					return Message{}, ErrMalformed
				}
			case 54:
				if n != 4 {
					return Message{}, ErrMalformed
				}
				m.ServerID = addr(v)
				if !usable(m.ServerID) {
					return Message{}, ErrMalformed
				}
			case 51:
				if n != 4 {
					return Message{}, ErrMalformed
				}
				m.LeaseSeconds = binary.BigEndian.Uint32(v)
				m.HasLeaseSeconds = true
			case 61:
				if n < 2 {
					return Message{}, ErrMalformed
				}
				m.ClientID = append([]byte(nil), v...)
			}
		}
	}
	if !end || !seen[53] {
		return Message{}, ErrMalformed
	}
	ci := !m.CIAddr.IsUnspecified()
	req := m.RequestedIP.IsValid()
	sid := m.ServerID.IsValid()
	if ci && !usable(m.CIAddr) {
		return Message{}, ErrMalformed
	}
	if !m.GIAddr.IsUnspecified() && !usable(m.GIAddr) {
		return Message{}, ErrMalformed
	}
	switch m.Type {
	case Discover:
		if ci || sid {
			return Message{}, ErrMalformed
		}
	case Request:
		if !((!ci && req) || (ci && !req && !sid)) || (sid && (ci || !req)) {
			return Message{}, ErrMalformed
		}
	case Decline:
		if ci || !req || !sid {
			return Message{}, ErrMalformed
		}
	case Release:
		if !ci || req || !sid {
			return Message{}, ErrMalformed
		}
	case Inform:
		if !ci || req || sid {
			return Message{}, ErrMalformed
		}
	default:
		return Message{}, ErrMalformed
	}
	if len(m.ClientID) > 0 {
		m.ClientKey = append([]byte{keyClientID}, m.ClientID...)
	} else {
		m.ClientKey = append([]byte{keyHardware, m.HardwareType, byte(len(m.HardwareAddress))}, m.HardwareAddress...)
	}
	return m, nil
}
