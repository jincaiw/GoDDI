package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/secretbox"
	_ "modernc.org/sqlite"
)

// snapshotTestKey is deliberately not a realistic key. A test that hard-codes a
// production-looking secret is how a secret ends up in a repository.
const snapshotTestKey = "snapshot-test-key-material"

// --- fixtures ---

// snapshotDB creates a SQLite file and runs the given statements against it.
// The handle stays open until the test ends, so a snapshot taken during the
// test sees the same write-ahead log a running deployment would have.
func snapshotDB(t *testing.T, path string, statements ...string) string {
	t.Helper()
	db := snapshotOpenDB(t, path)
	for i, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("statement %d against %s: %v", i, filepath.Base(path), err)
		}
	}
	return path
}

func snapshotOpenDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{"PRAGMA busy_timeout = 5000", "PRAGMA journal_mode = WAL"} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatalf("%s on %s: %v", pragma, path, err)
		}
	}
	return db
}

type snapshotFixture struct {
	dir     string
	control string
	zone    string
	lease   string
	config  string
}

// newSnapshotFixture builds a deployment-shaped set of three databases: a
// control database with a migration table and the two sealed-secret columns,
// and the two data-plane stores. It also writes a configuration file, so the
// archive always has something to carry beside the databases.
func newSnapshotFixture(t *testing.T) *snapshotFixture {
	t.Helper()
	dir := t.TempDir()
	fixture := &snapshotFixture{
		dir:     dir,
		control: filepath.Join(dir, "goddi.db"),
		zone:    filepath.Join(dir, "dnsdata.db"),
		lease:   filepath.Join(dir, "leases.db"),
		config:  filepath.Join(dir, "config.yaml"),
	}
	if err := os.WriteFile(fixture.config, []byte("server:\n  data_dir: ./data\n"), 0o600); err != nil {
		t.Fatalf("writing config.yaml: %v", err)
	}

	snapshotDB(t, fixture.control,
		`CREATE TABLE goose_db_version (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version_id INTEGER NOT NULL,
			is_applied BOOLEAN NOT NULL,
			tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`,
		// Version 4 is present but not applied, so a reader that ignores
		// is_applied reports 4 where the honest answer is 3.
		`INSERT INTO goose_db_version (version_id, is_applied) VALUES (1, 1), (2, 1), (3, 1), (4, 0)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT NOT NULL)`,
		`INSERT INTO users (id, username) VALUES ('u1', 'admin')`,
		`CREATE TABLE user_totp_secrets (user_id TEXT PRIMARY KEY, secret_ciphertext TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE dns_tsig_keys (id TEXT PRIMARY KEY, name TEXT NOT NULL, secret_ciphertext TEXT NOT NULL DEFAULT '')`,
		// Two sealed seeds and one left in the clear, so the inventory has
		// something to count that is not simply "all of them".
		`INSERT INTO user_totp_secrets (user_id, secret_ciphertext) VALUES
			('u1', '`+secretbox.Prefix+`AAAA'), ('u2', '`+secretbox.Prefix+`BBBB'), ('u3', 'PLAINTEXT')`,
		`INSERT INTO dns_tsig_keys (id, name, secret_ciphertext) VALUES
			('k1', 'transfer-key', '`+secretbox.Prefix+`CCCC')`,
	)
	snapshotDB(t, fixture.zone,
		`CREATE TABLE zones (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`INSERT INTO zones (id, name) VALUES ('z1', 'example.com')`,
	)
	snapshotDB(t, fixture.lease,
		`CREATE TABLE dhcp_leases (id TEXT PRIMARY KEY, ip_address TEXT NOT NULL)`,
		`INSERT INTO dhcp_leases (id, ip_address) VALUES ('l1', '192.0.2.10')`,
	)
	return fixture
}

func (f *snapshotFixture) targets() []SnapshotTarget {
	return []SnapshotTarget{
		{Name: "control", DSN: f.control, HoldsSecrets: true},
		{Name: "dnsdata", DSN: f.zone, Optional: true},
		{Name: "leases", DSN: f.lease, Optional: true},
	}
}

// takeSnapshot writes an encrypted snapshot of the fixture and returns its path
// and manifest.
func (f *snapshotFixture) takeSnapshot(t *testing.T, name string) (string, *SnapshotManifest) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	manifest, err := CreateSnapshot(SnapshotRequest{
		OutputPath:  path,
		Version:     "0.6.0-test",
		Targets:     f.targets(),
		KeyMaterial: snapshotTestKey,
	})
	if err != nil {
		t.Fatalf("creating a snapshot: %v", err)
	}
	return path, manifest
}

// --- the snapshot itself ---

func TestASnapshotCarriesEveryStoreWithItsSchemaVersion(t *testing.T) {
	fixture := newSnapshotFixture(t)
	_, manifest := fixture.takeSnapshot(t, "one"+SnapshotFileExt)

	if got, want := len(manifest.Databases), 3; got != want {
		t.Fatalf("the archive holds %d databases, want %d", got, want)
	}
	if manifest.Encryption != SnapshotEncryptionChunkedGCM {
		t.Errorf("the archive declares encryption %q, want %q", manifest.Encryption, SnapshotEncryptionChunkedGCM)
	}
	if manifest.FormatVersion != SnapshotFormatVersion {
		t.Errorf("the archive declares format version %d, want %d", manifest.FormatVersion, SnapshotFormatVersion)
	}

	byName := map[string]SnapshotDatabase{}
	for _, database := range manifest.Databases {
		byName[database.Name] = database
	}
	for _, name := range []string{"control", "dnsdata", "leases"} {
		entry, ok := byName[name]
		if !ok {
			t.Fatalf("the manifest does not describe the %s store: %+v", name, manifest.Databases)
		}
		if entry.SizeBytes <= 0 || len(entry.SHA256) != 64 {
			t.Errorf("%s: size %d digest %q, want a non-empty member with a sha256", name, entry.SizeBytes, entry.SHA256)
		}
		if entry.Member != SnapshotDirDatabases+"/"+name+".db" {
			t.Errorf("%s: member %q, want it under %s/", name, entry.Member, SnapshotDirDatabases)
		}
	}

	control := byName["control"]
	if !control.SchemaTablePresent {
		t.Error("the control store records no migration table, but the fixture has one")
	}
	// The fixture has version 4 recorded as not applied. Counting it would
	// report a schema the deployment has never run.
	if control.SchemaVersion != 3 {
		t.Errorf("the control store records schema version %d, want 3 (version 4 is present but not applied)", control.SchemaVersion)
	}
	if byName["leases"].SchemaVersion != 0 || byName["leases"].SchemaTablePresent {
		t.Errorf("the lease store has no migration table but records version %d present=%v",
			byName["leases"].SchemaVersion, byName["leases"].SchemaTablePresent)
	}
}

func TestTheSecretInventoryCountsWithoutReadingTheValues(t *testing.T) {
	fixture := newSnapshotFixture(t)
	_, manifest := fixture.takeSnapshot(t, "inventory"+SnapshotFileExt)

	if len(manifest.Secrets.Inventory) != 2 {
		t.Fatalf("the inventory has %d entries, want the two sealed columns: %+v", len(manifest.Secrets.Inventory), manifest.Secrets.Inventory)
	}
	counts := map[string]SecretInventory{}
	for _, entry := range manifest.Secrets.Inventory {
		counts[entry.Table] = entry
	}

	totp := counts["user_totp_secrets"]
	if totp.Rows != 3 || totp.Sealed != 2 {
		t.Errorf("the TOTP inventory is %d rows / %d sealed, want 3 / 2 (one seed is stored in the clear)", totp.Rows, totp.Sealed)
	}
	if totp.Label != secretbox.LabelTOTP {
		t.Errorf("the TOTP inventory names label %q, want %q", totp.Label, secretbox.LabelTOTP)
	}
	tsig := counts["dns_tsig_keys"]
	if tsig.Rows != 1 || tsig.Sealed != 1 || tsig.Label != secretbox.LabelTSIG {
		t.Errorf("the TSIG inventory is %+v, want 1 row under %s", tsig, secretbox.LabelTSIG)
	}
	// The point of the inventory is counting, so nothing it carries may be a
	// value the operator could read a secret out of.
	encoded, err := json.Marshal(manifest.Secrets)
	if err != nil {
		t.Fatalf("encoding the inventory: %v", err)
	}
	for _, forbidden := range []string{"AAAA", "BBBB", "CCCC", "PLAINTEXT"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Errorf("the key inventory carries the stored value %q: %s", forbidden, encoded)
		}
	}
}

// TestASnapshotIsOneConsistentInstant is the property the whole mechanism
// exists for.
//
// A snapshot taken while a write transaction is open must reflect the last
// committed state and not a mixture: copying the main file and the write-ahead
// log separately would produce a database that never existed. The test drives
// it without concurrency so the assertion is deterministic -- an uncommitted
// insert is either visible or it is not.
func TestASnapshotIsOneConsistentInstant(t *testing.T) {
	fixture := newSnapshotFixture(t)

	writer, err := sql.Open("sqlite", fixture.lease)
	if err != nil {
		t.Fatalf("opening the lease store: %v", err)
	}
	defer writer.Close()
	writer.SetMaxOpenConns(1)
	if _, err := writer.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		t.Fatalf("setting busy_timeout: %v", err)
	}

	tx, err := writer.Begin()
	if err != nil {
		t.Fatalf("beginning a write transaction: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO dhcp_leases (id, ip_address) VALUES ('l2', '192.0.2.11')`); err != nil {
		t.Fatalf("inserting inside the transaction: %v", err)
	}

	duringPath := filepath.Join(t.TempDir(), "during"+SnapshotFileExt)
	if _, err := CreateSnapshot(SnapshotRequest{
		OutputPath:  duringPath,
		Version:     "0.6.0-test",
		Targets:     fixture.targets(),
		KeyMaterial: snapshotTestKey,
	}); err != nil {
		t.Fatalf("snapshotting during a write transaction: %v", err)
	}

	during := extractAll(t, duringPath)
	if got := snapshotCount(t, filepath.Join(during, SnapshotDirDatabases, "leases.db")); got != 1 {
		t.Errorf("the snapshot taken mid-transaction holds %d leases, want 1: an uncommitted insert must not be visible", got)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("committing: %v", err)
	}

	committedPath := filepath.Join(t.TempDir(), "committed"+SnapshotFileExt)
	if _, err := CreateSnapshot(SnapshotRequest{
		OutputPath:  committedPath,
		Version:     "0.6.0-test",
		Targets:     fixture.targets(),
		KeyMaterial: snapshotTestKey,
	}); err != nil {
		t.Fatalf("snapshotting after the commit: %v", err)
	}
	committed := extractAll(t, committedPath)
	if got := snapshotCount(t, filepath.Join(committed, SnapshotDirDatabases, "leases.db")); got != 2 {
		t.Errorf("the snapshot taken after the commit holds %d leases, want 2", got)
	}
}

