// Package hasign verifies a deliberately restricted signed-document profile.
// It does not issue grants, validate takeover evidence, or authorize a role.
// The profile is a testable foundation, not a complete HA document schema.
package hasign

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const domain = "goddi.ha.signature.v1\x00"

// MaxKeyIDBytes limits the untrusted envelope key identifier before lookup.
const MaxKeyIDBytes = 256

// Kind identifies distinct signing purposes. A valid signature for one kind
// cannot be used as evidence of another kind.
type Kind string

const (
	TakeoverPermit  Kind = "takeover-permit"
	FreezeProof     Kind = "freeze-proof"
	RestoreManifest Kind = "restore-manifest"
)

var ErrInvalid = errors.New("hasign: invalid signed document")

// Envelope is an already-decoded transport envelope. Body is the original
// exact UTF-8 bytes, not a decoded and re-encoded JSON object. A transport
// decoder must itself reject duplicate and unknown envelope fields.
type Envelope struct {
	Body      []byte
	KeyID     string
	Algorithm string
	Signature string // unpadded canonical base64url
}

// TrustedKey is configured locally, never chosen or supplied by the document.
// Each key is authorized for exactly one document kind.
type TrustedKey struct {
	PublicKey ed25519.PublicKey
	Purpose   Kind
}

// Document is the deliberately minimal version-1 digest profile. Digest is an
// opaque 32-byte reference, not verified HA evidence and not a full permit.
type Document struct {
	Kind   Kind
	Digest [32]byte
}

func validKind(k Kind) bool {
	return k == TakeoverPermit || k == FreezeProof || k == RestoreManifest
}

// Verify validates the detached Ed25519 signature over domain || exact body.
// It only accepts JCS documents in the following conservative subset:
// {"digest":"<32-byte unpadded base64url>","kind":"<known kind>","version":"1"}.
// All keys and values are ASCII strings with no JSON escapes, in UTF-16/JCS
// order, and there is no whitespace. No number, Unicode, or nested JSON support
// is claimed. These restrictions make the exact canonical bytes unambiguous.
// Real permit, proof and manifest schemas require further independent review.
func Verify(env Envelope, expected Kind, keys map[string]TrustedKey) (Document, error) {
	var zero Document
	if !validKind(expected) || env.Algorithm != "Ed25519" || len(env.KeyID) == 0 || len(env.KeyID) > MaxKeyIDBytes {
		return zero, fmt.Errorf("%w: kind, algorithm or key ID", ErrInvalid)
	}
	key, ok := keys[env.KeyID]
	if !ok || key.Purpose != expected || len(key.PublicKey) != ed25519.PublicKeySize {
		return zero, fmt.Errorf("%w: unknown key or wrong purpose", ErrInvalid)
	}
	doc, err := parseBody(env.Body)
	if err != nil {
		return zero, err
	}
	if doc.Kind != expected {
		return zero, fmt.Errorf("%w: unexpected document kind", ErrInvalid)
	}
	sig, err := decode64(env.Signature, ed25519.SignatureSize)
	if err != nil {
		return zero, err
	}
	message := make([]byte, 0, len(domain)+len(env.Body))
	message = append(message, domain...)
	message = append(message, env.Body...)
	if !ed25519.Verify(key.PublicKey, message, sig) {
		return zero, fmt.Errorf("%w: signature", ErrInvalid)
	}
	return doc, nil
}

func decode64(s string, n int) ([]byte, error) {
	// Check length before decoding so attacker-controlled input cannot cause an
	// allocation proportional to its size. n is a fixed protocol byte length.
	if n < 0 || len(s) != base64.RawURLEncoding.EncodedLen(n) {
		return nil, fmt.Errorf("%w: invalid base64url length", ErrInvalid)
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != n || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, fmt.Errorf("%w: noncanonical base64url", ErrInvalid)
	}
	return b, nil
}

func parseBody(raw []byte) (Document, error) {
	var doc Document
	if len(raw) == 0 || len(raw) > 4096 || !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		return doc, fmt.Errorf("%w: encoding or size", ErrInvalid)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return doc, fmt.Errorf("%w: expected object", ErrInvalid)
	}
	values := make(map[string]string, 3)
	for dec.More() {
		tok, err = dec.Token()
		key, ok := tok.(string)
		if err != nil || !ok || (key != "digest" && key != "kind" && key != "version") {
			return doc, fmt.Errorf("%w: unknown key", ErrInvalid)
		}
		if _, exists := values[key]; exists {
			return doc, fmt.Errorf("%w: duplicate key", ErrInvalid)
		}
		var v any
		if err := dec.Decode(&v); err != nil {
			return doc, fmt.Errorf("%w: invalid value", ErrInvalid)
		}
		str, ok := v.(string)
		if !ok {
			return doc, fmt.Errorf("%w: expected string", ErrInvalid)
		}
		values[key] = str
	}
	if tok, err = dec.Token(); err != nil || tok != json.Delim('}') {
		return doc, fmt.Errorf("%w: incomplete object", ErrInvalid)
	}
	if _, err := dec.Token(); err != io.EOF {
		return doc, fmt.Errorf("%w: trailing data", ErrInvalid)
	}
	if len(values) != 3 || values["version"] != "1" || !validKind(Kind(values["kind"])) {
		return doc, fmt.Errorf("%w: missing field or unknown kind/version", ErrInvalid)
	}
	digest, err := decode64(values["digest"], 32)
	if err != nil {
		return doc, err
	}
	// The fields are fixed safe ASCII alphabets; this literal construction is
	// precisely their JCS encoding (no JSON escaping or number formatting).
	canonical := `{"digest":"` + values["digest"] + `","kind":"` + values["kind"] + `","version":"1"}`
	if !bytes.Equal(raw, []byte(canonical)) {
		return doc, fmt.Errorf("%w: noncanonical body", ErrInvalid)
	}
	doc.Kind = Kind(values["kind"])
	copy(doc.Digest[:], digest)
	return doc, nil
}
