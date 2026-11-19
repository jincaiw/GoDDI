package configrepl

import (
	"context"
	"database/sql"
	"testing"
)

// Control-DB schema for the IPAM parent/child pair. This is intentionally NOT
// the minimal data-plane projection: ipam_subnets.name is NOT NULL with no
// default, which is exactly the constraint a 3-column replica used to violate.
const controlIPAMSchema = `
CREATE TABLE ipam_spaces (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	lifecycle_state TEXT NOT NULL DEFAULT 'active',
	retired_at DATETIME,
	retired_by TEXT NOT NULL DEFAULT '',
	retire_reason TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT (datetime('now')),
	updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE ipam_subnets (
	id TEXT PRIMARY KEY,
	space_id TEXT NOT NULL REFERENCES ipam_spaces(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	cidr TEXT NOT NULL,
	vlan_id INTEGER,
	location TEXT,
	description TEXT,
	lifecycle_state TEXT NOT NULL DEFAULT 'active',
	retired_at DATETIME,
	retired_by TEXT NOT NULL DEFAULT '',
	retire_reason TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT (datetime('now')),
	updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

// Regression for the follower apply failure:
//
//	"configrepl: inserting into ipam_subnets: constraint failed:
//	 NOT NULL constraint failed: ipam_subnets.name (1299)"
//
// The whole snapshot must land, including the previously omitted columns.
func TestSnapshotCarriesFullIPAMSubnetRow(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(controlIPAMSchema); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := src.Exec(`INSERT INTO ipam_spaces(id, name, description) VALUES ('sp-1', 'lab', 'demo space')`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO ipam_subnets
		(id, space_id, name, cidr, vlan_id, location, description)
		VALUES ('sn-1', 'sp-1', 'lan', '10.77.3.0/24', 130, 'h2', 'demo subnet')`); err != nil {
		t.Fatal(err)
	}

	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	blob, err := EncodeSnapshotJSON(snap)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeSnapshotJSON(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	n, err := ApplySnapshot(ctx, dst, decoded, specs())
	if err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	if n < 2 {
		t.Fatalf("applied %d rows, want the space and the subnet", n)
	}

	var name, cidr, location, description string
	var vlan int
	if err := dst.QueryRow(`SELECT name, cidr, vlan_id, location, description
		FROM ipam_subnets WHERE id='sn-1'`).Scan(&name, &cidr, &vlan, &location, &description); err != nil {
		t.Fatalf("subnet did not land on follower: %v", err)
	}
	if name != "lan" || cidr != "10.77.3.0/24" || vlan != 130 || location != "h2" || description != "demo subnet" {
		t.Fatalf("follower subnet = %q/%q/%d/%q/%q", name, cidr, vlan, location, description)
	}
	var spaceName string
	if err := dst.QueryRow(`SELECT name FROM ipam_spaces WHERE id='sp-1'`).Scan(&spaceName); err != nil {
		t.Fatalf("parent space did not land on follower: %v", err)
	}
	if spaceName != "lab" {
		t.Fatalf("follower space name = %q", spaceName)
	}
}

// A follower whose control DB has the real NOT NULL constraint must never
// silently lose the replica: an error must be returned, not a partial apply.
func TestSnapshotApplyIsAtomicOnConstraintFailure(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	if _, err := src.Exec(controlIPAMSchema); err != nil {
		t.Fatal(err)
	}
	// Follower intentionally has an extra NOT NULL column the author does not
	// know about, so any partial insert must fail and roll back.
	if _, err := dst.Exec(`CREATE TABLE ipam_spaces (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT,
		created_at DATETIME, updated_at DATETIME);`); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(`CREATE TABLE ipam_subnets (
		id TEXT PRIMARY KEY, space_id TEXT NOT NULL, name TEXT NOT NULL,
		cidr TEXT NOT NULL, vlan_id INTEGER, location TEXT, description TEXT,
		created_at DATETIME, updated_at DATETIME,
		mandatory_extra TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO ipam_spaces(id,name) VALUES ('sp-1','lab')`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO ipam_subnets(id,space_id,name,cidr) VALUES ('sn-1','sp-1','lan','10.77.3.0/24')`); err != nil {
		t.Fatal(err)
	}
	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(ctx, dst, snap, specs()); err == nil {
		t.Fatal("apply must fail when a NOT NULL column is missing")
	}
	var count int
	if err := dst.QueryRow(`SELECT count(*) FROM ipam_spaces`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed apply left %d parent rows behind; transaction was not rolled back", count)
	}
}