func TestTheExtractedDatabasesAreTheOnesThatWereArchived(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path, manifest := fixture.takeSnapshot(t, "roundtrip"+SnapshotFileExt)

	dir := extractAll(t, path)

	control := filepath.Join(dir, SnapshotDirDatabases, "control.db")
	db, err := sql.Open("sqlite", "file:"+control+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the extracted control store: %v", err)
	}
	defer db.Close()
	var username string
	if err := db.QueryRow(`SELECT username FROM users WHERE id = 'u1'`).Scan(&username); err != nil {
		t.Fatalf("reading the extracted row: %v", err)
	}
	if username != "admin" {
		t.Errorf("the extracted row reads %q, want admin", username)
	}
	tables := 0
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables); err != nil {
		t.Fatalf("counting the extracted tables: %v", err)
	}
	if tables == 0 {
		t.Error("the extracted control store has no tables")
	}
	if got := manifest.Databases[0].Tables; got != tables {
		t.Errorf("the manifest records %d tables for the control store, the extract has %d", got, tables)
	}

	// A manifest that describes what was extracted, member for member.
	if got, want := len(manifest.Databases), 3; got != want {
		t.Fatalf("the manifest describes %d databases, want %d", got, want)
	}
	for _, database := range manifest.Databases {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(database.Member))); err != nil {
			t.Errorf("member %s is described but was not extracted: %v", database.Member, err)
		}
	}
}

