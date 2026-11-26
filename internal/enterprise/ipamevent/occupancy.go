// Package ipamevent defines a first, isolated occupancy event contract.
// Validation is not authorization, a database mutation, or replica apply.
package ipamevent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamaddr"
)

const Set = "ipam.occupancy.set"
const Cleared = "ipam.occupancy.cleared"

var ErrInvalid = errors.New("ipamevent: invalid occupancy event")

// Occupancy contains the full resulting row for set, or the exact deleted row
// for cleared. Decimal fields are strings to preserve signed SQLite precision.
type Occupancy struct {
	TenantID    string  `json:"tenantId"`
	SpaceID     string  `json:"spaceId"`
	ID          string  `json:"id"`
	SubnetID    string  `json:"subnetId"`
	Family      int     `json:"family"`
	Address     string  `json:"address"`
	Kind        string  `json:"kind"`
	OwnerRef    string  `json:"ownerRef"`
	CreatedAtMS string  `json:"createdAtMs"`
	ExpiresAtMS *string `json:"expiresAtMs"`
	Revision    string  `json:"revision"`
}

var bodyFields = map[string]bool{"tenantId": true, "spaceId": true, "id": true, "subnetId": true, "family": true, "address": true, "kind": true, "ownerRef": true, "createdAtMs": true, "expiresAtMs": true, "revision": true}

func integer(s string, positive bool) (int64, error) {
	if s == "" || len(s) > 19 || (len(s) > 1 && s[0] == '0') {
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

func (o Occupancy) Validate() error {
	for _, value := range []string{o.TenantID, o.SpaceID, o.ID, o.SubnetID} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return fmt.Errorf("%w: UUID", ErrInvalid)
		}
	}
	addr, err := ipamaddr.ParseAddr(o.Address)
	if err != nil || addr.String() != o.Address || (o.Family == 4 && !addr.Is4()) || (o.Family == 6 && !addr.Is6()) || (o.Family != 4 && o.Family != 6) {
		return fmt.Errorf("%w: family or address", ErrInvalid)
	}
	// Restrict owner references to printable ASCII in this initial schema;
	// identifiers do not depend on JSON Unicode normalization or escaping.
	if len(o.OwnerRef) == 0 || len(o.OwnerRef) > 1024 {
		return fmt.Errorf("%w: owner", ErrInvalid)
	}
	for i := range o.OwnerRef {
		if o.OwnerRef[i] < 0x20 || o.OwnerRef[i] > 0x7e {
			return fmt.Errorf("%w: owner", ErrInvalid)
		}
	}
	created, err := integer(o.CreatedAtMS, false)
	if err != nil {
		return fmt.Errorf("%w: creation time", ErrInvalid)
	}
	if _, err := integer(o.Revision, true); err != nil {
		return fmt.Errorf("%w: revision", ErrInvalid)
	}
	switch o.Kind {
	case "HOLD":
		if o.ExpiresAtMS == nil {
			return fmt.Errorf("%w: missing HOLD expiry", ErrInvalid)
		}
		expiry, err := integer(*o.ExpiresAtMS, false)
		if err != nil || expiry <= created {
			return fmt.Errorf("%w: HOLD expiry", ErrInvalid)
		}
	case "STATIC", "RESERVATION", "EXCLUDED":
		if o.ExpiresAtMS != nil {
			return fmt.Errorf("%w: unexpected expiry", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: kind", ErrInvalid)
	}
	return nil
}

// EncodeChange builds a deterministic body and the existing eventlog change
// envelope. It cannot determine whether a proposed transition is legitimate.
func EncodeChange(kind string, o Occupancy) (eventlog.Change, error) {
	if kind != Set && kind != Cleared {
		return eventlog.Change{}, ErrInvalid
	}
	if err := o.Validate(); err != nil {
		return eventlog.Change{}, err
	}
	body, err := json.Marshal(o)
	if err != nil {
		return eventlog.Change{}, err
	}
	return eventlog.Change{Kind: kind, ObjectID: o.ID, Revision: o.Revision, Body: body}, nil
}

// DecodePayload validates an entire original eventlog payload. Only the two
// occupancy types are accepted here; mixed event batches require a dispatcher.
type Change struct {
	Kind      string
	Occupancy Occupancy
}

func DecodePayload(raw []byte) ([]Change, error) {
	if !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		return nil, ErrInvalid
	}
	changes, err := eventlog.DecodePayload(raw)
	if err != nil {
		return nil, err
	}
	out := make([]Change, 0, len(changes))
	for _, c := range changes {
		if c.Kind != Set && c.Kind != Cleared {
			return nil, fmt.Errorf("%w: unsupported kind", ErrInvalid)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(c.Body, &fields); err != nil || fields == nil || len(fields) != len(bodyFields) {
			return nil, fmt.Errorf("%w: body fields", ErrInvalid)
		}
		for k := range fields {
			if !bodyFields[k] {
				return nil, fmt.Errorf("%w: unexpected body field", ErrInvalid)
			}
		}
		// null is permitted only for expiresAtMs; json.Unmarshal would silently
		// turn null into the zero value of a non-pointer string or integer.
		for k, v := range fields {
			if k != "expiresAtMs" && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				return nil, fmt.Errorf("%w: null field", ErrInvalid)
			}
		}
		var o Occupancy
		if err := json.Unmarshal(c.Body, &o); err != nil {
			return nil, fmt.Errorf("%w: body types", ErrInvalid)
		}
		if err := o.Validate(); err != nil {
			return nil, err
		}
		if c.ObjectID != o.ID || c.Revision != o.Revision {
			return nil, fmt.Errorf("%w: inconsistent identity or revision", ErrInvalid)
		}
		out = append(out, Change{Kind: c.Kind, Occupancy: o})
	}
	return out, nil
}

// Hash validates this slice then hashes the unchanged original payload via the
// eventlog framing. A successful hash does not apply or authenticate an event.
func Hash(e eventlog.Event) ([32]byte, error) {
	if _, err := DecodePayload(e.Payload); err != nil {
		return [32]byte{}, err
	}
	return e.Hash()
}

// DecimalRevision converts a validated positive revision for callers that need
// arithmetic; callers must still enforce transition and overflow rules.
func DecimalRevision(s string) (int64, error) { return integer(s, true) }
