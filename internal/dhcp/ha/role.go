package ha

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/auditlog"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	"github.com/jasonwa/goddi/internal/metrics"
)

// Meta keys this package owns for the decisions an operator makes.
//
// They live in the same table as the watermarks because they are the same kind
// of thing: facts this node must still know after a restart that no other node
// can tell it.
const (
	// metaRole records the role this node is running as, once an operator has
	// changed it. Absent means "the configuration's role has never been
	// overridden", which is the state every installation starts in.
	metaRole = "ha_role"
	// metaPeerSeq is the highest sequence the peer has reported, as this node
	// last heard it. On a standby it is the primary's progress, and the
	// distance between it and metaAppliedSeq is what a takeover is decided on.
	metaPeerSeq = "ha_peer_seq"
	// metaPeerSeqAt is when that was last heard. It is persisted rather than
	// kept in memory because the process that decides a takeover is not the
	// process that observed the link.
	metaPeerSeqAt = "ha_peer_seq_at"
	// metaFencedAt records when an operator took this node out of service.
	metaFencedAt = "ha_fenced_at"
	// metaTakeoverAt records when this node was promoted, so that a node that
	// has been promoted twice is distinguishable from one that never was.
	metaTakeoverAt = "ha_takeover_at"
	// metaAcceptedGap records the shortfall the operator accepted when this
	// node was promoted, and is written only by Takeover.
	//
	// It is separate from the distance between the two watermarks because
	// promotion clears the applied watermark -- it has to, a leftover figure
	// above the resumed sequence would satisfy every confirmation wait on the
	// spot -- and with it the ability to recover what was accepted. The
	// distance a promoted node can compute afterwards is peer_seq minus zero,
	// which is the peer's whole counter rather than the part this node is
	// missing. That number is wrong in the one direction that matters: it
	// overstates the loss, and it never changes again.
	metaAcceptedGap = "ha_accepted_gap"
)

// RoleFenced is the recorded role of a node an operator has taken out of
// service.
//
// It is not a configuration value. The configuration says what the deployment
// intends a node to be; this says what happened to it, which is why the two are
// allowed to disagree and why the recorded one wins.
const RoleFenced = "fenced"

// takeoverQuietMultiple is how many staleness bounds of silence a takeover
// requires.
//
// The bound exists so that a takeover cannot be pointed at a primary that is
// demonstrably answering: the standby's read deadline is one staleness bound, so
// three of them is several reconnect cycles past the point at which a live
// primary would have been heard from.
//
// It is deliberately not sold as a partition detector. A partitioned primary is
// silent in exactly the way a dead one is, and no value of this number changes
// that (INV-4). What rules out a partitioned primary is the operator's
// statement that it can no longer write, which is why that statement is a
// separate, unbypassable input.
const takeoverQuietMultiple = 3

// Operator errors. Each names a different reason to refuse, because the
// operator has to know which one to act on.
var (
	// ErrHAOff means HA is not enabled on this node.
	ErrHAOff = errors.New("ha: dhcp_ha.enabled is false")
	// ErrNoConfirmation means the operator did not confirm an action that
	// changes what this node serves.
	ErrNoConfirmation = errors.New("ha: this action changes what this node serves and requires explicit confirmation")
	// ErrUnfencedPrimary means a takeover was asked for without the operator
	// stating that the old primary can no longer write.
	ErrUnfencedPrimary = errors.New("ha: a takeover requires the old primary to be stopped or fenced first")
	// ErrPeerStillAnswering means the peer reported its watermarks recently
	// enough that promoting this node would create a second author.
	ErrPeerStillAnswering = errors.New("ha: the primary is still answering")
	// ErrUnexplainedGap means this node does not hold everything the primary
	// handed out, and the gap was not accepted by its exact size.
	ErrUnexplainedGap = errors.New("ha: this node does not hold every change the primary handed out")
	// ErrWrongRole means this node is not in the role the action applies to.
	ErrWrongRole = errors.New("ha: this node is not in the role that action applies to")
	// ErrNotFenced means a rejoin was asked for on a node that is not fenced.
	ErrNotFenced = errors.New("ha: this node is not fenced")
)

