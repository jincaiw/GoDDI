// Package facts defines the smallest shared event contract for the staged
// DHCP-DNS-IPAM convergence work. It is a protocol boundary, not yet the
// production source of truth or a replacement for the existing outboxes.
package facts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const CurrentEnvelopeVersion = 1

var (
	ErrInvalidEnvelope = errors.New("facts: invalid event envelope")
	ErrSequenceGap     = errors.New("facts: sequence gap")
)

// Envelope is the versioned identity and ordering wrapper shared by DHCP,
// DNS, and IPAM projections. Payload remains opaque to this package so each
// consumer can migrate independently during the compatibility period.
type Envelope struct {
	EventID        string          `json:"event_id"`
	Version        int             `json:"version"`
	Entity         string          `json:"entity"`
	Action         string          `json:"action"`
	Generation     int64           `json:"generation"`
	Sequence       int64           `json:"sequence"`
	Source         string          `json:"source"`
	OccurredAt     time.Time       `json:"occurred_at"`
	PayloadVersion int             `json:"payload_version"`
	Payload        json.RawMessage `json:"payload"`
}

func (e Envelope) Validate() error {
	if e.Version != CurrentEnvelopeVersion || e.PayloadVersion <= 0 ||
		e.EventID == "" || strings.TrimSpace(e.Entity) == "" ||
		strings.TrimSpace(e.Action) == "" || strings.TrimSpace(e.Source) == "" ||
		e.Generation < 0 || e.Sequence <= 0 || e.OccurredAt.IsZero() ||
		len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return fmt.Errorf("%w: event_id=%q sequence=%d", ErrInvalidEnvelope, e.EventID, e.Sequence)
	}
	return nil
}

func Encode(e Envelope) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func Decode(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return Envelope{}, fmt.Errorf("%w: decode: %v", ErrInvalidEnvelope, err)
	}
	if err := e.Validate(); err != nil {
		return Envelope{}, err
	}
	return e, nil
}

// Watermark accepts an event exactly once in sequence order. Replays at or
// below Applied are idempotent; a future event is rejected so a consumer cannot
// silently skip durable facts. The returned bool says whether the event was a
// new application rather than an idempotent replay.
type Watermark struct {
	Applied int64
}

func (w *Watermark) Apply(e Envelope) (bool, error) {
	if w == nil {
		return false, errors.New("facts: nil watermark")
	}
	if err := e.Validate(); err != nil {
		return false, err
	}
	if e.Sequence <= w.Applied {
		return false, nil
	}
	if e.Sequence != w.Applied+1 {
		return false, fmt.Errorf("%w: expected=%d got=%d", ErrSequenceGap, w.Applied+1, e.Sequence)
	}
	w.Applied = e.Sequence
	return true, nil
}
