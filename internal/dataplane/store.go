// Package dataplane owns the local stores the DNS and DHCP roles serve from.
//
// A data plane must keep doing its job while the management process is
// restarting, being upgraded or unreachable. Serving that from the control
// database is impossible by construction: the control database is the thing
// that goes away. So each data plane keeps a file it owns, and treats the
// control database as a source of configuration it copies in on a poll.
//
// Two directions of replication exist and they are not symmetric:
//
//   - Downward: control-plane configuration (scopes, reservations, options,
//     zones, records) is copied into the store. It is a replica — the control
//     plane is the author.
//   - Upward: lease state is authored here and copied up for the console, the
//     dependency checks and backups. The control database's copy is the
//     replica.
//
// The asymmetry is the point: whoever owns the data is the one that can keep
// working when the other side is gone.
package dataplane

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Migrations returns the embedded data-plane migrations.
func Migrations() fs.FS {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

// Store is a local data-plane database.
type Store struct {
	*sql.DB

	role config.DataPlaneRole
	dsn  string
}

// Role reports which data plane this store belongs to.
func (s *Store) Role() config.DataPlaneRole { return s.role }

// DSN reports the location the store was opened at.
func (s *Store) DSN() string { return s.dsn }

// Open creates or opens the data-plane store for a role and brings its schema
// up to date.
//
// The durability settings are the ones the acceptance criteria depend on: a
// lease must be on disk before the client is told it has one. SQLite's default
// synchronous=NORMAL in WAL mode acknowledges a commit before the write is
// durable, which turns a power loss into "the client believes it holds an
// address the server has never heard of". FULL requests a WAL sync before a
// successful commit is returned. The target filesystem and storage device
// still determine whether that sync survives physical power loss.
func Open(role config.DataPlaneRole, dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("dataplane: %s store has no DSN", role)
	}

	// A bare filesystem path needs its directory to exist before SQLite can
	// create the file, and the failure otherwise reads as an opaque
	// "unable to open database file".
	if dir := dsnDir(dsn); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("dataplane: creating %s for the %s store: %w", dir, role, err)
		}
	}

	db := sqlitepolicy.OpenWriter(dsn)

	// A data plane is a single writer over its own file. Capping the pool at
	// one connection also removes the read-then-write interleavings that
	// SQLite's deferred transactions cannot express.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("dataplane: connecting to %s store: %w", role, err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("dataplane: migrating %s store: %w", role, err)
	}

	slog.Info("data-plane store ready", "role", role, "dsn", dsn)
	return &Store{DB: db, role: role, dsn: dsn}, nil
}

func runMigrations(db *sql.DB) error {
	goose.SetBaseFS(Migrations())
	defer goose.SetBaseFS(nil)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(db, ".")
}

// dsnDir returns the directory a filesystem DSN needs to exist.
// OpenReadOnly opens a store for inspection.
//
// It deliberately does none of what Open does. It creates nothing, applies no
// pragmas and runs no migrations. Those are all writes, and a command whose
// output is a statement about what is already on disk must not be the thing
// that changes it -- `journal_mode = wal` on its own is a permanent change to
// the file it is applied to, and a migration is a larger one.
//
// The file must already exist. Opening a SQLite path creates it, so a caller
// that wants "there is no store here" as an answer has to ask before opening;
// this constructor refuses rather than bringing into existence the database it
// was asked to describe.
func OpenReadOnly(role config.DataPlaneRole, dsn string) (*Store, error) {
	path, err := ReadOnlyPath(dsn)
	if err != nil {
		return nil, fmt.Errorf("dataplane: the %s store: %w", role, err)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("dataplane: the %s store: %w", role, err)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("dataplane: resolving the %s store path: %w", role, err)
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute), RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, fmt.Errorf("dataplane: opening the %s store read-only: %w", role, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("dataplane: connecting to the %s store: %w", role, err)
	}
	return &Store{DB: db, role: role, dsn: dsn}, nil
}

// ReadOnlyPath extracts the filename without forwarding any DSN parameters to
// the driver. Bare paths are literal, including '?' and '#'; use a file URI
// for a DSN containing query parameters.
func ReadOnlyPath(dsn string) (string, error) {
	dsn = strings.TrimSpace(dsn)
	path := dsn
	if strings.HasPrefix(dsn, "file:") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("invalid SQLite file URI: %w", err)
		}
		if u.Fragment != "" || (u.Host != "" && u.Host != "localhost") || u.User != nil {
			return "", fmt.Errorf("unsupported SQLite file URI authority or fragment")
		}
		params, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return "", fmt.Errorf("invalid SQLite file URI query: %w", err)
		}
		for _, mode := range params["mode"] {
			if strings.EqualFold(mode, "memory") {
				return "", fmt.Errorf("memory databases cannot be inspected as files")
			}
		}
		path = u.Path
		if u.Opaque != "" {
			path, err = url.PathUnescape(u.Opaque)
			if err != nil {
				return "", fmt.Errorf("invalid SQLite file URI path: %w", err)
			}
		}
	}
	if path == "" || path == ":memory:" || strings.IndexByte(path, 0) >= 0 {
		return "", fmt.Errorf("SQLite DSN has no usable file path")
	}
	return path, nil
}

// dsnDir is the directory a DSN's file lives in, or "" when it is not a file.
func dsnDir(dsn string) string {
	path := dsnFile(dsn)
	if path == "" {
		return ""
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return ""
	}
	return dir
}

// dsnFile is the filesystem path a DSN names, or "" for an in-memory database.
func dsnFile(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if after, ok := strings.CutPrefix(dsn, "file:"); ok {
		dsn = after
	}
	// Strip any query string: the driver's parameters are not part of the path.
	if i := strings.Index(dsn, "?"); i >= 0 {
		dsn = dsn[:i]
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" || dsn == ":memory:" {
		return ""
	}
	return dsn
}
