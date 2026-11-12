package ipamevent

import (
	"encoding/json"
	"testing"
)

// ipameventSeed builds a valid full eventlog payload carrying one occupancy
// change, using the package's own encoder so the seed tracks the format.
func ipameventSeed(tb testing.TB) []byte {
	tb.Helper()
	occ := Occupancy{
		TenantID:    "00000000-0000-0000-0000-0000000000a1",
		SpaceID:     "00000000-0000-0000-0000-0000000000a2",
		ID:          "00000000-0000-0000-0000-0000000000a3",
		SubnetID:    "00000000-0000-0000-0000-0000000000a4",
		Family:      4,
		Address:     "192.0.2.10",
		Kind:        "STATIC",
		OwnerRef:    "owner-1",
		CreatedAtMS: "1",
		Revision:    "1",
	}
	change, err := EncodeChange(Set, occ)
	if err != nil {
		tb.Fatalf("encode change: %v", err)
	}
	wireChange := struct {
		Kind     string          `json:"kind"`
		ObjectID string          `json:"objectId"`
		Revision string          `json:"revision"`
		Body     json.RawMessage `json:"body"`
	}{change.Kind, change.ObjectID, change.Revision, change.Body}
	payload, err := json.Marshal(struct {
		Changes []any `json:"changes"`
	}{[]any{wireChange}})
	if err != nil {
		tb.Fatalf("marshal payload: %v", err)
	}
	return payload
}

// TestIpameventSeedIsValid keeps the fuzz seed round-tripping.
func TestIpameventSeedIsValid(t *testing.T) {
	raw := ipameventSeed(t)
	changes, err := DecodePayload(raw)
	if err != nil {
		t.Fatalf("seed does not decode: %v", err)
	}
	if len(changes) != 1 || changes[0].Kind != Set {
		t.Fatalf("unexpected decoded changes: %+v", changes)
	}
}

// FuzzDecodePayload asserts the untrusted occupancy-payload parser never panics
// and is deterministic: a successful decode repeats identically. This guards the
// peer-supplied event payload boundary.
func FuzzDecodePayload(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("{}"))
	f.Add([]byte("null"))
	f.Add([]byte(`{"changes":[]}`))
	f.Add([]byte(`{"changes":[{"kind":"x"}]}`))
	f.Add(ipameventSeed(f))

	f.Fuzz(func(t *testing.T, raw []byte) {
		changes, err := DecodePayload(raw)
		if err != nil {
			return
		}
		if len(changes) == 0 {
			t.Fatalf("successful decode returned no changes for %q", raw)
		}
		again, err2 := DecodePayload(raw)
		if err2 != nil || len(again) != len(changes) {
			t.Fatalf("non-deterministic decode: %v vs %v", err, err2)
		}
	})
}
