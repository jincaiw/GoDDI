package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// manifestMaxBytes bounds the manifest a reader will decode. It is generously
// above anything this build writes and exists so that a hostile archive cannot
// make the restore host allocate without limit.
const manifestMaxBytes = 8 << 20

// SnapshotReadRequest is one read of an archive.
type SnapshotReadRequest struct {
	Path        string
	KeyMaterial string

	// ExtractTo unpacks the archive into a directory. Empty verifies only,
	// which is what an operator does before trusting a file they were handed.
	ExtractTo string

	// Replace allows ExtractTo to be an existing directory whose contents are
	// replaced. Without it, an existing target is refused: unpacking a backup
	// over a half-populated directory would leave a mixture of two instances.
	Replace bool
}

// SnapshotReadResult is what a read established.
type SnapshotReadResult struct {
	Manifest    SnapshotManifest
	Encrypted   bool
	Members     []SnapshotMember
	ExtractedTo string
}

// SnapshotMember is one verified member of the archive.
type SnapshotMember struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// InspectSnapshot reads and verifies an archive without unpacking it.
func InspectSnapshot(path, keyMaterial string) (*SnapshotManifest, error) {
	result, err := ReadSnapshot(SnapshotReadRequest{Path: path, KeyMaterial: keyMaterial})
	if err != nil {
		return nil, err
	}
	return &result.Manifest, nil
}

