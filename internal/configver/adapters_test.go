package configver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// --- DHCP scope adapter -----------------------------------------------------

func TestDHCPScopeAdapter_ValidateRejectsIncompleteSnapshots(t *testing.T) {
	adapter := NewDHCPScopeAdapter()

	cases := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "missing name",
			content: `{"subnet":"192.0.2.0/24","start_ip":"192.0.2.10","end_ip":"192.0.2.20"}`,
			wantErr: true,
		},
		{
			name:    "missing subnet",
			content: `{"name":"s","start_ip":"192.0.2.10","end_ip":"192.0.2.20"}`,
			wantErr: true,
		},
		{
			name:    "start after end",
			content: `{"name":"s","subnet":"192.0.2.0/24","start_ip":"192.0.2.90","end_ip":"192.0.2.20"}`,
			wantErr: true,
		},
		{
			name:    "end outside subnet",
			content: `{"name":"s","subnet":"192.0.2.0/24","start_ip":"192.0.2.10","end_ip":"198.51.100.20"}`,
			wantErr: true,
		},
		{
			name:    "lease time above max",
			content: `{"name":"s","subnet":"192.0.2.0/24","start_ip":"192.0.2.10","end_ip":"192.0.2.20","lease_time":7200,"max_lease_time":3600}`,
			wantErr: true,
		},
		{
			name:    "valid",
			content: `{"name":"s","subnet":"192.0.2.0/24","start_ip":"192.0.2.10","end_ip":"192.0.2.20","lease_time":3600}`,
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := adapter.Validate(json.RawMessage(tc.content))
			if tc.wantErr && err == nil {
				t.Fatalf("Validate accepted invalid content: %s", tc.content)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate rejected valid content: %v", err)
			}
		})
	}
}

func TestDHCPScopeAdapter_DoesNotRequireNotify(t *testing.T) {
	// The DHCP server reads scopes from the database on every request, so the
	// adapter has nothing to invalidate. Asserting the no-op keeps a future
	// scope cache from being forgotten silently: if Notify stops being a
	// no-op, this test has to be replaced by one that checks the cache.
	adapter := NewDHCPScopeAdapter()
	if err := adapter.Notify("anything"); err != nil {
		t.Fatalf("Notify returned %v, want nil", err)
	}
	if adapter.Type() != ResourceDHCPScope {
		t.Fatalf("Type = %s, want %s", adapter.Type(), ResourceDHCPScope)
	}
}

// --- DNS zone adapter -------------------------------------------------------

type countingReloader struct {
	mu    sync.Mutex
	calls int
}

func (r *countingReloader) ReloadNow() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
}

