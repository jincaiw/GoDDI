package snapshotipam

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/auditwire"
	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
)

func candidateText(fields map[string]json.RawMessage, name string, limit int) (string, error) {
	var s string
	raw := fields[name]
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &s) != nil || !utf8.ValidString(s) || len(s) > limit || strings.ContainsRune(s, 0) {
		return "", ErrInvalid
	}
	return s, nil
}
func candidateNumber(fields map[string]json.RawMessage, name string) (int64, error) {
	s, err := candidateText(fields, name, 19)
	if err != nil || len(s) == 0 || len(s) > 1 && s[0] == '0' {
		return 0, ErrInvalid
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, ErrInvalid
	}
	return n, nil
}
func candidateBlob(fields map[string]json.RawMessage, name string, max int) ([]byte, error) {
	s, err := candidateText(fields, name, base64.RawURLEncoding.EncodedLen(max))
	if err != nil {
		return nil, err
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) > max || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	return b, nil
}
func candidateExactBlob(fields map[string]json.RawMessage, name string, n int) ([]byte, error) {
	b, e := candidateBlob(fields, name, n)
	if e != nil || len(b) != n {
		return nil, ErrInvalid
	}
	return b, nil
}

// auditKey validates the current STATIC creation audit candidate, not all
// future audits or an independent proof that historical records are complete.
func auditKey(f map[string]json.RawMessage) ([]string, error) {
	cluster, err := candidateExactBlob(f, "cluster_id", 16)
	if err != nil {
		return nil, err
	}
	digest, err := candidateExactBlob(f, "request_digest", 32)
	if err != nil {
		return nil, err
	}
	epoch, err := candidateNumber(f, "epoch")
	if err != nil || epoch <= 0 {
		return nil, ErrInvalid
	}
	occurred, err := candidateNumber(f, "occurred_at_ms")
	if err != nil {
		return nil, err
	}
	prior, err := candidateNumber(f, "prior_revision")
	if err != nil || prior != 0 {
		return nil, ErrInvalid
	}
	revision, err := candidateNumber(f, "new_revision")
	if err != nil || revision != 1 {
		return nil, ErrInvalid
	}
	names := []string{"tenant_id", "space_id", "resource_id", "principal", "action", "request_id", "result"}
	vals := make([]string, len(names))
	for i, n := range names {
		vals[i], err = candidateText(f, n, maxTextBytes)
		if err != nil {
			return nil, err
		}
	}
	if vals[4] != auditwire.Action || vals[6] != auditwire.Result {
		return nil, ErrInvalid
	}
	var h [32]byte
	copy(h[:], digest)
	_, err = auditwire.Encode(auditwire.Audit{ClusterID: uuid.UUID(cluster).String(), Epoch: epoch, TenantID: vals[0], SpaceID: vals[1], ResourceID: vals[2], Principal: vals[3], RequestID: vals[5], OccurredAtMS: occurred, RequestDigest: h})
	if err != nil {
		return nil, ErrInvalid
	}
	// The only allowed new_revision is the INTEGER value 1, making its numeric
	// order equal for all rows rather than comparing decimal strings.
	return []string{vals[0], vals[1], vals[2], "1"}, nil
}

func receiptKey(f map[string]json.RawMessage) ([]string, error) {
	tenant, err := candidateExactBlob(f, "tenant_id", 16)
	if err != nil || uuid.UUID(tenant) == uuid.Nil {
		return nil, ErrInvalid
	}
	if _, err = candidateExactBlob(f, "request_digest", 32); err != nil {
		return nil, err
	}
	names := []string{"principal", "method", "path", "idempotency_key"}
	vals := make([]string, len(names))
	for i, n := range names {
		vals[i], err = candidateText(f, n, maxTextBytes)
		if err != nil {
			return nil, err
		}
	}
	principal, method, p, key := vals[0], vals[1], vals[2], vals[3]
	if len(principal) == 0 || len(principal) > idempotency.MaxPrincipalBytes || strings.TrimSpace(principal) != principal || strings.ContainsAny(principal, "\r\n") {
		return nil, ErrInvalid
	}
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, ErrInvalid
	}
	if len(p) == 0 || len(p) > idempotency.MaxPathBytes || !strings.HasPrefix(p, "/") || path.Clean(p) != p || strings.ContainsAny(p, "%?#\\\r\n") {
		return nil, ErrInvalid
	}
	u, err := url.ParseRequestURI(p)
	if err != nil || u.Path != p || u.RawQuery != "" {
		return nil, ErrInvalid
	}
	parsed, err := uuid.Parse(key)
	if err != nil || parsed == uuid.Nil || parsed.String() != key {
		return nil, ErrInvalid
	}
	created, err := candidateNumber(f, "created_at_unix_ms")
	if err != nil || created <= 0 {
		return nil, ErrInvalid
	}
	expires, err := candidateNumber(f, "expires_at_unix_ms")
	if err != nil || expires <= created {
		return nil, ErrInvalid
	}
	const maxResponse = 8 + idempotency.MaxETagBytes + idempotency.MaxBodyBytes
	response, err := candidateBlob(f, "response", maxResponse)
	if err != nil || len(response) < 8 || string(response[:4]) != "GIR1" {
		return nil, ErrInvalid
	}
	status := binary.BigEndian.Uint16(response[4:6])
	n := int(binary.BigEndian.Uint16(response[6:8]))
	if status < 100 || status > 599 || n > idempotency.MaxETagBytes || n > len(response)-8 || len(response)-8-n > idempotency.MaxBodyBytes {
		return nil, ErrInvalid
	}
	etag := string(response[8 : 8+n])
	if !utf8.ValidString(etag) || strings.ContainsAny(etag, "\x00\r\n") {
		return nil, ErrInvalid
	}
	// Go compares strings lexicographically by raw bytes, matching SQLite BLOB
	// ordering; base64url string ordering does not have this property.
	return []string{string(tenant), principal, method, p, key}, nil
}
