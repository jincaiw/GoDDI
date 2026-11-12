package ipamstore

import (
	"context"
	"sync"
	"testing"
)

func TestV1PreflightRejectsNonPositiveInEveryTable(t *testing.T) {
	cases := []struct{ name, insert string }{
		{"ipam_resource_ids", `INSERT INTO ipam_resource_ids(rowid,kind,tenant_id,space_id,id) VALUES(?,'TENANT','','','extra')`},
		{"ipam_tenants", `INSERT INTO ipam_tenants(rowid,id,slug,display_name) VALUES(?,'extra','extra','extra')`},
		{"ipam_spaces", `INSERT INTO ipam_spaces(rowid,tenant_id,id,slug,display_name) VALUES(?,'t','extra','extra','extra')`},
		{"ipam_prefixes", `INSERT INTO ipam_prefixes(rowid,tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES(?,'t','s','extra',4,24,x'000000000000000000000000C6336400',x'000000000000000000000000C63364FF','SUBNET','STANDARD')`},
		{"ipam_pools", `INSERT INTO ipam_pools(rowid,tenant_id,space_id,id,subnet_id,family,pool_kind,first_ip,last_ip) VALUES(?,'t','s','extra','n',4,'IPAM',x'000000000000000000000000C0000264',x'000000000000000000000000C000026E')`},
		{"ipam_occupancies", `INSERT INTO ipam_occupancies(rowid,tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES(?,'t','s','extra','n',4,x'000000000000000000000000C000021E','STATIC','owner',1)`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := legacyFixture(t)
			for _, q := range []string{`INSERT INTO ipam_tenants(id,slug,display_name) VALUES('t','t','t')`, `INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES('t','s','s','s')`, `INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES('t','s','n',4,24,x'000000000000000000000000C0000200',x'000000000000000000000000C00002FF','SUBNET','STANDARD')`} {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(tc.insert, -1); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(context.Background(), db); err == nil {
				t.Fatal("negative rowid accepted")
			}
			markers(t, db, 1)
		})
	}
}
func TestConcurrentV1UpgradeSerializes(t *testing.T) {
	db := legacyFixture(t)
	db.SetMaxOpenConns(2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; out <- Migrate(context.Background(), db) }()
	}
	close(start)
	wg.Wait()
	close(out)
	for err := range out {
		if err != nil {
			t.Fatal(err)
		}
	}
	markers(t, db, 2)
}
