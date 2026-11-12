package ipamstore

import (
	"context"
	"strings"
	"testing"
)

// This exercises constraints in the installed migration, rather than testing
// only that the source SQL compiles. In particular, OR REPLACE must not evict
// another tenant's or owner's allocation.
func TestInstalledConstraintsRejectReplacementAndCrossScope(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO ipam_tenants(id,slug,display_name) VALUES ('t1','tenant','Tenant')`,
		`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES ('t1','s1','one','One')`,
		`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES ('t1','s2','two','Two')`,
		`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES ('t1','s1','net',4,24,x'000000000000000000000000C0000200',x'000000000000000000000000C00002FF','SUBNET','STANDARD')`,
		`INSERT INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES ('t1','s1','a1','net',4,x'000000000000000000000000C000020A','STATIC','owner1',1)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("setup %s: %v", q, err)
		}
	}
	_, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES ('t1','s1','a2','net',4,x'000000000000000000000000C000020A','STATIC','owner2',1)`)
	if err == nil || !strings.Contains(err.Error(), "IPAM_ADDRESS_OCCUPIED") {
		t.Fatalf("expected occupied address refusal, got %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES ('t1','s2','a3','net',4,x'000000000000000000000000C000020B','STATIC','owner3',1)`)
	if err == nil {
		t.Fatal("cross-space subnet reference was accepted")
	}
	var owner string
	if err := db.QueryRowContext(ctx, `SELECT owner_ref FROM ipam_occupancies WHERE tenant_id='t1' AND space_id='s1' AND id='a1'`).Scan(&owner); err != nil || owner != "owner1" {
		t.Fatalf("original occupant changed: %q, %v", owner, err)
	}
}
