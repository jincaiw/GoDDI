package rolelog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// ErrIndeterminate means the replacement occurred but directory durability
// could not be confirmed. The caller must keep all role gates closed.
var ErrIndeterminate = errors.New("rolelog: replacement durability is unknown")
var ErrTransition = errors.New("rolelog: forbidden record transition")

const recordName = "role.json"
const lockName = ".role.lock"

// Store persists data only; it does not authenticate permits or grant authority.
// The directory must already exist, be owned by this user, and have mode 0700.
// It must be inaccessible to untrusted processes running as the same user.
// A local record cannot detect a rollback of an entire disk or VM snapshot.
type Store struct {
	dir      string
	tempName func() (string, error)
	syncDir  func(int) error
}

func NewStore(dir string) *Store {
	return &Store{dir: dir, tempName: randomTempName, syncDir: unix.Fsync}
}

func (s *Store) syncDirectory(fd int) error {
	if err := s.syncDir(fd); err != nil {
		return errors.Join(ErrIndeterminate, err)
	}
	return nil
}

func randomTempName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".role-" + hex.EncodeToString(b[:]) + ".tmp", nil
}

// withDirectory resolves every component without following symbolic links and
// locks a directory-local file across processes before reading or replacing.
func (s *Store) withDirectory(fn func(int) error) error {
	path, err := filepath.Abs(s.dir)
	if err != nil {
		return err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { unix.Close(fd) }()
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return fmt.Errorf("rolelog: open directory: %w", e)
		}
		unix.Close(fd)
		fd = next
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&07777 != 0700 {
		return fmt.Errorf("rolelog: unsafe directory ownership or mode")
	}
	lock, err := unix.Openat(fd, lockName, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return fmt.Errorf("rolelog: open lock: %w", err)
	}
	defer unix.Close(lock)
	if err = checkFile(lock); err != nil {
		return err
	}
	if err = unix.Flock(lock, unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(lock, unix.LOCK_UN)
	return fn(fd)
}

func checkFile(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return fmt.Errorf("rolelog: unsafe file ownership, type, links or mode")
	}
	return nil
}

func readAt(dir int) (Record, error) {
	fd, err := unix.Openat(dir, recordName, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return Record{}, fmt.Errorf("rolelog: read record: %w", err)
	}
	f := os.NewFile(uintptr(fd), recordName)
	defer f.Close()
	if err = checkFile(fd); err != nil {
		return Record{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxRecordBytes+1))
	if err != nil {
		return Record{}, err
	}
	return Parse(raw)
}

// Read fails closed for missing, invalid, unsafe, or not durably confirmed records.
func (s *Store) Read() (Record, error) {
	var record Record
	err := s.withDirectory(func(fd int) error {
		value, err := readAt(fd)
		if err != nil {
			return err
		}
		if err := s.syncDirectory(fd); err != nil {
			return err
		}
		record = value
		return nil
	})
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

// Initialize creates only a BOOTSTRAP record and never replaces an existing one.
// It is not evidence of a valid role or permission to write business data.
func (s *Store) Initialize(r Record) error {
	if err := validate(r); err != nil {
		return err
	}
	if r.Phase != "BOOTSTRAP" {
		return fmt.Errorf("%w: initialization requires BOOTSTRAP", ErrTransition)
	}
	return s.withDirectory(func(fd int) error {
		var st unix.Stat_t
		err := unix.Fstatat(fd, recordName, &st, unix.AT_SYMLINK_NOFOLLOW)
		if err == nil {
			return fmt.Errorf("%w: record already exists", ErrTransition)
		}
		if !errors.Is(err, unix.ENOENT) {
			return err
		}
		return s.writeAt(fd, r)
	})
}

// Save permits only an identical record or a transition to FENCED in the same
// epoch with the same identity, nonce and evidence. Advancing epochs is refused
// until a durable nonce ledger and independently authenticated role protocol
// exist. Missing and corrupt records never bootstrap implicitly.
func (s *Store) Save(r Record) error {
	if err := validate(r); err != nil {
		return err
	}
	return s.withDirectory(func(fd int) error {
		old, err := readAt(fd)
		if err != nil {
			return err
		}
		if old == r {
			// A prior replacement may have reached rename but failed to sync
			// the directory. Reconfirm durability before reporting success.
			return s.syncDirectory(fd)
		}
		expected := old
		expected.Phase = "FENCED"
		if old.Phase == "FENCED" || r != expected {
			return ErrTransition
		}
		return s.writeAt(fd, r)
	})
}

func (s *Store) writeAt(dir int, r Record) (err error) {
	// Parse's wire format uses lower-case names and a decimal string for epoch.
	raw, err := json.Marshal(map[string]string{"version": r.Version, "cluster_id": r.ClusterID, "node_id": r.NodeID, "device_id": r.DeviceID, "epoch": strconv.FormatUint(r.Epoch, 10), "phase": r.Phase, "nonce": r.Nonce, "grant_digest": r.GrantDigest, "evidence_digest": r.EvidenceDigest, "db_generation": r.DBGeneration})
	if err != nil {
		return err
	}
	name, err := s.tempName()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(name, ".role-") || !strings.HasSuffix(name, ".tmp") || strings.Contains(name, "/") {
		return fmt.Errorf("rolelog: invalid temporary name")
	}
	fd, err := unix.Openat(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return fmt.Errorf("rolelog: create temporary record: %w", err)
	}
	f := os.NewFile(uintptr(fd), name)
	defer func() {
		if e := unix.Unlinkat(dir, name, 0); e != nil && !errors.Is(e, unix.ENOENT) {
			err = errors.Join(err, fmt.Errorf("rolelog: cleanup temporary record: %w", e))
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
	return s.syncDirectory(dir)
}
