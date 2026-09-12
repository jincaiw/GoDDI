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
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonwa/goddi/internal/config"
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
// address the server has never heard of". FULL is slower per commit and is the
// only setting that makes the ACK honest.
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

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("dataplane: opening %s store: %w", role, err)
	}

	// A data plane is a single writer over its own file. Capping the pool at
	// one connection also removes the read-then-write interleavings that
	// SQLite's deferred transactions cannot express.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("dataplane: connecting to %s store: %w", role, err)
	}

	if err := applyPragmas(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("dataplane: %s store: %w", role, err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("dataplane: migrating %s store: %w", role, err)
	}

	slog.Info("data-plane store ready", "role", role, "dsn", dsn)
	return &Store{DB: db, role: role, dsn: dsn}, nil
}

// requiredPragmas are applied and then read back. A pragma that silently did
// not take effect is worse than one that failed: the process would run with
// durability the operator did not ask for and cannot see.
var requiredPragmas = []struct {
	name string
	want string
}{
	{"busy_timeout", "5000"},
	{"foreign_keys", "1"},
	{"journal_mode", "wal"},
	{"synchronous", "2"}, // 2 = FULL
}

func applyPragmas(db *sql.DB) error {
	for _, p := range requiredPragmas {
		if _, err := db.Exec("PRAGMA " + p.name + " = " + p.want); err != nil {
			return fmt.Errorf("setting PRAGMA %s = %s: %w", p.name, p.want, err)
		}
		var got string
		if err := db.QueryRow("PRAGMA " + p.name).Scan(&got); err != nil {
			return fmt.Errorf("reading back PRAGMA %s: %w", p.name, err)
		}
		if !strings.EqualFold(strings.TrimSpace(got), p.want) {
			return fmt.Errorf("PRAGMA %s reads back as %q, want %q", p.name, got, p.want)
		}
	}
	return nil
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
	path := dsnFile(dsn)
	if path == "" {
		return nil, fmt.Errorf("dataplane: the %s store has no file path", role)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("dataplane: the %s store: %w", role, err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
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
