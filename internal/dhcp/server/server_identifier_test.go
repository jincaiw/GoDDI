package server

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// TestApplyServerIdentifier locks in the RFC 2131 option-54 behavior: an
// OFFER/ACK built without a server identifier gains one, and a non-IPv4 server
// address does not produce a malformed option.
func TestApplyServerIdentifier(t *testing.T) {
	req, err := dhcpv4.NewDiscovery(net.HardwareAddr{0x02, 0, 0, 0xde, 0xad, 0x65})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := dhcpv4.NewReplyFromRequest(req, dhcpv4.WithMessageType(dhcpv4.MessageTypeOffer))
	if err != nil {
		t.Fatal(err)
	}
	if sid := resp.ServerIdentifier(); sid != nil {
		t.Fatalf("precondition: server identifier already present: %v", sid)
	}
	applyServerIdentifier(resp, net.ParseIP("198.18.0.1"))
	got := resp.ServerIdentifier()
	if got == nil || !got.Equal(net.ParseIP("198.18.0.1")) {
		t.Fatalf("server identifier = %v, want 198.18.0.1", got)
	}

	resp6, err := dhcpv4.NewReplyFromRequest(req, dhcpv4.WithMessageType(dhcpv4.MessageTypeOffer))
	if err != nil {
		t.Fatal(err)
	}
	applyServerIdentifier(resp6, net.ParseIP("2001:db8::1"))
	if sid := resp6.ServerIdentifier(); sid != nil {
		t.Fatalf("unexpected server identifier for IPv6 server address: %v", sid)
	}
}
