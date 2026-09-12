package ha

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
)

// These tests cover the half of the contract that a machine cannot decide: the
// role this node is in, and the permission it runs on.
//
// Every case here fails if the mechanism it names is removed. That is the point
// of the file: a state an operator sets is easy to implement in a way that
// looks right and does nothing -- a marker written where nobody reads it, a
// gate that can be walked around with a flag, a promotion whose sequence
// restarts below what the peer already used.

// auditActions reads this store's audit trail in the order it was written.
//
// The rows are ordered by rowid rather than by the timestamp: the timestamp is
// whole seconds, and the entries this file produces land in the same second.
func auditActions(t *testing.T, store *dataplane.Store) []string {
	t.Helper()
	rows, err := store.Query(`SELECT action FROM audit_logs ORDER BY rowid`)
	if err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatalf("scanning an audit entry: %v", err)
		}
		out = append(out, action)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	return out
}

func hasAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

// TestEveryOperatorActionRequiresItsConfirmation is the rule that keeps these
// decisions in human hands.
//
// The requirement lives in the mechanism rather than in the flag parser, so a
// second caller cannot forget it -- and so that this test can reach it.
func TestEveryOperatorActionRequiresItsConfirmation(t *testing.T) {
	t.Run("degrade", func(t *testing.T) {
		store := openStore(t, "primary")
		cfg := testConfig(t, "primary")
		op := NewOperator(cfg, store)
		err := op.Degrade(DegradeOptions{Reason: "the operator did not confirm"})
		if !errors.Is(err, ErrNoConfirmation) {
			t.Fatalf("Degrade = %v, want ErrNoConfirmation", err)
		}
		if marker, _ := store.Meta(metaDegraded); marker != "" {
			t.Errorf("an unconfirmed degrade wrote the marker %q", marker)
		}
		if len(auditActions(t, store)) != 0 {
			t.Error("an unconfirmed degrade wrote an audit entry")
		}
	})

	t.Run("takeover", func(t *testing.T) {
		store := openStore(t, "standby")
		cfg := testConfig(t, "standby")
		op := NewOperator(cfg, store)
		if _, err := op.Takeover(TakeoverOptions{OldPrimaryCannotWrite: true}); !errors.Is(err, ErrNoConfirmation) {
			t.Fatalf("Takeover = %v, want ErrNoConfirmation", err)
		}
		if role, _ := EffectiveRole(cfg, store); role != config.HARoleStandby {
			t.Errorf("an unconfirmed takeover changed the role to %q", role)
		}
	})

	t.Run("takeover-without-the-fence-statement", func(t *testing.T) {
		store := openStore(t, "standby")
		cfg := testConfig(t, "standby")
		op := NewOperator(cfg, store)
		// The fence statement is checked before the watermarks, because it is
		// the one input no reading of the store can substitute for.
		if _, err := op.Takeover(TakeoverOptions{Confirmed: true}); !errors.Is(err, ErrUnfencedPrimary) {
			t.Fatalf("Takeover = %v, want ErrUnfencedPrimary", err)
		}
	})

	t.Run("fence", func(t *testing.T) {
		store := openStore(t, "primary")
		cfg := testConfig(t, "primary")
		op := NewOperator(cfg, store)
		if err := op.Fence(FenceOptions{}); !errors.Is(err, ErrNoConfirmation) {
			t.Fatalf("Fence = %v, want ErrNoConfirmation", err)
		}
		if role, _ := EffectiveRole(cfg, store); role != config.HARolePrimary {
			t.Errorf("an unconfirmed fence changed the role to %q", role)
		}
	})

	t.Run("rejoin", func(t *testing.T) {
		store := openStore(t, "primary")
		cfg := testConfig(t, "primary")
		op := NewOperator(cfg, store)
		if err := op.Fence(FenceOptions{Confirmed: true}); err != nil {
			t.Fatalf("fencing: %v", err)
		}
		if err := op.Rejoin(RejoinOptions{}); !errors.Is(err, ErrNoConfirmation) {
			t.Fatalf("Rejoin = %v, want ErrNoConfirmation", err)
		}
		if role, _ := EffectiveRole(cfg, store); role != RoleFenced {
			t.Errorf("an unconfirmed rejoin changed the role to %q", role)
		}
	})
}

