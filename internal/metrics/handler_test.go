package metrics

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"testing"
	"time"
)

func TestHourlyStatsNormalizeTimestamps(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE dns_query_logs(created_at TEXT, cached INTEGER, blocked INTEGER)`); err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Add(-time.Hour).Truncate(time.Hour)
	for _, stamp := range []string{hour.Add(time.Minute).Format(time.RFC3339), hour.Add(2 * time.Minute).Format("2006-01-02 15:04:05"), hour.Add(3 * time.Minute).In(time.FixedZone("test", 8*3600)).Format(time.RFC3339)} {
		if _, err = db.Exec(`INSERT INTO dns_query_logs VALUES (?,1,0)`, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO dns_query_logs VALUES (?,0,1)`, hour.Add(-48*time.Hour).Format(time.RFC3339)); err != nil {
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
