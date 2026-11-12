package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/sqlitepolicy"
	"github.com/pressly/goose/v3"
)

// DB wraps the database connection and provides helper methods.
type DB struct {
	*sql.DB
	cfg config.DatabaseConfig
}

// New creates a new database connection based on the configuration.
func New(cfg config.DatabaseConfig) (*DB, error) {
	var driverName string
	switch cfg.Driver {
	case "sqlite":
		driverName = "sqlite"
	case "mysql":
		driverName = "mysql"
	case "postgresql":
		driverName = "postgres"
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	slog.Info("connecting to database", "driver", cfg.Driver, "dsn", maskDSN(cfg.DSN))

	var db *sql.DB
	if cfg.Driver == "sqlite" {
		db = sqlitepolicy.OpenWriter(cfg.DSN)
	} else {
		var err error
		db, err = sql.Open(driverName, cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("opening database: %w", err)
		}
	}

	// Configure connection pool based on driver.
	if cfg.Driver == "sqlite" {
		// SQLite only supports one writer at a time.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	} else {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		// Set connection lifetime to avoid stale connections (e.g., behind load balancers).
		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetConnMaxIdleTime(2 * time.Minute)
	}

	// Verify connection.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	slog.Info("database connected successfully")
	return &DB{DB: db, cfg: cfg}, nil
}

// RunMigrations runs all pending database migrations from the specified directory.
func (db *DB) RunMigrations(migrationsDir string) error {
	goose.SetBaseFS(nil)
	return db.runMigrations(migrationsDir)
}

// RunMigrationsFS runs migrations from an embedded filesystem.
func (db *DB) RunMigrationsFS(migrations fs.FS) error {
	goose.SetBaseFS(migrations)
	defer goose.SetBaseFS(nil)
	return db.runMigrations(".")
}

func (db *DB) runMigrations(migrationsDir string) error {
	// Map our driver names to goose dialect names.
	dialectMap := map[string]string{
		"sqlite":     "sqlite3",
		"mysql":      "mysql",
		"postgresql": "postgres",
	}
	dialect, ok := dialectMap[db.cfg.Driver]
	if !ok {
		return fmt.Errorf("unsupported driver for migrations: %s", db.cfg.Driver)
	}

	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	if err := goose.Up(db.DB, migrationsDir); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	slog.Info("database migrations completed successfully")
	return nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	slog.Info("closing database connection")
	return db.DB.Close()
}

// maskDSN masks sensitive parts of the DSN for logging.
func maskDSN(dsn string) string {
	// Handle URL-style DSN (e.g., postgres://user:pass@host/db).
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		if u.User != nil {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
		return u.String()
	}
	// Handle key=value DSN (e.g., MySQL: user:pass@tcp(host)/db).
	if atIdx := strings.Index(dsn, "@"); atIdx > 0 {
		prefix := dsn[:atIdx]
		if colonIdx := strings.LastIndex(prefix, ":"); colonIdx > 0 {
			return dsn[:colonIdx+1] + "***" + dsn[atIdx:]
		}
	}
	// Fallback: mask everything after the first 10 chars if long enough.
	if len(dsn) <= 10 {
		return "***"
	}
	return dsn[:10] + "***"
}
