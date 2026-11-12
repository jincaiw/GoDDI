package sqlitepolicy

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

type coderErr struct{ code int }

func (e coderErr) Error() string { return fmt.Sprintf("sqlite error %d", e.code) }
func (e coderErr) Code() int     { return e.code }

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"full", coderErr{sqliteFull}, ErrStorageFull},
		{"ioerr", coderErr{sqliteIOErr}, ErrStorageIO},
		{"busy", coderErr{sqliteBusy}, ErrBusy},
		{"locked", coderErr{sqliteLocked}, ErrBusy},
		{"enospc", fmt.Errorf("write: %w", syscall.ENOSPC), ErrStorageFull},
		{"eio", fmt.Errorf("write: %w", syscall.EIO), ErrStorageIO},
		{"msg full", errors.New("database or disk is full (13)"), ErrStorageFull},
		{"msg locked", errors.New("database is locked (5)"), ErrBusy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(tc.in)
			if !errors.Is(got, tc.want) {
				t.Fatalf("ClassifyError(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}

	// Nil and unrelated errors pass through unchanged.
	if ClassifyError(nil) != nil {
		t.Fatal("nil should classify to nil")
	}
	plain := errors.New("some other failure")
	if ClassifyError(plain) != plain {
		t.Fatal("unrelated error should pass through unchanged")
	}
}
