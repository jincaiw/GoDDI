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

// queryer is the subset of *sql.DB / *sql.Tx used to read one table. A single
// *sql.Tx satisfies it for every table in a snapshot, giving a point-in-time
// view under WAL.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// readSnapshotAfterTableForTest lets a test mutate the source between two
// table reads. It is a no-op in production; see snapshot_read_tx_test.go.
var readSnapshotAfterTableForTest func(table string) = func(string) {}

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
	// One read-only transaction for every table: without it, each QueryContext
	// may use a different connection and observe a different commit, producing
	// a torn snapshot (e.g. a child without its parent).
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return snap, fmt.Errorf("configrepl: begin read snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, sp := range specs {
		rows, err := readTable(ctx, tx, sp)
		if err != nil {
			return snap, fmt.Errorf("configrepl: reading table for snapshot %s: %w", sp.Name, err)
		}
		if rows == nil {
			continue // table absent in this deployment
		}
		snap.Tables = append(snap.Tables, TableData{Name: sp.Name, Rows: rows})
		readSnapshotAfterTableForTest(sp.Name)
	}
	if err := tx.Commit(); err != nil {
		return snap, fmt.Errorf("configrepl: commit read snapshot: %w", err)
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
	// Keep the operator-visible version non-negative. The high bit carries no
	// equality information; masking it preserves collision resistance (63 bits)
	// while avoiding a negative "version" in the console. Zero is reserved for
	// "unknown" throughout the status contract, so map the vanishingly rare
	// all-zero low bits to 1.
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

// readTable returns nil (not an empty slice) when the table does not exist, so
// the caller can distinguish "absent" from "empty".
func readTable(ctx context.Context, q queryer, sp dataplane.ConfigTableSpec) ([][]any, error) {
	if len(sp.Columns) == 0 {
		return nil, fmt.Errorf("configrepl: table %s has no columns", sp.Name)
	}
	// Verify schema before SELECT: SQLite treats a missing double-quoted
	// identifier as a string literal, so a missing column would otherwise be
	// copied into every row as that literal instead of failing closed.
	metaRows, err := q.QueryContext(ctx, "PRAGMA table_info("+quoteIdent(sp.Name)+")")
	if err != nil {
		return nil, fmt.Errorf("configrepl: checking schema of %s: %w", sp.Name, err)
	}
	available := make(map[string]bool)
	for metaRows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := metaRows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			metaRows.Close()
			return nil, fmt.Errorf("configrepl: scanning schema of %s: %w", sp.Name, err)
		}
		available[name] = true
	}
	metaErr := metaRows.Err()
	metaRows.Close()
	if metaErr != nil {
		return nil, fmt.Errorf("configrepl: reading schema of %s: %w", sp.Name, metaErr)
	}
	if len(available) == 0 {
		return nil, nil // table absent in this deployment
	}
	for _, col := range sp.Columns {
		if !available[col] {
			return nil, fmt.Errorf("configrepl: table %s is missing column %s", sp.Name, col)
		}
	}
	cols := quoteCols(sp.Columns)
	querySQL := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), quoteIdent(sp.Name))
	rows, err := q.QueryContext(ctx, querySQL)
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
// ApplySnapshot replaces every replicated table in db with the snapshot rows.
// The apply and the follower watermark can be committed together by callers
// that need atomicity; see ApplySnapshotTx.
func ApplySnapshot(ctx context.Context, db *sql.DB, snap Snapshot, specs []dataplane.ConfigTableSpec) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("configrepl: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	n, err := ApplySnapshotTx(ctx, tx, snap, specs)
	if err != nil {
		return n, err
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("configrepl: commit: %w", err)
	}
	return n, nil
}

