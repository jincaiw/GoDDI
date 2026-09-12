package server

import (
	"bytes"
	"database/sql"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// Malformed and hostile datagrams.
//
// The v0.6.0 exit criteria name "malformed packets" as one of the protocol
// counter-examples that has to pass. It is the one class where the correct
// behaviour is *nothing at all* -- no reply, no lease, no panic, and a receive
// loop still reading the next packet -- so almost every assertion here is
// negative, and every negative needs a control. A server that answered nobody
// would otherwise look flawless.
//
// Two of these cases are about a datagram the parser accepts and the server
// must still refuse: one that carries no DHCP message type, and one whose
// opcode says it is a reply. The parser cannot help with either -- it has no
// opinion about meaning -- so the refusal is the server's own, and it is the
// server's own that is pinned below.

// wireConn is a stand-in for the socket replies are written to that keeps the
// bytes. recordingConn in relay_allowlist_test.go counts writes; this one is
// for the cases where what was written matters.
type wireConn struct {
	mu      sync.Mutex
	packets [][]byte
}

func (c *wireConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, io.EOF }

func (c *wireConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.packets = append(c.packets, append([]byte(nil), p...))
	return len(p), nil
}

func (c *wireConn) Close() error                     { return nil }
func (c *wireConn) LocalAddr() net.Addr              { return &net.UDPAddr{IP: net.IPv4zero, Port: 67} }
func (c *wireConn) SetDeadline(time.Time) error      { return nil }
func (c *wireConn) SetReadDeadline(time.Time) error  { return nil }
func (c *wireConn) SetWriteDeadline(time.Time) error { return nil }

func (c *wireConn) sent() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.packets...)
}

// rawPacket assembles a BOOTP header with the given option bytes appended, so a
// test can put things on the wire that no constructor in the library will
// produce. Offsets are the fixed BOOTP layout of RFC 951; the magic cookie sits
// at byte 236.
func rawPacket(opcode byte, options []byte) []byte {
	pkt := make([]byte, 240)
	pkt[0] = opcode
	pkt[1] = 1 // htype: ethernet
	pkt[2] = 6 // hlen
	copy(pkt[4:8], []byte{0xde, 0xad, 0xbe, 0xef})
	copy(pkt[28:34], []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01})
	copy(pkt[236:240], []byte{99, 130, 83, 99})
	return append(pkt, options...)
}

// unparseable returns datagrams the library's parser is expected to reject, one
// per way it can reject them. `rawPacket` output is included because a packet
// that is the right length and carries the right magic cookie is exactly the
// shape an attacker would send.
func unparseable() map[string][]byte {
	return map[string][]byte{
		"empty datagram":                        {},
		"shorter than the header":               {0x01, 0x01, 0x06},
		"one byte short of the BOOTP part":      bytes.Repeat([]byte{0x01}, 239),
		"right length, wrong magic cookie":      bytes.Repeat([]byte{0xff}, 300),
		"an option value that runs off the end": rawPacket(1, []byte{53, 5, 1, 1}),
		// Larger than the receive loop's buffer, so the kernel truncates it and
		// what arrives is not what was sent. A hostile sender does not have to
		// stay inside the size we happen to allocate.
		"larger than the receive buffer": bytes.Repeat([]byte{0xff}, 4096),
	}
}

// parseableButMeaningless returns datagrams that survive the parser and that
// the server must nevertheless refuse to act on. They are separated from the
// set above because the parser is not what refuses them.
func parseableButMeaningless() map[string][]byte {
	return map[string][]byte{
		"no message type at all": rawPacket(1, []byte{255}),
		"message type zero":      rawPacket(1, []byte{53, 1, 0, 255}),
		"unknown message type":   rawPacket(1, []byte{53, 1, 200, 255}),
		// OFFER, ACK and NAK are what *we* send. Receiving one means either a
		// forgery or something else's reply that reached our socket.
		"a message type only a server sends (OFFER)": rawPacket(1, []byte{53, 1, 2, 255}),
		"a message type only a server sends (ACK)":   rawPacket(1, []byte{53, 1, 5, 255}),
	}
}

