// Package dhcpreply encodes offline DHCPv4 replies. It does not send, allocate,
// authenticate or authorize. A Plan can be forged, including its relay data;
// equality with a request is not proof of a trusted relay or delivery.
// Before encoding and sending, the caller must establish trusted ingress and
// committed binding, compute the authorized remaining lease, revalidate scope,
// role, fencing and time, and hold the shared synchronous send gate through I/O.
package dhcpreply

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"net/netip"

	"github.com/jincaiw/GoDDI/internal/enterprise/dhcpwire"
)

var ErrInvalid = errors.New("dhcpreply: invalid request or plan")

const (
	Offer byte = 2
	Ack   byte = 5
	Nak   byte = 6
)
const MinPayload = 300
const MaxPayload = dhcpwire.MaxPayload

// Plan holds decisions made externally. RelayPayload is the verified raw Option
// 82 payload (not its code/length) and must exactly match this request. Supplying
// it does not establish trust. No arbitrary options can inject protocol fields.
type Plan struct {
	Type         byte
	ServerID     netip.Addr
	YourIP       netip.Addr
	LeaseSeconds uint32
	RelayPayload []byte
}

func validIP(a netip.Addr) bool {
	return a.Is4() && !a.IsUnspecified() && !a.IsMulticast() && a != netip.MustParseAddr("255.255.255.255")
}

// Encode re-decodes request and returns a new byte slice, or nil on any error.
// The caller is responsible for guarding against request/plan changes and must
// not send a response using a stale authorization or lease calculation.
func Encode(request []byte, plan Plan) ([]byte, error) {
	m, err := dhcpwire.Decode(request)
	if err != nil || !validIP(plan.ServerID) {
		return nil, ErrInvalid
	}
	// A SELECTING request addressed to a different server is never answered.
	if m.Type == dhcpwire.Request && m.ServerID.IsValid() && m.ServerID != plan.ServerID {
		return nil, ErrInvalid
	}
	lease := false
	switch {
	case plan.Type == Offer && m.Type == dhcpwire.Discover:
		lease = true
	case plan.Type == Ack && m.Type == dhcpwire.Request:
		lease = true
	case plan.Type == Ack && m.Type == dhcpwire.Inform:
	case plan.Type == Nak && m.Type == dhcpwire.Request:
	default:
		return nil, ErrInvalid
	}
	if lease {
		if !validIP(plan.YourIP) || plan.LeaseSeconds == 0 || plan.LeaseSeconds == math.MaxUint32 {
			return nil, ErrInvalid
		}
	} else if (plan.YourIP.IsValid() && (!plan.YourIP.Is4() || !plan.YourIP.IsUnspecified())) || plan.LeaseSeconds != 0 {
		return nil, ErrInvalid
	}
	if plan.Type == Ack && m.Type == dhcpwire.Request {
		address := m.RequestedIP
		if !address.IsValid() {
			address = m.CIAddr
		}
		if plan.YourIP != address {
			return nil, ErrInvalid
		}
	}
	if m.RelayInfo != nil {
		if m.GIAddr.IsUnspecified() || !bytes.Equal(plan.RelayPayload, m.RelayInfo.Raw) {
			return nil, ErrInvalid
		}
	} else if plan.RelayPayload != nil {
		return nil, ErrInvalid
	}
	out := make([]byte, 240)
	out[0] = 2
	out[1] = m.HardwareType
	out[2] = byte(len(m.HardwareAddress))
	binary.BigEndian.PutUint32(out[4:8], m.XID)
	flags := m.Flags
	if plan.Type == Nak && !m.GIAddr.IsUnspecified() {
		flags |= 0x8000
	}
	binary.BigEndian.PutUint16(out[10:12], flags)
	if plan.Type != Nak {
		ci := m.CIAddr.As4()
		copy(out[12:16], ci[:])
	}
	gi := m.GIAddr.As4()
	copy(out[24:28], gi[:])
	copy(out[28:44], m.HardwareAddress)
	if lease {
		yi := plan.YourIP.As4()
		copy(out[16:20], yi[:])
	}
	copy(out[236:240], []byte{99, 130, 83, 99})
	out = append(out, 53, 1, plan.Type, 54, 4)
	id := plan.ServerID.As4()
	out = append(out, id[:]...)
	if lease {
		out = append(out, 51, 4, 0, 0, 0, 0)
		binary.BigEndian.PutUint32(out[len(out)-4:], plan.LeaseSeconds)
	}
	if len(m.ClientID) > 0 {
		out = append(out, 61, byte(len(m.ClientID)))
		out = append(out, m.ClientID...)
	}
	if m.RelayInfo != nil {
		out = append(out, 82, byte(len(m.RelayInfo.Raw)))
		out = append(out, m.RelayInfo.Raw...)
	}
	out = append(out, 255)
	if len(out) > MaxPayload {
		return nil, ErrInvalid
	}
	if len(out) < MinPayload {
		out = append(out, make([]byte, MinPayload-len(out))...)
	}
	return out, nil
}