// TestARunningNodeAdoptsAnOperatorsPermission is the reason the marker is
// re-read rather than loaded once.
//
// The situation `goddi ha degrade` exists for is the one in which the node must
// keep serving: the peer is gone and clients are trying to renew. A permission
// that only took effect at the next restart would be a permission to restart,
// which is the outage it was meant to avoid.
func TestARunningNodeAdoptsAnOperatorsPermission(t *testing.T) {
	store := openStore(t, "primary")
	cfg := testConfig(t, "primary")
	repl, err := NewReplicator(cfg, store)
	if err != nil {
		t.Fatalf("building the primary: %v", err)
	}
	if repl.MayBind() {
		t.Fatal("a primary with no mirror promised an address before it was degraded")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = repl.Run(ctx) }()
	waitFor(t, "the primary to bind its peer port", func() bool { return repl.Addr() != nil })

	op := NewOperator(cfg, store)
	if err := op.Degrade(DegradeOptions{Confirmed: true, Reason: "drill: replacing the standby host"}); err != nil {
		t.Fatalf("degrading: %v", err)
	}
	waitFor(t, "the running node to adopt the permission", func() bool { return repl.MayBind() })
	if got := repl.State(); got != StatePrimaryDegraded {
		t.Errorf("state = %s, want %s", got, StatePrimaryDegraded)
	}

	if err := op.Degrade(DegradeOptions{Confirmed: true, Undo: true}); err != nil {
		t.Fatalf("withdrawing the permission: %v", err)
	}
	waitFor(t, "the running node to withdraw it again", func() bool { return !repl.MayBind() })
	if got := repl.State(); got != StatePaused {
		t.Errorf("state after the withdrawal = %s, want %s", got, StatePaused)
	}
}

