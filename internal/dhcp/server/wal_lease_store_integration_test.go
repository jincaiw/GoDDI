package server

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"github.com/jincaiw/GoDDI/internal/dhcp/lease"
)

// TestWALLeaseStoreServesDORAAndEmitsFacts drives a real DISCOVER/REQUEST through
// the server backed by the event-sourced WAL store (no synchronous facts seam)
// and proves the ACK is served from the WAL store while the binding and its
// fact are projected/derived from the same durable event.
func TestWALLeaseStoreServesDORAAndEmitsFacts(t *testing.T) {
	store := newLeaseStore(t)
	if _, err := store.Exec(`
		INSERT INTO dhcp_scopes (id, name, interface, subnet, start_ip, end_ip, subnet_mask,
			router, dns_servers, domain_name, lease_time, enabled, ping_check_enabled, dns_updates)
		VALUES ('scope-1', 'lan', 'eth0', '192.0.2.0/24', '192.0.2.100', '192.0.2.102',
			'255.255.255.0', '192.0.2.1', '192.0.2.1', 'example.test', 3600, 1, 0, 0)`); err != nil {
		t.Fatalf("seed scope: %v", err)
	}
	ctx := context.Background()
	walStore, err := lease.OpenWALLeaseStore(ctx, lease.WALLeaseStoreConfig{
		Path: filepath.Join(t.TempDir(), "leases.wal"), DB: store.DB, Capacity: 16,
		Source: "node-a", ResolveSpaceID: func(scopeID, ip string) (string, error) { return "space-1", nil },
	})
	if err != nil {
		t.Fatalf("OpenWALLeaseStore: %v", err)
	}
	defer walStore.Close(context.Background())

	s := NewWithLeaseStore(walStore, store.DB, []string{"eth0"}, nil)
	s.serverIPs = map[string]net.IP{"eth0": net.ParseIP(testServerIP)}

	mac := testMAC(0x93)
	offer, err := s.HandleDiscover(discoverMsg(t, mac), "eth0", net.ParseIP(testServerIP))
	if err != nil || offer == nil {
		t.Fatalf("HandleDiscover: offer=%v err=%v", offer, err)
	}
	ip := offer.YourIPAddr.String()
	ack, err := s.HandleRequest(selectingRequest(t, mac, ip), "eth0", net.ParseIP(testServerIP))
	if err != nil || ack == nil {
		t.Fatalf("HandleRequest: ack=%v err=%v", ack, err)
	}
	if err := walStore.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	if status := leaseStatusFor(t, store.DB, ip); status != "active" {
		t.Fatalf("projected lease status = %q, want active", status)
	}
	var action, source string
	if err := store.DB.QueryRow(
		`SELECT action, source FROM dhcp_ipam_observation_events ORDER BY sequence DESC LIMIT 1`).
		Scan(&action, &source); err != nil {
		t.Fatalf("read fact: %v", err)
	}
	if action != "request_activate" || source != "node-a" {
		t.Fatalf("fact = %s/%s, want request_activate/node-a", action, source)
	}
}
