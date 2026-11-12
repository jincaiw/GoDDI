package snapshotipam

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ExportRows reads the eight frozen candidate tables in strict SQLite BINARY
// primary-key order and emits candidate rows in the same wire shape Validate
// accepts — the inverse of the import side. The export and the feed are the
// same contract seen from two sides: rows this function emits must pass the
// validator unchanged, and the PK order must match the frozen BINARY order
// (an assertion inside the export proves the collation behaved).
//
// Column types come from the staged schema itself (PRAGMA table_info): BLOB
// columns are canonical base64url, INTEGER columns canonical decimal strings,
// TEXT columns verbatim, NULL preserved. The primary-key columns come from
// the schema's own PK index, so the export cannot drift from the staged DDL.
//
// It reads only; the caller owns the transaction/connection.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func ExportRows(ctx context.Context, q Querier) ([]Row, error) {
	var out []Row
	for _, table := range order {
		cols := definitions[table]
		pk, err := primaryKeyColumns(ctx, q, table)
		if err != nil {
			return nil, fmt.Errorf("snapshotipam: primary key for %s: %w", table, err)
		}
		query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s", joinComma(cols), table, joinComma(pk))
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("snapshotipam: exporting %s: %w", table, err)
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		var previousPK []string
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("snapshotipam: scanning %s row: %w", table, err)
			}
			key := make([]string, 0, len(pk))
			for i, c := range cols {
				if containsPK(pk, c) {
					key = append(key, keyPart(c, values[i]))
				}
			}
			// Strictly increasing: equal or decreasing keys mean the ORDER BY
			// collation does not match the frozen BINARY order.
			if previousPK != nil && compareKeys(previousPK, key) >= 0 {
				rows.Close()
				return nil, fmt.Errorf("snapshotipam: %s rows are not in strict BINARY primary-key order", table)
			}
			previousPK = key
			// Encode the wire form.
			obj := make(map[string]any, len(cols))
			for i, c := range cols {
				obj[c] = encodeColumn(values[i])
			}
			raw, err := json.Marshal(obj)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("snapshotipam: encoding %s row: %w", table, err)
			}
			out = append(out, Row{Table: table, Payload: raw})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("snapshotipam: iterating %s: %w", table, err)
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

func containsPK(pk []string, c string) bool {
	for _, k := range pk {
		if k == c {
			return true
		}
	}
	return false
}

// primaryKeyColumns reads the schema's own PK column order for a table.
func primaryKeyColumns(ctx context.Context, q Querier, table string) ([]string, error) {
	// The PK index: origin='pk' in index_list. Rowid tables with an inline
	// composite PRIMARY KEY get an implicit sqlite_autoindex_* entry.
	rows, err := q.QueryContext(ctx, `SELECT name FROM pragma_index_list(`+quote(table)+`) WHERE origin='pk'`)
	if err != nil {
		return nil, err
	}
	var pkIndex string
	for rows.Next() {
		if err := rows.Scan(&pkIndex); err != nil {
			rows.Close()
			return nil, err
		}
	}
	rows.Close()
	if pkIndex == "" {
		// A rowid table whose PK is the rowid alias (single INTEGER PK) has no
		// pk-origin index; fall back to the declared "rowid" column.
		return []string{"rowid"}, nil
	}
	info, err := q.QueryContext(ctx, `SELECT name FROM pragma_index_info(`+quote(pkIndex)+`) ORDER BY seqno`)
	if err != nil {
		return nil, err
	}
	defer info.Close()
	var cols []string
	for info.Next() {
		var c string
		if err := info.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, info.Err()
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// encodeColumn converts a SQL value into the wire form. BLOB columns are
// 16-byte by the frozen schema and encode as canonical base64url; INTEGER
// columns as canonical decimal strings; TEXT verbatim; NULL preserved.
func encodeColumn(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return base64.RawURLEncoding.EncodeToString(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return v
}

// keyPart renders one PK component for the strict-increase comparison: BLOBs
// by raw bytes, integers numerically through zero padding, TEXT byte-wise.
func keyPart(column string, v any) string {
	switch x := v.(type) {
	case []byte:
		return "b:" + string(x)
	case int64:
		return "n:" + fmt.Sprintf("%020d", x)
	default:
		return "t:" + fmt.Sprint(x)
	}
}

// compareKeys compares two rendered primary keys component-wise.
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
