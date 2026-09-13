package lease

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWALFileAppendDurableAssignsAndPersistsSequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.wal")
	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()

	event, err := wal.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l1"})
	if err != nil {
		t.Fatal(err)
	}
	if event.Seq != 1 || wal.Sequence() != 1 {
		t.Fatalf("event sequence=%d wal sequence=%d, want 1", event.Seq, wal.Sequence())
	}
	if _, err := wal.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l2"}); err != nil {
		t.Fatal(err)
	}
	if wal.Sequence() != 2 {
		t.Fatalf("sequence = %d, want 2", wal.Sequence())
	}
}

func TestWALFileReopensAndReplaysDurableRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.wal")
	first := Lease{ID: "l1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}

	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := wal.Append(WALEvent{Version: 1, Seq: 1, Op: WALEventUpsert, Lease: &first}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatal(err)
	}
	if got := wal.Sequence(); got != 1 {
		t.Fatalf("sequence = %d, want 1", got)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}

	wal, err = OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	var replayed []WALEvent
	last, err := wal.Replay(0, func(event WALEvent) error {
		replayed = append(replayed, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if last != 1 || len(replayed) != 1 || replayed[0].Lease == nil || replayed[0].Lease.ID != "l1" {
		t.Fatalf("replay last=%d events=%+v", last, replayed)
	}
}

func TestWALFileRejectsHeaderAndSequenceErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.wal")
	if err := os.WriteFile(path, []byte("not-a-goddi-wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWAL(path); !errors.Is(err, ErrWALHeader) {
		t.Fatalf("header error = %v, want ErrWALHeader", err)
	}

	path = filepath.Join(t.TempDir(), "gap.wal")
	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wal.file.WriteString(`{"version":1,"seq":2,"op":"remove","lease_id":"l1"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = wal.Close()
	if _, err := OpenWAL(path); !errors.Is(err, ErrWALGap) {
		t.Fatalf("gap error = %v, want ErrWALGap", err)
	}
}

func TestWALFileClosedOperationsFailClosed(t *testing.T) {
	wal, err := OpenWAL(filepath.Join(t.TempDir(), "leases.wal"))
	if err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := wal.Append(WALEvent{Version: 1, Seq: 1, Op: WALEventRemove, LeaseID: "l1"}); !errors.Is(err, ErrWALClosed) {
		t.Fatalf("append error = %v, want ErrWALClosed", err)
	}
	if err := wal.Sync(); !errors.Is(err, ErrWALClosed) {
		t.Fatalf("sync error = %v, want ErrWALClosed", err)
	}
	if _, err := wal.Replay(0, func(WALEvent) error { return nil }); !errors.Is(err, ErrWALClosed) {
		t.Fatalf("replay error = %v, want ErrWALClosed", err)
	}
	if _, err := wal.AppendDurable(WALEvent{Version: 1, Op: WALEventRemove, LeaseID: "l1"}); !errors.Is(err, ErrWALClosed) {
		t.Fatalf("append durable error = %v, want ErrWALClosed", err)
	}
}

func TestWALFileUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.wal")
	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions = %#o, want 0600", got)
	}
}
