// Package receiptwire defines a candidate receipt body, not an apply protocol.
// Successful decoding does not authenticate, authorize, or apply a receipt.
package receiptwire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
)

// MaxEncodedBytes limits this body only. The enclosing event payload has a
// separate 4 MiB limit, including all changes and JSON framing.
const MaxEncodedBytes = 2 << 20

var ErrInvalid = errors.New("receiptwire: invalid candidate receipt")

// Value binds the complete request scope, request digest, and exact response.
type Value struct {
	Scope         idempotency.Scope
	RequestDigest [32]byte
	Receipt       idempotency.Receipt
}

type wire struct {
	Version       string `json:"version"`
	Tenant        string `json:"tenantId"`
	Principal     string `json:"principal"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Key           string `json:"key"`
	RequestDigest string `json:"requestDigest"`
	Status        int    `json:"status"`
	ETag          string `json:"etag"`
	Body          string `json:"body"`
	Created       string `json:"createdAtMs"`
	Expires       string `json:"expiresAtMs"`
}

func valid(v Value) error {
	s, r := v.Scope, v.Receipt
	if s.Tenant == ([16]byte{}) || len(s.Principal) == 0 || len(s.Principal) > idempotency.MaxPrincipalBytes || !utf8.ValidString(s.Principal) || strings.TrimSpace(s.Principal) != s.Principal || strings.ContainsAny(s.Principal, "\x00\r\n") {
		return ErrInvalid
	}
	switch s.Method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return ErrInvalid
	}
	if len(s.Path) == 0 || len(s.Path) > idempotency.MaxPathBytes || !utf8.ValidString(s.Path) || !strings.HasPrefix(s.Path, "/") || path.Clean(s.Path) != s.Path || strings.ContainsAny(s.Path, "%?#\\\x00\r\n") {
		return ErrInvalid
	}
	u, err := url.ParseRequestURI(s.Path)
	if err != nil || u.Path != s.Path || u.RawQuery != "" {
		return ErrInvalid
	}
	id, err := uuid.Parse(s.Key)
	if err != nil || id == uuid.Nil || id.String() != s.Key {
		return ErrInvalid
	}
	if r.CreatedAtMS <= 0 || r.ExpiresAtMS <= r.CreatedAtMS || r.Response.Status < 100 || r.Response.Status > 599 || len(r.Response.ETag) > idempotency.MaxETagBytes || !utf8.ValidString(r.Response.ETag) || strings.ContainsAny(r.Response.ETag, "\x00\r\n") || len(r.Response.Body) > idempotency.MaxBodyBytes {
		return ErrInvalid
	}
	return nil
}

// Encode produces the exact canonical JSON candidate body.
func Encode(v Value) ([]byte, error) {
	if err := valid(v); err != nil {
		return nil, err
	}
	r := v.Receipt
	w := wire{Version: "1", Tenant: uuid.UUID(v.Scope.Tenant).String(), Principal: v.Scope.Principal, Method: v.Scope.Method, Path: v.Scope.Path, Key: v.Scope.Key, RequestDigest: base64.RawURLEncoding.EncodeToString(v.RequestDigest[:]), Status: r.Response.Status, ETag: r.Response.ETag, Body: base64.RawURLEncoding.EncodeToString(r.Response.Body), Created: strconv.FormatInt(r.CreatedAtMS, 10), Expires: strconv.FormatInt(r.ExpiresAtMS, 10)}
	b, err := json.Marshal(w)
	if err != nil || len(b) > MaxEncodedBytes {
		return nil, ErrInvalid
	}
	return b, nil
}

func decimal(s string) (int64, error) {
	if len(s) == 0 || len(s) > 19 || s[0] == '0' {
		return 0, ErrInvalid
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, ErrInvalid
	}
	return n, nil
}
func decode64(s string, max int) ([]byte, error) {
	if len(s) > base64.RawURLEncoding.EncodedLen(max) {
		return nil, ErrInvalid
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) > max || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	return b, nil
}

// Decode only accepts the exact encoding produced by Encode, including field
// order, escaping, and whitespace. It never authenticates or applies a receipt.
func Decode(raw []byte) (Value, error) {
	var v Value
	if len(raw) == 0 || len(raw) > MaxEncodedBytes || !utf8.Valid(raw) {
		return v, ErrInvalid
	}
	var w wire
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil || w.Version != "1" {
		return v, ErrInvalid
	}
	tenant, err := uuid.Parse(w.Tenant)
	if err != nil || tenant == uuid.Nil || tenant.String() != w.Tenant {
		return v, ErrInvalid
	}
	d, err := decode64(w.RequestDigest, 32)
	if err != nil || len(d) != 32 {
		return v, ErrInvalid
	}
	body, err := decode64(w.Body, idempotency.MaxBodyBytes)
	if err != nil {
		return v, ErrInvalid
	}
	created, err := decimal(w.Created)
	if err != nil {
		return v, ErrInvalid
	}
	expires, err := decimal(w.Expires)
	if err != nil {
		return v, ErrInvalid
	}
	v.Scope = idempotency.Scope{Tenant: [16]byte(tenant), Principal: w.Principal, Method: w.Method, Path: w.Path, Key: w.Key}
	copy(v.RequestDigest[:], d)
	v.Receipt = idempotency.Receipt{Response: idempotency.Response{Status: w.Status, ETag: w.ETag, Body: body}, CreatedAtMS: created, ExpiresAtMS: expires}
	canonical, err := Encode(v)
	if err != nil || !bytes.Equal(raw, canonical) {
		return Value{}, fmt.Errorf("%w: noncanonical or invalid body", ErrInvalid)
	}
	return v, nil
}
