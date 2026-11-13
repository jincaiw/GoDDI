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
		return false, nil
	}
	rows, err := ApplySnapshot(ctx, p.db, snap, p.specs)
	if err != nil {
		return false, err
	}
	p.lastVer = snap.Version
	p.haveState = true
	if err := p.saveState(ctx, snap.Version, rows, ""); err != nil {
		return false, err
	}
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

func (p *Puller) saveState(ctx context.Context, version int64, rows int, lastErr string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO config_repl_state (source_node, applied_version, applied_rows, last_error, synced_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(source_node) DO UPDATE SET
			applied_version = excluded.applied_version,
			applied_rows    = excluded.applied_rows,
			last_error      = excluded.last_error,
			synced_at       = excluded.synced_at`,
		p.cfg.SourceURL, version, rows, lastErr)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil // state table not present (older DB); replication still works, just no watermark
		}
		return fmt.Errorf("configrepl: recording repl state: %w", err)
	}
	return nil
}

func (p *Puller) recordError(ctx context.Context, cause error) {
	_ = p.saveState(ctx, p.lastVer, 0, cause.Error())
}
