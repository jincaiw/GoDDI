package lease

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// walSpikeEvent is deliberately independent from Manager and SQLite. It is a
// contract probe for B8.6, not a production LeaseStore implementation.
type walSpikeEvent struct {
	Version int    `json:"version"`
	Seq     int64  `json:"seq"`
	Op      string `json:"op"`
	Lease   string `json:"lease_id"`
	Status  string `json:"status"`
}

var (
	errWALInvalidEvent = errors.New("wal spike: invalid event")
	errWALGap          = errors.New("wal spike: sequence gap")
)

func encodeWALSpikeEvent(e walSpikeEvent) ([]byte, error) {
	if e.Version != 1 || e.Seq <= 0 || e.Op == "" || e.Lease == "" {
		return nil, errWALInvalidEvent
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("encode WAL event: %w", err)
	}
	return append(payload, '\n'), nil
}

func replayWALSpike(data []byte, applied int64, state map[string]string) (int64, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e walSpikeEvent
		if err := json.Unmarshal(line, &e); err != nil {
			return applied, fmt.Errorf("decode WAL event: %w", err)
		}
		if e.Version != 1 || e.Seq <= 0 || e.Op == "" || e.Lease == "" {
			return applied, errWALInvalidEvent
		}
		if e.Seq <= applied {
			continue
		}
		if e.Seq != applied+1 {
			return applied, fmt.Errorf("%w: got %d after %d", errWALGap, e.Seq, applied)
		}
		if e.Op == "delete" {
			delete(state, e.Lease)
		} else if e.Op == "upsert" {
			state[e.Lease] = e.Status
		} else {
			return applied, errWALInvalidEvent
		}
		applied = e.Seq
	}
	if err := scanner.Err(); err != nil {
		return applied, fmt.Errorf("scan WAL: %w", err)
	}
	return applied, nil
}

func TestWALSpikeReplayIsOrderedAndIdempotent(t *testing.T) {
	events := []walSpikeEvent{
		{Version: 1, Seq: 1, Op: "upsert", Lease: "lease-1", Status: "active"},
		{Version: 1, Seq: 2, Op: "upsert", Lease: "lease-1", Status: "released"},
		{Version: 1, Seq: 3, Op: "delete", Lease: "lease-1"},
	}
	var data []byte
	for _, event := range events {
		encoded, err := encodeWALSpikeEvent(event)
		if err != nil {
			t.Fatalf("encode event %d: %v", event.Seq, err)
		}
		data = append(data, encoded...)
	}

	state := map[string]string{}
	applied, err := replayWALSpike(data, 0, state)
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if applied != 3 || len(state) != 0 {
		t.Fatalf("first replay applied=%d state=%v, want seq=3 and empty state", applied, state)
	}

	// Replaying the same prefix from its checkpoint is a no-op for every event.
	applied, err = replayWALSpike(data, applied, state)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if applied != 3 || len(state) != 0 {
		t.Fatalf("idempotent replay applied=%d state=%v", applied, state)
	}
}

func TestWALSpikeRejectsGapAndCorruption(t *testing.T) {
	gap1, _ := encodeWALSpikeEvent(walSpikeEvent{Version: 1, Seq: 1, Op: "upsert", Lease: "lease-1", Status: "active"})
	gap3, _ := encodeWALSpikeEvent(walSpikeEvent{Version: 1, Seq: 3, Op: "upsert", Lease: "lease-3", Status: "active"})
	_, err := replayWALSpike(append(gap1, gap3...), 0, map[string]string{})
	if !errors.Is(err, errWALGap) {
		t.Fatalf("gap replay error = %v, want %v", err, errWALGap)
	}

	_, err = replayWALSpike([]byte(`{"version":1,"seq":1,"op":"upsert","lease_id":"lease-1"`), 0, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "decode WAL event") {
		t.Fatalf("corrupt replay error = %v, want decode error", err)
	}
}

func TestWALSpikeRejectsInvalidEvents(t *testing.T) {
	for _, event := range []walSpikeEvent{
		{Version: 2, Seq: 1, Op: "upsert", Lease: "lease-1"},
		{Version: 1, Seq: 0, Op: "upsert", Lease: "lease-1"},
		{Version: 1, Seq: 1, Op: "", Lease: "lease-1"},
		{Version: 1, Seq: 1, Op: "upsert", Lease: ""},
	} {
		if _, err := encodeWALSpikeEvent(event); !errors.Is(err, errWALInvalidEvent) {
			t.Errorf("encode %+v error = %v, want invalid event", event, err)
		}
	}
}

func TestWALSpikeBoundedAdmissionRejectsWhenFull(t *testing.T) {
	queue := make(chan walSpikeEvent, 1)
	queue <- walSpikeEvent{Version: 1, Seq: 1, Op: "upsert", Lease: "lease-1"}
	select {
	case queue <- walSpikeEvent{Version: 1, Seq: 2, Op: "upsert", Lease: "lease-2"}:
		t.Fatal("bounded WAL queue accepted an event after becoming full")
	default:
	}

	// A full queue is an admission decision, not an implicit unbounded wait.
	if got := len(queue); got != cap(queue) {
		t.Fatalf("queue depth=%d capacity=%d, want full queue", got, cap(queue))
	}
}

func TestWALSpikeRejectsIncompleteTrailingRecord(t *testing.T) {
	complete, err := encodeWALSpikeEvent(walSpikeEvent{Version: 1, Seq: 1, Op: "upsert", Lease: "lease-1", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]string{}
	_, err = replayWALSpike(append(complete, []byte(`{"version":1,"seq":2,"op":"upsert","lease_id":"lease-2"`)...), 0, state)
	if err == nil || !strings.Contains(err.Error(), "decode WAL event") {
		t.Fatalf("incomplete trailing record error = %v, want decode error", err)
	}
	if state["lease-1"] != "active" {
		t.Fatalf("complete prefix was not applied before trailing corruption: %v", state)
	}
}
