package facts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrWatermarkGap        = errors.New("facts: durable watermark gap")
	ErrWatermarkRegression = errors.New("facts: durable watermark regression")
)

// WatermarkStore persists one monotonically contiguous sequence per consumer.
// The caller must advance it in the same transaction as the projection write.
type WatermarkStore struct {
	db *sql.DB
}

func NewWatermarkStore(db *sql.DB) (*WatermarkStore, error) {
	if db == nil {
		return nil, errors.New("facts: nil watermark database")
	}
	return &WatermarkStore{db: db}, nil
}

func (s *WatermarkStore) Applied(ctx context.Context, domain string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("facts: nil watermark store")
	}
	return appliedTx(ctx, s.db, domain)
}

func (s *WatermarkStore) EnsureTx(ctx context.Context, tx *sql.Tx, domain string) (int64, error) {
	if s == nil || s.db == nil || tx == nil {
		return 0, errors.New("facts: invalid watermark transaction")
	}
	return appliedTx(ctx, tx, domain)
}

// AdvanceTx accepts a replay exactly once. Older events are idempotent; future
// events are rejected so a consumer cannot silently skip durable facts.
func (s *WatermarkStore) AdvanceTx(ctx context.Context, tx *sql.Tx, domain string, sequence int64) (bool, error) {
	if sequence <= 0 {
		return false, fmt.Errorf("%w: sequence=%d", ErrWatermarkRegression, sequence)
	}
	applied, err := s.EnsureTx(ctx, tx, domain)
	if err != nil {
		return false, err
	}
	if sequence <= applied {
		return false, nil
	}
	if sequence != applied+1 {
		return false, fmt.Errorf("%w: domain=%s expected=%d got=%d", ErrWatermarkGap, domain, applied+1, sequence)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE facts_projection_watermark SET applied_seq = ? WHERE domain = ?`, sequence, domain); err != nil {
		return false, fmt.Errorf("facts watermark: advance %s: %w", domain, err)
	}
	return true, nil
}

func appliedTx(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, domain string) (int64, error) {
	if domain == "" {
		return 0, errors.New("facts: empty watermark domain")
	}
	if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO facts_projection_watermark (domain, applied_seq) VALUES (?, 0)`, domain); err != nil {
		return 0, fmt.Errorf("facts watermark: ensure %s: %w", domain, err)
	}
	var applied int64
	if err := q.QueryRowContext(ctx, `SELECT applied_seq FROM facts_projection_watermark WHERE domain = ?`, domain).Scan(&applied); err != nil {
		return 0, fmt.Errorf("facts watermark: read %s: %w", domain, err)
	}
	return applied, nil
}