// EffectiveRole reports the role this node should run as.
//
// The recorded role wins over the configuration. That ordering is the whole
// point of recording it: the configuration is a declaration of intent that was
// written before anything happened, and a promotion or a fence is a fact about
// what happened afterwards. Letting the file win would mean the node an
// operator fenced came back as a primary on the next restart.
//
// A recorded role this build does not recognise is an error rather than a
// fallback to the configuration. Falling back would bring the node up in the
// role an operator had changed it away from, which is the one outcome the
// recording exists to prevent.
func EffectiveRole(cfg Config, store *dataplane.Store) (string, error) {
	recorded, err := store.Meta(metaRole)
	if err != nil {
		return "", fmt.Errorf("ha: reading the recorded role: %w", err)
	}
	switch recorded = strings.TrimSpace(recorded); recorded {
	case "":
		return cfg.Role, nil
	case config.HARolePrimary, config.HARoleStandby, RoleFenced:
		return recorded, nil
	default:
		return "", fmt.Errorf("ha: the recorded role %q is not a role this build understands", recorded)
	}
}

// Status is what this node's own store can say about its redundancy.
//
// It is deliberately weaker than what the running process knows. State here is
// derived from persisted facts alone, so it cannot tell `primary` from
// `paused`: that difference is whether the peer link is fresh, which only the
// process holding the link observes. A caller that needs that answer reads the
// process, not this.
type Status struct {
	// NodeID names this node as it identifies itself to its peer.
	NodeID string `json:"node_id"`
	// ConfiguredRole is what the configuration file says.
	ConfiguredRole string `json:"configured_role"`
	// Role is the role in force, which is the recorded one when an operator
	// has changed it.
	Role string `json:"role"`
	// State is what the persisted facts establish.
	State State `json:"state"`

	// Redundant is whether the leases this node promises exist in two places.
	// It is only an answer when RedundancyKnown is set.
	Redundant bool `json:"redundant"`
	// RedundancyKnown is false when this snapshot cannot establish whether the
	// node currently has a second copy.
	//
	// Redundancy is not a property of a store, it is the peer link, and only
	// the running process observes that. A snapshot of a primary whose store
	// records no withholding therefore cannot say "redundant": saying it is the
	// one reading that would let a script conclude a node is safe when nobody
	// has looked. The two states the store does settle are the two an operator
	// put there -- degraded, which is the absence of a second copy by
	// definition, and fenced, which is a node out of service.
	RedundancyKnown bool `json:"redundancy_known"`

	// Seq and AckedSeq are the primary's counter and its record of what the
	// mirror confirmed.
	Seq      int64 `json:"seq"`
	AckedSeq int64 `json:"acked_seq"`
	// AppliedSeq is the standby's watermark: the highest sequence it holds.
	AppliedSeq int64 `json:"applied_seq"`
	// PeerSeq is the highest sequence the peer reported, and PeerSeqAt is when
	// it last said anything.
	PeerSeq   int64     `json:"peer_seq"`
	PeerSeqAt time.Time `json:"peer_seq_at,omitzero"`
	// Gap is PeerSeq - AppliedSeq, floored at zero, and it is only computed on
	// a standby.
	//
	// The subtraction means "changes the primary handed out that this node does
	// not hold", and it only says that where the peer's counter is a figure
	// this node was expecting to catch up to. On any other role the peer's
	// counter is not this node's business: a plain primary never records one at
	// all, and a promoted one still holds the figure the node it took over from
	// reached in its previous life -- which, with the applied watermark cleared
	// by the promotion itself, would be reported as a shortfall of everything
	// the peer had ever handed out, permanently. A number that describes
	// nothing is worse than no number, so it is zero off a standby.
	Gap int64 `json:"gap"`
	// AcceptedGap is the shortfall this node's promotion accepted, and zero on
	// a node that has never been promoted.
	//
	// It answers the question Gap cannot answer after the fact -- how much was
	// given up when the other node was declared gone -- which is otherwise only
	// in the audit trail and in the terminal the operator ran it from.
	AcceptedGap int64 `json:"accepted_gap"`

	Degraded       bool      `json:"degraded"`
	DegradedAt     time.Time `json:"degraded_at,omitzero"`
	DegradedReason string    `json:"degraded_reason,omitempty"`
	FencedAt       time.Time `json:"fenced_at,omitzero"`
	TakeoverAt     time.Time `json:"takeover_at,omitzero"`
}

