package ha

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/jasonwa/goddi/internal/dataplane"
)

// ErrNoSecondCopy is returned when a binding cannot be confirmed because this
// node is not currently allowed to promise one.
//
// Callers must treat it as "stay silent", not as "reject": the address the
// client asked for is fine, this node simply cannot say so yet. A NAK here
// would tell the client to abandon a working address because the server is
// temporarily short of a second machine.
var ErrNoSecondCopy = errors.New("ha: no second copy of this binding is available")

// ErrNotConfirmed is returned when the mirror did not confirm within the
// configured timeout.
var ErrNotConfirmed = errors.New("ha: the second copy did not confirm in time")

// opsBatchLimit bounds one outgoing batch. It is a batch size rather than a
// frame size because the frames are built from rows whose size is bounded, and
// a row count is the thing an operator can reason about.
const opsBatchLimit = 512

// maxPendingOps bounds the unsent log.
//
// Nothing here is load-bearing for correctness: an op that falls off the front
// is one whose effect the next snapshot will carry instead. The bound exists so
// that a primary whose peer has been gone for a week does not grow a queue
// proportional to the week.
const maxPendingOps = 65536

type pendingOp struct {
	seq int64
	row LeaseRow
}

// Replicator is the primary half of the pair.
//
// It owns the sequence, which is the only thing in this design that has to be
// globally agreed: the primary hands out numbers, the standby reports the
// highest one it has durably applied, and a binding is acknowledged when the
// number carrying it has been reported back. Heartbeats do not participate --
// they say "something is listening", and what a binding needs to know is
// "somebody else has this".
type Replicator struct {
	cfg   Config
	store *dataplane.Store
	link  linkHealth

	mu      sync.Mutex
	seq     int64
	acked   int64
	pending []pendingOp
	// advance is closed and replaced whenever acked moves. Waiting on a
	// channel rather than a condition variable is what makes the wait
	// interruptible by a context and bounded by a timer.
	advance    chan struct{}
	lastSent   time.Time
	degraded   bool
	degradedAt time.Time

	// wake tells the send loop that the log is no longer empty. It is
	// buffered to one: a signal that arrives while one is already pending is
	// the same signal.
	wake chan struct{}

	// bound is the address the peer listener actually took. It is recorded
	// because "what port did that become" is not answerable from the
	// configuration when the operator asked for port 0.
	bound net.Addr
}

// Addr reports the address the peer listener is bound to, or nil before Run
// has taken the port.
func (r *Replicator) Addr() net.Addr {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bound
}

// NewReplicator opens the primary side against a lease store.
func NewReplicator(cfg Config, store *dataplane.Store) (*Replicator, error) {
	if cfg.IsStandby() {
		return nil, fmt.Errorf("ha: a replicator must be configured with role=%s, got %q", "primary", cfg.Role)
	}
	seq, err := readWatermark(store.DB, metaSeq)
	if err != nil {
		return nil, fmt.Errorf("ha: reading the sequence: %w", err)
	}
	acked, err := readWatermark(store.DB, metaAckedSeq)
	if err != nil {
		return nil, fmt.Errorf("ha: reading the acknowledged watermark: %w", err)
	}
	r := &Replicator{
		cfg:     cfg,
		store:   store,
		seq:     seq,
		acked:   acked,
		advance: make(chan struct{}),
		wake:    make(chan struct{}, 1),
	}
	marker, err := store.Meta(metaDegraded)
	if err != nil {
		return nil, fmt.Errorf("ha: reading the degraded marker: %w", err)
	}
	r.degraded = marker != ""
	if r.degraded {
		r.degradedAt, _ = time.Parse(time.RFC3339, marker)
	}
	return r, nil
}

// Seq reports the highest sequence handed out.
func (r *Replicator) Seq() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq
}

// AckedSeq reports the highest sequence the mirror has confirmed.
func (r *Replicator) AckedSeq() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.acked
}

