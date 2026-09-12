package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/secretbox"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver, for VACUUM INTO
)

// A snapshot is the file an operator restores from. It differs from the JSON
// exports in this package in what it promises: the exports describe the
// configuration a console can read back, while a snapshot is the databases
// themselves, copied in a single consistent instant, plus the configuration
// files and an inventory of the keys needed to read the secrets inside them.
//
// The two paths are deliberately separate. The JSON exports are a product
// feature -- an operator exporting the zones they just edited -- and they run
// inside a live process. A snapshot is a disaster-recovery artifact driven from
// the command line, and it is built to be restorable onto a host that has
// nothing: it carries the lease store and the DNS data store, which the exports
// never touched, and it records the schema versions so a restore can tell
// whether the instance it lands on can read what is inside.
const (
	// SnapshotFormatVersion is the layout version of the archive. A reader
	// refuses a version it does not know rather than guessing: half-reading a
	// backup and reporting success is worse than not reading it at all.
	SnapshotFormatVersion = 1

	// SnapshotFileExt is the suffix this build writes. Encrypted and plaintext
	// archives share it because the encryption announces itself in the first
	// bytes of the file.
	SnapshotFileExt = ".goddi-snap"

	// SnapshotManifestMember is the archive member holding the manifest. It is
	// written first, so a reader learns what the archive claims to hold before
	// it has to read the parts being described.
	SnapshotManifestMember = "manifest.json"

	SnapshotDirDatabases = "db"
	SnapshotDirConfig    = "config"
)

// SnapshotTarget names one SQLite file to copy into the archive.
type SnapshotTarget struct {
	// Name is the member name inside the archive. It must be a plain file
	// name: a target that could escape the archive directory would let the
	// caller write anywhere, and this value reaches the filesystem.
	Name string

	// DSN is where the process would open this database. Passed as the DSN
	// rather than as a path so the copy is made under the same driver the
	// process runs with.
	DSN string

	// Optional marks a store that may legitimately not exist yet: a data plane
	// whose listener has never run has no file. A missing mandatory target is
	// an error, because a backup that quietly omits the control database is
	// the worst possible outcome of a command that reported success.
	Optional bool

	// HoldsSecrets marks the store carrying the sealed-secret columns, so the
	// archive can record how many exist without needing a key to count them.
	HoldsSecrets bool
}

// ConfigSource names a file to place in the archive beside the databases.
type ConfigSource struct {
	Path string
	// Optional tolerates a file that is not there -- an alert rule file on a
	// deployment that does not ship one. A mandatory file that is missing
	// fails the snapshot, because the archive then does not contain what the
	// operator asked it to contain.
	Optional bool
}

// SnapshotRequest is one run of CreateSnapshot.
type SnapshotRequest struct {
	OutputPath  string
	Version     string
	Description string
	Targets     []SnapshotTarget
	ConfigFiles []ConfigSource

	// KeyMaterial encrypts the whole archive. Empty is refused unless
	// AllowPlaintext is set: the control database holds password hashes and
	// TOTP seeds, and writing those to a file in the clear should be a
	// decision someone made, not a default.
	KeyMaterial    string
	AllowPlaintext bool
}

