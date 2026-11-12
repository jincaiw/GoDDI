package facts

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

// ErrChainFork reports that an envelope does not extend the chain tip this
// store already holds: a gap, a reordering, or a tampered/replayed event.
// It is a safety condition (DEC-007) and must not be auto-retried away.
var ErrChainFork = errors.New("facts: hash chain fork or mismatch")

// PayloadDigest returns the hex SHA-256 of the payload bytes.
func PayloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// ComputeTip returns the hex SHA-256 of the chain link for e given its
// predecessor tip. It binds the previous tip, the epoch, the event identity,
// the sequence and the payload digest into one value, so any change to any of
// them changes the tip.
func ComputeTip(prev string, e Envelope) string {
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write([]byte{0})
	h.Write([]byte(strconv.FormatInt(e.Epoch, 10)))
	h.Write([]byte{0})
	h.Write([]byte(e.EventID))
	h.Write([]byte{0})
	h.Write([]byte(strconv.FormatInt(e.Sequence, 10)))
	h.Write([]byte{0})
	h.Write([]byte(e.PayloadHash))
	return hex.EncodeToString(h.Sum(nil))
}

// ChainTip is a domain's current chain head.
type ChainTip struct {
	Epoch int64
	Tip   string
}

// ChainBuilder stamps envelopes onto one domain's chain. It is not safe for
// concurrent use; a producer stamps inside its write transaction.
type ChainBuilder struct {
	domain string
	epoch  int64
	tip    string
}

// NewChainBuilder returns a builder seeded from a prior tip (zero value starts
// a fresh chain).
func NewChainBuilder(domain string, start ChainTip) *ChainBuilder {
	return &ChainBuilder{domain: domain, epoch: start.Epoch, tip: start.Tip}
}

// Domain reports the chain domain this builder stamps for.
func (b *ChainBuilder) Domain() string { return b.domain }

// Tip reports the current head.
func (b *ChainBuilder) Tip() ChainTip { return ChainTip{Epoch: b.epoch, Tip: b.tip} }

// Stamp fills an envelope's hash-chain fields and advances the builder. The
// caller still owns persistence; use ChainTipStore to make the advance durable
// in the same transaction as the outbox insert.
func (b *ChainBuilder) Stamp(e Envelope) (Envelope, error) {
	if len(e.Payload) == 0 {
		return Envelope{}, fmt.Errorf("%w: empty payload cannot be chained", ErrInvalidEnvelope)
	}
	e.Epoch = b.epoch
	e.PayloadHash = PayloadDigest(e.Payload)
	e.PrevHash = b.tip
	e.TipHash = ComputeTip(b.tip, e)
	b.tip = e.TipHash
	return e, nil
}

// VerifyEnvelope checks that e extends prev correctly, returning the new tip.
// It is the consumer-side mirror of Stamp. When e has no chain fields it
// returns prev unchanged and no error (backward compatible).
func VerifyEnvelope(prev ChainTip, e Envelope) (ChainTip, error) {
	if !e.HasChain() {
		return prev, nil
	}
	if e.Epoch < prev.Epoch {
		return prev, fmt.Errorf("%w: epoch regressed %d->%d", ErrChainFork, prev.Epoch, e.Epoch)
	}
	if e.PrevHash != prev.Tip {
		return prev, fmt.Errorf("%w: prev_hash %q != chain tip %q (event %s)", ErrChainFork, e.PrevHash, prev.Tip, e.EventID)
	}
	if got := PayloadDigest(e.Payload); got != e.PayloadHash {
		return prev, fmt.Errorf("%w: payload hash mismatch for event %s", ErrChainFork, e.EventID)
	}
	want := ComputeTip(e.PrevHash, e)
	if want != e.TipHash {
		return prev, fmt.Errorf("%w: tip hash mismatch for event %s", ErrChainFork, e.EventID)
	}
	return ChainTip{Epoch: e.Epoch, Tip: e.TipHash}, nil
}

