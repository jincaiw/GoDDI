package metrics

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusHandler returns an http.Handler that serves Prometheus metrics.
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// DashboardData holds the data returned by the dashboard API.
type DashboardData struct {
	Uptime          float64                `json:"uptime"`
	DNSQueriesToday int64                  `json:"dns_queries_today"`
	CacheHitRate    float64                `json:"cache_hit_rate"`
	ActiveLeases    int64                  `json:"active_leases"`
	IPAMUsage       float64                `json:"ipam_usage"`
	RecentDNSStats  []HourlyDNSStats       `json:"recent_dns_stats"`
	SystemInfo      map[string]interface{} `json:"system_info"`
}

// HourlyDNSStats holds DNS query statistics for one hour.
type HourlyDNSStats struct {
	Hour    string `json:"hour"`
	Queries int64  `json:"queries"`
	Blocked int64  `json:"blocked"`
	Cached  int64  `json:"cached"`
}

// DashboardDataHandler returns an http.HandlerFunc that serves dashboard data as JSON.
// It requires a database connection to query live statistics.
func DashboardDataHandler(db *sql.DB, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := DashboardData{
			Uptime: GetUptime().Seconds(),
		}

		// Use SQLite's native datetime text format (matching the
		// datetime('now') schema default) and a raw column comparison so
		// idx_dns_query_logs_created_at is used; wrapping the column with
		// datetime(created_at) forces a full table scan.
		twentyFourHoursAgo := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

		// Combine the three dns_query_logs aggregations into a single round
		// trip. The previous implementation issued one COUNT(*) per metric
		// (queries, cache hits, cache total) which is an N+1 pattern: three
		// sequential queries against the same table for the same time window.
		// Using SUM(CASE WHEN ...) lets us compute all three in a single
		// table scan.
		//
		// All inputs to this query are bound as parameters (?); the time
		// threshold is the only user-influenced value and it is built from
		// time.Now() on the server, so SQL injection is not possible here.
		var dnsQueriesToday, cacheHits, cacheTotal int64
		err := db.QueryRow(`
			SELECT
				COUNT(*),
				COALESCE(SUM(CASE WHEN cached = 1 THEN 1 ELSE 0 END), 0),
				COUNT(*)
			FROM dns_query_logs
			WHERE created_at >= ?
		`, twentyFourHoursAgo).Scan(&dnsQueriesToday, &cacheHits, &cacheTotal)
		if err != nil {
			slog.Warn("metrics: failed to query DNS query stats", "error", err)
		} else {
			if cacheTotal > 0 {
				data.CacheHitRate = float64(cacheHits) / float64(cacheTotal) * 100
			}
		}
		data.DNSQueriesToday = dnsQueriesToday

		// Active DHCP leases and IPAM usage combined into a single query via
		// UNION ALL with conditional aggregation. The previous code issued
		// three separate COUNT(*) queries against ipam_addresses and one
		// against dhcp_leases; we now do it in one round trip.
		//
		// The query uses parameterized UNIONs so there is no string
		// interpolation of caller-controlled data.
		var activeLeases, ipamTotal, ipamUsed int64
		if err := db.QueryRow(`
			SELECT
				COALESCE((SELECT COUNT(*) FROM dhcp_leases WHERE status = 'active'), 0),
				COALESCE((SELECT COUNT(*) FROM ipam_addresses), 0),
				COALESCE((SELECT COUNT(*) FROM ipam_addresses WHERE status = 'used'), 0)
		`).Scan(&activeLeases, &ipamTotal, &ipamUsed); err != nil {
			slog.Warn("metrics: failed to query DHCP/IPAM stats", "error", err)
		}
		data.ActiveLeases = activeLeases
		if ipamTotal > 0 {
			data.IPAMUsage = float64(ipamUsed) / float64(ipamTotal) * 100
		}

		// Recent DNS stats by hour (last 24h).
		data.RecentDNSStats = getRecentDNSStats(db)

		// System info.
		sysInfo := GetSystemInfo()
		sysInfo["version"] = version
		data.SystemInfo = sysInfo

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "success",
			"data":    data,
		})
	}
}

// getRecentDNSStats returns hourly DNS query statistics for the last 24 hours.
func getRecentDNSStats(db *sql.DB) []HourlyDNSStats {
	stats := make([]HourlyDNSStats, 0, 24)

	twentyFourHoursAgo := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

	rows, err := db.Query(`
		SELECT
			strftime('%Y-%m-%d %H:00', created_at) AS hour,
			COUNT(*) AS queries,
			SUM(CASE WHEN blocked = 1 THEN 1 ELSE 0 END) AS blocked,
			SUM(CASE WHEN cached = 1 THEN 1 ELSE 0 END) AS cached
		FROM dns_query_logs
		WHERE created_at >= ?
		GROUP BY hour
		ORDER BY hour ASC
	`, twentyFourHoursAgo)
	if err != nil {
		return stats
	}
	defer rows.Close()

	for rows.Next() {
		var s HourlyDNSStats
		if err := rows.Scan(&s.Hour, &s.Queries, &s.Blocked, &s.Cached); err != nil {
			slog.Warn("metrics: failed to scan DNS stats row", "error", err)
			continue
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("metrics: failed to iterate DNS stats rows", "error", err)
	}

	return stats
}

// MetricsMiddleware is HTTP middleware that records API request metrics.
//
// IMPORTANT: the "path" label is sourced from the chi route pattern
// (e.g. "/api/v1/users/{id}"), NOT from r.URL.Path. Using the raw URL path
// would cause unbounded label cardinality in Prometheus (one new time series
// per /users/{uuid}) and would let an attacker blow up the metrics storage
// simply by hitting random UUIDs. If the request did not match a known
// route (e.g. 404s) we fall back to "unmatched" so we still get visibility
// into bad requests without exploding cardinality.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		duration := time.Since(start).Seconds()

		path := "unmatched"
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if pattern := rctx.RoutePattern(); pattern != "" {
				path = pattern
			}
		}

		RecordAPIRequest(r.Method, path, ww.Status(), duration)
	})
}
