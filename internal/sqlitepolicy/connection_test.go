package sqlitepolicy

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/url"
	"path/filepath"
	"testing"
)

type policyRows struct {
	value string
	done  bool
}

func (r *policyRows) Columns() []string { return []string{"value"} }
func (r *policyRows) Close() error      { return nil }
func (r *policyRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.value
	return nil
}

type rejectingPolicyConn struct {
	version, source string
	writes          int
}

func (c *rejectingPolicyConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.writes++
	return driver.RowsAffected(0), nil
}
func (c *rejectingPolicyConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if query == "SELECT sqlite_version()" {
		return &policyRows{value: c.version}, nil
	}
	if query == "SELECT sqlite_source_id()" {
		return &policyRows{value: c.source}, nil
	}
	return nil, io.ErrUnexpectedEOF
}

func TestRejectedEngineNeverExecutesPragma(t *testing.T) {
	for _, tc := range []struct{ version, source string }{{"3.51.2", "source"}, {"broken", "source"}, {"3.53.2", " "}} {
		c := &rejectingPolicyConn{version: tc.version, source: tc.source}
		if err := configureSQLiteConnection(c, ""); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
		if c.writes != 0 {
			t.Fatalf("rejected %+v after %d writes", tc, c.writes)
		}
	}
}

func TestDSNPragmaRejectedBeforeDatabaseOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE marker (id INTEGER); INSERT INTO marker VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"_pragma=journal_mode(WAL)", "%5fpragma=journal_mode%28WAL%29", "cache=shared&_pragma=journal_mode%28WAL%29", "_pragma%ZZ=foo"} {
		writer := OpenWriter("file:" + path + "?" + query)
		if err := writer.Ping(); err == nil {
			writer.Close()
			t.Fatalf("accepted %q", query)
		}
		writer.Close()
		check, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		var mode string
		var value int
		if err := check.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if err := check.QueryRow("SELECT id FROM marker").Scan(&value); err != nil {
			t.Fatal(err)
		}
		check.Close()
		if mode != "delete" || value != 42 {
			t.Fatalf("query %q changed database: mode=%q value=%d", query, mode, value)
		}
	}
}

func TestDSNValidation(t *testing.T) {
	for _, dsn := range []string{"plain.db", "plain.db?_time_format=sqlite", "file:db?mode=rwc&cache=private", "file:db?_txlock=immediate", "file:db?x=" + url.QueryEscape("_pragma=journal_mode(WAL)")} {
		if err := validateDSN(dsn); err != nil {
			t.Errorf("valid DSN %q: %v", dsn, err)
		}
	}
	for _, dsn := range []string{"plain.db?_pragma=foreign_keys(0)", "file:db?%5Fpragma=journal_mode(WAL)", "file:db?_PRAGMA=x", "file:db?%76fs=custom", "file:db?immutable=1", "file:db?x=%ZZ", "file:db?x=1;y=2"} {
		if err := validateDSN(dsn); err == nil {
			t.Errorf("accepted %q", dsn)
		}
	}
}

func TestSQLiteVersionSupported(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"3.51.2", false}, {"3.51.3", true}, {"3.51.4", true}, {"3.52.0", true},
		{"4.0.0", true}, {"3.50.7", false}, {"3.44.6", false}, {"3.51", false}, {"3.51.3-rc1", false},
	} {
		if got := sqliteVersionSupported(tc.version); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.version, got, tc.want)
		}
	}
}