// ApplySnapshotTx applies a snapshot inside an existing transaction. It lets
// the caller put the applied-version watermark in the same commit: a crash
// can then never leave configuration applied but marked as old (which would
// either replay work or, worse, let a stale in-memory watermark skip it).
func ApplySnapshotTx(ctx context.Context, tx *sql.Tx, snap Snapshot, specs []dataplane.ConfigTableSpec) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	byName := make(map[string]TableData, len(snap.Tables))
	for _, t := range snap.Tables {
		byName[t.Name] = t
	}
	total := 0
	// _configrepl_keys holds the keys present in the snapshot for the table
	// currently being applied; a temp table keeps the DELETE bounded.
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE _configrepl_keys (k TEXT PRIMARY KEY)"); err != nil {
		return total, fmt.Errorf("configrepl: create key temp table: %w", err)
	}
	for _, sp := range specs {
		td, ok := byName[sp.Name]
		if !ok {
			continue
		}
		switch sp.Mode {
		case dataplane.ConfigReplaceWhole, dataplane.ConfigReplaceControlAuthored:
		default:
			return total, fmt.Errorf("configrepl: unknown replace mode %q for %s", sp.Mode, sp.Name)
		}
		// Delete ONLY rows the author no longer has. A wholesale DELETE or
		// INSERT OR REPLACE would delete parent rows and fire ON DELETE CASCADE
		// into local-only children (ipam_addresses/ipam_history).
		if _, err := tx.ExecContext(ctx, "DELETE FROM _configrepl_keys"); err != nil {
			return total, fmt.Errorf("configrepl: clear key temp table: %w", err)
		}
		keyIdx := -1
		for i, col := range sp.Columns {
			if col == sp.Key {
				keyIdx = i
				break
			}
		}
		if keyIdx < 0 {
			return total, fmt.Errorf("configrepl: table %s: key %q not in columns", sp.Name, sp.Key)
		}
		if len(td.Rows) > 0 {
			kstmt, err := tx.PrepareContext(ctx, "INSERT OR IGNORE INTO _configrepl_keys(k) VALUES (?)")
			if err != nil {
				return total, fmt.Errorf("configrepl: prepare keys for %s: %w", sp.Name, err)
			}
			for _, row := range td.Rows {
				if len(row) != len(sp.Columns) {
					kstmt.Close()
					return total, fmt.Errorf("configrepl: %s row has %d values, want %d", sp.Name, len(row), len(sp.Columns))
				}
				if _, err := kstmt.ExecContext(ctx, row[keyIdx]); err != nil {
					kstmt.Close()
					return total, fmt.Errorf("configrepl: key for %s: %w", sp.Name, err)
				}
			}
			if err := kstmt.Close(); err != nil {
				return total, fmt.Errorf("configrepl: close keys for %s: %w", sp.Name, err)
			}
		}
		del := fmt.Sprintf("DELETE FROM %s WHERE %s NOT IN (SELECT k FROM _configrepl_keys)",
			quoteIdent(sp.Name), quoteIdent(sp.Key))
		if sp.Mode == dataplane.ConfigReplaceControlAuthored {
			del += " AND COALESCE(authored_locally,0) = 0"
		}
		if _, err := tx.ExecContext(ctx, del); err != nil {
			if isNoSuchTable(err) {
				continue
			}
			return total, fmt.Errorf("configrepl: pruning %s: %w", sp.Name, err)
		}
		if len(td.Rows) == 0 {
			continue
		}
		// Upsert in place: ON CONFLICT DO UPDATE never deletes the old row.
		cols := quoteCols(sp.Columns)
		collist := strings.Join(cols, ", ")
		placeholders := strings.TrimRight(strings.Repeat("?,", len(sp.Columns)), ",")
		updates := make([]string, 0, len(cols)-1)
		for _, col := range cols {
			if strings.ReplaceAll(col, "\"", "") == sp.Key {
				continue
			}
			updates = append(updates, fmt.Sprintf("%s=excluded.%s", col, col))
		}
		ins := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(%s) DO UPDATE SET %s",
			quoteIdent(sp.Name), collist, placeholders, quoteIdent(sp.Key), strings.Join(updates, ", "))
		stmt, err := tx.PrepareContext(ctx, ins)
		if err != nil {
			return total, fmt.Errorf("configrepl: preparing upsert into %s: %w", sp.Name, err)
		}
		for _, row := range td.Rows {
			if _, err := stmt.ExecContext(ctx, row...); err != nil {
				stmt.Close()
				return total, fmt.Errorf("configrepl: upserting into %s: %w", sp.Name, err)
			}
			total++
		}
		if err := stmt.Close(); err != nil {
			return total, fmt.Errorf("configrepl: closing upsert for %s: %w", sp.Name, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE _configrepl_keys"); err != nil {
		return total, fmt.Errorf("configrepl: drop key temp table: %w", err)
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
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Only "no such table: <name>" means the deployment does not carry the
	// table. A missing column is a schema mismatch and must fail closed: it
	// is reported as "no such column: <name>" and would otherwise be treated
	// as an absent table, publishing a silently incomplete snapshot.
	return strings.Contains(msg, "no such table:")
}
