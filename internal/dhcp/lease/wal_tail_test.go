package lease

import (
	"errors"
	"strings"
	"testing"
)

func TestReplayWALRejectsIncompleteTailFailClosed(t *testing.T) {
	input := strings.NewReader(`{"version":1,"seq":1,"op":"remove","lease_id":"l1"}
{"version":1,"seq":2,"op":"upsert","lease":`)
	last, err := ReplayWAL(input, 0, func(WALEvent) error { return nil })
	if !errors.Is(err, ErrWALCorrupt) {
		t.Fatalf("error = %v, want ErrWALCorrupt", err)
	}
	if last != 1 {
		t.Fatalf("last = %d, want 1", last)
	}
}