// State reports what this node believes it is.
//
// It is derived from facts rather than assigned: the degraded marker is the
// only thing here an operator sets, and everything else is "is the peer
// current". That is deliberate -- a state a code path can set is a state a
// code path can get wrong.
func (r *Replicator) State() State {
	r.mu.Lock()
	degraded := r.degraded
	r.mu.Unlock()
	if degraded {
		return StatePrimaryDegraded
	}
	if r.link.fresh(r.cfg.PeerStaleAfter) {
		return StatePrimary
	}
	return StatePaused
}

// MayBind reports whether this node may promise an address or an extension.
func (r *Replicator) MayBind() bool { return r.State().MayBind() }

// PeerWatermarks reports the last handshake this node saw from its mirror.
func (r *Replicator) PeerWatermarks() Watermarks {
	_, _, w := r.link.snapshot()
	return w
}

// LinkSnapshot reports the peer link's health for the probe and the metrics.
func (r *Replicator) LinkSnapshot() (up bool, lastSeen time.Time) {
	up, lastSeen, _ = r.link.snapshot()
	return up, lastSeen
}

// Degraded reports whether the node is running on an operator's explicit
// permission rather than on a second copy.
func (r *Replicator) Degraded() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.degraded
}

// DegradedReason returns the operator's stated reason, or "".
//
// This is the only reader of the reason row; the only writer is
// Operator.Degrade. There is deliberately no MarkDegraded on this type. A
// second writer would be a second author for the one fact in this design that
// must have exactly one, and it would sit inside the process that serves --
// which is where an automatic promotion would come from.
func (r *Replicator) DegradedReason() string {
	reason, err := r.store.Meta(metaDegradedReason)
	if err != nil {
		return ""
	}
	return reason
}