// VerifyChain verifies a contiguous slice in order (used by tests/recovery).
func VerifyChain(events []Envelope) error {
	prev := ChainTip{}
	for _, e := range events {
		next, err := VerifyEnvelope(prev, e)
		if err != nil {
			return err
		}
		prev = next
	}
	return nil
}

// ChainTipStore persists a domain's chain head so a producer can enforce a
// single linear chain across transactions and restarts.
type ChainTipStore struct{ db *sql.DB }

func NewChainTipStore(db *sql.DB) *ChainTipStore { return &ChainTipStore{db: db} }

// Ensure creates the tip table if absent (idempotent).
func (s *ChainTipStore) Ensure(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrOutboxClosed
	}
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS facts_chain_tip (
			domain TEXT PRIMARY KEY,
			epoch  INTEGER NOT NULL,
			tip    TEXT NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("facts: ensure chain tip table: %w", err)
	}
	return nil
}

// LoadTx reads the current tip inside a transaction (zero when absent).
func (s *ChainTipStore) LoadTx(ctx context.Context, tx *sql.Tx, domain string) (ChainTip, error) {
	var epoch int64
	var tip string
	err := tx.QueryRowContext(ctx, `SELECT epoch, tip FROM facts_chain_tip WHERE domain = ?`, domain).Scan(&epoch, &tip)
	if err == sql.ErrNoRows {
		return ChainTip{}, nil
	}
	if err != nil {
		return ChainTip{}, fmt.Errorf("facts: read chain tip: %w", err)
	}
	return ChainTip{Epoch: epoch, Tip: tip}, nil
}

// AdvanceTx compares-and-swaps the tip: it only succeeds when the stored tip
// still equals expectedTip. A concurrent writer that already advanced the
// chain causes ErrChainFork rather than a silent fork.
func (s *ChainTipStore) AdvanceTx(ctx context.Context, tx *sql.Tx, domain string, expectedTip string, next ChainTip) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE facts_chain_tip SET epoch = ?, tip = ?
		WHERE domain = ? AND tip = ?`, next.Epoch, next.Tip, domain, expectedTip)
	if err != nil {
		return fmt.Errorf("facts: advance chain tip: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	// No row updated: either no row exists yet, or the tip moved.
	if expectedTip == "" {
		_, ierr := tx.ExecContext(ctx, `
			INSERT INTO facts_chain_tip (domain, epoch, tip) VALUES (?, ?, ?)
			ON CONFLICT(domain) DO NOTHING`, domain, next.Epoch, next.Tip)
		if ierr != nil {
			return fmt.Errorf("facts: insert chain tip: %w", ierr)
		}
		// Re-check we won the insert race.
		var tip string
		if qerr := tx.QueryRowContext(ctx, `SELECT tip FROM facts_chain_tip WHERE domain = ?`, domain).Scan(&tip); qerr != nil {
			return fmt.Errorf("facts: verify chain tip insert: %w", qerr)
		}
		if tip != next.Tip {
			return fmt.Errorf("%w: concurrent chain advance for %s", ErrChainFork, domain)
		}
		return nil
	}
	return fmt.Errorf("%w: tip moved, expected %q", ErrChainFork, expectedTip)
}

// StampTx loads the domain tip, stamps e, and advances the tip, all inside the
// caller's transaction. This is the single call a producer makes beside its
// outbox insert so the chain and the event commit together.
func (s *ChainTipStore) StampTx(ctx context.Context, tx *sql.Tx, domain string, e Envelope) (Envelope, error) {
	tip, err := s.LoadTx(ctx, tx, domain)
	if err != nil {
		return Envelope{}, err
	}
	b := NewChainBuilder(domain, tip)
	stamped, err := b.Stamp(e)
	if err != nil {
		return Envelope{}, err
	}
	if err := s.AdvanceTx(ctx, tx, domain, tip.Tip, b.Tip()); err != nil {
		return Envelope{}, err
	}
	return stamped, nil
}
