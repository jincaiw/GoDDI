package ha

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jasonwa/goddi/internal/dataplane"
)

// Meta keys this package owns in dataplane_meta.
const (
	// metaSeq is the primary's sequence counter: the highest sequence it has
	// ever handed out. Monotonic across restarts, which is the property the
	// takeover arithmetic needs.
	metaSeq = "ha_seq"
	// metaAckedSeq is the primary's record of what its mirror reported. It is
	// persisted because it is the fact recovery is reasoned about with, and a
	// restart must not turn it into a guess.
	metaAckedSeq = "ha_acked_seq"
	// metaAppliedSeq is the standby's record of how far its mirror has been
	// brought. This is the only fact a standby can offer as evidence, so it
	// is written in the same transaction as the rows it describes: a
	// watermark that can be ahead of its own content is worse than none.
	metaAppliedSeq = "ha_applied_seq"
	// metaDegraded marks an operator's explicit decision to run without a
	// second copy. Nothing in this package sets it; see Operator.Degrade.
	// There is exactly one writer, and it is not on any serving path.
	metaDegraded = "ha_degraded"
	// metaDegradedReason records what the operator said they were doing. It is
	// named for its content rather than for a moment: it holds a sentence, and
	// the decision's own timestamp lives in metaDegraded.
	metaDegradedReason = "ha_degraded_reason"
)

// applyBatchLimit bounds one standby transaction.
//
// The mirror is brought up to date in transactions of a bounded size so that a
// large catch-up -- a snapshot after a long outage -- does not become one
// transaction holding one connection for its whole duration.
const applyBatchLimit = 512

// linkHealth is the shared view of whether the peer is currently reachable and
// how recently it said anything. Both sides keep one; only the primary acts on
// it, because only the primary has promises to withhold.
type linkHealth struct {
	mu       sync.Mutex
	up       bool
	lastSeen time.Time
	peer     Watermarks
}

func (h *linkHealth) markUp(w Watermarks) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.up = true
	h.lastSeen = time.Now()
	h.peer = w
}

func (h *linkHealth) markHeard(w Watermarks) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSeen = time.Now()
	h.peer = w
}

// touch records that the peer said something, without changing what is known
// about it.
//
// It exists because most frames carry no watermarks. A confirmation carries a
// sequence and nothing else, and treating that as "the peer is now an empty
// struct" would erase the identity the handshake established -- while not
// touching the clock at all would let a healthy, quiet pair be torn down on
// every staleness interval.
func (h *linkHealth) touch() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSeen = time.Now()
}

func (h *linkHealth) markDown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.up = false
}

// fresh reports whether the peer is connected and has said something within
// the staleness bound.
func (h *linkHealth) fresh(stale time.Duration) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.up && time.Since(h.lastSeen) <= stale
}

func (h *linkHealth) snapshot() (bool, time.Time, Watermarks) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.up, h.lastSeen, h.peer
}

// Mirror is the standby half of the pair.
//
// It has one job, and the job description is deliberately narrow: keep a copy
// of the primary's lease facts that is good enough to prove, after the primary
// is gone, what was promised. It serves no client, authors no lease, and never
// decides on its own to become the primary.
type Mirror struct {
	cfg   Config
	store *dataplane.Store
	link  linkHealth

	mu      sync.Mutex
	applied int64
	fenced  bool
}

// NewMirror opens the standby side against a lease store.
func NewMirror(cfg Config, store *dataplane.Store) (*Mirror, error) {
	if !cfg.IsStandby() {
		return nil, fmt.Errorf("ha: a mirror must be configured with role=%s, got %q", "standby", cfg.Role)
	}
	applied, err := readWatermark(store.DB, metaAppliedSeq)
	if err != nil {
		return nil, fmt.Errorf("ha: reading the applied watermark: %w", err)
	}
	return &Mirror{cfg: cfg, store: store, applied: applied}, nil
}

// AppliedSeq reports how far the mirror has been brought.
func (m *Mirror) AppliedSeq() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applied
}

// State reports what this node is. A standby is a standby whether or not the
// primary is currently reachable: losing sight of the primary is not a reason
// to start serving, and this node has nothing to withhold, so its state does
// not change with the link.
func (m *Mirror) State() State {
	m.mu.Lock()
	fenced := m.fenced
	m.mu.Unlock()
	if fenced {
		return StateFenced
	}
	return StateStandby
}

// Fence marks this node as one that must not answer anything.
//
// It is set by an operator decision (ADR 0003 decision 7), not by the code:
// a standby that has been superseded by a takeover must be stopped explicitly,
// because the reason to stop it is a fact about the other node that this one
// cannot observe.
func (m *Mirror) Fence() error {
	m.mu.Lock()
	m.fenced = true
	m.mu.Unlock()
	return nil
}

