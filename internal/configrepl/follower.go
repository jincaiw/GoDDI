// Package configrepl is the follower-side model of ADR-0013 configuration
// replication.
//
// ADR-0005 bundled two different problems. Replicating the author's
// configuration to the other node (so a promoted standby can actually serve
// scopes and zones) needs only an ordered, gap-aware stream copied one way from
// a single fixed author. Detecting and fencing a second *writer* needs leader
// election, quorum and witness, which is a much heavier and riskier problem.
// ADR-0011 already fixes who may author (one node, by deployment statement), so
// replication here is single-writer and read-only on the follower and cannot
// diverge. This package therefore implements only the first problem; the
// coordinator (ADR-0005) keeps ErrNotImplemented.
//
// The model is deliberately pure: no database, no transport, no HTTP. It is the
// executable contract the migration, relay and applier are later measured
// against, and it can be tested without any of them.
package configrepl

import (
	"errors"
	"fmt"
	"sort"
)

// Revision is one entry of the author's configuration release stream.
//
// Seq is the entry's position in the author's monotonic release log
// (config_release_outbox.id in the real schema). It is the ordering that makes
// "apply seq == applied+1" a meaningful rule.
type Revision struct {
	Seq            int64
	IdempotencyKey string
	ResourceType   string
	ResourceID     string
	// Revision is the per-resource version this entry produces (>=1).
	Revision       int64
	ContentHash    string
	RolledBackFrom int64 // >0 when this entry is a rollback of that resource revision
}

// The errors a follower can return. They are distinct because the caller's
// response differs: a gap means "paused, fetch a snapshot"; a conflict means
// "the stream is corrupt, stop and alert"; a stale entry is normal and ignored.
var (
	// ErrGap means an entry arrived ahead of the next expected position. The
	// follower must NOT skip it: it pauses and asks for a snapshot, because
	// applying seq N+2 without N+1 would silently drop a configuration change.
	ErrGap = errors.New("configrepl: sequence gap; a snapshot is required before continuing")
	// ErrConflict means the same idempotency key arrived with different content.
	// Two different changes cannot share a key; the stream is inconsistent.
	ErrConflict = errors.New("configrepl: idempotency key reused with different content")
	// ErrBadRevision means an entry is structurally invalid.
	ErrBadRevision = errors.New("configrepl: invalid revision")
)

// Follower applies an author's release stream in order.
//
// It is not safe for concurrent use; the real applier drains one stream on one
// goroutine, and keeping the model single-threaded keeps the invariants legible.
type Follower struct {
	applied   int64
	keys      map[string]keyRecord
	resource  map[string]int64 // resourceType/resourceID -> highest revision applied
	resources map[string]struct{}
}

type keyRecord struct {
	seq         int64
	contentHash string
}

// NewFollower starts a follower at appliedSeq. A fresh follower starts at 0; a
// follower restored from a snapshot starts at the snapshot's applied position.
func NewFollower(appliedSeq int64) *Follower {
	if appliedSeq < 0 {
		appliedSeq = 0
	}
	return &Follower{
		applied:   appliedSeq,
		keys:      make(map[string]keyRecord),
		resource:  make(map[string]int64),
		resources: make(map[string]struct{}),
	}
}

// AppliedSeq reports the highest contiguous position applied so far.
func (f *Follower) AppliedSeq() int64 { return f.applied }

// ResourceRevision reports the highest revision applied for a resource.
func (f *Follower) ResourceRevision(resourceType, resourceID string) (int64, bool) {
	r, ok := f.resource[resourceKey(resourceType, resourceID)]
	return r, ok
}

// Known reports whether an idempotency key has already been applied, and the
// position it was applied at. It answers "did this publish take effect?" without
// applying anything -- the follower-side analogue of the author's publish-status
// query for an unknown result.
func (f *Follower) Known(idempotencyKey string) (int64, bool) {
	rec, ok := f.keys[idempotencyKey]
	if !ok {
		return 0, false
	}
	return rec.seq, true
}

