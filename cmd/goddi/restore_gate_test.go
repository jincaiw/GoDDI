package main

import (
	"strings"
	"testing"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/backup"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/dataplane"
)

// TestCompareVersionOrdersDottedNumbers covers the comparison the gate is
// built on.
func TestCompareVersionOrdersDottedNumbers(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
		wantOK      bool
	}{
		{"0.5.2", "0.5.2", 0, true},
		{"0.5.2", "0.6.0", -1, true},
		{"0.6.0", "0.5.2", 1, true},
		{"0.10.0", "0.9.0", 1, true},
		{"1.0", "1.0.0", 0, true},
		{"v0.6.0", "0.6.0", 0, true},
		{"", "0.6.0", 0, false},
		{"dev", "0.6.0", 0, false},
		{"0.6.0", "dev", 0, false},
		{"0.6.0-3-gabc123", "0.6.0", 0, false},
		{"0.6.x", "0.6.0", 0, false},
	}
	for _, entry := range cases {
		got, ok := compareVersion(entry.left, entry.right)
		if ok != entry.wantOK {
			t.Errorf("compareVersion(%q, %q) ok = %v, want %v", entry.left, entry.right, ok, entry.wantOK)
			continue
		}
		if ok && got != entry.want {
			t.Errorf("compareVersion(%q, %q) = %d, want %d", entry.left, entry.right, got, entry.want)
		}
	}
}

// TestAnUnparseableVersionIsReportedAsUncompared is the fail-visible guard.
//
// A development build is stamped with something that is not a version. Reading
// the numeric prefix and guessing at the rest would make the gate compare
// nothing while appearing to have compared -- so the parse fails, the report
// says the comparison was skipped, and the schema comparison is what carries
// the decision.
func TestAnUnparseableVersionIsReportedAsUncompared(t *testing.T) {
	defer restoreVersion(Version)

	Version = "dev"
	manifest := backup.SnapshotManifest{
		Version:   "0.6.0",
		Databases: []backup.SnapshotDatabase{{Name: "control", SchemaVersion: 1}},
	}

	fitness, err := assessArchive(manifest)
	if err != nil {
		t.Fatalf("assessArchive: %v", err)
	}
	if fitness.VersionKnown {
		t.Error("a version of \"dev\" must not be treated as comparable")
	}
	if fitness.ProductNewer {
		t.Error("an uncompared version must not be reported as an ordering")
	}
}

// TestTheGateJudgesAgainstThisBinaryNotTheLiveDatabase pins the correction
// W12-b made.
//
// assessArchive is given a manifest and nothing else -- no host, no live
// database, no filesystem. That is the point: the gate has to reach the same
// verdict on a host with no databases yet, which is where a restore is usually
// prepared. The implementation this replaced compared the archive against the
// live database, which checked nothing at all when there was no live database
// and refused a legitimate restore when there was.
func TestTheGateJudgesAgainstThisBinaryNotTheLiveDatabase(t *testing.T) {
	defer restoreVersion(Version)
	Version = "0.6.0"

	ceiling, err := knownSchemaVersions()
	if err != nil {
		t.Fatalf("knownSchemaVersions: %v", err)
	}
	control, ok := ceiling["control"]
	if !ok || control <= 0 {
		t.Fatalf("the control migration set has no ceiling: %v", ceiling)
	}

	ahead := backup.SnapshotManifest{
		Version:   Version,
		Databases: []backup.SnapshotDatabase{{Name: "control", SchemaVersion: control + 1}},
	}
	fitness, err := assessArchive(ahead)
	if err != nil {
		t.Fatalf("assessArchive: %v", err)
	}
	if len(fitness.SchemaAhead) != 1 {
		t.Fatalf("a schema above this build's ceiling must be reported as ahead, got %+v", fitness)
	}
	if err := fitness.refusal(); err == nil {
		t.Error("an archive from a newer schema must be refused")
	} else if !strings.Contains(err.Error(), "control") {
		t.Errorf("the refusal must name the store: %v", err)
	}

	behind := backup.SnapshotManifest{
		Version:   Version,
		Databases: []backup.SnapshotDatabase{{Name: "control", SchemaVersion: control - 1}},
	}
	fitness, err = assessArchive(behind)
	if err != nil {
		t.Fatalf("assessArchive: %v", err)
	}
	if len(fitness.SchemaBehind) != 1 {
		t.Fatalf("an older schema must be reported as behind, got %+v", fitness)
	}
	if err := fitness.refusal(); err != nil {
		t.Errorf("restoring a backup older than the live database is the ordinary case and must not be refused: %v", err)
	}
}

