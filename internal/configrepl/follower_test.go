package configrepl

import (
	"errors"
	"testing"
)

func rev(seq int64, key string, resourceRev int64) Revision {
	return Revision{
		Seq: seq, IdempotencyKey: key, ResourceType: "dhcp_scope", ResourceID: "s1",
		Revision: resourceRev, ContentHash: "hash-" + key,
	}
}

// Invariant 1: retrying a publish with the same idempotency key produces one
// logical revision. The second attempt is a replay (applied=false, no error),
// and the watermark does not move twice.
func TestApplyIdempotentByKey(t *testing.T) {
	f := NewFollower(0)
	applied, err := f.Apply(rev(1, "k1", 1))
	if err != nil || !applied {
		t.Fatalf("first apply: applied=%v err=%v", applied, err)
	}
	applied, err = f.Apply(rev(1, "k1", 1))
	if err != nil {
		t.Fatalf("replay must not error, got %v", err)
	}
	if applied {
		t.Fatalf("replay must not advance the watermark")
	}
	if f.AppliedSeq() != 1 {
		t.Fatalf("watermark = %d, want 1", f.AppliedSeq())
	}
	if seq, ok := f.Known("k1"); !ok || seq != 1 {
		t.Fatalf("Known(k1) = %d,%v want 1,true", seq, ok)
	}
}

// Invariant 2: an entry at or below the watermark is a replay and is ignored,
// never applied twice (this is the "old seq" case a snapshot creates).
func TestApplyStaleSeqIgnored(t *testing.T) {
	f := NewFollower(5) // restored from a snapshot at 5
	applied, err := f.Apply(rev(3, "old", 1))
	if err != nil {
		t.Fatalf("stale apply must not error, got %v", err)
	}
	if applied {
		t.Fatalf("stale entry must not advance the watermark")
	}
	if f.AppliedSeq() != 5 {
		t.Fatalf("watermark = %d, want 5", f.AppliedSeq())
	}
}