// ReadSnapshot verifies an archive, and optionally unpacks it, in a single
// pass.
//
// Verification is not optional and is not a separate mode: every member is
// hashed as it is read and compared against the manifest before anything is
// written, and the extraction lands in a staging directory that is renamed into
// place only once the whole archive has been read. A restore that half-unpacked
// an archive it was about to reject is its own outage.
func ReadSnapshot(req SnapshotReadRequest) (*SnapshotReadResult, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, errors.New("snapshot: no input path")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("snapshot: opening %s: %w", path, err)
	}
	defer file.Close()

	encrypted, err := snapshotIsEncrypted(file)
	if err != nil {
		return nil, fmt.Errorf("snapshot: %s: %w", path, err)
	}

	var plain io.Reader = file
	var decryptor *snapshotDecryptor
	if encrypted {
		material := strings.TrimSpace(req.KeyMaterial)
		if material == "" {
			return nil, errors.New("snapshot: this archive is encrypted and no key material is configured; set GODDI_SECURITY_ENCRYPTION_KEY to the material it was written with")
		}
		decryptor, err = newSnapshotDecryptor(file, material)
		if err != nil {
			return nil, err
		}
		plain = decryptor
	}

	gz, err := gzip.NewReader(plain)
	if err != nil {
		if encrypted {
			// Reaching here with the right header means the container opened
			// but the payload did not: the key is wrong, or the body was cut.
			return nil, fmt.Errorf("snapshot: %s decrypted to something that is not a compressed archive: the key material is not the one the archive was written with, or the archive is damaged (%w)", path, err)
		}
		return nil, fmt.Errorf("snapshot: %s starts with neither %s nor a gzip header; it is not a GoDDI snapshot (%w)", path, snapshotEncMagic, err)
	}
	defer gz.Close()

	result := &SnapshotReadResult{Encrypted: encrypted}
	tr := tar.NewReader(gz)

	// The staging directory exists only if something is being unpacked, and it
	// is a sibling of the destination so that the final rename cannot cross a
	// filesystem boundary.
	var staging string
	var destination string
	if strings.TrimSpace(req.ExtractTo) != "" {
		destination = filepath.Clean(req.ExtractTo)
		parent := filepath.Dir(destination)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return nil, fmt.Errorf("snapshot: preparing %s: %w", parent, err)
		}
		staging, err = os.MkdirTemp(parent, filepath.Base(destination)+".partial-")
		if err != nil {
			return nil, fmt.Errorf("snapshot: creating a staging directory: %w", err)
		}
		if err := os.Chmod(staging, 0o700); err != nil {
			return nil, fmt.Errorf("snapshot: securing the staging directory: %w", err)
		}
		// Once the rename has happened this is a no-op; until then it is what
		// keeps a rejected archive from leaving anything behind.
		defer os.RemoveAll(staging)
	}

	expected := map[string]expectedMember{}
	seen := map[string]bool{}
	first := true
	var manifest SnapshotManifest

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot: reading %s: %w", path, err)
		}
		name := filepath.ToSlash(header.Name)
		if err := validSnapshotMember(name); err != nil {
			return nil, fmt.Errorf("snapshot: %s holds %q: %w", path, name, err)
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("snapshot: %s holds %q, which is not a regular file; this archive was not written by this build", path, name)
		}

		if first {
			first = false
			if name != SnapshotManifestMember {
				return nil, fmt.Errorf("snapshot: %s begins with %q rather than %s; this archive was not written by this build", path, name, SnapshotManifestMember)
			}
			if err := json.NewDecoder(io.LimitReader(tr, manifestMaxBytes)).Decode(&manifest); err != nil {
				return nil, fmt.Errorf("snapshot: reading the manifest: %w", err)
			}
			if err := checkManifest(manifest, encrypted); err != nil {
				return nil, err
			}
			var err error
			if expected, err = expectedMembers(manifest); err != nil {
				return nil, err
			}
			result.Manifest = manifest
			continue
		}

		want, ok := expected[name]
		if !ok {
			return nil, fmt.Errorf("snapshot: %s holds %q, which the manifest does not describe; the archive is not the one it claims to be", path, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("snapshot: %s holds %q twice", path, name)
		}
		seen[name] = true

		var out *os.File
		hash := sha256.New()
		var sink io.Writer = hash
		if staging != "" {
			target := filepath.Join(staging, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return nil, fmt.Errorf("snapshot: preparing %s: %w", target, err)
			}
			out, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
			if err != nil {
				return nil, fmt.Errorf("snapshot: creating %s: %w", target, err)
			}
			sink = io.MultiWriter(hash, out)
		}

		size, copyErr := io.Copy(sink, tr)
		if out != nil {
			if cerr := out.Close(); cerr != nil && copyErr == nil {
				copyErr = cerr
			}
		}
		if copyErr != nil {
			return nil, fmt.Errorf("snapshot: reading %s: %w", name, copyErr)
		}

		digest := hex.EncodeToString(hash.Sum(nil))
		if size != want.size || digest != want.sha {
			return nil, fmt.Errorf("snapshot: %s does not match the manifest: the archive holds %d bytes with digest %s, the manifest records %d bytes with digest %s. The archive is damaged or was altered",
				name, size, shortDigest(digest), want.size, shortDigest(want.sha))
		}
		result.Members = append(result.Members, SnapshotMember{Name: name, SizeBytes: size, SHA256: digest})
	}

	if first {
		return nil, fmt.Errorf("snapshot: %s is empty", path)
	}
	var missing []string
	for name := range expected {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("snapshot: the manifest describes %s, which the archive does not hold; the archive is incomplete", strings.Join(missing, ", "))
	}

	// Asked for explicitly rather than left implicit: the archive should end
	// where its writer said it would.
	if decryptor != nil {
		if err := decryptor.Finish(); err != nil {
			return nil, err
		}
	}

	if staging != "" {
		if info, err := os.Stat(destination); err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("snapshot: %s exists and is not a directory", destination)
			}
			if !req.Replace {
				return nil, fmt.Errorf("snapshot: %s already exists; pass --replace to unpack over it", destination)
			}
			if err := os.RemoveAll(destination); err != nil {
				return nil, fmt.Errorf("snapshot: clearing %s: %w", destination, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot: checking %s: %w", destination, err)
		}
		if err := os.Rename(staging, destination); err != nil {
			return nil, fmt.Errorf("snapshot: moving the unpacked archive into %s: %w", destination, err)
		}
		result.ExtractedTo = destination
	}

	return result, nil
}

type expectedMember struct {
	size int64
	sha  string
}

