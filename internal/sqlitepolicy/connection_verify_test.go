package sqlitepolicy

import (
	"context"
	"database/sql/driver"
	"io"
	"testing"
)

// A connection that accepts every PRAGMA but reports the wrong readback must
// be rejected. The policy verifies values rather than trusting ExecContext.
type badReadbackConn struct {
	query    string
	badValue string
	writes   int
}

func (c *badReadbackConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.writes++
	return driver.RowsAffected(0), nil
}

func (c *badReadbackConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch query {
	case "SELECT sqlite_version()":
		return &policyRows{value: "3.53.2"}, nil
	case "SELECT sqlite_source_id()":
		return &policyRows{value: "source-id"}, nil
	}
	want := map[string]string{
		"PRAGMA busy_timeout": "5000",
		"PRAGMA foreign_keys": "1",
		"PRAGMA journal_mode": "wal",
		"PRAGMA synchronous":  "2",
	}[query]
	if want == "" {
		return nil, io.ErrUnexpectedEOF
	}
	if query == c.query {
		want = c.badValue
	}
	return &policyRows{value: want}, nil
}

func TestConnectionPolicyRejectsBadPragmaReadback(t *testing.T) {
	for _, query := range []string{"PRAGMA busy_timeout", "PRAGMA foreign_keys", "PRAGMA journal_mode", "PRAGMA synchronous"} {
		c := &badReadbackConn{query: query, badValue: "wrong"}
		if err := configureSQLiteConnection(c, ""); err == nil {
			t.Fatalf("accepted bad readback for %s", query)
		}
		if c.writes == 0 {
			t.Fatalf("%s was rejected before any pragma was attempted", query)
		}
	}
}
