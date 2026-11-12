// Package snapshotverify checks bounded snapshot transport integrity in memory.
// It does not authenticate peers, parse rows, install data, or authorize roles.
package snapshotverify

import (
	"crypto/sha256"
	"errors"
	"math"
	"sync"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("snapshotverify: invalid manifest or chunk")
var ErrDirty = errors.New("snapshotverify: verifier permanently failed")

const MaxChunkBytes = 1 << 20
const MaxConfiguredChunks = 65536
const MaxConfiguredTables = 256
const MaxConfiguredBytes = uint64(1) << 40
const MaxConfiguredRows = uint64(1_000_000_000)
const EncodingNDJSONV1 = 1

type Position struct {
	Epoch, Seq uint64
	Hash       [32]byte
}
type ChunkSpec struct {
	Index  uint32
	Size   uint64
	SHA256 [32]byte
}
type TableCount struct {
	Name string
	Rows uint64
}
type Manifest struct {
	Cluster, Leader, Snapshot [16]byte
	Schema                    uint32
	Anchor                    Position
	Encoding                  uint32
	RetentionUntilMS          int64
	TotalBytes                uint64
	Chunks                    []ChunkSpec
	Tables                    []TableCount
}

// Trust is supplied only after external peer authentication and snapshot
// request/anchor binding. The boolean is a caller assertion, not authentication.
type Trust struct {
	Authenticated             bool
	Cluster, Leader, Snapshot [16]byte
	Anchor                    Position
}
type Limits struct {
	MaxBytes  uint64
	MaxChunks uint32
	MaxRows   uint64
	Tables    map[string]bool
	Schema    uint32
}

// TransportResult deliberately does not state that rows, domain invariants,
// database consistency, authority, or activation readiness were verified.
type TransportResult struct {
	Bytes        uint64
	Chunks       uint32
	DeclaredRows uint64
	RowsVerified bool
}

type Verifier struct {
	mu                    sync.Mutex
	specs                 []ChunkSpec
	next                  int
	total, expected, rows uint64
	expiry, lastUpper     int64
	dirty, complete       bool
}

func validID(id [16]byte) bool { return uuid.UUID(id) != uuid.Nil }

// New binds every manifest identity and anchor to values from the caller's
// independent authenticated context. upperMS is a conservative current-time
// upper bound; the caller must refresh it on every Feed and VerifyComplete.
// An empty snapshot has no chunks, zero bytes and zero declared rows. Nonempty
// snapshots require nonempty chunks and at least one declared row.
func New(m Manifest, trust Trust, limits Limits, upperMS int64) (*Verifier, error) {
	if !trust.Authenticated || !validID(trust.Cluster) || !validID(trust.Leader) || trust.Cluster == trust.Leader || !validID(trust.Snapshot) || m.Cluster != trust.Cluster || m.Leader != trust.Leader || m.Snapshot != trust.Snapshot || m.Anchor != trust.Anchor {
		return nil, ErrInvalid
	}
	if m.Anchor.Epoch == 0 || m.Anchor.Epoch > math.MaxInt64 || m.Anchor.Seq > math.MaxInt64 || (m.Anchor.Epoch == 1 && m.Anchor.Seq == 0 && m.Anchor.Hash != ([32]byte{})) {
		return nil, ErrInvalid
	}
	if upperMS <= 0 || m.RetentionUntilMS <= upperMS || limits.Schema == 0 || m.Schema != limits.Schema || m.Encoding != EncodingNDJSONV1 || limits.MaxBytes == 0 || limits.MaxBytes > MaxConfiguredBytes || limits.MaxChunks == 0 || limits.MaxChunks > MaxConfiguredChunks || limits.MaxRows == 0 || limits.MaxRows > MaxConfiguredRows || len(limits.Tables) == 0 || len(limits.Tables) > MaxConfiguredTables {
		return nil, ErrInvalid
	}
	if uint64(len(m.Chunks)) > uint64(limits.MaxChunks) || m.TotalBytes > limits.MaxBytes || (len(m.Chunks) == 0) != (m.TotalBytes == 0) {
		return nil, ErrInvalid
	}
	var total uint64
	for i, c := range m.Chunks {
		if uint64(i) > math.MaxUint32 || c.Index != uint32(i) || c.Size == 0 || c.Size > MaxChunkBytes || c.Size > limits.MaxBytes-total {
			return nil, ErrInvalid
		}
		total += c.Size
	}
	if total != m.TotalBytes {
		return nil, ErrInvalid
	}
	var rows uint64
	if len(m.Tables) == 0 || len(m.Tables) > len(limits.Tables) {
		return nil, ErrInvalid
	}
	seen := make(map[string]struct{}, len(m.Tables))
	for _, table := range m.Tables {
		if !limits.Tables[table.Name] {
			return nil, ErrInvalid
		}
		if _, ok := seen[table.Name]; ok {
			return nil, ErrInvalid
		}
		seen[table.Name] = struct{}{}
		if table.Rows > limits.MaxRows-rows {
			return nil, ErrInvalid
		}
		rows += table.Rows
	}
	if (total == 0) != (rows == 0) {
		return nil, ErrInvalid
	}
	return &Verifier{specs: append([]ChunkSpec(nil), m.Chunks...), expected: total, rows: rows, expiry: m.RetentionUntilMS, lastUpper: upperMS}, nil
}
func (v *Verifier) checkTime(upperMS int64) error {
	if upperMS <= 0 || upperMS < v.lastUpper || upperMS >= v.expiry {
		return ErrInvalid
	}
	v.lastUpper = upperMS
	return nil
}

// Feed returns an independently owned copy of the verified bytes and retains
// no chunk body. Callers must not modify raw concurrently during this call.
// Every error permanently invalidates the verifier, including after completion.
func (v *Verifier) Feed(index uint32, raw []byte, upperMS int64) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dirty {
		return nil, ErrDirty
	}
	if v.complete || v.checkTime(upperMS) != nil || v.next >= len(v.specs) || uint64(index) != uint64(v.next) {
		v.dirty = true
		return nil, ErrInvalid
	}
	spec := v.specs[v.next]
	if uint64(len(raw)) != spec.Size || spec.Size > v.expected-v.total {
		v.dirty = true
		return nil, ErrInvalid
	}
	copyBytes := append([]byte(nil), raw...)
	if sha256.Sum256(copyBytes) != spec.SHA256 {
		v.dirty = true
		return nil, ErrInvalid
	}
	v.total += spec.Size
	v.next++
	return copyBytes, nil
}

// VerifyComplete proves only manifest/chunk transport consistency. It never
// claims NDJSON parse, table row counts or domain verification succeeded.
func (v *Verifier) VerifyComplete(upperMS int64) (TransportResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dirty {
		return TransportResult{}, ErrDirty
	}
	if v.complete || v.checkTime(upperMS) != nil || v.next != len(v.specs) || v.total != v.expected {
		v.dirty = true
		return TransportResult{}, ErrInvalid
	}
	v.complete = true
	return TransportResult{Bytes: v.total, Chunks: uint32(v.next), DeclaredRows: v.rows, RowsVerified: false}, nil
}