// TestTheGateRefusesAStoreThisBinaryCannotPlace is the fail-closed half.
//
// An archive carrying a store this binary has no migrations for cannot be
// judged. Assuming it is readable is the one answer that must never be given
// without evidence.
func TestTheGateRefusesAStoreThisBinaryCannotPlace(t *testing.T) {
	defer restoreVersion(Version)
	Version = "0.6.0"

	manifest := backup.SnapshotManifest{
		Version: Version,
		Databases: []backup.SnapshotDatabase{
			{Name: "control", SchemaVersion: 1},
			{Name: "something_new", SchemaVersion: 1},
		},
	}
	fitness, err := assessArchive(manifest)
	if err != nil {
		t.Fatalf("assessArchive: %v", err)
	}
	if len(fitness.UnknownStores) != 1 || fitness.UnknownStores[0] != "something_new" {
		t.Fatalf("unknown stores = %v, want [something_new]", fitness.UnknownStores)
	}
	if err := fitness.refusal(); err == nil {
		t.Error("an archive this binary cannot place must be refused rather than assumed readable")
	}
}

// TestTheGateRefusesAnArchiveFromANewerBuild covers the product-version half of
// the gate, which is the signal a human reads.
func TestTheGateRefusesAnArchiveFromANewerBuild(t *testing.T) {
	defer restoreVersion(Version)
	Version = "0.5.2"

	manifest := backup.SnapshotManifest{
		Version:   "0.6.0",
		Databases: []backup.SnapshotDatabase{{Name: "control", SchemaVersion: 1}},
	}
	fitness, err := assessArchive(manifest)
	if err != nil {
		t.Fatalf("assessArchive: %v", err)
	}
	if !fitness.ProductNewer {
		t.Fatal("0.6.0 is newer than 0.5.2")
	}
	err = fitness.refusal()
	if err == nil {
		t.Fatal("an archive written by a newer build must be refused")
	}
	if !strings.Contains(err.Error(), "0.6.0") || !strings.Contains(err.Error(), "0.5.2") {
		t.Errorf("the refusal must name both versions: %v", err)
	}
}

// TestTheSchemaSignalOutranksTheProductVersion keeps the report ordered by what
// is precise.
//
// The product version is a label a human wrote into a build; the schema
// version is what the database actually says. When both are wrong, the message
// has to lead with the one that names the store.
func TestTheSchemaSignalOutranksTheProductVersion(t *testing.T) {
	defer restoreVersion(Version)
	Version = "0.5.2"

	ceiling, err := knownSchemaVersions()
	if err != nil {
		t.Fatalf("knownSchemaVersions: %v", err)
	}
	manifest := backup.SnapshotManifest{
		Version:   "0.6.0",
		Databases: []backup.SnapshotDatabase{{Name: "leases", SchemaVersion: ceiling["leases"] + 5}},
	}
	fitness, err := assessArchive(manifest)
	if err != nil {
		t.Fatalf("assessArchive: %v", err)
	}
	if err := fitness.refusal(); err == nil {
		t.Fatal("expected a refusal")
	} else if !strings.Contains(err.Error(), "leases") {
		t.Errorf("the schema is the precise signal and must lead the refusal: %v", err)
	}
}

// TestKnownSchemaVersionsReadsBothMigrationSets checks the ceilings against the
// sets they come from, so a store mapped to the wrong filesystem is caught.
func TestKnownSchemaVersionsReadsBothMigrationSets(t *testing.T) {
	ceiling, err := knownSchemaVersions()
	if err != nil {
		t.Fatalf("knownSchemaVersions: %v", err)
	}

	control, err := database.ShippedMigrations(goddiassets.Migrations())
	if err != nil {
		t.Fatalf("listing control migrations: %v", err)
	}
	data, err := database.ShippedMigrations(dataplane.Migrations())
	if err != nil {
		t.Fatalf("listing data plane migrations: %v", err)
	}
	if ceiling["control"] != control[len(control)-1] {
		t.Errorf("control ceiling = %d, want %d", ceiling["control"], control[len(control)-1])
	}
	for _, set := range []string{"dnsdata", "leases"} {
		if ceiling[set] != data[len(data)-1] {
			t.Errorf("%s ceiling = %d, want %d", set, ceiling[set], data[len(data)-1])
		}
	}
}

func restoreVersion(previous string) {
	Version = previous
}
