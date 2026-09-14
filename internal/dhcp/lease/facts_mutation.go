package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/facts"
)

// FactsMutationWriter is an explicit, migration-stage transaction seam for
// lease mutations that publish the unified DHCP-DNS-IPAM fact envelope.
//
// The default Manager methods and DHCP server constructor do not use this
// writer. Callers opting in must provide the event identity and IPAM space ID;
// the authoritative lease update, canonical sequence allocation, envelope
// construction, and durable outbox insert are committed together.
//
// DNSMutationSink is the optional compatibility bridge for the legacy DNS
// outbox. Implementations must write through the supplied transaction; they
// must not commit it. Keeping the interface here avoids making the migration
// seam depend on the DHCP package.
type DNSMutationSink interface {
	EnqueueTx(tx *sql.Tx, l *Lease, action DNSMutationAction) error
}

type FactsMutationWriter struct {
	manager   *Manager
	allocator *facts.SequenceAllocator
	outbox    *facts.ObservationOutbox
	dns       DNSMutationSink
}

func NewFactsMutationWriter(manager *Manager, allocator *facts.SequenceAllocator, outbox *facts.ObservationOutbox) (*FactsMutationWriter, error) {
	if manager == nil || manager.db == nil {
		return nil, errors.New("lease facts mutation: nil manager")
	}
	if allocator == nil {
		return nil, errors.New("lease facts mutation: nil sequence allocator")
	}
	if outbox == nil {
		return nil, errors.New("lease facts mutation: nil observation outbox")
	}
	return &FactsMutationWriter{manager: manager, allocator: allocator, outbox: outbox}, nil
}

// WithDNSSink enables the legacy DNS outbox compatibility write. It remains
// opt-in and is applied in the same transaction as the lease and facts event.
func (w *FactsMutationWriter) WithDNSSink(sink DNSMutationSink) *FactsMutationWriter {
	if w != nil {
		w.dns = sink
	}
	return w
}

// ActivateLease performs the offered -> active transition and commits its fact
// in one transaction. It is opt-in and intentionally does not alter
// Manager.ActivateLease or the default DHCP request path.
func (w *FactsMutationWriter) ActivateLease(ctx context.Context, eventID, source, spaceID, id string, duration time.Duration) (*Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, err
	}
	tx, err := w.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lease facts mutation: begin activate: %w", err)
	}
	before, after, err := w.ActivateLeaseTx(ctx, tx, eventID, source, spaceID, id, duration)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lease facts mutation: commit activate: %w", err)
	}
	w.manager.auditBind(before, after)
	return after, nil
}

// ActivateLeaseTx performs the same transition without committing. The caller
// owns commit/rollback, allowing the authoritative mutation to share a larger
// transaction boundary when the remaining side effects are migrated.
func (w *FactsMutationWriter) ActivateLeaseTx(ctx context.Context, tx *sql.Tx, eventID, source, spaceID, id string, duration time.Duration) (*Lease, *Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	if tx == nil {
		return nil, nil, errors.New("lease facts mutation: nil transaction")
	}
	before, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: read activate %s: %w", id, err)
	}
	if !found || before.Status != LeaseStatusOffered {
		return nil, nil, fmt.Errorf("%w: activate requires offered lease %s", ErrInvalidMutationState, id)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE dhcp_leases
		SET lease_start=?, lease_end=?, status=?, last_seen=datetime('now'), generation=generation+1
		WHERE id=? AND status=?`,
		now.Format("2006-01-02T15:04:05Z"), now.Add(duration).Format("2006-01-02T15:04:05Z"),
		string(LeaseStatusActive), id, string(LeaseStatusOffered)); err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: update activate %s: %w", id, err)
	}
	after, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil || !found {
		if err == nil {
			err = errors.New("lease disappeared after activate")
		}
		return nil, nil, fmt.Errorf("lease facts mutation: read activated %s: %w", id, err)
	}
	if err := w.enqueueMutationFact(ctx, tx, MutationCommand{Kind: MutationActivate, Before: before, After: after}, eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// RenewLease performs the active -> active transition and commits its fact in
// one transaction. It is opt-in and leaves the default Manager path unchanged.
func (w *FactsMutationWriter) RenewLease(ctx context.Context, eventID, source, spaceID, id string, duration time.Duration) (*Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, err
	}
	tx, err := w.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lease facts mutation: begin renew: %w", err)
	}
	before, after, err := w.RenewLeaseTx(ctx, tx, eventID, source, spaceID, id, duration)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lease facts mutation: commit renew: %w", err)
	}
	w.manager.auditBind(before, after)
	return after, nil
}

// RenewLeaseTx performs renewal and fact enqueue without committing.
func (w *FactsMutationWriter) RenewLeaseTx(ctx context.Context, tx *sql.Tx, eventID, source, spaceID, id string, duration time.Duration) (*Lease, *Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	if tx == nil {
		return nil, nil, errors.New("lease facts mutation: nil transaction")
	}
	before, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: read renew %s: %w", id, err)
	}
	if !found || before.Status != LeaseStatusActive {
		return nil, nil, fmt.Errorf("%w: renew requires active lease %s", ErrInvalidMutationState, id)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE dhcp_leases
		SET lease_start=?, lease_end=?, status=?, last_seen=datetime('now'), generation=generation+1
		WHERE id=? AND status=?`,
		now.Format("2006-01-02T15:04:05Z"), now.Add(duration).Format("2006-01-02T15:04:05Z"),
		string(LeaseStatusActive), id, string(LeaseStatusActive)); err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: update renew %s: %w", id, err)
	}
	after, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil || !found {
		if err == nil {
			err = errors.New("lease disappeared after renew")
		}
		return nil, nil, fmt.Errorf("lease facts mutation: read renewed %s: %w", id, err)
	}
	if err := w.enqueueMutationFact(ctx, tx, MutationCommand{Kind: MutationRenew, Before: before, After: after}, eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// DeclineLease performs one active/offered -> conflict transition and commits
// its fact in one transaction. Unknown-address DECLINE tombstones remain on the
// legacy QuarantineIP path because they have no prior lease snapshot.
func (w *FactsMutationWriter) DeclineLease(ctx context.Context, eventID, source, spaceID, id string, quarantine time.Duration) (*Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, err
	}
	tx, err := w.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lease facts mutation: begin decline: %w", err)
	}
	before, after, err := w.DeclineLeaseTx(ctx, tx, eventID, source, spaceID, id, quarantine)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lease facts mutation: commit decline: %w", err)
	}
	w.manager.auditDecline(before, after)
	return after, nil
}

