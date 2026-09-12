package address

import (
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

// ipam_history.space_id is denormalised from the subnet, and every read of the
// history goes through it: HistoryFor filters on `space_id = ? AND ip_address
// = ?`. That makes an unset space_id a silent data loss rather than a visible
// error -- the row is written, the change happened, and the trail says nothing.
//
// There are four write paths and they all set it today. "They all do it today"
// is what this file stops being the only guarantee of.
func TestEveryHistoryWritePathRecordsTheSpace(t *testing.T) {
	db := newTestDB(t)
	seedSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedSubnet(t, db, "sp2", "sn2", "198.51.100.0/24")
	m := NewManager(db)

	// 1. Explicit allocation -> allocateSpecific.
	if _, err := m.AllocateIP(AllocateRequest{
		SubnetID: "sn1", IPAddress: "192.0.2.10", Status: StatusStatic, Actor: "tester",
	}); err != nil {
		t.Fatalf("explicit allocate: %v", err)
	}

	// 2. Automatic allocation -> allocateAuto.
	if _, err := m.AllocateIP(AllocateRequest{
		SubnetID: "sn1", Status: StatusUsed, Actor: "tester",
	}); err != nil {
		t.Fatalf("auto allocate: %v", err)
	}

	// 3. Status transition -> TransitionAddress. `conflict` is one of the
	//    states a `static` address may move to, so this exercises the write
	//    path rather than the transition table.
	a, err := m.GetAddressBySpaceIP("sp1", "192.0.2.10")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if _, err := m.TransitionAddress(a.ID, StatusConflict, "tester", "two parties claimed it", SourceAdmin); err != nil {
		t.Fatalf("transition: %v", err)
	}

	// 4. Data-plane observation -> ObserveBySpaceIP. The observation has to
	//    imply a change, or no history row is written at all and this path
	//    would be silently skipped: `available` observed as in use is the
	//    ordinary DHCP case, and one of the few that always records.
	insertAddress(t, db, "sp1", "sn1", "192.0.2.12", StatusAvailable)
	if _, err := m.ObserveBySpaceIP("sp1", "192.0.2.12", Observation{
		State: ObservedInUse, Source: SourceDHCP, Actor: "dhcp",
	}); err != nil {
		t.Fatalf("observe: %v", err)
	}

	// The direct assertion: no row anywhere is missing its space. A per-path
	// assertion would have to be repeated for every future path; this one does
	// not.
	var blank int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM ipam_history WHERE space_id IS NULL OR space_id = ''`,
	).Scan(&blank); err != nil {
		t.Fatalf("count blank: %v", err)
	}
	if blank != 0 {
		var total int
		_ = db.QueryRow(`SELECT COUNT(*) FROM ipam_history`).Scan(&total)
		t.Fatalf("%d of %d history rows have no space_id: the change happened and "+
			"HistoryFor can never return it", blank, total)
	}

	// The reverse direction: every row is attributed to the space it came
	// from, and nothing leaked into the other space.
	rows, err := db.Query(
		`SELECT space_id, COUNT(*) FROM ipam_history GROUP BY space_id ORDER BY space_id`)
	if err != nil {
		t.Fatalf("group history: %v", err)
	}
	defer rows.Close()
	type spaceCount struct {
		SpaceID string
		Rows    int
	}
	var groups []spaceCount
	for rows.Next() {
		var g spaceCount
		if err := rows.Scan(&g.SpaceID, &g.Rows); err != nil {
			t.Fatal(err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].SpaceID != "sp1" || groups[0].Rows != 4 {
		t.Fatalf("history grouped by space = %+v, want one group {sp1 4}", groups)
	}

	// And each kind of change is reachable through the space-keyed lookup.
	seen := map[string]bool{}
	reached, err := m.HistoryFor("sp1", "192.0.2.10", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range reached {
		seen[h.Action] = true
	}
	observed, err := m.HistoryFor("sp1", "192.0.2.12", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range observed {
		seen[h.Action] = true
	}
	for _, action := range []string{"allocate", "transition", "observe"} {
		if !seen[action] {
			t.Errorf("no %q row was reachable through HistoryFor: reached=%+v observed=%+v",
				action, reached, observed)
		}
	}
}

// The behaviour above holds because every history write goes through one
// function with a SpaceID field. A second INSERT written directly against the
// table would bypass that, and nothing else in the codebase would notice.
//
// So the count is pinned here. The fix when this fails is not to add the file
// to an allow-list: it is to call insertHistoryTx.
func TestTheOnlyProductionWriterOfIPAMHistoryIsTheOneThatSetsTheSpace(t *testing.T) {
	// The test runs with the package directory as its working directory.
	root := ".."
	insert := regexp.MustCompile(`(?i)INSERT\s+INTO\s+ipam_history`)

	writers := map[string]int{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if n := len(insert.FindAll(raw, -1)); n > 0 {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			writers[filepath.ToSlash(rel)] += n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", root, err)
	}

	if len(writers) != 1 || writers["address/address.go"] != 1 {
		t.Fatalf("ipam_history is inserted from %v; it must be written only by "+
			"insertHistoryTx in address/address.go, which is the function that "+
			"sets space_id -- a second INSERT bypasses it and the row becomes "+
			"invisible to HistoryFor", writers)
	}
}

// databaseAtVersion opens an in-memory database migrated to exactly `version`,
// so a later migration can be applied to data laid out the way that migration
// expects to find it.
func databaseAtVersion(t *testing.T, version int64) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { db.Close() })

	// The embedded filesystem has to stay installed for the whole test: the
	// caller applies a later migration by hand, and goose resolves it through
	// this global.
	goose.SetBaseFS(goddiassets.Migrations())
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.UpTo(db, ".", version); err != nil {
		t.Fatalf("migrate to %d: %v", version, err)
	}
	return db
}

// Migration 020 backfills space_id from `ipam_subnets`, one row at a time. A
// scalar subquery that matches no row yields NULL, and the column is NOT NULL,
// so a history row whose subnet is gone aborts the migration -- and a failed
// migration is a server that will not start, on a database whose only defect
// is a few unattributable history rows.
//
// Such a row is reachable. ipam_history.subnet_id carries ON DELETE CASCADE,
// but SQLite enforces that only when the foreign_keys pragma is on, and it is
// per connection and off by default: any delete issued through the sqlite3 CLI,
// or on a connection that never had the pragma applied, leaves the row behind.
// The application's own delete path removes the history rows explicitly, which
// is the reason this is rare rather than impossible -- and rare is exactly the
// case that reaches production untested.
//
// This test runs the real migration against a real pre-020 database rather than
// restating its SQL, so the guard cannot drift from the migration it guards.
func TestMigration020UpgradesADatabaseWithAnUnattributableHistoryRow(t *testing.T) {
	db := databaseAtVersion(t, 19)

	if _, err := db.Exec(
		`INSERT INTO ipam_spaces (id, name) VALUES ('sp1', 'space-sp1')`); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES ('sn1', 'sp1', 'sn1', '192.0.2.0/24')`,
	); err != nil {
		t.Fatalf("seed subnet: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO ipam_history (id, subnet_id, ip_address, action) VALUES ('h1', 'sn1', '192.0.2.5', 'allocate')`,
	); err != nil {
		t.Fatalf("seed attributable history: %v", err)
	}
	// The orphan. Nothing enabled the foreign_keys pragma on this connection,
	// so the cascade that would have removed this row never ran.
	if _, err := db.Exec(
		`INSERT INTO ipam_history (id, subnet_id, ip_address, action) VALUES ('h2', 'sn-gone', '192.0.2.6', 'allocate')`,
	); err != nil {
		t.Fatalf("seed unattributable history: %v", err)
	}

	if err := goose.UpTo(db, ".", 20); err != nil {
		t.Fatalf("migration 020 refuses a database holding a history row whose "+
			"subnet is gone, so the upgrade is blocked rather than completed: %v", err)
	}

	// The rows that can be attributed still are.
	var space string
	if err := db.QueryRow(`SELECT space_id FROM ipam_history WHERE id = 'h1'`).Scan(&space); err != nil {
		t.Fatalf("read backfilled row: %v", err)
	}
	if space != "sp1" {
		t.Errorf("attributable row space_id = %q, want %q", space, "sp1")
	}

	// The unattributable one keeps the empty string. NULL would mean the
	// backfill tried to write a value the column rejects, which is the failure
	// this test exists to prevent.
	var orphan sql.NullString
	if err := db.QueryRow(`SELECT space_id FROM ipam_history WHERE id = 'h2'`).Scan(&orphan); err != nil {
		t.Fatalf("read unattributable row: %v", err)
	}
	if !orphan.Valid {
		t.Fatal("the unattributable row's space_id is NULL; the column is NOT NULL and the migration should have written the empty string")
	}
	if orphan.String != "" {
		t.Errorf("unattributable row space_id = %q, want the empty string: there is no "+
			"space to attribute it to, and borrowing one would put another space's "+
			"changes in this space's history", orphan.String)
	}
}

// Migration 020 narrows the address identity from (subnet, ip) to (space, ip),
// and the new unique index is the point of the change. It is also the second
// way the migration can fail on data it did not create: two rows that were
// distinct under (subnet, ip) collapse into a duplicate.
//
// Migration 020's own comment names the way that happens -- "several subnets in
// one space may cover the same address across a resize". This test establishes
// whether that state is reachable, because if it is, an operator upgrading a
// real deployment meets a failed migration rather than an error message naming
// the rows to fix.
func TestMigration020CollapsesDuplicateAddressesOrRefusesThemClearly(t *testing.T) {
	db := databaseAtVersion(t, 19)

	if _, err := db.Exec(
		`INSERT INTO ipam_spaces (id, name) VALUES ('sp1', 'space-sp1')`); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	// Two subnets in one space, both covering 192.0.2.7. Pre-020 the unique
	// key was (subnet_id, ip_address), so one address row in each was legal.
	for _, sn := range []struct{ id, cidr string }{
		{"sn-a", "192.0.2.0/24"},
		{"sn-b", "192.0.2.0/25"},
	} {
		if _, err := db.Exec(
			`INSERT INTO ipam_subnets (id, space_id, name, cidr) VALUES (?, 'sp1', ?, ?)`,
			sn.id, sn.id, sn.cidr); err != nil {
			t.Fatalf("seed subnet %s: %v", sn.id, err)
		}
	}
	for i, sn := range []string{"sn-a", "sn-b"} {
		if _, err := db.Exec(`
			INSERT INTO ipam_addresses (id, subnet_id, ip_address, status)
			VALUES (?, ?, '192.0.2.7', 'used')`, "addr-"+sn, sn); err != nil {
			t.Fatalf("seed address %d: %v", i, err)
		}
	}
	// Sanity: the state is legal under the old key, so the failure below is the
	// migration's and not the fixture's.
	var pre int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM ipam_addresses WHERE space_id = '' OR space_id IS NULL`,
	).Scan(&pre); err != nil {
		// Pre-020 the column does not exist at all; that is the expected path.
		pre = -1
	}
	if pre != -1 {
		t.Fatalf("pre-020 database already has a space_id column (counted %d)", pre)
	}

	err := goose.UpTo(db, ".", 20)
	if err == nil {
		// The migration resolved the collision itself. Whatever it did, the
		// result must not contain two rows claiming the same address.
		var dup int
		if scanErr := db.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT space_id, ip_address FROM ipam_addresses
				GROUP BY space_id, ip_address HAVING COUNT(*) > 1)`,
		).Scan(&dup); scanErr != nil {
			t.Fatalf("count duplicates: %v", scanErr)
		}
		if dup != 0 {
			t.Fatalf("migration 020 succeeded but left %d duplicated (space, ip) "+
				"pairs, which the unique index it creates forbids", dup)
		}
		return
	}

	// It refused. That is acceptable only if the error names the conflict
	// rather than reporting a bare constraint failure, because the operator's
	// next step is to decide which row to keep and the message is all they get.
	t.Logf("migration 020 refused the upgrade: %v", err)
	if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Errorf("migration 020 failed for a duplicate (space, ip) with an error that "+
			"does not mention uniqueness, so the operator cannot tell this apart "+
			"from any other migration failure: %v", err)
	}
}
