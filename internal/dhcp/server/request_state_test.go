package server

import (
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
	_ "modernc.org/sqlite"
)

const testServerIP = "192.0.2.1"

func testMAC(b byte) net.HardwareAddr {
	return net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, b}
}

// newDHCPTestServer builds a server backed by the real lease store, holding one
// scope whose subnet is wider than its allocation pool. That layout mirrors a
// relayed deployment: the identifying address (a relay's giaddr, or a static
// host) sits inside the subnet but outside the pool.
//
// The store is the shipped one rather than a hand-written schema, because the
// lease manager records every state change it makes in an audit table. A
// fixture without that table does not fail -- the write is logged and dropped,
// as it is in production when the database is unhappy -- so it would make the
// audit look absent rather than broken.
func newDHCPTestServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()

	store := newLeaseStore(t)
	if _, err := store.Exec(`
		INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip, subnet_mask,
			router, dns_servers, domain_name, lease_time, enabled, ping_check_enabled, dns_updates)
		VALUES ('scope-1', 'lan', 'eth0', '192.0.2.0/24', '192.0.2.100', '192.0.2.102',
			'255.255.255.0', '192.0.2.1', '192.0.2.1', 'example.test', 3600, 1, 0, 0)`); err != nil {
		t.Fatalf("seed scope: %v", err)
	}

	s := New(store.DB, []string{"eth0"}, nil)
	// New() builds the managers but derives server IPs from the host's
	// interfaces; pin them so the tests are independent of the machine.
	s.serverIPs = map[string]net.IP{"eth0": net.ParseIP(testServerIP)}
	return s, store.DB
}

func leaseStatusFor(t *testing.T, db *sql.DB, ip string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(
		"SELECT status FROM dhcp_leases WHERE ip_address = ? ORDER BY lease_end DESC LIMIT 1",
		ip).Scan(&status); err != nil {
		t.Fatalf("read lease status for %s: %v", ip, err)
	}
	return status
}

func discoverMsg(t *testing.T, mac net.HardwareAddr) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover),
		dhcpv4.WithHwAddr(mac),
	)
	if err != nil {
		t.Fatalf("build DISCOVER: %v", err)
	}
	return msg
}

// selectingRequest builds the REQUEST a client sends to accept our OFFER.
func selectingRequest(t *testing.T, mac net.HardwareAddr, ip string) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(net.ParseIP(testServerIP))),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(ip))),
	)
	if err != nil {
		t.Fatalf("build SELECTING REQUEST: %v", err)
	}
	return msg
}

// ---------------------------------------------------------------------------
// classifyRequest
// ---------------------------------------------------------------------------

