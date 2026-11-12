// Package auditwire defines an isolated candidate encoding for STATIC create
// audit. It does not authenticate, authorize, archive, or apply an event.
package auditwire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxBytes = 4096
const Action = "ipam.occupancy.create"
const Result = "SUCCESS"

var ErrInvalid = errors.New("auditwire: invalid candidate audit")

type Audit struct {
	ClusterID, TenantID, SpaceID, ResourceID, Principal, RequestID string
	Epoch, OccurredAtMS                                            int64
	RequestDigest                                                  [32]byte
}

type wire struct {
	Version       string `json:"version"`
	ClusterID     string `json:"clusterId"`
	Epoch         string `json:"epoch"`
	TenantID      string `json:"tenantId"`
	SpaceID       string `json:"spaceId"`
	ResourceID    string `json:"resourceId"`
	Principal     string `json:"principal"`
	Action        string `json:"action"`
	RequestID     string `json:"requestId"`
	PriorRevision string `json:"priorRevision"`
	NewRevision   string `json:"newRevision"`
	Result        string `json:"result"`
	OccurredAtMS  string `json:"occurredAtMs"`
	RequestDigest string `json:"requestDigest"`
}

func check(a Audit) error {
	for _, s := range []string{a.ClusterID, a.TenantID, a.SpaceID, a.ResourceID, a.RequestID} {
		id, err := uuid.Parse(s)
		if err != nil || id == uuid.Nil || id.String() != s {
			return ErrInvalid
		}
	}
	if a.Epoch <= 0 || a.OccurredAtMS < 0 || len(a.Principal) == 0 || len(a.Principal) > 256 || !utf8.ValidString(a.Principal) || strings.TrimSpace(a.Principal) != a.Principal || strings.ContainsAny(a.Principal, "\x00\r\n") {
		return ErrInvalid
	}
	return nil
}

// Encode produces deterministic candidate body bytes.
func Encode(a Audit) ([]byte, error) {
	if err := check(a); err != nil {
		return nil, err
	}
	w := wire{"1", a.ClusterID, strconv.FormatInt(a.Epoch, 10), a.TenantID, a.SpaceID, a.ResourceID, a.Principal, Action, a.RequestID, "0", "1", Result, strconv.FormatInt(a.OccurredAtMS, 10), base64.RawURLEncoding.EncodeToString(a.RequestDigest[:])}
	b, err := json.Marshal(w)
	if err != nil || len(b) > MaxBytes {
		return nil, ErrInvalid
	}
	return b, nil
}
func number(s string, positive bool) (int64, error) {
	if len(s) == 0 || len(s) > 19 || (len(s) > 1 && s[0] == '0') {
		return 0, ErrInvalid
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || (positive && n == 0) {
		return 0, ErrInvalid
	}
	return n, nil
}

// Decode rejects any bytes other than the exact canonical Encode output.
func Decode(raw []byte) (Audit, error) {
	var a Audit
	if len(raw) == 0 || len(raw) > MaxBytes || !utf8.Valid(raw) {
		return a, ErrInvalid
	}
	var w wire
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil || w.Version != "1" || w.Action != Action || w.Result != Result || w.PriorRevision != "0" || w.NewRevision != "1" {
		return a, ErrInvalid
	}
	epoch, err := number(w.Epoch, true)
	if err != nil {
		return a, err
	}
	at, err := number(w.OccurredAtMS, false)
	if err != nil {
		return a, err
	}
	if len(w.RequestDigest) != base64.RawURLEncoding.EncodedLen(32) {
		return a, ErrInvalid
	}
	digest, err := base64.RawURLEncoding.Strict().DecodeString(w.RequestDigest)
	if err != nil || len(digest) != 32 || base64.RawURLEncoding.EncodeToString(digest) != w.RequestDigest {
		return a, ErrInvalid
	}
	a = Audit{ClusterID: w.ClusterID, TenantID: w.TenantID, SpaceID: w.SpaceID, ResourceID: w.ResourceID, Principal: w.Principal, RequestID: w.RequestID, Epoch: epoch, OccurredAtMS: at}
	copy(a.RequestDigest[:], digest)
	canonical, err := Encode(a)
	if err != nil || !bytes.Equal(canonical, raw) {
		return Audit{}, ErrInvalid
	}
	return a, nil
}