// DeclineLeaseTx performs conflict quarantine and fact enqueue without
// committing. The quarantine duration is supplied by the caller so this seam
// does not expose the legacy package constant.
func (w *FactsMutationWriter) DeclineLeaseTx(ctx context.Context, tx *sql.Tx, eventID, source, spaceID, id string, quarantine time.Duration) (*Lease, *Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	if tx == nil {
		return nil, nil, errors.New("lease facts mutation: nil transaction")
	}
	if quarantine <= 0 {
		return nil, nil, errors.New("lease facts mutation: quarantine duration must be positive")
	}
	before, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: read decline %s: %w", id, err)
	}
	if !found || (before.Status != LeaseStatusActive && before.Status != LeaseStatusOffered) {
		return nil, nil, fmt.Errorf("%w: decline requires active or offered lease %s", ErrInvalidMutationState, id)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dhcp_leases SET status=?, lease_end=?, last_seen=datetime('now')
		WHERE id=? AND status IN (?, ?)`, string(LeaseStatusConflict), time.Now().UTC().Add(quarantine).Format("2006-01-02T15:04:05Z"), id,
		string(LeaseStatusActive), string(LeaseStatusOffered)); err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: update decline %s: %w", id, err)
	}
	after, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil || !found {
		if err == nil {
			err = errors.New("lease disappeared after decline")
		}
		return nil, nil, fmt.Errorf("lease facts mutation: read declined %s: %w", id, err)
	}
	if err := w.enqueueMutationFact(ctx, tx, MutationCommand{Kind: MutationDecline, Before: before, After: after}, eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// ExpireLease performs one held -> expired transition and commits its fact in
// one transaction. Batch ExpireLeases remains the default compatibility path;
// this method is only the explicit migration seam for a caller that already
// owns per-lease expiry selection.
func (w *FactsMutationWriter) ExpireLease(ctx context.Context, eventID, source, spaceID, id string) (*Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, err
	}
	tx, err := w.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lease facts mutation: begin expire: %w", err)
	}
	before, after, err := w.ExpireLeaseTx(ctx, tx, eventID, source, spaceID, id)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lease facts mutation: commit expire: %w", err)
	}
	w.manager.auditExpire(before, after)
	return after, nil
}

// ExpireLeaseTx performs one expiry and fact enqueue without committing.
func (w *FactsMutationWriter) ExpireLeaseTx(ctx context.Context, tx *sql.Tx, eventID, source, spaceID, id string) (*Lease, *Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	if tx == nil {
		return nil, nil, errors.New("lease facts mutation: nil transaction")
	}
	before, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: read expire %s: %w", id, err)
	}
	if !found || (before.Status != LeaseStatusActive && before.Status != LeaseStatusOffered && before.Status != LeaseStatusConflict) {
		return nil, nil, fmt.Errorf("%w: expire requires held lease %s", ErrInvalidMutationState, id)
	}
	result, err := tx.ExecContext(ctx, `UPDATE dhcp_leases SET status=?, last_seen=datetime('now')
		WHERE id=? AND status IN (?, ?, ?)`, string(LeaseStatusExpired), id,
		string(LeaseStatusActive), string(LeaseStatusOffered), string(LeaseStatusConflict))
	if err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: update expire %s: %w", id, err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err == nil {
			err = fmt.Errorf("updated=%d rows", rows)
		}
		return nil, nil, fmt.Errorf("lease facts mutation: expire %s did not apply: %w", id, err)
	}
	after, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil || !found {
		if err == nil {
			err = errors.New("lease disappeared after expire")
		}
		return nil, nil, fmt.Errorf("lease facts mutation: read expired %s: %w", id, err)
	}
	if err := w.enqueueMutationFact(ctx, tx, MutationCommand{Kind: MutationExpire, Before: before, After: after}, eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

// ReleaseLease performs the active/offered -> released transition and commits
// its fact atomically. It is an opt-in compatibility seam; the legacy
// Manager.ReleaseLease API remains unchanged.
func (w *FactsMutationWriter) ReleaseLease(ctx context.Context, eventID, source, spaceID, id string) (*Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, err
	}
	tx, err := w.manager.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lease facts mutation: begin release: %w", err)
	}
	before, after, err := w.ReleaseLeaseTx(ctx, tx, eventID, source, spaceID, id)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lease facts mutation: commit release: %w", err)
	}
	w.manager.auditRelease(before, after)
	return after, nil
}

// ReleaseLeaseTx performs release and fact enqueue without committing.
func (w *FactsMutationWriter) ReleaseLeaseTx(ctx context.Context, tx *sql.Tx, eventID, source, spaceID, id string) (*Lease, *Lease, error) {
	if err := validateFactIdentity(eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	if tx == nil {
		return nil, nil, errors.New("lease facts mutation: nil transaction")
	}
	before, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: read release %s: %w", id, err)
	}
	if !found || (before.Status != LeaseStatusActive && before.Status != LeaseStatusOffered) {
		return nil, nil, fmt.Errorf("%w: release requires active or offered lease %s", ErrInvalidMutationState, id)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dhcp_leases SET status=?, last_seen=datetime('now') WHERE id=?`, string(LeaseStatusReleased), id); err != nil {
		return nil, nil, fmt.Errorf("lease facts mutation: update release %s: %w", id, err)
	}
	after, found, err := w.manager.findLeaseTx(ctx, tx, id)
	if err != nil || !found {
		if err == nil {
			err = errors.New("lease disappeared after release")
		}
		return nil, nil, fmt.Errorf("lease facts mutation: read released %s: %w", id, err)
	}
	if err := w.enqueueMutationFact(ctx, tx, MutationCommand{Kind: MutationRelease, Before: before, After: after}, eventID, source, spaceID); err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

