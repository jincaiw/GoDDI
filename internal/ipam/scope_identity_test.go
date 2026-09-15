package ipam

import (
	"database/sql"
	"testing"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestScopeIdentityResolverUsesExactCanonicalSubnet(t *testing.T) {
	db := newIPAMTestDB(t)
	resolver, err := NewScopeIdentityResolver(db)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip) VALUES ('scope-1', 'lan', '192.0.2.7/24', '192.0.2.10', '192.0.2.20')`); err != nil {
		t.Fatalf("insert scope: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_spaces (id, name) VALUES ('space-1', 'prod')`); err != nil {
		t.Fatalf("insert space: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES ('subnet-1', 'space-1', 'lan', '192.0.2.0/24')`); err != nil {
		t.Fatalf("insert subnet: %v", err)
	}

	got, err := resolver.Resolve("scope-1", "192.0.2.10")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.ScopeID != "scope-1" || got.SpaceID != "space-1" || got.SubnetID != "subnet-1" {
		t.Fatalf("identity = %+v", got)
	}
}

func TestScopeIdentityResolverChoosesMostSpecificContainingSubnet(t *testing.T) {
	db := newIPAMTestDB(t)
	resolver, err := NewScopeIdentityResolver(db)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip) VALUES ('scope-1', 'lan', 'not-a-cidr', '192.0.2.10', '192.0.2.20')`); err != nil {
		t.Fatalf("insert scope: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_spaces (id, name) VALUES ('space-1', 'prod')`); err != nil {
		t.Fatalf("insert space: %v", err)
	}
	for _, row := range []struct{ id, cidr string }{{"subnet-wide", "192.0.2.0/24"}, {"subnet-narrow", "192.0.2.0/28"}} {
		if _, err := db.Exec(`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES (?, 'space-1', ?, ?)`, row.id, row.id, row.cidr); err != nil {
			t.Fatalf("insert subnet %s: %v", row.id, err)
		}
	}

	got, err := resolver.Resolve("scope-1", "192.0.2.10")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.SubnetID != "subnet-narrow" {
		t.Fatalf("subnet = %q, want subnet-narrow", got.SubnetID)
	}
}

func TestScopeIdentityResolverFailsClosedForMissingIdentity(t *testing.T) {
	db := newIPAMTestDB(t)
	resolver, err := NewScopeIdentityResolver(db)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	for _, tc := range []struct{ name, scope, ip string }{{"missing scope", "", "192.0.2.10"}, {"bad ip", "scope-1", "not-an-ip"}, {"missing subnet", "scope-1", "192.0.2.10"}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolver.Resolve(tc.scope, tc.ip); err == nil {
				t.Fatal("resolve succeeded without a complete identity")
			}
		})
	}
}

func TestNewScopeIdentityResolverRejectsNilDatabase(t *testing.T) {
	if _, err := NewScopeIdentityResolver(nil); err == nil {
		t.Fatal("nil database accepted")
	}
}

func newIPAMTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	goose.SetBaseFS(goddiassets.Migrations())
	t.Cleanup(func() { goose.SetBaseFS(nil); db.Close() })
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
