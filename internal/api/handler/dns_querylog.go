package handler

import (
	"context"
	"encoding/csv"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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

	ctx, cancel := queryLogRequestContext(r)
	defer cancel()
	entries, total, err := dnsquerylog.QueryLogsContext(ctx, DNSServices.DB, filters, page, pageSize)
	if err != nil {
		writeQueryLogQueryError(w, err)
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
	ctx, cancel := queryLogRequestContext(r)
	defer cancel()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="dns-query-logs.csv"`)

	// csv.Writer writes each database row immediately. The stream intentionally
	// avoids the paginated list's COUNT(*) and result slice allocation.
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"timestamp", "client_ip", "client_port", "protocol",
		"query_name", "query_type", "response_code",
		"response_time_ms", "upstream", "cached", "blocked",
	}); err != nil {
		return
	}
	err := dnsquerylog.StreamQueryLogsContext(ctx, DNSServices.DB, filters, maxExportRows, func(e dnsquerylog.QueryLogEntry) error {
		if err := cw.Write([]string{
			e.CreatedAt, e.ClientIP, strconv.Itoa(e.ClientPort), e.Protocol,
			e.QueryName, e.QueryType, e.ResponseCode,
			strconv.FormatFloat(e.ResponseTimeMs, 'f', -1, 64),
			e.Upstream,
			strconv.FormatBool(e.Cached),
			strconv.FormatBool(e.Blocked),
		}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	})
	if err != nil {
		// Headers and some CSV rows may already be written; do not append a JSON
		// envelope to a download. The client can retry; context cancellation is
		// expected when a browser aborts a large download.
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Error("dns query-log export failed", "error", err)
		}
		return
	}
	cw.Flush()
}

const queryLogQueryTimeout = 10 * time.Second

func queryLogRequestContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), queryLogQueryTimeout)
}

func writeQueryLogQueryError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		response.ServiceUnavailable(w, "日志查询繁忙或已超时，请缩小时间范围后重试", nil)
		return
	}
	response.InternalErrorWithLog(w, "failed to query logs", err)
}
