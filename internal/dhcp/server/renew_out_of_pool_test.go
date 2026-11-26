package server

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/dhcp/lease"
)

// renewingRequest builds the RFC 2131 RENEWING REQUEST: ciaddr names the
// binding being extended, no server identifier and no requested-IP option.
func renewingOutOfPoolRequest(t *testing.T, mac net.HardwareAddr, ciaddr string) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithClientIP(net.ParseIP(ciaddr)),
	)
	if err != nil {
		t.Fatalf("build RENEWING REQUEST: %v", err)
	}
	return msg
}

func messageTypeOf(resp *dhcpv4.DHCPv4) dhcpv4.MessageType {
	if resp == nil {
		return dhcpv4.MessageType(0)
	}
	return resp.MessageType()
}

// The historical default must not change: a statically configured host can
// still renew an in-subnet address outside the pool.
func TestRenewOutOfPoolDefaultAllowsStaticHost(t *testing.T) {
	s, db := newDHCPTestServer(t)
	msg := renewingOutOfPoolRequest(t, testMAC(11), "192.0.2.50")

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if got := messageTypeOf(resp); got != dhcpv4.MessageTypeAck {
		t.Fatalf("default policy reply = %v, want ACK", got)
	}
	if got := leaseStatusFor(t, db, "192.0.2.50"); got != string(lease.LeaseStatusActive) {
		t.Errorf("lease status = %s, want active", got)
	}
}

// The strict policy must refuse a brand-new binding for an address the pool
// never allocated, and must not write a lease on the way out.
func TestRenewOutOfPoolStrictRefusesUnboundAddress(t *testing.T) {
	s, db := newDHCPTestServer(t)
	s.SetRenewOutOfPoolPolicy(config.RenewOutOfPoolExistingBindingOnly)
	msg := renewingOutOfPoolRequest(t, testMAC(12), "192.0.2.51")

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if got := messageTypeOf(resp); got != dhcpv4.MessageTypeNak {
		t.Fatalf("strict policy reply = %v, want NAK", got)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE ip_address = '192.0.2.51'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("a refused renewal wrote %d leases", count)
	}
}

// An existing held lease is an operator-visible binding: the strict policy
// must still renew it even though it is outside the pool.
func TestRenewOutOfPoolStrictAllowsExistingBinding(t *testing.T) {
	s, db := newDHCPTestServer(t)
	s.SetRenewOutOfPoolPolicy(config.RenewOutOfPoolExistingBindingOnly)
	mac := testMAC(13)
	if _, err := db.Exec(`
		INSERT INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen)
		VALUES ('renew-strict-1', 'scope-1', '192.0.2.52', ?, 'host13', '',
			datetime('now'), datetime('now', '+5 minutes'), 'active', datetime('now'))`,
		mac.String()); err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	resp, err := s.HandleRequest(renewingOutOfPoolRequest(t, mac, "192.0.2.52"), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if got := messageTypeOf(resp); got != dhcpv4.MessageTypeAck {
		t.Fatalf("existing binding reply = %v, want ACK", got)
	}
}

// An operator-created reservation is explicit authorisation for that address.
func TestRenewOutOfPoolStrictAllowsReservation(t *testing.T) {
	s, db := newDHCPTestServer(t)
	s.SetRenewOutOfPoolPolicy(config.RenewOutOfPoolExistingBindingOnly)
	mac := testMAC(14)
	if _, err := db.Exec(`
		INSERT INTO dhcp_reservations (id, scope_id, ip_address, mac_address, hostname, enabled)
		VALUES ('res-strict-1', 'scope-1', '192.0.2.53', ?, 'host14', 1)`, mac.String()); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}

	resp, err := s.HandleRequest(renewingOutOfPoolRequest(t, mac, "192.0.2.53"), "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if got := messageTypeOf(resp); got != dhcpv4.MessageTypeAck {
		t.Fatalf("reserved address reply = %v, want ACK", got)
	}
}

// The policy is about out-of-pool addresses only; in-pool renewals keep the
// allocation semantics they had before.
func TestRenewOutOfPoolStrictLeavesInPoolUnchanged(t *testing.T) {
	s, db := newDHCPTestServer(t)
	s.SetRenewOutOfPoolPolicy(config.RenewOutOfPoolExistingBindingOnly)
	msg := renewingOutOfPoolRequest(t, testMAC(15), "192.0.2.101") // inside 100-102 pool

	resp, err := s.HandleRequest(msg, "eth0", net.ParseIP(testServerIP))
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if got := messageTypeOf(resp); got != dhcpv4.MessageTypeAck {
		t.Fatalf("in-pool renewal reply = %v, want ACK", got)
	}
	if got := leaseStatusFor(t, db, "192.0.2.101"); got != string(lease.LeaseStatusActive) {
		t.Errorf("lease status = %s, want active", got)
	}
}

// A typo must never silently tighten or loosen the policy.
func TestSetRenewOutOfPoolPolicyFallsBackOnUnknownValue(t *testing.T) {
	s, _ := newDHCPTestServer(t)
	s.SetRenewOutOfPoolPolicy("stricter-than-strict")
	if s.strictOutOfPoolRenewal() {
		t.Fatal("an unknown policy unexpectedly enabled strict mode")
	}
	if s.renewOutOfPoolPolicy != config.RenewOutOfPoolAllow {
		t.Errorf("fallback policy = %q, want %q", s.renewOutOfPoolPolicy, config.RenewOutOfPoolAllow)
	}
	s.SetRenewOutOfPoolPolicy("")
	if s.renewOutOfPoolPolicy != config.RenewOutOfPoolAllow {
		t.Errorf("zero-value policy = %q, want %q", s.renewOutOfPoolPolicy, config.RenewOutOfPoolAllow)
	}
}
