package lease

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type syncBuffer struct {
	bytes.Buffer
	syncErr error
}

func (b *syncBuffer) Sync() error { return b.syncErr }

func TestReplayWALIsOrderedAndIdempotent(t *testing.T) {
	first := Lease{ID: "l1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}
	var data bytes.Buffer
	if err := AppendWALEvent(&data, WALEvent{Version: 1, Seq: 1, Op: WALEventUpsert, Lease: &first}); err != nil {
		t.Fatal(err)
	}
	if err := AppendWALEvent(&data, WALEvent{Version: 1, Seq: 2, Op: WALEventRemove, LeaseID: "l1"}); err != nil {
		t.Fatal(err)
	}
	var applied []int64
	got, err := ReplayWAL(bytes.NewReader(data.Bytes()), 0, func(event WALEvent) error {
		applied = append(applied, event.Seq)
		return nil
	})
	if err != nil || got != 2 {
		t.Fatalf("replay = %d, %v", got, err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied = %v", applied)
	}
	got, err = ReplayWAL(bytes.NewReader(data.Bytes()), 1, func(WALEvent) error { return nil })
	if err != nil || got != 2 {
		t.Fatalf("idempotent replay = %d, %v", got, err)
	}
}

func TestReplayWALRejectsGapAndCorruption(t *testing.T) {
	var gap bytes.Buffer
	_ = AppendWALEvent(&gap, WALEvent{Version: 1, Seq: 2, Op: WALEventRemove, LeaseID: "l1"})
	if _, err := ReplayWAL(bytes.NewReader(gap.Bytes()), 0, func(WALEvent) error { return nil }); !errors.Is(err, ErrWALGap) {
		t.Fatalf("gap error = %v, want ErrWALGap", err)
	}
	if _, err := ReplayWAL(bytes.NewBufferString("{bad}\n"), 0, func(WALEvent) error { return nil }); !errors.Is(err, ErrWALCorrupt) {
		t.Fatalf("corrupt error = %v, want ErrWALCorrupt", err)
	}
}

func TestWALWriterRequiresSyncForDurableBoundary(t *testing.T) {
	file := &syncBuffer{}
	writer := NewWALWriter(file, 0)
	if err := writer.Append(WALEvent{Version: 1, Seq: 1, Op: WALEventRemove, LeaseID: "l1"}); err != nil {
		t.Fatal(err)
	}
	if writer.Sequence() != 1 {
		t.Fatalf("sequence = %d, want 1", writer.Sequence())
	}
	if err := writer.Sync(); err != nil {
		t.Fatal(err)
	}
	file.syncErr = io.ErrClosedPipe
	if err := writer.Sync(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("sync error = %v, want closed pipe", err)
	}
}

func TestWALWriterRejectsNonContiguousAppend(t *testing.T) {
	writer := NewWALWriter(&syncBuffer{}, 4)
	if err := writer.Append(WALEvent{Version: 1, Seq: 6, Op: WALEventRemove, LeaseID: "l1"}); !errors.Is(err, ErrWALGap) {
		t.Fatalf("append error = %v, want ErrWALGap", err)
	}
}
