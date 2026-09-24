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
	ChunkCount   int64  `json:"chunk_count"`
	Digest       string `json:"digest"`
}

// ReplicaSnapshotChunk is a transport-neutral bounded group of consecutive
// events. Index is zero-based and FirstSequence/LastSequence bind its declared
// boundaries to its contents.
type ReplicaSnapshotChunk struct {
	Index         int64          `json:"index"`
	FirstSequence int64          `json:"first_sequence"`
	LastSequence  int64          `json:"last_sequence"`
	Events        []ReplicaEvent `json:"events"`
}

// ReplicaSnapshotAccumulator validates page continuity while building a
// bounded-memory digest for one stable snapshot read.
type ReplicaSnapshotAccumulator struct {
	hash         hash.Hash
	lastSequence int64
	eventCount   int64
	chunkCount   int64
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
	if len(page.Events) > maxReplicaPageEvents {
		return errors.New("facts: replica snapshot page exceeds event limit")
	}
	if page.NextAfter < a.nextAfter || page.NextAfter > a.lastSequence {
		return fmt.Errorf("facts: invalid replica snapshot cursor %d after %d (head=%d)", page.NextAfter, a.nextAfter, a.lastSequence)
	}
	expected := a.nextAfter + 1
	encodedEvents := make([][]byte, 0, len(page.Events))
	encodedBytes := 0
	for _, event := range page.Events {
		if event.Envelope.Sequence != expected {
			return fmt.Errorf("%w: expected=%d got=%d", ErrReplicaSequenceGap, expected, event.Envelope.Sequence)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("facts: encode replica event %s: %w", event.Envelope.EventID, err)
		}
		encodedBytes += len(encoded)
		if encodedBytes > maxReplicaPageBytes {
			return errors.New("facts: replica snapshot page exceeds byte limit")
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
	if len(page.Events) > 0 {
		a.chunkCount++
	}
	return nil
}

// AddChunk checks the declared chunk index and sequence range before adding
// its events to the snapshot digest.
func (a *ReplicaSnapshotAccumulator) AddChunk(chunk ReplicaSnapshotChunk) error {
	if a == nil || a.hash == nil || a.finished {
		return errors.New("facts: replica snapshot accumulator is unavailable")
	}
	if len(chunk.Events) == 0 {
		return errors.New("facts: replica snapshot chunk is empty")
	}
	if chunk.Index != a.chunkCount {
		return fmt.Errorf("facts: replica snapshot chunk index=%d expected=%d", chunk.Index, a.chunkCount)
	}
	first := chunk.Events[0].Envelope.Sequence
	last := chunk.Events[len(chunk.Events)-1].Envelope.Sequence
	if chunk.FirstSequence != first || chunk.LastSequence != last || first != a.nextAfter+1 {
		return fmt.Errorf("facts: invalid replica snapshot chunk range declared=%d..%d actual=%d..%d expected_start=%d",
			chunk.FirstSequence, chunk.LastSequence, first, last, a.nextAfter+1)
	}
	page := ReplicaPage{
		LastSequence: a.lastSequence, NextAfter: last,
		Complete: last == a.lastSequence, Events: chunk.Events,
	}
	return a.AddPage(page)
}

func (a *ReplicaSnapshotAccumulator) Manifest() (ReplicaSnapshotManifest, error) {
	if a == nil || a.hash == nil || a.finished || !a.complete || a.nextAfter != a.lastSequence || a.eventCount != a.lastSequence {
		return ReplicaSnapshotManifest{}, errors.New("facts: replica snapshot is incomplete")
	}
	a.finished = true
	return ReplicaSnapshotManifest{
		Version: ReplicaSnapshotVersion, LastSequence: a.lastSequence,
		EventCount: a.eventCount, ChunkCount: a.chunkCount, Digest: hex.EncodeToString(a.hash.Sum(nil)),
	}, nil
}

// Verify checks a sender's final manifest against the locally accumulated
// chunks. Calling it closes the accumulator, whether verification succeeds or
// fails, so a partial or altered snapshot cannot be retried in place.
func (a *ReplicaSnapshotAccumulator) Verify(expected ReplicaSnapshotManifest) error {
	if expected.Version != ReplicaSnapshotVersion || expected.LastSequence < 0 || expected.EventCount < 0 || expected.ChunkCount < 0 {
		return errors.New("facts: invalid replica snapshot manifest")
	}
	if len(expected.Digest) != sha256.Size*2 {
		return errors.New("facts: invalid replica snapshot digest length")
	}
	if _, err := hex.DecodeString(expected.Digest); err != nil {
		return fmt.Errorf("facts: invalid replica snapshot digest: %w", err)
	}
	actual, err := a.Manifest()
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("facts: replica snapshot manifest mismatch: got=%+v expected=%+v", actual, expected)
	}
	return nil
}
