package dhcpreply

import (
	"bytes"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jincaiw/GoDDI/internal/enterprise/dhcpingress"
	"github.com/jincaiw/GoDDI/internal/enterprise/dhcpwire"
)

// This exercises only in-process composition. The tuple here is fabricated by
// the test; Match cannot prove that a real socket or network supplied it.
func TestRelayDecodeMatchEncodeIntegration(t *testing.T) {
	relay := []byte{99, 2, 0xaa, 0xbb, 2, 2, 0x30, 0x31, 1, 2, 0x10, 0x11, 77, 1, 0xcc}
	clientID := []byte{1, 0xde, 0xad, 0xbe, 0xef}
	options := []byte{50, 4, 192, 0, 2, 55, 54, 4, 192, 0, 2, 1, 61, byte(len(clientID))}
	options = append(options, clientID...)
	options = append(options, 82, byte(len(relay)))
	options = append(options, relay...)
	raw := request(dhcpwire.Request, options...)
	copy(raw[24:28], []byte{198, 51, 100, 1})
	table, err := dhcpingress.Compile([]dhcpingress.Rule{{Interface: "eth0", Destination: ip("192.0.2.1"), Relay: ip("192.0.2.2"), GIAddr: ip("198.51.100.1"), Target: dhcpingress.Target{Tenant: "t1", Space: "s1", Scope: "scope1"}, CircuitID: []byte{0x10, 0x11}, RemoteID: []byte{0x30, 0x31}}})
	if err != nil {
		t.Fatal(err)
	}
	// Only construct a plan after the full ingress match succeeds.
	encode := func(raw []byte, source string) ([]byte, error) {
		msg, e := dhcpwire.Decode(raw)
		if e != nil {
			return nil, e
		}
		var option []byte
		if msg.RelayInfo != nil {
			option = msg.RelayInfo.Raw
		}
		result, e := table.Match(dhcpingress.Packet{Interface: "eth0", Destination: ip("192.0.2.1"), Source: ip(source), GIAddr: msg.GIAddr, Option82: option})
		if e != nil {
			return nil, e
		}
		if result.Target.Scope != "scope1" {
			t.Fatal("incorrect scope")
		}
		return Encode(raw, Plan{Type: Ack, ServerID: ip("192.0.2.1"), YourIP: ip("192.0.2.55"), LeaseSeconds: 60, RelayPayload: result.EchoOption82()})
	}
	out, err := encode(raw, "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := dhcpv4.FromBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reply.Options.Get(dhcpv4.OptionRelayAgentInformation), relay) || !bytes.Equal(reply.Options.Get(dhcpv4.OptionClientIdentifier), clientID) {
		t.Fatal("relay ordering, unknown suboptions or client ID changed")
	}
	if b, e := encode(raw, "192.0.2.3"); e == nil || b != nil {
		t.Fatal("untrusted source reached encoder")
	}
	forged := bytes.Clone(raw)
	i := bytes.Index(forged, relay)
	if i < 0 {
		t.Fatal("test relay not found")
	}
	forged[i+10] ^= 1
	if b, e := encode(forged, "192.0.2.2"); e == nil || b != nil {
		t.Fatal("mismatched circuit reached encoder")
	}
}
