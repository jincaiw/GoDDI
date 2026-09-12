package lease

import (
	"errors"
	"fmt"
	"io"
	"sort"
)

var ErrRecoveryTarget = errors.New("lease wal: recovery target is nil")

// RecoverMemoryIndex rebuilds a temporary lease snapshot from the durable
// baseline and WAL, then atomically publishes it to target only after the
// complete sequence has been validated. The target is untouched on any replay
// or apply error, so callers can keep the data plane unready fail-closed.
func RecoverMemoryIndex(target *MemoryIndex, baseline []Lease, pools []AddressPool, r io.Reader, applied int64) (int64, error) {
	if target == nil {
		return applied, ErrRecoveryTarget
	}
	if r == nil {
		return applied, fmt.Errorf("%w: WAL reader is nil", ErrWALCorrupt)
	}
	return recoverMemoryIndex(target, baseline, pools, func(apply func(WALEvent) error) (int64, error) {
		return ReplayWAL(r, applied, apply)
	})
}

// RecoverMemoryIndex rebuilds an index directly from this open WAL file. The
// file remains owned by the WAL lifecycle component, so callers do not need to
// reopen or seek it during startup assembly.
func (w *WALFile) RecoverMemoryIndex(target *MemoryIndex, baseline []Lease, pools []AddressPool, applied int64) (int64, error) {
	if w == nil {
		return applied, ErrWALClosed
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || w.writer == nil {
		return applied, ErrWALClosed
	}
	return recoverMemoryIndex(target, baseline, pools, func(apply func(WALEvent) error) (int64, error) {
		return replayWALFile(w.file, applied, apply)
	})
}

func recoverMemoryIndex(target *MemoryIndex, baseline []Lease, pools []AddressPool, replay func(func(WALEvent) error) (int64, error)) (int64, error) {
	if target == nil {
		return 0, ErrRecoveryTarget
	}
	values := make(map[string]Lease, len(baseline))
	for _, value := range baseline {
		if value.ID == "" {
			return 0, fmt.Errorf("%w: baseline lease id is empty", ErrWALCorrupt)
		}
		values[value.ID] = value
	}
	last, err := replay(func(event WALEvent) error {
		switch event.Op {
		case WALEventUpsert:
			values[event.Lease.ID] = *event.Lease
		case WALEventRemove:
			delete(values, event.LeaseID)
		default:
			return fmt.Errorf("%w: unsupported recovery operation %q", ErrWALCorrupt, event.Op)
		}
		return nil
	})
	if err != nil {
		return last, err
	}

	recovered := make([]Lease, 0, len(values))
	for _, value := range values {
		recovered = append(recovered, value)
	}
	sort.Slice(recovered, func(i, j int) bool { return recovered[i].ID < recovered[j].ID })
	target.Replace(recovered, pools)
	return last, nil
}
