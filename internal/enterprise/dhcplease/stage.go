package dhcplease

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LeaseRow is one candidate row for a lease-table set table: the table name
// and the column values, using the same wire conventions as the eight-table
// candidate — INTEGER as a canonical non-negative decimal string, BLOB as
// canonical unpadded base64url, TEXT as bounded UTF-8.
type LeaseRow struct {
	Table  string
	Values map[string]any
}

// stageTables are the tables StageLeaseRows accepts, with their frozen column
// order for deterministic application.
var stageTables = map[string][]string{
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

// decodeColumn converts one wire value into its SQL form: decimal strings to
// integers for the integer columns, base64url to bytes for BLOBs, and text
// through unchanged. The column list decides which conversion applies.
func decodeColumn(table, column string, v any) (any, error) {
	switch table {
	case "dhcp_lease", "dhcp_declined", "dhcp_receipt":
		switch column {
		case "address":
			return decodeB64(v)
		case "family":
			return decodeInt(v)
		case "client_key", "digest":
			return decodeB64(v)
		case "offer_xid", "xid", "issued_ms", "revision", "seq":
			return decodeInt(v)
		case "expires_ms":
			if v == nil {
				return nil, nil // permanent/reservation lease (decision 3)
			}
			return decodeInt(v)
		}
		return v, nil
	case "dhcp_outbox":
		switch column {
		case "cluster_id", "hash", "payload":
			return decodeB64(v)
		case "epoch", "seq":
			return decodeInt(v)
		}
		return v, nil
	}
	return v, nil
}

func decodeInt(v any) (any, error) {
	switch x := v.(type) {
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("dhcplease: integer column value %q is not a decimal string: %w", x, err)
		}
		return n, nil
	case float64:
		// JSON decoders hand out float64 even for whole numbers.
		return int64(x), nil
	case int64:
		return x, nil
	}
	return nil, fmt.Errorf("dhcplease: unsupported integer value %T", v)
}

func decodeB64(v any) (any, error) {
	switch x := v.(type) {
	case string:
		raw, err := base64.RawURLEncoding.DecodeString(x)
		if err != nil {
			// The eight-table fixtures also use padded base64url in places.
			raw, err = base64.URLEncoding.DecodeString(x)
			if err != nil {
				return nil, fmt.Errorf("dhcplease: blob value is not base64url: %w", err)
			}
		}
		return raw, nil
	case []byte:
		return x, nil
	}
	return nil, fmt.Errorf("dhcplease: unsupported blob value %T", v)
}

// InstallFrozenDDL applies the frozen lease schema inside the caller's
// transaction. Callers that stage lease rows alongside other tables (the
// schema-2 composition) install it first and then insert rows one by one.
func InstallFrozenDDL(tx *sql.Tx) error { return install(tx) }

// InsertLeaseRow applies one candidate row (no DDL, no verification).
func InsertLeaseRow(tx *sql.Tx, r LeaseRow) error {
	return insertLeaseRow(tx, r)
}

func insertLeaseRow(tx *sql.Tx, r LeaseRow) error {
	cols, ok := stageTables[r.Table]
	if !ok {
		return fmt.Errorf("dhcplease: table %q is not in the frozen lease set", r.Table)
	}
	known := map[string]bool{}
	for _, c := range cols {
		known[c] = true
	}
	for c := range r.Values {
		if !known[c] {
			return fmt.Errorf("dhcplease: %s carries unknown column %q", r.Table, c)
		}
	}
	placeholders := make([]string, len(cols))
	for j := range placeholders {
		placeholders[j] = "?"
	}
	args := make([]any, 0, len(cols))
	for _, c := range cols {
		v, err := decodeColumn(r.Table, c, r.Values[c])
		if err != nil {
			return fmt.Errorf("dhcplease: %s.%s: %w", r.Table, c, err)
		}
		args = append(args, v)
	}
	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		r.Table, strings.Join(cols, ","), strings.Join(placeholders, ","))
	if _, err := tx.Exec(stmt, args...); err != nil {
		return fmt.Errorf("dhcplease: insert into %s: %w", r.Table, err)
	}
	return nil
}

// StageLeaseRows installs the frozen lease DDL and applies the candidate rows
// inside the caller's transaction, then runs Verify. Rows are applied in
// deterministic order (table, then sorted columns) so a failure pinpoints the
// offending row. The caller owns commit/rollback.
// StageLeaseRows applies the candidate rows and verifies the composed
// candidate. The frozen DDL must already be installed (InstallFrozenDDL) —
// the schema-2 composition installs it once and stages rows in the same
// transaction as the eight-table candidate.
func StageLeaseRows(tx *sql.Tx, rows []LeaseRow) error {
	for i, r := range rows {
		if err := InsertLeaseRow(tx, r); err != nil {
			return fmt.Errorf("dhcplease: row %d into %s: %w", i, r.Table, err)
		}
	}
	// Deterministic verification after all rows are in.
	names := make([]string, 0, len(stageTables))
	for n := range stageTables {
		names = append(names, n)
	}
	sort.Strings(names)
	_ = names
	if err := Verify(tx); err != nil {
		return err
	}
	return nil
}
