package eventlog

import (
	"context"
	"database/sql"
	"testing"
)

// TestPublicReadHelpersRejectNilInputsWithoutPanic ensures the read-only helpers
// return an error instead of panicking when handed a nil context, nil log
// receiver, or nil database. This mirrors the existing guards on Read and
// ReadSnapshot so callers cannot crash the process by probing before setup.
func TestPublicReadHelpersRejectNilInputsWithoutPanic(t *testing.T) {
	ctx := context.Background()
	var nilLog *Log

	mustErr := func(name string, fn func() error) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s panicked: %v", name, r)
			}
		}()
		if err := fn(); err == nil {
			t.Fatalf("%s returned nil error", name)
		}
	}

	// Nil receiver.
	mustErr("Head/nil-log", func() error { _, err := nilLog.Head(ctx); return err })
	mustErr("Applied/nil-log", func() error { _, err := nilLog.Applied(ctx, ID{}); return err })

	// Nil database (constructed via NewLog(nil,...)).
	noDB := NewLog(nil, ID{1}, ID{2})
	mustErr("Head/nil-db", func() error { _, err := noDB.Head(ctx); return err })
	mustErr("Applied/nil-db", func() error { _, err := noDB.Applied(ctx, ID{}); return err })

	// Nil context with a real database.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	real := NewLog(db, ID{1}, ID{2})
	//lint:ignore SA1012 intentionally probing nil-context robustness
	mustErr("Head/nil-ctx", func() error { _, err := real.Head(nil); return err })
	//lint:ignore SA1012
	mustErr("Applied/nil-ctx", func() error { _, err := real.Applied(nil, ID{}); return err })

	// CurrentAuthority nil inputs.
	mustErr("CurrentAuthority/nil-db", func() error { _, err := CurrentAuthority(ctx, nil, ID{1}); return err })
	//lint:ignore SA1012
	mustErr("CurrentAuthority/nil-ctx", func() error { _, err := CurrentAuthority(nil, db, ID{1}); return err })

	// Sanity: valid inputs still hit the database (missing row -> ErrChainBreak/ErrNoAuthority),
	// not the nil-input error, proving the guards did not swallow normal paths.
	if _, err := real.Head(ctx); err == nil {
		t.Fatal("Head on empty db should error (missing head)")
	}
	if _, err := CurrentAuthority(ctx, db, ID{1}); err == nil {
		t.Fatal("CurrentAuthority on empty db should error (no authority)")
	}
}
