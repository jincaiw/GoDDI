package eventlogguard

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	_ "modernc.org/sqlite"
)

// ErrInvalid means that the candidate could not be verified against an accepted reference.
var ErrInvalid = errors.New("eventlogguard: invalid reference")

const maxObjects = 512
const maxSchemaFieldBytes = 128 << 10
const maxSchemaBytes = 4 << 20

func invalid(where string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: %s", ErrInvalid, where)
	}
	return fmt.Errorf("%w: %s: %w", ErrInvalid, where, cause)
}

type reader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type object struct {
	kind, name, table string
	definition        sql.NullString
}

// Version describes only the inspected local candidate database structure. It
// is unrelated to event schema, snapshot or RPC protocol versions and must not
// be used for production feature negotiation or authority.
type Version uint8

const (
	Unknown Version = iota
	LegacyReference
	HardenedReference
)

// Inspect identifies an exact legacy or hardened reference using one read-only
// snapshot. The caller owns the dedicated connection and must exclude other
// writers, schema changes and concurrent use for the whole call. This checks
// limited structural/data preconditions, not role, authority or history. Legacy
// also requires positive rowids and the guarded key storage types, so its
// acceptance is narrower than replicaschema.Verify and it is not a claim of
// safety or write compatibility. No target PRAGMA or persistent row is changed.
// SQLite source IDs are compared with this process's reference driver, not a
// separately pinned release identity or WAL-fix attestation.
func Inspect(ctx context.Context, conn *sql.Conn) (version Version, err error) {
	if ctx == nil || conn == nil {
		return Unknown, invalid("nil input", nil)
	}
	defer func() {
		if e := ctx.Err(); e != nil {
			err = errors.Join(err, invalid("context at cleanup", e))
		}
		if err != nil {
			version = Unknown
		}
	}()
	tx, e := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return Unknown, invalid("begin read transaction", e)
	}
	defer func() {
		if e := tx.Rollback(); e != nil {
			err = errors.Join(err, invalid("read transaction cleanup", e))
			if retireErr := conn.Raw(func(any) error { return driver.ErrBadConn }); retireErr != nil && !errors.Is(retireErr, driver.ErrBadConn) {
				err = errors.Join(err, invalid("retire uncertain connection", retireErr))
			}
		}
	}()
	return InspectTx(ctx, tx)
}