// TestTheReceiveLoopSurvivesDatagramsItCannotParse drives the real loop over a
// real socket. The assertion is not "the garbage was rejected" -- a loop that
// died would reject it too -- but "the loop is still reading afterwards", which
// is observed the only way it can be from outside: the valid DISCOVER that
// follows still gets an address reserved for it.
//
// This is the case that fails if the parse error ever stops being a `continue`.
func TestTheReceiveLoopSurvivesDatagramsItCannotParse(t *testing.T) {
	s, db := newDHCPTestServer(t)

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.wg.Add(1)
	go s.receiveLoop(conn, "eth0", net.ParseIP(testServerIP))
	t.Cleanup(func() {
		close(s.quit)
		s.wg.Wait()
		_ = conn.Close()
	})

	sender, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen (sender): %v", err)
	}
	t.Cleanup(func() { _ = sender.Close() })

	send := func(name string, payload []byte) {
		t.Helper()
		if _, err := sender.WriteTo(payload, conn.LocalAddr()); err != nil {
			t.Fatalf("send %s: %v", name, err)
		}
	}

	for name, payload := range unparseable() {
		send(name, payload)
	}
	for name, payload := range parseableButMeaningless() {
		send(name, payload)
	}

	// Every datagram above carried the same chaddr. If any of them had been
	// acted on, that MAC would have a lease by now.
	time.Sleep(50 * time.Millisecond)
	if n := leasesFor(t, db, testMAC(1).String()); n != 0 {
		t.Fatalf("%d lease(s) came out of datagrams that carry no request", n)
	}

	// The control: one well-formed DISCOVER, sent after all of the above.
	send("valid DISCOVER", mustBytes(t, discoverMsg(t, testMAC(9))))

	waitFor(t, 3*time.Second, "the receive loop to serve a valid DISCOVER after the garbage", func() bool {
		return leasesFor(t, db, testMAC(9).String()) > 0
	})
}

// TestAMessageThatSaysItIsAReplyIsDroppedInSilence covers the one field that
// makes a well-formed packet provably not a request.
//
// RFC 2131 section 3 defines the direction rather than mandating a check: every
// client-to-server message carries BOOTREQUEST, and BOOTREPLY travels only the
// other way. Why it is worth refusing rather than shrugging is in the control
// below -- the answer to such a packet would go out with the wrong opcode.
func TestAMessageThatSaysItIsAReplyIsDroppedInSilence(t *testing.T) {
	s, db := newDHCPTestServer(t)

	asReply := discoverMsg(t, testMAC(2))
	asReply.OpCode = dhcpv4.OpcodeBootReply

	conn := &wireConn{}
	s.handleMessage(asReply, &net.UDPAddr{IP: net.ParseIP("192.0.2.50"), Port: 68}, conn, "eth0", net.ParseIP(testServerIP))

	if n := len(conn.sent()); n != 0 {
		t.Errorf("the server answered a message whose opcode said it was a reply (%d packet(s))", n)
	}
	if n := countLeases(t, db); n != 0 {
		t.Errorf("a message whose opcode said it was a reply left %d lease(s) behind", n)
	}
}

// TestTheAnswerToARequestSaysItIsAReply is the control for the test above, and
// the reason the opcode check is not merely tidiness.
//
// dhcpv4.WithReply derives the answer's opcode from the request's: BOOTREQUEST
// in, BOOTREPLY out -- and anything else in, BOOTREQUEST out. So a server
// without the check does not just act on a strange packet, it replies with a
// packet whose opcode says "this is a request", which no client can accept and
// which reads on the wire as though the server were asking for an address.
func TestTheAnswerToARequestSaysItIsAReply(t *testing.T) {
	s, _ := newDHCPTestServer(t)

	conn := &wireConn{}
	s.handleMessage(discoverMsg(t, testMAC(3)), &net.UDPAddr{IP: net.ParseIP("192.0.2.50"), Port: 68}, conn, "eth0", net.ParseIP(testServerIP))

	sent := conn.sent()
	if len(sent) == 0 {
		t.Fatal("a well-formed DISCOVER was not answered, so this control proves nothing")
	}
	reply, err := dhcpv4.FromBytes(sent[0])
	if err != nil {
		t.Fatalf("the reply does not parse: %v", err)
	}
	if reply.OpCode != dhcpv4.OpcodeBootReply {
		t.Errorf("opcode = %d, want BOOTREPLY (%d)", uint8(reply.OpCode), uint8(dhcpv4.OpcodeBootReply))
	}
	if reply.MessageType() != dhcpv4.MessageTypeOffer {
		t.Errorf("message type = %s, want OFFER", reply.MessageType())
	}
}

