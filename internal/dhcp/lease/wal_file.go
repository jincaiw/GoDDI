package lease

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	ErrWALHeader = errors.New("lease wal: invalid header")
	ErrWALClosed = errors.New("lease wal: closed")
)

const (
	walHeaderVersion byte = 1
	walHeaderMagic        = "GODDI-LEASE-WAL"
)

var walHeader = []byte(walHeaderMagic + "\x00" + string([]byte{walHeaderVersion}) + "\n")

// WALFile owns the lifecycle of an on-disk lease WAL. The file format is a
// small versioned header followed by the newline-delimited WALEvent records
// handled by ReplayWAL. Append does not imply durability; callers must call
// Sync before using the record as an ACK-safe boundary.
type WALFile struct {
	mu     sync.Mutex
	file   *os.File
	writer *WALWriter
}

// OpenWAL opens or creates path with owner-only permissions. Existing records
// are fully validated before the file is made available for appends. A gap or
// corrupt record therefore fails closed during startup rather than later in
// the DHCP request path.
func OpenWAL(path string) (*WALFile, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lease wal: %w", err)
	}
	closeOnError := func(err error) (*WALFile, error) {
		_ = file.Close()
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		return closeOnError(fmt.Errorf("stat lease wal: %w", err))
	}
	if info.Size() == 0 {
		if _, err := file.Write(walHeader); err != nil {
			return closeOnError(fmt.Errorf("write lease wal header: %w", err))
		}
		if err := file.Sync(); err != nil {
			return closeOnError(fmt.Errorf("sync lease wal header: %w", err))
		}
	} else if err := validateWALHeader(file, info.Size()); err != nil {
		return closeOnError(err)
	}

	last, err := replayWALFile(file, 0, func(WALEvent) error { return nil })
	if err != nil {
		return closeOnError(err)
	}
	return &WALFile{file: file, writer: NewWALWriter(file, last)}, nil
}

func validateWALHeader(file *os.File, size int64) error {
	if size < int64(len(walHeader)) {
		return fmt.Errorf("%w: truncated header", ErrWALHeader)
	}
	got := make([]byte, len(walHeader))
	if _, err := file.ReadAt(got, 0); err != nil {
		return fmt.Errorf("%w: %v", ErrWALHeader, err)
	}
	if string(got) != string(walHeader) {
		return fmt.Errorf("%w: version or magic mismatch", ErrWALHeader)
	}
	return nil
}

func replayWALFile(file *os.File, applied int64, apply func(WALEvent) error) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return applied, fmt.Errorf("stat lease wal for replay: %w", err)
	}
	reader := io.NewSectionReader(file, int64(len(walHeader)), info.Size()-int64(len(walHeader)))
	last, err := ReplayWAL(reader, applied, apply)
	if err != nil {
		return last, fmt.Errorf("replay lease wal: %w", err)
	}
	return last, nil
}

// Append adds one validated event in sequence order. It does not call Sync.
func (w *WALFile) Append(event WALEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || w.writer == nil {
		return ErrWALClosed
	}
	return w.writer.Append(event)
}

// Sync flushes the WAL through the operating system's file sync boundary.
func (w *WALFile) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || w.writer == nil {
		return ErrWALClosed
	}
	return w.writer.Sync()
}

// Sequence returns the highest successfully appended sequence number.
func (w *WALFile) Sequence() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer == nil {
		return 0
	}
	return w.writer.Sequence()
}

// Replay applies records after applied. It does not mutate the append
// sequence; replay is intentionally separate from the SQLite applied
// watermark so recovery can rebuild memory before admission is enabled.
func (w *WALFile) Replay(applied int64, apply func(WALEvent) error) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || w.writer == nil {
		return applied, ErrWALClosed
	}
	return replayWALFile(w.file, applied, apply)
}

// Close closes the file without implicitly claiming a durable boundary.
// Call Sync explicitly when the caller needs the close to be durable.
func (w *WALFile) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return ErrWALClosed
	}
	file := w.file
	w.file = nil
	w.writer = nil
	return file.Close()
}
