// Package dnssnapshot supplies an in-memory snapshot primitive. It does not
// authenticate releases, persist activation, enforce network ACLs, or drain
// readers that obtained a snapshot before a gate was closed.
package dnssnapshot

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sync"
)

var (
	ErrInvalid      = errors.New("dnssnapshot: invalid snapshot")
	ErrDigest       = errors.New("dnssnapshot: digest mismatch")
	ErrStale        = errors.New("dnssnapshot: older version")
	ErrConflict     = errors.New("dnssnapshot: version identity conflict")
	ErrNotRequested = errors.New("dnssnapshot: snapshot was not requested")
	ErrClosed       = errors.New("dnssnapshot: gate closed or no active snapshot")
)

// Identity binds a content digest to one cluster, release and version. IDs
// are opaque fixed-width values; validity and authorization are external.
type Identity struct {
	Cluster [16]byte
	Release [16]byte
	Version uint64
	Digest  [sha256.Size]byte
}

// Snapshot is immutable after construction; accessors never expose its bytes.
type Snapshot struct {
	identity Identity
	data     []byte
}

// New validates the exact bytes and a caller-supplied positive size limit.
// maxBytes is a resource limit, not a claim that the payload is a valid DNS
// configuration. Callers must not mutate data concurrently with this call.
func New(id Identity, data []byte, maxBytes int) (*Snapshot, error) {
	if id.Version == 0 || maxBytes <= 0 || len(data) == 0 || len(data) > maxBytes {
		return nil, ErrInvalid
	}
	copied := bytes.Clone(data)
	if sha256.Sum256(copied) != id.Digest {
		return nil, ErrDigest
	}
	return &Snapshot{identity: id, data: copied}, nil
}

func (s *Snapshot) Identity() Identity { return s.identity }
func (s *Snapshot) Bytes() []byte      { return bytes.Clone(s.data) }

// Store serializes gate and publication decisions. A Load returns one immutable
// handle; callers must ensure their actual network path is separately closed
// and in-flight responses drained when access is revoked.
type Store struct {
	mu         sync.RWMutex
	cluster    [16]byte
	active     *Snapshot
	pending    *Identity
	highest    *Identity
	open       bool
	generation uint64
	exhausted  bool
}

// NewStore starts closed and is permanently bound to the supplied cluster.
func NewStore(cluster [16]byte) *Store { return &Store{cluster: cluster} }

// Request selects the exact version eligible for subsequent publication.
// A same-identity retry is idempotent; changing a previously selected identity
// at the same version is a conflict. A newer request supersedes an older one.
func (s *Store) Request(id Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id.Cluster != s.cluster || id.Version == 0 {
		return ErrInvalid
	}
	for _, existing := range []*Identity{identityOf(s.active), s.highest} {
		if existing == nil {
			continue
		}
		if id.Version < existing.Version {
			return ErrStale
		}
		if id.Version == existing.Version && id != *existing {
			return ErrConflict
		}
	}
	if s.active != nil && id == s.active.identity {
		return nil
	}
	copyID := id
	s.pending = &copyID
	highest := id
	s.highest = &highest
	return nil
}

func identityOf(s *Snapshot) *Identity {
	if s == nil {
		return nil
	}
	return &s.identity
}

// Publish installs exactly the requested snapshot; it never opens the gate.
// A retry of the currently active identical snapshot is a no-op.
func (s *Store) Publish(candidate *Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if candidate == nil || candidate.identity.Cluster != s.cluster {
		return ErrInvalid
	}
	id := candidate.identity
	if s.active != nil {
		if id.Version < s.active.identity.Version {
			return ErrStale
		}
		if id.Version == s.active.identity.Version {
			if id != s.active.identity {
				return ErrConflict
			}
			return nil
		}
	}
	if s.pending == nil || id != *s.pending {
		return ErrNotRequested
	}
	// The caller may overwrite the exported Snapshot value after Publish.
	// Keep a private value; its data is inaccessible and immutable outside this package.
	private := *candidate
	s.active = &private
	s.pending = nil
	return nil
}

// OpenToken orders local open requests; it is not an authorization credential.
type OpenToken struct {
	store      *Store
	generation uint64
	identity   Identity
}

// RequestOpen issues a new token for the exact active snapshot, invalidating
// earlier tokens. The caller must separately establish current authorization.
func (s *Store) RequestOpen(id Identity) (OpenToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil || s.active.identity != id {
		return OpenToken{}, ErrNotRequested
	}
	if !s.advanceGeneration() {
		return OpenToken{}, ErrClosed
	}
	return OpenToken{store: s, generation: s.generation, identity: id}, nil
}

// Open opens only for a token issued after the most recent Close, and only if
// its active snapshot still matches. Neither operation authenticates a release.
func (s *Store) Open(token OpenToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exhausted || token.store != s || token.generation != s.generation || s.active == nil || s.active.identity != token.identity {
		return ErrNotRequested
	}
	s.open = true
	return nil
}

func (s *Store) advanceGeneration() bool {
	if s.exhausted || s.generation == ^uint64(0) {
		s.exhausted = true
		s.open = false
		return false
	}
	s.generation++
	return true
}

// Close invalidates all tokens and pending publication work. It cannot cancel
// previously returned snapshots or close and drain an actual network path.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = false
	s.pending = nil
	s.advanceGeneration()
}

// Load atomically returns an independent snapshot value when the gate is open.
func (s *Store) Load() (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.open || s.active == nil {
		return Snapshot{}, ErrClosed
	}
	return *s.active, nil
}
