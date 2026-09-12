package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	goddiassets "github.com/jasonwa/goddi"
)

// migrationNames is a small migration set written by hand so the parsing rules
// can be asserted without depending on how many migrations the project happens
// to ship today. The names are deliberately out of order, one does not follow
// the rule, and one version appears twice.
func migrationNames() fstest.MapFS {
	return fstest.MapFS{
		"010_dns_v014.sql":                 &fstest.MapFile{},
		"001_init_auth_tables.sql":         &fstest.MapFile{},
		"003_init_dhcp_tables.sql":         &fstest.MapFile{},
		"002_init_dns_tables.sql":          &fstest.MapFile{},
		"009_add_backup_description.sql":   &fstest.MapFile{},
		"010_second_file_for_same_version": &fstest.MapFile{},
		"011_dns_zone_acl.sql":             &fstest.MapFile{},
		"notes.md":                         &fstest.MapFile{},
		"022_record_authorship.sql":        &fstest.MapFile{},
	}
}

// TestShippedMigrationsReadsTheNumericPrefix checks that versions are compared
// as numbers. A directory listing read as strings puts 010 before 002, and a
// report built that way would name the wrong migration as the newest one.
func TestShippedMigrationsReadsTheNumericPrefix(t *testing.T) {
	versions, err := ShippedMigrations(migrationNames())
	if err != nil {
		t.Fatalf("ShippedMigrations: %v", err)
	}
	want := []int64{1, 2, 3, 9, 10, 11, 22}
	if len(versions) != len(want) {
		t.Fatalf("versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("versions = %v, want %v", versions, want)
		}
	}
}

// TestAFreshDatabaseReportsEveryMigrationPending checks the state of a
// database that has been created but never migrated. It must be distinguishable
// from a migrated database, and from one that has no file at all.
func TestAFreshDatabaseReportsEveryMigrationPending(t *testing.T) {
	conn := openMemory(t)

	status := MigrationStatusOf(conn, migrationNames(), "control")

	if !status.FilePresent {
		t.Error("the connection was open, so the file is there")
	}
	if status.TablePresent {
		t.Error("a database with no goose table has never been migrated")
	}
	if status.Applied != 0 {
		t.Errorf("applied = %d, want 0", status.Applied)
	}
	if status.Known != 22 {
		t.Errorf("known = %d, want 22", status.Known)
	}
	if len(status.Pending) != 7 {
		t.Errorf("pending = %v, want all seven shipped versions", status.Pending)
	}
	if status.UpToDate() {
		t.Error("a database with everything pending is not up to date")
	}
}

// TestAMigratedDatabaseReportsNothingPending is the ordinary case after
// `goddi migrate`, including for the project's real migration set.
func TestAMigratedDatabaseReportsNothingPending(t *testing.T) {
	conn := newMigratedDB(t)

	status := MigrationStatusOf(conn, goddiassets.Migrations(), "control")

	if !status.TablePresent {
		t.Fatal("a migrated database has a goose table")
	}
	if len(status.Pending) != 0 {
		t.Errorf("pending = %v, want none", status.Pending)
	}
	if len(status.Withdrawn) != 0 {
		t.Errorf("withdrawn = %v, want none", status.Withdrawn)
	}
	if status.Applied != status.Known {
		t.Errorf("applied = %d, known = %d; a fully migrated database is at the ceiling",
			status.Applied, status.Known)
	}
	if !status.UpToDate() {
		t.Error("a fully migrated database is up to date")
	}
}

// TestARolledBackMigrationIsReportedPendingAgain is the case a naive
// implementation gets wrong.
//
// goose's version table is append-only: rolling a migration back writes a
// second row for the same version rather than removing the first. Reading
// MAX(version_id) therefore reports rolled-back work as present, and a status
// report that claims work is applied when it is not is worse than no report.
func TestARolledBackMigrationIsReportedPendingAgain(t *testing.T) {
	conn := newMigratedDB(t)

	shipped, err := ShippedMigrations(goddiassets.Migrations())
	if err != nil {
		t.Fatalf("ShippedMigrations: %v", err)
	}
	target := shipped[len(shipped)-3]

	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.DownTo(conn, ".", target); err != nil {
		t.Fatalf("rolling back to %d: %v", target, err)
	}

	status := MigrationStatusOf(conn, goddiassets.Migrations(), "control")

	if status.Applied != target {
		t.Errorf("applied = %d, want %d; the rollback records were read as applied work",
			status.Applied, target)
	}
	if len(status.Pending) == 0 {
		t.Fatal("the rolled-back migrations are pending; a report that says otherwise is wrong")
	}
	if len(status.Pending) != 2 {
		t.Errorf("pending = %v, want the two versions above %d", status.Pending, target)
	}
	if status.UpToDate() {
		t.Error("a database with rolled-back migrations is not up to date")
	}
}