func TestTheConfigurationTravelsWithTheArchive(t *testing.T) {
	fixture := newSnapshotFixture(t)
	absentPath := filepath.Join(fixture.dir, "prometheus-alerts.yml")

	out := filepath.Join(t.TempDir(), "config"+SnapshotFileExt)
	manifest, err := CreateSnapshot(SnapshotRequest{
		OutputPath:  out,
		Version:     "0.6.0-test",
		Targets:     fixture.targets(),
		ConfigFiles: []ConfigSource{{Path: fixture.config}, {Path: absentPath, Optional: true}},
		KeyMaterial: snapshotTestKey,
	})
	if err != nil {
		t.Fatalf("creating a snapshot: %v", err)
	}

	if len(manifest.ConfigFiles) != 1 {
		t.Fatalf("the manifest lists %d config files, want 1: an optional file that is not there is left out", len(manifest.ConfigFiles))
	}
	entry := manifest.ConfigFiles[0]
	if entry.Member != SnapshotDirConfig+"/config.yaml" {
		t.Errorf("the config member is %q, want %s/config.yaml", entry.Member, SnapshotDirConfig)
	}
	if entry.Source != fixture.config {
		t.Errorf("the config source is %q, want the path it was read from %q", entry.Source, fixture.config)
	}

	dir := extractAll(t, out)
	data, err := os.ReadFile(filepath.Join(dir, SnapshotDirConfig, "config.yaml"))
	if err != nil {
		t.Fatalf("reading the extracted config: %v", err)
	}
	if string(data) != "server:\n  data_dir: ./data\n" {
		t.Errorf("the extracted config reads %q", data)
	}
}