// Apply applies one revision.
//
// Return contract:
//   - applied=true  the entry advanced the watermark;
//   - applied=false, err=nil  the entry was a replay (already applied) and was
//     correctly ignored -- NOT a failure;
//   - applied=false, err!=nil  the entry was refused; nothing changed.
//
// The three cases are kept separate because collapsing a replay into an error
// would make a normal retry look like a fault, and the caller must be able to
// tell "nothing to do" from "cannot proceed".
func (f *Follower) Apply(r Revision) (applied bool, err error) {
	if r.Seq <= 0 {
		return false, fmt.Errorf("%w: seq must be positive, got %d", ErrBadRevision, r.Seq)
	}
	if r.ResourceType == "" || r.ResourceID == "" {
		return false, fmt.Errorf("%w: resource type and id are required", ErrBadRevision)
	}
	if r.Revision <= 0 {
		return false, fmt.Errorf("%w: revision must be positive, got %d", ErrBadRevision, r.Revision)
	}

	// Idempotency first: a retried publish must return the original result, not
	// create a second logical revision. Same key + same content is a replay;
	// same key + different content is a corruption the follower refuses to hide.
	if r.IdempotencyKey != "" {
		if rec, ok := f.keys[r.IdempotencyKey]; ok {
			if rec.contentHash != r.ContentHash {
				return false, fmt.Errorf("%w: key %q applied at seq %d with hash %q, now %q",
					ErrConflict, r.IdempotencyKey, rec.seq, rec.contentHash, r.ContentHash)
			}
			return false, nil // pure replay
		}
	}

	// A position already covered by the watermark is a replay of an old entry
	// (e.g. after a snapshot). It must not be applied twice. If it carries a key
	// we have not seen, record it so a later duplicate is still detected.
	if r.Seq <= f.applied {
		if r.IdempotencyKey != "" {
			f.keys[r.IdempotencyKey] = keyRecord{seq: r.Seq, contentHash: r.ContentHash}
		}
		return false, nil
	}

	// The ordering rule that makes the stream safe: only the very next position
	// may be applied. Anything further ahead is a gap, and applying past a gap
	// would silently drop a change. Pause and ask for a snapshot instead.
	if r.Seq != f.applied+1 {
		return false, fmt.Errorf("%w: expected seq %d, got %d", ErrGap, f.applied+1, r.Seq)
	}

	key := resourceKey(r.ResourceType, r.ResourceID)
	if cur, ok := f.resource[key]; ok && r.Revision < cur {
		// A per-resource revision going backwards on the ordered stream is a
		// sign of a corrupt or misordered release, not a normal rollback (a
		// rollback is a *new* higher revision). Refuse rather than regress.
		return false, fmt.Errorf("%w: resource %s revision would regress %d -> %d",
			ErrConflict, key, cur, r.Revision)
	}

	f.resource[key] = r.Revision
	f.resources[key] = struct{}{}
	if r.IdempotencyKey != "" {
		f.keys[r.IdempotencyKey] = keyRecord{seq: r.Seq, contentHash: r.ContentHash}
	}
	f.applied = r.Seq
	return true, nil
}

// Snapshot replaces the follower's state with a baseline taken from the author.
//
// This is what a follower does after a gap: it cannot apply forward, so it takes
// the author's current per-resource revisions at a known position and resumes
// from there. resourceRevisions maps "type/id" -> revision.
func (f *Follower) Snapshot(appliedSeq int64, resourceRevisions map[string]int64) error {
	if appliedSeq < 0 {
		return fmt.Errorf("%w: snapshot seq must be >= 0", ErrBadRevision)
	}
	if appliedSeq < f.applied {
		// A snapshot older than what is already applied would move state
		// backwards. Refuse: the caller asked for the wrong snapshot.
		return fmt.Errorf("%w: snapshot seq %d is behind applied %d", ErrConflict, appliedSeq, f.applied)
	}
	f.applied = appliedSeq
	for k, v := range resourceRevisions {
		f.resource[k] = v
		f.resources[k] = struct{}{}
	}
	return nil
}

// PendingGap reports the next position the follower needs in order to advance.
// A caller that got ErrGap can use it to request a snapshot from exactly there.
func (f *Follower) PendingGap() int64 { return f.applied + 1 }

// AppliedResources returns the resource keys applied so far, sorted, for audits
// and for asserting a failover node actually received what it needs.
func (f *Follower) AppliedResources() []string {
	out := make([]string, 0, len(f.resources))
	for k := range f.resources {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func resourceKey(resourceType, resourceID string) string {
	return resourceType + "/" + resourceID
}
