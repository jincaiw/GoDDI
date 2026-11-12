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
// changed lease. Unmapped leases still expire, but cannot produce a space-scoped
// fact until their local IPAM mapping is available.
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

	// Select candidates and resolve their IPAM spaces BEFORE opening the write
	// transaction. The lease store is a single-connection handle
	// (SetMaxOpenConns(1)) and the space resolver reads that same store, so
	// resolving inside the transaction would block forever on the one
	// connection the transaction holds. Resolving up front also keeps the
	// "one bad mapping fails the whole batch" guarantee without a half-applied
	// update: a resolver error returns before any write happens.
	candidates, err := expiredCandidates(ctx, w.manager.db)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	spaces := make(map[string]string, len(candidates))
	for _, before := range candidates {
		spaceID, err := spaceIDFor(before)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("lease facts expiry: resolve space for %s: %w", before.ID, err)
		}
		if strings.TrimSpace(spaceID) == "" {
			return nil, fmt.Errorf("lease facts expiry: empty space for %s", before.ID)
		}
		spaces[before.ID] = spaceID
	}

	tx, err := w.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lease facts expiry: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	type expiredPair struct{ before, after *Lease }
	var expired []*Lease
	var pairs []expiredPair
	for _, before := range candidates {
		// Re-check eligibility inside the transaction: the row may have been
		// renewed or released between the read above and this write. Only a row
		// this statement actually expires earns a fact and an audit entry.
		res, err := tx.ExecContext(ctx, `UPDATE dhcp_leases SET status=?
			WHERE id=? AND status IN (?, ?, ?) AND julianday(lease_end) <= julianday('now')`,
			string(LeaseStatusExpired), before.ID,
			string(LeaseStatusActive), string(LeaseStatusOffered), string(LeaseStatusConflict))
		if err != nil {
			return nil, fmt.Errorf("lease facts expiry: update %s: %w", before.ID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("lease facts expiry: rows affected %s: %w", before.ID, err)
		}
		if n == 0 {
			continue
		}
		after := *before
		after.Status = LeaseStatusExpired
		expired = append(expired, &after)
		pairs = append(pairs, expiredPair{before: before, after: &after})
		spaceID, mapped := spaces[before.ID]
		if !mapped {
			continue
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
	for _, p := range pairs {
		w.manager.auditExpire(p.before, p.after)
	}
	w.notifyPostCommit()
	return expired, nil
}

// expiredCandidates reads the leases eligible for expiry using a plain (non
// transactional) query so the single store connection is released before the
// caller opens its write transaction and before any resolver touches the store.
func expiredCandidates(ctx context.Context, db *sql.DB) ([]*Lease, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
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
