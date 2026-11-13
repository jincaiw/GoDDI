package configrepl

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/dataplane"
)

// TableData is one configuration table carried in a snapshot: rows in column
// order.
type TableData struct {
	Name string  `json:"name"`
	Rows [][]any `json:"rows"`
}

// Snapshot is a full point-in-time copy of a node's configuration tables plus a
// monotonic Version that lets a follower skip work when nothing changed.
//
// A snapshot is idempotent to apply: replacing every replicated table with the
// author's current rows converges to the same state regardless of how many times
// it is applied. That is why a follower needs no gap arithmetic to use one -- a
// gap is resolved by taking a fresh snapshot, not by replaying missed entries.
type Snapshot struct {
	Version int64       `json:"version"`
	NodeID  string      `json:"node_id"`
	Tables  []TableData `json:"tables"`
}

// ReadSnapshot reads every configuration table in specs from db.
//
// A table that does not exist in this deployment (for example the DNS tables
// when DNS is disabled) is simply absent from the snapshot; the follower keeps
// whatever it had for that table, which is empty in that case too.
func ReadSnapshot(ctx context.Context, db *sql.DB, nodeID string, specs []dataplane.ConfigTableSpec) (Snapshot, error) {
	snap := Snapshot{NodeID: nodeID}
	if err := ctx.Err(); err != nil {
		return snap, err
	}
	for _, sp := range specs {
		rows, err := readTable(ctx, db, sp)
		if err != nil {
			return snap, err
		}
		if rows == nil {
			continue // table absent in this deployment
		}
		snap.Tables = append(snap.Tables, TableData{Name: sp.Name, Rows: rows})
	}
	snap.Version = fingerprint(snap.Tables)
	return snap, nil
}

// fingerprint is a stable 64-bit hash of the snapshot contents.
//
// The version must change whenever the configuration changes, regardless of
// which write path produced it. A DHCP scope created through the console does
// not necessarily append to config_release_outbox, so an outbox-id version would
// stay 0 and the follower would never pull -- the exact bug this replaces. A
// content hash is independent of the write path and of clock granularity.
func fingerprint(tables []TableData) int64 {
	h := fnv.New64a()
	for _, t := range tables {
		_, _ = h.Write([]byte(t.Name))
		_, _ = h.Write([]byte{0})
		rows := make([]string, 0, len(t.Rows))
		for _, r := range t.Rows {
			b, _ := json.Marshal(r)
			rows = append(rows, string(b))
		}
		sort.Strings(rows)
		for _, r := range rows {
			_, _ = h.Write([]byte(r))
			_, _ = h.Write([]byte{0})
		}
	}
	return int64(h.Sum64())
}

// readTable returns nil (not an empty slice) when the table does not exist, so
// the caller can distinguish "absent" from "empty".
func readTable(ctx context.Context, db *sql.DB, sp dataplane.ConfigTableSpec) ([][]any, error) {
	if len(sp.Columns) == 0 {
		return nil, fmt.Errorf("configrepl: table %s has no columns", sp.Name)
	}
	cols := quoteCols(sp.Columns)
	q := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), quoteIdent(sp.Name))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		if isNoSuchTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("configrepl: reading %s: %w", sp.Name, err)
	}
	defer rows.Close()

	out := make([][]any, 0)
	for rows.Next() {
		vals := make([]any, len(sp.Columns))
		ptrs := make([]any, len(sp.Columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("configrepl: scanning %s: %w", sp.Name, err)
		}
		for i, v := range vals {
			vals[i] = normalizeReadValue(v)
		}
		out = append(out, vals)
	}
	return out, rows.Err()
}

