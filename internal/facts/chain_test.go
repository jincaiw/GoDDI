package facts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func mkEnv(t *testing.T, seq int64, payload string) Envelope {
	t.Helper()
	return Envelope{
		EventID: "evt-" + payload, Version: CurrentEnvelopeVersion, Entity: "lease",
		Action: "bind", Generation: 1, Sequence: seq, Source: "node-a",
		OccurredAt: time.Unix(1700000000+seq, 0).UTC(), PayloadVersion: 1,
		Payload: json.RawMessage(payload),
	}
}

// TestChainStampVerifyDetectsTamper proves a stamped chain verifies, and that
// altering a payload, reordering, or dropping an event is detected.
func TestChainStampVerifyDetectsTamper(t *testing.T) {
	b := NewChainBuilder("domain", ChainTip{})
	var envs []Envelope
	for i := int64(1); i <= 4; i++ {
		e, err := b.Stamp(mkEnv(t, i, `{"ip":"192.0.2.`+string(rune('0'+i))+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		envs = append(envs, e)
	}
	if err := VerifyChain(envs); err != nil {
		t.Fatalf("clean chain failed: %v", err)
	}

	tampered := append([]Envelope(nil), envs...)
	tampered[2].Payload = json.RawMessage(`{"ip":"192.0.2.99"}`)
	if err := VerifyChain(tampered); !errors.Is(err, ErrChainFork) {
		t.Fatalf("tampered payload not detected: %v", err)
	}

	reordered := []Envelope{envs[0], envs[2], envs[1], envs[3]}
	if err := VerifyChain(reordered); !errors.Is(err, ErrChainFork) {
		t.Fatalf("reorder not detected: %v", err)
	}

	dropped := []Envelope{envs[0], envs[1], envs[3]}
	if err := VerifyChain(dropped); !errors.Is(err, ErrChainFork) {
		t.Fatalf("drop/gap not detected: %v", err)
	}
}

func TestVerifyEnvelopeBackwardCompatible(t *testing.T) {
	// An unstamped (legacy) envelope carries no chain fields; verification is
	// a no-op and returns the previous tip unchanged.
	prev := ChainTip{Epoch: 3, Tip: "abc"}
	next, err := VerifyEnvelope(prev, mkEnv(t, 1, `{}`))
	if err != nil || next != prev {
		t.Fatalf("legacy envelope: next=%v err=%v, want unchanged", next, err)
	}
}

func TestChainTipStoreAdvanceAndFork(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	ctx := context.Background()
	store := NewChainTipStore(db)
	if err := store.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	// First advance from empty tip.
	tx, _ := db.BeginTx(ctx, nil)
	if err := store.AdvanceTx(ctx, tx, "d", "", ChainTip{Epoch: 1, Tip: "t1"}); err != nil {
		t.Fatalf("first advance: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// Correct advance.
	tx2, _ := db.BeginTx(ctx, nil)
	if err := store.AdvanceTx(ctx, tx2, "d", "t1", ChainTip{Epoch: 1, Tip: "t2"}); err != nil {
		t.Fatalf("second advance: %v", err)
	}
	_ = tx2.Commit()
	// Stale expected tip -> fork. Roll back before reading on the same
	// single-connection in-memory DB (holding the tx would deadlock the pool).
	tx3, _ := db.BeginTx(ctx, nil)
	forkErr := store.AdvanceTx(ctx, tx3, "d", "t1", ChainTip{Epoch: 1, Tip: "t3"})
	_ = tx3.Rollback()
	if !errors.Is(forkErr, ErrChainFork) {
		t.Fatalf("stale advance not rejected: %v", forkErr)
	}
	var tip string
	if err := db.QueryRow(`SELECT tip FROM facts_chain_tip WHERE domain='d'`).Scan(&tip); err != nil || tip != "t2" {
		t.Fatalf("tip=%q err=%v, want t2 (fork must not change it)", tip, err)
	}
}

// TestChainTipStoreStampTx proves the producer integration point: two events
// stamped in separate transactions extend one linear chain.
func TestChainTipStoreStampTx(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	defer db.Close()
	ctx := context.Background()
	store := NewChainTipStore(db)
	if err := store.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	var stamped []Envelope
	for i := int64(1); i <= 2; i++ {
		tx, _ := db.BeginTx(ctx, nil)
		e, err := store.StampTx(ctx, tx, "d", mkEnv(t, i, `{"i":`+string(rune('0'+i))+`}`))
		if err != nil {
			t.Fatalf("stamp %d: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		stamped = append(stamped, e)
	}
	if err := VerifyChain(stamped); err != nil {
		t.Fatalf("chain from StampTx did not verify: %v", err)
	}
}
