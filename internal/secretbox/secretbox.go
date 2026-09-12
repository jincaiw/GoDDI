// Package secretbox seals small secrets (TOTP seeds, TSIG keys, DNSSEC
// private keys) for storage in the database.
//
// Every caller names its own purpose through the label passed to New. The
// label is mixed into the key derivation, so two purposes that happen to be
// configured with the same key material still never share a keystream — and
// a ciphertext cannot be moved from one column to another without the move
// being noticed.
//
// The on-the-wire format is "enc:v1:" followed by the raw-standard base64 of
// nonce||ciphertext. That is the format the TOTP store shipped with, so
// adopting this package does not re-encrypt anything: a TOTP secret written
// before this package existed still opens, provided the label and key
// material resolve the same way they did then.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Prefix identifies a sealed value. Values without it are returned verbatim
// by Open, which is what makes an installation that never configured key
// material behave exactly as it did before sealing existed.
const Prefix = "enc:v1:"

// ErrNoKeyMaterial is returned by NewRequired when there is nothing to
// derive a key from.
var ErrNoKeyMaterial = errors.New("secretbox: no key material configured")

// LabelTOTP, LabelTSIG and LabelSnapshot are the purposes this build seals.
// They are constants rather than literals at the call sites so that a typo
// cannot silently produce a second, unreadable key space.
//
// LabelSnapshot does not sit on a database column; it names the key that
// encrypts a whole backup archive. It is here rather than in the backup
// package for the same reason the other two are here: the label is part of
// the derivation, so the archive key and the column keys can be derived from
// the same configured material while remaining different keys.
const (
	LabelTOTP     = "goddi-totp-v1"
	LabelTSIG     = "goddi-tsig-v1"
	LabelSnapshot = "goddi-snapshot-v1"
)

// Sealer encrypts and decrypts secrets with AES-256-GCM.
//
// The zero Sealer — and any Sealer built from empty key material — is
// disabled: Seal returns its input unchanged and Open returns the stored
// value as-is. Callers that must not fall back to plaintext use NewRequired.
type Sealer struct {
	label string
	aead  cipher.AEAD
}

// Derive turns key material plus a purpose label into a 32-byte AES key.
//
// The label prefix is part of the derivation, so this is exactly the
// sha256(label + ":" + keyMaterial) that the TOTP store used before this
// package existed.
func Derive(label, keyMaterial string) []byte {
	sum := sha256.Sum256([]byte(label + ":" + keyMaterial))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

// New returns a Sealer for the given purpose. Empty key material yields a
// disabled Sealer rather than an error; see the type comment.
func New(label, keyMaterial string) *Sealer {
	if keyMaterial == "" {
		return &Sealer{label: label}
	}
	block, err := aes.NewCipher(Derive(label, keyMaterial))
	if err != nil {
		// Derive always yields 32 bytes, so this cannot happen; returning a
		// disabled sealer is still better than panicking on a code path that
		// guards credential storage.
		return &Sealer{label: label}
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return &Sealer{label: label}
	}
	return &Sealer{label: label, aead: gcm}
}

// NewRequired is New, except that empty key material is an error. Use it
// where storing a secret in the clear would be a defect rather than a
// documented fallback.
func NewRequired(label, keyMaterial string) (*Sealer, error) {
	if keyMaterial == "" {
		return nil, fmt.Errorf("%w (label %q)", ErrNoKeyMaterial, label)
	}
	s := New(label, keyMaterial)
	if !s.Enabled() {
		return nil, fmt.Errorf("secretbox: could not initialise the AEAD for label %q", label)
	}
	return s, nil
}

// Enabled reports whether this Sealer actually encrypts.
func (s *Sealer) Enabled() bool { return s != nil && s.aead != nil }

// Label returns the purpose label this Sealer was built with.
func (s *Sealer) Label() string {
	if s == nil {
		return ""
	}
	return s.label
}

// IsSealed reports whether a stored value carries the sealed prefix.
func IsSealed(value string) bool { return strings.HasPrefix(value, Prefix) }

// Seal encrypts plaintext. A disabled Sealer returns the input unchanged.
func (s *Sealer) Seal(plaintext string) (string, error) {
	if !s.Enabled() {
		return plaintext, nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secretbox: generating nonce: %w", err)
	}
	sealed := s.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return Prefix + base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// Open decrypts a stored value. Values that do not carry the sealed prefix
// are returned unchanged, so a column that predates sealing keeps working.
func (s *Sealer) Open(value string) (string, error) {
	if !IsSealed(value) {
		return value, nil
	}
	if !s.Enabled() {
		return "", fmt.Errorf("secretbox: value is sealed but no key material is configured (label %q)", s.Label())
	}
	return s.openSealed(value)
}

// OpenSealed decrypts a value that must carry the sealed prefix. It is the
// strict counterpart to Open: a plaintext value is an error rather than a
// pass-through, which is how a caller proves it is not silently reading
// something an operator left in the clear.
func (s *Sealer) OpenSealed(value string) (string, error) {
	if !IsSealed(value) {
		return "", fmt.Errorf("secretbox: value is not sealed (label %q)", s.Label())
	}
	if !s.Enabled() {
		return "", fmt.Errorf("secretbox: value is sealed but no key material is configured (label %q)", s.Label())
	}
	return s.openSealed(value)
}

func (s *Sealer) openSealed(value string) (string, error) {
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, Prefix))
	if err != nil {
		return "", fmt.Errorf("secretbox: decoding sealed value: %w", err)
	}
	if len(payload) < s.aead.NonceSize() {
		return "", errors.New("secretbox: sealed value is shorter than its nonce")
	}
	nonce, ciphertext := payload[:s.aead.NonceSize()], payload[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: decrypting sealed value: %w", err)
	}
	return string(plaintext), nil
}

// Fingerprint returns a short, stable identifier for a secret, suitable for
// display in a list response. It is derived from the plaintext, so an
// operator can tell two keys apart without either value being retrievable
// from the response.
func Fingerprint(plaintext string) string {
	sum := sha256.Sum256([]byte("goddi-fingerprint-v1:" + plaintext))
	return base64.RawStdEncoding.EncodeToString(sum[:8])
}