func TestClassifyRequest(t *testing.T) {
	s, _ := newDHCPTestServer(t)
	mac := testMAC(1)

	tests := []struct {
		name      string
		build     func() *dhcpv4.DHCPv4
		wantState requestState
		wantIP    string
		wantOK    bool
	}{
		{
			name: "our server identifier means SELECTING",
			build: func() *dhcpv4.DHCPv4 {
				return selectingRequest(t, mac, "192.0.2.100")
			},
			wantState: requestSelecting,
			wantIP:    "192.0.2.100",
			wantOK:    true,
		},
		{
			name: "another server's identifier must be ignored",
			build: func() *dhcpv4.DHCPv4 {
				msg, _ := dhcpv4.New(
					dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
					dhcpv4.WithHwAddr(mac),
					dhcpv4.WithOption(dhcpv4.OptServerIdentifier(net.ParseIP("192.0.2.99"))),
					dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("192.0.2.100"))),
				)
				return msg
			},
			wantOK: false,
		},
		{
			name: "ciaddr without server identifier means RENEWING",
			build: func() *dhcpv4.DHCPv4 {
				msg, _ := dhcpv4.New(
					dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
					dhcpv4.WithHwAddr(mac),
					dhcpv4.WithClientIP(net.ParseIP("192.0.2.50")),
				)
				return msg
			},
			wantState: requestRenewing,
			wantIP:    "192.0.2.50",
			wantOK:    true,
		},
		{
			name: "requested address without server identifier means INIT-REBOOT",
			build: func() *dhcpv4.DHCPv4 {
				msg, _ := dhcpv4.New(
					dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
					dhcpv4.WithHwAddr(mac),
					dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("192.0.2.101"))),
				)
				return msg
			},
			wantState: requestInitReboot,
			wantIP:    "192.0.2.101",
			wantOK:    true,
		},
		{
			name: "a REQUEST with neither ciaddr nor requested address is unusable",
			build: func() *dhcpv4.DHCPv4 {
				msg, _ := dhcpv4.New(
					dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
					dhcpv4.WithHwAddr(mac),
				)
				return msg
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, ip, ok := s.classifyRequest(tt.build())
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if state != tt.wantState {
				t.Errorf("state = %v, want %v", state, tt.wantState)
			}
			if ip.String() != tt.wantIP {
				t.Errorf("ip = %s, want %s", ip, tt.wantIP)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DISCOVER / REQUEST handshake
// ---------------------------------------------------------------------------

// Regression: DISCOVER used to write an active lease, so a client that never
// sent REQUEST held the address with the full lease time and the server
// reported a binding the client had never accepted.
func TestHandleDiscover_ReservesWithoutCreatingABinding(t *testing.T) {
	s, db := newDHCPTestServer(t)

	resp, err := s.HandleDiscover(discoverMsg(t, testMAC(1)), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if resp == nil || resp.MessageType() != dhcpv4.MessageTypeOffer {
		t.Fatalf("expected an OFFER, got %v", resp)
	}

	granted := resp.YourIPAddr.String()
	if got := leaseStatusFor(t, db, granted); got != string(lease.LeaseStatusOffered) {
		t.Errorf("lease status = %s, want offered", got)
	}

	// The OFFER must advertise the full lease time the client will get on ACK,
	// even though the internal hold is short.
	if resp.IPAddressLeaseTime(0) != time.Duration(3600)*time.Second {
		t.Errorf("OFFER lease time = %s, want 1h", resp.IPAddressLeaseTime(0))
	}
}

func TestHandleRequest_SelectingPromotesOfferToBinding(t *testing.T) {
	s, db := newDHCPTestServer(t)
	mac := testMAC(2)

	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	ip := offer.YourIPAddr.String()

	resp, err := s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp == nil {
		t.Fatal("SELECTING REQUEST must be answered with an ACK")
	}
	if resp.MessageType() != dhcpv4.MessageTypeAck {
		t.Fatalf("message type = %s, want ACK", resp.MessageType())
	}
	if resp.YourIPAddr.String() != ip {
		t.Errorf("ACK address = %s, want the offered %s", resp.YourIPAddr, ip)
	}

	if got := leaseStatusFor(t, db, ip); got != string(lease.LeaseStatusActive) {
		t.Errorf("lease status = %s, want active", got)
	}

	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM dhcp_leases WHERE ip_address = ?", ip).Scan(&rows); err != nil {
		t.Fatalf("count leases: %v", err)
	}
	if rows != 1 {
		t.Errorf("got %d lease rows for the address, want the single promoted row", rows)
	}
}

func TestHandleRequest_InternalErrorReportsNilOrNakButNeverSilentSuccess(t *testing.T) {
	s, db := newDHCPTestServer(t)
	mac := testMAC(3)

	// An address held by another client must be refused with a NAK, not
	// silently acknowledged.
	if _, err := db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen)
		VALUES ('held-1', 'scope-1', '192.0.2.100', 'aa:bb:cc:dd:ee:ff', 'other', '',
			datetime('now'), datetime('now', '+1 hour'), 'active', datetime('now'))`); err != nil {
		t.Fatalf("seed held lease: %v", err)
	}

	resp, err := s.HandleRequest(selectingRequest(t, mac, "192.0.2.100"), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a NAK, got silence")
	}
	if resp.MessageType() != dhcpv4.MessageTypeNak {
		t.Fatalf("message type = %s, want NAK", resp.MessageType())
	}
}

// Regression: an INIT-REBOOT for a client the server has no record of must be
// answered with silence (RFC 2131 §4.3.2). A NAK would tell the client its
// working address is invalid.
func TestHandleRequest_InitRebootForUnknownClientIsSilent(t *testing.T) {
	s, _ := newDHCPTestServer(t)

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(testMAC(4)),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("192.0.2.101"))),
	)
	if err != nil {
		t.Fatalf("build INIT-REBOOT REQUEST: %v", err)
	}

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected silence, got %s", resp.MessageType())
	}
}

func TestHandleRequest_InitRebootWithMismatchedBindingIsNaked(t *testing.T) {
	s, db := newDHCPTestServer(t)
	mac := testMAC(5)

	// We know this client, but bound to a different address.
	if _, err := db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen)
		VALUES ('bound-1', 'scope-1', '192.0.2.102', ?, 'host5', '',
			datetime('now'), datetime('now', '+1 hour'), 'active', datetime('now'))`,
		mac.String()); err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("192.0.2.101"))),
	)
	if err != nil {
		t.Fatalf("build REQUEST: %v", err)
	}

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp == nil || resp.MessageType() != dhcpv4.MessageTypeNak {
		t.Fatalf("expected a NAK, got %v", resp)
	}
}

