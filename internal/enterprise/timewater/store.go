// Package timewater stores a local trusted-time high watermark. It never proves
// that time is trustworthy and does not authorize serving traffic. Missing or
// corrupt state must keep dependent gates closed. A disk/VM snapshot rollback,
// cloned identity, or another machine cannot be detected by this file alone.
package timewater

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

var ErrIndeterminate = errors.New("timewater: replacement durability is uncertain")
var ErrRollback = errors.New("timewater: watermark would decrease")
var ErrExists = errors.New("timewater: watermark already exists")

const recordName = "watermark"
const lockName = ".timewater.lock"
const maxBytes = 64

// Store requires an existing 0700 directory owned by the effective uid. The
// directory must be protected from untrusted processes with the same uid.
type Store struct {
	dir     string
	syncDir func(int) error
}

func NewStore(dir string) *Store { return &Store{dir: dir, syncDir: unix.Fsync} }

func checkFile(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return errors.New("timewater: unsafe file type, ownership, mode or link count")
	}
	return nil
}
func (s *Store) locked(fn func(int) error) error {
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
			return fmt.Errorf("timewater: open directory: %w", e)
		}
		unix.Close(fd)
		fd = next
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&07777 != 0700 {
		return errors.New("timewater: unsafe directory ownership or mode")
	}
	lock, err := unix.Openat(fd, lockName, unix.O_RDWR|unix.O_CREAT|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return fmt.Errorf("timewater: open lock: %w", err)
	}
	defer unix.Close(lock)
	if err := checkFile(lock); err != nil {
		return err
	}
	if err := unix.Flock(lock, unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(lock, unix.LOCK_UN)
	return fn(fd)
}

func readAt(dir int) (int64, error) {
	fd, err := unix.Openat(dir, recordName, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, fmt.Errorf("timewater: open watermark: %w", err)
	}
	f := os.NewFile(uintptr(fd), recordName)
	defer f.Close()
	if err := checkFile(fd); err != nil {
		return 0, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return 0, err
	}
	if len(raw) > maxBytes || !strings.HasPrefix(string(raw), "v1\n") || !strings.HasSuffix(string(raw), "\n") {
		return 0, errors.New("timewater: invalid encoding")
	}
	text := string(raw[3 : len(raw)-1])
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != text {
		return 0, errors.New("timewater: invalid watermark")
	}
	return n, nil
}

// Read returns an error for missing, corrupt or unsafe state; callers must close gates.
func (s *Store) Read() (int64, error) {
	var n int64
	err := s.locked(func(fd int) error {
		var e error
		n, e = readAt(fd)
		if e != nil {
			return e
		}
		// A different process may have renamed the record but failed to
		// confirm directory durability. Do not report it as durable yet.
		return s.syncDirectory(fd)
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Bootstrap creates a record only if none exists. It is an explicit data setup
// operation, not proof of historical completeness, trusted time or authority.
// Never bootstrap automatically when prior history is unknown.
func (s *Store) Bootstrap(n int64) error {
	if n <= 0 {
		return errors.New("timewater: watermark must be positive")
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
		return s.writeAt(fd, n)
	})
}

// Advance stores n when it is not lower than the existing watermark. Equal
// retries sync the directory to resolve a prior uncertain rename.
func (s *Store) Advance(n int64) error {
	if n <= 0 {
		return errors.New("timewater: watermark must be positive")
	}
	return s.locked(func(fd int) error {
		old, err := readAt(fd)
		if err != nil {
			return err
		}
		if n < old {
			return ErrRollback
		}
		if n == old {
			return s.syncDirectory(fd)
		}
		return s.writeAt(fd, n)
	})
}
func (s *Store) syncDirectory(fd int) error {
	if err := s.syncDir(fd); err != nil {
		return errors.Join(ErrIndeterminate, err)
	}
	return nil
}
func (s *Store) writeAt(dir int, n int64) (err error) {
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		return err
	}
	name := ".timewater-" + hex.EncodeToString(b[:]) + ".tmp"
	fd, err := unix.Openat(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
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
	if _, err = fmt.Fprintf(f, "v1\n%d\n", n); err != nil {
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