// Operator performs the decisions that only a human may make.
//
// Every method here is called from `goddi ha`, and none of them is called from
// a serving path. That is not a convention to be maintained by review: INV-6
// exists because a system that can decide to carry on alone will eventually
// decide it by accident, so the code paths that permit it are separated from
// the ones that serve.
type Operator struct {
	cfg   Config
	store *dataplane.Store
}

// NewOperator builds the operator side of a node.
func NewOperator(cfg Config, store *dataplane.Store) *Operator {
	return &Operator{cfg: cfg, store: store}
}

// Status reads this node's persisted redundancy facts.
func (o *Operator) Status() (Status, error) {
	if err := o.usable(); err != nil {
		return Status{}, err
	}
	role, err := EffectiveRole(o.cfg, o.store)
	if err != nil {
		return Status{}, err
	}
	st := Status{NodeID: o.cfg.NodeID, ConfiguredRole: o.cfg.Role, Role: role}

	if st.Seq, err = readWatermark(o.store.DB, metaSeq); err != nil {
		return Status{}, err
	}
	if st.AckedSeq, err = readWatermark(o.store.DB, metaAckedSeq); err != nil {
		return Status{}, err
	}
	if st.AppliedSeq, err = readWatermark(o.store.DB, metaAppliedSeq); err != nil {
		return Status{}, err
	}
	if st.PeerSeq, err = readWatermark(o.store.DB, metaPeerSeq); err != nil {
		return Status{}, err
	}

	var marker string
	if marker, err = o.store.Meta(metaDegraded); err != nil {
		return Status{}, err
	}
	st.Degraded = marker != ""
	if st.Degraded {
		st.DegradedAt, _ = time.Parse(time.RFC3339, marker)
		if st.DegradedReason, err = o.store.Meta(metaDegradedReason); err != nil {
			return Status{}, err
		}
	}

	if st.PeerSeqAt, err = o.timestamp(metaPeerSeqAt); err != nil {
		return Status{}, err
	}
	if st.FencedAt, err = o.timestamp(metaFencedAt); err != nil {
		return Status{}, err
	}
	if st.TakeoverAt, err = o.timestamp(metaTakeoverAt); err != nil {
		return Status{}, err
	}

	// Only a mirror is missing somebody else's changes. See the field's comment
	// for why the subtraction is not the answer anywhere else.
	if role == config.HARoleStandby {
		if st.Gap = st.PeerSeq - st.AppliedSeq; st.Gap < 0 {
			st.Gap = 0
		}
	}
	// The applied watermark is cleared by a promotion, so this figure -- not
	// the subtraction -- is the only surviving record of what was accepted.
	if st.AcceptedGap, err = readWatermark(o.store.DB, metaAcceptedGap); err != nil {
		return Status{}, err
	}

	switch {
	case role == RoleFenced:
		// Out of service on purpose: nothing is replicated and nothing is
		// promised, which the store is enough to establish.
		st.State = StateFenced
		st.Redundant, st.RedundancyKnown = false, true
	case role == config.HARoleStandby:
		// A standby holds somebody else's facts. Whether they are still
		// arriving is the link, and whether the pair is redundant is the
		// primary's fact to report, not this one's.
		st.State = StateStandby
	case st.Degraded:
		// The absence of a second copy is what the permission says.
		st.State = StatePrimaryDegraded
		st.Redundant, st.RedundancyKnown = false, true
	default:
		// The store records no withholding. Whether this node is nonetheless
		// withholding is whether the peer link is fresh, and that is not here.
		st.State = StatePrimary
	}
	return st, nil
}

// DegradeOptions carries an operator's decision to run without a second copy,
// or to withdraw one.
type DegradeOptions struct {
	// Confirmed must be set. It is a field rather than a flag check so that the
	// requirement lives with the mechanism that enforces it: a second caller
	// cannot forget it.
	Confirmed bool
	// Reason is what the operator says they are doing and why. It is recorded
	// with the marker and served back on the console.
	Reason string
	// Undo withdraws the permission instead of granting it. Withdrawing is
	// always the safe direction -- the node returns to withholding
	// acknowledgements -- so it takes the same confirmation and no more.
	Undo bool
}