func (r *countingReloader) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func seedZone(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO dns_zones
		(id, name, type, enabled, default_ttl, soa_mname, soa_rname, serial,
		 refresh, retry, expire, minimum)
		VALUES (?, ?, 'primary', 1, 3600, 'ns1.example.test.', 'hostmaster.example.test.', 1,
		        3600, 600, 86400, 300)`, id, name); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
}

func zoneContent(t *testing.T, mutate func(*DNSZoneContent)) json.RawMessage {
	t.Helper()
	c := DNSZoneContent{
		Name:       "example.test.",
		Type:       "primary",
		Enabled:    true,
		DefaultTTL: 3600,
		SOAMName:   "ns1.example.test.",
		SOARName:   "hostmaster.example.test.",
		Refresh:    3600,
		Retry:      600,
		Expire:     86400,
		Minimum:    300,
	}
	if mutate != nil {
		mutate(&c)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal zone content: %v", err)
	}
	return b
}

func newZoneService(t *testing.T, db *sql.DB) (*Service, *countingReloader) {
	t.Helper()
	reloader := &countingReloader{}
	svc := NewService(db)
	if err := svc.Register(NewDNSZoneAdapter(reloader)); err != nil {
		t.Fatalf("register dns adapter: %v", err)
	}
	return svc, reloader
}

func TestDNSZoneAdapter_ReleaseForcesStoreReload(t *testing.T) {
	db := newTestDB(t)
	svc, reloader := newZoneService(t, db)
	seedZone(t, db, "zone-1", "example.test.")

	// A publish alone must not touch the resolver: only a completed release
	// should make the new configuration live.
	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceDNSZone, ResourceID: "zone-1",
		Content: zoneContent(t, func(c *DNSZoneContent) {
			c.DefaultTTL = 60
			c.SOARName = "dns-admin.example.test."
		}),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if reloader.count() != 0 {
		t.Fatalf("reload calls = %d before the release, want 0", reloader.count())
	}

	if applied, err := svc.DrainOutbox(10); err != nil || applied != 1 {
		t.Fatalf("drain = %d err = %v, want 1/nil", applied, err)
	}

	// The zone store answers from memory, so without this reload the record is
	// written but never served -- the classic "the change is in the database
	// and nobody can see it" failure.
	if reloader.count() != 1 {
		t.Fatalf("reload calls = %d, want 1 (the store must be reloaded)", reloader.count())
	}

	var ttl int
	var rname string
	if err := db.QueryRow(
		`SELECT default_ttl, soa_rname FROM dns_zones WHERE id = 'zone-1'`).Scan(&ttl, &rname); err != nil {
		t.Fatalf("read zone: %v", err)
	}
	if ttl != 60 || rname != "dns-admin.example.test." {
		t.Fatalf("zone = (ttl %d, rname %q), want the published values", ttl, rname)
	}
}

func TestDNSZoneAdapter_Validate(t *testing.T) {
	adapter := NewDNSZoneAdapter(nil)

	cases := []struct {
		name    string
		content json.RawMessage
		wantErr bool
	}{
		{"missing name", zoneContent(t, func(c *DNSZoneContent) { c.Name = "" }), true},
		{"invalid domain", zoneContent(t, func(c *DNSZoneContent) { c.Name = "not a domain" }), true},
		{"unknown type", zoneContent(t, func(c *DNSZoneContent) { c.Type = "banana" }), true},
		{"missing soa mname", zoneContent(t, func(c *DNSZoneContent) { c.SOAMName = "" }), true},
		// A negative SOA timer is stored happily by SQLite and then becomes a
		// huge positive value on the wire.
		{"negative refresh", zoneContent(t, func(c *DNSZoneContent) { c.Refresh = -1 }), true},
		{"negative ttl", zoneContent(t, func(c *DNSZoneContent) { c.DefaultTTL = -5 }), true},
		// A malformed ACL is ignored by the reader, which silently turns a
		// restrictive policy into an unrestricted one.
		{"malformed transfer policy", zoneContent(t, func(c *DNSZoneContent) { c.TransferPolicy = "{not json" }), true},
		{"valid", zoneContent(t, nil), false},
		{"valid with acl", zoneContent(t, func(c *DNSZoneContent) {
			c.TransferPolicy = `["192.0.2.0/24"]`
		}), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := adapter.Validate(tc.content)
			if tc.wantErr && err == nil {
				t.Fatalf("Validate accepted invalid content")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate rejected valid content: %v", err)
			}
		})
	}
}

// --- IPAM subnet adapter ----------------------------------------------------

func TestIPAMSubnetAdapter_PublishesAndValidates(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewIPAMSubnetAdapter()); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO ipam_spaces (id, name, description)
		VALUES ('space-1', 'default', '')`); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_subnets (id, space_id, name, cidr, vlan_id)
		VALUES ('subnet-1', 'space-1', 'office', '192.0.2.0/24', 10)`); err != nil {
		t.Fatalf("seed subnet: %v", err)
	}

	content := func(mutate func(*IPAMSubnetContent)) json.RawMessage {
		c := IPAMSubnetContent{Name: "office", CIDR: "192.0.2.0/24", VLANID: 10, Location: "HQ"}
		if mutate != nil {
			mutate(&c)
		}
		b, _ := json.Marshal(c)
		return b
	}

	adapter := NewIPAMSubnetAdapter()
	if err := adapter.Validate(content(func(c *IPAMSubnetContent) { c.CIDR = "192.0.2.0/33" })); err == nil {
		t.Fatalf("Validate accepted a /33 CIDR")
	}
	if err := adapter.Validate(content(func(c *IPAMSubnetContent) { c.VLANID = 4095 })); err == nil {
		t.Fatalf("Validate accepted VLAN 4095, which is reserved")
	}

	if _, err := svc.Publish(PublishRequest{
		ResourceType: ResourceIPAMSubnet, ResourceID: "subnet-1",
		Content: content(func(c *IPAMSubnetContent) {
			c.VLANID = 20
			c.Location = "Building B"
		}),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if applied, err := svc.DrainOutbox(10); err != nil || applied != 1 {
		t.Fatalf("drain = %d err = %v, want 1/nil", applied, err)
	}

	var vlan int
	var location string
	if err := db.QueryRow(
		`SELECT vlan_id, location FROM ipam_subnets WHERE id = 'subnet-1'`).Scan(&vlan, &location); err != nil {
		t.Fatalf("read subnet: %v", err)
	}
	if vlan != 20 || location != "Building B" {
		t.Fatalf("subnet = (vlan %d, location %q), want the published values", vlan, location)
	}
}

func TestIPAMSubnetAdapter_RejectsMissingResource(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	if err := svc.Register(NewIPAMSubnetAdapter()); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := svc.Publish(PublishRequest{
		ResourceType: ResourceIPAMSubnet, ResourceID: "missing",
		Content: json.RawMessage(`{"name":"x","cidr":"192.0.2.0/24"}`),
	})
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("err = %v, want ErrResourceNotFound", err)
	}
}
