// Package ipamstore installs the reviewed IPAM reference schema in an empty
// SQLite database for development and tests. It does not yet implement business
// writes, authorisation, event logging, or migration from upstream databases.
package ipamstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	_ "modernc.org/sqlite" // used only to build the in-memory reference schema
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const initialMigrationPath = "migrations/001_ipam.sql"
const rowidMigrationPath = "migrations/002_rowid_guards.sql"

// MigrationPaths returns a fresh list of reference migrations in apply order.
func MigrationPaths() []string { return []string{initialMigrationPath, rowidMigrationPath} }

// migrationDigests pins the reviewed reference schema. A silent edit to a
// migration would change what the concurrency guards mean without anything
// noticing, so the digest is part of the contract under test.
var migrationDigests = map[string]string{
	"migrations/001_ipam.sql":         "d5b68888951e6edda528cb7c19f6e7a3ae7f5b234fc12657edafb88a12688ad3",
	"migrations/002_rowid_guards.sql": "9172d79d81131b13e22d36cd6c16706ecb1500d5b7e141cf16de87d3409734f3",
}

// ErrMigrationDrifted is returned when a migration file no longer matches the
// reviewed digest.
var ErrMigrationDrifted = errors.New("ipamstore: migration content drifted from the reviewed reference")

// Store is one node's IPAM slice plus its replication log.
type Store struct {
	db          *sql.DB
	log         *eventlog.Log
	tenantSpace TenantSpace
}

// TenantSpace names the scope a Store instance operates on. Repository
// methods take it explicitly: SQLite has no row-level security, so a missing
// predicate is a cross-tenant read, not an accident of the driver
// (docs/02-data-ipam.md "仓储方法必须显式接收 tenant / space").
type TenantSpace struct {
	TenantID string
	SpaceID  string
}

// New binds a database handle to a node identity and one tenant/scope pair.
func New(db *sql.DB, cluster, node eventlog.ID, scope TenantSpace) *Store {
	return &Store{
		db:          db,
		log:         eventlog.NewLog(db, cluster, node),
		tenantSpace: scope,
	}
}

// Log exposes the replication log for readers and for the takeover tooling.
func (s *Store) Log() *eventlog.Log { return s.log }

// DB exposes the handle for tests and for the migration runner. Application
// writes must go through the transactional methods, never through this.
func (s *Store) DB() *sql.DB { return s.db }

// ErrExistingSchema prevents applying the empty-database reference schema to
// an existing upstream database. A real upgrade requires a separate migration.
var ErrExistingSchema = errors.New("ipamstore: reference migration requires an empty database")

// Migrate installs the pinned reference schema only in an otherwise empty
// database. The version marker and all DDL are committed together. It does not
// constitute an upgrade of an existing GoDDI database.
func Migrate(ctx context.Context, db *sql.DB) error {
	return migrate(ctx, db, applyMigration)
}

