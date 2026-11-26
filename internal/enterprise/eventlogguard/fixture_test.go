package eventlogguard

// This runner exists only in tests. It is not a deployment migration.
import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	_ "modernc.org/sqlite"
)

type hooks struct {
	afterDDL     func(*sql.Conn) error
	beforeCommit func() error
	afterCommit  func() error
}

type schemaObject struct {
	kind, name, table string
	sql               sql.NullString
}
type qreader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func objects(ctx context.Context, q qreader) (map[string]schemaObject, error) {
	rows, e := q.QueryContext(ctx, `SELECT type,name,tbl_name,sql FROM main.sqlite_schema`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]schemaObject{}
	for rows.Next() {
		var o schemaObject
		if e = rows.Scan(&o.kind, &o.name, &o.table, &o.sql); e != nil {
			return nil, e
		}
		key := o.kind + "\x00" + o.name
		if _, ok := out[key]; ok {
			return nil, errDrift
		}
		out[key] = o
	}
	return out, rows.Err()
}
func reference(ctx context.Context, version int) (*sql.DB, error) {
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*sql.DB, error) { db.Close(); return nil, err }
	for _, p := range ipamstore.MigrationPaths() {
		text, e := ipamstore.ContentsOf(p)
		if e != nil {
			return fail(e)
		}
		if _, e = db.ExecContext(ctx, text); e != nil {
			return fail(e)
		}
	}
	if _, e = db.ExecContext(ctx, ipamMarkerDDL); e != nil {
		return fail(e)
	}
	if _, e = db.ExecContext(ctx, eventlog.Schema); e != nil {
		return fail(e)
	}
	if version == 1 {
		if _, e = db.ExecContext(ctx, guardSQL); e != nil {
			return fail(e)
		}
		if _, e = db.ExecContext(ctx, markerDDL); e != nil {
			return fail(e)
		}
	}
	return db, nil
}
func sameSchema(ctx context.Context, c *sql.Conn, version int) error {
	ref, e := reference(ctx, version)
	if e != nil {
		return e
	}
	defer ref.Close()
	var a, b string
	if e = c.QueryRowContext(ctx, `SELECT sqlite_source_id()`).Scan(&a); e != nil {
		return e
	}
	if e = ref.QueryRowContext(ctx, `SELECT sqlite_source_id()`).Scan(&b); e != nil {
		return e
	}
	if a == "" || a != b {
		return errDrift
	}
	want, e := objects(ctx, ref)
	if e != nil {
		return e
	}
	got, e := objects(ctx, c)
	if e != nil {
		return e
	}
	if len(got) != len(want) {
		return errDrift
	}
	for k, w := range want {
		if x, ok := got[k]; !ok || x != w {
			return fmt.Errorf("%w: object %s", errDrift, k)
		}
	}
	return nil
}
func markers(ctx context.Context, c *sql.Conn, version int) error {
	rows, e := c.QueryContext(ctx, `SELECT CASE WHEN typeof(version)='integer' THEN version END,CASE WHEN typeof(digest)='text' AND length(CAST(digest AS BLOB))=64 THEN digest END FROM main.enterprise_schema_migrations ORDER BY version LIMIT 3`)
	if e != nil {
		return e
	}
	paths := ipamstore.MigrationPaths()
	n := 0
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
	closeErr := rows.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if n != len(paths) {
		return errDrift
	}
	if version == 0 {
		return nil
	}
	rows, e = c.QueryContext(ctx, `SELECT CASE WHEN typeof(version)='integer' THEN version END, CASE WHEN typeof(base_digest)='text' AND length(CAST(base_digest AS BLOB))=64 THEN base_digest END,CASE WHEN typeof(guard_digest)='text' AND length(CAST(guard_digest AS BLOB))=64 THEN guard_digest END FROM main.eventlog_reference_guard_migrations LIMIT 2`)
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
	closeErr = rows.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if n != 1 {
		return errDrift
	}
	return nil
}
func checkEnvironment(ctx context.Context, c *sql.Conn) error {
	for _, p := range []struct{ q, w string }{{"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"}, {"PRAGMA busy_timeout", "5000"}, {"PRAGMA defer_foreign_keys", "0"}, {"PRAGMA query_only", "0"}, {"PRAGMA main.encoding", "UTF-8"}} {
		var got string
		if e := c.QueryRowContext(ctx, p.q).Scan(&got); e != nil {
			return e
		}
		if !strings.EqualFold(got, p.w) {
			return errDrift
		}
	}
	rows, e := c.QueryContext(ctx, `PRAGMA database_list`)
	if e != nil {
		return e
	}
	main := false
	for rows.Next() {
		var idx int
		var name, path string
		if e = rows.Scan(&idx, &name, &path); e != nil {
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
	var count int
	if e = c.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_temp_schema`).Scan(&count); e != nil {
		return e
	}
	if count != 0 {
		return errDrift
	}
	return nil
}
func checkData(ctx context.Context, c *sql.Conn) error {
	var status string
	if e := c.QueryRowContext(ctx, `PRAGMA main.integrity_check(1)`).Scan(&status); e != nil {
		return e
	}
	if status != "ok" {
		return errDrift
	}
	rows, e := c.QueryContext(ctx, `PRAGMA main.foreign_key_check`)
	if e != nil {
		return e
	}
	bad := rows.Next()
	e = rows.Err()
	closeErr := rows.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if bad {
		return errDrift
	}
	for _, table := range append(append([]string(nil), protected...), ipamTables...) {
		var bad int
		if e = c.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.`+table+` WHERE rowid<=0)`).Scan(&bad); e != nil {
			return e
		}
		if bad != 0 {
			return errDrift
		}
	}
	for _, q := range []string{`SELECT EXISTS(SELECT 1 FROM main.writer_authority WHERE typeof(cluster_id)<>'blob' OR length(cluster_id)<>16)`, `SELECT EXISTS(SELECT 1 FROM main.replica_head WHERE typeof(cluster_id)<>'blob' OR length(cluster_id)<>16)`, `SELECT EXISTS(SELECT 1 FROM main.replica_checkpoint WHERE typeof(cluster_id)<>'blob' OR length(cluster_id)<>16 OR typeof(source_id)<>'blob' OR length(source_id)<>16)`, `SELECT EXISTS(SELECT 1 FROM main.idempotency_receipt WHERE typeof(tenant_id)<>'blob' OR length(tenant_id)<>16 OR typeof(principal)<>'text' OR typeof(method)<>'text' OR typeof(path)<>'text' OR typeof(idempotency_key)<>'text')`} {
		var bad int
		if e = c.QueryRowContext(ctx, q).Scan(&bad); e != nil {
			return e
		}
		if bad != 0 {
			return errDrift
		}
	}
	return nil
}

