package dhcplease

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
)

// ExportLeaseRows reads the four frozen lease tables in strict SQLite BINARY
// primary-key order and emits the candidate rows in the same wire shape
// validateRow accepts (INTEGER decimal strings, BLOB canonical base64url).
// The export and the import are the same contract seen from two sides: rows
// this function emits must pass validateRow unchanged, and re-staging them
// into a fresh database must reproduce the same data.
//
// It reads only; the caller owns the transaction/connection.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// exportOrder is the frozen table order: a candidate stream must list rows
// in non-decreasing declared-table order (snapshotrows checks it), so the
// export cannot iterate the definitions map (random order).
var exportOrder = []string{"dhcp_lease", "dhcp_declined", "dhcp_outbox", "dhcp_receipt"}

func ExportLeaseRows(ctx context.Context, q Querier) ([]LeaseRow, error) {
	var out []LeaseRow
	for _, table := range exportOrder {
		cols := definitions[table]
		// ORDER BY the primary-key columns: SQLite compares BLOBs byte-wise
		// and TEXT with BINARY collation by default, which is exactly the
		// frozen row order. The strict-increase assertion below proves the
		// collation behaved.
		pk := pkColumns[table]
		query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s", joinComma(cols), table, joinComma(pk))
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("dhcplease: exporting %s: %w", table, err)
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		pkCols := pkColumns[table]
		var previousPK []string
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("dhcplease: scanning %s row: %w", table, err)
			}
			row := LeaseRow{Table: table, Values: map[string]any{}}
			key := make([]string, 0, len(pkCols))
			for i, c := range cols {
				row.Values[c] = encodeColumn(table, c, values[i])
				if containsPK(pkCols, c) {
					dec, derr := decodeField(table, c, row.Values[c])
					if derr != nil {
						rows.Close()
						return nil, fmt.Errorf("dhcplease: decoding %s pk column %s: %w", table, c, derr)
					}
					key = append(key, keyPart(c, dec))
				}
			}
			// Strictly increasing: equal or decreasing keys mean the ORDER BY
			// collation does not match the frozen BINARY order.
			if previousPK != nil && compareKeys(previousPK, key) >= 0 {
				rows.Close()
				return nil, fmt.Errorf("dhcplease: %s rows are not in strict BINARY primary-key order", table)
			}
			previousPK = key
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("dhcplease: iterating %s: %w", table, err)
		}
		rows.Close()
	}
	return out, nil
}

func joinComma(cols []string) string {
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ", "
		}
		out += c
	}
	return out
}

// encodeColumn converts a SQL value into the wire form (the inverse of
// decodeColumn).
func encodeColumn(table, column string, v any) any {
	if v == nil {
		return nil
	}
	if raw, ok := v.([]byte); ok && blobColumns[column] {
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	switch x := v.(type) {
	case []byte:
		return base64.RawURLEncoding.EncodeToString(x)
	case int64:
		return fmt.Sprintf("%d", x)
	}
	return v
}

// compareKeys compares two rendered primary keys component-wise. Components
// carry their type prefix ("b:" BLOB, "n:" numeric, "t:" text), so BLOB keys
// compare by raw bytes (the frozen BINARY order) and numeric keys numerically
// through their zero-padded rendering.
func compareKeys(a, b []string) int {
	for i := range a {
		if i >= len(b) {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(b) > len(a) {
		return -1
	}
	return 0
}
