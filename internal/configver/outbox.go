package configver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Release worker tuning.
//
// The values mirror the DHCP DNS-event queue (internal/dhcp/dns_outbox.go) on
// purpose: the two queues have the same failure shape -- a durable event that
// must reach in-memory state -- so they should not drift into different
// retry behaviour that operators then have to learn twice.
const (
	// ReleaseRetryBase is the first backoff step.
	ReleaseRetryBase = 2 * time.Second
	// ReleaseRetryMax caps the backoff so a long outage does not push the
	// next attempt past the point where the process has restarted anyway.
	ReleaseRetryMax = 5 * time.Minute
	// ReleaseMaxAttempts is where a release stops retrying and is surfaced as
	// failed. The row is kept, never deleted: an operator must be able to see
	// that a configuration is stuck, and the last applied revision stays LKG.
	ReleaseMaxAttempts = 8
	// ReleaseBatchSize bounds one drain pass.
	ReleaseBatchSize = 64
	// ReleaseInterval is the worker's idle poll interval.
	ReleaseInterval = 500 * time.Millisecond
)

// Outbox statuses. pending -> applied | failed.
const (
	ReleasePending = "pending"
	ReleaseApplied = "applied"
	ReleaseFailed  = "failed"
)

// releaseSelect is the shared column list, kept in one place so the scanners
// cannot drift out of step with it.
const releaseSelect = `SELECT id, revision_id, resource_type, resource_id, revision,
	status, attempts, resource_applied, applied_generation, last_error,
	created_at, COALESCE(applied_at, '')
	FROM config_release_outbox`

// Release is one queued data-plane notification.
type Release struct {
	ID                int64        `json:"id"`
	RevisionID        string       `json:"revision_id"`
	ResourceType      ResourceType `json:"resource_type"`
	ResourceID        string       `json:"resource_id"`
	Revision          int64        `json:"revision"`
	Status            string       `json:"status"`
	Attempts          int          `json:"attempts"`
	ResourceApplied   bool         `json:"resource_applied"`
	AppliedGeneration int64        `json:"applied_generation"`
	LastError         string       `json:"last_error,omitempty"`
	CreatedAt         string       `json:"created_at"`
	AppliedAt         string       `json:"applied_at,omitempty"`
}

// DrainOutbox releases at most limit due events and reports how many succeeded.
//
// Entries are drained in id order so that, when several revisions of the same
// resource are queued, the newest one is applied last and therefore wins. A
// single failing entry does not stop the pass: one broken resource must not
// block every other resource's release.
func (s *Service) DrainOutbox(limit int) (int, error) {
	if limit <= 0 {
		limit = ReleaseBatchSize
	}

	rows, err := s.db.Query(`SELECT id FROM config_release_outbox
		WHERE status = ? AND next_attempt_at <= datetime('now')
		ORDER BY id ASC LIMIT ?`, ReleasePending, limit)
	if err != nil {
		return 0, fmt.Errorf("configver: read outbox: %w", err)
	}

	// Materialise and close before doing any further query. The pool is capped
	// at one connection for SQLite; issuing another statement while this cursor
	// is open would block forever instead of failing.
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	applied := 0
	var firstErr error
	for _, id := range ids {
		ok, err := s.releaseOne(id)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if ok {
			applied++
		}
	}
	return applied, firstErr
}

