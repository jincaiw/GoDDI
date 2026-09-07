package handler

import (
	"net/http"
	"time"

	"github.com/jasonwa/goddi/internal/api/response"
)

// --- Long-term Statistics API (Technitium v13.4 parity) ---
//
// Aggregations run directly against the persistent dns_query_logs table,
// which is the authoritative long-term record (the in-memory TopStats is
// a hot-path convenience only and is lost on restart). All bucketing is
// done in SQLite via strftime so the API stays a single query per call.

// StatsSummary aggregates a time range into headline counters.
type StatsSummary struct {
	Total         int64   `json:"total"`
	NoError       int64   `json:"noerror"`
	NXDomain      int64   `json:"nxdomain"`
	ServFail      int64   `json:"servfail"`
	Refused       int64   `json:"refused"`
	Blocked       int64   `json:"blocked"`
	Cached        int64   `json:"cached"`
	Clients       int64   `json:"clients"`
	AvgResponseMs float64 `json:"avg_response_ms"`
}

// StatsBucket is one time slice of the series.
type StatsBucket struct {
	Bucket  string `json:"bucket"`
	Total   int64  `json:"total"`
	Blocked int64  `json:"blocked"`
	Cached  int64  `json:"cached"`
}

// StatsResult is the /stats response payload.
type StatsResult struct {
	Range   string        `json:"range"`
	Start   string        `json:"start"`
	End     string        `json:"end"`
	Summary StatsSummary  `json:"summary"`
	Series  []StatsBucket `json:"series"`
}

// GetStats handles GET /api/v1/stats?range=hour|day|week|custom&start=&end=
func GetStats(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.DB == nil {
		response.InternalError(w, "系统服务未初始化")
		return
	}
	db := SystemServices.DB

	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "day"
	}

	// Resolve window and bucket granularity. Custom ranges pick the
	// granularity from the span: <=2h -> minute, <=48h -> hour, else day.
	var start time.Time
	var bucketFmt string
	now := time.Now()
	switch rangeName {
	case "hour":
		start = now.Add(-time.Hour)
		bucketFmt = "%Y-%m-%d %H:%M"
	case "week":
		start = now.AddDate(0, 0, -7)
		bucketFmt = "%Y-%m-%d %H:00"
	case "custom":
		var err error
		start, err = time.Parse(time.RFC3339, r.URL.Query().Get("start"))
		if err != nil {
			response.BadRequest(w, "start 必须是 RFC3339 时间")
			return
		}
		end, err := time.Parse(time.RFC3339, r.URL.Query().Get("end"))
		if err != nil {
			response.BadRequest(w, "end 必须是 RFC3339 时间")
			return
		}
		if !end.After(start) {
			response.BadRequest(w, "end 必须晚于 start")
			return
		}
		// Cap custom ranges at 90 days to bound query cost.
		if end.Sub(start) > 90*24*time.Hour {
			response.BadRequest(w, "自定义范围最长 90 天")
			return
		}
		now = end
		switch span := end.Sub(start); {
		case span <= 2*time.Hour:
			bucketFmt = "%Y-%m-%d %H:%M"
		case span <= 48*time.Hour:
			bucketFmt = "%Y-%m-%d %H:00"
		default:
			bucketFmt = "%Y-%m-%d"
		}
		rangeName = "custom"
	default: // "day"
		start = now.AddDate(0, 0, -1)
		bucketFmt = "%Y-%m-%d %H:00"
	}

	startStr := start.UTC().Format(time.RFC3339)
	endStr := now.UTC().Format(time.RFC3339)

	var s StatsSummary
	err := db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN response_code = 'NOERROR' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN response_code = 'NXDOMAIN' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN response_code = 'SERVFAIL' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN response_code = 'REFUSED' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN blocked = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN cached = 1 THEN 1 ELSE 0 END), 0),
			COUNT(DISTINCT client_ip),
			COALESCE(AVG(response_time_ms), 0)
		FROM dns_query_logs
		WHERE datetime(created_at) >= datetime(?) AND datetime(created_at) <= datetime(?)
	`, startStr, endStr).Scan(
		&s.Total, &s.NoError, &s.NXDomain, &s.ServFail, &s.Refused,
		&s.Blocked, &s.Cached, &s.Clients, &s.AvgResponseMs,
	)
	if err != nil {
		response.InternalErrorWithLog(w, "统计查询失败", err)
		return
	}

	series := make([]StatsBucket, 0, 64)
	rows, err := db.Query(`
		SELECT
			strftime(?, created_at) AS bucket,
			COUNT(*),
			SUM(CASE WHEN blocked = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN cached = 1 THEN 1 ELSE 0 END)
		FROM dns_query_logs
		WHERE datetime(created_at) >= datetime(?) AND datetime(created_at) <= datetime(?)
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketFmt, startStr, endStr)
	if err != nil {
		response.InternalErrorWithLog(w, "统计序列查询失败", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var b StatsBucket
		var blocked, cached interface{}
		if err := rows.Scan(&b.Bucket, &b.Total, &blocked, &cached); err != nil {
			continue
		}
		if v, ok := blocked.(int64); ok {
			b.Blocked = v
		}
		if v, ok := cached.(int64); ok {
			b.Cached = v
		}
		series = append(series, b)
	}

	result := StatsResult{
		Range:   rangeName,
		Start:   startStr,
		End:     endStr,
		Summary: s,
		Series:  series,
	}

	response.OK(w, result)
}
