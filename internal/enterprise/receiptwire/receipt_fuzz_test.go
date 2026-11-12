package receiptwire

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
)

func receiptSeed(t testing.TB) []byte {
	t.Helper()
	var tenant [16]byte
	tenant[0] = 1
	v := Value{
		Scope:         idempotency.Scope{Tenant: tenant, Principal: "p", Method: "POST", Path: "/x", Key: "00000000-0000-0000-0000-0000000000d4"},
		RequestDigest: [32]byte{1},
		Receipt: idempotency.Receipt{
			Response:    idempotency.Response{Status: 200, ETag: "e", Body: []byte("b")},
			CreatedAtMS: 1,
			ExpiresAtMS: 2,
		},
	}
	raw, err := Encode(v)
	if err != nil {
		t.Fatalf("encode seed: %v", err)
	}
	return raw
}

// TestReceiptSeedIsValid ensures the fuzz seed still round-trips so a format
// change that invalidates it is caught without an actual fuzzing run.
func TestReceiptSeedIsValid(t *testing.T) {
	raw := receiptSeed(t)
	if _, err := Decode(raw); err != nil {
		t.Fatalf("seed does not decode: %v", err)
	}
}

// FuzzDecode asserts the untrusted receipt-body parser never panics, always
// returns a typed ErrInvalid on rejection, and re-encodes accepted input to the
// identical canonical bytes. This guards the peer-supplied receipt boundary.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("{}"))
	f.Add([]byte("null"))
	f.Add([]byte(`{"version":"1"}`))
	f.Add(receiptSeed(f))

	f.Fuzz(func(t *testing.T, raw []byte) {
		v, err := Decode(raw)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Decode returned untyped error %v for %q", err, raw)
			}
			return
		}
		canon, cerr := Encode(v)
		if cerr != nil || !bytes.Equal(canon, raw) {
			t.Fatalf("accepted non-canonical receipt: err=%v in=%q out=%q", cerr, raw, canon)
		}
	})
}
