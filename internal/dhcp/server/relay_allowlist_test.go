package server

import (
	"database/sql"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// recordingConn stands in for the socket replies are written to. Writing is the
// only thing these tests observe from outside -- "the request was dropped" has
// no other trace, because a dropped request is exactly one with no observable
// effect.
type recordingConn struct {
	mu     sync.Mutex
	writes []net.Addr
}

func (c *recordingConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, io.EOF }

func (c *recordingConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, addr)
	return len(p), nil
}

func (c *recordingConn) Close() error                     { return nil }
func (c *recordingConn) LocalAddr() net.Addr              { return &net.UDPAddr{IP: net.IPv4zero, Port: 67} }
func (c *recordingConn) SetDeadline(time.Time) error      { return nil }
func (c *recordingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordingConn) SetWriteDeadline(time.Time) error { return nil }

func (c *recordingConn) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.writes)
}

func countLeases(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM dhcp_leases").Scan(&n); err != nil {
		t.Fatalf("count leases: %v", err)
	}
	return n
}

// relayedDiscover builds a DISCOVER that arrived through a relay whose address
// on the client subnet is giaddr.
func relayedDiscover(t *testing.T, mac net.HardwareAddr, giaddr string) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithGatewayIP(net.ParseIP(giaddr).To4()),
	)
	if err != nil {
		t.Fatalf("build relayed DISCOVER: %v", err)
	}
	return msg
}

// requestFrom builds the packet described by the case: a direct client when
// giaddr is empty, a relayed one otherwise.
func requestFrom(t *testing.T, giaddr string) *dhcpv4.DHCPv4 {
	t.Helper()
	if giaddr == "" {
		return discoverMsg(t, testMAC(1))
	}
	return relayedDiscover(t, testMAC(1), giaddr)
}

// TestTheRelayAllowListIsOpenUntilAnOperatorClosesIt walks the decision itself.
// Only two rows are refusals, and the rows above them are what keeps the
// setting from being a switch that takes a network down: a list says nothing
// about clients that never went through a relay, and an empty list says nothing
// about anything.
func TestTheRelayAllowListIsOpenUntilAnOperatorClosesIt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		relays  []string
		giaddr  string
		trusted bool
	}{
		{name: "no list, direct client", trusted: true},
		{name: "no list, relayed", giaddr: "198.51.100.1", trusted: true},
		{name: "list, direct client", relays: []string{"198.51.100.0/24"}, trusted: true},
		{name: "list, listed relay", relays: []string{"198.51.100.0/24"}, giaddr: "198.51.100.7", trusted: true},
		{name: "list, listed relay in a later entry", relays: []string{"203.0.113.0/24", "198.51.100.0/24"}, giaddr: "198.51.100.7", trusted: true},
		{name: "list, the network address itself", relays: []string{"198.51.100.0/24"}, giaddr: "198.51.100.0", trusted: true},
		{name: "list, the broadcast address", relays: []string{"198.51.100.0/24"}, giaddr: "198.51.100.255", trusted: true},
		{name: "list, unlisted relay", relays: []string{"198.51.100.0/24"}, giaddr: "203.0.113.7", trusted: false},
		{name: "list, a neighbour of the listed network", relays: []string{"198.51.100.0/24"}, giaddr: "198.51.101.7", trusted: false},
		{name: "ipv6 relay network never matches an ipv4 giaddr", relays: []string{"2001:db8::/32"}, giaddr: "198.51.100.7", trusted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{}
			s.SetTrustedRelays(tc.relays)
			got := s.relayIsTrusted(requestFrom(t, tc.giaddr))
			if got != tc.trusted {
				t.Errorf("trusted = %v, want %v (relays %v, giaddr %q)", got, tc.trusted, tc.relays, tc.giaddr)
			}
		})
	}
}

// TestAnUnreadableRelayEntryRefusesRelayedRequestsAndNothingElse pins the
// direction a failed allowlist load must move in. Dropping the entry would
// leave the list wider than the operator wrote; refusing everything, including
// the clients that never touched a relay, would turn a typo in a relay setting
// into an outage on a network that has no relays.
func TestAnUnreadableRelayEntryRefusesRelayedRequestsAndNothingElse(t *testing.T) {
	s := &Server{}
	// The bare address is the likely typo: it means one host, and it is written
	// the way every other list in this program is written.
	s.SetTrustedRelays([]string{"198.51.100.0/24", "10.0.0.5"})

	if s.relayIsTrusted(relayedDiscover(t, testMAC(1), "198.51.100.7")) {
		t.Error("a relay inside a list that could not be read in full was served")
	}
	if !s.relayIsTrusted(discoverMsg(t, testMAC(2))) {
		t.Error("a typo in the relay setting stopped a directly attached client")
	}
}

// TestARelayWeDoNotKnowIsDroppedInSilence is the end-to-end half: the decision
// above, taken on a real message, must leave no reply and no lease behind.
//
// Silence and not a NAK is the whole point. The client behind that relay did
// nothing wrong, and a NAK would tell it to discard a lease that is valid on
// the subnet it actually sits on.
func TestARelayWeDoNotKnowIsDroppedInSilence(t *testing.T) {
	s, db := newDHCPTestServer(t)
	s.SetTrustedRelays([]string{"198.51.100.0/24"})

	conn := &recordingConn{}
	from := &net.UDPAddr{IP: net.ParseIP("203.0.113.7"), Port: 67}
	s.handleMessage(relayedDiscover(t, testMAC(3), "203.0.113.7"), from, conn, "eth0", net.ParseIP(testServerIP))

	if n := conn.count(); n != 0 {
		t.Errorf("the server answered a request from an unlisted relay with %d packet(s)", n)
	}
	if n := countLeases(t, db); n != 0 {
		t.Errorf("a request from an unlisted relay left %d lease(s) behind", n)
	}
}

// TestAListedRelayIsServed is the control for the test above. Without it, a
// server that dropped every relayed request -- because the allowlist check was
// inverted, or because it fired before the message was parsed -- would pass.
func TestAListedRelayIsServed(t *testing.T) {
	s, db := newDHCPTestServer(t)
	s.SetTrustedRelays([]string{"192.0.2.0/24"})

	conn := &recordingConn{}
	from := &net.UDPAddr{IP: net.ParseIP("192.0.2.254"), Port: 67}
	s.handleMessage(relayedDiscover(t, testMAC(4), "192.0.2.254"), from, conn, "eth0", net.ParseIP(testServerIP))

	if n := conn.count(); n == 0 {
		t.Fatal("a request from a listed relay was not answered, so the allowlist proves nothing")
	}
	if n := countLeases(t, db); n == 0 {
		t.Error("a request from a listed relay left no lease behind, so it was not really served")
	}
}
