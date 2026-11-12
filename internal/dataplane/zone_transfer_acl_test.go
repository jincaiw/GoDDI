package dataplane

import (
	"context"
	"testing"

	"github.com/jasonwa/goddi/internal/dns/transfer"
)

// A DNS data plane answers AXFR/IXFR without ever opening the control
// database. The transfer handler reads the ACL from the store it serves from,
// so the grants have to be replicated there. Before this was fixed, the query
// failed on a missing table and every transfer was denied in the roles that
// actually serve DNS.
func TestZoneTransferACLIsReplicatedIntoTheDataPlaneStore(t *testing.T) {
	control, store, rep := newZonePair(t)

	insertControlZone(t, control, "z1", "example.test.")
	if _, err := control.Exec(`INSERT INTO dns_zone_transfer (id, zone_id, allowed_cidr, tsig_key_name)
		VALUES ('acl1', 'z1', '192.0.2.0/24', NULL)`); err != nil {
		t.Fatalf("insert transfer ACL: %v", err)
	}

	if _, err := rep.Sync(context.Background(), DomainDNS); err != nil {
		t.Fatalf("sync dns domain: %v", err)
	}

	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dns_zone_transfer`).Scan(&count); err != nil {
		t.Fatalf("the data-plane store has no dns_zone_transfer table: %v", err)
	}
	if count != 1 {
		t.Fatalf("replicated ACL rows = %d, want 1", count)
	}

	handler := transfer.NewAXFRHandler(store.DB)
	if !handler.CheckTransferACL("example.test", "192.0.2.10") {
		t.Error("a client inside the allowed CIDR was denied AXFR")
	}
	if handler.CheckTransferACL("example.test", "198.51.100.10") {
		t.Error("a client outside the allowed CIDR was granted AXFR")
	}
	// No ACL rows means deny by default, including an unknown zone.
	if handler.CheckTransferACL("other.test", "192.0.2.10") {
		t.Error("a zone with no grants was granted AXFR")
	}
}

// Changing an allow-CIDR must invalidate the data plane's copy. Without the
// triggers on dns_zone_transfer, Sync would see an unchanged revision and keep
// serving the stale policy forever.
func TestZoneTransferACLChangesInvalidateTheDataPlaneCopy(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	insertControlZone(t, control, "z1", "example.test.")
	if _, err := control.Exec(`INSERT INTO dns_zone_transfer (id, zone_id, allowed_cidr)
		VALUES ('acl1', 'z1', '192.0.2.0/24')`); err != nil {
		t.Fatalf("insert transfer ACL: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	var before int64
	if err := control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = 'dns'`).Scan(&before); err != nil {
		t.Fatalf("read dns revision: %v", err)
	}

	// Narrow the grant. The old client must lose access after the next sync.
	if _, err := control.Exec(`UPDATE dns_zone_transfer SET allowed_cidr = '198.51.100.0/24' WHERE id = 'acl1'`); err != nil {
		t.Fatalf("update transfer ACL: %v", err)
	}

	var after int64
	if err := control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = 'dns'`).Scan(&after); err != nil {
		t.Fatalf("read dns revision after ACL change: %v", err)
	}
	if after <= before {
		t.Fatalf("ACL change did not bump the dns revision: before=%d after=%d", before, after)
	}

	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("resync after ACL change: %v", err)
	}

	var cidr string
	if err := store.QueryRow(`SELECT allowed_cidr FROM dns_zone_transfer WHERE id = 'acl1'`).Scan(&cidr); err != nil {
		t.Fatalf("read replicated CIDR: %v", err)
	}
	if cidr != "198.51.100.0/24" {
		t.Fatalf("replicated CIDR = %q, want the updated value", cidr)
	}

	handler := transfer.NewAXFRHandler(store.DB)
	if handler.CheckTransferACL("example.test", "192.0.2.10") {
		t.Error("the revoked client still has AXFR access")
	}
	if !handler.CheckTransferACL("example.test", "198.51.100.10") {
		t.Error("the newly allowed client was denied AXFR")
	}

	// A deleted grant must disappear from the store as well.
	if _, err := control.Exec(`DELETE FROM dns_zone_transfer WHERE id = 'acl1'`); err != nil {
		t.Fatalf("delete transfer ACL: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("resync after ACL delete: %v", err)
	}
	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dns_zone_transfer`).Scan(&count); err != nil {
		t.Fatalf("count ACL rows: %v", err)
	}
	if count != 0 {
		t.Errorf("deleted ACL row survived the sync: count=%d", count)
	}
	if handler.CheckTransferACL("example.test", "198.51.100.10") {
		t.Error("a deleted grant still permits AXFR")
	}
}

// Keep the store's schema honest: a projection written by a future migration
// must not quietly diverge from the columns readAll selects from control.
func TestZoneTransferProjectionColumnsMatchControl(t *testing.T) {
	control, store, _ := newZonePair(t)
	tables := TablesFor(DomainDNS)
	var projection *table
	for i := range tables {
		if tables[i].name == "dns_zone_transfer" {
			projection = &tables[i]
			break
		}
	}
	if projection == nil {
		t.Fatal("dns_zone_transfer is not in the DNS replication domain")
	}
	for _, col := range projection.columns {
		var name string
		row := control.QueryRow(`SELECT name FROM pragma_table_info('dns_zone_transfer') WHERE name = ?`, col)
		if err := row.Scan(&name); err != nil {
			t.Errorf("control dns_zone_transfer has no column %q: %v", col, err)
		}
		var local string
		row = store.QueryRow(`SELECT name FROM pragma_table_info('dns_zone_transfer') WHERE name = ?`, col)
		if err := row.Scan(&local); err != nil {
			t.Errorf("data-plane dns_zone_transfer has no column %q: %v", col, err)
		}
	}
}
