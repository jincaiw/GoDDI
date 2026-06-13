package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/miekg/dns"
)

// GetDNSCacheStats returns cache statistics.
// GET /api/v1/dns/cache
func GetDNSCacheStats(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Cache == nil {
		response.InternalError(w, "DNS缓存未初始化")
		return
	}

	stats := DNSServices.Cache.Stats()
	response.OK(w, stats)
}

// FlushDNSCache flushes all cache entries.
// DELETE /api/v1/dns/cache
func FlushDNSCache(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Cache == nil {
		response.InternalError(w, "DNS缓存未初始化")
		return
	}

	DNSServices.Cache.Flush()
	response.OK(w, map[string]string{"message": "缓存已清空"})
}

// FlushDNSCacheEntry flushes a specific cache entry.
// DELETE /api/v1/dns/cache/{name}/{type}
func FlushDNSCacheEntry(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Cache == nil {
		response.InternalError(w, "DNS缓存未初始化")
		return
	}

	name := chi.URLParam(r, "name")
	qtypeStr := chi.URLParam(r, "type")

	if name == "" || qtypeStr == "" {
		response.BadRequest(w, "缺少名称和类型")
		return
	}

	qtype, ok := dns.StringToType[qtypeStr]
	if !ok {
		response.BadRequest(w, "无效的DNS记录类型: "+qtypeStr)
		return
	}

	DNSServices.Cache.Remove(name, qtype)
	response.OK(w, map[string]string{"message": "缓存条目已清空", "name": name, "type": qtypeStr})
}
