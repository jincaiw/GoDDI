package eventlog

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// Change is one logical business change inside a V1 payload. Revision is the
// object's own monotonic version, carried as a decimal string so a JSON number
// cannot round it (docs/04:28).
type Change struct {
	Kind     string          `json:"kind"`
	ObjectID string          `json:"objectId"`
	Revision string          `json:"revision"`
	Body     json.RawMessage `json:"body"`
}

// KnownChangeKindsContains limits the provisional envelope vocabulary. It does
// not define complete business bodies or make any event safe to apply.

// DecodePayload validates the provisional V1 envelope over the ORIGINAL bytes
// and preserves change order. It rejects a byte-order mark, a non-object root,
// trailing content, an empty change list, unknown fields, repeated object keys
// and kinds outside the provisional vocabulary. It does not validate business
// bodies or authorize a receiver to apply an event.
func DecodePayload(raw []byte) ([]Change, error) {
	if err := checkPayloadBytes(raw); err != nil {
		return nil, err
	}
	if hasDuplicateKeys(raw) {
		return nil, ErrPayloadDuplicate
	}
	if err := checkExactFields(raw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p wirePayload
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPayloadNotJSON, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing content", ErrPayloadNotJSON)
	}
	if len(p.Changes) == 0 {
		return nil, fmt.Errorf("%w: changes must be a non-empty array", ErrPayloadNotJSON)
	}
	for _, c := range p.Changes {
		if err := validateChangeShape(c); err != nil {
			return nil, err
		}
	}
	return p.Changes, nil
}

// Check envelope keys before struct decoding, which otherwise matches keys
// case-insensitively and permits aliases to overwrite each other.
func checkExactFields(raw []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || root == nil {
		return fmt.Errorf("%w: expected object", ErrPayloadNotJSON)
	}
	for k := range root {
		if k != "changes" {
			return fmt.Errorf("%w: unknown field %q", ErrPayloadNotJSON, k)
		}
	}
	var changes []json.RawMessage
	if err := json.Unmarshal(root["changes"], &changes); err != nil {
		return fmt.Errorf("%w: invalid changes", ErrPayloadNotJSON)
	}
	for _, rawChange := range changes {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawChange, &fields); err != nil || fields == nil {
			return fmt.Errorf("%w: change must be an object", ErrPayloadNotJSON)
		}
		for k := range fields {
			switch k {
			case "kind", "objectId", "revision", "body":
			default:
				return fmt.Errorf("%w: unknown field %q", ErrPayloadNotJSON, k)
			}
		}
	}
	return nil
}

func checkPayloadBytes(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxPayloadBytes {
		return fmt.Errorf("%w: %d bytes", ErrPayloadSize, len(raw))
	}
	if !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		return ErrPayloadEncoding
	}
	if !json.Valid(raw) {
		return ErrPayloadNotJSON
	}
	return nil
}

// MarshalChanges renders the original payload bytes for one or more changes.
// Callers must hash exactly these bytes; decoding and re-encoding a payload for
// the digest would silently break the chain.
func MarshalChanges(changes ...Change) ([]byte, error) {
	if len(changes) == 0 {
		return nil, fmt.Errorf("%w: no changes", ErrPayloadNotJSON)
	}
	for _, c := range changes {
		if err := validateChangeShape(c); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(wirePayload{Changes: changes})
	if err != nil {
		return nil, err
	}
	if _, err := DecodePayload(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func validateChangeShape(c Change) error {
	if !kindRE.MatchString(c.Kind) {
		return fmt.Errorf("%w: %q", ErrUnknownKind, c.Kind)
	}
	if !KnownChangeKindsContains(c.Kind) {
		return fmt.Errorf("%w: %q", ErrUnknownKind, c.Kind)
	}
	if !uuidRE.MatchString(c.ObjectID) {
		return fmt.Errorf("%w: objectId %q is not a canonical UUID", ErrPayloadNotJSON, c.ObjectID)
	}
	if !int64RE.MatchString(c.Revision) {
		return fmt.Errorf("%w: revision %q must be an unsigned decimal string", ErrPayloadNotJSON, c.Revision)
	}
	if _, err := strconv.ParseInt(c.Revision, 10, 64); err != nil {
		return fmt.Errorf("%w: revision exceeds int64", ErrPayloadNotJSON)
	}
	if !utf8.Valid(c.Body) {
		return ErrPayloadEncoding
	}
	if len(c.Body) == 0 {
		return fmt.Errorf("%w: body is required", ErrPayloadNotJSON)
	}
	if !json.Valid(c.Body) {
		return fmt.Errorf("%w: body of %q is not JSON", ErrPayloadNotJSON, c.Kind)
	}
	return nil
}

// KnownChangeKindsContains reports whether a kind belongs to the frozen
// schema_version 1 registry.
func KnownChangeKindsContains(kind string) bool {
	switch kind {
	case "ipam.occupancy.set", "ipam.occupancy.audit", "idempotency.receipt.set", "ipam.occupancy.cleared", "ipam.prefix.created", "ipam.prefix.removed", "ipam.pool.set", "ipam.pool.removed", "dhcp.lease.committed", "dhcp.lease.renewed", "dhcp.lease.released", "dhcp.lease.declined", "dhcp.offers.expired", "config.release.committed":
		return true
	default:
		return false
	}
}

// ParseHash accepts the 32-byte or lowercase-hex form of a chain digest.
func ParseHash(raw []byte) ([HashBytes]byte, error) {
	var h [HashBytes]byte
	if len(raw) == HashBytes {
		copy(h[:], raw)
		return h, nil
	}
	b, err := hex.DecodeString(string(raw))
	if err != nil || len(b) != HashBytes {
		return h, ErrHashLength
	}
	copy(h[:], b)
	return h, nil
}
