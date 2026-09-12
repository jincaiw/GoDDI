package lease

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const currentWALEventVersion = 1

var (
	ErrWALCorrupt = errors.New("lease wal: corrupt record")
	ErrWALGap     = errors.New("lease wal: sequence gap")
)

// WALEvent is the versioned durable boundary record. The complete Lease value
// is carried so replay can rebuild the read index without consulting SQLite.
type WALEvent struct {
	Version int    `json:"version"`
	Seq     int64  `json:"seq"`
	Op      string `json:"op"`
	Lease   *Lease `json:"lease,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
}

const (
	WALEventUpsert = "upsert"
	WALEventRemove = "remove"
)

func EncodeWALEvent(event WALEvent) ([]byte, error) {
	if err := validateWALEvent(event); err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

func validateWALEvent(event WALEvent) error {
	if event.Version != currentWALEventVersion || event.Seq <= 0 {
		return fmt.Errorf("%w: version=%d seq=%d", ErrWALCorrupt, event.Version, event.Seq)
	}
	switch event.Op {
	case WALEventUpsert:
		if event.Lease == nil || event.Lease.ID == "" {
			return fmt.Errorf("%w: upsert lease is missing", ErrWALCorrupt)
		}
	case WALEventRemove:
		if event.LeaseID == "" {
			return fmt.Errorf("%w: remove lease id is missing", ErrWALCorrupt)
		}
	default:
		return fmt.Errorf("%w: unknown operation %q", ErrWALCorrupt, event.Op)
	}
	return nil
}

// AppendWALEvent writes one newline-delimited record. The caller owns the
// durable boundary and must call Sync before treating the event as ACK-safe.
func AppendWALEvent(w io.Writer, event WALEvent) error {
	encoded, err := EncodeWALEvent(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", encoded)
	return err
}

// ReplayWAL replays records in order. Duplicate sequence numbers are
// idempotently ignored; gaps and malformed records fail closed.
func ReplayWAL(r io.Reader, applied int64, apply func(WALEvent) error) (int64, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		var event WALEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return applied, fmt.Errorf("%w: decode: %v", ErrWALCorrupt, err)
		}
		if err := validateWALEvent(event); err != nil {
			return applied, err
		}
		if event.Seq <= applied {
			continue
		}
		if event.Seq != applied+1 {
			return applied, fmt.Errorf("%w: expected=%d got=%d", ErrWALGap, applied+1, event.Seq)
		}
		if err := apply(event); err != nil {
			return applied, err
		}
		applied = event.Seq
	}
	if err := scanner.Err(); err != nil {
		return applied, fmt.Errorf("%w: read: %v", ErrWALCorrupt, err)
	}
	return applied, nil
}

// WALWriter serializes append operations and exposes an explicit Sync boundary.
type WALWriter struct {
	mu   sync.Mutex
	file interface {
		io.Writer
		Sync() error
	}
	seq int64
}

func NewWALWriter(file interface {
	io.Writer
	Sync() error
}, nextSeq int64) *WALWriter {
	return &WALWriter{file: file, seq: nextSeq}
}

func (w *WALWriter) Append(event WALEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if event.Seq != w.seq+1 {
		return fmt.Errorf("%w: expected=%d got=%d", ErrWALGap, w.seq+1, event.Seq)
	}
	if err := AppendWALEvent(w.file, event); err != nil {
		return err
	}
	w.seq = event.Seq
	return nil
}

func (w *WALWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Sync()
}

func (w *WALWriter) Sequence() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seq
}
