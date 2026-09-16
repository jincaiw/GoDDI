package facts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const sequenceAllocatorDomain = "dhcp_dns_ipam"

var ErrSequenceRegression = errors.New("facts: sequence regression")

// SequenceAllocator allocates the canonical sequence for the unified facts
// stream. It is migration-stage infrastructure and must be called inside the
// same SQL transaction as the authoritative mutation and EnqueueTx.
type SequenceAllocator struct {
	db *sql.DB
}

func NewSequenceAllocator(db *sql.DB) (*SequenceAllocator, error) {
	if db == nil {
		return nil, errors.New("facts: nil sequence allocator database")
	}
	return &SequenceAllocator{db: db}, nil
}

// EnsureTx initializes the allocator row without committing.
func (a *SequenceAllocator) EnsureTx(ctx context.Context, tx *sql.Tx) error {
	if a == nil || a.db == nil {
		return ErrOutboxClosed
	}
	if tx == nil {
		return errors.New("facts: nil sequence transaction")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO facts_sequence_allocator (domain, last_sequence)
		VALUES (?, 0) ON CONFLICT(domain) DO NOTHING`, sequenceAllocatorDomain)
	if err != nil {
		return fmt.Errorf("facts: ensure sequence row: %w", err)
	}
	return nil
}

// NextTx atomically advances and returns the next canonical sequence. The
// caller owns the transaction and must commit it with the mutation/outbox.
func (a *SequenceAllocator) NextTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	if err := a.EnsureTx(ctx, tx); err != nil {
		return 0, err
	}
	var current int64
	if err := tx.QueryRowContext(ctx, `
		SELECT last_sequence FROM facts_sequence_allocator WHERE domain = ?`, sequenceAllocatorDomain).Scan(&current); err != nil {
		return 0, fmt.Errorf("facts: read sequence: %w", err)
	}
	next := current + 1
	result, err := tx.ExecContext(ctx, `
		UPDATE facts_sequence_allocator SET last_sequence = ?
		WHERE domain = ? AND last_sequence = ?`, next, sequenceAllocatorDomain, current)
	if err != nil {
		return 0, fmt.Errorf("facts: advance sequence: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("facts: advance sequence rows: %w", err)
	}
	if rows != 1 {
		return 0, fmt.Errorf("%w: expected one allocator row, updated=%d", ErrSequenceRegression, rows)
	}
	return next, nil
}

// Database returns the database that owns the allocator row. It is exposed
// only for migration-stage wiring checks; it does not provide cross-database
// transaction semantics.
func (a *SequenceAllocator) Database() *sql.DB {
	if a == nil {
		return nil
	}
	return a.db
}

func (a *SequenceAllocator) Current(ctx context.Context) (int64, error) {
	if a == nil || a.db == nil {
		return 0, ErrOutboxClosed
	}
	var current int64
	if err := a.db.QueryRowContext(ctx, `
		SELECT last_sequence FROM facts_sequence_allocator WHERE domain = ?`, sequenceAllocatorDomain).Scan(&current); err != nil {
		return 0, fmt.Errorf("facts: read sequence: %w", err)
	}
	return current, nil
}