// SnapshotManifest is the index of an archive. It is JSON rather than a schema
// because it has to be readable by a human with a hex editor and by a script
// that has never heard of this project.
type SnapshotManifest struct {
	FormatVersion int                `json:"format_version"`
	Version       string             `json:"version"`
	Description   string             `json:"description,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	Hostname      string             `json:"hostname,omitempty"`
	Encryption    string             `json:"encryption"`
	Databases     []SnapshotDatabase `json:"databases"`
	Absent        []SnapshotAbsence  `json:"absent,omitempty"`
	ConfigFiles   []SnapshotFile     `json:"config_files,omitempty"`
	Secrets       SnapshotSecrets    `json:"secrets"`
}

// SnapshotDatabase describes one database in the archive.
type SnapshotDatabase struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Member string `json:"member"`
	// SizeBytes and SHA256 describe the member as it sits in the archive. The
	// reader checks both, so a copy that was altered or truncated on the way
	// to the restore host is refused rather than restored.
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	// SchemaVersion is the highest applied goose migration in the copy, and
	// SchemaTablePresent says whether the migration table was there at all: a
	// database that has never been migrated reports the same 0 as one that is
	// at revision 0, and a restore needs to tell them apart.
	SchemaVersion      int64 `json:"schema_version"`
	SchemaTablePresent bool  `json:"schema_table_present"`
	Tables             int   `json:"tables"`
}

// SnapshotAbsence records a store that was expected and not found. It is
// written down rather than omitted so that "the DNS plane has never run on this
// host" and "the archiver lost the DNS store" are different statements.
type SnapshotAbsence struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// SnapshotFile describes one configuration file in the archive.
type SnapshotFile struct {
	Source    string `json:"source"`
	Member    string `json:"member"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// SnapshotSecrets is the key inventory: which purposes this build seals, a
// fingerprint of the key each purpose resolves to, and how many stored secrets
// exist in each column.
//
// The counts answer the question an operator asks when they pick an archive off
// a shelf -- whether the TOTP seeds and TSIG keys they need are inside it --
// without having to restore it first. The fingerprints identify the key
// material the archive belongs to, so a shelf of archives can be told apart by
// the key each one expects.
//
// Nothing here is a secret value: the counts are counts, and a fingerprint is
// derived from the key, not from anything the key opens.
type SnapshotSecrets struct {
	Labels    []string          `json:"labels"`
	Keys      map[string]string `json:"keys,omitempty"`
	Inventory []SecretInventory `json:"inventory,omitempty"`
	Note      string            `json:"note,omitempty"`
}

// SecretInventory counts the stored secrets in one column. It records how many,
// never what.
type SecretInventory struct {
	Table  string `json:"table"`
	Column string `json:"column"`
	Label  string `json:"label"`
	Rows   int64  `json:"rows"`
	Sealed int64  `json:"sealed"`
}

// CreateSnapshot copies every target into a single encrypted archive and
// returns the manifest it wrote.
//
// The databases are copied with VACUUM INTO, which runs inside a read
// transaction: the archive is a single instant of a database that was being
// written to, not a main file and a write-ahead log read at two different
// moments. That distinction is the whole point -- for a lease store, the other
// outcome is an address that was handed out and then forgotten.
func CreateSnapshot(req SnapshotRequest) (*SnapshotManifest, error) {
	if strings.TrimSpace(req.OutputPath) == "" {
		return nil, errors.New("snapshot: no output path")
	}
	material := strings.TrimSpace(req.KeyMaterial)
	if material == "" && !req.AllowPlaintext {
		return nil, errors.New("snapshot: no key material is configured and encryption was not waived: this archive would hold password hashes, TOTP seeds and TSIG keys in the clear (set GODDI_SECURITY_ENCRYPTION_KEY, or pass --plaintext to accept that)")
	}
	if material == "" {
		slog.Warn("snapshot: writing an unencrypted archive; anyone who can read the file can read every stored secret in it")
	}

	work, err := os.MkdirTemp("", "goddi-snapshot-")
	if err != nil {
		return nil, fmt.Errorf("snapshot: creating a working directory: %w", err)
	}
	defer os.RemoveAll(work)

	hostname, _ := os.Hostname()
	manifest := SnapshotManifest{
		FormatVersion: SnapshotFormatVersion,
		Version:       req.Version,
		Description:   req.Description,
		CreatedAt:     time.Now().UTC(),
		Hostname:      hostname,
		Encryption:    SnapshotEncryptionNone,
	}
	if material != "" {
		manifest.Encryption = SnapshotEncryptionChunkedGCM
	}
	manifest.Secrets = describeSnapshotSecrets(material)

	for _, target := range req.Targets {
		entry, absence, err := snapshotDatabase(work, target)
		if err != nil {
			return nil, err
		}
		if absence != nil {
			manifest.Absent = append(manifest.Absent, *absence)
			continue
		}
		manifest.Databases = append(manifest.Databases, *entry)

		if target.HoldsSecrets {
			inventory, err := secretInventory(snapshotDatabasePath(work, target.Name))
			if err != nil {
				return nil, err
			}
			manifest.Secrets.Inventory = inventory
		}
	}

	used := map[string]bool{}
	for _, source := range req.ConfigFiles {
		entry, err := snapshotConfigFile(work, source, used)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			manifest.ConfigFiles = append(manifest.ConfigFiles, *entry)
		}
	}

	if err := writeSnapshotManifest(work, manifest); err != nil {
		return nil, err
	}
	if err := writeSnapshotArchive(work, req.OutputPath, material); err != nil {
		return nil, err
	}

	slog.Info("snapshot written",
		"path", req.OutputPath,
		"databases", len(manifest.Databases),
		"absent", len(manifest.Absent),
		"config_files", len(manifest.ConfigFiles),
		"encrypted", manifest.Encryption != SnapshotEncryptionNone)
	return &manifest, nil
}

