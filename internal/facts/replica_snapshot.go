package facts

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
)

const ReplicaSnapshotVersion = 1

// ReplicaSnapshotManifest identifies a complete, ordered facts snapshot.
// Digest covers the canonical JSON encoding of each ReplicaEvent, prefixed by
// its 8-byte big-endian length. It is independent of transport frame boundaries.
type ReplicaSnapshotManifest struct {
	Version      int    `json:"version"`
	LastSequence int64  `json:"last_sequence"`
	EventCount   int64  `json:"event_count"`
	Digest       string `json:"digest"`
}

// ReplicaSnapshotAccumulator validates page continuity while building a
// bounded-memory digest for one stable snapshot read.
type ReplicaSnapshotAccumulator struct {
	hash         hash.Hash
	lastSequence int64
	eventCount   int64
	nextAfter    int64
	complete     bool
	finished     bool
}

func NewReplicaSnapshotAccumulator(lastSequence int64) (*ReplicaSnapshotAccumulator, error) {
	if lastSequence < 0 {
		return nil, fmt.Errorf("facts: invalid replica snapshot sequence %d", lastSequence)
	}
	return &ReplicaSnapshotAccumulator{
		hash: sha256.New(), lastSequence: lastSequence, complete: lastSequence == 0,
	}, nil
}

// AddPage appends one page from the same read transaction. Pages must start at
// the previous cursor and report the same allocator high water.
func (a *ReplicaSnapshotAccumulator) AddPage(page ReplicaPage) error {
	if a == nil || a.hash == nil || a.finished {
		return errors.New("facts: replica snapshot accumulator is unavailable")
	}
	if a.complete {
		return errors.New("facts: replica snapshot already complete")
	}
	if page.LastSequence != a.lastSequence {
		return fmt.Errorf("facts: replica snapshot high water changed: expected=%d got=%d", a.lastSequence, page.LastSequence)
	}
	if page.NextAfter < a.nextAfter || page.NextAfter > a.lastSequence {
		return fmt.Errorf("facts: invalid replica snapshot cursor %d after %d (head=%d)", page.NextAfter, a.nextAfter, a.lastSequence)
	}
	expected := a.nextAfter + 1
	encodedEvents := make([][]byte, 0, len(page.Events))
	for _, event := range page.Events {
		if event.Envelope.Sequence != expected {
			return fmt.Errorf("%w: expected=%d got=%d", ErrReplicaSequenceGap, expected, event.Envelope.Sequence)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("facts: encode replica event %s: %w", event.Envelope.EventID, err)
		}
		encodedEvents = append(encodedEvents, encoded)
		expected++
	}
	if len(page.Events) == 0 && !page.Complete {
		return errors.New("facts: empty replica snapshot page is not complete")
	}
	if page.NextAfter != expected-1 || page.Complete != (page.NextAfter == a.lastSequence) {
		return fmt.Errorf("facts: inconsistent replica page cursor=%d expected=%d complete=%t head=%d", page.NextAfter, expected-1, page.Complete, a.lastSequence)
	}
	for _, encoded := range encodedEvents {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(encoded)))
		_, _ = a.hash.Write(length[:])
		_, _ = a.hash.Write(encoded)
		a.eventCount++
	}
	a.nextAfter = page.NextAfter
	a.complete = page.Complete
	return nil
}

func (a *ReplicaSnapshotAccumulator) Manifest() (ReplicaSnapshotManifest, error) {
	if a == nil || a.hash == nil || a.finished || !a.complete || a.nextAfter != a.lastSequence || a.eventCount != a.lastSequence {
		return ReplicaSnapshotManifest{}, errors.New("facts: replica snapshot is incomplete")
	}
	a.finished = true
	return ReplicaSnapshotManifest{
		Version: ReplicaSnapshotVersion, LastSequence: a.lastSequence,
		EventCount: a.eventCount, Digest: hex.EncodeToString(a.hash.Sum(nil)),
	}, nil
}