// Degrade records that this node will serve without a second copy, or that the
// operator has changed their mind.
//
// It is the only writer of the marker, and it is not reachable from the serving
// path. A system that can decide to carry on alone will, eventually, decide it
// by accident: a heartbeat times out, a retry gives up, a startup check passes
// the wrong way, and the deployment is silently running without the redundancy
// it was designed around.
func (o *Operator) Degrade(opts DegradeOptions) error {
	if err := o.usable(); err != nil {
		return err
	}
	if !opts.Confirmed {
		return fmt.Errorf("%w: this node will acknowledge bindings it cannot recover", ErrNoConfirmation)
	}
	role, err := EffectiveRole(o.cfg, o.store)
	if err != nil {
		return err
	}
	// A standby makes no promises of its own, so there is none to permit. The
	// refusal is not a formality: degrading a mirror would be taken by an
	// operator as "this node may now serve", which is the opposite of what a
	// standby is for.
	if role != config.HARolePrimary {
		return fmt.Errorf("%w: %s is %q, and only a primary has promises to permit", ErrWrongRole, o.cfg.NodeID, role)
	}

	reason := strings.TrimSpace(opts.Reason)
	if reason == "" {
		if opts.Undo {
			reason = "withdrawn by goddi ha degrade --undo"
		} else {
			reason = "granted by goddi ha degrade"
		}
	}

	if opts.Undo {
		if err := clearMeta(o.store.DB, metaDegraded, metaDegradedReason); err != nil {
			return err
		}
		o.audit(auditlog.ActionHADegrade,
			"single-copy permission withdrawn: "+reason,
			"second copy required", "second copy required unless the peer is current")
		return nil
	}

	now := time.Now().UTC()
	if err := setMetaText(o.store.DB, metaDegraded, now.Format(time.RFC3339)); err != nil {
		return err
	}
	if err := setMetaText(o.store.DB, metaDegradedReason, reason); err != nil {
		return err
	}
	o.audit(auditlog.ActionHADegrade,
		"single-copy permission granted: "+reason,
		"second copy required", "serving without a second copy")
	return nil
}

// TakeoverOptions carries an operator's decision to promote this node.
type TakeoverOptions struct {
	// Confirmed must be set.
	Confirmed bool
	// OldPrimaryCannotWrite must be set. It is the operator's statement that
	// the node which was primary has been stopped, disconnected or fenced --
	// the one fact this node cannot observe, because a dead primary and a
	// partitioned one are both silent to it.
	OldPrimaryCannotWrite bool
	// AcceptGap is the exact size of the shortfall the operator is accepting.
	// A non-zero gap is refused unless this matches it, so that proceeding past
	// a shortfall requires having read how big it is.
	AcceptGap int64
}

// TakeoverOutcome reports what a promotion decided, for the operator's log and
// for the audit entry.
type TakeoverOutcome struct {
	// AppliedSeq is what this node held, PeerSeq what the primary had reported,
	// and Gap the difference.
	AppliedSeq int64
	PeerSeq    int64
	Gap        int64
	// PeerSeqAt is when the primary last said anything. It is the evidence
	// behind "the primary is still answering" when this call refuses.
	PeerSeqAt time.Time
	// Seq is the sequence the new primary continues from.
	Seq int64
}