// TestADegradedNodeMakesThePromiseItself is what makes the permission mean
// something to a client.
//
// A degraded node has no mirror to wait for. If Confirm waited anyway, every
// request would end in a withheld acknowledgement after the confirmation
// timeout, and the operator's decision would be invisible in exactly the way
// that matters -- the client would still lose its address.
func TestADegradedNodeMakesThePromiseItself(t *testing.T) {
	store := openStore(t, "primary")
	cfg := testConfig(t, "primary")
	repl, err := NewReplicator(cfg, store)
	if err != nil {
		t.Fatalf("building the primary: %v", err)
	}

	row := LeaseRow{ID: "l1", ScopeID: "scope-1", IPAddress: "10.0.0.5", Status: "active"}
	if err := repl.Confirm(context.Background(), row); !errors.Is(err, ErrNoSecondCopy) {
		t.Fatalf("Confirm with no mirror and no permission = %v, want ErrNoSecondCopy", err)
	}

	if err := NewOperator(cfg, store).Degrade(DegradeOptions{Confirmed: true}); err != nil {
		t.Fatalf("degrading: %v", err)
	}
	// Rebuilt so that the permission is read at construction, which is the path
	// a restarted service takes.
	degraded, err := NewReplicator(cfg, store)
	if err != nil {
		t.Fatalf("rebuilding the primary: %v", err)
	}

	start := time.Now()
	if err := degraded.Confirm(context.Background(), row); err != nil {
		t.Fatalf("Confirm on a degraded node = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > testConfirmTimeout/2 {
		t.Errorf("Confirm took %s; a degraded node has nothing to wait for", elapsed)
	}
	if degraded.Seq() != 1 {
		t.Errorf("seq = %d, want 1: the change must still be recorded for the mirror", degraded.Seq())
	}
}

// TestLosingTheMirrorNeverDegradesANodeItself is the negative half of INV-6.
//
// A system that can decide to carry on alone will, eventually, decide it by
// accident. This case states the rule as an observation: the mirror dies, the
// node goes quiet, and no permission appears.
func TestLosingTheMirrorNeverDegradesANodeItself(t *testing.T) {
	primary, primaryStore, _ := startPair(t)
	if primary.State() != StatePrimary {
		t.Fatalf("the pair did not come up redundant: state = %s", primary.State())
	}

	// Losing the mirror. The pair's own fail-closed transition is covered by
	// the W08 tests; what this one adds is that the node does not reach for the
	// permission on its way down.
	primary.link.markDown()
	waitFor(t, "the primary to stop promising", func() bool { return !primary.MayBind() })

	if primary.Degraded() {
		t.Fatal("a node whose mirror went away recorded a permission it did not ask for")
	}
	marker, err := primaryStore.Meta(metaDegraded)
	if err != nil {
		t.Fatalf("reading the marker: %v", err)
	}
	if marker != "" {
		t.Fatalf("the store holds a single-copy permission nobody granted: %q", marker)
	}
}

// TestATakeoverIsRefusedWhileThePrimaryStillAnswers is the split-brain gate.
//
// A standby cannot tell a dead primary from a partitioned one, so it cannot
// refuse a takeover on those grounds. What it can see is a primary that is
// demonstrably alive -- and taking over from one of those is two writers.
func TestATakeoverIsRefusedWhileThePrimaryStillAnswers(t *testing.T) {
	primary, _, mirrorStore := startPair(t)

	cfg := testConfig(t, "standby")
	op := NewOperator(cfg, mirrorStore)
	out, err := op.Takeover(TakeoverOptions{Confirmed: true, OldPrimaryCannotWrite: true})
	if !errors.Is(err, ErrPeerStillAnswering) {
		t.Fatalf("Takeover against a live primary = %v, want ErrPeerStillAnswering", err)
	}
	if out.PeerSeqAt.IsZero() {
		t.Error("the refusal did not report when the primary was last heard from")
	}
	// That moment has to be a moment, not a date rounded to the second: it is
	// compared against a window measured in seconds, and a value truncated to
	// whole seconds is up to a second of error in the one calculation that
	// decides whether a live primary can be taken over from.
	if heard := time.Since(out.PeerSeqAt); heard > 100*time.Millisecond {
		t.Errorf("the primary was heard from %s ago but this pair formed milliseconds ago; "+
			"the moment is being recorded with less precision than the gate needs", heard)
	}
	if role, _ := EffectiveRole(cfg, mirrorStore); role != config.HARoleStandby {
		t.Errorf("the refused takeover changed the role to %q", role)
	}
	if marker, _ := mirrorStore.Meta(metaRole); marker != "" {
		t.Errorf("the refused takeover recorded the role %q", marker)
	}
	if !primary.MayBind() {
		t.Error("the primary was disturbed by a takeover that was refused")
	}
}

// TestAShortfallMustBeNamedToBeAccepted covers the watermark verification.
//
// The numbers are not a correctness check -- a binding is acknowledged only
// once the standby has applied it, so a change this node never applied was
// never acknowledged. They are the measure of how far the two nodes had drifted
// when contact was lost, which is what an operator has to read before deciding
// that the other node is gone rather than merely unreachable.
func TestAShortfallMustBeNamedToBeAccepted(t *testing.T) {
	store := openStore(t, "standby")
	cfg := testConfig(t, "standby")
	// A primary that reached 10 while this node holds 7, and that stopped being
	// heard from long enough ago to be gone.
	setMetaText(store.DB, metaAppliedSeq, "7")
	setMetaText(store.DB, metaPeerSeq, "10")
	setMetaText(store.DB, metaPeerSeqAt, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano))

	op := NewOperator(cfg, store)
	out, err := op.Takeover(TakeoverOptions{Confirmed: true, OldPrimaryCannotWrite: true})
	if !errors.Is(err, ErrUnexplainedGap) {
		t.Fatalf("Takeover with an unaccepted shortfall = %v, want ErrUnexplainedGap", err)
	}
	if out.Gap != 3 || out.AppliedSeq != 7 || out.PeerSeq != 10 {
		t.Fatalf("the refusal reported held=%d peer=%d gap=%d, want 7/10/3",
			out.AppliedSeq, out.PeerSeq, out.Gap)
	}
	if !strings.Contains(err.Error(), "--accept-gap 3") {
		t.Errorf("the refusal does not say what to pass to accept it: %v", err)
	}

	// A figure that does not match what is there is not an acceptance; it is a
	// sign the operator is reading a stale report.
	if _, err := op.Takeover(TakeoverOptions{Confirmed: true, OldPrimaryCannotWrite: true, AcceptGap: 2}); !errors.Is(err, ErrUnexplainedGap) {
		t.Fatalf("Takeover with the wrong figure = %v, want ErrUnexplainedGap", err)
	}

	out, err = op.Takeover(TakeoverOptions{Confirmed: true, OldPrimaryCannotWrite: true, AcceptGap: 3})
	if err != nil {
		t.Fatalf("Takeover with the shortfall accepted: %v", err)
	}

	// The sequence resumes above everything either node has used. Below it, the
	// re-joined peer would be handed numbers it had already used.
	if out.Seq != 10 {
		t.Errorf("resumed at seq %d, want 10: the counter must clear the peer's own", out.Seq)
	}
	if role, _ := EffectiveRole(cfg, store); role != config.HARolePrimary {
		t.Errorf("role in force = %q, want %q", role, config.HARolePrimary)
	}
	if seq, _ := readWatermark(store.DB, metaSeq); seq != 10 {
		t.Errorf("the persisted counter is %d, want 10", seq)
	}
	if got, _ := store.Meta(metaDegraded); got == "" {
		t.Error("the promotion did not grant the permission to serve alone, so the new primary would withhold everything")
	}

	// The two watermarks that describe a mirror this node does not have are
	// gone. A leftover acknowledged figure above the resumed sequence would
	// satisfy every confirmation wait on the spot.
	for _, key := range []string{metaAppliedSeq, metaAckedSeq} {
		if v, _ := readWatermark(store.DB, key); v != 0 {
			t.Errorf("%s = %d after the promotion, want it cleared", key, v)
		}
	}

	// And the promoted node serves: it comes up degraded, reports the resumed
	// counter, and can acknowledge without a mirror.
	//
	// The role has to be read back and substituted, because the settings still
	// say standby -- the promotion changed what is recorded, not the file. A
	// constructor that only looked at the file would refuse to start the node
	// an operator had just promoted, which is the whole feature failing.
	role, err := EffectiveRole(cfg, store)
	if err != nil {
		t.Fatalf("reading the role in force: %v", err)
	}
	if _, err := NewReplicator(cfg, store); err == nil {
		t.Error("a replicator was built from the configured role, which still says standby")
	}
	repl, err := NewReplicator(cfg.ForRole(role), store)
	if err != nil {
		t.Fatalf("building the promoted primary: %v", err)
	}
	if got := repl.State(); got != StatePrimaryDegraded {
		t.Fatalf("state = %s, want %s", got, StatePrimaryDegraded)
	}
	if repl.Seq() != 10 {
		t.Errorf("seq = %d, want 10: the counter did not survive the promotion", repl.Seq())
	}
	repl.Replicate(LeaseRow{ID: "l2", ScopeID: "scope-1", IPAddress: "10.0.0.6", Status: "offered"})
	if repl.Seq() != 11 {
		t.Errorf("seq = %d, want 11: the next number handed out follows the peer's", repl.Seq())
	}
	if err := repl.Confirm(context.Background(), LeaseRow{ID: "l3", ScopeID: "scope-1", IPAddress: "10.0.0.7", Status: "active"}); err != nil {
		t.Errorf("the promoted primary could not acknowledge: %v", err)
	}

	if !hasAction(auditActions(t, store), "dhcp_ha_takeover") {
		t.Error("the promotion left no audit entry")
	}

	// What the promotion gave up is still readable afterwards, and the
	// distance between the two watermarks no longer claims to be it.
	//
	// The promotion cleared the applied watermark -- it has to -- so the only
	// subtraction a report can still do is peer_seq minus zero, which is the
	// peer's whole counter presented as a shortfall that never shrinks. The
	// figure the operator named is the one worth keeping, and it is kept.
	st, err := op.Status()
	if err != nil {
		t.Fatalf("reading the status after the promotion: %v", err)
	}
	if st.Gap != 0 {
		t.Errorf("a promoted primary reports a shortfall of %d: the taken-over node's counter is not this node's missing set", st.Gap)
	}
	if st.AcceptedGap != 3 {
		t.Errorf("accepted gap = %d, want 3: what the promotion gave up was not recorded", st.AcceptedGap)
	}
}

