package sqlitepolicy

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
)

// Storage/IO errors are classified into sentinels so a caller (for example the
// DHCP ACK gate) can treat "the disk is full / the file system failed / the
// database is locked" as an explicit, non-ACKable failure instead of an
// opaque message. The write itself already failed; classification only makes
// the failure nameable and testable.

var (
	// ErrStorageFull reports SQLITE_FULL / ENOSPC.
	ErrStorageFull = errors.New("sqlite: storage full")
	// ErrStorageIO reports SQLITE_IOERR / EIO.
	ErrStorageIO = errors.New("sqlite: storage i/o error")
	// ErrBusy reports SQLITE_BUSY / SQLITE_LOCKED (lock contention/timeout).
	ErrBusy = errors.New("sqlite: database busy or locked")
)

// sqliteCoder is implemented by driver errors that expose a numeric result
// code (modernc.org/sqlite's *sqlite.Error has Code() int).
type sqliteCoder interface{ Code() int }

const (
	sqliteBusy   = 5
	sqliteLocked = 6
	sqliteIOErr  = 10
	sqliteFull   = 13
)

// ClassifyError wraps err with a sentinel describing the storage failure, or
// returns it unchanged when it is not a recognised storage/IO/lock error.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	var c sqliteCoder
	if errors.As(err, &c) {
		switch c.Code() {
		case sqliteFull:
			return fmt.Errorf("%w: %v", ErrStorageFull, err)
		case sqliteIOErr:
			return fmt.Errorf("%w: %v", ErrStorageIO, err)
		case sqliteBusy, sqliteLocked:
			return fmt.Errorf("%w: %v", ErrBusy, err)
		}
	}
	// Filesystem errors from WAL/db file operations.
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return fmt.Errorf("%w: %v", ErrStorageFull, err)
	case errors.Is(err, syscall.EIO):
		return fmt.Errorf("%w: %v", ErrStorageIO, err)
	}
	// Fallback for drivers that only expose a message.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "database or disk is full"), strings.Contains(msg, "disk full"):
		return fmt.Errorf("%w: %v", ErrStorageFull, err)
	case strings.Contains(msg, "disk i/o error"), strings.Contains(msg, "i/o error"):
		return fmt.Errorf("%w: %v", ErrStorageIO, err)
	case strings.Contains(msg, "database is locked"), strings.Contains(msg, "database table is locked"):
		return fmt.Errorf("%w: %v", ErrBusy, err)
	}
	return err
}