// PeerWatermarks reports the last handshake the mirror saw. It is a read-only
// view for the console and the probe.
func (m *Mirror) PeerWatermarks() Watermarks {
	_, _, w := m.link.snapshot()
	return w
}

// LinkSnapshot reports whether the primary is currently reachable, and when it
// was last heard from.
//
// It exists because a standby's readiness is a question about its link: it
// serves nobody either way, so the only thing that can go wrong is losing sight
// of the node whose promises it holds. Without this the process has no way to
// answer that on its own.
func (m *Mirror) LinkSnapshot() (up bool, lastSeen time.Time) {
	up, lastSeen, _ = m.link.snapshot()
	return up, lastSeen
}

// Run dials the primary and keeps the mirror current until ctx is cancelled.
//
// The standby is the side that dials. A standby that restarts has to
// re-establish the mirror to be useful at all, and a dial loop with backoff is
// the only shape in which its own restart is recovered from without an operator.
func (m *Mirror) Run(ctx context.Context) error {
	backoff := 250 * time.Millisecond
	const maxBackoff = 5 * time.Second

	for {
		if ctx.Err() != nil {
			return nil
		}
		err := m.session(ctx)
		m.link.markDown()
		if ctx.Err() != nil {
			return nil
		}
		slog.Warn("HA: the mirror lost its primary; reconnecting",
			"peer", m.cfg.PeerAddress, "applied_seq", m.AppliedSeq(), "error", err)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// session is one connection's lifetime: dial, handshake, then read frames until
// the link breaks.
func (m *Mirror) session(ctx context.Context) error {
	d := net.Dialer{Timeout: m.cfg.PeerStaleAfter}
	conn, err := d.DialContext(ctx, "tcp", m.cfg.PeerAddress)
	if err != nil {
		return fmt.Errorf("dialling %s: %w", m.cfg.PeerAddress, err)
	}
	defer conn.Close()

	// The connection is closed when ctx is cancelled, which is what unblocks
	// the reads below. Without it a cancellation would wait for the peer to
	// say something.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	if err := m.handshake(conn); err != nil {
		return err
	}
	slog.Info("HA: mirror connected to its primary",
		"peer", m.cfg.PeerAddress, "applied_seq", m.AppliedSeq())

	for {
		conn.SetReadDeadline(time.Now().Add(m.cfg.PeerStaleAfter))
		frame, err := readFrame(conn, maxFrameBytes)
		if err != nil {
			return err
		}

		switch frame.Type {
		case FrameSnapshot:
			if err := m.applySnapshot(ctx, frame.Seq, frame.Leases); err != nil {
				return err
			}
			slog.Info("HA: mirror rebuilt from a snapshot", "seq", frame.Seq, "rows", len(frame.Leases))
			if err := m.confirm(conn, frame.Seq); err != nil {
				return err
			}
		case FrameOps:
			if err := m.applyOps(ctx, frame.Seq, frame.Leases); err != nil {
				return err
			}
			if err := m.confirm(conn, frame.Seq); err != nil {
				return err
			}
		case FramePing:
			// A ping is answered with the watermark rather than a bare
			// acknowledgement: it is the same liveness signal and it carries
			// the fact the other side actually wants.
			//
			// It also carries the primary's own counter, and this is the only
			// frame that reports progress no row here reflects. A takeover
			// reads it back as "how far the primary had got when it was last
			// heard from", so it is recorded even though nothing is applied.
			if err := m.rememberPeer(frame.Seq); err != nil {
				return err
			}
			if err := m.confirm(conn, m.AppliedSeq()); err != nil {
				return err
			}
		case FrameHello:
			// A second handshake on an established connection means the peer
			// restarted its side without closing. Re-handshaking is the
			// correct response and re-sending a snapshot is the safe way to
			// establish where the other side actually is.
			if err := m.handshake(conn); err != nil {
				return err
			}
		default:
			return fmt.Errorf("ha: unexpected %q frame from the primary", frame.Type)
		}
	}
}

func (m *Mirror) handshake(conn net.Conn) error {
	conn.SetWriteDeadline(time.Now().Add(m.cfg.PeerStaleAfter))
	if err := writeFrame(conn, Frame{
		Type:     FrameHello,
		Protocol: Protocol,
		Token:    m.cfg.PeerToken,
		Watermarks: &Watermarks{
			NodeID:     m.cfg.NodeID,
			Role:       m.cfg.Role,
			AppliedSeq: m.AppliedSeq(),
			AckedSeq:   0,
		},
	}); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(m.cfg.PeerStaleAfter))
	frame, err := readFrame(conn, maxFrameBytes)
	if err != nil {
		return fmt.Errorf("reading the primary's handshake: %w", err)
	}
	if frame.Type != FrameHello || frame.Watermarks == nil {
		return fmt.Errorf("ha: expected a handshake, got a %q frame", frame.Type)
	}
	if frame.Protocol != Protocol {
		return fmt.Errorf("%w: peer speaks v%d, this node speaks v%d",
			ErrProtocolMismatch, frame.Protocol, Protocol)
	}
	if frame.Watermarks.Role != "primary" {
		return fmt.Errorf("%w: a standby mirrors a primary, the peer says %q",
			ErrRoleMismatch, frame.Watermarks.Role)
	}
	// The handshake is where a whole number arrives rather than a delta: the
	// primary states its current counter, which is the highest figure this node
	// will ever learn about a primary it then loses contact with.
	if err := m.rememberPeer(frame.Watermarks.AppliedSeq); err != nil {
		return err
	}
	m.link.markUp(*frame.Watermarks)
	return nil
}

