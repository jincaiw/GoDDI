package metrics

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newQueryLogTestDB creates the dns_query_logs table exactly as the
// migration does for the columns these tests rely on, including the
// created_at index, so EXPLAIN QUERY PLAN assertions are meaningful.
func newQueryLogTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE dns_query_logs (
		id TEXT PRIMARY KEY,
		client_ip TEXT NOT NULL,
		query_name TEXT NOT NULL,
		query_type TEXT NOT NULL,
		response_code TEXT,
		response_time_ms REAL,
		cached BOOLEAN DEFAULT FALSE,
		blocked BOOLEAN DEFAULT FALSE,
		created_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE INDEX idx_dns_query_logs_created_at ON dns_query_logs(created_at)`); err != nil {
		t.Fatal(err)
	}
	return db
}

// Storage contract: created_at is always written by the schema default
// datetime('now'), i.e. "YYYY-MM-DD HH:MM:SS" in UTC. The range predicates
// rely on that format for raw (index-friendly) string comparison.
func TestHourlyStatsUsesSQLiteTimestampFormat(t *testing.T) {
	db := newQueryLogTestDB(t)
	hour := time.Now().UTC().Add(-time.Hour).Truncate(time.Hour)
	for _, stamp := range []string{
		hour.Add(time.Minute).Format("2006-01-02 15:04:05"),
		hour.Add(2 * time.Minute).Format("2006-01-02 15:04:05"),
		hour.Add(3 * time.Minute).Format("2006-01-02 15:04:05"),
	} {
		if _, err := db.Exec(`INSERT INTO dns_query_logs (client_ip, query_name, query_type, cached, blocked, created_at) VALUES ('127.0.0.1','a.test.','A',1,0,?)`, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO dns_query_logs (client_ip, query_name, query_type, cached, blocked, created_at) VALUES ('127.0.0.1','old.test.','A',0,1,?)`, hour.Add(-48*time.Hour).Format("2006-01-02 15:04:05")); err != nil {
		t.Fatal(err)
	}
	stats := getRecentDNSStats(db)
	if len(stats) != 1 || stats[0].Queries != 3 || stats[0].Cached != 3 || stats[0].Blocked != 0 {
		t.Fatalf("incorrect hourly totals: %+v", stats)
	}
	if stats[0].Hour != hour.Format("2006-01-02 15:00") {
		t.Fatalf("incorrect hour: %s", stats[0].Hour)
	}
}

// TestQueryLogRangeQueryUsesIndex guards against regressions that wrap the
// created_at column in datetime(...): such wrappers defeat
// idx_dns_query_logs_created_at and force full table scans that take minutes
// on large query-log tables.
func TestQueryLogRangeQueryUsesIndex(t *testing.T) {
	db := newQueryLogTestDB(t)
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	plan, err := queryPlan(db, `SELECT COUNT(*) FROM dns_query_logs WHERE created_at >= ?`, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "idx_dns_query_logs_created_at") {
		t.Fatalf("expected index usage in query plan, got: %s", plan)
	}
	if strings.Contains(plan, "SCAN dns_query_logs") && !strings.Contains(plan, "USING") {
		t.Fatalf("expected index-driven search, got full scan: %s", plan)
	}
}

func queryPlan(db *sql.DB, query string, args ...any) (string, error) {
	rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var id, parent int
		var notUsed, detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			return "", err
		}
		sb.WriteString(detail)
		sb.WriteString("\n")
	}
	return sb.String(), rows.Err()
}