// Takeover promotes a standby to primary.
//
// The three inputs are the three things that cannot be inferred from this node:
// that the operator means it, that the old primary cannot write, and that a
// shortfall is understood. Everything else is read from the store.
func (o *Operator) Takeover(opts TakeoverOptions) (TakeoverOutcome, error) {
	if err := o.usable(); err != nil {
		return TakeoverOutcome{}, err
	}
	role, err := EffectiveRole(o.cfg, o.store)
	if err != nil {
		return TakeoverOutcome{}, err
	}
	if role != config.HARoleStandby {
		return TakeoverOutcome{}, fmt.Errorf("%w: a takeover promotes a standby, and %s is %q",
			ErrWrongRole, o.cfg.NodeID, role)
	}
	if !opts.Confirmed {
		return TakeoverOutcome{}, fmt.Errorf("%w: this node becomes the only author of its leases", ErrNoConfirmation)
	}
	if !opts.OldPrimaryCannotWrite {
		return TakeoverOutcome{}, ErrUnfencedPrimary
	}

	out := TakeoverOutcome{}
	if out.AppliedSeq, err = readWatermark(o.store.DB, metaAppliedSeq); err != nil {
		return out, err
	}
	if out.PeerSeq, err = readWatermark(o.store.DB, metaPeerSeq); err != nil {
		return out, err
	}
	if out.PeerSeqAt, err = o.timestamp(metaPeerSeqAt); err != nil {
		return out, err
	}
	if out.Gap = out.PeerSeq - out.AppliedSeq; out.Gap < 0 {
		out.Gap = 0
	}

	// A primary that answered within the quiet window is alive, and two writers
	// is the failure a takeover exists to avoid. This is the one refusal here
	// with no way past it: there is no argument an operator could make that
	// would make a live peer safe to take over from.
	if quiet := time.Since(out.PeerSeqAt); !out.PeerSeqAt.IsZero() && quiet < takeoverQuietMultiple*o.cfg.PeerStaleAfter {
		return out, fmt.Errorf("%w: it reported its watermarks %s ago, which is inside the %s window this node waits before promoting",
			ErrPeerStillAnswering, quiet.Round(time.Millisecond), takeoverQuietMultiple*o.cfg.PeerStaleAfter)
	}

	// The shortfall is reported, not hidden. It cannot contain an acknowledged
	// binding -- a binding is acknowledged only once the standby has applied
	// the sequence carrying it, so anything this node never applied was never
	// acknowledged -- which is exactly why it is a judgement call rather than a
	// correctness check, and why the operator is asked for the number.
	if out.Gap > 0 && opts.AcceptGap != out.Gap {
		return out, fmt.Errorf("%w: the primary reached sequence %d and this node holds %d, so %d changes are missing; re-run with --accept-gap %d if that is understood",
			ErrUnexplainedGap, out.PeerSeq, out.AppliedSeq, out.Gap, out.Gap)
	}

	tx, err := o.store.Begin()
	if err != nil {
		return out, fmt.Errorf("ha: opening the promotion: %w", err)
	}
	defer tx.Rollback()
	ctx := context.Background()

	// The new counter starts above everything either node has used.
	//
	// Below it would hand out a number a standby has already applied and read
	// as a duplicate; equal to it would re-use the last row's number. The
	// peer's figure is included even though the rule in the contract names only
	// this node's two: the peer is the node whose sequence space has to be
	// joined up with, and a promotion that restarted below what the peer had
	// reached would hand the re-joined peer numbers it had already used.
	resume, err := readWatermarkTx(ctx, tx, metaSeq)
	if err != nil {
		return out, err
	}
	for _, candidate := range []int64{out.AppliedSeq, out.PeerSeq} {
		if candidate > resume {
			resume = candidate
		}
	}
	out.Seq = resume

	if err := setMetaTextTx(ctx, tx, metaRole, config.HARolePrimary); err != nil {
		return out, err
	}
	if err := setWatermark(ctx, tx, metaSeq, resume); err != nil {
		return out, err
	}
	// The promotion comes with the permission to serve alone, because that is
	// what it is for. A primary that came up paused would withhold every
	// acknowledgement until the node it was promoted away from came back, and
	// the takeover would have bought nothing.
	now := time.Now().UTC()
	if err := setMetaTextTx(ctx, tx, metaDegraded, now.Format(time.RFC3339)); err != nil {
		return out, err
	}
	if err := setMetaTextTx(ctx, tx, metaDegradedReason, "promoted by goddi ha takeover"); err != nil {
		return out, err
	}
	if err := setMetaTextTx(ctx, tx, metaTakeoverAt, now.Format(time.RFC3339)); err != nil {
		return out, err
	}
	// What was given up is recorded in the same transaction as the promotion,
	// because the applied watermark that would otherwise be the only way to
	// recover it is cleared by that same transaction. Written even when it is
	// zero: "promoted with nothing missing" is an answer an operator reading
	// the record later needs just as much as a shortfall is.
	if err := setWatermark(ctx, tx, metaAcceptedGap, out.Gap); err != nil {
		return out, err
	}
	// This node is not a mirror any more, and it must not carry a mirror's
	// watermark into a later life as one: a stale applied_seq would make a
	// future reconnect ignore the snapshot it should adopt.
	//
	// The acknowledged watermark goes too, and that one is not housekeeping.
	// It records what a mirror confirmed in an earlier life of this node, and a
	// figure left over from then can sit above the sequence this node is about
	// to resume from -- at which point every confirmation would satisfy its
	// wait immediately and a binding would be acknowledged with no second copy
	// at all. Nothing has been confirmed by the mirror this node does not yet
	// have, so the honest value is none.
	if err := clearMetaTx(ctx, tx, metaAppliedSeq, metaAckedSeq); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, fmt.Errorf("ha: committing the promotion: %w", err)
	}

	o.audit(auditlog.ActionHATakeover,
		fmt.Sprintf("promoted to primary: applied_seq %d, primary_seq %d, gap %d, resuming at seq %d",
			out.AppliedSeq, out.PeerSeq, out.Gap, out.Seq),
		fmt.Sprintf("applied_seq=%d primary_seq=%d", out.AppliedSeq, out.PeerSeq),
		fmt.Sprintf("role=primary seq=%d serving_without_a_second_copy=true", out.Seq))
	return out, nil
}

