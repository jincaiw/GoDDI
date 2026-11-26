package configrepl

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/jincaiw/GoDDI/internal/dataplane"
	_ "modernc.org/sqlite"
)

// minimal schemas covering a replace_whole table (dhcp_scopes) and a
// replace_control_authored table (dns_records), enough to exercise the copy.
const schemaScopes = `CREATE TABLE dhcp_scopes (
	id TEXT PRIMARY KEY, name TEXT, interface TEXT, subnet TEXT, start_ip TEXT,
	end_ip TEXT, subnet_mask TEXT, router TEXT, dns_servers TEXT, ntp_servers TEXT,
	domain_name TEXT, lease_time INTEGER, max_lease_time INTEGER, enabled INTEGER,
	ping_check_enabled INTEGER, dns_updates INTEGER, comment TEXT,
	created_at DATETIME, updated_at DATETIME)`

const schemaRecords = `CREATE TABLE dns_records (
	id TEXT PRIMARY KEY, zone_id TEXT, name TEXT, type TEXT, value TEXT, ttl INTEGER,
	priority INTEGER, weight INTEGER, port INTEGER, enabled INTEGER, comment TEXT,
	tags TEXT, tag TEXT, flag INTEGER, owner TEXT, owner_ref TEXT, owner_generation INTEGER,
	expires_at DATETIME, authored_locally INTEGER, created_at DATETIME, updated_at DATETIME)`

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func specs() []dataplane.ConfigTableSpec { return dataplane.ConfigTableSpecs() }

func specByName(name string) dataplane.ConfigTableSpec {
	for _, s := range specs() {
		if s.Name == name {
			return s
		}
	}
	panic("no spec " + name)
}

func seedScope(t *testing.T, db *sql.DB, id, name, subnet string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO dhcp_scopes
		(id,name,subnet,start_ip,end_ip,lease_time,enabled,ping_check_enabled,dns_updates)
		VALUES (?,?,?,?,?,?,?,?,?)`, id, name, subnet, "10.0.0.10", "10.0.0.20", 3600, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
}

// A scope authored on node-a must appear on an empty node-b after a snapshot
// round-trip -- the property that lets a promoted standby serve that scope.

// The operator-facing version is an int64. A raw uint64 hash cast into it
// makes half of all configurations appear as a negative version, which reads
// like a corrupt state even though the equality semantics are intact.
func TestFingerprintIsNonNegativeStableAndContentSensitive(t *testing.T) {
	tables := []TableData{{Name: "dhcp_scopes", Rows: [][]any{{"scope-1", "10.0.0.0/24"}}}}
	first := fingerprint(tables)
	if first < 0 {
		t.Fatalf("fingerprint = %d, want a non-negative operator-visible version", first)
	}
	if second := fingerprint(tables); second != first {
		t.Fatalf("fingerprint is unstable: %d then %d", first, second)
	}
	changed := []TableData{{Name: "dhcp_scopes", Rows: [][]any{{"scope-1", "10.0.1.0/24"}}}}
	if next := fingerprint(changed); next == first {
		t.Fatalf("different content kept fingerprint %d", first)
	}
	for i := 0; i < 256; i++ {
		name := fmt.Sprintf("scope-%d", i)
		v := fingerprint([]TableData{{Name: "dhcp_scopes", Rows: [][]any{{name, "10.0.0.0/24"}}}})
		if v < 0 {
			t.Fatalf("fingerprint for %s = %d, want non-negative", name, v)
		}
	}
}

func TestSnapshotCarriesScopeToEmptyFollower(t *testing.T) {
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaScopes); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := src.Exec(`CREATE TABLE config_release_outbox (id INTEGER PRIMARY KEY AUTOINCREMENT, revision_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO config_release_outbox(revision_id) VALUES ('r1'),('r2')`); err != nil {
		t.Fatal(err)
	}
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")

	snap, err := ReadSnapshot(context.Background(), src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version == 0 {
		t.Fatalf("version fingerprint = 0, want a non-zero content hash")
	}

	// Round-trip through JSON to prove the wire form is faithful.
	blob, err := EncodeSnapshotJSON(snap)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSnapshotJSON(blob)
	if err != nil {
		t.Fatal(err)
	}
	n, err := ApplySnapshot(context.Background(), dst, decoded, specs())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if n < 1 {
		t.Fatalf("applied %d rows, want >=1", n)
	}
	var name, subnet string
	if err := dst.QueryRow(`SELECT name, subnet FROM dhcp_scopes WHERE id='scope-1'`).Scan(&name, &subnet); err != nil {
		t.Fatalf("scope did not land on follower: %v", err)
	}
	if name != "lan" || subnet != "10.0.0.0/24" {
		t.Fatalf("scope content = %q/%q", name, subnet)
	}
}

// replace_control_authored: a locally-authored record on the follower must
// survive a snapshot, while the author's control-authored rows are replaced.
func TestSnapshotPreservesLocallyAuthoredRecords(t *testing.T) {
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaRecords); err != nil {
			t.Fatal(err)
		}
	}
	// Author has one control-authored record.
	if _, err := src.Exec(`INSERT INTO dns_records(id,zone_id,name,type,value,authored_locally) VALUES('a1','z1','www','A','192.0.2.1',0)`); err != nil {
		t.Fatal(err)
	}
	// Follower has one locally-authored (dynamic update) record + a stale
	// control-authored one that must be replaced.
	if _, err := dst.Exec(`INSERT INTO dns_records(id,zone_id,name,type,value,authored_locally) VALUES('b1','z1','dyn','A','192.0.2.9',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(`INSERT INTO dns_records(id,zone_id,name,type,value,authored_locally) VALUES('stale','z1','gone','A','192.0.2.8',0)`); err != nil {
		t.Fatal(err)
	}

	snap, err := ReadSnapshot(context.Background(), src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(context.Background(), dst, snap, specs()); err != nil {
		t.Fatal(err)
	}

	var dyn, www, stale int
	dst.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE id='b1'`).Scan(&dyn)
	dst.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE id='a1'`).Scan(&www)
	dst.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE id='stale'`).Scan(&stale)
	if dyn != 1 {
		t.Fatalf("locally-authored record was removed by replication")
	}
	if www != 1 {
		t.Fatalf("author control-authored record did not land")
	}
	if stale != 0 {
		t.Fatalf("stale control-authored record should be replaced")
	}
}

// Applying the same snapshot twice is idempotent.
func TestApplySnapshotIdempotent(t *testing.T) {
	src := newDB(t)
	dst := newDB(t)
	for _, db := range []*sql.DB{src, dst} {
		if _, err := db.Exec(schemaScopes); err != nil {
			t.Fatal(err)
		}
	}
	seedScope(t, src, "scope-1", "lan", "10.0.0.0/24")
	snap, err := ReadSnapshot(context.Background(), src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(context.Background(), dst, snap, specs()); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySnapshot(context.Background(), dst, snap, specs()); err != nil {
		t.Fatal(err)
	}
	var c int
	dst.QueryRow(`SELECT COUNT(*) FROM dhcp_scopes`).Scan(&c)
	if c != 1 {
		t.Fatalf("scope rows = %d after two applies, want 1", c)
	}
}
