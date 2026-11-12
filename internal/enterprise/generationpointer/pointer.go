// Package generationpointer exposes a read-only durable generation record.
// A record or digest is not proof of safe installation, authority, or fencing.
// Full disk/VM rollback and malicious same-UID processes cannot be detected.
package generationpointer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var ErrNoAuthority = errors.New("generationpointer: no installation authority")
var ErrInvalid = errors.New("generationpointer: invalid record or identity")
var ErrTransition = errors.New("generationpointer: transition refused")
var ErrIndeterminate = errors.New("generationpointer: directory durability unconfirmed")

const recordName = "generation.json"
const lockName = ".generationpointer.lock"
const maxRecordBytes = 2048

// Import, Install, Activate, and Switch are intentionally unavailable. They
// cannot grant authority based on the result of Inspect or a digest.
func Import(context.Context) error   { return ErrNoAuthority }
func Install(context.Context) error  { return ErrNoAuthority }
func Activate(context.Context) error { return ErrNoAuthority }
func Switch(context.Context) error   { return ErrNoAuthority }

type Record struct {
	Version           string `json:"version"`
	ClusterID         string `json:"clusterId"`
	NodeID            string `json:"nodeId"`
	OldGeneration     string `json:"oldGeneration"`
	CurrentGeneration string `json:"currentGeneration"`
	TargetGeneration  string `json:"targetGeneration"`
	Phase             string `json:"phase"`
	EvidenceDigest    string `json:"evidenceDigest"`
}

// Expected binds inspection to independently selected identities, generations,
// and a nonzero canonical digest. It does not prove role or snapshot validity.
type Expected struct{ ClusterID, NodeID, OldGeneration, TargetGeneration, EvidenceDigest string }

