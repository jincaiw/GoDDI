package facts

import (
	"context"
	"errors"
	"sync"
	"testing"

	"database/sql"
	_ "modernc.org/sqlite"
)

func newSequenceDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:sequence-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE facts_sequence_allocator (
		domain TEXT PRIMARY KEY,
		last_sequence INTEGER NOT NULL DEFAULT 0 CHECK (last_sequence >= 0)
	)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSequenceAllocatorAllocatesAndRollsBack(t *testing.T) {
	db := newSequenceDB(t)
	allocator, err := NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := allocator.NextTx(ctx, tx); err != nil || got != 1 {
		t.Fatalf("first sequence = %d, %v", got, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := allocator.Current(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Current after rollback = %v, want sql.ErrNoRows", err)
	}

	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := allocator.NextTx(ctx, tx)
	if err != nil || got != 1 {
		t.Fatalf("sequence after rollback = %d, %v", got, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got, err := allocator.Current(ctx); err != nil || got != 1 {
		t.Fatalf("Current = %d, %v", got, err)
	}
}

func TestSequenceAllocatorAllocatesContiguousSequences(t *testing.T) {
	db := newSequenceDB(t)
	allocator, err := NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for want := int64(1); want <= 3; want++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := allocator.NextTx(ctx, tx)
		if err != nil || got != want {
			t.Fatalf("sequence = %d, %v; want %d", got, err, want)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSequenceAllocatorConcurrentTransactionsRemainUnique(t *testing.T) {
	db := newSequenceDB(t)
	allocator, err := NewSequenceAllocator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const workers = 8
	got := make(chan int64, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				errs <- err
				return
			}
			seq, err := allocator.NextTx(ctx, tx)
			if err != nil {
				_ = tx.Rollback()
				errs <- err
				return
			}
			if err := tx.Commit(); err != nil {
				errs <- err
				return
			}
			got <- seq
		}()
	}
	wg.Wait()
	close(got)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for seq := range got {
		if seen[seq] {
			t.Fatalf("duplicate sequence %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != workers {
		t.Fatalf("allocated %d sequences, want %d", len(seen), workers)
	}
}
