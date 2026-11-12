package auditwire

import (
	"bytes"
	"errors"
	"testing"
)

func auditSeed(t testing.TB) []byte {
	t.Helper()
	a := Audit{
		ClusterID:     "00000000-0000-0000-0000-0000000000a1",
		TenantID:      "00000000-0000-0000-0000-0000000000a2",
		SpaceID:       "00000000-0000-0000-0000-0000000000a3",
		ResourceID:    "00000000-0000-0000-0000-0000000000a4",
		Principal:     "p",
		RequestID:     "00000000-0000-0000-0000-0000000000a5",
		Epoch:         1,
		OccurredAtMS:  1,
		RequestDigest: [32]byte{1},
	}
	raw, err := Encode(a)
	if err != nil {
		t.Fatalf("encode seed: %v", err)
	}
	return raw
}

// TestAuditSeedIsValid keeps the fuzz seed round-tripping without an actual run.
func TestAuditSeedIsValid(t *testing.T) {
	raw := auditSeed(t)
	if _, err := Decode(raw); err != nil {
		t.Fatalf("seed does not decode: %v", err)
	}
}

// FuzzDecode asserts the untrusted audit-body parser never panics, always
// returns a typed ErrInvalid on rejection, and re-encodes accepted input to the
// identical canonical bytes. This guards the peer-supplied audit boundary.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("{}"))
	f.Add([]byte("null"))
	f.Add([]byte(`{"version":"1"}`))
	f.Add(auditSeed(f))

	f.Fuzz(func(t *testing.T, raw []byte) {
		a, err := Decode(raw)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Decode returned untyped error %v for %q", err, raw)
			}
			return
		}
		canon, cerr := Encode(a)
		if cerr != nil || !bytes.Equal(canon, raw) {
			t.Fatalf("accepted non-canonical audit: err=%v in=%q out=%q", cerr, raw, canon)
		}
	})
}
