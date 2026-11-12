package dhcplease

// NDJSON row validation for the lease-table set, following the conventions
// the eight-table candidate uses (snapshotipam): frozen column order per
// table, INTEGER as a canonical non-negative decimal string, BLOB as
// canonical unpadded base64url, TEXT as bounded UTF-8 without NUL, and strict
// SQLite BINARY primary-key order within each table.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// definitions maps each frozen lease table to its frozen column order.
var definitions = map[string][]string{
	"dhcp_lease": {
		"tenant", "space", "address", "family", "lease_id", "client_key",
		"kind", "offer_xid", "issued_ms", "expires_ms", "revision",
	},
	"dhcp_declined": {
		"tenant", "space", "address", "family", "reason", "declined_ms", "revision",
	},
	"dhcp_outbox": {
		"cluster_id", "epoch", "seq", "hash", "payload",
	},
	"dhcp_receipt": {
		"tenant", "space", "client_key", "xid", "kind", "digest",
		"reply_kind", "address", "expires_ms", "revision", "seq",
	},
}

// pkColumns lists the primary-key columns per table in key order; the sort
// order across rows is strict SQLite BINARY on these.
var pkColumns = map[string][]string{
	"dhcp_lease":    {"tenant", "space", "address"},
	"dhcp_declined": {"tenant", "space", "address"},
	"dhcp_outbox":   {"cluster_id", "epoch", "seq"},
	"dhcp_receipt":  {"tenant", "space", "client_key", "xid", "kind"},
}

// blobColumns are base64url-encoded in the candidate rows.
var blobColumns = map[string]bool{
	"address": true, "client_key": true, "digest": true,
	"cluster_id": true, "hash": true, "payload": true,
}

// intColumns are canonical non-negative decimal strings in the rows
// (expires_ms is the one integer that may be null — a permanent lease).
var intColumns = map[string]bool{
	"family": true, "offer_xid": true, "issued_ms": true, "expires_ms": true,
	"revision": true, "epoch": true, "seq": true, "xid": true,
	"declined_ms": true,
}

// enumColumns carry frozen value sets.
var enumColumns = map[string][]string{
	"kind": {"OFFER", "BOUND", "RELEASED", "EXPIRED", "DECLINED"},
	// dhcp_receipt.kind and dhcp_lease.kind share the same frozen enum; the
	// reply_kind is a reply code, not the lease enum, and is only checked for
	// bounded text.
}

// maxText bounds a TEXT column. The proposal sets no number; 4 KiB covers a
// hostname/reason with generous headroom while keeping a row far under the
// 2 MiB line bound.
const maxText = 4096

// validateRow validates one candidate row for a frozen lease table: exact
// column set, field shapes, and the frozen enum. It returns the decoded
// primary key for order checking.
func validateRow(table string, raw []byte) (pk []string, err error) {
	cols, ok := definitions[table]
	if !ok {
		return nil, fmt.Errorf("dhcplease: unknown lease table %q", table)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("dhcplease: %s row is not a JSON object: %w", table, err)
	}
	if len(obj) != len(cols) {
		return nil, fmt.Errorf("dhcplease: %s row has %d fields, want exactly %d (frozen column set)", table, len(obj), len(cols))
	}
	for _, c := range cols {
		v, ok := obj[c]
		if !ok {
			return nil, fmt.Errorf("dhcplease: %s row is missing column %q", table, c)
		}
		dec, err := decodeField(table, c, v)
		if err != nil {
			return nil, err
		}
		if pkColumns[table] != nil && containsPK(pkColumns[table], c) {
			pk = append(pk, keyPart(c, dec))
		}
	}
	return pk, nil
}

func containsPK(pk []string, c string) bool {
	for _, k := range pk {
		if k == c {
			return true
		}
	}
	return false
}

func decodeField(table, column string, v any) (any, error) {
	switch {
	case column == "expires_ms" && v == nil:
		return nil, nil // permanent/reservation lease (decision 3)
	case blobColumns[column]:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("dhcplease: %s.%s must be a base64url string", table, column)
		}
		raw, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("dhcplease: %s.%s is not base64url: %w", table, column, err)
		}
		if column == "address" && len(raw) != 16 {
			return nil, fmt.Errorf("dhcplease: %s.address must be 16 bytes, got %d", table, len(raw))
		}
		return raw, nil
	case intColumns[column]:
		if v == nil {
			return nil, fmt.Errorf("dhcplease: %s.%s must not be null", table, column)
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("dhcplease: %s.%s must be a decimal string", table, column)
		}
		if strings.HasPrefix(s, "-") {
			return nil, fmt.Errorf("dhcplease: %s.%s must be non-negative, got %q", table, column, s)
		}
		var n int64
		if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
			return nil, fmt.Errorf("dhcplease: %s.%s is not a decimal integer: %w", table, column, err)
		}
		return n, nil
	default:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("dhcplease: %s.%s must be a string", table, column)
		}
		if len(s) > maxText {
			return nil, fmt.Errorf("dhcplease: %s.%s exceeds %d bytes", table, column, maxText)
		}
		if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return nil, fmt.Errorf("dhcplease: %s.%s must be valid UTF-8 without NUL", table, column)
		}
		if enums, ok := enumColumns[column]; ok {
			found := false
			for _, e := range enums {
				if s == e {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("dhcplease: %s.%s = %q is outside the frozen enum %v", table, column, s, enums)
			}
		}
		return s, nil
	}
}

// keyPart renders one primary-key component for BINARY order comparison.
func keyPart(column string, decoded any) string {
	switch x := decoded.(type) {
	case []byte:
		return "b:" + string(x) // BLOB: raw byte order
	case int64:
		// Integers in the PK (epoch, seq, xid) compare numerically; render
		// zero-padded so lexicographic order matches numeric order for the
		// magnitudes the snapshot carries.
		return fmt.Sprintf("n:%020d", x)
	default:
		return "t:" + fmt.Sprint(x) // TEXT: UTF-8 byte order
	}
}
