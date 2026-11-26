package server

// Shared fixtures for the DHCP linkage tests. The DDNS outbox tests that used
// to live in ddns_linkage_test.go were retired with the legacy queue; these
// helpers build the same real-schema server for the IPAM linkage tests.

import (
	"database/sql"
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jincaiw/GoDDI/internal/dns/zone"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jincaiw/GoDDI"
)

const (
	linkageZone    = "example.test."
	linkageReverse = "2.0.192.in-addr.arpa."
	linkageDomain  = "example.test"
	// The pool is a single address so the assertions do not depend on which
	// candidate the allocator happens to pick.
	linkagePoolIP = "192.0.2.100"
)

// newLinkageServer builds a DHCP server wired to a real authoritative zone
// store over the real migration schema.
//
// Using the real store is the point: the database row alone proves nothing,
// because queries are answered from the in-memory zone data. Only a lookup
// through the store shows that the linkage is actually closed.
func newLinkageServer(t *testing.T) (*Server, *sql.DB, *zone.Store) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	goose.SetBaseFS(goddiassets.Migrations())
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	goose.SetBaseFS(nil)
	t.Cleanup(func() { db.Close() })

	mustExec(t, db, `INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial,
		refresh, retry, expire, minimum)
		VALUES ('zone-forward', ?, 'primary', 1, 'ns1.example.test.', 'hostmaster.example.test.',
		        1, 3600, 600, 86400, 300)`, linkageZone)
	mustExec(t, db, `INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial,
		refresh, retry, expire, minimum)
		VALUES ('zone-reverse', ?, 'primary', 1, 'ns1.example.test.', 'hostmaster.example.test.',
		        1, 3600, 600, 86400, 300)`, linkageReverse)
	mustExec(t, db, `INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip,
		subnet_mask, router, dns_servers, domain_name, lease_time, enabled, ping_check_enabled, dns_updates)
		VALUES ('scope-1', 'lan', 'eth0', '192.0.2.0/24', ?, ?, '255.255.255.0',
		        '192.0.2.1', '192.0.2.1', ?, 3600, 1, 0, 1)`,
		linkagePoolIP, linkagePoolIP, linkageDomain)

	store := zone.NewStore(db)
	s := New(db, []string{"eth0"}, nil)
	s.serverIPs = map[string]net.IP{"eth0": net.ParseIP(testServerIP)}
	return s, db, store
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func discoverWithHostname(t *testing.T, mac net.HardwareAddr, hostname string) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptHostName(hostname)),
	)
	if err != nil {
		t.Fatalf("build DISCOVER: %v", err)
	}
	return msg
}

func requestWithHostname(t *testing.T, mac net.HardwareAddr, ip, hostname string) *dhcpv4.DHCPv4 {
	t.Helper()
	msg, err := dhcpv4.New(
		dhcpv4.WithMessageType(dhcpv4.MessageTypeRequest),
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(net.ParseIP(testServerIP))),
		dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(ip))),
		dhcpv4.WithOption(dhcpv4.OptHostName(hostname)),
	)
	if err != nil {
		t.Fatalf("build REQUEST: %v", err)
	}
	return msg
}