// snapshotDatabase copies one target into the working directory and describes
// it.
func snapshotDatabase(work string, target SnapshotTarget) (*SnapshotDatabase, *SnapshotAbsence, error) {
	if err := safeMemberName(target.Name); err != nil {
		return nil, nil, fmt.Errorf("snapshot: database target name %q: %w", target.Name, err)
	}
	path := dsnPath(target.DSN)
	if path == "" {
		return nil, nil, fmt.Errorf("snapshot: the %s store has no resolvable file path (dsn %q); only file-backed SQLite stores can be snapshotted", target.Name, target.DSN)
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) && target.Optional {
			return nil, &SnapshotAbsence{Name: target.Name, Source: path, Reason: "no such file"}, nil
		}
		return nil, nil, fmt.Errorf("snapshot: the %s store at %s: %w", target.Name, path, err)
	}

	dest := snapshotDatabasePath(work, target.Name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return nil, nil, fmt.Errorf("snapshot: preparing the %s copy: %w", target.Name, err)
	}
	if err := vacuumInto(target.DSN, dest); err != nil {
		return nil, nil, fmt.Errorf("snapshot: copying the %s store: %w", target.Name, err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot: reading back the %s copy: %w", target.Name, err)
	}
	digest, err := fileSHA256(dest)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot: hashing the %s copy: %w", target.Name, err)
	}
	version, present, tables, err := readSchemaSummary(dest)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot: reading the %s schema: %w", target.Name, err)
	}

	return &SnapshotDatabase{
		Name:               target.Name,
		Source:             path,
		Member:             snapshotDatabaseMember(target.Name),
		SizeBytes:          info.Size(),
		SHA256:             digest,
		SchemaVersion:      version,
		SchemaTablePresent: present,
		Tables:             tables,
	}, nil, nil
}

// snapshotDatabaseMember and snapshotDatabasePath are the one place the member
// name and the file it is read from are derived from each other. They drifted
// apart once already -- the manifest promised db/ and the file was written to
// the archive root -- and an archive whose manifest describes members that are
// not in it fails at restore time, which is the worst possible moment to find
// out.
func snapshotDatabaseMember(name string) string {
	return SnapshotDirDatabases + "/" + name + ".db"
}

func snapshotDatabasePath(work, name string) string {
	return filepath.Join(work, filepath.FromSlash(snapshotDatabaseMember(name)))
}

