package dataplane

import (
	"context"
	"testing"
)

// The RFC 2136 handler reads update policies from the data-plane store, so a
// grant written on control has to be replicated before any signed UPDATE can
// be accepted.
func TestDynamicUpdatePoliciesAreReplicatedToTheStore(t *testing.T) {
	control, store, rep := newZonePair(t)
	ctx := context.Background()

	if _, err := control.Exec(`
		INSERT INTO dns_zones (id, name, type, enabled, dnssec_enabled, default_ttl,
			soa_mname, soa_rname, serial, refresh, retry, expire, minimum)
		VALUES ('z1', 'policy.example.test.', 'primary', 1, 0, 300,
			'ns1.policy.example.test.', 'admin.policy.example.test.',
			1, 3600, 600, 86400, 300)`); err != nil {
		t.Fatalf("insert zone: %v", err)
	}
	if _, err := control.Exec(`
		INSERT INTO dns_dynamic_update_policies
			(id, zone_id, name, pattern, action, tsig_key_name, enabled)
		VALUES ('p1', 'z1', 'allow-local', '127.0.0.0/8', 'allow', 'probe-key.', 1)`); err != nil {
		t.Fatalf("insert policy: %v", err)
	}

	var before int64
	if err := control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = 'dns'`).Scan(&before); err != nil {
		t.Fatalf("read dns revision: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("sync dns domain: %v", err)
	}

	var action, keyName string
	if err := store.QueryRow(`
		SELECT action, tsig_key_name FROM dns_dynamic_update_policies WHERE id = 'p1'`,
	).Scan(&action, &keyName); err != nil {
		t.Fatalf("the data-plane store has no dns_dynamic_update_policies table: %v", err)
	}
	if action != "allow" || keyName != "probe-key." {
		t.Fatalf("replicated policy = (%q, %q), want (allow, probe-key.)", action, keyName)
	}

	// Revoking the grant must invalidate the data-plane copy.
	if _, err := control.Exec(`UPDATE dns_dynamic_update_policies SET action = 'deny' WHERE id = 'p1'`); err != nil {
		t.Fatalf("revoke policy: %v", err)
	}
	var after int64
	if err := control.QueryRow(`SELECT revision FROM dataplane_revision WHERE domain = 'dns'`).Scan(&after); err != nil {
		t.Fatalf("read dns revision after policy change: %v", err)
	}
	if after <= before {
		t.Fatalf("policy change did not bump the dns revision: before=%d after=%d", before, after)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("resync: %v", err)
	}
	if err := store.QueryRow(`SELECT action FROM dns_dynamic_update_policies WHERE id = 'p1'`).Scan(&action); err != nil {
		t.Fatalf("read replicated policy: %v", err)
	}
	if action != "deny" {
		t.Errorf("replicated policy action = %q, want deny", action)
	}

	if _, err := control.Exec(`DELETE FROM dns_dynamic_update_policies WHERE id = 'p1'`); err != nil {
		t.Fatalf("delete policy: %v", err)
	}
	if _, err := rep.Sync(ctx, DomainDNS); err != nil {
		t.Fatalf("resync after delete: %v", err)
	}
	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dns_dynamic_update_policies`).Scan(&count); err != nil {
		t.Fatalf("count policies: %v", err)
	}
	if count != 0 {
		t.Errorf("deleted policy survived the sync: count=%d", count)
	}
}

func TestDynamicUpdatePolicyProjectionColumnsMatchControl(t *testing.T) {
	control, store, _ := newZonePair(t)
	var projection *table
	tables := TablesFor(DomainDNS)
	for i := range tables {
		if tables[i].name == "dns_dynamic_update_policies" {
			projection = &tables[i]
			break
		}
	}
	if projection == nil {
		t.Fatal("dns_dynamic_update_policies is not in the DNS replication domain")
	}
	for _, col := range projection.columns {
		var name string
		if err := control.QueryRow(
			`SELECT name FROM pragma_table_info('dns_dynamic_update_policies') WHERE name = ?`, col,
		).Scan(&name); err != nil {
			t.Errorf("control dns_dynamic_update_policies has no column %q: %v", col, err)
		}
		if err := store.QueryRow(
			`SELECT name FROM pragma_table_info('dns_dynamic_update_policies') WHERE name = ?`, col,
		).Scan(&name); err != nil {
			t.Errorf("data-plane dns_dynamic_update_policies has no column %q: %v", col, err)
		}
	}
}
