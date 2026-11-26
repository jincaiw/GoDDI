package configrepl

import (
	"context"
	"github.com/jincaiw/GoDDI/internal/dataplane"
	"testing"
)

// A follower missing a non-key replicated column must fail before writing.
// SQLite would otherwise treat the missing double-quoted identifier in the
// upsert as a string literal and silently copy that literal into the row.
func TestApplySnapshotRejectsFollowerMissingColumn(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	if _, err := src.Exec(schemaScopes); err != nil {
		t.Fatal(err)
	}
	// Follower table exists but lacks "interface" and several other columns.
	if _, err := dst.Exec(`CREATE TABLE dhcp_scopes (id TEXT PRIMARY KEY, name TEXT, subnet TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(`INSERT INTO dhcp_scopes(id,name,subnet) VALUES ('keep','keep','10.0.0.0/24')`); err != nil {
		t.Fatal(err)
	}
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")
	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(ctx, dst, snap, specs()); err == nil {
		t.Fatal("apply must reject a follower table missing a replicated column")
	}
	var name string
	if err := dst.QueryRow(`SELECT name FROM dhcp_scopes WHERE id='keep'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "keep" {
		t.Fatalf("pre-existing row changed to %q", name)
	}
}

// replace_control_authored pruning reads authored_locally; a follower schema
// without it must fail closed instead of skipping the protection.
func TestApplySnapshotRejectsMissingAuthoredLocally(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	if _, err := src.Exec(schemaRecords); err != nil {
		t.Fatal(err)
	}
	// Same table without the protection column.
	if _, err := dst.Exec(`CREATE TABLE dns_records (id TEXT PRIMARY KEY, zone_id TEXT, name TEXT, type TEXT, value TEXT, ttl INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO dns_records(id,zone_id,name,type,value,authored_locally) VALUES('r1','z1','www','A','192.0.2.1',0)`); err != nil {
		t.Fatal(err)
	}
	snap, err := ReadSnapshot(ctx, src, "node-a", []dataplane.ConfigTableSpec{specByName("dns_records")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(ctx, dst, snap, []dataplane.ConfigTableSpec{specByName("dns_records")}); err == nil {
		t.Fatal("apply must reject a missing authored_locally column")
	}
}
