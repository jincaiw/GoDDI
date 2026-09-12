package transfer

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/miekg/dns"
)

// TSIGAlgorithm represents a supported TSIG algorithm.
type TSIGAlgorithm string

const (
	TSIGHMACSHA256 TSIGAlgorithm = "hmac-sha256"
	TSIGHMACSHA512 TSIGAlgorithm = "hmac-sha512"
)

// SupportedTSIGAlgorithms lists all supported TSIG algorithms. MD5 and SHA1
// are intentionally excluded because they are cryptographically broken.
var SupportedTSIGAlgorithms = map[TSIGAlgorithm]bool{
	TSIGHMACSHA256: true,
	TSIGHMACSHA512: true,
}

// TSIGKey represents a TSIG key.
type TSIGKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Algorithm string `json:"algorithm"`
	Secret    string `json:"secret"` // Base64-encoded
}

// normalizeTSIGAlgorithm maps an operator-supplied algorithm name onto the
// stored form, rejecting anything we will not sign with.
//
// The two vocabularies differ by a trailing dot. The wire carries the
// canonical name from RFC 8945 — "hmac-sha256." — which is what miekg/dns
// exposes as dns.HmacSHA256, while the database and the API store
// "hmac-sha256". Accepting both here is deliberate: an operator copying a key
// definition from a BIND or Windows zone-transfer configuration will have the
// wire spelling.
//
// The previous implementation validated the stored name with the wire-form
// constant, so creating a key with the endpoint's own default algorithm always
// failed with "unsupported TSIG algorithm: hmac-sha256".
func normalizeTSIGAlgorithm(algorithm string) (string, error) {
	trimmed := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(algorithm)), ".")
	algo := TSIGAlgorithm(trimmed)
	if algo == "" {
		algo = TSIGHMACSHA256
	}
	if !SupportedTSIGAlgorithms[algo] {
		return "", fmt.Errorf("unsupported TSIG algorithm: %s (use hmac-sha256 or hmac-sha512)", algorithm)
	}
	return string(algo), nil
}

// CreateTSIGKey generates a new TSIG key with the specified name and algorithm.
// Returns the base64-encoded secret.
func CreateTSIGKey(name, algorithm string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("key name is required")
	}
	if _, err := normalizeTSIGAlgorithm(algorithm); err != nil {
		return "", err
	}

	// Generate a random secret (512 bits = 64 bytes).
	secret := make([]byte, 64)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generating random secret: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(secret)
	return encoded, nil
}

// VerifyTSIG verifies a TSIG message using the specified key. It delegates to
// the miekg/dns library, which implements the full RFC 2845 / RFC 4635 wire
// format. The signature covers the message body and the TSIG variables (key
// name, algorithm, time signed, fudge, MAC of the previous request) — not just
// the raw message bytes, as the previous implementation attempted to.
func VerifyTSIG(msg []byte, keyName string, secret string, algorithm string) bool {
	// The TSIG RR is required. miekg's TsigVerify parses the message to find
	// it and to read the algorithm that was actually used on the wire.
	dnsMsg := new(dns.Msg)
	if err := dnsMsg.Unpack(msg); err != nil {
		return false
	}

	t := dnsMsg.IsTsig()
	if t == nil {
		return false
	}

	// If a key name is supplied, it must match the TSIG RR's owner name.
	if keyName != "" && !equalFoldASCII(t.Hdr.Name, keyName) {
		return false
	}

	// Sanity-check that the algorithm on the wire is one we accept. This
	// also blocks legacy MD5/SHA1.
	if !isAlgorithmAccepted(t.Algorithm) {
		return false
	}

	// miekg's tsigHMACProvider expects a base64-encoded secret, which is
	// what the API stores, so the value can be passed through as-is.
	return dns.TsigVerify(msg, secret, "", false) == nil
}

// isAlgorithmAccepted reports whether the on-the-wire algorithm name is one we
// accept. We accept only SHA-256 and SHA-512.
func isAlgorithmAccepted(algo string) bool {
	switch algo {
	case dns.HmacSHA256, dns.HmacSHA512:
		return true
	}
	return false
}

// equalFoldASCII reports whether a and b are equal, ignoring case for ASCII
// letters (DNS names are case-insensitive).
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// ComputeHMAC is a compatibility shim. It now simply returns an empty string
// because the canonical wire-format HMAC must be computed by miekg/dns in
// order to cover the TSIG variables. Callers that need a TSIG MAC should use
// dns.TsigGenerate directly. This function is kept only so that other packages
// that import it continue to compile; it always returns "".
func ComputeHMAC(message []byte, secret string, algorithm TSIGAlgorithm) string {
	_ = message
	_ = secret
	_ = algorithm
	return ""
}

// GenerateTSIGKeyName generates a unique TSIG key name.
func GenerateTSIGKeyName() string {
	return fmt.Sprintf("key-%s", uuid.New().String()[:8])
}
