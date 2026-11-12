package dnswire

import (
	"bytes"
	"errors"
	"testing"
)

// TestDecodeSeedIsValid sanity-checks the seed document used by FuzzDecode so a
// change to the format that invalidates it is caught immediately, not just under
// an actual fuzzing run.
func TestDecodeSeedIsValid(t *testing.T) {
	raw := validSeedBytes(t)
	s, err := Decode(raw)
	if err != nil {
		t.Fatalf("seed does not decode: %v", err)
	}
	if !bytes.Equal(s.Bytes(), raw) {
		t.Fatal("decoded snapshot bytes differ from canonical input")
	}
}

func validSeedBytes(t testing.TB) []byte {
	t.Helper()
	doc := Document{
		Cluster:  "00000000-0000-0000-0000-0000000000a1",
		Node:     "00000000-0000-0000-0000-0000000000b2",
		Snapshot: "00000000-0000-0000-0000-0000000000c3",
		Version:  "1",
		View:     "internal",
		Zones: []Zone{{
			Name:   "example.test.",
			Serial: 1,
			Records: []Record{
				{Owner: "example.test.", Type: "SOA", RData: "ns.example.test. hostmaster.example.test. 1 3600 600 86400 60", TTL: 300},
				{Owner: "example.test.", Type: "NS", RData: "ns.example.test.", TTL: 300},
			},
		}},
	}
	s, err := Build(doc)
	if err != nil {
		t.Fatalf("build seed: %v", err)
	}
	return s.Bytes()
}

// FuzzDecode asserts the untrusted snapshot parser never panics, always returns
// a typed ErrInvalid on rejection (no raw parser errors escape), and, on
// acceptance, is canonical and idempotent: the accepted bytes re-decode to
// identical bytes. This guards the peer-supplied snapshot ingestion boundary.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("{}"))
	f.Add([]byte("null"))
	f.Add([]byte(`{"cluster":"x"}`))
	f.Add(validSeedBytes(f))

	f.Fuzz(func(t *testing.T, raw []byte) {
		s, err := Decode(raw)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Decode returned untyped error %v for %q", err, raw)
			}
			return
		}
		// Accepted input must be exactly canonical and idempotent.
		if !bytes.Equal(s.Bytes(), raw) {
			t.Fatalf("accepted non-canonical bytes: in=%q out=%q", raw, s.Bytes())
		}
		s2, err := Decode(s.Bytes())
		if err != nil || !bytes.Equal(s2.Bytes(), s.Bytes()) {
			t.Fatalf("re-decode not idempotent: err=%v", err)
		}
	})
}
