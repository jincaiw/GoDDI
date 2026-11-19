package server

import (
	"errors"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func dhcpAckForSendTest(t *testing.T) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(dhcpv4.WithMessageType(dhcpv4.MessageTypeAck),
		dhcpv4.WithHwAddr(net.HardwareAddr{0x02, 0, 0, 0, 0, 1}))
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// The send path is the single choke point for every DHCP reply. A guard that
// denies authorization must leave the socket completely untouched: silence,
// not a NAK and not a partial write.
func TestSendAuthorizationGuardBlocksAllReplies(t *testing.T) {
	s := New(nil, nil, nil)
	conn := &recordingConn{}
	req := dhcpAckForSendTest(t)
	addr := &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	srvIP := net.IPv4(10, 77, 3, 1)

	// No guard installed: historical single-node behaviour is unchanged.
	s.sendResponse(req, req, addr, conn, srvIP)
	if got := conn.count(); got != 1 {
		t.Fatalf("no guard: writes = %d, want 1", got)
	}

	// Guard present and allowing: still exactly one write.
	s.SetSendAuthorizationGuard(func() error { return nil })
	s.sendResponse(req, req, addr, conn, srvIP)
	if got := conn.count(); got != 2 {
		t.Fatalf("allowing guard: writes = %d, want 2", got)
	}

	// Guard denies: no write at all, and no NAK written by this path.
	s.SetSendAuthorizationGuard(func() error { return errors.New("write authority revoked") })
	s.sendResponse(req, req, addr, conn, srvIP)
	if got := conn.count(); got != 2 {
		t.Fatalf("denied guard: writes = %d, want unchanged 2", got)
	}
}

// The check must be per packet, not a one-time gate at startup: a node that
// loses authority after one ACK must not send the next one.
func TestSendAuthorizationIsRecheckedPerPacket(t *testing.T) {
	s := New(nil, nil, nil)
	conn := &recordingConn{}
	req := dhcpAckForSendTest(t)
	addr := &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	srvIP := net.IPv4(10, 77, 3, 1)
	authorized := true
	s.SetSendAuthorizationGuard(func() error {
		if !authorized {
			return errors.New("revoked")
		}
		return nil
	})
	s.sendResponse(req, req, addr, conn, srvIP)
	authorized = false
	s.sendResponse(req, req, addr, conn, srvIP)
	if got := conn.count(); got != 1 {
		t.Fatalf("writes = %d, want 1 (second packet must be withheld)", got)
	}
}