// TestAVersionThisBuildDoesNotShipIsReportedAsWithdrawn covers the state a
// restore of a newer backup onto an older binary leaves behind.
//
// goose ignores versions above its own ceiling, so nothing fails and nothing
// is logged -- the only way an operator finds out is if something reports it.
func TestAVersionThisBuildDoesNotShipIsReportedAsWithdrawn(t *testing.T) {
	conn := newMigratedDB(t)

	if _, err := conn.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (99, 1)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	status := MigrationStatusOf(conn, goddiassets.Migrations(), "control")

	if len(status.Withdrawn) != 1 || status.Withdrawn[0] != 99 {
		t.Errorf("withdrawn = %v, want [99]", status.Withdrawn)
	}
	if status.UpToDate() {
		t.Error("a database carrying a version this binary does not know is not up to date")
	}
	if status.Applied != status.Known {
		t.Errorf("applied = %d, want %d; a version the build does not ship must not raise the ceiling",
			status.Applied, status.Known)
	}
}

// TestAnUnreadableFlagIsNotReportedAsApplied is the fail-closed check.
//
// The guard reads a column whose value arrives as whatever the driver hands
// back. The failure mode worth preventing is not a crash: it is a value this
// code cannot interpret being read as "applied", which would report a database
// as ready when its state was never established.
func TestAnUnreadableFlagIsNotReportedAsApplied(t *testing.T) {
	conn := newMigratedDB(t)

	if _, err := conn.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (97, 7)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	status := MigrationStatusOf(conn, goddiassets.Migrations(), "control")

	if status.Err == nil {
		t.Fatal("an is_applied value of 7 was accepted; the guard must fail when it cannot read")
	}
	if status.UpToDate() {
		t.Error("a database whose state could not be read must not be reported as up to date")
	}
	if summary := status.Summary(); summary == "" {
		t.Error("the report must say something even when the read failed")
	}
}

// TestAMissingMigrationTableIsOnlyForgivenForThatTable checks the one
// exception in error handling.
//
// "no such table" means "never migrated" only when the table named is goose's
// own version table. Forgiving every instance of the phrase would turn an
// unrelated schema problem into a database that looks untried.
func TestAMissingMigrationTableIsOnlyForgivenForThatTable(t *testing.T) {
	if !isMissingMigrationTable(errStub("no such table: goose_db_version")) {
		t.Error("a missing goose version table means the database has never been migrated")
	}
	if isMissingMigrationTable(errStub("no such table: dns_records")) {
		t.Error("a missing table that is not goose's must not be read as an unmigrated database")
	}
	if isMissingMigrationTable(errStub("database is locked")) {
		t.Error("an unrelated failure must not be read as an unmigrated database")
	}
}

// TestAFileThatIsNotADatabaseIsReportedAsAReadFailure checks that a corrupt or
// foreign file surfaces as an error instead of an empty migration history.
func TestAFileThatIsNotADatabaseIsReportedAsAReadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-database.db")
	if err := os.WriteFile(path, []byte("this is a text file that happens to end in .db"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	status := MigrationStatusOf(conn, migrationNames(), "control")

	if status.Err == nil {
		t.Fatal("a file that is not a database was read as a migration history")
	}
	if status.UpToDate() {
		t.Error("an unreadable file must not be reported as up to date")
	}
}

// TestPendingMigrationStatusDoesNotClaimToHaveReadAnything checks the state a
// store with no file is reported from: nothing was opened, so nothing may be
// claimed to have been found.
func TestPendingMigrationStatusDoesNotClaimToHaveReadAnything(t *testing.T) {
	status := PendingMigrationStatus(migrationNames(), "leases")

	if status.FilePresent || status.TablePresent {
		t.Error("nothing was opened, so neither the file nor the table may be reported as present")
	}
	if status.Known != 22 || len(status.Pending) != 7 {
		t.Errorf("known = %d, pending = %v; every shipped migration is pending", status.Known, status.Pending)
	}
	if status.UpToDate() {
		t.Error("a store that does not exist yet is not up to date")
	}
}

// TestTheVersionListCompressesRuns keeps the report readable as the migration
// count grows, without losing the boundaries: 13-24 must not be printed for a
// set that skips 20.
func TestTheVersionListCompressesRuns(t *testing.T) {
	cases := []struct {
		versions []int64
		want     string
	}{
		{nil, "无"},
		{[]int64{7}, "7"},
		{[]int64{21, 22, 23, 24}, "21-24"},
		{[]int64{20, 21, 22, 24}, "20-22,24"},
		{[]int64{1, 3, 5}, "1,3,5"},
	}
	for _, entry := range cases {
		if got := versionList(entry.versions); got != entry.want {
			t.Errorf("versionList(%v) = %q, want %q", entry.versions, got, entry.want)
		}
	}
}

func openMemory(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	t.Cleanup(func() { conn.Close() })
	return conn
}

// errStub is a minimal error carrying only a message.
type errStub string

func (e errStub) Error() string { return string(e) }