// migrateFixture operates only on an isolated test database. Other writers and
// authority are outside its scope; successful verification proves no history.
func migrateFixture(ctx context.Context, db *sql.DB, h hooks) (err error) {
	if ctx == nil || db == nil {
		return errDrift
	}
	if err = verifyPins(); err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	checkCtx, checkCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	err = checkEnvironment(checkCtx, conn)
	checkCancel()
	if err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		// The transaction state may be uncertain after an interrupted BEGIN.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(cleanup, `ROLLBACK`); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("rollback uncertain: %w", e))
		} else if e := checkEnvironment(cleanup, conn); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("connection retired after rollback: %w", e))
		}
	}()
	if err = checkEnvironment(ctx, conn); err != nil {
		return err
	}
	var count int
	if err = conn.QueryRowContext(ctx, `SELECT count(*) FROM main.sqlite_schema`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%w: empty database needs existing reference initialization", errDrift)
	}
	var marker int
	if err = conn.QueryRowContext(ctx, `SELECT count(*) FROM main.sqlite_schema WHERE type='table' AND name='eventlog_reference_guard_migrations'`).Scan(&marker); err != nil {
		return err
	}
	if marker > 1 {
		return errDrift
	}
	if err = sameSchema(ctx, conn, marker); err != nil {
		return err
	}
	if err = markers(ctx, conn, marker); err != nil {
		return err
	}
	if err = checkData(ctx, conn); err != nil {
		return err
	}
	if marker == 0 {
		if _, err = conn.ExecContext(ctx, guardSQL); err != nil {
			return err
		}
		if _, err = conn.ExecContext(ctx, markerDDL); err != nil {
			return err
		}
		if h.afterDDL != nil {
			if err = h.afterDDL(conn); err != nil {
				return err
			}
		}
		var result sql.Result
		if result, err = conn.ExecContext(ctx, `INSERT INTO main.eventlog_reference_guard_migrations VALUES(1,?,?)`, baseDigest, guardDigest); err != nil {
			return err
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return errDrift
		}
		if err = sameSchema(ctx, conn, 1); err != nil {
			return err
		}
		if err = markers(ctx, conn, 1); err != nil {
			return err
		}
		if err = checkData(ctx, conn); err != nil {
			return err
		}
	}
	if h.beforeCommit != nil {
		if err = h.beforeCommit(); err != nil {
			return err
		}
	}
	// The hooks are trusted test code and must not execute transaction control.
	// Recheck after hooks so TEMP, ATTACH or connection policy drift cannot
	// accidentally be accepted as a migrated reference.
	if err = checkEnvironment(ctx, conn); err != nil {
		return err
	}
	if err = sameSchema(ctx, conn, 1); err != nil {
		return err
	}
	if err = markers(ctx, conn, 1); err != nil {
		return err
	}
	if err = checkData(ctx, conn); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `COMMIT`); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("commit outcome unknown: %w", err)
	}
	committed = true
	if h.afterCommit != nil {
		if err = h.afterCommit(); err != nil {
			return fmt.Errorf("simulated post-commit uncertainty: %w", err)
		}
	}
	return nil
}