func TestAMandatoryConfigurationFileThatIsMissingFailsTheSnapshot(t *testing.T) {
	fixture := newSnapshotFixture(t)
	err := createSnapshotForTest(t, filepath.Join(t.TempDir(), "missing"+SnapshotFileExt), SnapshotRequest{
		Targets:     fixture.targets(),
		KeyMaterial: snapshotTestKey,
		ConfigFiles: []ConfigSource{{Path: filepath.Join(fixture.dir, "nope.yaml")}},
	})
	if err == nil {
		t.Fatal("a snapshot of a file that does not exist reported success")
	}
	if !strings.Contains(err.Error(), "nope.yaml") {
		t.Errorf("the error does not name the missing file: %v", err)
	}
}

// --- absent stores, refused writes ---

func TestAnOptionalStoreThatIsNotThereIsRecordedRatherThanFaked(t *testing.T) {
	fixture := newSnapshotFixture(t)
	if err := os.Remove(fixture.zone); err != nil {
		t.Fatalf("removing the DNS store: %v", err)
	}

	_, manifest := fixture.takeSnapshot(t, "absent"+SnapshotFileExt)

	if len(manifest.Databases) != 2 {
		t.Errorf("the archive holds %d databases, want 2", len(manifest.Databases))
	}
	if len(manifest.Absent) != 1 || manifest.Absent[0].Name != "dnsdata" {
		t.Fatalf("the manifest records %+v as absent, want the dnsdata store", manifest.Absent)
	}
	for _, database := range manifest.Databases {
		if database.Name == "dnsdata" {
			t.Error("the absent store appears as a database in the archive")
		}
	}
}

