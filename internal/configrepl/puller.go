package configrepl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/dataplane"
)

// PullerConfig configures the follower side of ADR-0013 configuration
// replication.
type PullerConfig struct {
	// SourceURL is the author's base URL, e.g. "http://10.77.3.4:6080".
	SourceURL string
	// Token is the shared secret presented as X-Goddi-Replication-Token.
	Token string
	// Interval is the pull cadence. 0 means 5s.
	Interval time.Duration
}

// Puller periodically pulls the author's configuration snapshot and applies it
// to this follower's control database.
//
// A snapshot is a full replace and is idempotent, so the puller does not track
// gaps: it compares the author's version with the last applied version and, when
// newer, replaces every replicated table in one transaction. That is simpler and
// strictly safer than replaying entries -- a missed entry cannot be missed
// because the next pull carries the whole table.
type Puller struct {
	db        *sql.DB
	client    *http.Client
	cfg       PullerConfig
	specs     []dataplane.ConfigTableSpec
	onApplied func()
	lastVer   int64
	haveState bool
}

// NewPuller builds a follower puller against the control database. onApplied
// (optional) runs after a successful apply so the caller can wake the
// data-plane sync immediately.
func NewPuller(db *sql.DB, cfg PullerConfig, specs []dataplane.ConfigTableSpec, onApplied func()) *Puller {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	return &Puller{
		db:        db,
		client:    &http.Client{Timeout: 10 * time.Second},
		cfg:       cfg,
		specs:     specs,
		onApplied: onApplied,
	}
}

// Run pulls until ctx is cancelled.
func (p *Puller) Run(ctx context.Context) error {
	if strings.TrimSpace(p.cfg.SourceURL) == "" {
		return fmt.Errorf("configrepl: puller has no source URL")
	}
	if err := p.loadState(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		// Pull once immediately, then on each tick.
		if _, err := p.pullOnce(ctx); err != nil && ctx.Err() == nil {
			// Transient by design: the author may be down. Recorded, not fatal.
			p.recordError(ctx, err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// PullOnce performs one pull, exported for tests and for a one-shot manual sync.
func (p *Puller) PullOnce(ctx context.Context) (bool, error) { return p.pullOnce(ctx) }

func (p *Puller) pullOnce(ctx context.Context) (bool, error) {
	snap, err := p.fetch(ctx)
	if err != nil {
		return false, err
	}
	// Version is a content fingerprint, not a monotonic counter, so "unchanged"
	// is equality. A follower with no prior state always applies once.
	if p.haveState && snap.Version == p.lastVer {
		// A successful pull is a liveness signal even when the configuration is
		// unchanged. config_repl_state.synced_at is the console's replication
		// health clock: if it only moved on an actual apply, a perfectly healthy
		// idle follower would be reported as stale a few seconds later.
		if err := p.touchState(ctx, snap.Version); err != nil {
			return false, err
		}
		return false, nil
	}
	// Config rows AND the applied-version watermark must land in one commit.
	// Otherwise a crash or a state-write failure leaves the in-memory lastVer
	// ahead of the database, and every future pull is treated as a no-op even
	// though the configuration never applied.
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("configrepl: begin apply: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := ApplySnapshotTx(ctx, tx, snap, p.specs)
	if err != nil {
		return false, err
	}
	if err := saveStateTx(ctx, tx, p.cfg.SourceURL, snap.Version, rows, ""); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("configrepl: commit apply: %w", err)
	}
	// Only now advance the in-memory watermark; a failed state write keeps
	// the previous version so the next pull retries the apply.
	p.lastVer = snap.Version
	p.haveState = true
	if p.onApplied != nil {
		p.onApplied()
	}
	return true, nil
}

func (p *Puller) fetch(ctx context.Context) (Snapshot, error) {
	url := strings.TrimRight(p.cfg.SourceURL, "/") + "/internal/cluster/config/snapshot"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Snapshot{}, err
	}
	req.Header.Set("X-Goddi-Replication-Token", p.cfg.Token)
	resp, err := p.client.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("configrepl: fetching snapshot: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Snapshot{}, fmt.Errorf("configrepl: snapshot endpoint %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return Snapshot{}, fmt.Errorf("configrepl: reading snapshot body: %w", err)
	}
	return DecodeSnapshotJSON(body)
}

func (p *Puller) loadState(ctx context.Context) error {
	var v int64
	err := p.db.QueryRowContext(ctx, `SELECT applied_version FROM config_repl_state WHERE source_node = ?`, p.cfg.SourceURL).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "no such") {
			return nil // no prior state: start from version 0
		}
		return fmt.Errorf("configrepl: reading repl state: %w", err)
	}
	p.lastVer = v
	p.haveState = true
	return nil
}

// stateWriter is satisfied by both *sql.DB and *sql.Tx so the watermark can
// be written in the same transaction as the configuration apply.
type stateWriter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (p *Puller) saveState(ctx context.Context, version int64, rows int, lastErr string) error {
	return saveStateTx(ctx, p.db, p.cfg.SourceURL, version, rows, lastErr)
}

func saveStateTx(ctx context.Context, q stateWriter, source string, version int64, rows int, lastErr string) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO config_repl_state (source_node, applied_version, applied_rows, last_error, synced_at, expected_version)
		VALUES (?, ?, ?, ?, datetime('now'), ?)
		ON CONFLICT(source_node) DO UPDATE SET
			applied_version = excluded.applied_version,
			applied_rows    = excluded.applied_rows,
			last_error      = excluded.last_error,
			synced_at       = excluded.synced_at,
			expected_version = excluded.expected_version`,
		source, version, rows, lastErr, version)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil // state table not present (older DB); replication still works, just no watermark
		}
		return fmt.Errorf("configrepl: recording repl state: %w", err)
	}
	return nil
}

// touchState records a successful pull without changing the applied watermark.
// A successful fetch also clears a previous transient error (for example an
// author that was briefly unreachable); a failed APPLY is still retried because
// its version differs from lastVer, so it cannot be hidden by this path.
func (p *Puller) touchState(ctx context.Context, expectedVersion int64) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE config_repl_state SET synced_at = datetime('now'), last_error = '', expected_version = ?
		WHERE source_node = ?`, expectedVersion, p.cfg.SourceURL)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil // older DB without the state table; replication still works
		}
		return fmt.Errorf("configrepl: touching repl state: %w", err)
	}
	return nil
}

func (p *Puller) recordError(ctx context.Context, cause error) {
	// An error is not a new apply: preserve the last successful applied_rows
	// watermark instead of zeroing it, or a healthy replica would look like it
	// never applied anything after a transient author outage.
	res, err := p.db.ExecContext(ctx,
		`UPDATE config_repl_state SET last_error = ? WHERE source_node = ?`,
		cause.Error(), p.cfg.SourceURL)
	if err == nil {
		if n, e := res.RowsAffected(); e == nil && n > 0 {
			return
		}
	}
	// No state row yet (or an older DB): record a full state row so the
	// console still sees an unknown/failed replica instead of nothing.
	_ = p.saveState(ctx, p.lastVer, 0, cause.Error())
}