// TestAShortfallIsOnlyReportedWhereItMeansSomething pins the role the figure
// is computed on.
//
// The subtraction is "the peer's counter minus what this node applied", and it
// describes a shortfall only where the peer's counter is a figure this node was
// catching up to. A store can hold a peer counter for other reasons -- a
// promotion leaves the taken-over node's figure behind, and the applied
// watermark the promotion clears is exactly the one that would have made the
// subtraction an answer -- and turning that into a shortfall reports
// everything the other node had ever handed out as missing, permanently, on a
// node that is serving clients.
func TestAShortfallIsOnlyReportedWhereItMeansSomething(t *testing.T) {
	store := openStore(t, "promoted")
	setMetaText(store.DB, metaPeerSeq, "10")
	op := NewOperator(testConfig(t, "primary"), store)

	st, err := op.Status()
	if err != nil {
		t.Fatalf("reading the status: %v", err)
	}
	if st.Role != config.HARolePrimary {
		t.Fatalf("role = %q, want %q", st.Role, config.HARolePrimary)
	}
	if st.Gap != 0 {
		t.Errorf("a primary reports a shortfall of %d against a peer counter of %d: a node that mirrors nobody is missing nothing",
			st.Gap, st.PeerSeq)
	}

	// The same store, holding the same peer counter, is a mirror that is behind
	// -- and then the figure is the answer. Without this half the gate above
	// would be indistinguishable from never reporting it at all.
	setMetaText(store.DB, metaAppliedSeq, "7")
	setMetaText(store.DB, metaRole, config.HARoleStandby)
	st, err = op.Status()
	if err != nil {
		t.Fatalf("reading the status as a standby: %v", err)
	}
	if st.Role != config.HARoleStandby || st.Gap != 3 {
		t.Errorf("role = %q, shortfall = %d, want %q/3", st.Role, st.Gap, config.HARoleStandby)
	}
	if st.AcceptedGap != 0 {
		t.Errorf("accepted gap = %d on a node that was never promoted, want 0", st.AcceptedGap)
	}
}