func migrate(ctx context.Context, db *sql.DB, apply func(context.Context, *sql.Conn, string) error) (err error) {
	if err = VerifyDigests(); err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, check := range []struct{ query, expected string }{{"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"}, {"PRAGMA defer_foreign_keys", "0"}} {
		var got string
		if err = conn.QueryRowContext(ctx, check.query).Scan(&got); err != nil {
			return err
		}
		if !strings.EqualFold(got, check.expected) {
			return fmt.Errorf("ipamstore: unsafe %s", check.query)
		}
	}
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(cleanup, "ROLLBACK"); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("ipamstore: rollback unconfirmed: %w", e))
		}
	}()
	if err = verifyDatabaseList(ctx, conn); err != nil {
		return err
	}
	var tempCount int
	if err = conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_temp_schema`).Scan(&tempCount); err != nil {
		return err
	}
	if tempCount != 0 {
		return fmt.Errorf("%w: temporary objects", ErrMigrationDrifted)
	}
	var markerExists, count int
	if err = conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='enterprise_schema_migrations'`).Scan(&markerExists); err != nil {
		return err
	}
	if err = conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'enterprise_schema_migrations'`).Scan(&count); err != nil {
		return err
	}
	version := 0
	if markerExists == 0 {
		if count != 0 {
			return ErrExistingSchema
		}
		if err = apply(ctx, conn, initialMigrationPath); err != nil {
			return fmt.Errorf("ipamstore: migrate v1: %w", err)
		}
		if _, err = conn.ExecContext(ctx, `CREATE TABLE enterprise_schema_migrations (version INTEGER PRIMARY KEY, digest TEXT NOT NULL) STRICT`); err != nil {
			return err
		}
		if err = insertMarker(ctx, conn, 1, migrationDigests[initialMigrationPath]); err != nil {
			return err
		}
		version = 1
	} else {
		version, err = readVersion(ctx, conn)
		if err != nil {
			return err
		}
	}
	if err = verifySchema(ctx, conn, version); err != nil {
		return err
	}
	if err = verifyData(ctx, conn); err != nil {
		return err
	}
	if version == 1 {
		if err = apply(ctx, conn, rowidMigrationPath); err != nil {
			return fmt.Errorf("ipamstore: migrate v2: %w", err)
		}
		if err = insertMarker(ctx, conn, 2, migrationDigests[rowidMigrationPath]); err != nil {
			return err
		}
		if err = verifySchema(ctx, conn, 2); err != nil {
			return err
		}
		if err = verifyData(ctx, conn); err != nil {
			return err
		}
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("ipamstore: migration commit outcome unknown: %w", err)
	}
	committed = true
	return nil
}
func verifyDatabaseList(ctx context.Context, c *sql.Conn) error {
	rows, err := c.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return err
	}
	defer rows.Close()
	seenMain := false
	for rows.Next() {
		var n int
		var name, path string
		if err := rows.Scan(&n, &name, &path); err != nil {
			return err
		}
		if name == "main" {
			seenMain = true
		} else if name != "temp" {
			return fmt.Errorf("%w: attached database", ErrMigrationDrifted)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !seenMain {
		return ErrMigrationDrifted
	}
	return nil
}
func insertMarker(ctx context.Context, c *sql.Conn, version int, digest string) error {
	result, err := c.ExecContext(ctx, `INSERT INTO enterprise_schema_migrations(version,digest) VALUES (?,?)`, version, digest)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrMigrationDrifted
	}
	return nil
}
func readVersion(ctx context.Context, c *sql.Conn) (int, error) {
	rows, err := c.QueryContext(ctx, `SELECT CASE WHEN typeof(version)='integer' THEN version END, CASE WHEN typeof(digest)='text' AND length(CAST(digest AS BLOB))=64 THEN digest END FROM main.enterprise_schema_migrations ORDER BY version LIMIT 3`)
	if err != nil {
		return 0, fmt.Errorf("%w: marker: %v", ErrMigrationDrifted, err)
	}
	defer rows.Close()
	paths := MigrationPaths()
	version := 0
	for rows.Next() {
		var n sql.NullInt64
		var digest sql.NullString
		if err := rows.Scan(&n, &digest); err != nil {
			return 0, fmt.Errorf("%w: invalid marker", ErrMigrationDrifted)
		}
		if !n.Valid || !digest.Valid || version >= len(paths) || n.Int64 != int64(version+1) || digest.String != migrationDigests[paths[version]] {
			return 0, fmt.Errorf("%w: invalid marker", ErrMigrationDrifted)
		}
		version++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if version == 0 {
		return 0, ErrMigrationDrifted
	}
	return version, nil
}
func verifyData(ctx context.Context, c *sql.Conn) error {
	var status string
	if err := c.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&status); err != nil {
		return err
	}
	if status != "ok" {
		return fmt.Errorf("%w: integrity", ErrMigrationDrifted)
	}
	rows, err := c.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	bad := rows.Next()
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if bad {
		return fmt.Errorf("%w: foreign keys", ErrMigrationDrifted)
	}
	for _, name := range []string{"ipam_resource_ids", "ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_pools", "ipam_occupancies"} {
		var bad int
		if err = c.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+name+` WHERE rowid<=0)`).Scan(&bad); err != nil {
			return err
		}
		if bad != 0 {
			return fmt.Errorf("%w: nonpositive rowid in %s", ErrMigrationDrifted, name)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, path string) error {
	raw, err := migrationFS.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: %v", fs.ErrNotExist, err)
	}
	want, ok := migrationDigests[path]
	if !ok || digestOf(raw) != want {
		return fmt.Errorf("%w: %s", ErrMigrationDrifted, path)
	}
	// The pinned reference includes its own transaction wrapper. The outer
	// pinned connection owns it here, allowing reliable rollback on failure.
	body := string(raw)
	if strings.Count(body, "BEGIN IMMEDIATE;") != 1 || !strings.HasSuffix(strings.TrimSpace(body), "COMMIT;") {
		return fmt.Errorf("%w: unexpected transaction wrapper", ErrMigrationDrifted)
	}
	body = strings.Replace(body, "BEGIN IMMEDIATE;", "", 1)
	body = strings.TrimSuffix(strings.TrimSpace(body), "COMMIT;")
	_, err = conn.ExecContext(ctx, body)
	return err
}

// verifySchema compares every IPAM object with an independently constructed
// copy of the pinned reference. A checksum marker alone cannot prove the
// database still has the tables, indexes and triggers it claims to have.
func verifySchema(ctx context.Context, conn *sql.Conn, version int) error {
	if version != 1 && version != 2 {
		return ErrMigrationDrifted
	}
	reference, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer reference.Close()
	reference.SetMaxOpenConns(1)
	for _, path := range MigrationPaths()[:version] {
		raw, e := migrationFS.ReadFile(path)
		if e != nil {
			return e
		}
		if _, e = reference.ExecContext(ctx, string(raw)); e != nil {
			return fmt.Errorf("ipamstore: build reference schema: %w", e)
		}
	}
	if _, err = reference.ExecContext(ctx, `CREATE TABLE enterprise_schema_migrations (version INTEGER PRIMARY KEY, digest TEXT NOT NULL) STRICT`); err != nil {
		return err
	}
	want, err := schemaObjects(ctx, reference)
	if err != nil {
		return err
	}
	got, err := schemaObjects(ctx, conn)
	if err != nil {
		return err
	}
	if len(got) != len(want) {
		return fmt.Errorf("%w: reference object count", ErrMigrationDrifted)
	}
	for k, v := range want {
		if actual, ok := got[k]; !ok || actual != v {
			return fmt.Errorf("%w: missing or changed schema object %s", ErrMigrationDrifted, k)
		}
	}
	return nil
}

type schemaQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func schemaObjects(ctx context.Context, q schemaQuerier) (map[string]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT type, name, sql FROM sqlite_schema
		WHERE (lower(substr(name,1,5))='ipam_' OR lower(name)='enterprise_schema_migrations' OR (type IN ('index','trigger') AND (lower(substr(tbl_name,1,5))='ipam_' OR lower(tbl_name)='enterprise_schema_migrations'))) AND type IN ('table','index','trigger','view') AND sql IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	objects := make(map[string]string)
	for rows.Next() {
		var kind, name, ddl string
		if err := rows.Scan(&kind, &name, &ddl); err != nil {
			return nil, err
		}
		objects[kind+":"+name] = ddl
	}
	return objects, rows.Err()
}

func digestOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// DigestOf reports the pinned digest of a migration, for the drift test.
func DigestOf(path string) (string, bool) {
	d, ok := migrationDigests[path]
	return d, ok
}

// ContentsOf returns the embedded migration text; tests and the CLI use it
// instead of reaching into the filesystem.
func ContentsOf(path string) (string, error) {
	raw, err := migrationFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// VerifyDigests recomputes the digests of the embedded migrations against the
// pinned values. It is cheap and runs at startup so an operator sees drift
// before the first write, not after a failed failover.
func VerifyDigests() error {
	for _, path := range MigrationPaths() {
		raw, err := migrationFS.ReadFile(path)
		if err != nil {
			return err
		}
		want, ok := migrationDigests[path]
		if !ok {
			return fmt.Errorf("%w: %s", ErrMigrationDrifted, path)
		}
		got := sha256.Sum256(raw)
		if hex.EncodeToString(got[:]) != want {
			return fmt.Errorf("%w: %s is %x, reviewed %s", ErrMigrationDrifted, path, got, want)
		}
	}
	return nil
}