// ApplySnapshot replaces every replicated table in db with the snapshot's rows.
//
// Tables are applied in spec order (parent first) inside one transaction so a
// child row never references a parent that has not landed yet, and a partially
// applied snapshot is never visible.
func ApplySnapshot(ctx context.Context, db *sql.DB, snap Snapshot, specs []dataplane.ConfigTableSpec) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	byName := make(map[string]TableData, len(snap.Tables))
	for _, t := range snap.Tables {
		byName[t.Name] = t
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("configrepl: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	total := 0
	for _, sp := range specs {
		td, ok := byName[sp.Name]
		if !ok {
			continue // author does not carry this table; leave local copy as-is
		}
		// Clear according to the replace mode, then insert the author's rows.
		switch sp.Mode {
		case dataplane.ConfigReplaceWhole:
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+quoteIdent(sp.Name)); err != nil {
				if isNoSuchTable(err) {
					continue
				}
				return total, fmt.Errorf("configrepl: clearing %s: %w", sp.Name, err)
			}
		case dataplane.ConfigReplaceControlAuthored:
			// Keep rows this follower authored locally (e.g. records written by
			// a DNS dynamic update); replace only the control-authored ones.
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM "+quoteIdent(sp.Name)+" WHERE COALESCE(authored_locally,0) = 0"); err != nil {
				if isNoSuchTable(err) {
					continue
				}
				return total, fmt.Errorf("configrepl: clearing control-authored %s: %w", sp.Name, err)
			}
		default:
			return total, fmt.Errorf("configrepl: unknown replace mode %q for %s", sp.Mode, sp.Name)
		}

		if len(td.Rows) == 0 {
			continue
		}
		collist := strings.Join(quoteCols(sp.Columns), ", ")
		placeholders := strings.TrimRight(strings.Repeat("?,", len(sp.Columns)), ",")
		ins := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)", quoteIdent(sp.Name), collist, placeholders)
		stmt, err := tx.PrepareContext(ctx, ins)
		if err != nil {
			return total, fmt.Errorf("configrepl: preparing insert into %s: %w", sp.Name, err)
		}
		for _, row := range td.Rows {
			if len(row) != len(sp.Columns) {
				stmt.Close()
				return total, fmt.Errorf("configrepl: %s row has %d values, want %d", sp.Name, len(row), len(sp.Columns))
			}
			if _, err := stmt.ExecContext(ctx, row...); err != nil {
				stmt.Close()
				return total, fmt.Errorf("configrepl: inserting into %s: %w", sp.Name, err)
			}
			total++
		}
		if err := stmt.Close(); err != nil {
			return total, fmt.Errorf("configrepl: closing insert for %s: %w", sp.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return total, fmt.Errorf("configrepl: commit: %w", err)
	}
	return total, nil
}

// normalizeReadValue turns driver values into something json.Marshal encodes
// faithfully and json.Unmarshal+UseNumber can round-trip: []byte -> string,
// time.Time -> RFC3339 string. Numbers are left as the driver returned them
// (modernc/sqlite yields int64 for integer-affinity columns).
func normalizeReadValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}

// DecodeSnapshotJSON parses a snapshot from JSON preserving integer fidelity:
// numbers arrive as json.Number and are converted to int64 when integral, so an
// INTEGER column is not silently stored as REAL.
func DecodeSnapshotJSON(b []byte) (Snapshot, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var raw Snapshot
	if err := dec.Decode(&raw); err != nil {
		return Snapshot{}, fmt.Errorf("configrepl: decoding snapshot: %w", err)
	}
	for ti := range raw.Tables {
		for ri := range raw.Tables[ti].Rows {
			for ci, v := range raw.Tables[ti].Rows[ri] {
				raw.Tables[ti].Rows[ri][ci] = coerceJSONNumber(v)
			}
		}
	}
	return raw, nil
}

func coerceJSONNumber(v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	if iv, err := n.Int64(); err == nil {
		return iv
	}
	if fv, err := n.Float64(); err == nil {
		return fv
	}
	return n.String()
}

// EncodeSnapshotJSON is the author-side encoder. Rows carry int64/string/nil so
// encoding is direct; it exists so both sides share one entry point.
func EncodeSnapshotJSON(s Snapshot) ([]byte, error) {
	return json.Marshal(s)
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func quoteCols(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = quoteIdent(c)
	}
	return out
}

func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")
}