// TestAMessageWithNoTypeOrAnUnknownTypeLeavesNothingBehind covers the packets
// the parser accepts and the dispatcher has no case for. They carry the same
// chaddr, so a lease would be the proof that one of them was treated as a
// request.
func TestAMessageWithNoTypeOrAnUnknownTypeLeavesNothingBehind(t *testing.T) {
	for name, payload := range parseableButMeaningless() {
		t.Run(name, func(t *testing.T) {
			s, db := newDHCPTestServer(t)

			msg, err := dhcpv4.FromBytes(payload)
			if err != nil {
				t.Fatalf("this datagram was meant to survive the parser: %v", err)
			}

			conn := &wireConn{}
			s.handleMessage(msg, &net.UDPAddr{IP: net.ParseIP("192.0.2.50"), Port: 68}, conn, "eth0", net.ParseIP(testServerIP))

			if n := len(conn.sent()); n != 0 {
				t.Errorf("%s produced %d reply packet(s)", name, n)
			}
			if n := countLeases(t, db); n != 0 {
				t.Errorf("%s left %d lease(s) behind", name, n)
			}
		})
	}
}

// TestAHostileDatagramDoesNotTakeTheInterfaceDown sends the garbage *through*
// the loop and then asserts the server can still be reached through the same
// interface, which is the property the recover() in handleMessage exists for.
//
// The garbage is a packet that parses and then names a scope whose range is
// unusable -- the shape that reaches deepest into the request path without
// being something the parser can reject on its own.
func TestAHostileDatagramDoesNotTakeTheInterfaceDown(t *testing.T) {
	s, db := newDHCPTestServer(t)

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.wg.Add(1)
	go s.receiveLoop(conn, "eth0", net.ParseIP(testServerIP))
	t.Cleanup(func() {
		close(s.quit)
		s.wg.Wait()
		_ = conn.Close()
	})

	sender, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen (sender): %v", err)
	}
	t.Cleanup(func() { _ = sender.Close() })

	// A REQUEST for an address outside every subnet, then a REQUEST that
	// carries a 255-byte option value, then a RELEASE with no address at all.
	hostile := [][]byte{
		rawPacket(1, []byte{53, 1, 3, 50, 4, 203, 0, 113, 9, 255}),
		rawPacket(1, append([]byte{53, 1, 3, 12, 255}, bytes.Repeat([]byte{0x2f}, 255)...)),
		rawPacket(1, []byte{53, 1, 7, 255}),
	}
	for i, payload := range hostile {
		if _, err := sender.WriteTo(payload, conn.LocalAddr()); err != nil {
			t.Fatalf("send hostile #%d: %v", i, err)
		}
	}

	time.Sleep(50 * time.Millisecond)

	valid := mustBytes(t, discoverMsg(t, testMAC(10)))
	if _, err := sender.WriteTo(valid, conn.LocalAddr()); err != nil {
		t.Fatalf("send valid DISCOVER: %v", err)
	}
	waitFor(t, 3*time.Second, "the loop to keep serving after hostile packets", func() bool {
		return leasesFor(t, db, testMAC(10).String()) > 0
	})
}

// TestTheNonIPv4GuardRefusesWhatTheWireCannotDeliver pins the guard in
// HandleRequest that drops a REQUEST naming an address that is not IPv4.
//
// Honest labelling: the wire cannot reach it today. The option decoder reads an
// address as exactly four bytes and returns nil for anything else, so the value
// this branch tests for cannot arrive in a parsed packet. It is kept because a
// future decoder change, or a caller that builds the struct directly, would
// otherwise silently start NAKing packets it cannot interpret -- and it is
// tested directly for the same reason, rather than left to be discovered.
func TestTheNonIPv4GuardRefusesWhatTheWireCannotDeliver(t *testing.T) {
	s, _ := newDHCPTestServer(t)

	// RENEWING is the state that carries the address in ciaddr, so this is the
	// one path where the guard is reached. The message is built by hand rather
	// than parsed, because no parse can produce this value.
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(testMAC(4)),
	)
	if err != nil {
		t.Fatalf("build REQUEST: %v", err)
	}
	msg.ClientIPAddr = net.ParseIP("2001:db8::1")

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp != nil {
		t.Errorf("a REQUEST naming an IPv6 address was answered with %s", resp.MessageType())
	}
}

// ---- helpers ---------------------------------------------------------------

func mustBytes(t *testing.T, msg *dhcpv4.DHCPv4) []byte {
	t.Helper()
	data := msg.ToBytes()
	if len(data) == 0 {
		t.Fatal("message serialised to nothing")
	}
	return data
}

func leasesFor(t *testing.T, db *sql.DB, mac string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM dhcp_leases WHERE mac_address = ?", mac).Scan(&n); err != nil {
		t.Fatalf("count leases for %s: %v", mac, err)
	}
	return n
}

func waitFor(t *testing.T, limit time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", limit, what)
}
