package handler

import (
	"net/http"

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
