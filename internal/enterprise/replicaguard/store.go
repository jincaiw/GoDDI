// Package replicaguard stores a local quarantine marker. It cannot authorize
// replication, clear quarantine, or detect rollback/cloning of a disk or VM.
package replicaguard

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var ErrInvalid = errors.New("replicaguard: invalid quarantine record")
var ErrExists = errors.New("replicaguard: record already exists")
var ErrTransition = errors.New("replicaguard: identity or quarantine change refused")
var ErrIndeterminate = errors.New("replicaguard: directory durability uncertain")

const recordName = "replica-quarantine.json"
const lockName = ".replicaguard.lock"
const maxRecordBytes = 2048

// Record is a bounded diagnostic marker, not proof of authority. Reason is an
// enumerated code rather than arbitrary error text, avoiding secret leakage.
type Record struct {
	Version        string `json:"version"`
	State          string `json:"state"`
	ClusterID      string `json:"clusterId"`
	NodeID         string `json:"nodeId"`
	DBGeneration   string `json:"databaseGeneration"`
	Reason         string `json:"reason"`
	Epoch          uint64 `json:"epoch"`
	Seq            uint64 `json:"seq"`
	PositionDigest string `json:"positionDigest"`
}

func (r Record) validate() error {
	if r.Version != "1" || r.State != "QUARANTINED" {
		return ErrInvalid
	}
	for _, s := range []string{r.ClusterID, r.NodeID} {
		u, err := uuid.Parse(s)
		if err != nil || u == uuid.Nil || u.String() != s {
			return ErrInvalid
		}
	}
	if r.ClusterID == r.NodeID {
		return ErrInvalid
	}
	if len(r.DBGeneration) == 0 || len(r.DBGeneration) > 128 {
		return ErrInvalid
	}
	for i := range r.DBGeneration {
		c := r.DBGeneration[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return ErrInvalid
		}
	}
	switch r.Reason {
	case "BOOTSTRAP", "CHAIN_GAP", "DIVERGENCE", "INVALID_EVENT", "EPOCH_MISMATCH", "IDENTITY_MISMATCH", "STORAGE_FAILURE", "UNKNOWN_TAIL":
	default:
		return ErrInvalid
	}
	if r.Epoch == 0 || r.Epoch > math.MaxInt64 || r.Seq > math.MaxInt64 || len(r.PositionDigest) != 64 || strings.ToLower(r.PositionDigest) != r.PositionDigest {
		return ErrInvalid
	}
	if _, err := hex.DecodeString(r.PositionDigest); err != nil {
		return ErrInvalid
	}
	return nil
}

// Store requires a pre-existing 0700 directory owned by the effective UID,
// protected from untrusted processes sharing that UID. State is not protected
// from full filesystem or VM snapshot rollback; external evidence is required.
type Store struct {
	dir      string
	syncDir  func(int) error
	tempName func() (string, error)
}

func NewStore(dir string) *Store { return &Store{dir: dir, syncDir: unix.Fsync, tempName: randomName} }
func randomName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".replicaguard-" + hex.EncodeToString(b[:]) + ".tmp", nil
}
func checkFile(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return errors.New("replicaguard: unsafe file")
	}
	return nil
}
func (s *Store) sync(fd int) error {
	if err := s.syncDir(fd); err != nil {
		return errors.Join(ErrIndeterminate, err)
	}
	return nil
}
func (s *Store) locked(fn func(int) error) error {
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
			return fmt.Errorf("replicaguard: open directory: %w", e)
		}
		unix.Close(fd)
		fd = next
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&07777 != 0700 {
		return errors.New("replicaguard: unsafe directory")
	}
	lock, err := unix.Openat(fd, lockName, unix.O_RDWR|unix.O_CREAT|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
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
func readAt(dir int) (Record, error) {
	fd, err := unix.Openat(dir, recordName, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return Record{}, err
	}
	f := os.NewFile(uintptr(fd), recordName)
	defer f.Close()
	if err = checkFile(fd); err != nil {
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
	if err = r.validate(); err != nil {
		return Record{}, err
	}
	canonical, err := json.Marshal(r)
	if err != nil || !bytes.Equal(raw, canonical) {
		return Record{}, ErrInvalid
	}
	return r, nil
}

// Read returns a zero record on every error and confirms directory durability.
func (s *Store) Read() (Record, error) {
	var r Record
	err := s.locked(func(fd int) error {
		v, e := readAt(fd)
		if e != nil {
			return e
		}
		if e = s.sync(fd); e != nil {
			return e
		}
		r = v
		return nil
	})
	if err != nil {
		return Record{}, err
	}
	return r, nil
}

// Bootstrap explicitly creates only a quarantined marker. It must never be
// invoked automatically when previous state or database history is unknown.
func (s *Store) Bootstrap(r Record) error {
	if err := r.validate(); err != nil {
		return err
	}
	return s.locked(func(fd int) error {
		var st unix.Stat_t
		err := unix.Fstatat(fd, recordName, &st, unix.AT_SYMLINK_NOFOLLOW)
		if err == nil {
			return ErrExists
		}
		if !errors.Is(err, unix.ENOENT) {
			return err
		}
		return s.writeAt(fd, r)
	})
}

// Quarantine only reconfirms the exact existing record. Keeping the first
// diagnostic avoids overwriting prior evidence; new identities and changes of
// generation or state require a separate, independently authorized protocol.
func (s *Store) Quarantine(r Record) error {
	if err := r.validate(); err != nil {
		return err
	}
	return s.locked(func(fd int) error {
		old, err := readAt(fd)
		if err != nil {
			return err
		}
		if old != r {
			return ErrTransition
		}
		return s.sync(fd)
	})
}
func (s *Store) writeAt(dir int, r Record) (err error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	name, err := s.tempName()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(name, ".replicaguard-") || !strings.HasSuffix(name, ".tmp") || strings.Contains(name, "/") {
		return ErrInvalid
	}
	fd, err := unix.Openat(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), name)
	defer func() {
		if e := unix.Unlinkat(dir, name, 0); e != nil && !errors.Is(e, unix.ENOENT) {
			err = errors.Join(err, e)
		}
	}()
	defer f.Close()
	if err = checkFile(fd); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = unix.Renameat(dir, name, dir, recordName); err != nil {
		return err
	}
	return s.sync(dir)
}
