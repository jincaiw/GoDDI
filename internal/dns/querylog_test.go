package dns

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestQueryLoggerStatsTracksWriterFailures(t *testing.T) {
	ql := NewQueryLogger(nil, 0)
	defer ql.Close()

	ql.flush([]QueryLogEntry{{ID: "one"}, {ID: "two"}})
	stats := ql.Stats()
	if stats.BeginFailures != 2 {
		t.Fatalf("BeginFailures = %d, want 2", stats.BeginFailures)
	}
	if stats.Written != 0 {
		t.Fatalf("Written = %d, want 0", stats.Written)
	}
	if stats.QueueCapacity != 10000 {
		t.Fatalf("QueueCapacity = %d, want 10000", stats.QueueCapacity)
	}
}

func TestQueryLoggerStatsTracksQueueDrops(t *testing.T) {
	ql := &QueryLogger{entries: make(chan QueryLogEntry, 1)}
	ql.Log(QueryLogEntry{ID: "one"})
	ql.Log(QueryLogEntry{ID: "two"})

	stats := ql.Stats()
	if stats.QueueDepth != 1 {
		t.Fatalf("QueueDepth = %d, want 1", stats.QueueDepth)
	}
	if stats.DroppedFull != 1 {
		t.Fatalf("DroppedFull = %d, want 1", stats.DroppedFull)
	}
}

func TestQueryLoggerCleanupDeletesExpiredRowsInBatches(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE dns_query_logs (id TEXT, created_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for i := range queryLogCleanupBatchSize + 5 {
		if _, err := db.Exec(`INSERT INTO dns_query_logs (id, created_at) VALUES (?, ?)`, i, "2000-01-01 00:00:00"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO dns_query_logs (id, created_at) VALUES (?, ?)`, "fresh", "2999-01-01 00:00:00"); err != nil {
		t.Fatal(err)
	}

	ql := &QueryLogger{db: db, retentionDays: 1}
	ql.cleanup()

	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_query_logs`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("remaining rows = %d, want 1 fresh row", remaining)
	}
	stats := ql.Stats()
	if stats.CleanupRuns != 1 {
		t.Fatalf("CleanupRuns = %d, want 1", stats.CleanupRuns)
	}
	if stats.CleanupDeleted != int64(queryLogCleanupBatchSize+5) {
		t.Fatalf("CleanupDeleted = %d, want %d", stats.CleanupDeleted, queryLogCleanupBatchSize+5)
	}
	if stats.CleanupFailures != 0 || stats.CleanupTimeouts != 0 {
		t.Fatalf("unexpected cleanup failures: %#v", stats)
	}
}

func TestStreamQueryLogsContextDoesNotNeedPaginationCount(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE dns_query_logs (
		id TEXT, client_ip TEXT, client_port INTEGER, protocol TEXT, query_name TEXT,
		query_type TEXT, response_code TEXT, response_time_ms REAL, upstream TEXT,
		cached BOOLEAN, blocked BOOLEAN, created_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dns_query_logs VALUES
		('1','192.0.2.1',53,'udp','one.example.','A','NOERROR',1.2,'8.8.8.8:53',0,0,'2026-01-01 00:00:00'),
		('2','192.0.2.2',53,'udp','two.example.','A','NOERROR',2.3,'1.1.1.1:53',1,0,'2026-01-02 00:00:00')`); err != nil {
		t.Fatal(err)
	}

	var got []string
	err = StreamQueryLogsContext(context.Background(), db, QueryLogFilters{}, 100, func(entry QueryLogEntry) error {
		got = append(got, entry.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamQueryLogsContext: %v", err)
	}
	if len(got) != 2 || got[0] != "2" || got[1] != "1" {
		t.Fatalf("streamed IDs = %v, want descending timestamps", got)
	}
}

func TestQueryLogPageContextReturnsLookaheadWithoutCount(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE dns_query_logs (
		id TEXT, client_ip TEXT, client_port INTEGER, protocol TEXT, query_name TEXT,
		query_type TEXT, response_code TEXT, response_time_ms REAL, upstream TEXT,
		cached BOOLEAN, blocked BOOLEAN, created_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err := db.Exec(`INSERT INTO dns_query_logs VALUES (?, '192.0.2.1', 53, 'udp', ?, 'A', 'NOERROR', 1, '', 0, 0, ?)`,
			i, "entry.example.", "2026-01-01 00:00:0"+string(rune('1'+i))); err != nil {
			t.Fatal(err)
		}
	}

	entries, hasMore, err := QueryLogPageContext(context.Background(), db, QueryLogFilters{}, 1, 2)
	if err != nil {
		t.Fatalf("QueryLogPageContext: %v", err)
	}
	if len(entries) != 2 || !hasMore {
		t.Fatalf("page = %d entries, hasMore=%t; want 2 entries and true", len(entries), hasMore)
	}

	entries, hasMore, err = QueryLogPageContext(context.Background(), db, QueryLogFilters{}, 2, 2)
	if err != nil {
		t.Fatalf("QueryLogPageContext second page: %v", err)
	}
	if len(entries) != 1 || hasMore {
		t.Fatalf("second page = %d entries, hasMore=%t; want 1 entry and false", len(entries), hasMore)
	}
}

func TestQueryLogsContextHonorsCanceledContext(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := QueryLogsContext(ctx, db, QueryLogFilters{}, 1, 10); err == nil {
		t.Fatal("expected canceled context error")
	}
}