// TestATakeoverWaitsOutTheQuietWindow pins the width of the window rather than
// only that it exists.
//
// A gate that is accidentally two staleness bounds wide is a gate that lets a
// live primary be taken over from; one that is accidentally a day wide is a
// gate that can never be passed. Neither would fail the test above.
func TestATakeoverWaitsOutTheQuietWindow(t *testing.T) {
	cfg := testConfig(t, "standby")
	window := takeoverQuietMultiple * testPeerStaleAfter

	for _, tc := range []struct {
		name    string
		heard   time.Duration
		refused bool
	}{
		{"inside the window", window / 2, true},
		{"outside the window", window * 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := openStore(t, "standby")
			setMetaText(store.DB, metaAppliedSeq, "4")
			setMetaText(store.DB, metaPeerSeq, "4")
			// Written with the precision the mechanism itself uses. A
			// whole-second timestamp here would put up to a second of error
			// into the very comparison under test, and the case would pass or
			// fail depending on where in the second it ran.
			setMetaText(store.DB, metaPeerSeqAt, time.Now().Add(-tc.heard).UTC().Format(time.RFC3339Nano))

			op := NewOperator(cfg, store)
			_, err := op.Takeover(TakeoverOptions{Confirmed: true, OldPrimaryCannotWrite: true})
			if tc.refused && !errors.Is(err, ErrPeerStillAnswering) {
				t.Fatalf("Takeover = %v, want ErrPeerStillAnswering", err)
			}
			if !tc.refused && err != nil {
				t.Fatalf("Takeover = %v, want it to be allowed", err)
			}
		})
	}
}