func (w *FactsMutationWriter) enqueueMutationFact(ctx context.Context, tx *sql.Tx, command MutationCommand, eventID, source, spaceID string) error {
	if strings.TrimSpace(eventID) == "" {
		identity, err := w.Identity(source, spaceID, command.Kind, command.After)
		if err != nil {
			return fmt.Errorf("lease facts mutation: derive event identity: %w", err)
		}
		eventID = identity.EventID
	}
	sequence, err := w.allocator.NextTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("lease facts mutation: allocate sequence: %w", err)
	}
	event, err := command.FactEnvelope(eventID, source, spaceID, sequence, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("lease facts mutation: build envelope: %w", err)
	}
	if err := w.outbox.EnqueueTx(ctx, tx, event); err != nil {
		return fmt.Errorf("lease facts mutation: enqueue %s: %w", eventID, err)
	}
	if w.dns != nil {
		action := DNSActionUpsert
		switch command.Kind {
		case MutationRelease, MutationDecline, MutationExpire:
			action = DNSActionDelete
		}
		if err := w.dns.EnqueueTx(tx, command.After, action); err != nil {
			return fmt.Errorf("lease facts mutation: enqueue legacy dns %s: %w", eventID, err)
		}
	}
	return nil
}

func (w *FactsMutationWriter) Identity(source, spaceID string, kind MutationKind, l *Lease) (MutationFactIdentity, error) {
	if l == nil {
		return MutationFactIdentity{}, errors.New("lease facts mutation: lease is required for identity")
	}
	return NewMutationFactIdentity(source, spaceID, kind, l.ID, l.Generation)
}

func validateFactIdentity(eventID, source, spaceID string) error {
	// EventID may be omitted: enqueueMutationFact derives the retry-stable ID
	// from the post-mutation snapshot. Source and space remain caller-owned
	// identity inputs and are always mandatory.
	if strings.TrimSpace(source) == "" || strings.TrimSpace(spaceID) == "" {
		return errors.New("lease facts mutation: source and space ID are required")
	}
	return nil
}

func (m *Manager) findLeaseTx(ctx context.Context, tx *sql.Tx, id string) (*Lease, bool, error) {
	l := &Lease{}
	var hostname, clientID sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
		lease_start, lease_end, status, last_seen, generation FROM dhcp_leases WHERE id=?`, id).
		Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
			&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	l.Hostname = hostname.String
	l.ClientID = clientID.String
	return l, true, nil
}
