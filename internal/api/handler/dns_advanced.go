package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/cache"
	"github.com/jasonwa/goddi/internal/metrics"
)

// --- Temporary blocking disable (T4) ---

// TemporaryDisableBlocking handles POST /api/v1/dns/security/temporary-disable
// Body: {"minutes": 15}. Range: 1-1440 minutes. minutes=0 re-enables.
func TemporaryDisableBlocking(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		Minutes int `json:"minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.Minutes < 0 || req.Minutes > 1440 {
		response.BadRequest(w, "minutes 必须在 0-1440 之间（0 表示立即恢复阻断）")
		return
	}

	DNSServices.Filter.TemporaryDisableBlocking(time.Duration(req.Minutes) * time.Minute)

	enabled, until := DNSServices.Filter.BlockingStatus()
	response.OK(w, map[string]interface{}{
		"blocking_enabled":     enabled,
		"disabled_until":       until,
		"disabled_for_minutes": req.Minutes,
	})
}

// GetBlockingStatus handles GET /api/v1/dns/security/blocking-status
func GetBlockingStatus(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	enabled, until := DNSServices.Filter.BlockingStatus()
	response.OK(w, map[string]interface{}{
		"blocking_enabled": enabled,
		"disabled_until":   until,
	})
}

// --- Block list URL refresh (T1) ---

// RefreshBlockList handles POST /api/v1/dns/security/blocklists/{id}/refresh
// It triggers an immediate fetch of an external block list URL.
func RefreshBlockList(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil || DNSServices.BlockListFetcher == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if err := DNSServices.BlockListFetcher.FetchList(r.Context(), id); err != nil {
		response.BadRequest(w, "刷新失败: "+err.Error())
		return
	}

	list, ok := DNSServices.Filter.BlockListMgr.GetList(id)
	if !ok {
		response.NotFound(w, "黑名单未找到")
		return
	}
	response.OK(w, list)
}

// --- Cache entry listing (T7) ---

// ListDNSCacheEntries handles GET /api/v1/dns/cache/entries
// Query params: qname, qtype, page, page_size.
func ListDNSCacheEntries(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Cache == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)
	entries, total := DNSServices.Cache.ListEntries(
		r.URL.Query().Get("qname"),
		r.URL.Query().Get("qtype"),
		pageSize,
		(page-1)*pageSize,
	)
	response.OKPaginated(w, entries, int64(total), page, pageSize)
}

// CacheEntryType is re-exported for OpenAPI documentation purposes.
type CacheEntryType = cache.EntryInfo

// --- Dashboard Top-N (T6) ---

// GetDashboardTop handles GET /api/v1/dashboard/top?range=hour|day|week
func GetDashboardTop(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.DB == nil {
		response.InternalError(w, "系统服务未初始化")
		return
	}

	rangeName := r.URL.Query().Get("range")
	switch rangeName {
	case "hour", "day", "week":
	default:
		rangeName = "hour"
	}
	limit := 10
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 50 {
		limit = l
	}

	response.OK(w, metrics.TopStatsGlobal.Top(rangeName, limit))
}