// vacuumInto copies a live database to dest using SQLite's own VACUUM INTO.
//
// This is what makes the copy trustworthy. VACUUM INTO runs inside a read
// transaction, so the result is one consistent point in time even while
// clients are being served. Copying the files would take the main file and the
// write-ahead log at two different moments and produce a database that never
// existed -- for a lease store, an address handed out and then forgotten, or
// handed out twice.
//
// The source is opened through its configured DSN, so the copy is made under
// the same driver and pragmas the process runs with. It is opened read-write
// rather than read-only on purpose: a read-only connection to a WAL database
// that no writer currently has open cannot create the shared-memory file it
// needs, and fails on a database that is perfectly healthy.
func vacuumInto(dsn, dest string) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("opening the source: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// A snapshot taken during a busy period should wait for its turn rather
	// than fail: the alternative is a backup schedule that reports failures on
	// exactly the days the instance was busy.
	if _, err := db.Exec("PRAGMA busy_timeout = 30000"); err != nil {
		return fmt.Errorf("setting busy_timeout: %w", err)
	}
	if _, err := db.Exec("VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("VACUUM INTO %s: %w", dest, err)
	}
	return nil
}

// readSchemaSummary reports the highest applied migration in a database file,
// whether the migration table is there at all, and how many tables it holds.
//
// It opens the file read-only, which is right for the copies inside a snapshot:
// VACUUM INTO writes a rollback-journal database, so a read-only connection
// needs nothing it cannot create. It is not right for the deployment's own live
// database -- see SchemaVersionOf.
func readSchemaSummary(path string) (int64, bool, int, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, false, 0, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	return schemaSummaryOf(db)
}

// SchemaVersionOf reports the highest applied migration in a live database file
// and whether the migration table is there at all.
//
// The connection is read-write rather than read-only on purpose: a read-only
// connection to a write-ahead-log database that has no shared-memory file yet
// cannot create one, and fails on a database that is perfectly healthy. This is
// opened against a file belonging to a running deployment, which is exactly the
// case that trips over it.
func SchemaVersionOf(path string) (int64, bool, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return 0, false, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return 0, false, fmt.Errorf("waiting for %s: %w", path, err)
	}
	version, present, _, err := schemaSummaryOf(db)
	return version, present, err
}

func schemaSummaryOf(db *sql.DB) (int64, bool, int, error) {
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables); err != nil {
		return 0, false, 0, fmt.Errorf("counting tables: %w", err)
	}

	var present bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version')`).Scan(&present); err != nil {
		return 0, false, tables, fmt.Errorf("looking for the migration table: %w", err)
	}
	if !present {
		return 0, false, tables, nil
	}

	// A database copied mid-migration can carry a migration that was applied
	// but whose work is not visible in the same instant; goose records
	// is_applied only on commit, so filtering on it is what keeps this number
	// honest.
	var version sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version); err != nil {
		return 0, true, tables, fmt.Errorf("reading the migration version: %w", err)
	}
	return version.Int64, true, tables, nil
}

// secretInventory counts the stored secrets in the columns this build seals.
//
// It reads secretbox's compiled-in list rather than a copy of it, so adding a
// sealed column without adding it there stays a visible omission. Table and
// column names therefore come from constants and never from configuration or a
// request, which is what makes interpolating them safe.
func secretInventory(path string) ([]SecretInventory, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("snapshot: opening the snapshot to count secrets: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	out := make([]SecretInventory, 0, 2)
	for _, target := range secretbox.ControlDatabaseSecrets() {
		var present bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, target.Table).Scan(&present); err != nil {
			return nil, fmt.Errorf("snapshot: looking for %s: %w", target.Table, err)
		}
		if !present {
			continue
		}
		entry := SecretInventory{Table: target.Table, Column: target.ValueColumn, Label: target.Label}
		query := fmt.Sprintf(`SELECT
			COALESCE(SUM(CASE WHEN %[1]s IS NOT NULL AND %[1]s <> '' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN %[1]s LIKE ? THEN 1 ELSE 0 END), 0)
			FROM %[2]s`, target.ValueColumn, target.Table)
		if err := db.QueryRow(query, secretbox.Prefix+"%").Scan(&entry.Rows, &entry.Sealed); err != nil {
			return nil, fmt.Errorf("snapshot: counting the secrets in %s: %w", target.Table, err)
		}
		out = append(out, entry)
	}
	return out, nil
}

