package dhcpreply

import (
	"bytes"
	"encoding/binary"
	"math"
	"net/netip"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jincaiw/GoDDI/internal/enterprise/dhcpwire"
)

func ip(s string) netip.Addr { return netip.MustParseAddr(s) }
func request(typ byte, opts ...byte) []byte {
	p := make([]byte, 240)
	p[0] = 1
	p[1] = 1
	p[2] = 6
	binary.BigEndian.PutUint32(p[4:8], 0x12345678)
	binary.BigEndian.PutUint16(p[10:12], 0x0123)
	copy(p[28:34], []byte{1, 2, 3, 4, 5, 6})
	copy(p[236:240], []byte{99, 130, 83, 99})
	p = append(p, 53, 1, typ)
	p = append(p, opts...)
	return append(p, 255, 0, 0)
}
func TestOfferAndRequestAckThirdPartyDecode(t *testing.T) {
	for _, tc := range []struct {
		req, reply byte
		opts       []byte
	}{{dhcpwire.Discover, Offer, nil}, {dhcpwire.Request, Ack, []byte{50, 4, 192, 0, 2, 55}}} {
		raw := request(tc.req, append(tc.opts, 61, 3, 1, 2, 3)...)
		plan := Plan{Type: tc.reply, ServerID: ip("192.0.2.1"), YourIP: ip("192.0.2.55"), LeaseSeconds: 60}
		out, err := Encode(raw, plan)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != MinPayload || out[0] != 2 || binary.BigEndian.Uint32(out[4:8]) != 0x12345678 || binary.BigEndian.Uint16(out[10:12]) != 0x0123 || !bytes.Equal(out[28:34], []byte{1, 2, 3, 4, 5, 6}) {
			t.Fatalf("bad header %x", out[:44])
		}
		msg, err := dhcpv4.FromBytes(out)
		if err != nil {
			t.Fatal(err)
		}
		if byte(msg.MessageType()) != tc.reply || msg.YourIPAddr.String() != "192.0.2.55" || msg.ServerIdentifier().String() != "192.0.2.1" || !bytes.Equal(msg.Options.Get(dhcpv4.OptionClientIdentifier), []byte{1, 2, 3}) || !bytes.Equal(msg.Options.Get(dhcpv4.OptionIPAddressLeaseTime), []byte{0, 0, 0, 60}) {
			t.Fatalf("decoded %+v", msg)
		}
		original := append([]byte(nil), out...)
		raw[28] = 99
		if !bytes.Equal(out, original) {
			t.Fatal("reply aliases request")
		}
	}
}
func TestRelayEchoAndNAKFlags(t *testing.T) {
	relay := []byte{2, 1, 5, 1, 2, 7, 8, 99, 1, 3}
	raw := request(dhcpwire.Request, append([]byte{50, 4, 192, 0, 2, 55, 82, byte(len(relay))}, relay...)...)
	copy(raw[24:28], []byte{192, 0, 2, 2})
	plan := Plan{Type: Nak, ServerID: ip("192.0.2.1"), RelayPayload: append([]byte(nil), relay...)}
	out, err := Encode(raw, plan)
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(out[10:12]) != 0x8123 {
		t.Fatalf("flags %x", out[10:12])
	}
	msg, err := dhcpv4.FromBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(msg.Options.Get(dhcpv4.OptionRelayAgentInformation), relay) || msg.Options.Get(dhcpv4.OptionIPAddressLeaseTime) != nil || !msg.YourIPAddr.IsUnspecified() {
		t.Fatal("bad NAK")
	}
	plan.RelayPayload[0] = 3
	if b, e := Encode(raw, plan); e == nil || b != nil {
		t.Fatal("accepted forged relay")
	}
	plan.RelayPayload = relay
	raw[24] = 0
	raw[25] = 0
	raw[26] = 0
	raw[27] = 0
	if b, e := Encode(raw, plan); e == nil || b != nil {
		t.Fatal("accepted direct option82")
	}
}
func TestInformNoLease(t *testing.T) {
	raw := request(dhcpwire.Inform)
	copy(raw[12:16], []byte{192, 0, 2, 50})
	out, err := Encode(raw, Plan{Type: Ack, ServerID: ip("192.0.2.1")})
	if err != nil {
		t.Fatal(err)
	}
	m, err := dhcpv4.FromBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	if !m.YourIPAddr.IsUnspecified() || m.ClientIPAddr.String() != "192.0.2.50" || m.Options.Get(dhcpv4.OptionIPAddressLeaseTime) != nil || m.Options.Get(dhcpv4.OptionRenewTimeValue) != nil || m.Options.Get(dhcpv4.OptionRebindingTimeValue) != nil {
		t.Fatal("invalid inform ACK")
	}
}
func TestRequestAckMatchesRequestedOrClientAddress(t *testing.T) {
	for _, tc := range []struct {
		name      string
		opts      []byte
		ci, relay bool
	}{
		{"selecting", []byte{50, 4, 192, 0, 2, 55, 54, 4, 192, 0, 2, 1}, false, false},
		{"init-reboot", []byte{50, 4, 192, 0, 2, 55}, false, false},
		{"renew", nil, true, false},
		{"rebind-relay", nil, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := request(dhcpwire.Request, tc.opts...)
			if tc.ci {
				copy(raw[12:16], []byte{192, 0, 2, 55})
			}
			if tc.relay {
				copy(raw[24:28], []byte{192, 0, 2, 2})
			}
			p := Plan{Type: Ack, ServerID: ip("192.0.2.1"), YourIP: ip("192.0.2.55"), LeaseSeconds: 60}
			if b, err := Encode(raw, p); err != nil || b == nil {
				t.Fatalf("correct address: %v", err)
			}
			p.YourIP = ip("192.0.2.56")
			if b, err := Encode(raw, p); err == nil || b != nil {
				t.Fatalf("wrong address accepted: %x %v", b, err)
			}
		})
	}
}

