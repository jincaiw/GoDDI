// Package replicaschema checks an isolated candidate database structure. It is
// not a migration, source authentication, data validation, or permission to apply.
package replicaschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlogguard"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	_ "modernc.org/sqlite"
)

var ErrInvalid = errors.New("replicaschema: unverified candidate schema")

type object struct {
	Type, Name, Table string
	SQL               sql.NullString
}
type reader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Verify checks a caller-owned connection already set query_only=ON and using
// the SQLite durability policy. It changes no target setting or persistent data.
// It rejects all unknown main objects and all TEMP objects: DHCP prototype and
// other application schemas are intentionally unsupported until reviewed.
//
// Both frozen local reference structures are accepted: the legacy reference
// and the hardened reference that adds rowid/logical-key guards and its own
// marker table. Classification and marker/row validation are delegated to
// eventlogguard.InspectTx so the two verifiers cannot disagree about which
// structure they are looking at. A success is structural evidence only, not a
// trusted source, a write permit or a production gate.
// The caller must prevent concurrent connection use and keep this check and any
// later operation within its own appropriate isolation and authority protocol.
func Verify(ctx context.Context, conn *sql.Conn) error {
	if conn == nil {
		return ErrInvalid
	}
	for _, c := range []struct{ q, w string }{{"PRAGMA query_only", "1"}, {"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"}, {"PRAGMA busy_timeout", "5000"}, {"PRAGMA defer_foreign_keys", "0"}} {
		var got string
		if err := conn.QueryRowContext(ctx, c.q).Scan(&got); err != nil || !strings.EqualFold(got, c.w) {
			return fmt.Errorf("%w: %s", ErrInvalid, c.q)
		}
	}
	paths := ipamstore.MigrationPaths()
	if len(paths) != 2 || paths[0] != "migrations/001_ipam.sql" || paths[1] != "migrations/002_rowid_guards.sql" {
		return fmt.Errorf("%w: unsupported migration set", ErrInvalid)
	}
	if err := ipamstore.VerifyDigests(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	// Build the reference from the same frozen DDL set eventlogguard uses, so
	// the two verifiers cannot drift apart. The source ID comparison only
	// needs one reference; the structural match is done inside the transaction
	// below against the detected version.
	legacyDDLs, err := eventlogguard.ReferenceDDLs(eventlogguard.LegacyReference)
	if err != nil {
		return fmt.Errorf("%w: reference DDL: %v", ErrInvalid, err)
	}
	sourceRef, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer sourceRef.Close()
	sourceRef.SetMaxOpenConns(1)
	for _, ddl := range legacyDDLs {
		if _, err = sourceRef.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("%w: build reference: %v", ErrInvalid, err)
		}
	}
	var targetSource, referenceSource string
	if err = conn.QueryRowContext(ctx, "SELECT sqlite_source_id()").Scan(&targetSource); err != nil {
		return err
	}
	if err = sourceRef.QueryRowContext(ctx, "SELECT sqlite_source_id()").Scan(&referenceSource); err != nil {
		return err
	}
	if targetSource == "" || targetSource != referenceSource {
		return fmt.Errorf("%w: SQLite source mismatch", ErrInvalid)
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		return err
	}
	for rows.Next() {
		var seq int
		var name, file string
		if err = rows.Scan(&seq, &name, &file); err != nil {
			rows.Close()
			return err
		}
		if name != "main" && name != "temp" {
			rows.Close()
			return fmt.Errorf("%w: attached database", ErrInvalid)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	temp, err := objects(ctx, tx, "sqlite_temp_schema")
	if err != nil {
		return err
	}
	if len(temp) != 0 {
		return fmt.Errorf("%w: temporary objects", ErrInvalid)
	}
	// Classify the target as exactly one frozen local reference. InspectTx
	// performs its own exact object comparison plus marker and limited row
	// validation for both the legacy and the hardened structure.
	version, gerr := eventlogguard.InspectTx(ctx, tx)
	if gerr != nil || (version != eventlogguard.LegacyReference && version != eventlogguard.HardenedReference) {
		return fmt.Errorf("%w: local structure: %v", ErrInvalid, gerr)
	}
	ddls, err := eventlogguard.ReferenceDDLs(version)
	if err != nil {
		return fmt.Errorf("%w: reference DDL: %v", ErrInvalid, err)
	}
	ref, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer ref.Close()
	ref.SetMaxOpenConns(1)
	for _, ddl := range ddls {
		if _, err = ref.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("%w: build %v reference: %v", ErrInvalid, version, err)
		}
	}
	want, err := objects(ctx, ref, "sqlite_schema")
	if err != nil {
		return err
	}
	got, err := objects(ctx, tx, "main.sqlite_schema")
	if err != nil {
		return err
	}
	if len(got) != len(want) {
		return fmt.Errorf("%w: object count", ErrInvalid)
	}
	for key, v := range want {
		if actual, ok := got[key]; !ok || actual != v {
			return fmt.Errorf("%w: schema object %s", ErrInvalid, key)
		}
	}
	markers, err := tx.QueryContext(ctx, `SELECT CASE WHEN typeof(version)='integer' THEN version END, CASE WHEN typeof(digest)='text' AND length(CAST(digest AS BLOB))=64 THEN digest END FROM main.enterprise_schema_migrations ORDER BY version LIMIT 3`)
	if err != nil {
		return fmt.Errorf("%w: marker: %v", ErrInvalid, err)
	}
	count := 0
	for markers.Next() {
		var version sql.NullInt64
		var actual sql.NullString
		if err = markers.Scan(&version, &actual); err != nil {
			markers.Close()
			return err
		}
		count++
		if !version.Valid || !actual.Valid || count > len(paths) || version.Int64 != int64(count) {
			markers.Close()
			return fmt.Errorf("%w: invalid marker", ErrInvalid)
		}
		digest, ok := ipamstore.DigestOf(paths[count-1])
		if !ok || actual.String != digest {
			markers.Close()
			return fmt.Errorf("%w: invalid marker", ErrInvalid)
		}
	}
	if err = markers.Err(); err != nil {
		markers.Close()
		return err
	}
	markers.Close()
	if count != len(paths) {
		return fmt.Errorf("%w: marker count", ErrInvalid)
	}
	return nil
}
func objects(ctx context.Context, q reader, table string) (map[string]object, error) {
	rows, err := q.QueryContext(ctx, `SELECT type,name,tbl_name,sql FROM `+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]object{}
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.Type, &o.Name, &o.Table, &o.SQL); err != nil {
			return nil, err
		}
		key := o.Type + ":" + o.Name
		if _, ok := result[key]; ok {
			return nil, ErrInvalid
		}
		result[key] = o
	}
	return result, rows.Err()
}