// releaseOne applies and announces one queued release.
//
// Order matters: the resource is written first and announced second. If the
// announcement fails after the write committed, the retry re-applies the same
// content (Apply is idempotent) and announces again, so the pair converges.
// Announcing first would let the data plane load content the database does not
// have, which a restart would then silently undo.
func (s *Service) releaseOne(id int64) (bool, error) {
	rel, err := s.loadRelease(id)
	if err != nil {
		return false, err
	}
	if rel == nil || rel.Status != ReleasePending {
		return false, nil
	}

	// A missing revision can never be released, so it is failed rather than
	// retried forever.
	rev, err := s.Get(rel.RevisionID)
	if err != nil {
		return false, s.recordFailure(rel, false, fmt.Errorf("configver: load revision for release: %w", err))
	}

	adapter, ok := s.adapters[rel.ResourceType]
	if !ok {
		return false, s.recordFailure(rel, false, fmt.Errorf("%w: %s", ErrNoAdapter, rel.ResourceType))
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("configver: begin release: %w", err)
	}
	if err := adapter.Apply(tx, rel.ResourceID, rev.Content); err != nil {
		_ = tx.Rollback()
		return false, s.recordFailure(rel, false, fmt.Errorf("apply: %w", err))
	}
	// The "the resource was written" flag is committed together with the write
	// it describes. Separate statements would allow a crash between them to
	// leave a written resource whose release still claims nothing happened.
	if _, err := tx.Exec(
		`UPDATE config_release_outbox SET resource_applied = 1 WHERE id = ?`, rel.ID); err != nil {
		_ = tx.Rollback()
		return false, s.recordFailure(rel, false, fmt.Errorf("record resource write: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return false, s.recordFailure(rel, false, fmt.Errorf("commit release: %w", err))
	}

	// Only now does the data plane find out. This is the step that cannot be
	// transactional with SQLite, which is why the event was persisted first.
	if err := adapter.Notify(rel.ResourceID); err != nil {
		return false, s.recordFailure(rel, true, fmt.Errorf("notify: %w", err))
	}

	if err := s.recordSuccess(rel, rev); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) recordSuccess(rel *Release, rev *Revision) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("configver: begin release commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := setRevisionStatus(tx, rev.ID, rev.Status, StatusApplied, ""); err != nil {
		return fmt.Errorf("configver: mark revision applied: %w", err)
	}
	if _, err := tx.Exec(`UPDATE config_release_outbox
		SET status = ?, attempts = attempts + 1, applied_generation = ?,
		    last_error = '', applied_at = datetime('now'), next_attempt_at = datetime('now')
		WHERE id = ?`, ReleaseApplied, rev.Revision, rel.ID); err != nil {
		return fmt.Errorf("configver: mark release applied: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("configver: commit release: %w", err)
	}
	return nil
}

// recordFailure backs the release off, or gives up and marks it failed.
//
// Giving up is deliberate after ReleaseMaxAttempts: retrying forever hides a
// permanent fault (a resource type whose adapter was removed, content that can
// no longer be applied) behind a queue that always looks busy. The row stays
// visible so the fault is seen.
//
// The revision is only marked `failed` when the resource write never
// succeeded. If the write landed and only the announcement kept failing, the
// configuration IS in the database and a restart will load it, so reporting
// the revision as failed would tell an operator that nothing changed when
// something did. In that case the revision stays `staged` and the release row
// carries the failure.
func (s *Service) recordFailure(rel *Release, resourceApplied bool, cause error) error {
	attempts := rel.Attempts + 1
	msg := cause.Error()
	// Bound the stored message: a driver error can be long, and this column is
	// displayed in the UI.
	if len(msg) > 500 {
		msg = msg[:500]
	}
	written := resourceApplied || rel.ResourceApplied

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("configver: begin release failure: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if attempts >= ReleaseMaxAttempts {
		if _, err := tx.Exec(`UPDATE config_release_outbox
			SET status = ?, attempts = ?, last_error = ?
			WHERE id = ?`, ReleaseFailed, attempts, msg, rel.ID); err != nil {
			return fmt.Errorf("configver: mark release failed: %w", err)
		}
		if !written {
			rev, err := scanRevision(tx.QueryRow(revisionSelect+` WHERE id = ?`, rel.RevisionID))
			if err == nil {
				if err := setRevisionStatus(tx, rev.ID, rev.Status, StatusFailed, msg); err != nil {
					return err
				}
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	} else {
		next := nowFunc().Add(releaseBackoff(attempts)).Format("2006-01-02 15:04:05")
		if _, err := tx.Exec(`UPDATE config_release_outbox
			SET status = ?, attempts = ?, last_error = ?, next_attempt_at = ?
			WHERE id = ?`, ReleasePending, attempts, msg, next, rel.ID); err != nil {
			return fmt.Errorf("configver: back off release: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("configver: commit release failure: %w", err)
	}
	return nil
}

// releaseBackoff is exponential from ReleaseRetryBase, capped at
// ReleaseRetryMax. Exported behaviour is tested; the formula is not part of the
// API contract.
func releaseBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	d := ReleaseRetryBase
	for i := 1; i < attempts; i++ {
		d *= 2
		if d >= ReleaseRetryMax {
			return ReleaseRetryMax
		}
	}
	return d
}

func (s *Service) loadRelease(id int64) (*Release, error) {
	var rel Release
	var appliedAt sql.NullString
	err := s.db.QueryRow(releaseSelect+` WHERE id = ?`, id).Scan(
		&rel.ID, &rel.RevisionID, &rel.ResourceType, &rel.ResourceID, &rel.Revision,
		&rel.Status, &rel.Attempts, &rel.ResourceApplied, &rel.AppliedGeneration,
		&rel.LastError, &rel.CreatedAt, &appliedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("configver: load release: %w", err)
	}
	rel.AppliedAt = appliedAt.String
	return &rel, nil
}

// ListReleases returns queued release events for operations visibility.
func (s *Service) ListReleases(status string, limit int) ([]Release, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := releaseSelect
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("configver: list releases: %w", err)
	}
	defer rows.Close()

	var out []Release
	for rows.Next() {
		var rel Release
		var appliedAt sql.NullString
		if err := rows.Scan(&rel.ID, &rel.RevisionID, &rel.ResourceType, &rel.ResourceID,
			&rel.Revision, &rel.Status, &rel.Attempts, &rel.ResourceApplied, &rel.AppliedGeneration,
			&rel.LastError, &rel.CreatedAt, &appliedAt); err != nil {
			return nil, err
		}
		rel.AppliedAt = appliedAt.String
		out = append(out, rel)
	}
	return out, rows.Err()
}

// OutboxStats counts pending and failed releases. Failed is the number an
// operator should alarm on: it means a configuration is written but not in
// service.
func (s *Service) OutboxStats() (pending, failed int64, err error) {
	err = s.db.QueryRow(`SELECT
		  COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0)
		FROM config_release_outbox`, ReleasePending, ReleaseFailed).Scan(&pending, &failed)
	if err != nil {
		return 0, 0, fmt.Errorf("configver: outbox stats: %w", err)
	}
	return pending, failed, nil
}

// RetryFailed moves failed releases for a resource back to pending.
//
// Without this, a release that gave up after a transient outage would need a
// manual database edit to recover -- which is exactly the kind of operational
// trap this project is trying to remove.
func (s *Service) RetryFailed(rt ResourceType, id string) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("configver: begin retry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var query strings.Builder
	query.WriteString(`UPDATE config_release_outbox
		SET status = ?, attempts = 0, next_attempt_at = datetime('now'), last_error = ''
		WHERE status = ?`)
	args := []any{ReleasePending, ReleaseFailed}
	if rt != "" {
		query.WriteString(" AND resource_type = ?")
		args = append(args, rt)
	}
	if id != "" {
		query.WriteString(" AND resource_id = ?")
		args = append(args, id)
	}

	res, err := tx.Exec(query.String(), args...)
	if err != nil {
		return 0, fmt.Errorf("configver: reset failed releases: %w", err)
	}
	affected, _ := res.RowsAffected()

	// The revisions failed alongside their release must return to a state the
	// release worker will act on, or the queue would drain while the revision
	// stayed permanently marked failed.
	if _, err := tx.Exec(`UPDATE config_revisions
		SET status = ?, error = '', updated_at = datetime('now')
		WHERE status = ?
		  AND id IN (SELECT revision_id FROM config_release_outbox WHERE status = ?)`,
		StatusStaged, StatusFailed, ReleasePending); err != nil {
		return 0, fmt.Errorf("configver: reset failed revisions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("configver: commit retry: %w", err)
	}
	return int(affected), nil
}

// Run drives the release worker until ctx is cancelled.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = ReleaseInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			applied, err := s.DrainOutbox(ReleaseBatchSize)
			if err != nil {
				slog.Error("config release drain failed", "error", err)
			}
			if applied > 0 {
				slog.Info("config releases applied", "count", applied)
			}
		}
	}
}