// FenceOptions carries an operator's decision to take a node out of service.
type FenceOptions struct {
	// Confirmed must be set.
	Confirmed bool
	// Reason is recorded with the fence.
	Reason string
}

// Fence marks this node as one that must not answer anything.
//
// It is what makes a takeover safe on the other side: the node that was primary
// is the only one that can stop itself, and a takeover from a node that has not
// been stopped is two writers. Like every other state here it is applied with
// the operator's knowledge and lifted the same way.
func (o *Operator) Fence(opts FenceOptions) error {
	if err := o.usable(); err != nil {
		return err
	}
	if !opts.Confirmed {
		return fmt.Errorf("%w: this node stops serving and stops mirroring", ErrNoConfirmation)
	}
	role, err := EffectiveRole(o.cfg, o.store)
	if err != nil {
		return err
	}
	if role == RoleFenced {
		return fmt.Errorf("%w: %s is already fenced", ErrWrongRole, o.cfg.NodeID)
	}

	now := time.Now().UTC()
	if err := setMetaText(o.store.DB, metaRole, RoleFenced); err != nil {
		return err
	}
	if err := setMetaText(o.store.DB, metaFencedAt, now.Format(time.RFC3339)); err != nil {
		return err
	}
	reason := strings.TrimSpace(opts.Reason)
	if reason == "" {
		reason = "fenced by goddi ha fence"
	}
	o.audit(auditlog.ActionHAFence, "taken out of service: "+reason,
		"role="+role, "role="+RoleFenced)
	return nil
}

// RejoinOptions carries an operator's decision to bring a fenced node back.
type RejoinOptions struct {
	// Confirmed must be set.
	Confirmed bool
}

// Rejoin returns a fenced node to the pair as a standby.
//
// A rejoining node always comes back as a mirror, never as a primary: it has
// been out of the pair while somebody else was serving, so the only thing it
// can be trusted to do is hold a copy of what that node holds.
func (o *Operator) Rejoin(opts RejoinOptions) error {
	if err := o.usable(); err != nil {
		return err
	}
	role, err := EffectiveRole(o.cfg, o.store)
	if err != nil {
		return err
	}
	if role != RoleFenced {
		return fmt.Errorf("%w: %s is %q, so there is nothing to return to the pair", ErrNotFenced, o.cfg.NodeID, role)
	}
	if !opts.Confirmed {
		return fmt.Errorf("%w: this node's leases will be replaced by the primary's", ErrNoConfirmation)
	}

	if err := setMetaText(o.store.DB, metaRole, config.HARoleStandby); err != nil {
		return err
	}
	// The fence marker and both watermarks go. The first describes a state this
	// node is no longer in; the mirror's watermark describes content it is
	// about to have replaced wholesale, and keeping it would make the reconnect
	// ignore the snapshot that should supersede it. The acknowledged watermark
	// describes a mirror this node does not have, and it is cleared here for
	// the reason a promotion clears it: a stale figure can outrank the sequence
	// a future promotion resumes from, and every wait would then be satisfied
	// on the spot.
	if err := clearMeta(o.store.DB, metaFencedAt, metaAppliedSeq, metaAckedSeq); err != nil {
		return err
	}
	o.audit(auditlog.ActionHARejoin, "returned to the pair as a standby",
		"role="+RoleFenced, "role="+config.HARoleStandby)
	return nil
}

// usable refuses the operator actions on a node that has HA switched off.
//
// A node with no second copy to lose and no peer to promote away from has
// nothing for these actions to mean, and accepting them would write state that
// nothing reads.
func (o *Operator) usable() error {
	if !o.cfg.Enabled {
		return ErrHAOff
	}
	if o.store == nil {
		return fmt.Errorf("ha: this node has no lease store")
	}
	return nil
}

