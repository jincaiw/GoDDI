package handler

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newStatsTestDB creates the dns_query_logs table with the columns and the
// created_at index that GetStats relies on.
func newStatsTestDB(t *testing.T) *sql.DB {
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

func insertQueryLog(t *testing.T, db *sql.DB, code string, blocked, cached bool, createdAt time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO dns_query_logs
		(client_ip, query_name, query_type, response_code, cached, blocked, created_at)
		VALUES ('192.0.2.1', 'a.test.', 'A', ?, ?, ?, ?)`,
		code, cached, blocked, createdAt.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatal(err)
	}
}

// TestGetStats_RangeAndIndex verifies that /stats returns correct aggregates
// for the requested window and that the range predicate uses
// idx_dns_query_logs_created_at (raw comparison, no datetime(created_at)
// column wrapper). The column wrapper previously forced a full table scan
// that took minutes on production-sized query-log tables.
func TestGetStats_RangeAndIndex(t *testing.T) {
	db := newStatsTestDB(t)
	now := time.Now().UTC()

	insertQueryLog(t, db, "NOERROR", false, true, now.Add(-30*time.Minute))
	insertQueryLog(t, db, "NXDOMAIN", false, false, now.Add(-2*time.Hour))
	insertQueryLog(t, db, "SERVFAIL", false, false, now.Add(-3*time.Hour))
	// Out of the 24h window.
	insertQueryLog(t, db, "NOERROR", false, false, now.Add(-48*time.Hour))

	saved := SystemServices
	SystemServices = &SystemServiceContainer{DB: db}
	t.Cleanup(func() { SystemServices = saved })

	req := httptest.NewRequest("GET", "/api/v1/stats?range=day", nil)
	rec := httptest.NewRecorder()
	GetStats(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Range   string `json:"range"`
			Start   string `json:"start"`
			End     string `json:"end"`
			Summary struct {
				Total    int64   `json:"total"`
				NoError  int64   `json:"noerror"`
				NXDomain int64   `json:"nxdomain"`
				ServFail int64   `json:"servfail"`
				Cached   int64   `json:"cached"`
				Clients  int64   `json:"clients"`
			} `json:"summary"`
			Series []struct {
				Bucket string `json:"bucket"`
				Total  int64  `json:"total"`
			} `json:"series"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	s := body.Data.Summary
	if s.Total != 3 || s.NoError != 1 || s.NXDomain != 1 || s.ServFail != 1 || s.Cached != 1 || s.Clients != 1 {
		t.Fatalf("unexpected summary: %+v", s)
	}
	if body.Data.Range != "day" {
		t.Fatalf("unexpected range: %s", body.Data.Range)
	}
	// Start/End remain RFC3339 in the API response.
	if !strings.HasSuffix(body.Data.Start, "Z") {
		t.Fatalf("expected RFC3339 start, got: %s", body.Data.Start)
	}

	// The series query must be index-driven too.
	plan, err := statsQueryPlan(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "idx_dns_query_logs_created_at") {
		t.Fatalf("expected index usage in series query plan, got: %s", plan)
	}
}

func statsQueryPlan(db *sql.DB, now time.Time) (string, error) {
	startSQL := now.AddDate(0, 0, -1).UTC().Format("2006-01-02 15:04:05")
	endSQL := now.UTC().Format("2006-01-02 15:04:05")
	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT strftime('%Y-%m-%d %H:00', created_at) AS bucket, COUNT(*)
		FROM dns_query_logs
		WHERE created_at >= ? AND created_at <= ?
		GROUP BY bucket
		ORDER BY bucket ASC`, startSQL, endSQL)
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