func TestHandleRequest_RenewingExtendsExistingBinding(t *testing.T) {
	s, db := newDHCPTestServer(t)
	mac := testMAC(6)

	if _, err := db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen)
		VALUES ('renew-1', 'scope-1', '192.0.2.100', ?, 'host6', '',
			datetime('now'), datetime('now', '+5 minutes'), 'active', datetime('now'))`,
		mac.String()); err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP("192.0.2.100")),
	)
	if err != nil {
		t.Fatalf("build RENEWING REQUEST: %v", err)
	}

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp == nil || resp.MessageType() != dhcpv4.MessageTypeAck {
		t.Fatalf("expected an ACK, got %v", resp)
	}

	var end string
	if err := db.QueryRow("SELECT lease_end FROM dhcp_leases WHERE id = 'renew-1'").Scan(&end); err != nil {
		t.Fatalf("read lease_end: %v", err)
	}
	expires, err := time.Parse("2006-01-02T15:04:05Z", end)
	if err != nil {
		t.Fatalf("parse lease_end %q: %v", end, err)
	}
	if time.Until(expires) < 50*time.Minute {
		t.Errorf("lease only extended to %s, want the full lease time", end)
	}
}

// A static host inside the subnet but outside the pool must still renew: the
// pool bounds govern allocation, not the confirmation of an existing binding.
// Matching scopes on pool bounds instead of the subnet is what broke this.
func TestHandleRequest_RenewingOutsidePoolInsideSubnetIsAcked(t *testing.T) {
	s, db := newDHCPTestServer(t)
	mac := testMAC(7)

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP("192.0.2.50")),
	)
	if err != nil {
		t.Fatalf("build RENEWING REQUEST: %v", err)
	}

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp == nil || resp.MessageType() != dhcpv4.MessageTypeAck {
		t.Fatalf("expected an ACK for a static host, got %v", resp)
	}
	if got := leaseStatusFor(t, db, "192.0.2.50"); got != string(lease.LeaseStatusActive) {
		t.Errorf("lease status = %s, want the address to be tracked as active", got)
	}
}

// A REQUEST aimed at another server must produce no reply at all: answering
// would leave two servers fighting over the same client.
func TestHandleRequest_ForeignServerIdentifierProducesNoReply(t *testing.T) {
	s, _ := newDHCPTestServer(t)

	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(testMAC(8)),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(net.ParseIP("192.0.2.99"))),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("192.0.2.100"))),
	)
	if err != nil {
		t.Fatalf("build REQUEST: %v", err)
	}

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected no reply, got %s", resp.MessageType())
	}
}

// ---------------------------------------------------------------------------
// DECLINE
// ---------------------------------------------------------------------------

// Regression: DECLINE only relabelled the lease while the availability query
// looked at 'active' alone, so the next DISCOVER was offered the very address
// the previous client had just rejected.
func TestHandleDecline_TakesAddressOutOfThePool(t *testing.T) {
	s, db := newDHCPTestServer(t)
	mac := testMAC(9)

	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	declined := offer.YourIPAddr.String()

	decline, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(declined))),
	)
	if err != nil {
		t.Fatalf("build DECLINE: %v", err)
	}
	if err := s.HandleDecline(decline); err != nil {
		t.Fatalf("HandleDecline: %v", err)
	}
	if got := leaseStatusFor(t, db, declined); got != string(lease.LeaseStatusConflict) {
		t.Errorf("lease status = %s, want conflict", got)
	}

	// The next client must be offered a different address.
	next, err := s.HandleDiscover(discoverMsg(t, testMAC(10)), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("second HandleDiscover: %v", err)
	}
	if next.YourIPAddr.String() == declined {
		t.Fatalf("the declined address %s was offered again", declined)
	}
}

func TestHandleDecline_QuarantinesAddressWithNoLease(t *testing.T) {
	s, db := newDHCPTestServer(t)

	// A DECLINE can arrive for an address this server never leased.
	decline, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
		dhcpv4.WithHwAddr(testMAC(11)),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("192.0.2.100"))),
	)
	if err != nil {
		t.Fatalf("build DECLINE: %v", err)
	}
	if err := s.HandleDecline(decline); err != nil {
		t.Fatalf("HandleDecline: %v", err)
	}

	if got := leaseStatusFor(t, db, "192.0.2.100"); got != string(lease.LeaseStatusConflict) {
		t.Errorf("tombstone status = %s, want conflict", got)
	}

	offer, err := s.HandleDiscover(discoverMsg(t, testMAC(12)), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleDiscover: %v", err)
	}
	if offer.YourIPAddr.String() == "192.0.2.100" {
		t.Fatal("a quarantined address with no prior lease was offered again")
	}
}