func TestAMandatoryStoreThatIsNotThereFailsTheSnapshot(t *testing.T) {
	fixture := newSnapshotFixture(t)
	if err := os.Remove(fixture.control); err != nil {
		t.Fatalf("removing the control store: %v", err)
	}
	targets := fixture.targets()
	targets[0].Optional = false

	err := createSnapshotForTest(t, filepath.Join(t.TempDir(), "nodb"+SnapshotFileExt), SnapshotRequest{
		Targets:     targets,
		KeyMaterial: snapshotTestKey,
	})
	if err == nil {
		t.Fatal("a snapshot with no control database reported success")
	}
	if !strings.Contains(err.Error(), "control") {
		t.Errorf("the error does not name the missing store: %v", err)
	}
}

func TestASnapshotWithoutKeyMaterialIsRefusedUnlessWaived(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path := filepath.Join(t.TempDir(), "plain"+SnapshotFileExt)

	err := createSnapshotForTest(t, path, SnapshotRequest{Targets: fixture.targets()})
	if err == nil {
		t.Fatal("a snapshot of password hashes and TOTP seeds was written in the clear without being asked to")
	}
	if !strings.Contains(err.Error(), "in the clear") {
		t.Errorf("the refusal does not say what it is refusing: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("the refused snapshot left a file behind")
	}

	manifest, err := CreateSnapshot(SnapshotRequest{
		OutputPath:     path,
		Version:        "0.6.0-test",
		Targets:        fixture.targets(),
		AllowPlaintext: true,
	})
	if err != nil {
		t.Fatalf("a snapshot with encryption waived was refused: %v", err)
	}
	if manifest.Encryption != SnapshotEncryptionNone {
		t.Errorf("the archive declares encryption %q, want %q", manifest.Encryption, SnapshotEncryptionNone)
	}
	if manifest.Secrets.Keys != nil {
		t.Errorf("an unencrypted archive records key fingerprints: %+v", manifest.Secrets.Keys)
	}
	if manifest.Secrets.Note == "" {
		t.Error("an unencrypted archive records no note saying so")
	}
	// It still has to be readable, which is the case the waiver exists for.
	if _, err := InspectSnapshot(path, ""); err != nil {
		t.Fatalf("verifying the unencrypted archive: %v", err)
	}
}

func TestASnapshotWillNotOverwriteAnotherOne(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path := filepath.Join(t.TempDir(), "taken"+SnapshotFileExt)
	if _, err := CreateSnapshot(SnapshotRequest{
		OutputPath:  path,
		Targets:     fixture.targets(),
		KeyMaterial: snapshotTestKey,
	}); err != nil {
		t.Fatalf("creating the first snapshot: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the first snapshot: %v", err)
	}

	_, err = CreateSnapshot(SnapshotRequest{
		OutputPath:  path,
		Targets:     fixture.targets(),
		KeyMaterial: snapshotTestKey,
	})
	if err == nil {
		t.Fatal("a second snapshot silently overwrote the first")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("the refusal does not say why: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-reading the first snapshot: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the first snapshot was modified by the refused second run")
	}
}

// --- reading ---

func TestTheArchiveIsUnreadableWithoutTheKeyItWasWrittenWith(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path, _ := fixture.takeSnapshot(t, "sealed"+SnapshotFileExt)

	if _, err := InspectSnapshot(path, "some-other-key-material"); err == nil {
		t.Fatal("an archive opened with the wrong key material")
	}
	if _, err := InspectSnapshot(path, ""); err == nil {
		t.Fatal("an encrypted archive opened with no key material at all")
	}
	if _, err := InspectSnapshot(path, snapshotTestKey); err != nil {
		t.Fatalf("the archive did not open with the material it was written with: %v", err)
	}
}

func TestTheKeyFingerprintReportsMaterialThatCannotOpenTheArchive(t *testing.T) {
	fixture := newSnapshotFixture(t)
	_, manifest := fixture.takeSnapshot(t, "fingerprint"+SnapshotFileExt)

	if got := SnapshotKeyMismatches(*manifest, snapshotTestKey); len(got) != 0 {
		t.Errorf("the material the archive was written with is reported as a mismatch: %v", got)
	}
	mismatched := SnapshotKeyMismatches(*manifest, "some-other-key-material")
	if len(mismatched) != 2 {
		t.Fatalf("the wrong material mismatches %v, want both sealed purposes", mismatched)
	}
	for _, want := range []string{secretbox.LabelTOTP, secretbox.LabelTSIG} {
		found := false
		for _, label := range mismatched {
			if label == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the mismatch list does not name %q: %v", want, mismatched)
		}
	}
}

// TestTheArchiveKeyIsNotAColumnKey records why the snapshot label exists. The
// archive is encrypted with the same configured material that seals TOTP seeds
// and TSIG keys, so if the label did not take part in the derivation, a
// snapshot file and a sealed column would share a key space.
func TestTheArchiveKeyIsNotAColumnKey(t *testing.T) {
	archive := secretbox.Derive(secretbox.LabelSnapshot, snapshotTestKey)
	for _, label := range []string{secretbox.LabelTOTP, secretbox.LabelTSIG} {
		if bytes.Equal(archive, secretbox.Derive(label, snapshotTestKey)) {
			t.Errorf("the archive key equals the %s key derived from the same material", label)
		}
	}
}

// --- tampering ---

func TestAMemberThatDoesNotMatchTheManifestIsRefused(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path := filepath.Join(t.TempDir(), "altered"+SnapshotFileExt)
	writePlainSnapshot(t, path, fixture, map[string][]byte{
		SnapshotDirDatabases + "/control.db": []byte("this is not a sqlite database"),
	})

	err := inspectForTest(t, path)
	if err == nil {
		t.Fatal("an archive whose control database was replaced reported success")
	}
	if !strings.Contains(err.Error(), "does not match the manifest") {
		t.Errorf("the refusal does not say what is wrong: %v", err)
	}
}

func TestAnArchiveMissingAMemberIsRefused(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path := filepath.Join(t.TempDir(), "missing"+SnapshotFileExt)
	manifest := writePlainSnapshot(t, path, fixture, nil)

	// Rewrite without the lease store, leaving the manifest describing it.
	contents := readArchiveForTest(t, path)
	var kept []archiveMember
	for _, member := range contents.members {
		if member.name == SnapshotDirDatabases+"/leases.db" {
			continue
		}
		kept = append(kept, member)
	}
	contents.members = kept
	writeArchiveForTest(t, path, contents)

	err := inspectForTest(t, path)
	if err == nil {
		t.Fatal("an archive that does not hold what its manifest describes reported success")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("the refusal does not say the archive is incomplete: %v", err)
	}
	if len(manifest.Databases) != 3 {
		t.Fatalf("the fixture manifest describes %d databases, want 3", len(manifest.Databases))
	}
}

func TestAnArchiveFromANewerFormatIsRefusedByName(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path := filepath.Join(t.TempDir(), "future"+SnapshotFileExt)
	writePlainSnapshot(t, path, fixture, nil)

	contents := readArchiveForTest(t, path)
	contents.manifest["format_version"] = SnapshotFormatVersion + 1
	writeArchiveForTest(t, path, contents)

	err := inspectForTest(t, path)
	if err == nil {
		t.Fatal("an archive from a newer format was read as if it were this one")
	}
	if !strings.Contains(err.Error(), "format version") {
		t.Errorf("the refusal does not name the format version: %v", err)
	}
}

func TestATruncatedArchiveIsRefused(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path, _ := fixture.takeSnapshot(t, "truncated"+SnapshotFileExt)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	if err := os.WriteFile(path, data[:len(data)*3/4], 0o600); err != nil {
		t.Fatalf("truncating the archive: %v", err)
	}
	if err := inspectForTest(t, path); err == nil {
		t.Fatal("a truncated archive reported success")
	}
}

func TestAFileThatIsNotAnArchiveAtAllIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notanarchive")
	if err := os.WriteFile(path, []byte("hello, this is a text file"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	err := inspectForTest(t, path)
	if err == nil {
		t.Fatal("a text file was accepted as a snapshot")
	}
	if !strings.Contains(err.Error(), "neither") {
		t.Errorf("the refusal does not say what arrived: %v", err)
	}
}

// --- extraction ---

func TestExtractionRefusesToWriteOverADirectoryWithoutBeingTold(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path, _ := fixture.takeSnapshot(t, "twice"+SnapshotFileExt)
	into := filepath.Join(t.TempDir(), "unpacked")
	if err := os.MkdirAll(into, 0o700); err != nil {
		t.Fatalf("creating the target directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(into, "operator-notes.txt"), []byte("do not lose this"), 0o600); err != nil {
		t.Fatalf("writing a file into the target: %v", err)
	}

	_, err := ReadSnapshot(SnapshotReadRequest{Path: path, KeyMaterial: snapshotTestKey, ExtractTo: into})
	if err == nil {
		t.Fatal("extraction wrote over an existing directory without being told to")
	}
	if !strings.Contains(err.Error(), "--replace") {
		t.Errorf("the refusal does not say how to proceed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(into, "operator-notes.txt")); err != nil {
		t.Errorf("the refused extraction touched the existing directory: %v", err)
	}

	result, err := ReadSnapshot(SnapshotReadRequest{Path: path, KeyMaterial: snapshotTestKey, ExtractTo: into, Replace: true})
	if err != nil {
		t.Fatalf("extraction with --replace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(into, "operator-notes.txt")); !os.IsNotExist(err) {
		t.Error("--replace left a file from the previous contents behind")
	}
	if result.ExtractedTo != into {
		t.Errorf("the result names the extraction directory %q, want %q", result.ExtractedTo, into)
	}
	// The staging directory must not survive a successful extraction.
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(into), filepath.Base(into)+".partial-*"))
	if err != nil {
		t.Fatalf("globbing for staging directories: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("extraction left staging directories behind: %v", matches)
	}
}

func TestARefusedArchiveLeavesNoHalfExtractedDirectory(t *testing.T) {
	fixture := newSnapshotFixture(t)
	path := filepath.Join(t.TempDir(), "altered2"+SnapshotFileExt)
	writePlainSnapshot(t, path, fixture, map[string][]byte{
		// The config member is written last, so a reader that unpacked as it
		// went would already have written both databases by the time it
		// noticed something was wrong.
		SnapshotDirConfig + "/config.yaml": []byte("not what the manifest describes"),
	})

	into := filepath.Join(t.TempDir(), "never")
	_, err := ReadSnapshot(SnapshotReadRequest{Path: path, KeyMaterial: snapshotTestKey, ExtractTo: into})
	if err == nil {
		t.Fatal("an archive that fails verification was extracted anyway")
	}
	if _, statErr := os.Stat(into); !os.IsNotExist(statErr) {
		t.Errorf("a refused archive left %s behind", into)
	}
	matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(into), filepath.Base(into)+".partial-*"))
	if globErr != nil {
		t.Fatalf("globbing for staging directories: %v", globErr)
	}
	if len(matches) != 0 {
		t.Errorf("a refused archive left staging directories behind: %v", matches)
	}
}

func TestVerificationReportsEveryMemberItChecked(t *testing.T) {
	fixture := newSnapshotFixture(t)
	configPath := filepath.Join(fixture.dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("a: b\n"), 0o600); err != nil {
		t.Fatalf("writing the config file: %v", err)
	}
	path := filepath.Join(t.TempDir(), "members"+SnapshotFileExt)
	if _, err := CreateSnapshot(SnapshotRequest{
		OutputPath:  path,
		Targets:     fixture.targets(),
		ConfigFiles: []ConfigSource{{Path: configPath}},
		KeyMaterial: snapshotTestKey,
	}); err != nil {
		t.Fatalf("creating a snapshot: %v", err)
	}

	result, err := ReadSnapshot(SnapshotReadRequest{Path: path, KeyMaterial: snapshotTestKey})
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if len(result.Members) != 4 {
		t.Fatalf("verification reports %d members, want the three databases and one config file: %+v", len(result.Members), result.Members)
	}
	for _, member := range result.Members {
		if member.SizeBytes <= 0 || len(member.SHA256) != 64 {
			t.Errorf("member %s reports %d bytes and digest %q", member.Name, member.SizeBytes, member.SHA256)
		}
	}
	if !result.Encrypted {
		t.Error("the result does not report the archive as encrypted")
	}
	// Verify-only reads must not unpack anything.
	if result.ExtractedTo != "" {
		t.Errorf("a verify-only read reported extracting to %q", result.ExtractedTo)
	}
}

// --- helpers ---

func createSnapshotForTest(t *testing.T, path string, req SnapshotRequest) error {
	t.Helper()
	req.OutputPath = path
	if req.Version == "" {
		req.Version = "0.6.0-test"
	}
	_, err := CreateSnapshot(req)
	return err
}

func inspectForTest(t *testing.T, path string) error {
	t.Helper()
	_, err := ReadSnapshot(SnapshotReadRequest{Path: path, KeyMaterial: snapshotTestKey})
	return err
}

func extractAll(t *testing.T, path string) string {
	t.Helper()
	into := filepath.Join(t.TempDir(), "unpacked")
	if _, err := ReadSnapshot(SnapshotReadRequest{Path: path, KeyMaterial: snapshotTestKey, ExtractTo: into}); err != nil {
		t.Fatalf("extracting %s: %v", filepath.Base(path), err)
	}
	return into
}

func snapshotCount(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dhcp_leases`).Scan(&count); err != nil {
		t.Fatalf("counting leases in %s: %v", path, err)
	}
	return count
}

type archiveMember struct {
	name string
	body []byte
}

type archiveContents struct {
	manifest map[string]any
	members  []archiveMember
}

// writePlainSnapshot writes an unencrypted archive whose member bodies are
// produced by the caller, so a test can hand the reader an archive that
// disagrees with its own manifest. A guard nobody has seen fail is a guard
// nobody knows is wired up.
func writePlainSnapshot(t *testing.T, path string, fixture *snapshotFixture, overrides map[string][]byte) *SnapshotManifest {
	t.Helper()
	manifest, err := CreateSnapshot(SnapshotRequest{
		OutputPath:     path,
		Version:        "0.6.0-test",
		Targets:        fixture.targets(),
		ConfigFiles:    []ConfigSource{{Path: fixture.config}},
		AllowPlaintext: true,
	})
	if err != nil {
		t.Fatalf("creating the plaintext snapshot: %v", err)
	}
	if overrides == nil {
		return manifest
	}

	contents := readArchiveForTest(t, path)
	for i, member := range contents.members {
		if body, ok := overrides[member.name]; ok {
			contents.members[i].body = body
		}
	}
	writeArchiveForTest(t, path, contents)
	return manifest
}

func readArchiveForTest(t *testing.T, path string) archiveContents {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("reading %s as gzip: %v", path, err)
	}
	defer gz.Close()

	contents := archiveContents{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("reading member %s: %v", header.Name, err)
		}
		if header.Name == SnapshotManifestMember {
			if err := json.Unmarshal(body, &contents.manifest); err != nil {
				t.Fatalf("decoding the manifest: %v", err)
			}
			continue
		}
		contents.members = append(contents.members, archiveMember{name: header.Name, body: body})
	}
	return contents
}

func writeArchiveForTest(t *testing.T, path string, contents archiveContents) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	manifest, err := json.Marshal(contents.manifest)
	if err != nil {
		t.Fatalf("encoding the manifest: %v", err)
	}
	writeTarMemberForTest(t, tw, SnapshotManifestMember, manifest)
	for _, member := range contents.members {
		writeTarMemberForTest(t, tw, member.name, member.body)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing the tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("closing the gzip writer: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func writeTarMemberForTest(t *testing.T, tw *tar.Writer, name string, body []byte) {
	t.Helper()
	header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("writing the header for %s: %v", name, err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}
