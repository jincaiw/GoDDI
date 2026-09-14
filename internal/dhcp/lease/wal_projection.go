package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrProjectionUnknownOperation = errors.New("lease projection: unknown operation")

// NewSQLiteProjectionApplier creates the staged SQLite projection adapter. It
// initializes the durable watermark, reads its value, and then connects the
// ordered AsyncApplier to a transaction that applies the lease snapshot and
// watermark together. This is an explicit opt-in helper; it does not alter the
// default DHCP server construction or ACK path.
func NewSQLiteProjectionApplier(parent context.Context, db *sql.DB, capacity int) (*AsyncApplier, error) {
	if db == nil {
		return nil, errors.New("lease projection: nil database")
	}
	if err := EnsureAppliedWatermark(parent, db); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(parent, nil)
	if err != nil {
		return nil, fmt.Errorf("lease projection: begin watermark read: %w", err)
	}
	watermark, err := ReadAppliedWatermark(parent, tx)
	if rollbackErr := tx.Rollback(); err != nil {
		return nil, err
	} else if rollbackErr != nil {
		return nil, fmt.Errorf("lease projection: close watermark read: %w", rollbackErr)
	}
	if err != nil {
		return nil, err
	}
	return NewAsyncApplier(parent, capacity, watermark.Applied, func(ctx context.Context, event WALEvent) error {
		projectionTx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("lease projection: begin apply: %w", err)
		}
		if _, err := ApplyWALEventTx(ctx, projectionTx, event); err != nil {
			_ = projectionTx.Rollback()
			return err
		}
		if err := projectionTx.Commit(); err != nil {
			return fmt.Errorf("lease projection: commit apply: %w", err)
		}
		return nil
	})
}

// ApplyWALEventTx applies one durable WAL snapshot and advances the projection
// watermark in the same SQL transaction. The caller owns Begin/Commit/Rollback;
// this helper intentionally does not commit so a future transaction can add
// DNS outbox and IPAM observation writes before the same commit.
func ApplyWALEventTx(ctx context.Context, tx *sql.Tx, event WALEvent) (bool, error) {
	if tx == nil {
		return false, errors.New("lease projection: nil transaction")
	}
	if err := validateWALEvent(event); err != nil {
		return false, err
	}

	watermark, err := ReadAppliedWatermark(ctx, tx)
	if err != nil {
		return false, err
	}
	if event.Seq <= watermark.Applied {
		return false, nil
	}
	if event.Seq != watermark.Applied+1 {
		return false, fmt.Errorf("%w: expected=%d got=%d", ErrWatermarkGap, watermark.Applied+1, event.Seq)
	}

	switch event.Op {
	case WALEventUpsert:
		if err := upsertLeaseProjectionTx(ctx, tx, *event.Lease); err != nil {
			return false, err
		}
	case WALEventRemove:
		if _, err := tx.ExecContext(ctx, `DELETE FROM dhcp_leases WHERE id = ?`, event.LeaseID); err != nil {
			return false, fmt.Errorf("lease projection: remove %s: %w", event.LeaseID, err)
		}
	default:
		return false, fmt.Errorf("%w %q", ErrProjectionUnknownOperation, event.Op)
	}

	if _, err := ApplyWatermark(ctx, tx, event.Seq); err != nil {
		return false, err
	}
	return true, nil
}

func upsertLeaseProjectionTx(ctx context.Context, tx *sql.Tx, value Lease) error {
	if value.ID == "" || value.ScopeID == "" || value.IPAddress == "" || value.MACAddress == "" {
		return fmt.Errorf("%w: incomplete lease %q", ErrWALCorrupt, value.ID)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO dhcp_leases (
			id, scope_id, ip_address, mac_address, hostname, client_id,
			lease_start, lease_end, status, last_seen, generation
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scope_id = excluded.scope_id,
			ip_address = excluded.ip_address,
			mac_address = excluded.mac_address,
			hostname = excluded.hostname,
			client_id = excluded.client_id,
			lease_start = excluded.lease_start,
			lease_end = excluded.lease_end,
			status = excluded.status,
			last_seen = excluded.last_seen,
			generation = excluded.generation`,
		value.ID, value.ScopeID, value.IPAddress, value.MACAddress, value.Hostname, value.ClientID,
		value.LeaseStart, value.LeaseEnd, string(value.Status), value.LastSeen, value.Generation)
	if err != nil {
		return fmt.Errorf("lease projection: upsert %s: %w", value.ID, err)
	}
	return nil
}