func (m *Mirror) confirm(conn net.Conn, seq int64) error {
	conn.SetWriteDeadline(time.Now().Add(m.cfg.PeerStaleAfter))
	return writeFrame(conn, Frame{Type: FrameApplied, Seq: m.AppliedSeq()})
}

// rememberPeer records what the primary said it had reached, and when.
//
// It is used on the frames that carry no rows -- a ping reports the primary's
// counter and nothing else -- and is folded into the apply transactions for the
// frames that do, so that a busy pair does not pay for it twice.
func (m *Mirror) rememberPeer(seq int64) error {
	return writePeerWatermark(m.store.DB, seq)
}

// metaExec is the part of database/sql both a pool and a transaction offer. The
// two writers below take it so that the same statement cannot end up spelled
// two ways -- and, more importantly, so that a caller inside a transaction
// cannot accidentally issue a statement on the store's single connection while
// that transaction holds it. That mistake does not fail; it waits forever.
type metaExec interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// writePeerWatermark records the peer's reported progress and the moment it was
// heard from.
//
// The sequence is written as a maximum rather than an assignment: a frame
// carries whatever the primary had reached when it was built, and a snapshot
// taken after a reconnect can report a figure lower than a ping that overtook
// it on the wire. Letting a lower number overwrite a higher one would make a
// shortfall disappear, and a shortfall that can disappear is not evidence.
//
// The timestamp is written unconditionally, because it answers a different
// question -- "is this peer still answering" -- and the takeover gate reads it
// as the only thing that distinguishes a dead primary from a live one.
//
// It is written with sub-second precision, unlike the other instants this
// package records. The others are dates an operator reads; this one is an input
// to a comparison against a window measured in seconds, and RFC3339's whole
// seconds would put up to a second of error into the one calculation that
// decides whether a live primary can be taken over from. time.Parse with the
// RFC3339 layout reads the fraction back, so the reader needs no special case.
func writePeerWatermark(ex metaExec, seq int64) error {
	var raw string
	err := ex.QueryRow(`SELECT value FROM dataplane_meta WHERE key = ?`, metaPeerSeq).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("ha: reading the peer's watermark: %w", err)
	}
	var seen int64
	if strings.TrimSpace(raw) != "" {
		if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &seen); err != nil {
			return fmt.Errorf("ha: the %s watermark %q is not a number: %w", metaPeerSeq, raw, err)
		}
	}
	if seq > seen {
		if _, err := ex.Exec(`
			INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			metaPeerSeq, fmt.Sprintf("%d", seq)); err != nil {
			return fmt.Errorf("ha: writing the peer's watermark: %w", err)
		}
	}
	if _, err := ex.Exec(`
		INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		metaPeerSeqAt, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("ha: writing the moment the peer was heard: %w", err)
	}
	return nil
}

// applySnapshot replaces the mirror with the primary's rows as of one sequence.
//
// The whole replacement is one transaction, including the watermark. Reading
// the watermark and the rows from two transactions would allow a crash between
// them to leave a mirror that claims to be ahead of what it holds -- and that
// claim is the evidence a takeover is decided on.
func (m *Mirror) applySnapshot(ctx context.Context, seq int64, rows []LeaseRow) error {
	if seq < m.AppliedSeq() {
		// A snapshot older than what is already here is not an update. Taking
		// it would move the mirror backwards, which is the one direction it
		// must never move.
		return nil
	}
	tx, err := m.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ha: opening the snapshot transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM dhcp_leases"); err != nil {
		return fmt.Errorf("ha: clearing the mirror: %w", err)
	}
	for start := 0; start < len(rows); start += applyBatchLimit {
		end := start + applyBatchLimit
		if end > len(rows) {
			end = len(rows)
		}
		if err := upsertLeases(ctx, tx, rows[start:end]); err != nil {
			return err
		}
	}
	if err := setWatermark(ctx, tx, metaAppliedSeq, seq); err != nil {
		return err
	}
	if err := writePeerWatermark(tx, seq); err != nil {
		return err
	}
	// The mirror owns no lease, so nothing it holds is owed to the control
	// database. The dirty triggers fire on these writes the same as on a
	// primary's, and leaving their output here would have the standby push a
	// replica of somebody else's leases upward as if they were its own.
	if _, err := tx.ExecContext(ctx, "DELETE FROM dhcp_lease_dirty"); err != nil {
		return fmt.Errorf("ha: discarding the mirror's upward queue: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ha: committing the snapshot: %w", err)
	}
	m.mu.Lock()
	m.applied = seq
	m.mu.Unlock()
	return nil
}

