package scope

import (
	"database/sql"
	"net"
	"testing"

	_ "modernc.org/sqlite"
)

func newScopeDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE dhcp_scopes (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		interface TEXT,
		subnet TEXT NOT NULL,
		start_ip TEXT NOT NULL,
		end_ip TEXT NOT NULL,
		subnet_mask TEXT,
		router TEXT,
		dns_servers TEXT,
		ntp_servers TEXT,
		domain_name TEXT,
		lease_time INTEGER DEFAULT 86400,
		max_lease_time INTEGER,
		enabled BOOLEAN DEFAULT TRUE,
		ping_check_enabled BOOLEAN DEFAULT TRUE,
		dns_updates BOOLEAN DEFAULT FALSE,
		comment TEXT,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	-- The pool is a narrow slice of the subnet: a relay's giaddr (or a static
	-- host) lives in the subnet but outside the pool.
	INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, subnet_mask, enabled)
		VALUES ('scope-1', 'lan', '192.0.2.0/24', '192.0.2.100', '192.0.2.110', '255.255.255.0', 1);
	-- Legacy row whose subnet is not a valid CIDR: must fall back to the pool.
	INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, subnet_mask, enabled)
		VALUES ('scope-legacy', 'legacy', 'not-a-cidr', '198.51.100.10', '198.51.100.20', '255.255.255.0', 1);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// Regression: FindScopeByIP matched the allocation pool, so a relayed REQUEST
// whose giaddr sat inside the subnet but outside the pool found no scope and the
// client was silently ignored.
func TestFindScopeByIP_MatchesSubnetNotPool(t *testing.T) {
	db := newScopeDB(t)
	m := NewManager(db)

	tests := []struct {
		name   string
		ip     string
		wantID string
	}{
		{"relay address outside the pool but inside the subnet", "192.0.2.1", "scope-1"},
		{"static host outside the pool but inside the subnet", "192.0.2.50", "scope-1"},
		{"address inside the pool", "192.0.2.105", "scope-1"},
		{"network address", "192.0.2.0", "scope-1"},
		{"broadcast address", "192.0.2.255", "scope-1"},
		{"legacy scope falls back to pool bounds", "198.51.100.15", "scope-legacy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := m.FindScopeByIP(tt.ip)
			if err != nil {
				t.Fatalf("FindScopeByIP(%s): %v", tt.ip, err)
			}
			if sc.ID != tt.wantID {
				t.Errorf("scope = %s, want %s", sc.ID, tt.wantID)
			}
		})
	}
}

func TestFindScopeByIP_RejectsAddressesOutsideEverySubnet(t *testing.T) {
	db := newScopeDB(t)
	m := NewManager(db)

	for _, ip := range []string{
		"10.0.0.1",       // unrelated subnet
		"192.0.3.1",      // adjacent subnet, not ours
		"198.51.100.5",   // legacy scope, outside its pool
		"not-an-address", // malformed
	} {
		if sc, err := m.FindScopeByIP(ip); err == nil {
			t.Errorf("FindScopeByIP(%s) = %s, want no match", ip, sc.ID)
		}
	}
}

func TestScopeContainsIP_PrefersCIDROverPoolBounds(t *testing.T) {
	s := &Scope{
		Subnet:  "192.0.2.0/24",
		StartIP: "192.0.2.100",
		EndIP:   "192.0.2.110",
	}

	// The pool bounds must not narrow scope membership.
	if !scopeContainsIP(s, net.ParseIP("192.0.2.1")) {
		t.Error("an address inside the subnet but outside the pool must belong to the scope")
	}
	if scopeContainsIP(s, net.ParseIP("192.0.3.1")) {
		t.Error("an address outside the subnet must not belong to the scope")
	}
}
