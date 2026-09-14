package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const leaseProjectionWatermarkDomain = "dhcp_lease_projection"

var (
	// ErrWatermarkGap aliases the WAL ordering error so generic async replay and
	// the transaction-aware projection expose one errors.Is-compatible boundary.
	ErrWatermarkGap        = ErrWALGap
	ErrWatermarkRegression = errors.New("lease projection: watermark regression")
)

// AppliedWatermark is the durable projection sequence used by WAL replay. It
// is intentionally separate from dataplane_revision and DHCP HA watermarks:
// those counters describe configuration changes and peer replication, not the
// committed lease projection sequence.
type AppliedWatermark struct {
	Domain  string
	Applied int64
}

// ReadAppliedWatermark reads the lease projection watermark from the supplied
// transaction. The row must be initialized by the migration or by
// EnsureAppliedWatermark before it is read.
func ReadAppliedWatermark(ctx context.Context, tx *sql.Tx) (AppliedWatermark, error) {
	if tx == nil {
		return AppliedWatermark{}, errors.New("lease projection: nil transaction")
	}
	var watermark AppliedWatermark
	if err := tx.QueryRowContext(ctx, `
		SELECT domain, applied_seq
		FROM lease_projection_watermark
		WHERE domain = ?`, leaseProjectionWatermarkDomain).
		Scan(&watermark.Domain, &watermark.Applied); err != nil {
		return AppliedWatermark{}, fmt.Errorf("lease projection: read watermark: %w", err)
	}
	return watermark, nil
}

// EnsureAppliedWatermark creates the durable row at zero when missing. It is
// safe to call during startup or migration-assisted test setup.
func EnsureAppliedWatermark(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("lease projection: nil database")
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO lease_projection_watermark (domain, applied_seq)
		VALUES (?, 0)
		ON CONFLICT(domain) DO NOTHING`, leaseProjectionWatermarkDomain)
	if err != nil {
		return fmt.Errorf("lease projection: ensure watermark: %w", err)
	}
	return nil
}

// ApplyWatermark advances a watermark only for the next contiguous sequence.
// Replays at or below the persisted sequence are idempotent. A gap or
// regression never changes the durable row.
func ApplyWatermark(ctx context.Context, tx *sql.Tx, seq int64) (bool, error) {
	if tx == nil {
		return false, errors.New("lease projection: nil transaction")
	}
	if seq <= 0 {
		return false, fmt.Errorf("%w: sequence=%d", ErrWatermarkRegression, seq)
	}
	watermark, err := ReadAppliedWatermark(ctx, tx)
	if err != nil {
		return false, err
	}
	if seq <= watermark.Applied {
		return false, nil
	}
	if seq != watermark.Applied+1 {
		return false, fmt.Errorf("%w: expected=%d got=%d", ErrWatermarkGap, watermark.Applied+1, seq)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE lease_projection_watermark
		SET applied_seq = ?
		WHERE domain = ? AND applied_seq = ?`, seq, leaseProjectionWatermarkDomain, watermark.Applied); err != nil {
		return false, fmt.Errorf("lease projection: advance watermark: %w", err)
	}
	return true, nil
}
