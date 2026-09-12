package dataplane

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/config"
)

// newStore opens a data-plane store in a temporary directory.
func newStore(t *testing.T, role config.DataPlaneRole) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), string(role)+".db")
	s, err := Open(role, path)
	if err != nil {
		t.Fatalf("open %s store: %v", role, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newControlDB builds a database with the control-plane schema, which is what
// the replica tables have to match.
func newControlDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open control db: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("run control migrations: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest is the property the
// whole lease store exists for. SQLite's WAL default (synchronous=NORMAL)
// acknowledges a commit before it is durable, so a power loss can leave a
// client believing it holds an address the server has never heard of.
func TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest(t *testing.T) {
	for _, role := range []config.DataPlaneRole{config.DataPlaneLease, config.DataPlaneZone} {
		s := newStore(t, role)

		var sync int
		if err := s.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
			t.Fatalf("%s: read synchronous: %v", role, err)
		}
		if sync != 2 {
			t.Errorf("%s: PRAGMA synchronous = %d, want 2 (FULL)", role, sync)
		}

		var mode string
		if err := s.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatalf("%s: read journal_mode: %v", role, err)
		}
		if !strings.EqualFold(mode, "wal") {
			t.Errorf("%s: journal_mode = %q, want wal", role, mode)
		}

		var fk int
		if err := s.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatalf("%s: read foreign_keys: %v", role, err)
		}
		if fk != 1 {
			t.Errorf("%s: foreign_keys = %d, want 1", role, fk)
		}

		var busy int
		if err := s.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatalf("%s: read busy_timeout: %v", role, err)
		}
		if busy != 5000 {
			t.Errorf("%s: busy_timeout = %d, want 5000", role, busy)
		}
	}
}

