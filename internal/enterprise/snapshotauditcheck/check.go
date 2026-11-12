// Package snapshotauditcheck verifies bounded audit and receipt row structure in
// an isolated SQLite staging view. It does not authenticate a source, establish
// business references, or authorize installation or a role.
package snapshotauditcheck

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
	"github.com/jasonwa/goddi/internal/enterprise/snapshotipam"
)

var ErrInvalid = errors.New("snapshotauditcheck: invalid staging rows or limits")

const MaxRows uint64 = 100000
const MaxBytes uint64 = 64 << 20
const maxTextBytes = 4096
const maxReceiptBytes = 8 + idempotency.MaxETagBytes + idempotency.MaxBodyBytes

type Counts struct{ Audit, Receipts uint64 }
type Limits struct {
	Rows, Bytes uint64
	Expected    Counts
}

// Evidence covers only structure, key order and counts, not provenance,
// historical completeness, cross-table relationships or authorization.
type Evidence struct {
	AuditRows, ReceiptRows bool
	Counts                 Counts
}
type field struct {
	name, kind string
	max        int
}
type table struct {
	name, order string
	fields      []field
}

var tables = []table{
	{"static_occupancy_audit", "tenant_id,space_id,resource_id,new_revision", []field{
		{"cluster_id", "blob", 16}, {"epoch", "integer", 0}, {"tenant_id", "text", maxTextBytes}, {"space_id", "text", maxTextBytes}, {"resource_id", "text", maxTextBytes}, {"principal", "text", maxTextBytes}, {"action", "text", maxTextBytes}, {"request_id", "text", maxTextBytes}, {"prior_revision", "integer", 0}, {"new_revision", "integer", 0}, {"result", "text", maxTextBytes}, {"occurred_at_ms", "integer", 0}, {"request_digest", "blob", 32},
	}},
	{"idempotency_receipt", "tenant_id,principal,method,path,idempotency_key", []field{
		{"tenant_id", "blob", 16}, {"principal", "text", maxTextBytes}, {"method", "text", maxTextBytes}, {"path", "text", maxTextBytes}, {"idempotency_key", "text", maxTextBytes}, {"request_digest", "blob", 32}, {"response", "blob", maxReceiptBytes}, {"created_at_unix_ms", "integer", 0}, {"expires_at_unix_ms", "integer", 0},
	}},
}

