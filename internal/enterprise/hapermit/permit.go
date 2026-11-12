// Package hapermit verifies a candidate takeover-permit document. Verification
// never issues authority, consumes a nonce, validates fencing or changes roles.
package hapermit

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"
)

const domain = "goddi.ha.signature.v1\x00"
const MaxBodyBytes = 8192

var ErrInvalid = errors.New("hapermit: invalid candidate permit")

// Approval is a detached signature, presented in strictly ascending KeyID
// order. Signature is canonical unpadded base64url. Algorithm is fixed locally.
type Approval struct {
	KeyID     string
	Signature string
}

// Approver is configured locally with a stable human identity and approver
// purpose. The source of this keyring must be trusted independently.
type Approver struct {
	PublicKey ed25519.PublicKey
	Principal string
	Purpose   string
}

// Bounds are trusted UTC Unix milliseconds, inclusive lower and upper bounds.
// This package cannot establish the trust or freshness of these values.
type Bounds struct{ LowerMS, UpperMS int64 }

var names = []string{"anchorEpoch", "anchorHash", "anchorSeq", "candidateId", "clusterId", "fencingDigest", "kind", "newEpoch", "nonce", "notAfterMs", "notBeforeMs", "oldEpoch", "planDigest", "predecessorId", "requester", "version"}

func safe(s string) bool {
	if len(s) == 0 || len(s) > 256 {
		return false
	}
	for i := range s {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':' || c == '@') {
			return false
		}
	}
	return true
}
func id(s string) bool { u, e := uuid.Parse(s); return e == nil && u != uuid.Nil && u.String() == s }
func integer(s string, positive bool) (int64, error) {
	if s == "" || len(s) > 19 || len(s) > 1 && s[0] == '0' {
		return 0, ErrInvalid
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, ErrInvalid
		}
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || positive && n == 0 {
		return 0, ErrInvalid
	}
	return n, nil
}
func binary(s string, n int) ([]byte, error) {
	if len(s) != base64.RawURLEncoding.EncodedLen(n) {
		return nil, ErrInvalid
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(s)
	if e != nil || len(b) != n || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	return b, nil
}
func parse(raw []byte) (map[string]string, error) {
	if len(raw) == 0 || len(raw) > MaxBodyBytes || !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		return nil, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, e := dec.Token()
	if e != nil || token != json.Delim('{') {
		return nil, ErrInvalid
	}
	values := make(map[string]string, len(names))
	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[n] = true
	}
	for dec.More() {
		tok, e := dec.Token()
		k, ok := tok.(string)
		if e != nil || !ok || !allowed[k] {
			return nil, ErrInvalid
		}
		if _, ok = values[k]; ok {
			return nil, ErrInvalid
		}
		var v any
		if dec.Decode(&v) != nil {
			return nil, ErrInvalid
		}
		s, ok := v.(string)
		if !ok || !safe(s) {
			return nil, ErrInvalid
		}
		values[k] = s
	}
	if tok, e := dec.Token(); e != nil || tok != json.Delim('}') {
		return nil, ErrInvalid
	}
	if _, e = dec.Token(); e != io.EOF || len(values) != len(names) {
		return nil, ErrInvalid
	}
	// All keys/values are restricted to ASCII characters requiring no JSON
	// escaping. For this subset ASCII sort is RFC 8785 UTF-16 key order.
	keys := append([]string(nil), names...)
	sort.Strings(keys)
	var canon bytes.Buffer
	canon.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			canon.WriteByte(',')
		}
		canon.WriteByte('"')
		canon.WriteString(k)
		canon.WriteString(`":"`)
		canon.WriteString(values[k])
		canon.WriteByte('"')
	}
	canon.WriteByte('}')
	if !bytes.Equal(raw, canon.Bytes()) {
		return nil, ErrInvalid
	}
	return values, nil
}

// Verify checks document structure, two independent signatures and the whole
// trusted time interval. A nil error is not a role grant, fencing proof, nonce
// consumption, registry check, chain continuity proof or authorization handle.
func Verify(body []byte, approvals []Approval, keys map[string]Approver, now Bounds) error {
	v, e := parse(body)
	if e != nil {
		return e
	}
	if v["kind"] != "takeover-permit" || v["version"] != "1" {
		return ErrInvalid
	}
	for _, k := range []string{"clusterId", "predecessorId", "candidateId", "nonce"} {
		if !id(v[k]) {
			return ErrInvalid
		}
	}
	if v["clusterId"] == v["predecessorId"] || v["clusterId"] == v["candidateId"] || v["predecessorId"] == v["candidateId"] {
		return ErrInvalid
	}
	old, e := integer(v["oldEpoch"], true)
	if e != nil {
		return e
	}
	next, e := integer(v["newEpoch"], true)
	if e != nil || next <= old {
		return ErrInvalid
	}
	anchor, e := integer(v["anchorEpoch"], true)
	if e != nil || anchor != old {
		return ErrInvalid
	}
	seq, e := integer(v["anchorSeq"], false)
	if e != nil {
		return e
	}
	h, e := binary(v["anchorHash"], 32)
	if e != nil {
		return e
	}
	zero := bytes.Equal(h, make([]byte, 32))
	if seq == 0 && old == 1 && !zero || seq > 0 && zero {
		return ErrInvalid
	}
	for _, k := range []string{"planDigest", "fencingDigest"} {
		b, e := binary(v[k], 32)
		if e != nil || bytes.Equal(b, make([]byte, 32)) {
			return ErrInvalid
		}
	}
	before, e := integer(v["notBeforeMs"], false)
	if e != nil {
		return e
	}
	after, e := integer(v["notAfterMs"], false)
	if e != nil || after <= before || now.LowerMS < before || now.UpperMS >= after || now.LowerMS < 0 || now.UpperMS < now.LowerMS {
		return ErrInvalid
	}
	if len(approvals) != 2 || !safe(approvals[0].KeyID) || !safe(approvals[1].KeyID) || approvals[0].KeyID >= approvals[1].KeyID {
		return ErrInvalid
	}
	msg := make([]byte, 0, len(domain)+len(body))
	msg = append(msg, domain...)
	msg = append(msg, body...)
	var principals [2]string
	for i, a := range approvals {
		k, ok := keys[a.KeyID]
		if !ok || k.Purpose != "approver" || !safe(k.Principal) || k.Principal == v["requester"] || len(k.PublicKey) != ed25519.PublicKeySize {
			return ErrInvalid
		}
		sig, e := binary(a.Signature, ed25519.SignatureSize)
		if e != nil || !ed25519.Verify(k.PublicKey, msg, sig) {
			return fmt.Errorf("%w: approval", ErrInvalid)
		}
		principals[i] = k.Principal
	}
	if principals[0] == principals[1] || bytes.Equal(keys[approvals[0].KeyID].PublicKey, keys[approvals[1].KeyID].PublicKey) {
		return ErrInvalid
	}
	return nil
}