// audit records one operator decision.
//
// The entry goes into this node's own store, where the data plane's other
// audit entries are written, so it travels up to the archive by the path that
// already exists. It is written after the decision has been committed: a trail
// entry for something that did not happen is worse than a decision whose entry
// was lost, and the decision is also recorded in the meta row it wrote.
func (o *Operator) audit(action, detail, oldValue, newValue string) {
	if _, err := auditlog.Append(o.store.DB, auditlog.Entry{
		// There is no authenticated operator on a command line. The name of
		// the OS account that ran it is the only actor there is, and saying so
		// is better than implying a console login that did not happen.
		UserID:       "cli",
		Username:     "cli:" + osUser(),
		Action:       action,
		ResourceType: auditlog.ResourceNode,
		ResourceID:   o.cfg.NodeID,
		Detail:       detail,
		OldValue:     oldValue,
		NewValue:     newValue,
	}); err != nil {
		warnAuditFailure(action, o.cfg.NodeID, err)
	}
}

// timestamp reads a meta row holding an RFC 3339 instant. A row that is missing
// is the zero time; a row that cannot be parsed is an error, because the
// alternative is a timestamp that silently reads as "never".
func (o *Operator) timestamp(key string) (time.Time, error) {
	raw, err := o.store.Meta(key)
	if err != nil {
		return time.Time{}, fmt.Errorf("ha: reading %s: %w", key, err)
	}
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("ha: %s holds %q, which is not a time: %w", key, raw, err)
	}
	return at, nil
}

// readWatermarkTx reads a watermark inside a transaction, so that a
// read-modify-write of the sequence cannot interleave with a writer.
func readWatermarkTx(ctx context.Context, tx *sql.Tx, key string) (int64, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT value FROM dataplane_meta WHERE key = ?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ha: reading the %s watermark: %w", key, err)
	}
	var n int64
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &n); err != nil {
		return 0, fmt.Errorf("ha: the %s watermark %q is not a number: %w", key, raw, err)
	}
	return n, nil
}

func setMetaText(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("ha: writing %s: %w", key, err)
	}
	return nil
}

func setMetaTextTx(ctx context.Context, tx *sql.Tx, key, value string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("ha: writing %s: %w", key, err)
	}
	return nil
}

func clearMeta(db *sql.DB, keys ...string) error {
	statement, args := clearMetaStatement(keys)
	if statement == "" {
		return nil
	}
	if _, err := db.Exec(statement, args...); err != nil {
		return fmt.Errorf("ha: clearing %s: %w", strings.Join(keys, ", "), err)
	}
	return nil
}

func clearMetaTx(ctx context.Context, tx *sql.Tx, keys ...string) error {
	statement, args := clearMetaStatement(keys)
	if statement == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		return fmt.Errorf("ha: clearing %s: %w", strings.Join(keys, ", "), err)
	}
	return nil
}

// clearMetaStatement builds one DELETE for a set of keys, so that the two call
// shapes above cannot drift into two different statements.
func clearMetaStatement(keys []string) (string, []any) {
	if len(keys) == 0 {
		return "", nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(keys)), ", ")
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	return `DELETE FROM dataplane_meta WHERE key IN (` + placeholders + `)`, args
}

// warnAuditFailure reports an audit entry that could not be written.
//
// It is a warning rather than an error for the same reason the data plane's
// other audit writers log and carry on: the decision has already been made and
// recorded in the meta row it changed, and refusing now would leave the caller
// believing an action did not happen when it did.
func warnAuditFailure(action, nodeID string, err error) {
	slog.Warn("ha: could not write the audit entry for an operator decision",
		"action", action, "node_id", nodeID, "error", err)
	metrics.RecordDBError()
}

// osUser names the OS account that ran the command.
//
// os/user is avoided on purpose: it links cgo on some platforms for no benefit
// here, and an empty answer is acceptable for an audit field whose value is
// informational.
func osUser() string {
	if u := strings.TrimSpace(os.Getenv("USER")); u != "" {
		return u
	}
	if u := strings.TrimSpace(os.Getenv("LOGNAME")); u != "" {
		return u
	}
	return "unknown"
}