// Confirm replicates one lease change and waits for the second copy to be
// durable.
//
// It is the whole of the ACK gate. The row has already been committed locally
// by the time this is called -- that ordering is the point: the mirror is only
// ever asked to hold something this node has, never the reverse.
func (r *Replicator) Confirm(ctx context.Context, row LeaseRow) error {
	// The state is read once. Reading it again after the wait would be asking a
	// different question than the one the caller is waiting on the answer to.
	state := r.State()
	if !state.MayBind() {
		return ErrNoSecondCopy
	}
	seq, err := r.enqueue(row)
	if err != nil {
		// The row is recorded for the mirror anyway, so the mirror keeps
		// converging; what failed is this node's ability to say when. Not
		// being able to say when is the same as not knowing.
		return err
	}

	// A node serving on an operator's permission makes the promise itself.
	//
	// Waiting here would be waiting for a second copy whose absence is exactly
	// what the permission is about -- so the wait would end in a withheld
	// acknowledgement, and the operator's decision would have no effect on what
	// clients experience. That is the difference between this state and paused:
	// paused cannot promise, degraded promises on its own authority and says so
	// everywhere it reports.
	if !state.Redundant() {
		return nil
	}

	deadline := time.NewTimer(r.cfg.ConfirmTimeout)
	defer deadline.Stop()

	for {
		r.mu.Lock()
		if r.acked >= seq {
			r.mu.Unlock()
			return nil
		}
		ch := r.advance
		r.mu.Unlock()

		select {
		case <-ch:
		case <-deadline.C:
			slog.Warn("HA: withholding an acknowledgement; the mirror did not confirm in time",
				"seq", seq, "acked_seq", r.AckedSeq(), "timeout", r.cfg.ConfirmTimeout)
			return fmt.Errorf("%w (seq %d, mirror at %d, timeout %s)",
				ErrNotConfirmed, seq, r.AckedSeq(), r.cfg.ConfirmTimeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Replicate records a lease change for the mirror without waiting for it.
//
// This is for changes that are not promises: an offer is a reservation with an
// expiry, and a client that never receives it has lost a round trip rather than
// an address. Replicating them keeps the mirror close enough to the primary
// that a takeover does not hand out an address the primary had just offered.
func (r *Replicator) Replicate(row LeaseRow) {
	if _, err := r.enqueue(row); err != nil {
		slog.Error("HA: could not record a lease change for the mirror", "lease_id", row.ID, "error", err)
	}
}

// enqueue assigns the next sequence and appends the row to the outgoing log.
func (r *Replicator) enqueue(row LeaseRow) (int64, error) {
	r.mu.Lock()
	r.seq++
	seq := r.seq
	r.pending = append(r.pending, pendingOp{seq: seq, row: row})
	if len(r.pending) > maxPendingOps {
		// The oldest entries are the ones the next snapshot will cover in
		// full, so they are the ones to give up.
		drop := len(r.pending) - maxPendingOps
		r.pending = append(r.pending[:0], r.pending[drop:]...)
	}
	r.mu.Unlock()

	select {
	case r.wake <- struct{}{}:
	default:
	}

	// The sequence outlives the process. A counter that restarts from zero
	// would let a restarted primary hand out numbers a standby has already
	// seen, and the standby would read the duplicates as "already applied".
	if _, err := r.store.Exec(`
		INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, metaSeq, fmt.Sprintf("%d", seq)); err != nil {
		return seq, fmt.Errorf("ha: persisting sequence %d: %w", seq, err)
	}
	return seq, nil
}

// Run accepts the mirror and keeps it current until ctx is cancelled.
//
// Connections are handled one at a time. Two mirrors is not a configuration
// this contract supports, and accepting a second one silently would give two
// nodes the same watermarks while only one of them is the one being trusted.
func (r *Replicator) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", r.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("ha: listening for the peer on %s: %w", r.cfg.ListenAddr, err)
	}
	defer ln.Close()

	r.mu.Lock()
	r.bound = ln.Addr()
	r.mu.Unlock()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	// Started with the listener rather than with a session: the decision this
	// adopts is most needed exactly when there is no session, because the peer
	// it would be waiting for is the one that is gone.
	go r.watchOperatorDecisions(ctx)

	slog.Info("HA: primary is listening for its mirror",
		"addr", ln.Addr().String(), "node_id", r.cfg.NodeID, "seq", r.Seq())

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("ha: accepting a peer connection: %w", err)
		}
		// The peer link is marked down before the failure is reported, so that
		// the state printed with it is the state this node is actually in. The
		// other order reported "primary" on the line announcing that the
		// mirror had died, which is the one word an operator reading that line
		// must not see.
		err = r.session(ctx, conn)
		conn.Close()
		r.link.markDown()
		if err != nil && ctx.Err() == nil {
			slog.Warn("HA: the mirror session ended", "error", err, "state", r.State())
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// session is one mirror connection's lifetime.
func (r *Replicator) session(ctx context.Context, conn net.Conn) error {
	peer, err := r.handshake(conn)
	if err != nil {
		return err
	}
	r.link.markUp(peer)
	slog.Info("HA: mirror connected",
		"peer_node_id", peer.NodeID, "peer_applied_seq", peer.AppliedSeq,
		"state", r.State())

	// Every connection starts from a snapshot. Incremental catch-up would need
	// the primary to know what the mirror is missing, and the only thing it
	// can honestly know is what the mirror has confirmed -- which is exactly
	// what a snapshot makes irrelevant. One bounded transfer per reconnect
	// removes a class of gap-tracking bugs that are otherwise found in
	// production.
	snapSeq, rows, err := r.readSnapshot(ctx)
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(r.cfg.PeerStaleAfter))
	if err := writeFrame(conn, Frame{Type: FrameSnapshot, Seq: snapSeq, Leases: rows}); err != nil {
		return err
	}
	r.prunePending(snapSeq)
	slog.Info("HA: sent a snapshot to the mirror", "seq", snapSeq, "rows", len(rows))

	frames := make(chan Frame)
	failures := make(chan error, 1)
	go func() {
		for {
			conn.SetReadDeadline(time.Now().Add(3 * r.cfg.PeerStaleAfter))
			f, err := readFrame(conn, maxFrameBytes)
			if err != nil {
				failures <- err
				return
			}
			select {
			case frames <- f:
			case <-ctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(r.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case f := <-frames:
			if err := r.handleFrame(f); err != nil {
				return err
			}
		case err := <-failures:
			return err
		case <-r.wake:
			// fall through to the send below
		case <-ticker.C:
			if !r.link.fresh(r.cfg.PeerStaleAfter) {
				// The peer is connected but silent. Closing the session is
				// what moves this node to paused, which is the state that
				// stops it promising addresses.
				return fmt.Errorf("ha: the mirror has been silent for more than %s", r.cfg.PeerStaleAfter)
			}
		case <-ctx.Done():
			return nil
		}
		if err := r.sendWork(conn); err != nil {
			return err
		}
	}
}

func (r *Replicator) handshake(conn net.Conn) (Watermarks, error) {
	conn.SetReadDeadline(time.Now().Add(r.cfg.PeerStaleAfter))
	frame, err := readFrame(conn, maxFrameBytes)
	if err != nil {
		return Watermarks{}, fmt.Errorf("reading the mirror's handshake: %w", err)
	}
	if frame.Type != FrameHello || frame.Watermarks == nil {
		return Watermarks{}, fmt.Errorf("ha: expected a handshake, got a %q frame", frame.Type)
	}
	if frame.Protocol != Protocol {
		return Watermarks{}, fmt.Errorf("%w: peer speaks v%d, this node speaks v%d",
			ErrProtocolMismatch, frame.Protocol, Protocol)
	}
	// Constant-time comparison: the token is a shared secret, and a comparison
	// that returns early is a comparison that says how much of it was right.
	if subtle.ConstantTimeCompare([]byte(frame.Token), []byte(r.cfg.PeerToken)) != 1 {
		return Watermarks{}, fmt.Errorf("%w: node %q presented the wrong token", ErrAuthFailed, frame.Watermarks.NodeID)
	}
	if frame.Watermarks.Role != "standby" {
		return Watermarks{}, fmt.Errorf("%w: a primary mirrors a standby, the peer says %q",
			ErrRoleMismatch, frame.Watermarks.Role)
	}
	if frame.Watermarks.NodeID == r.cfg.NodeID {
		return Watermarks{}, fmt.Errorf("%w: the peer calls itself %q, which is this node",
			ErrRoleMismatch, r.cfg.NodeID)
	}

	conn.SetWriteDeadline(time.Now().Add(r.cfg.PeerStaleAfter))
	if err := writeFrame(conn, Frame{
		Type:     FrameHello,
		Protocol: Protocol,
		Watermarks: &Watermarks{
			NodeID:     r.cfg.NodeID,
			Role:       r.cfg.Role,
			AppliedSeq: r.Seq(),
			AckedSeq:   r.AckedSeq(),
		},
	}); err != nil {
		return Watermarks{}, err
	}
	return *frame.Watermarks, nil
}

func (r *Replicator) handleFrame(f Frame) error {
	// Any frame at all is evidence the peer is alive, and the session's own
	// liveness check is what decides whether this node may promise addresses.
	// A confirmation carries no identity, so the clock is advanced without
	// overwriting what the handshake established.
	r.link.touch()
	switch f.Type {
	case FrameApplied:
		r.noteConfirmed(f.Seq)
		return nil
	case FrameHello:
		// The mirror restarted its side without closing. Its watermarks are
		// what it says they are, and a fresh snapshot is how the pair
		// re-establishes a shared view.
		return fmt.Errorf("ha: the mirror re-handshook on an open connection")
	case FrameSnapshot, FrameOps:
		return fmt.Errorf("ha: the mirror sent a %q frame, which only a primary sends", f.Type)
	default:
		return fmt.Errorf("ha: unexpected %q frame from the mirror", f.Type)
	}
}

// noteConfirmed records that the mirror has durably applied everything up to
// seq, and wakes anybody waiting on that.
//
// The degraded check is deliberately outside the "did the watermark move"
// branch. A node that has never handed out a sequence has an acknowledged
// watermark of zero, and a confirmation of zero is not progress -- but it is
// still the fact that the second copy is present, and it has to be able to
// clear a marker that was set while it was absent.
func (r *Replicator) noteConfirmed(seq int64) {
	r.mu.Lock()
	var advanced chan struct{}
	if seq > r.acked {
		r.acked = seq
		advanced = r.advance
		r.advance = make(chan struct{})
	}
	// The reason to be degraded is a missing second copy. When the mirror has
	// caught up to everything this node has handed out, that reason is gone,
	// and a marker that outlives its reason is how a deployment stays degraded
	// forever after one bad afternoon.
	clearDegraded := r.degraded && seq >= r.seq
	if clearDegraded {
		r.degraded = false
	}
	r.mu.Unlock()

	if advanced == nil && !clearDegraded {
		return
	}
	if advanced != nil {
		close(advanced)
		if _, err := r.store.Exec(`
			INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, metaAckedSeq, fmt.Sprintf("%d", seq)); err != nil {
			slog.Warn("HA: could not persist the acknowledged watermark", "seq", seq, "error", err)
		}
	}
	if clearDegraded {
		if err := r.clearDegradedMarker(); err != nil {
			slog.Warn("HA: could not clear the degraded marker", "error", err)
		} else {
			slog.Info("HA: the mirror has caught up; this node is redundant again",
				"node_id", r.cfg.NodeID, "acked_seq", seq)
		}
	}
}

func (r *Replicator) clearDegradedMarker() error {
	if _, err := r.store.Exec(`DELETE FROM dataplane_meta WHERE key IN (?, ?)`, metaDegraded, metaDegradedReason); err != nil {
		return err
	}
	return nil
}

// watchOperatorDecisions adopts the single-copy marker an operator has changed
// while this process was running.
//
// Without it, `goddi ha degrade` would only take effect at the next restart --
// and the situation it exists for is the one in which the node must keep
// serving: the peer is gone and clients are trying to renew. A restart there is
// the outage the permission was meant to avoid.
//
// The interval is the heartbeat. The marker is read from a table with a handful
// of rows on a store this process already owns, so the cost is a lookup a
// second, and the delay it bounds is a fraction of the confirmation timeout the
// clients already tolerate.
func (r *Replicator) watchOperatorDecisions(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.reloadDegraded(); err != nil {
				slog.Warn("HA: could not read the operator's single-copy decision; the last one stands",
					"error", err)
			}
		}
	}
}

// reloadDegraded adopts whatever the marker row now says.
//
// The two directions are not symmetric, and neither is dangerous. The marker is
// created by an operator and by nothing else, so adopting its presence can only
// follow a human decision -- the node starts acknowledging bindings again, and
// says so in the log. Adopting its absence returns the node to withholding
// acknowledgements, which is the conservative direction whatever the reason.
// A read that fails changes nothing: the last known decision stands rather than
// being replaced by a guess.
func (r *Replicator) reloadDegraded() error {
	marker, err := r.store.Meta(metaDegraded)
	if err != nil {
		return err
	}
	reason, err := r.store.Meta(metaDegradedReason)
	if err != nil {
		return err
	}
	degraded := marker != ""

	r.mu.Lock()
	changed := degraded != r.degraded
	r.degraded = degraded
	if degraded {
		r.degradedAt, _ = time.Parse(time.RFC3339, marker)
	}
	r.mu.Unlock()

	if !changed {
		return nil
	}
	if degraded {
		slog.Warn("HA: an operator has approved running without a second copy; this node acknowledges bindings again",
			"node_id", r.cfg.NodeID, "reason", reason, "seq", r.Seq(), "acked_seq", r.AckedSeq())
		return nil
	}
	slog.Warn("HA: the single-copy approval was withdrawn; this node withholds acknowledgements until its mirror is current",
		"node_id", r.cfg.NodeID, "seq", r.Seq(), "acked_seq", r.AckedSeq())
	return nil
}

// sendWork emits one frame: the next batch of changes if there are any, and a
// request for a confirmation if there are not.
func (r *Replicator) sendWork(conn net.Conn) error {
	r.mu.Lock()
	var batch []pendingOp
	if len(r.pending) > 0 {
		n := len(r.pending)
		if n > opsBatchLimit {
			n = opsBatchLimit
		}
		batch = append([]pendingOp(nil), r.pending[:n]...)
		r.pending = append(r.pending[:0], r.pending[n:]...)
	}
	pingDue := time.Since(r.lastSent) >= r.cfg.HeartbeatInterval
	seq := r.seq
	r.mu.Unlock()

	conn.SetWriteDeadline(time.Now().Add(r.cfg.PeerStaleAfter))

	if len(batch) > 0 {
		rows := make([]LeaseRow, len(batch))
		for i, op := range batch {
			rows[i] = op.row
		}
		if err := writeFrame(conn, Frame{Type: FrameOps, Seq: batch[len(batch)-1].seq, Leases: rows}); err != nil {
			return err
		}
		r.markSent()
		return nil
	}
	if pingDue {
		// The sequence is carried so the mirror's log line and the primary's
		// can be read against each other; the reply is the same either way.
		if err := writeFrame(conn, Frame{Type: FramePing, Seq: seq}); err != nil {
			return err
		}
		r.markSent()
	}
	return nil
}

func (r *Replicator) markSent() {
	r.mu.Lock()
	r.lastSent = time.Now()
	r.mu.Unlock()
}

func (r *Replicator) prunePending(seq int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keep := r.pending[:0]
	for _, op := range r.pending {
		if op.seq > seq {
			keep = append(keep, op)
		}
	}
	r.pending = keep
}

// readSnapshot materialises the whole lease table and the sequence that
// describes it, in one read transaction.
//
// The cursor is fully drained and closed before the sequence is read. SQLite
// runs on a single connection per handle, and issuing a statement while a
// cursor is open waits on the connection that cursor holds -- with no error and
// no timeout.
func (r *Replicator) readSnapshot(ctx context.Context) (int64, []LeaseRow, error) {
	tx, err := r.store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, nil, fmt.Errorf("ha: opening the snapshot read: %w", err)
	}
	defer tx.Rollback()

	cols := ""
	for i, c := range leaseColumns {
		if i > 0 {
			cols += ", "
		}
		cols += c
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+cols+" FROM dhcp_leases ORDER BY id")
	if err != nil {
		return 0, nil, fmt.Errorf("ha: reading the lease table: %w", err)
	}
	var out []LeaseRow
	for rows.Next() {
		var l LeaseRow
		if err := rows.Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &l.Hostname,
			&l.ClientID, &l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation); err != nil {
			rows.Close()
			return 0, nil, fmt.Errorf("ha: scanning a lease for the snapshot: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, nil, fmt.Errorf("ha: reading the lease table: %w", err)
	}
	rows.Close()

	var raw string
	err = tx.QueryRowContext(ctx, `SELECT value FROM dataplane_meta WHERE key = ?`, metaSeq).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return 0, nil, fmt.Errorf("ha: reading the sequence for the snapshot: %w", err)
	}
	var seq int64
	if raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &seq); err != nil {
			return 0, nil, fmt.Errorf("ha: the sequence %q is not a number: %w", raw, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, fmt.Errorf("ha: closing the snapshot read: %w", err)
	}
	return seq, out, nil
}
