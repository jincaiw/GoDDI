package configrepl

import (
	"context"
	"database/sql"
	"testing"
)

// controlIPAMWithChildren is the real control-DB schema for the IPAM family:
// ipam_addresses and ipam_history are NOT in ConfigTableSpecs (not replicated),
// yet both reference ipam_subnets with ON DELETE CASCADE. Applying a snapshot
// must not destroy this local-only data.
const controlIPAMWithChildren = controlIPAMSchema + `
CREATE TABLE ipam_addresses (
	id TEXT PRIMARY KEY,
	subnet_id TEXT NOT NULL REFERENCES ipam_subnets(id) ON DELETE CASCADE,
	ip_address TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'available',
	mac_address TEXT,
	hostname TEXT,
	created_at DATETIME NOT NULL DEFAULT (datetime('now')),
	updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE ipam_history (
	id TEXT PRIMARY KEY,
	subnet_id TEXT NOT NULL REFERENCES ipam_subnets(id) ON DELETE CASCADE,
	ip_address TEXT NOT NULL,
	action TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

// enableFK mirrors the real control DB, which enforces ON DELETE CASCADE.
func enableFK(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
}

func seedIPAMParentChild(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO ipam_spaces(id,name,description) VALUES ('sp-1','lab','space')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_subnets(id,space_id,name,cidr,vlan_id,location,description)
		VALUES ('sn-1','sp-1','lan','10.77.3.0/24',130,'h2','subnet')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_addresses(id,subnet_id,ip_address,status,hostname)
		VALUES ('addr-1','sn-1','10.77.3.10','assigned','host-a')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_history(id,subnet_id,ip_address,action)
		VALUES ('hist-1','sn-1','10.77.3.10','assign')`); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Evidence test: applying an UNCHANGED snapshot must not delete local-only
// ipam_addresses/ipam_history via ON DELETE CASCADE on replicated parents.
func TestApplySnapshotPreservesNonReplicatedIPAMChildren(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		enableFK(t, db)
		if _, err := db.Exec(controlIPAMWithChildren); err != nil {
			t.Fatal(err)
		}
	}
	seedIPAMParentChild(t, src)
	seedIPAMParentChild(t, dst)

	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(ctx, dst, snap, specs()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if got := countRows(t, dst, "ipam_addresses"); got != 1 {
		t.Fatalf("local ipam_addresses = %d after unchanged snapshot; want 1 (ON DELETE CASCADE fired on replicated parent)", got)
	}
	if got := countRows(t, dst, "ipam_history"); got != 1 {
		t.Fatalf("local ipam_history = %d after unchanged snapshot; want 1 (ON DELETE CASCADE fired on replicated parent)", got)
	}
}

// An author-side edit of an existing subnet (name/vlan change) must update the
// follower row in place, not delete+reinsert it (which would cascade-delete
// local children even though the subnet id is unchanged).
func TestApplySnapshotUpdateKeepsNonReplicatedChildren(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		enableFK(t, db)
		if _, err := db.Exec(controlIPAMWithChildren); err != nil {
			t.Fatal(err)
		}
	}
	seedIPAMParentChild(t, src)
	seedIPAMParentChild(t, dst)
	if _, err := src.Exec(`UPDATE ipam_subnets SET name='lan-v2', vlan_id=131 WHERE id='sn-1'`); err != nil {
		t.Fatal(err)
	}

	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(ctx, dst, snap, specs()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var name string
	var vlan int
	if err := dst.QueryRow(`SELECT name,vlan_id FROM ipam_subnets WHERE id='sn-1'`).Scan(&name, &vlan); err != nil {
		t.Fatal(err)
	}
	if name != "lan-v2" || vlan != 131 {
		t.Fatalf("subnet = %q/%d, want lan-v2/131", name, vlan)
	}
	if got := countRows(t, dst, "ipam_addresses"); got != 1 {
		t.Fatalf("local addresses lost on in-place update: %d", got)
	}
	if got := countRows(t, dst, "ipam_history"); got != 1 {
		t.Fatalf("local history lost on in-place update: %d", got)
	}
}

// A retire transition on the author must leave local-only children intact.
// The parent row stays in the snapshot as a tombstone, so the follower upserts
// the lifecycle state instead of pruning the parent and firing ON DELETE CASCADE.
func TestApplySnapshotRetiredParentPreservesNonReplicatedChildren(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		enableFK(t, db)
		if _, err := db.Exec(controlIPAMWithChildren); err != nil {
			t.Fatal(err)
		}
	}
	seedIPAMParentChild(t, src)
	seedIPAMParentChild(t, dst)
	if _, err := src.Exec(`UPDATE ipam_subnets SET lifecycle_state='retired',
		retired_at=datetime('now'), retired_by='tester', retire_reason='archive'
		WHERE id='sn-1'`); err != nil {
		t.Fatal(err)
	}

	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(ctx, dst, snap, specs()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var state string
	if err := dst.QueryRow(`SELECT lifecycle_state FROM ipam_subnets WHERE id='sn-1'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "retired" {
		t.Fatalf("follower lifecycle state = %q, want retired", state)
	}
	if got := countRows(t, dst, "ipam_addresses"); got != 1 {
		t.Fatalf("local addresses lost after retire: %d", got)
	}
	if got := countRows(t, dst, "ipam_history"); got != 1 {
		t.Fatalf("local history lost after retire: %d", got)
	}
}
