package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ExpireFactsBatch is the explicit migration seam for the existing batch
// expiry operation. It preserves candidate selection and the single
// transaction boundary of Manager.ExpireLeases while adding one fact per
// changed lease. It is not used by the default expiry sweep.
func (w *FactsMutationWriter) ExpireFactsBatch(ctx context.Context, source string, spaceIDFor func(*Lease) (string, error)) ([]*Lease, error) {
	if w == nil || w.manager == nil || w.allocator == nil || w.outbox == nil {
		return nil, errors.New("lease facts expiry: nil writer")
	}
	if strings.TrimSpace(source) == "" {
		return nil, errors.New("lease facts expiry: source is required")
	}
	if spaceIDFor == nil {
		return nil, errors.New("lease facts expiry: space resolver is required")
	}
	if ctx == nil {
		return nil, errors.New("lease facts expiry: nil context")
	}

	tx, err := w.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lease facts expiry: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	candidates, err := expiredCandidatesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("lease facts expiry: commit empty batch: %w", err)
		}
		return nil, nil
	}

	if _, err := tx.ExecContext(ctx, `UPDATE dhcp_leases SET status=?
		WHERE status IN (?, ?, ?) AND julianday(lease_end) <= julianday('now')`,
		string(LeaseStatusExpired), string(LeaseStatusActive), string(LeaseStatusOffered), string(LeaseStatusConflict)); err != nil {
		return nil, fmt.Errorf("lease facts expiry: update batch: %w", err)
	}

	for _, before := range candidates {
		after := *before
		after.Status = LeaseStatusExpired
		spaceID, err := spaceIDFor(before)
		if err != nil {
			return nil, fmt.Errorf("lease facts expiry: resolve space for %s: %w", before.ID, err)
		}
		if strings.TrimSpace(spaceID) == "" {
			return nil, fmt.Errorf("lease facts expiry: empty space for %s", before.ID)
		}
		identity, err := w.Identity(source, spaceID, MutationExpire, &after)
		if err != nil {
			return nil, fmt.Errorf("lease facts expiry: identity for %s: %w", before.ID, err)
		}
		if err := w.enqueueMutationFact(ctx, tx, MutationCommand{Kind: MutationExpire, Before: before, After: &after}, identity.EventID, source, spaceID); err != nil {
			return nil, fmt.Errorf("lease facts expiry: enqueue %s: %w", before.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lease facts expiry: commit: %w", err)
	}
	return candidates, nil
}

func expiredCandidatesTx(ctx context.Context, tx *sql.Tx) ([]*Lease, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
		lease_start, lease_end, status, last_seen, generation FROM dhcp_leases
		WHERE status IN (?, ?, ?) AND julianday(lease_end) <= julianday('now')`,
		string(LeaseStatusActive), string(LeaseStatusOffered), string(LeaseStatusConflict))
	if err != nil {
		return nil, fmt.Errorf("lease facts expiry: select candidates: %w", err)
	}
	defer rows.Close()
	var result []*Lease
	for rows.Next() {
		l := &Lease{}
		var hostname, clientID sql.NullString
		if err := rows.Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
			&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation); err != nil {
			return nil, fmt.Errorf("lease facts expiry: scan candidate: %w", err)
		}
		l.Hostname, l.ClientID = hostname.String, clientID.String
		result = append(result, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lease facts expiry: iterate candidates: %w", err)
	}
	return result, nil
}
