package facts

import (
	"errors"
	"testing"
	"time"
)

func testEnvelope(seq int64) Envelope {
	return Envelope{
		EventID:        "event-" + string(rune('0'+seq)),
		Version:        CurrentEnvelopeVersion,
		Entity:         "dhcp_lease",
		Action:         "bind",
		Generation:     1,
		Sequence:       seq,
		Source:         "dhcp",
		OccurredAt:     time.Unix(seq, 0).UTC(),
		PayloadVersion: 1,
		Payload:        []byte(`{"lease_id":"lease-1"}`),
	}
}

func TestEnvelopeRoundTripAndRejectsInvalidValues(t *testing.T) {
	encoded, err := Encode(testEnvelope(1))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != "event-1" || got.Sequence != 1 {
		t.Fatalf("decoded envelope = %+v", got)
	}
	bad := testEnvelope(1)
	bad.Payload = []byte(`not-json`)
	if _, err := Encode(bad); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("invalid payload error = %v", err)
	}
}

func TestWatermarkIsIdempotentAndRejectsGaps(t *testing.T) {
	var w Watermark
	if applied, err := w.Apply(testEnvelope(1)); err != nil || !applied {
		t.Fatalf("first apply = %v, %v", applied, err)
	}
	if applied, err := w.Apply(testEnvelope(1)); err != nil || applied {
		t.Fatalf("duplicate apply = %v, %v", applied, err)
	}
	if _, err := w.Apply(testEnvelope(3)); !errors.Is(err, ErrSequenceGap) {
		t.Fatalf("gap error = %v", err)
	}
	if w.Applied != 1 {
		t.Fatalf("watermark after gap = %d, want 1", w.Applied)
	}
	if applied, err := w.Apply(testEnvelope(2)); err != nil || !applied {
		t.Fatalf("second apply = %v, %v", applied, err)
	}
}
