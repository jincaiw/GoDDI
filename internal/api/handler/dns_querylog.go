package handler

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/jasonwa/goddi/internal/api/response"
	dnsquerylog "github.com/jasonwa/goddi/internal/dns"
)

// ListDNSQueryLogs lists DNS query logs with filtering and pagination.
// GET /api/v1/logs/dns
func ListDNSQueryLogs(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS services not initialized")
		return
	}

	// Parse pagination.
	page, pageSize := response.ParsePagination(r)

	// Parse filters.
	filters := dnsquerylog.QueryLogFilters{
		ClientIP:     r.URL.Query().Get("client_ip"),
		Domain:       r.URL.Query().Get("domain"),
		QueryType:    r.URL.Query().Get("query_type"),
		ResponseCode: r.URL.Query().Get("response_code"),
		StartTime:    r.URL.Query().Get("start_time"),
		EndTime:      r.URL.Query().Get("end_time"),
	}

	// Parse blocked filter.
	if blockedStr := r.URL.Query().Get("blocked"); blockedStr != "" {
		blocked := blockedStr == "true"
		filters.Blocked = &blocked
	}

	entries, total, err := dnsquerylog.QueryLogs(DNSServices.DB, filters, page, pageSize)
	if err != nil {
		response.InternalError(w, "failed to query logs: "+err.Error())
		return
	}

	response.OKPaginated(w, entries, total, page, pageSize)
}

// ExportDNSQueryLogs streams the filtered DNS query logs as a CSV download.
// GET /api/v1/logs/dns/export
//
// Technitium v13.4 parity. The export shares the list filters but ignores
// pagination; a hard cap bounds the result so a runaway export cannot
// exhaust memory (the browser download keeps streaming rows as they are
// encoded).
func ExportDNSQueryLogs(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS services not initialized")
		return
	}

	// Reuse the list filters; pagination is intentionally ignored.
	filters := dnsquerylog.QueryLogFilters{
		ClientIP:     r.URL.Query().Get("client_ip"),
		Domain:       r.URL.Query().Get("domain"),
		QueryType:    r.URL.Query().Get("query_type"),
		ResponseCode: r.URL.Query().Get("response_code"),
		StartTime:    r.URL.Query().Get("start_time"),
		EndTime:      r.URL.Query().Get("end_time"),
	}
	if blockedStr := r.URL.Query().Get("blocked"); blockedStr != "" {
		blocked := blockedStr == "true"
		filters.Blocked = &blocked
	}

	const maxExportRows = 100000
	entries, _, err := dnsquerylog.QueryLogs(DNSServices.DB, filters, 1, maxExportRows)
	if err != nil {
		response.InternalError(w, "failed to query logs: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="dns-query-logs.csv"`)

	// csv.Writer over the HTTP body streams each row without buffering the
	// whole export in memory.
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"timestamp", "client_ip", "client_port", "protocol",
		"query_name", "query_type", "response_code",
		"response_time_ms", "upstream", "cached", "blocked",
	})
	for _, e := range entries {
		_ = cw.Write([]string{
			e.CreatedAt, e.ClientIP, strconv.Itoa(e.ClientPort), e.Protocol,
			e.QueryName, e.QueryType, e.ResponseCode,
			strconv.FormatFloat(e.ResponseTimeMs, 'f', -1, 64),
			e.Upstream,
			strconv.FormatBool(e.Cached),
			strconv.FormatBool(e.Blocked),
		})
	}
	cw.Flush()
}