// TestAFencedNodeComesBackOnlyAsAMirror covers the other end of a takeover.
//
// The node that was primary is the only one that can stop itself, so fencing it
// is an operator action on that node. It comes back as a mirror and never as a
// primary: it has been out of the pair while somebody else was serving, so all
// it can be trusted to hold is a copy.
func TestAFencedNodeComesBackOnlyAsAMirror(t *testing.T) {
	store := openStore(t, "primary")
	cfg := testConfig(t, "primary")

	// A node that was an author has watermarks from that life.
	setMetaText(store.DB, metaSeq, "42")
	setMetaText(store.DB, metaAckedSeq, "40")
	op := NewOperator(cfg, store)

	if err := op.Rejoin(RejoinOptions{Confirmed: true}); !errors.Is(err, ErrNotFenced) {
		t.Fatalf("Rejoin on a node that is not fenced = %v, want ErrNotFenced", err)
	}

	if err := op.Fence(FenceOptions{Confirmed: true, Reason: "the other node is taking over"}); err != nil {
		t.Fatalf("fencing: %v", err)
	}
	if role, _ := EffectiveRole(cfg, store); role != RoleFenced {
		t.Fatalf("role in force = %q, want %q", role, RoleFenced)
	}
	status, err := op.Status()
	if err != nil {
		t.Fatalf("reading the status: %v", err)
	}
	if status.State != StateFenced || status.FencedAt.IsZero() {
		t.Errorf("status = %+v, want the fenced state with the moment it happened", status)
	}
	// A snapshot cannot see the peer link, so it must not claim to. The two
	// states it can settle are the two an operator put there, and this is one
	// of them.
	if !status.RedundancyKnown || status.Redundant {
		t.Errorf("a fenced node reported redundant=%v known=%v, want false/true",
			status.Redundant, status.RedundancyKnown)
	}
	if err := op.Fence(FenceOptions{Confirmed: true}); !errors.Is(err, ErrWrongRole) {
		t.Errorf("fencing a fenced node = %v, want ErrWrongRole", err)
	}

	if err := op.Rejoin(RejoinOptions{Confirmed: true}); err != nil {
		t.Fatalf("rejoining: %v", err)
	}
	if role, _ := EffectiveRole(cfg, store); role != config.HARoleStandby {
		t.Fatalf("role in force after the rejoin = %q, want %q", role, config.HARoleStandby)
	}

	// The counter survives, because a later promotion of this node still has to
	// clear every number either node has used. The two mirror watermarks do
	// not: they describe content and a peer this node is about to be given.
	if seq, _ := readWatermark(store.DB, metaSeq); seq != 42 {
		t.Errorf("the counter is %d after the rejoin, want it kept at 42", seq)
	}
	if v, _ := store.Meta(metaFencedAt); v != "" {
		t.Errorf("the fence marker survived the rejoin: %q", v)
	}
	for _, key := range []string{metaAppliedSeq, metaAckedSeq} {
		if v, _ := readWatermark(store.DB, key); v != 0 {
			t.Errorf("%s = %d after the rejoin, want it cleared", key, v)
		}
	}

	actions := auditActions(t, store)
	for _, want := range []string{"dhcp_ha_fence", "dhcp_ha_rejoin"} {
		if !hasAction(actions, want) {
			t.Errorf("the trail has no %s entry: %v", want, actions)
		}
	}
}

