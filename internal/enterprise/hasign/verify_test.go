package hasign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// Public key and seed are RFC 8032 test vector 1. The signature below was
// independently generated with Python cryptography over domain+NUL+body.
const publicHex = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"
const signature = "pILuouepepz_0AHcJDatzKB1H6hYJ4Pe_gnN8gUAOXP3yMxGmPH0RYtgqG0zY-md1lgEh6uKP7PRoPCpCk7hBw"
const body = `{"digest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","kind":"takeover-permit","version":"1"}`

func fixture(t *testing.T) (Envelope, map[string]TrustedKey) {
	t.Helper()
	pub, err := hex.DecodeString(publicHex)
	if err != nil {
		t.Fatal(err)
	}
	return Envelope{[]byte(body), "k1", "Ed25519", signature}, map[string]TrustedKey{"k1": {ed25519.PublicKey(pub), TakeoverPermit}}
}

func TestKnownVector(t *testing.T) {
	e, keys := fixture(t)
	doc, err := Verify(e, TakeoverPermit, keys)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != TakeoverPermit || doc.Digest != [32]byte{} {
		t.Fatalf("unexpected document: %+v", doc)
	}
}

func TestRejectInvalid(t *testing.T) {
	tests := map[string]func(*Envelope, map[string]TrustedKey){
		"tamper digest": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, "AAAAAAAA", "BAAAAAAA", 1))
		},
		"invalid signature": func(e *Envelope, _ map[string]TrustedKey) {
			e.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
		},
		"padded signature": func(e *Envelope, _ map[string]TrustedKey) { e.Signature += "==" },
		"short signature":  func(e *Envelope, _ map[string]TrustedKey) { e.Signature = "AA" },
		"algorithm":        func(e *Envelope, _ map[string]TrustedKey) { e.Algorithm = "none" },
		"unknown key":      func(e *Envelope, _ map[string]TrustedKey) { e.KeyID = "unknown" },
		"wrong purpose": func(_ *Envelope, keys map[string]TrustedKey) {
			k := keys["k1"]
			k.Purpose = FreezeProof
			keys["k1"] = k
		},
		"short key": func(_ *Envelope, keys map[string]TrustedKey) {
			k := keys["k1"]
			k.PublicKey = []byte{1}
			keys["k1"] = k
		},
		"duplicate": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `"version":"1"`, `"version":"1","version":"1"`, 1))
		},
		"escaped duplicate": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `"version":"1"`, `"version":"1","vers\u0069on":"1"`, 1))
		},
		"unknown field": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `"version":"1"`, `"extra":"x","version":"1"`, 1))
		},
		"missing": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `,"version":"1"`, "", 1))
		},
		"unknown kind": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, "takeover-permit", "other", 1))
		},
		"different kind": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, "takeover-permit", "freeze-proof", 1))
		},
		"version": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `"version":"1"`, `"version":"2"`, 1))
		},
		"whitespace": func(e *Envelope, _ map[string]TrustedKey) { e.Body = []byte(" " + body) },
		"key order": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(`{"kind":"takeover-permit","digest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","version":"1"}`)
		},
		"escaped key": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `"version"`, `"vers\u0069on"`, 1))
		},
		"padded digest": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `AAA","kind"`, `AAA=","kind"`, 1))
		},
		"noncanonical base64 bits": func(e *Envelope, _ map[string]TrustedKey) {
			e.Body = []byte(strings.Replace(body, `AAA","kind"`, `AAB","kind"`, 1))
		},
		"bom":          func(e *Envelope, _ map[string]TrustedKey) { e.Body = append([]byte{0xef, 0xbb, 0xbf}, e.Body...) },
		"invalid utf8": func(e *Envelope, _ map[string]TrustedKey) { e.Body = append([]byte{0xff}, e.Body...) },
		"trailing":     func(e *Envelope, _ map[string]TrustedKey) { e.Body = append(e.Body, []byte(`{}`)...) },
		"too large":    func(e *Envelope, _ map[string]TrustedKey) { e.Body = []byte(strings.Repeat(" ", 4097)) },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			e, keys := fixture(t)
			change(&e, keys)
			if _, err := Verify(e, TakeoverPermit, keys); err == nil {
				t.Fatal("accepted invalid document")
			}
		})
	}
}

func TestInputLengthLimits(t *testing.T) {
	t.Run("huge signature", func(t *testing.T) {
		e, keys := fixture(t)
		e.Signature = strings.Repeat("A", 1<<20)
		if _, err := Verify(e, TakeoverPermit, keys); err == nil {
			t.Fatal("accepted huge signature")
		}
	})
	t.Run("key ID boundary", func(t *testing.T) {
		e, keys := fixture(t)
		key := keys[e.KeyID]
		e.KeyID = strings.Repeat("k", MaxKeyIDBytes)
		keys[e.KeyID] = key
		if _, err := Verify(e, TakeoverPermit, keys); err != nil {
			t.Fatalf("boundary key ID rejected: %v", err)
		}
		e.KeyID += "k"
		keys[e.KeyID] = key
		if _, err := Verify(e, TakeoverPermit, keys); err == nil {
			t.Fatal("accepted oversized registered key ID")
		}
		e.KeyID = strings.Repeat("k", 1<<20)
		if _, err := Verify(e, TakeoverPermit, keys); err == nil {
			t.Fatal("accepted huge key ID")
		}
	})
	t.Run("digest decoder length", func(t *testing.T) {
		if _, err := decode64(strings.Repeat("A", 1<<20), 32); err == nil {
			t.Fatal("accepted huge digest")
		}
	})
}

func TestExpectedPurposeIsMandatory(t *testing.T) {
	e, keys := fixture(t)
	for _, kind := range []Kind{"", "unknown", FreezeProof, RestoreManifest} {
		if _, err := Verify(e, kind, keys); err == nil {
			t.Fatalf("accepted kind %q", kind)
		}
	}
}

func TestAllKindsRecognized(t *testing.T) {
	seed, _ := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	private := ed25519.NewKeyFromSeed(seed)
	for _, kind := range []Kind{TakeoverPermit, FreezeProof, RestoreManifest} {
		raw := []byte(strings.Replace(body, "takeover-permit", string(kind), 1))
		msg := append([]byte(domain), raw...)
		e := Envelope{raw, "key", "Ed25519", base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, msg))}
		keys := map[string]TrustedKey{"key": {private.Public().(ed25519.PublicKey), kind}}
		if _, err := Verify(e, kind, keys); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}