// describeSnapshotSecrets records which key each sealed purpose resolves to
// under the material the archive was written with.
func describeSnapshotSecrets(material string) SnapshotSecrets {
	out := SnapshotSecrets{}
	if material == "" {
		out.Note = "the archive is unencrypted and no key material is configured; stored secrets were neither encrypted nor checked"
		return out
	}
	out.Keys = map[string]string{}
	seen := map[string]bool{}
	for _, target := range secretbox.ControlDatabaseSecrets() {
		if seen[target.Label] {
			continue
		}
		seen[target.Label] = true
		out.Labels = append(out.Labels, target.Label)
		out.Keys[target.Label] = snapshotKeyFingerprint(target.Label, material)
	}
	sort.Strings(out.Labels)
	return out
}

// snapshotKeyFingerprint identifies the key a purpose's secrets were sealed
// with.
//
// Its job is identification, not access control: an operator holding several
// archives and one key can tell which of them that key belongs to, and a reader
// can assert that the manifest it just opened is internally consistent with the
// material it opened it with.
//
// It hashes the derived key rather than the material, so it reveals nothing
// about the material that the ability to derive this key does not already
// imply, and it is not a password check: key material is required to be
// high-entropy, and the manifest it sits in is itself encrypted.
func snapshotKeyFingerprint(label, material string) string {
	if material == "" {
		return ""
	}
	derived := secretbox.Derive(label, material)
	sum := sha256.Sum256(append([]byte("goddi-snapshot-key-v1:"+label+":"), derived...))
	return base64.RawStdEncoding.EncodeToString(sum[:8])
}

// snapshotConfigFile copies one configuration file into the working directory.
func snapshotConfigFile(work string, source ConfigSource, used map[string]bool) (*SnapshotFile, error) {
	path := strings.TrimSpace(source.Path)
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) && source.Optional {
			slog.Info("snapshot: optional file is not present and was left out", "path", path)
			return nil, nil
		}
		return nil, fmt.Errorf("snapshot: %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("snapshot: %s is a directory; only files can be placed in an archive", path)
	}

	base := filepath.Base(path)
	member := SnapshotDirConfig + "/" + base
	// Two files can share a base name (a config.yaml from two directories);
	// the archive has one flat config directory, so the second one is
	// disambiguated rather than silently overwriting the first.
	for n := 2; used[member]; n++ {
		member = fmt.Sprintf("%s/%d-%s", SnapshotDirConfig, n, base)
	}
	used[member] = true

	dest := filepath.Join(work, filepath.FromSlash(member))
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return nil, fmt.Errorf("snapshot: preparing %s: %w", member, err)
	}
	if err := copyFile(path, dest, 0o600); err != nil {
		return nil, fmt.Errorf("snapshot: copying %s: %w", path, err)
	}
	digest, err := fileSHA256(dest)
	if err != nil {
		return nil, fmt.Errorf("snapshot: hashing %s: %w", path, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	return &SnapshotFile{
		Source:    absolute,
		Member:    member,
		SizeBytes: info.Size(),
		SHA256:    digest,
	}, nil
}

func writeSnapshotManifest(work string, manifest SnapshotManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: encoding the manifest: %w", err)
	}
	path := filepath.Join(work, SnapshotManifestMember)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("snapshot: writing the manifest: %w", err)
	}
	return nil
}

