// Package rolelog validates and persists a local role record and compares an
// independently supplied authority view. It does not verify signatures, fence
// nodes, or grant write access. Comparison is only one input to the HA gate.
package rolelog

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxRecordBytes bounds local record parsing before decoding untrusted input.
const MaxRecordBytes = 16 << 10

var ErrInvalid = errors.New("rolelog: invalid record")
var ErrMismatch = errors.New("rolelog: authority mismatch")

// Record is a version 1 local record. Decimal integers are strings so their
// exact value survives JSON implementations with limited numeric precision.
// Digest values are lower-case SHA-256 hex; IDs and nonce are canonical UUIDs.
type Record struct {
	Version        string
	ClusterID      string
	NodeID         string
	DeviceID       string
	Epoch          uint64
	Phase          string
	Nonce          string
	GrantDigest    string
	EvidenceDigest string
	DBGeneration   string
}

// AuthorityView contains independently acquired comparison values. In
// particular, today's writer_authority table does not contain all these fields;
// callers must obtain the missing values from their authoritative sources.
// Constructing a view or a successful comparison never grants write permission.
type AuthorityView struct {
	ClusterID      string
	NodeID         string
	DeviceID       string
	Epoch          uint64
	Phase          string
	Nonce          string
	GrantDigest    string
	EvidenceDigest string
	DBGeneration   string
}

var fields = map[string]bool{
	"version": true, "cluster_id": true, "node_id": true, "device_id": true,
	"epoch": true, "phase": true, "nonce": true, "grant_digest": true,
	"evidence_digest": true, "db_generation": true,
}

// Parse rejects malformed UTF-8, duplicate or unknown keys, missing values,
// non-string fields and trailing JSON. It performs no I/O or authentication.
func Parse(raw []byte) (Record, error) {
	var record Record
	if len(raw) > MaxRecordBytes {
		return record, fmt.Errorf("%w: record exceeds size limit", ErrInvalid)
	}
	if !utf8.Valid(raw) {
		return record, fmt.Errorf("%w: invalid UTF-8", ErrInvalid)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return record, fmt.Errorf("%w: expected object", ErrInvalid)
	}
	values := make(map[string]string, len(fields))
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return record, fmt.Errorf("%w: malformed key", ErrInvalid)
		}
		key, ok := tok.(string)
		if !ok || !fields[key] {
			return record, fmt.Errorf("%w: unknown field", ErrInvalid)
		}
		if _, exists := values[key]; exists {
			return record, fmt.Errorf("%w: duplicate field %q", ErrInvalid, key)
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return record, fmt.Errorf("%w: malformed field %q", ErrInvalid, key)
		}
		s, ok := value.(string)
		if !ok {
			return record, fmt.Errorf("%w: field %q must be a string", ErrInvalid, key)
		}
		values[key] = s
	}
	if tok, err = dec.Token(); err != nil || tok != json.Delim('}') {
		return record, fmt.Errorf("%w: malformed object", ErrInvalid)
	}
	if _, err := dec.Token(); err != io.EOF {
		return record, fmt.Errorf("%w: trailing data", ErrInvalid)
	}
	if len(values) != len(fields) {
		return record, fmt.Errorf("%w: missing field", ErrInvalid)
	}
	if values["version"] != "1" {
		return record, fmt.Errorf("%w: unsupported version", ErrInvalid)
	}
	epochText := values["epoch"]
	if epochText == "" || epochText[0] == '0' || strings.Trim(epochText, "0123456789") != "" {
		return record, fmt.Errorf("%w: invalid epoch", ErrInvalid)
	}
	epoch, err := strconv.ParseUint(epochText, 10, 63)
	if err != nil || epoch == 0 || epoch > math.MaxInt64 {
		return record, fmt.Errorf("%w: invalid epoch", ErrInvalid)
	}
	record = Record{Version: values["version"], ClusterID: values["cluster_id"], NodeID: values["node_id"],
		DeviceID: values["device_id"], Epoch: epoch, Phase: values["phase"], Nonce: values["nonce"],
		GrantDigest: values["grant_digest"], EvidenceDigest: values["evidence_digest"], DBGeneration: values["db_generation"]}
	if err := validate(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func validate(r Record) error {
	if r.Version != "1" || r.Epoch == 0 || r.Epoch > math.MaxInt64 {
		return fmt.Errorf("%w: version or epoch", ErrInvalid)
	}
	for _, s := range []string{r.ClusterID, r.NodeID, r.Nonce} {
		id, err := uuid.Parse(s)
		if err != nil || id == uuid.Nil || id.String() != s {
			return fmt.Errorf("%w: noncanonical or nil UUID", ErrInvalid)
		}
	}
	if r.ClusterID == r.NodeID {
		return fmt.Errorf("%w: cluster and node identities coincide", ErrInvalid)
	}
	for _, s := range []string{r.DeviceID, r.DBGeneration} {
		if len(s) == 0 || len(s) > 256 || strings.TrimSpace(s) != s || !utf8.ValidString(s) || strings.ContainsAny(s, "\x00\r\n") {
			return fmt.Errorf("%w: invalid identity or generation", ErrInvalid)
		}
	}
	switch r.Phase {
	case "BOOTSTRAP", "STANDBY", "ACTIVE", "DEGRADED", "PROMOTING", "FENCED":
	default:
		return fmt.Errorf("%w: unknown phase", ErrInvalid)
	}
	for _, s := range []string{r.GrantDigest, r.EvidenceDigest} {
		if len(s) != 64 || strings.ToLower(s) != s {
			return fmt.Errorf("%w: invalid digest", ErrInvalid)
		}
		b, err := hex.DecodeString(s)
		if err != nil || bytes.Equal(b, make([]byte, 32)) {
			return fmt.Errorf("%w: invalid digest", ErrInvalid)
		}
	}
	return nil
}

// Compare checks all fields against an independently supplied view. It is not
// a signature check, role transition, fencing check, or permission decision.
func Compare(r Record, a AuthorityView) error {
	if err := validate(r); err != nil {
		return err
	}
	if r.ClusterID != a.ClusterID || r.NodeID != a.NodeID || r.DeviceID != a.DeviceID ||
		r.Epoch != a.Epoch || r.Phase != a.Phase || r.Nonce != a.Nonce ||
		r.GrantDigest != a.GrantDigest || r.EvidenceDigest != a.EvidenceDigest || r.DBGeneration != a.DBGeneration {
		return ErrMismatch
	}
	return nil
}
