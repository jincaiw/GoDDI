// Package sqlitepolicy configures durable SQLite writer connections.
package sqlitepolicy

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"modernc.org/sqlite"
)

// OpenWriter creates a pool whose physical connections enforce the writer policy.
// The caller configures pool limits and must Ping before using the database.
func OpenWriter(dsn string) *sql.DB {
	d := &sqlite.Driver{}
	d.RegisterConnectionHook(configureSQLiteConnection)
	return sql.OpenDB(sqliteConnector{driver: d, dsn: dsn})
}

// sqliteConnector confines the hook to pools that opt into this policy.
type sqliteConnector struct {
	driver *sqlite.Driver
	dsn    string
}

func (c sqliteConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateDSN(c.dsn); err != nil {
		return nil, err
	}
	return c.driver.Open(c.dsn)
}
func (c sqliteConnector) Driver() driver.Driver { return c.driver }

// The driver executes _pragma parameters before the connection hook runs.
// Parse the query exactly as the driver does, including on non-file DSNs.
func validateDSN(dsn string) error {
	pos := strings.IndexRune(dsn, '?')
	if pos < 1 {
		return nil
	}
	params, err := url.ParseQuery(dsn[pos+1:])
	if err != nil {
		return fmt.Errorf("invalid SQLite DSN query: %w", err)
	}
	for name := range params {
		// A custom VFS changes the storage implementation before the hook can
		// verify durability. immutable changes SQLite's locking assumptions.
		if strings.EqualFold(name, "_pragma") || strings.EqualFold(name, "vfs") || strings.EqualFold(name, "immutable") {
			return fmt.Errorf("SQLite DSN parameter %q is not allowed for writer connections", name)
		}
	}
	return nil
}

// configureSQLiteConnection is called by modernc on every physical open. The
// driver closes the connection when the hook fails, before it reaches the pool.
func configureSQLiteConnection(conn sqlite.ExecQuerierContext, _ string) error {
	ctx := context.Background()
	version, err := sqliteScalar(ctx, conn, "SELECT sqlite_version()")
	if err != nil {
		return fmt.Errorf("read SQLite version: %w", err)
	}
	source, err := sqliteScalar(ctx, conn, "SELECT sqlite_source_id()")
	if err != nil {
		return fmt.Errorf("read SQLite source ID: %w", err)
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("empty SQLite source ID")
	}
	if !sqliteVersionSupported(version) {
		return fmt.Errorf("SQLite version %q is below 3.51.3 or invalid", version)
	}
	for _, stmt := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = FULL",
	} {
		if _, err := conn.ExecContext(ctx, stmt, nil); err != nil {
			return fmt.Errorf("set %s: %w", stmt, err)
		}
	}
	for _, check := range []struct{ query, want string }{
		{"PRAGMA busy_timeout", "5000"},
		{"PRAGMA foreign_keys", "1"},
		{"PRAGMA journal_mode", "wal"},
		{"PRAGMA synchronous", "2"},
	} {
		got, err := sqliteScalar(ctx, conn, check.query)
		if err != nil {
			return fmt.Errorf("verify %s: %w", check.query, err)
		}
		if !strings.EqualFold(got, check.want) {
			return fmt.Errorf("verify %s: got %q, want %q", check.query, got, check.want)
		}
	}
	slog.Info("SQLite connection verified", "sqlite_version", version, "sqlite_source_id", source)
	return nil
}

func sqliteScalar(ctx context.Context, conn sqlite.ExecQuerierContext, query string) (string, error) {
	rows, err := conn.QueryContext(ctx, query, nil)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	values := make([]driver.Value, len(rows.Columns()))
	if len(values) != 1 {
		return "", fmt.Errorf("expected one column, got %d", len(values))
	}
	if err := rows.Next(values); err != nil {
		if err == io.EOF {
			return "", fmt.Errorf("no result")
		}
		return "", err
	}
	return fmt.Sprint(values[0]), nil
}

// Older patched branches need independent verification and are not accepted by
// the default policy, even when their patch release is known upstream.
func sqliteVersionSupported(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return false
		}
		nums[i] = n
	}
	minimum := [3]int{3, 51, 3}
	for i := range nums {
		if nums[i] != minimum[i] {
			return nums[i] > minimum[i]
		}
	}
	return true
}