// writeSnapshotArchive streams the working directory into a tar archive, gzip
// compressed, optionally sealed by the chunked container.
//
// Nothing is buffered: the tar writer wraps the encryptor wraps the file, so
// the archive never exists in memory at any point.
func writeSnapshotArchive(work, outputPath, material string) error {
	if dir := filepath.Dir(outputPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("snapshot: creating %s: %w", dir, err)
		}
	}
	// O_EXCL: an archive that silently overwrites yesterday's is how a single
	// mistyped path turns into one backup instead of two. Refusing is cheap.
	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("snapshot: %s already exists; move it aside or choose another name", outputPath)
		}
		return fmt.Errorf("snapshot: creating %s: %w", outputPath, err)
	}

	var encryptor *snapshotEncryptor
	var sink io.Writer = file
	if material != "" {
		encryptor, err = newSnapshotEncryptor(file, material)
		if err != nil {
			file.Close()
			os.Remove(outputPath)
			return err
		}
		sink = encryptor
	}

	// A partially written archive is worse than none: it looks like a backup
	// and fails only when it is needed. Every failure past this point removes
	// the file.
	cleanup := func(err error) error {
		file.Close()
		os.Remove(outputPath)
		return err
	}

	gz := gzip.NewWriter(sink)
	tw := tar.NewWriter(gz)
	if err := writeSnapshotMembers(tw, work); err != nil {
		return cleanup(err)
	}
	if err := tw.Close(); err != nil {
		return cleanup(fmt.Errorf("snapshot: closing the archive: %w", err))
	}
	if err := gz.Close(); err != nil {
		return cleanup(fmt.Errorf("snapshot: finishing compression: %w", err))
	}
	if encryptor != nil {
		if err := encryptor.Close(); err != nil {
			return cleanup(err)
		}
	}
	if err := file.Close(); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("snapshot: closing %s: %w", outputPath, err)
	}
	return nil
}

// writeSnapshotMembers writes every file in the working directory, manifest
// first and the rest in a stable order so that two archives of the same data
// differ only in what genuinely differs.
func writeSnapshotMembers(tw *tar.Writer, work string) error {
	var members []string
	err := filepath.WalkDir(work, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(work, path)
		if err != nil {
			return err
		}
		members = append(members, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return fmt.Errorf("snapshot: listing the archive contents: %w", err)
	}
	sort.Strings(members)

	ordered := make([]string, 0, len(members))
	found := false
	for _, member := range members {
		if member == SnapshotManifestMember {
			found = true
			continue
		}
		ordered = append(ordered, member)
	}
	if !found {
		return errors.New("snapshot: the manifest was not written; refusing to produce an archive nothing can read")
	}
	ordered = append([]string{SnapshotManifestMember}, ordered...)

	for _, member := range ordered {
		if err := writeSnapshotMember(tw, work, member); err != nil {
			return err
		}
	}
	return nil
}

func writeSnapshotMember(tw *tar.Writer, work, member string) error {
	source := filepath.Join(work, filepath.FromSlash(member))
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("snapshot: %s: %w", member, err)
	}
	header := &tar.Header{
		Name:     member,
		Mode:     0o600,
		Size:     info.Size(),
		ModTime:  info.ModTime(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("snapshot: writing the header for %s: %w", member, err)
	}
	file, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("snapshot: opening %s: %w", member, err)
	}
	defer file.Close()
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("snapshot: writing %s: %w", member, err)
	}
	return nil
}

// safeMemberName rejects a name that would not stay a plain file name once it
// reached the filesystem.
func safeMemberName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("the name is empty")
	case name == "." || name == "..":
		return errors.New("the name is a directory reference")
	case strings.ContainsAny(name, "/\\"):
		return errors.New("the name contains a path separator")
	case filepath.IsAbs(name):
		return errors.New("the name is an absolute path")
	}
	return nil
}

// DSNPath turns a configured DSN into the file it names. It accepts the shapes
// this build writes -- a bare path, and file:<path> with optional driver
// parameters -- and returns "" for anything else, including the in-memory
// stores the tests use, which cannot be snapshotted by copying a file.
//
// It is exported so that a restore can resolve the same targets a snapshot was
// taken from without carrying a second, drifting implementation of the same
// rule: a restore that disagrees with the backup about where a store lives
// would replace the wrong file.
func DSNPath(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if after, ok := strings.CutPrefix(dsn, "file:"); ok {
		dsn = after
	}
	if i := strings.Index(dsn, "?"); i >= 0 {
		dsn = dsn[:i]
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" || dsn == ":memory:" {
		return ""
	}
	return dsn
}

// dsnPath is the internal spelling of DSNPath.
func dsnPath(dsn string) string { return DSNPath(dsn) }

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source, dest string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