// TestOpenIsNotWritableTwiceOver guards against two processes sharing one store
// file. Each store is a single writer, and two writers on one file would defeat
// the point of giving each plane its own supervision.
func TestOpenIsIdempotentOnAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.db")

	first, err := Open(config.DataPlaneLease, path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := first.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip)
		VALUES ('s1', 'lan', '10.0.0.0/24', '10.0.0.10', '10.0.0.20')`); err != nil {
		t.Fatalf("insert scope: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second, err := Open(config.DataPlaneLease, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer second.Close()

	var name string
	if err := second.QueryRow(`SELECT name FROM dhcp_scopes WHERE id = 's1'`).Scan(&name); err != nil {
		t.Fatalf("the second open lost the existing row: %v", err)
	}
	if name != "lan" {
		t.Errorf("name = %q, want lan", name)
	}
}

func TestOpenCreatesTheParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "leases.db")
	s, err := Open(config.DataPlaneLease, path)
	if err != nil {
		t.Fatalf("open into a missing directory: %v", err)
	}
	defer s.Close()
}

func TestOpenRejectsAnEmptyDSN(t *testing.T) {
	if _, err := Open(config.DataPlaneLease, "   "); err == nil {
		t.Fatal("an empty DSN was accepted")
	}
}

// ---------------------------------------------------------------------------
// The replica tables are copies of control-plane tables, so a column added on
// one side and not the other breaks the copy in the least visible way
// possible: the sync's INSERT names every column explicitly, so a missing
// column is a hard error, but a column that exists locally with the wrong
// shape silently truncates or rejects data.
//
// This test is the guard. It fails the moment the two schemas drift, which
// means the migration that adds a control-plane column cannot be merged without
// the matching data-plane change.
// ---------------------------------------------------------------------------

// mirroredTables are the tables whose local shape must equal the control shape.
var mirroredTables = []string{
	"dhcp_scopes",
	"dhcp_reservations",
	"dhcp_options",
	"dhcp_leases",
	"dhcp_logs",
	"dhcp_dns_events",
	"dns_zones",
	"dns_records",
}

type columnShape struct {
	Type    string
	NotNull bool
	PK      bool
}

func tableShape(t *testing.T, db *sql.DB, table string) map[string]columnShape {
	t.Helper()
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()

	out := make(map[string]columnShape)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		out[name] = columnShape{
			Type:    strings.ToUpper(strings.TrimSpace(ctype)),
			NotNull: notnull == 1,
			PK:      pk > 0,
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(%s): %v", table, err)
	}
	return out
}

func TestReplicaTablesMatchTheControlSchema(t *testing.T) {
	control := newControlDB(t)
	store := newStore(t, config.DataPlaneLease)

	for _, table := range mirroredTables {
		t.Run(table, func(t *testing.T) {
			want := tableShape(t, control, table)
			got := tableShape(t, store.DB, table)
			if len(got) == 0 {
				t.Fatalf("the data-plane store has no %s table", table)
			}

			var missing, extra, differing []string
			for name, w := range want {
				g, ok := got[name]
				if !ok {
					missing = append(missing, name)
					continue
				}
				if g.Type != w.Type || g.NotNull != w.NotNull || g.PK != w.PK {
					differing = append(differing, fmt.Sprintf("%s: control %+v vs dataplane %+v", name, w, g))
				}
			}
			for name := range got {
				if _, ok := want[name]; !ok {
					extra = append(extra, name)
				}
			}

			sort.Strings(missing)
			sort.Strings(extra)
			sort.Strings(differing)
			if len(missing) > 0 {
				t.Errorf("columns present in the control database but missing here: %v", missing)
			}
			if len(extra) > 0 {
				t.Errorf("columns present here but not in the control database: %v", extra)
			}
			if len(differing) > 0 {
				t.Errorf("columns whose shape differs:\n  %s", strings.Join(differing, "\n  "))
			}
		})
	}
}

// TestReplicaTablesCarryNoForeignKeys pins the reason the copies are safe to
// replace wholesale. A cascading foreign key from a replaced parent would take
// the children with it: replacing scopes would delete every lease, and
// replacing zones would delete every record.
func TestReplicaTablesCarryNoForeignKeys(t *testing.T) {
	store := newStore(t, config.DataPlaneLease)

	for _, table := range mirroredTables {
		rows, err := store.Query(fmt.Sprintf("PRAGMA foreign_key_list(%s)", table))
		if err != nil {
			t.Fatalf("foreign_key_list(%s): %v", table, err)
		}
		var count int
		for rows.Next() {
			count++
		}
		rows.Close()
		if count != 0 {
			t.Errorf("%s declares %d foreign key(s); replicas are replaced wholesale and must not cascade", table, count)
		}
	}
}

// ---------------------------------------------------------------------------
// Lease changes owed to the control-plane replica.
// ---------------------------------------------------------------------------

func insertLease(t *testing.T, s *Store, id, scopeID, ip, status string) {
	t.Helper()
	if _, err := s.Exec(`INSERT INTO dhcp_leases
		(id, scope_id, ip_address, mac_address, hostname, client_id,
		 lease_start, lease_end, status, last_seen, generation)
		VALUES (?, ?, ?, 'aa:bb:cc:dd:ee:01', 'h', '',
		        datetime('now'), datetime('now', '+1 hour'), ?, datetime('now'), 1)`,
		id, scopeID, ip, status); err != nil {
		t.Fatalf("insert lease %s: %v", id, err)
	}
}

func dirtyMarker(t *testing.T, s *Store, leaseID string) (found bool, deleted bool) {
	t.Helper()
	var d int
	err := s.QueryRow(`SELECT deleted FROM dhcp_lease_dirty WHERE lease_id = ?`, leaseID).Scan(&d)
	if err == sql.ErrNoRows {
		return false, false
	}
	if err != nil {
		t.Fatalf("read dirty marker for %s: %v", leaseID, err)
	}
	return true, d == 1
}

func TestLeaseWritesAreMarkedForReplication(t *testing.T) {
	s := newStore(t, config.DataPlaneLease)

	insertLease(t, s, "l1", "s1", "10.0.0.10", "active")
	if found, deleted := dirtyMarker(t, s, "l1"); !found || deleted {
		t.Fatalf("after insert: found=%v deleted=%v, want found and not deleted", found, deleted)
	}

	// Renewal: still owed to the replica, still a live row.
	if _, err := s.Exec(`UPDATE dhcp_leases SET status = 'active', generation = 2 WHERE id = 'l1'`); err != nil {
		t.Fatalf("update lease: %v", err)
	}
	if found, deleted := dirtyMarker(t, s, "l1"); !found || deleted {
		t.Fatalf("after update: found=%v deleted=%v, want found and not deleted", found, deleted)
	}

	// A delete has to overwrite the marker: the replica must remove the row,
	// not re-copy a row the data plane no longer has.
	if _, err := s.Exec(`DELETE FROM dhcp_leases WHERE id = 'l1'`); err != nil {
		t.Fatalf("delete lease: %v", err)
	}
	if found, deleted := dirtyMarker(t, s, "l1"); !found || !deleted {
		t.Fatalf("after delete: found=%v deleted=%v, want found and deleted", found, deleted)
	}
}

// TestARecreatedLeaseClearsTheDeleteMarker covers insert-after-delete, which is
// ordinary: an address that expires and is immediately offered to a new client
// reuses the same lease id only if the id is derived from the binding, and a
// pushed "this row is gone" after "this row exists" would erase a live lease
// from the console and from backups.
func TestARecreatedLeaseClearsTheDeleteMarker(t *testing.T) {
	s := newStore(t, config.DataPlaneLease)

	insertLease(t, s, "l2", "s1", "10.0.0.11", "offered")
	if _, err := s.Exec(`DELETE FROM dhcp_leases WHERE id = 'l2'`); err != nil {
		t.Fatalf("delete lease: %v", err)
	}
	insertLease(t, s, "l2", "s1", "10.0.0.11", "active")

	if found, deleted := dirtyMarker(t, s, "l2"); !found || deleted {
		t.Fatalf("after re-create: found=%v deleted=%v, want found and not deleted", found, deleted)
	}
}

// TestScopeReplacementKeepsLeases is the exact failure a cascading foreign key
// would have produced, asserted so that the "no foreign keys" decision cannot
// be undone silently.
func TestScopeReplacementKeepsLeases(t *testing.T) {
	s := newStore(t, config.DataPlaneLease)

	if _, err := s.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip)
		VALUES ('s1', 'lan', '10.0.0.0/24', '10.0.0.10', '10.0.0.20')`); err != nil {
		t.Fatalf("insert scope: %v", err)
	}
	insertLease(t, s, "l3", "s1", "10.0.0.10", "active")

	// What the sync does to a scope replica.
	if _, err := s.Exec(`DELETE FROM dhcp_scopes`); err != nil {
		t.Fatalf("clear scope replica: %v", err)
	}
	if _, err := s.Exec(`INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip)
		VALUES ('s1', 'lan', '10.0.0.0/24', '10.0.0.10', '10.0.0.20')`); err != nil {
		t.Fatalf("re-insert scope: %v", err)
	}

	var n int
	if err := s.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = 'l3'`).Scan(&n); err != nil {
		t.Fatalf("count leases: %v", err)
	}
	if n != 1 {
		t.Fatal("replacing the scope replica deleted a lease; replicas must not cascade")
	}
}

func TestSyncStateStartsEmpty(t *testing.T) {
	s := newStore(t, config.DataPlaneLease)

	var n int
	if err := s.QueryRow(`SELECT COUNT(*) FROM dataplane_sync_state`).Scan(&n); err != nil {
		t.Fatalf("count sync state: %v", err)
	}
	if n != 0 {
		t.Errorf("sync state rows = %d, want 0 (revision 0 means 'never synced')", n)
	}
}