func validUUID(s string) bool {
	u, err := uuid.Parse(s)
	return err == nil && u != uuid.Nil && u.String() == s
}
func (r Record) valid() error {
	if r.Version != "1" || !validUUID(r.ClusterID) || !validUUID(r.NodeID) || !validUUID(r.OldGeneration) || !validUUID(r.CurrentGeneration) || !validUUID(r.TargetGeneration) || r.ClusterID == r.NodeID || r.OldGeneration == r.TargetGeneration {
		return ErrInvalid
	}
	switch r.Phase {
	case "STAGED":
		if r.CurrentGeneration != r.OldGeneration {
			return ErrInvalid
		}
	case "INSTALLED":
		if r.CurrentGeneration != r.TargetGeneration {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if len(r.EvidenceDigest) != 64 || strings.ToLower(r.EvidenceDigest) != r.EvidenceDigest {
		return ErrInvalid
	}
	b, err := hex.DecodeString(r.EvidenceDigest)
	if err != nil || bytes.Equal(b, make([]byte, 32)) {
		return ErrInvalid
	}
	return nil
}
func (r Record) matches(e Expected) bool {
	return r.ClusterID == e.ClusterID && r.NodeID == e.NodeID && r.OldGeneration == e.OldGeneration && r.TargetGeneration == e.TargetGeneration && r.EvidenceDigest == e.EvidenceDigest
}

// Inspect only reports a canonical persisted record bound to independently
// supplied IDs. Directory must be pre-existing 0700, owned by the effective UID
// and inaccessible to untrusted same-UID processes. An error returns zero data.
// It requires an existing lock file and does not create files. Directory fsync
// cannot prove an arbitrary creator synced the record's contents before rename;
// only an independently trusted creator can establish that ordering. STAGED or
// INSTALLED never means that opening or activating a DB is safe.
func Inspect(dir string, expected Expected) (Record, error) { return newStore(dir).inspect(expected) }

type store struct {
	dir      string
	syncDir  func(int) error
	syncFile func(*os.File) error
	rename   func(int, string, int, string) error
	tempName func() (string, error)
}

func newStore(dir string) *store {
	return &store{dir: dir, syncDir: unix.Fsync, syncFile: func(f *os.File) error { return f.Sync() }, rename: unix.Renameat, tempName: randomName}
}
func randomName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".generation-" + hex.EncodeToString(b[:]) + ".tmp", nil
}
func checkFile(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return errors.New("generationpointer: unsafe file")
	}
	return nil
}
func (s *store) sync(fd int) error {
	if err := s.syncDir(fd); err != nil {
		return errors.Join(ErrIndeterminate, err)
	}
	return nil
}
func (s *store) locked(createLock bool, fn func(int) error) error {
	p, err := filepath.Abs(s.dir)
	if err != nil {
		return err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { unix.Close(fd) }()
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Clean(p), "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return e
		}
		unix.Close(fd)
		fd = next
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&07777 != 0700 {
		return errors.New("generationpointer: unsafe directory")
	}
	flags := unix.O_RDONLY | unix.O_NONBLOCK | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if createLock {
		flags = unix.O_RDWR | unix.O_CREAT | unix.O_NONBLOCK | unix.O_NOFOLLOW | unix.O_CLOEXEC
	}
	lock, err := unix.Openat(fd, lockName, flags, 0600)
	if err != nil {
		return err
	}
	defer unix.Close(lock)
	if err = checkFile(lock); err != nil {
		return err
	}
	if err = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	defer unix.Flock(lock, unix.LOCK_UN)
	return fn(fd)
}
func readAt(fd int) (Record, error) {
	file, err := unix.Openat(fd, recordName, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return Record{}, err
	}
	f := os.NewFile(uintptr(file), recordName)
	defer f.Close()
	if err = checkFile(file); err != nil {
		return Record{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return Record{}, err
	}
	if len(raw) > maxRecordBytes {
		return Record{}, ErrInvalid
	}
	var r Record
	if err = json.Unmarshal(raw, &r); err != nil {
		return Record{}, ErrInvalid
	}
	if err = r.valid(); err != nil {
		return Record{}, err
	}
	canonical, err := json.Marshal(r)
	if err != nil || !bytes.Equal(raw, canonical) {
		return Record{}, ErrInvalid
	}
	return r, nil
}
func (s *store) inspect(expected Expected) (Record, error) {
	var r Record
	err := s.locked(false, func(fd int) error {
		value, e := readAt(fd)
		if e != nil {
			return e
		}
		if !value.matches(expected) {
			return ErrInvalid
		}
		if e = s.sync(fd); e != nil {
			return e
		}
		r = value
		return nil
	})
	if err != nil {
		return Record{}, err
	}
	return r, nil
}

// stage and advance are unexported experimental primitives; no production
// caller can acquire authority from them. Tests exercise filesystem semantics.
func (s *store) stage(r Record) error {
	if err := r.valid(); err != nil {
		return err
	}
	if r.Phase != "STAGED" {
		return ErrTransition
	}
	return s.locked(true, func(fd int) error {
		var st unix.Stat_t
		e := unix.Fstatat(fd, recordName, &st, unix.AT_SYMLINK_NOFOLLOW)
		if e == nil {
			return ErrTransition
		}
		if !errors.Is(e, unix.ENOENT) {
			return e
		}
		return s.writeAt(fd, r)
	})
}
func (s *store) advance(next Record, expected Expected) error {
	if err := next.valid(); err != nil {
		return err
	}
	if !next.matches(expected) || next.Phase != "INSTALLED" {
		return ErrTransition
	}
	return s.locked(false, func(fd int) error {
		old, err := readAt(fd)
		if err != nil {
			return err
		}
		if !old.matches(expected) {
			return ErrTransition
		}
		if old == next {
			return s.sync(fd)
		}
		if old.Phase != "STAGED" {
			return ErrTransition
		}
		old.Phase = "INSTALLED"
		old.CurrentGeneration = old.TargetGeneration
		if old != next {
			return ErrTransition
		}
		return s.writeAt(fd, next)
	})
}
func (s *store) writeAt(dir int, r Record) (err error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	name, err := s.tempName()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(name, ".generation-") || !strings.HasSuffix(name, ".tmp") || strings.Contains(name, "/") {
		return ErrInvalid
	}
	fd, err := unix.Openat(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), name)
	defer func() {
		if e := unix.Unlinkat(dir, name, 0); e != nil && !errors.Is(e, unix.ENOENT) {
			err = errors.Join(err, fmt.Errorf("generationpointer: temp cleanup: %w", e))
		}
	}()
	defer f.Close()
	if err = checkFile(fd); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = s.syncFile(f); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = s.rename(dir, name, dir, recordName); err != nil {
		return errors.Join(ErrIndeterminate, err)
	}
	return s.sync(dir)
}