// Invariant 3: a gap (an entry ahead of the next expected position) pauses the
// follower -- it returns ErrGap and does NOT skip forward. This is the rule that
// stops a lost configuration change from being silently dropped.
func TestApplyGapPausesAndDoesNotSkip(t *testing.T) {
	f := NewFollower(0)
	if _, err := f.Apply(rev(1, "k1", 1)); err != nil {
		t.Fatal(err)
	}
	applied, err := f.Apply(rev(3, "k3", 3)) // seq 2 is missing
	if !errors.Is(err, ErrGap) {
		t.Fatalf("apply over a gap: err=%v, want ErrGap", err)
	}
	if applied {
		t.Fatalf("a gap must not advance the watermark")
	}
	if f.AppliedSeq() != 1 {
		t.Fatalf("watermark = %d, want 1 (paused at the gap)", f.AppliedSeq())
	}
	if f.PendingGap() != 2 {
		t.Fatalf("PendingGap = %d, want 2", f.PendingGap())
	}
	// After a snapshot at 3, the follower resumes cleanly.
	if err := f.Snapshot(3, map[string]int64{"dhcp_scope/s1": 3}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if f.AppliedSeq() != 3 {
		t.Fatalf("watermark after snapshot = %d, want 3", f.AppliedSeq())
	}
	if r, ok := f.ResourceRevision("dhcp_scope", "s1"); !ok || r != 3 {
		t.Fatalf("resource revision after snapshot = %d,%v want 3,true", r, ok)
	}
}

// Invariant 4: a rollback is a NEW higher revision on the stream, not a
// re-wind. It advances the watermark like any other entry.
func TestRollbackIsNewRevision(t *testing.T) {
	f := NewFollower(0)
	if _, err := f.Apply(rev(1, "k1", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Apply(rev(2, "k2", 2)); err != nil {
		t.Fatal(err)
	}
	rollback := Revision{
		Seq: 3, IdempotencyKey: "k3", ResourceType: "dhcp_scope", ResourceID: "s1",
		Revision: 3, ContentHash: "hash-k3", RolledBackFrom: 2,
	}
	applied, err := f.Apply(rollback)
	if err != nil || !applied {
		t.Fatalf("rollback apply: applied=%v err=%v", applied, err)
	}
	if f.AppliedSeq() != 3 {
		t.Fatalf("watermark = %d, want 3", f.AppliedSeq())
	}
	if r, _ := f.ResourceRevision("dhcp_scope", "s1"); r != 3 {
		t.Fatalf("resource revision = %d, want 3 (rollback is a new revision)", r)
	}
}

// A per-resource revision that goes backwards on the ordered stream is refused:
// only a rollback expressed as a *higher* revision is legal.
func TestPerResourceRevisionRegressionRefused(t *testing.T) {
	f := NewFollower(0)
	if _, err := f.Apply(rev(1, "k1", 2)); err != nil {
		t.Fatal(err)
	}
	regress := Revision{Seq: 2, IdempotencyKey: "k2", ResourceType: "dhcp_scope", ResourceID: "s1", Revision: 1, ContentHash: "h"}
	if _, err := f.Apply(regress); !errors.Is(err, ErrConflict) {
		t.Fatalf("regression err=%v, want ErrConflict", err)
	}
}

// The same idempotency key with different content is a corruption the follower
// refuses to hide.
func TestSameKeyDifferentContentConflict(t *testing.T) {
	f := NewFollower(0)
	if _, err := f.Apply(rev(1, "k1", 1)); err != nil {
		t.Fatal(err)
	}
	other := rev(2, "k1", 2)
	other.ContentHash = "different"
	if _, err := f.Apply(other); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting key err=%v, want ErrConflict", err)
	}
}

// Invariant 5: "did this publish take effect?" is answerable without applying
// anything -- the follower-side query for an unknown/uncertain result.
func TestKnownReportsAppliedPosition(t *testing.T) {
	f := NewFollower(0)
	if _, ok := f.Known("nope"); ok {
		t.Fatalf("Known on unknown key must be false")
	}
	if _, err := f.Apply(rev(1, "yes", 1)); err != nil {
		t.Fatal(err)
	}
	if seq, ok := f.Known("yes"); !ok || seq != 1 {
		t.Fatalf("Known(yes)=%d,%v want 1,true", seq, ok)
	}
}

// A snapshot older than the applied position would move state backwards and is
// refused.
func TestSnapshotBehindAppliedRefused(t *testing.T) {
	f := NewFollower(0)
	if _, err := f.Apply(rev(1, "k1", 1)); err != nil {
		t.Fatal(err)
	}
	if err := f.Snapshot(0, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale snapshot err=%v, want ErrConflict", err)
	}
}

func TestInvalidRevisionRefused(t *testing.T) {
	f := NewFollower(0)
	cases := []Revision{
		{Seq: 0, ResourceType: "a", ResourceID: "b", Revision: 1},
		{Seq: 1, ResourceType: "", ResourceID: "b", Revision: 1},
		{Seq: 1, ResourceType: "a", ResourceID: "", Revision: 1},
		{Seq: 1, ResourceType: "a", ResourceID: "b", Revision: 0},
	}
	for i, c := range cases {
		if _, err := f.Apply(c); !errors.Is(err, ErrBadRevision) {
			t.Fatalf("case %d: err=%v, want ErrBadRevision", i, err)
		}
	}
}

// A failover scenario in miniature: the follower must end up holding every
// resource the author published, in order, so a promoted node has what it needs
// to serve. This is the property that makes a standby able to answer a DHCP
// scope after it becomes primary.
func TestFollowerCarriesAllResourcesForFailover(t *testing.T) {
	f := NewFollower(0)
	stream := []Revision{
		{Seq: 1, IdempotencyKey: "z1", ResourceType: "dns_zone", ResourceID: "example.test.", Revision: 1, ContentHash: "a"},
		{Seq: 2, IdempotencyKey: "s1", ResourceType: "dhcp_scope", ResourceID: "scope-1", Revision: 1, ContentHash: "b"},
		{Seq: 3, IdempotencyKey: "s2", ResourceType: "dhcp_scope", ResourceID: "scope-1", Revision: 2, ContentHash: "c"},
	}
	for _, r := range stream {
		if applied, err := f.Apply(r); err != nil || !applied {
			t.Fatalf("apply seq %d: applied=%v err=%v", r.Seq, applied, err)
		}
	}
	if got := f.AppliedResources(); len(got) != 2 {
		t.Fatalf("applied resources = %v, want 2 distinct", got)
	}
	if r, ok := f.ResourceRevision("dhcp_scope", "scope-1"); !ok || r != 2 {
		t.Fatalf("scope-1 revision = %d,%v want 2,true (failover node must hold it)", r, ok)
	}
}