func expectedMembers(manifest SnapshotManifest) (map[string]expectedMember, error) {
	out := make(map[string]expectedMember, len(manifest.Databases)+len(manifest.ConfigFiles))
	add := func(member string, size int64, sha string) error {
		if err := validSnapshotMember(member); err != nil {
			return fmt.Errorf("snapshot: the manifest names %q: %w", member, err)
		}
		if _, dup := out[member]; dup {
			return fmt.Errorf("snapshot: the manifest names %q twice", member)
		}
		out[member] = expectedMember{size: size, sha: sha}
		return nil
	}
	for _, database := range manifest.Databases {
		if err := add(database.Member, database.SizeBytes, database.SHA256); err != nil {
			return nil, err
		}
	}
	for _, file := range manifest.ConfigFiles {
		if err := add(file.Member, file.SizeBytes, file.SHA256); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// checkManifest refuses an archive whose declared layout or encryption this
// build does not implement, and one whose declaration disagrees with the file.
func checkManifest(manifest SnapshotManifest, encrypted bool) error {
	if manifest.FormatVersion != SnapshotFormatVersion {
		return fmt.Errorf("snapshot: the archive is format version %d and this build reads version %d; restore it with the build that wrote it",
			manifest.FormatVersion, SnapshotFormatVersion)
	}
	switch manifest.Encryption {
	case SnapshotEncryptionNone:
		if encrypted {
			return errors.New("snapshot: the file is encrypted but the manifest says it is not; the archive is inconsistent")
		}
	case SnapshotEncryptionChunkedGCM:
		if !encrypted {
			return errors.New("snapshot: the manifest says the archive is encrypted but the file is not; the archive is inconsistent")
		}
	default:
		return fmt.Errorf("snapshot: the archive declares encryption %q, which this build does not implement", manifest.Encryption)
	}
	return nil
}

// snapshotIsEncrypted decides which of the two containers the file uses, and
// refuses anything that is neither. Guessing would mean treating an unreadable
// file as a valid empty one.
func snapshotIsEncrypted(file *os.File) (bool, error) {
	head := make([]byte, len(snapshotEncMagic))
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	head = head[:n]
	if bytes.HasPrefix(head, []byte(snapshotEncMagic)) {
		return true, nil
	}
	if bytes.HasPrefix(head, []byte{0x1f, 0x8b}) {
		return false, nil
	}
	return false, errors.New("it starts with neither the encrypted container header nor a gzip header")
}

// validSnapshotMember accepts only the two member shapes this build writes. An
// unexpected member means the archive is not the one the manifest describes, so
// it is refused rather than skipped: a member silently left out of a restore is
// exactly the failure a restore is supposed to not have.
func validSnapshotMember(name string) error {
	if name == "" {
		return errors.New("the member name is empty")
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return fmt.Errorf("the member name is absolute")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("the member is not a plain relative path")
		}
	}
	switch {
	case name == SnapshotManifestMember:
		return nil
	case strings.HasPrefix(name, SnapshotDirDatabases+"/"):
		return nil
	case strings.HasPrefix(name, SnapshotDirConfig+"/"):
		return nil
	default:
		return fmt.Errorf("the member is outside %s/ and %s/", SnapshotDirDatabases, SnapshotDirConfig)
	}
}

// SnapshotKeyFingerprint reports the fingerprint a snapshot would record for a
// purpose under this key material. Callers use it to compare the material at
// hand against what a manifest was written with, before anything is restored.
func SnapshotKeyFingerprint(label, material string) string {
	return snapshotKeyFingerprint(label, material)
}

// SnapshotKeyMismatches returns the labels whose recorded fingerprint does not
// match the key material at hand. An empty result means the material was the one
// the manifest was written with.
//
// With the design as it stands -- one configured material, one derivation per
// purpose -- an archive that opens always matches, so this is a consistency
// assertion rather than a recovery tool: it fails if a reader ever opened a
// manifest with material other than what wrote it, which would be a defect in
// the derivation rather than something an operator can cause. It is kept
// because it is the assertion that would say so, and because it becomes load
// bearing the day the archive key and the column keys are allowed to differ.
//
// A label with no recorded fingerprint is not a mismatch and is not reported:
// an unencrypted archive records none, and a build that stops sealing a purpose
// would otherwise report a failure on an archive that is entirely readable.
func SnapshotKeyMismatches(manifest SnapshotManifest, material string) []string {
	var mismatched []string
	for _, label := range manifest.Secrets.Labels {
		recorded := manifest.Secrets.Keys[label]
		if recorded == "" {
			continue
		}
		if snapshotKeyFingerprint(label, material) != recorded {
			mismatched = append(mismatched, label)
		}
	}
	sort.Strings(mismatched)
	return mismatched
}

// SnapshotKeyLabels lists the purposes a manifest records key material for.
func SnapshotKeyLabels(manifest SnapshotManifest) []string {
	out := append([]string(nil), manifest.Secrets.Labels...)
	sort.Strings(out)
	return out
}

// shortDigest trims a hex digest for an error message. An operator comparing a
// digest by eye needs the first few characters; a full 64-character hex string
// twice in one line is unreadable, and the full value is in the manifest.
func shortDigest(digest string) string {
	if len(digest) <= 16 {
		return digest
	}
	return digest[:16] + "..."
}
