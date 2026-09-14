package lease

import (
	"context"
	"errors"
	"testing"
)

func TestApplyWatermarkIsContiguousAndIdempotent(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	if err := EnsureAppliedWatermark(ctx, store.DB); err != nil {
		t.Fatalf("EnsureAppliedWatermark() error = %v", err)
	}

	apply := func(seq int64) (bool, error) {
		tx, err := store.DB.BeginTx(ctx, nil)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()
		changed, err := ApplyWatermark(ctx, tx, seq)
		if err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return changed, nil
	}

	changed, err := apply(1)
	if err != nil || !changed {
		t.Fatalf("apply(1) = changed=%v err=%v, want true nil", changed, err)
	}
	changed, err = apply(1)
	if err != nil || changed {
		t.Fatalf("duplicate apply(1) = changed=%v err=%v, want false nil", changed, err)
	}
	changed, err = apply(3)
	if !errors.Is(err, ErrWatermarkGap) || changed {
		t.Fatalf("gap apply(3) = changed=%v err=%v, want false ErrWatermarkGap", changed, err)
	}
	changed, err = apply(2)
	if err != nil || !changed {
		t.Fatalf("apply(2) = changed=%v err=%v, want true nil", changed, err)
	}
}

func TestApplyWatermarkRejectsNonPositiveSequence(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	if err := EnsureAppliedWatermark(ctx, store.DB); err != nil {
		t.Fatalf("EnsureAppliedWatermark() error = %v", err)
	}
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := ApplyWatermark(ctx, tx, 0); !errors.Is(err, ErrWatermarkRegression) {
		t.Fatalf("error = %v, want ErrWatermarkRegression", err)
	}
}

func TestEnsureAppliedWatermarkIsIdempotent(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	if err := EnsureAppliedWatermark(ctx, store.DB); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAppliedWatermark(ctx, store.DB); err != nil {
		t.Fatal(err)
	}
	var applied int64
	if err := store.DB.QueryRow(`SELECT applied_seq FROM lease_projection_watermark WHERE domain = ?`, leaseProjectionWatermarkDomain).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("applied_seq = %d, want 0", applied)
	}
}