// applyOps adds a batch of lease changes.
//
// Upserts, not replacements: the batch carries the rows the primary changed,
// and a row the primary did not send is one it did not change. There is no
// delete here because the primary never deletes a lease row -- it marks it
// expired or released -- so there is nothing for a deletion to say.
func (m *Mirror) applyOps(ctx context.Context, seq int64, rows []LeaseRow) error {
	if len(rows) == 0 {
		if seq > m.AppliedSeq() {
			return m.recordWatermark(ctx, seq)
		}
		return nil
	}
	if seq < m.AppliedSeq() {
		return nil
	}
	tx, err := m.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ha: opening the apply transaction: %w", err)
	}
	defer tx.Rollback()

	for start := 0; start < len(rows); start += applyBatchLimit {
		end := start + applyBatchLimit
		if end > len(rows) {
			end = len(rows)
		}
		if err := upsertLeases(ctx, tx, rows[start:end]); err != nil {
			return err
		}
	}
	if err := setWatermark(ctx, tx, metaAppliedSeq, seq); err != nil {
		return err
	}
	if err := writePeerWatermark(tx, seq); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM dhcp_lease_dirty"); err != nil {
		return fmt.Errorf("ha: discarding the mirror's upward queue: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ha: committing an apply batch: %w", err)
	}
	m.mu.Lock()
	m.applied = seq
	m.mu.Unlock()
	return nil
}

// recordWatermark advances the watermark without rows, which is what a batch
// that carried nothing but a sequence means.
func (m *Mirror) recordWatermark(ctx context.Context, seq int64) error {
	tx, err := m.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ha: opening the watermark transaction: %w", err)
	}
	defer tx.Rollback()
	if err := setWatermark(ctx, tx, metaAppliedSeq, seq); err != nil {
		return err
	}
	if err := writePeerWatermark(tx, seq); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ha: committing the watermark: %w", err)
	}
	m.mu.Lock()
	m.applied = seq
	m.mu.Unlock()
	return nil
}

// upsertLeases writes rows into the mirror.
//
// A unique-index violation here is not swallowed. The index forbids two rows
// holding the same address in one scope, the primary's own index forbids the
// same thing, and the rows arrive in the order the primary wrote them -- so a
// collision means this mirror has diverged from the primary and no longer
// describes a state the primary ever held. The error tears the connection down
// and the next reconnect replaces the mirror wholesale, which is the only
// repair whose result can be trusted.
func upsertLeases(ctx context.Context, tx *sql.Tx, rows []LeaseRow) error {
	stmt := `INSERT INTO dhcp_leases (` + strings.Join(leaseColumns, ", ") + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scope_id    = excluded.scope_id,
			ip_address  = excluded.ip_address,
			mac_address = excluded.mac_address,
			hostname    = excluded.hostname,
			client_id   = excluded.client_id,
			lease_start = excluded.lease_start,
			lease_end   = excluded.lease_end,
			status      = excluded.status,
			last_seen   = excluded.last_seen,
			generation  = excluded.generation`
	for _, row := range rows {
		if row.ID == "" {
			return fmt.Errorf("ha: a lease row arrived without an id; it cannot be identified or repaired")
		}
		if _, err := tx.ExecContext(ctx, stmt, row.values()...); err != nil {
			return fmt.Errorf("ha: mirroring lease %s: %w", row.ID, err)
		}
	}
	return nil
}

func readWatermark(db *sql.DB, key string) (int64, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM dataplane_meta WHERE key = ?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var n int64
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &n); err != nil {
		// A watermark that cannot be read is not zero. Zero is "this node has
		// nothing", which is the reading that would let a mirror be treated as
		// current when it is not.
		return 0, fmt.Errorf("ha: the %s watermark %q is not a number: %w", key, raw, err)
	}
	return n, nil
}

func setWatermark(ctx context.Context, tx *sql.Tx, key string, value int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO dataplane_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, fmt.Sprintf("%d", value))
	if err != nil {
		return fmt.Errorf("ha: writing the %s watermark: %w", key, err)
	}
	return nil
}