// InspectTx identifies a reference within a caller-owned transaction without
// beginning, ending, or otherwise managing that transaction or its connection.
// The caller must enable query_only on a clean dedicated connection before
// beginning the transaction, never write within it, maintain exclusive control,
// exclude other writers and DDL through related reads, and handle transaction
// failure/cleanup. The PRAGMA check does not prove no earlier transaction writes.
// Results concern only this
// snapshot and are not authority or a guarantee about future state. It cannot
// prevent the caller from ending the transaction or changing connection state,
// and does not protect against external file changes.
func InspectTx(ctx context.Context, tx *sql.Tx) (version Version, err error) {
	if ctx == nil || tx == nil {
		return Unknown, invalid("nil input", nil)
	}
	defer func() {
		if e := ctx.Err(); e != nil {
			err = errors.Join(err, invalid("context at inspection end", e))
		}
		if err != nil {
			version = Unknown
		}
	}()
	if err = verifyPins(); err != nil {
		return Unknown, invalid("frozen migration pins", err)
	}
	var e error
	var present int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM main.sqlite_schema`).Scan(&present); e != nil {
		return Unknown, invalid("pin main snapshot", e)
	}
	if e = environment(ctx, tx); e != nil {
		return Unknown, invalid("connection environment", e)
	}
	got, e := boundedObjects(ctx, tx)
	if e != nil {
		return Unknown, invalid("target schema", e)
	}
	var targetID string
	if e = tx.QueryRowContext(ctx, `SELECT sqlite_source_id()`).Scan(&targetID); e != nil {
		return Unknown, invalid("target sqlite source", e)
	}
	if targetID == "" {
		return Unknown, invalid("empty sqlite source", nil)
	}
	matches := 0
	for _, candidate := range []Version{LegacyReference, HardenedReference} {
		ref, e := makeReference(ctx, candidate)
		if e != nil {
			return Unknown, invalid("build in-memory reference", e)
		}
		var refID string
		e = ref.QueryRowContext(ctx, `SELECT sqlite_source_id()`).Scan(&refID)
		if e == nil && targetID != refID {
			e = errDrift
		}
		var want map[string]object
		if e == nil {
			want, e = boundedObjects(ctx, ref)
		}
		closeErr := ref.Close()
		if e != nil || closeErr != nil {
			return Unknown, invalid("reference schema/source", errors.Join(e, closeErr))
		}
		if equalObjects(got, want) {
			version = candidate
			matches++
		}
	}
	if matches != 1 {
		return Unknown, invalid("schema does not match one reference", nil)
	}
	if e = validateMarkers(ctx, tx, version); e != nil {
		return Unknown, invalid("markers", e)
	}
	if e = validateRows(ctx, tx); e != nil {
		return Unknown, invalid("limited row structure", e)
	}
	if e = ctx.Err(); e != nil {
		return Unknown, invalid("context before cleanup", e)
	}
	return version, nil
}
func equalObjects(a, b map[string]object) bool {
	if len(a) != len(b) {
		return false
	}
	for k, w := range b {
		if x, ok := a[k]; !ok || x != w {
			return false
		}
	}
	return true
}

// Verify accepts only the hardened reference and uses the same one snapshot as
// Inspect. It conveys no production authority or future-state guarantee.
func Verify(ctx context.Context, conn *sql.Conn) error {
	v, err := Inspect(ctx, conn)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return invalid("context after inspection", err)
	}
	if v != HardenedReference {
		return invalid("legacy reference is not hardened", nil)
	}
	return nil
}
func environment(ctx context.Context, q reader) error {
	for _, p := range []struct{ query, want string }{{"PRAGMA query_only", "1"}, {"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"}, {"PRAGMA busy_timeout", "5000"}, {"PRAGMA defer_foreign_keys", "0"}, {"PRAGMA main.encoding", "UTF-8"}} {
		var got string
		if e := q.QueryRowContext(ctx, p.query).Scan(&got); e != nil {
			return e
		}
		if !strings.EqualFold(got, p.want) {
			return errDrift
		}
	}
	rows, e := q.QueryContext(ctx, `PRAGMA database_list`)
	if e != nil {
		return e
	}
	main := false
	for rows.Next() {
		var i int
		var name, path string
		if e = rows.Scan(&i, &name, &path); e != nil {
			rows.Close()
			return e
		}
		if name == "main" {
			main = true
		} else if name != "temp" {
			rows.Close()
			return errDrift
		}
	}
	e = rows.Err()
	closeErr := rows.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if !main {
		return errDrift
	}
	var temp int
	if e = q.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_temp_schema`).Scan(&temp); e != nil {
		return e
	}
	if temp != 0 {
		return errDrift
	}
	return nil
}
func makeReference(ctx context.Context, version Version) (*sql.DB, error) {
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*sql.DB, error) { db.Close(); return nil, err }
	ddls, e := ReferenceDDLs(version)
	if e != nil {
		return fail(e)
	}
	for _, ddl := range ddls {
		if _, e = db.ExecContext(ctx, ddl); e != nil {
			return fail(e)
		}
	}
	return db, nil
}
func boundedObjects(ctx context.Context, q reader) (map[string]object, error) {
	rows, e := q.QueryContext(ctx, `SELECT CASE WHEN typeof(type)='text' AND length(CAST(type AS BLOB))<=128 THEN type END, CASE WHEN typeof(name)='text' AND length(CAST(name AS BLOB))<=512 THEN name END, CASE WHEN typeof(tbl_name)='text' AND length(CAST(tbl_name AS BLOB))<=512 THEN tbl_name END, typeof(sql), CASE WHEN typeof(sql)='text' AND length(CAST(sql AS BLOB))<=131072 THEN sql END FROM main.sqlite_schema LIMIT 513`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make(map[string]object)
	total := 0
	for rows.Next() {
		if len(out) >= maxObjects {
			return nil, errDrift
		}
		var k, n, t, kind sql.NullString
		var ddlType string
		if e = rows.Scan(&k, &n, &t, &ddlType, &kind); e != nil {
			return nil, e
		}
		if !k.Valid || !n.Valid || !t.Valid || ((ddlType == "text") != kind.Valid) || (ddlType != "text" && ddlType != "null") {
			return nil, errDrift
		}
		total += len(k.String) + len(n.String) + len(t.String) + len(kind.String)
		if total > maxSchemaBytes {
			return nil, errDrift
		}
		o := object{k.String, n.String, t.String, kind}
		key := o.kind + "\x00" + o.name
		if _, ok := out[key]; ok {
			return nil, errDrift
		}
		out[key] = o
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if e = rows.Close(); e != nil {
		return nil, e
	}
	return out, nil
}
func validateMarkers(ctx context.Context, q reader, version Version) error {
	rows, e := q.QueryContext(ctx, `SELECT CASE WHEN typeof(version)='integer' THEN version END,CASE WHEN typeof(digest)='text' AND length(CAST(digest AS BLOB))=64 THEN digest END FROM main.enterprise_schema_migrations ORDER BY version LIMIT 3`)
	if e != nil {
		return e
	}
	n := 0
	paths := ipamstore.MigrationPaths()
	for rows.Next() {
		var v sql.NullInt64
		var d sql.NullString
		if e = rows.Scan(&v, &d); e != nil {
			rows.Close()
			return e
		}
		if n >= len(paths) || !v.Valid || !d.Valid || v.Int64 != int64(n+1) {
			rows.Close()
			return errDrift
		}
		want, ok := ipamstore.DigestOf(paths[n])
		if !ok || d.String != want {
			rows.Close()
			return errDrift
		}
		n++
	}
	e = rows.Err()
	ce := rows.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if n != 2 {
		return errDrift
	}
	if version == LegacyReference {
		return nil
	}
	if version != HardenedReference {
		return errDrift
	}
	rows, e = q.QueryContext(ctx, `SELECT CASE WHEN typeof(version)='integer' THEN version END,CASE WHEN typeof(base_digest)='text' AND length(CAST(base_digest AS BLOB))=64 THEN base_digest END,CASE WHEN typeof(guard_digest)='text' AND length(CAST(guard_digest AS BLOB))=64 THEN guard_digest END FROM main.eventlog_reference_guard_migrations LIMIT 2`)
	if e != nil {
		return e
	}
	n = 0
	for rows.Next() {
		var v sql.NullInt64
		var b, g sql.NullString
		if e = rows.Scan(&v, &b, &g); e != nil {
			rows.Close()
			return e
		}
		if !v.Valid || !b.Valid || !g.Valid || v.Int64 != 1 || b.String != baseDigest || g.String != guardDigest {
			rows.Close()
			return errDrift
		}
		n++
	}
	e = rows.Err()
	ce = rows.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if n != 1 {
		return errDrift
	}
	return nil
}
func validateRows(ctx context.Context, q reader) error {
	var status string
	if e := q.QueryRowContext(ctx, `PRAGMA main.integrity_check(1)`).Scan(&status); e != nil {
		return e
	}
	if status != "ok" {
		return errDrift
	}
	rows, e := q.QueryContext(ctx, `PRAGMA main.foreign_key_check`)
	if e != nil {
		return e
	}
	has := rows.Next()
	e = rows.Err()
	ce := rows.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if has {
		return errDrift
	}
	for _, table := range append(append([]string(nil), protected...), ipamTables...) {
		var bad int
		if e = q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.`+table+` WHERE rowid<=0)`).Scan(&bad); e != nil {
			return e
		}
		if bad != 0 {
			return errDrift
		}
	}
	for _, query := range []string{`SELECT EXISTS(SELECT 1 FROM main.writer_authority WHERE typeof(cluster_id)<>'blob' OR length(cluster_id)<>16)`, `SELECT EXISTS(SELECT 1 FROM main.replica_head WHERE typeof(cluster_id)<>'blob' OR length(cluster_id)<>16)`, `SELECT EXISTS(SELECT 1 FROM main.replica_checkpoint WHERE typeof(cluster_id)<>'blob' OR length(cluster_id)<>16 OR typeof(source_id)<>'blob' OR length(source_id)<>16)`, `SELECT EXISTS(SELECT 1 FROM main.idempotency_receipt WHERE typeof(tenant_id)<>'blob' OR length(tenant_id)<>16 OR typeof(principal)<>'text' OR typeof(method)<>'text' OR typeof(path)<>'text' OR typeof(idempotency_key)<>'text')`} {
		var bad int
		if e = q.QueryRowContext(ctx, query).Scan(&bad); e != nil {
			return e
		}
		if bad != 0 {
			return errDrift
		}
	}
	return nil
}