// Check requires a reviewed eventlog schema, an isolated database, and exclusive
// use of this connection with no concurrent writers or schema changes. Counts
// must come independently from the verified manifest. query_only is not an
// authorization boundary. All receipts are validated, including expired ones.
func Check(ctx context.Context, conn *sql.Conn, limits Limits) (out Evidence, err error) {
	if conn == nil || limits.Rows == 0 || limits.Rows > MaxRows || limits.Bytes == 0 || limits.Bytes > MaxBytes {
		return Evidence{}, ErrInvalid
	}
	for _, q := range []string{"PRAGMA query_only", "PRAGMA foreign_keys"} {
		var n int
		if e := conn.QueryRowContext(ctx, q).Scan(&n); e != nil {
			return Evidence{}, e
		}
		if n != 1 {
			return Evidence{}, ErrInvalid
		}
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Evidence{}, err
	}
	defer func() {
		if e := tx.Rollback(); e != nil {
			out = Evidence{}
			err = errors.Join(err, fmt.Errorf("snapshotauditcheck: rollback: %w", e))
		}
	}()
	var pinned, temp int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.sqlite_schema LIMIT 1)`).Scan(&pinned); err != nil {
		return Evidence{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_temp_schema LIMIT 1)`).Scan(&temp); err != nil {
		return Evidence{}, err
	}
	if temp != 0 {
		return Evidence{}, ErrInvalid
	}
	dbs, e := tx.QueryContext(ctx, `PRAGMA database_list`)
	if e != nil {
		return Evidence{}, e
	}
	mainSeen := false
	for dbs.Next() {
		var seq int
		var name, path string
		if e = dbs.Scan(&seq, &name, &path); e != nil {
			_ = dbs.Close()
			return Evidence{}, e
		}
		if seq == 0 && name == "main" && !mainSeen {
			mainSeen = true
			continue
		}
		if seq != 1 || name != "temp" {
			_ = dbs.Close()
			return Evidence{}, ErrInvalid
		}
	}
	e = dbs.Err()
	closeErr := dbs.Close()
	if e != nil {
		return Evidence{}, e
	}
	if closeErr != nil {
		return Evidence{}, closeErr
	}
	if !mainSeen {
		return Evidence{}, ErrInvalid
	}
	var encoding string
	if err = tx.QueryRowContext(ctx, `PRAGMA main.encoding`).Scan(&encoding); err != nil {
		return Evidence{}, err
	}
	if encoding != "UTF-8" {
		return Evidence{}, ErrInvalid
	}
	validator := snapshotipam.New()
	var counts Counts
	var rows, total uint64
	for index, table := range tables {
		projections := make([]string, 0, len(table.fields)*2)
		for _, f := range table.fields {
			cond := "typeof(" + f.name + ")='" + f.kind + "'"
			if f.kind == "text" {
				cond += " AND length(CAST(" + f.name + " AS BLOB))<=" + strconv.Itoa(f.max)
			}
			if f.kind == "blob" {
				if f.name == "response" {
					cond += " AND length(" + f.name + ")<=" + strconv.Itoa(f.max)
				} else {
					cond += " AND length(" + f.name + ")=" + strconv.Itoa(f.max)
				}
			}
			projections = append(projections, "typeof("+f.name+")", "CASE WHEN "+cond+" THEN "+f.name+" END")
		}
		query := "SELECT " + strings.Join(projections, ",") + " FROM main." + table.name + " ORDER BY " + table.order + " LIMIT ?"
		rs, e := tx.QueryContext(ctx, query, int64(limits.Rows)+1)
		if e != nil {
			return Evidence{}, e
		}
		for rs.Next() {
			rows++
			if rows > limits.Rows {
				_ = rs.Close()
				return Evidence{}, ErrInvalid
			}
			if index == 0 {
				counts.Audit++
			} else {
				counts.Receipts++
			}
			dest := make([]any, len(table.fields)*2)
			ptr := make([]any, len(dest))
			for i := range dest {
				ptr[i] = &dest[i]
			}
			if e = rs.Scan(ptr...); e != nil {
				_ = rs.Close()
				return Evidence{}, e
			}
			obj := make(map[string]any, len(table.fields))
			for i, f := range table.fields {
				typ, ok := dest[2*i].(string)
				value := dest[2*i+1]
				if !ok || typ != f.kind || value == nil {
					_ = rs.Close()
					return Evidence{}, ErrInvalid
				}
				var size uint64
				switch f.kind {
				case "integer":
					n, ok := value.(int64)
					if !ok {
						_ = rs.Close()
						return Evidence{}, ErrInvalid
					}
					size = 8
					obj[f.name] = strconv.FormatInt(n, 10)
				case "text":
					s, ok := value.(string)
					if !ok || !utf8.ValidString(s) {
						_ = rs.Close()
						return Evidence{}, ErrInvalid
					}
					size = uint64(len(s))
					obj[f.name] = s
				case "blob":
					b, ok := value.([]byte)
					if !ok {
						_ = rs.Close()
						return Evidence{}, ErrInvalid
					}
					size = uint64(len(b))
					obj[f.name] = base64.RawURLEncoding.EncodeToString(b)
				}
				if size > limits.Bytes-total {
					_ = rs.Close()
					return Evidence{}, ErrInvalid
				}
				total += size
			}
			raw, e := json.Marshal(obj)
			if e != nil {
				_ = rs.Close()
				return Evidence{}, e
			}
			if _, e = validator.Validate(table.name, raw); e != nil {
				_ = rs.Close()
				return Evidence{}, fmt.Errorf("snapshotauditcheck: %s: %w", table.name, e)
			}
		}
		e = rs.Err()
		closeErr = rs.Close()
		if e != nil {
			return Evidence{}, e
		}
		if closeErr != nil {
			return Evidence{}, closeErr
		}
	}
	if counts != limits.Expected {
		return Evidence{}, ErrInvalid
	}
	if err = ctx.Err(); err != nil {
		return Evidence{}, err
	}
	return Evidence{AuditRows: true, ReceiptRows: true, Counts: counts}, nil
}