// TestTheRecordedRoleOutranksTheConfiguration covers the ordering the whole
// recorded-role mechanism rests on.
//
// The configuration is an intention written before anything happened. A
// promotion and a fence are facts about what happened afterwards, and letting
// the file win would bring the node back in the role an operator changed it
// away from -- which is the one outcome the recording exists to prevent.
func TestTheRecordedRoleOutranksTheConfiguration(t *testing.T) {
	store := openStore(t, "standby")
	cfg := testConfig(t, "standby")

	if role, err := EffectiveRole(cfg, store); err != nil || role != config.HARoleStandby {
		t.Fatalf("EffectiveRole = (%q, %v), want the configured role while nothing is recorded", role, err)
	}

	if err := setMetaText(store.DB, metaRole, config.HARolePrimary); err != nil {
		t.Fatalf("recording a role: %v", err)
	}
	if role, err := EffectiveRole(cfg, store); err != nil || role != config.HARolePrimary {
		t.Fatalf("EffectiveRole = (%q, %v), want the recorded role", role, err)
	}

	// A role this build does not know is refused rather than ignored. Falling
	// back to the configuration would be the same failure as letting the file
	// win, with the added surprise of a node that says nothing about it.
	if err := setMetaText(store.DB, metaRole, "understudy"); err != nil {
		t.Fatalf("recording an unknown role: %v", err)
	}
	if role, err := EffectiveRole(cfg, store); err == nil {
		t.Fatalf("EffectiveRole = %q with no error, want a refusal", role)
	}
}

// TestAStandbyHasNoPromiseToPermit keeps the permission from being used as a
// way to make a mirror serve.
//
// A standby makes no promises of its own, so there is none to permit. An
// operator who degraded one would reasonably read the result as "this node may
// now serve", which is the opposite of what a standby is for.
func TestAStandbyHasNoPromiseToPermit(t *testing.T) {
	store := openStore(t, "standby")
	cfg := testConfig(t, "standby")
	op := NewOperator(cfg, store)

	err := op.Degrade(DegradeOptions{Confirmed: true, Reason: "no promise to permit"})
	if !errors.Is(err, ErrWrongRole) {
		t.Fatalf("Degrade on a standby = %v, want ErrWrongRole", err)
	}
	if marker, _ := store.Meta(metaDegraded); marker != "" {
		t.Errorf("a standby recorded a single-copy permission: %q", marker)
	}
}

// TestUnconfiguredHACannotBeOperated is the shape check on the mechanism's own
// guard.
//
// A node whose HA is switched off has no role to report and no promise to
// permit. Refusing is better than writing state nothing reads -- an operator
// who ran `goddi ha degrade` against a configuration with HA off would
// otherwise be told it worked.
func TestUnconfiguredHACannotBeOperated(t *testing.T) {
	store := openStore(t, "primary")
	cfg := testConfig(t, "primary")
	cfg.Enabled = false
	op := NewOperator(cfg, store)

	if err := op.Degrade(DegradeOptions{Confirmed: true}); !errors.Is(err, ErrHAOff) {
		t.Fatalf("Degrade with HA off = %v, want ErrHAOff", err)
	}
	if _, err := op.Status(); !errors.Is(err, ErrHAOff) {
		t.Fatalf("Status with HA off = %v, want ErrHAOff", err)
	}
	if marker, _ := store.Meta(metaDegraded); marker != "" {
		t.Errorf("a node with HA off recorded a permission: %q", marker)
	}
}