func TestNAKZerosClientAddress(t *testing.T) {
	for _, relay := range []bool{false, true} {
		raw := request(dhcpwire.Request)
		copy(raw[12:16], []byte{192, 0, 2, 55})
		if relay {
			copy(raw[24:28], []byte{192, 0, 2, 2})
		}
		b, err := Encode(raw, Plan{Type: Nak, ServerID: ip("192.0.2.1")})
		if err != nil || !bytes.Equal(b[12:16], []byte{0, 0, 0, 0}) {
			t.Fatalf("NAK ciaddr: %x %v", b, err)
		}
		if relay && binary.BigEndian.Uint16(b[10:12])&0x8000 == 0 {
			t.Fatal("relay NAK lacks broadcast flag")
		}
	}
}

func TestSelectingAnotherServerRejected(t *testing.T) {
	raw := request(dhcpwire.Request, 50, 4, 192, 0, 2, 55, 54, 4, 192, 0, 2, 99)
	for _, typ := range []byte{Ack, Nak} {
		p := Plan{Type: typ, ServerID: ip("192.0.2.1")}
		if typ == Ack {
			p.YourIP = ip("192.0.2.55")
			p.LeaseSeconds = 60
		}
		if b, err := Encode(raw, p); err == nil || b != nil {
			t.Fatalf("replied to other server: %x %v", b, err)
		}
	}
}

func TestInvalidNeverReturnsBytes(t *testing.T) {
	discover := request(dhcpwire.Discover)
	req := request(dhcpwire.Request, 50, 4, 192, 0, 2, 55)
	valid := Plan{Type: Offer, ServerID: ip("192.0.2.1"), YourIP: ip("192.0.2.55"), LeaseSeconds: 10}
	cases := []struct {
		raw  []byte
		plan Plan
	}{{nil, valid}, {make([]byte, dhcpwire.MaxPayload+1), valid}, {discover, Plan{Type: Ack, ServerID: valid.ServerID}}, {req, valid}, {discover, Plan{Type: Offer, ServerID: valid.ServerID, YourIP: valid.YourIP}}, {discover, Plan{Type: Offer, ServerID: valid.ServerID, YourIP: valid.YourIP, LeaseSeconds: math.MaxUint32}}, {discover, Plan{Type: Offer, ServerID: valid.ServerID, YourIP: ip("255.255.255.255"), LeaseSeconds: 1}}, {discover, Plan{Type: Offer, ServerID: valid.ServerID, YourIP: valid.YourIP, LeaseSeconds: 1, RelayPayload: []byte{1, 1, 1}}}, {req, Plan{Type: Nak, ServerID: valid.ServerID, LeaseSeconds: 1}}}
	for i, c := range cases {
		if b, e := Encode(c.raw, c.plan); e == nil || b != nil {
			t.Fatalf("%d output %x err %v", i, b, e)
		}
	}
}
