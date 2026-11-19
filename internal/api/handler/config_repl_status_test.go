package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const replStateSchema = `
CREATE TABLE config_repl_state (
	source_node TEXT PRIMARY KEY, applied_version INTEGER NOT NULL DEFAULT 0,
	applied_rows INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
	synced_at DATETIME, expected_version INTEGER NOT NULL DEFAULT 0);`

func replTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(replStateSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func statusFrom(t *testing.T, db *sql.DB, info ConfigReplicationInfo) configReplicationStatus {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/config-replication", nil)
	rec := httptest.NewRecorder()
	ConfigReplicationStatusHandler(db, info)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var env struct {
		Data configReplicationStatus `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func TestConfigReplicationStatusAuthorReportsExpectedVersion(t *testing.T) {
	db := replTestDB(t)
	if _, err := db.Exec(schemaScopesForStatusTest); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dhcp_scopes
		(id,name,subnet,start_ip,end_ip,lease_time,enabled)
		VALUES ('s-1','lan','10.0.0.0/24','10.0.0.10','10.0.0.20',3600,1)`); err != nil {
		t.Fatal(err)
	}
	got := statusFrom(t, db, ConfigReplicationInfo{IsAuthor: true, NodeID: "node-a"})
	if got.Role != "author" || !got.Healthy {
		t.Fatalf("author status = %+v", got)
	}
	if got.ExpectedVersion == 0 {
		t.Fatal("author did not report an expected config version")
	}
}

const schemaScopesForStatusTest = `CREATE TABLE dhcp_scopes (
	id TEXT PRIMARY KEY, name TEXT, interface TEXT, subnet TEXT, start_ip TEXT,
	end_ip TEXT, subnet_mask TEXT, router TEXT, dns_servers TEXT, ntp_servers TEXT,
	domain_name TEXT, lease_time INTEGER, max_lease_time INTEGER, enabled INTEGER,
	ping_check_enabled INTEGER, dns_updates INTEGER, comment TEXT,
	created_at DATETIME, updated_at DATETIME)`

func TestConfigReplicationStatusAuthorUnhealthyWhenVersionUnreadable(t *testing.T) {
	db := replTestDB(t)
	db.Close()
	got := statusFrom(t, db, ConfigReplicationInfo{IsAuthor: true, NodeID: "node-a"})
	if got.Healthy {
		t.Fatalf("author with unreadable version must be unhealthy: %+v", got)
	}
	if !strings.Contains(got.LastError, "期望版本不可读") {
		t.Fatalf("last_error = %q", got.LastError)
	}
}

func TestConfigReplicationStatusAuthorAndDisabled(t *testing.T) {
	db := replTestDB(t)
	if got := statusFrom(t, db, ConfigReplicationInfo{IsAuthor: true}); got.Role != "author" || !got.Healthy {
		t.Fatalf("author status = %+v", got)
	}
	if got := statusFrom(t, db, ConfigReplicationInfo{}); got.Role != "disabled" || !got.Healthy {
		t.Fatalf("disabled status = %+v", got)
	}
}

func TestConfigReplicationStatusFollowerErrorIsUnhealthy(t *testing.T) {
	db := replTestDB(t)
	src := "http://node-a:6080"
	if _, err := db.Exec(`INSERT INTO config_repl_state
		(source_node, applied_version, applied_rows, last_error, synced_at)
		VALUES (?, 7, 0, ?, datetime('now'))`, src, "configrepl: inserting into ipam_subnets: NOT NULL"); err != nil {
		t.Fatal(err)
	}
	got := statusFrom(t, db, ConfigReplicationInfo{Enabled: true, Source: src, IntervalSeconds: 3})
	if got.Role != "follower" || got.Healthy {
		t.Fatalf("failed follower must be unhealthy: %+v", got)
	}
	if got.LastError != "configrepl: inserting into ipam_subnets: NOT NULL" {
		t.Fatalf("last_error = %q", got.LastError)
	}
	if got.AppliedVersion != 7 {
		t.Fatalf("applied_version = %d", got.AppliedVersion)
	}
}

func TestConfigReplicationStatusFollowerFreshIsHealthy(t *testing.T) {
	db := replTestDB(t)
	src := "http://node-a:6080"
	if _, err := db.Exec(`INSERT INTO config_repl_state
		(source_node, applied_version, applied_rows, last_error, synced_at)
		VALUES (?, 9, 12, '', datetime('now'))`, src); err != nil {
		t.Fatal(err)
	}
	got := statusFrom(t, db, ConfigReplicationInfo{Enabled: true, Source: src, IntervalSeconds: 3})
	if !got.Healthy || got.AppliedRows != 12 {
		t.Fatalf("fresh follower must be healthy: %+v", got)
	}
}

func TestConfigReplicationStatusStaleIsUnhealthy(t *testing.T) {
	db := replTestDB(t)
	src := "http://node-a:6080"
	old := time.Now().UTC().Add(-10 * time.Minute).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO config_repl_state
		(source_node, applied_version, applied_rows, last_error, synced_at)
		VALUES (?, 9, 12, '', ?)`, src, old); err != nil {
		t.Fatal(err)
	}
	got := statusFrom(t, db, ConfigReplicationInfo{Enabled: true, Source: src, IntervalSeconds: 3})
	if got.Healthy {
		t.Fatalf("stale follower must be unhealthy: %+v", got)
	}
}

func TestConfigReplicationStatusFutureTimestampIsUnhealthy(t *testing.T) {
	db := replTestDB(t)
	src := "http://node-a:6080"
	future := time.Now().UTC().Add(10 * time.Minute).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO config_repl_state
		(source_node, applied_version, applied_rows, last_error, synced_at)
		VALUES (?, 9, 12, '', ?)`, src, future); err != nil {
		t.Fatal(err)
	}
	got := statusFrom(t, db, ConfigReplicationInfo{Enabled: true, Source: src, IntervalSeconds: 3})
	if got.Healthy {
		t.Fatalf("future timestamp must be fail-closed: %+v", got)
	}
	if !strings.Contains(got.LastError, "超前") {
		t.Fatalf("last_error = %q", got.LastError)
	}
}

