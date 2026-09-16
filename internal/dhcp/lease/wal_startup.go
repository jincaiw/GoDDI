package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrStartupNotReady = errors.New("lease startup: projection is not ready")

// RecoverSQLiteProjection replays the durable WAL events that are newer than
// the persisted lease projection watermark. Recovery is synchronous and
// fail-closed: callers must not open DHCP admission until it returns nil.
//
// Each event is applied through ApplyWALEventTx, so the lease row and applied
// watermark commit together. The WAL file remains the durable source; this
// function does not truncate or rewrite it.
func RecoverSQLiteProjection(ctx context.Context, wal *WALFile, db *sql.DB) (int64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("%w: context is nil", ErrStartupNotReady)
	}
	if wal == nil {
		return 0, fmt.Errorf("%w: WAL is nil", ErrStartupNotReady)
	}
	if db == nil {
		return 0, fmt.Errorf("%w: database is nil", ErrStartupNotReady)
	}
	if ctx == nil {
		return 0, fmt.Errorf("%w: context is nil", ErrStartupNotReady)
	}
	if err := EnsureAppliedWatermark(ctx, db); err != nil {
		return 0, fmt.Errorf("%w: ensure watermark: %v", ErrStartupNotReady, err)
	}
	applied, err := readAppliedWatermarkDB(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("%w: read watermark: %v", ErrStartupNotReady, err)
	}
	last, err := wal.Replay(applied, func(event WALEvent) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin projection recovery: %w", err)
		}
		if _, err := ApplyWALEventTx(ctx, tx, event); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit projection recovery: %w", err)
		}
		return nil
	})
	if err != nil {
		return last, fmt.Errorf("%w: %v", ErrStartupNotReady, err)
	}
	return last, nil
}

func readAppliedWatermarkDB(ctx context.Context, db *sql.DB) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	watermark, err := ReadAppliedWatermark(ctx, tx)
	if err != nil {
		return 0, err
	}
	return watermark.Applied, nil
}