func TestConfigReplicationStatusFollowerBehindIsVisible(t *testing.T) {
	db := replTestDB(t)
	src := "http://node-a:6080"
	if _, err := db.Exec(`INSERT INTO config_repl_state
		(source_node, applied_version, applied_rows, last_error, synced_at, expected_version)
		VALUES (?, 7, 12, '', datetime('now'), 9)`, src); err != nil {
		t.Fatal(err)
	}
	got := statusFrom(t, db, ConfigReplicationInfo{Enabled: true, Source: src, IntervalSeconds: 3})
	if !got.Healthy {
		t.Fatalf("fresh but behind follower should still be healthy: %+v", got)
	}
	if got.AppliedVersion != 7 || got.ExpectedVersion != 9 {
		t.Fatalf("versions = applied %d / expected %d, want 7/9", got.AppliedVersion, got.ExpectedVersion)
	}
}

func TestConfigReplicationStatusNoStateIsUnhealthy(t *testing.T) {
	db := replTestDB(t)
	got := statusFrom(t, db, ConfigReplicationInfo{Enabled: true, Source: "http://node-a:6080", IntervalSeconds: 3})
	if got.Healthy || got.LastError == "" {
		t.Fatalf("unknown replica must be fail-closed: %+v", got)
	}
}

// NTP corrections and VM restore can move either clock. The status view must
// not treat a moderately future timestamp as proof of freshness forever, nor
// a timestamp left behind by a forward jump as fresh. These are local boundary
// regressions; they do not replace real clock injection in T-TIME-01.
func TestConfigReplicationStatusClockSkewBoundaries(t *testing.T) {
	cases := []struct {
		name        string
		delta       time.Duration
		wantHealthy bool
		errContains string
	}{
		{"backward correction within tolerance", 20 * time.Second, true, ""},
		{"backward correction beyond tolerance", replClockSkewTolerance + time.Second, false, "超前"},
		{"forward correction leaves timestamp stale", -(30*time.Second + time.Second), false, "过期"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := replTestDB(t)
			src := "http://node-a:6080"
			at := time.Now().UTC().Add(tc.delta).Format("2006-01-02 15:04:05")
			if _, err := db.Exec("INSERT INTO config_repl_state (source_node, applied_version, applied_rows, last_error, synced_at) VALUES (?, 9, 12, '', ?)", src, at); err != nil {
				t.Fatal(err)
			}
			got := statusFrom(t, db, ConfigReplicationInfo{Enabled: true, Source: src, IntervalSeconds: 3})
			if got.Healthy != tc.wantHealthy {
				t.Fatalf("healthy = %v, want %v; status = %+v", got.Healthy, tc.wantHealthy, got)
			}
			if tc.errContains != "" && !strings.Contains(got.LastError, tc.errContains) {
				t.Fatalf("last_error = %q, want %q", got.LastError, tc.errContains)
			}
		})
	}
}
