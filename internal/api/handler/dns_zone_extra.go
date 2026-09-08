package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/rbac"
)

// --- Zone enable/disable + history API handlers (Technitium parity) ---

// EnableDNSZone re-enables a disabled zone; the server answers for it again.
// POST /api/v1/dns/zones/{id}/enable
func EnableDNSZone(w http.ResponseWriter, r *http.Request) {
	setZoneEnabled(w, r, true)
}

// DisableDNSZone disables a zone; the server stops answering for it
// (queries fall through as if the zone did not exist).
// POST /api/v1/dns/zones/{id}/disable
func DisableDNSZone(w http.ResponseWriter, r *http.Request) {
	setZoneEnabled(w, r, false)
}

func setZoneEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域 ID")
		return
	}

	opts := zone.ZoneOptions{Enabled: &enabled}
	z, err := getZoneManager().UpdateZone(id, opts)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	action := "启用"
	if !enabled {
		action = "禁用"
	}
	response.OKWithMessage(w, action+"成功", z)
}

// GetZoneHistory returns the recorded RR mutations for a zone, newest first.
// GET /api/v1/dns/zones/{id}/history?page=&page_size=
func GetZoneHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域 ID")
		return
	}

	page, pageSize := response.ParsePagination(r)
	entries, total, err := getZoneManager().ListZoneHistory(id, page, pageSize)
	if err != nil {
		response.InternalErrorWithLog(w, "查询区域历史失败", err)
		return
	}

	response.OKPaginated(w, entries, total, page, pageSize)
}

// --- Per-zone permissions (Technitium Zone Permissions parity) ---

// GetZonePermissions returns the user/group permission list of a zone.
// GET /api/v1/dns/zones/{id}/permissions
func GetZonePermissions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域 ID")
		return
	}

	perms, err := getZoneManager().GetZonePermissions(id)
	if err != nil {
		response.InternalErrorWithLog(w, "查询区域权限失败", err)
		return
	}

	response.OK(w, perms)
}

// SetZonePermissions replaces the full permission list of a zone. An empty
// list reverts the zone to global-RBAC-only behavior.
// PUT /api/v1/dns/zones/{id}/permissions
func SetZonePermissions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域 ID")
		return
	}

	var req struct {
		Permissions []zone.ZonePermission `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if err := getZoneManager().SetZonePermissions(id, req.Permissions); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	perms, err := getZoneManager().GetZonePermissions(id)
	if err != nil {
		response.InternalErrorWithLog(w, "查询区域权限失败", err)
		return
	}

	response.OKWithMessage(w, "区域权限已更新", perms)
}

// RequireZonePermission enforces per-zone permission lists at the route
// level. It resolves the zone id from either the "id" (zone routes) or
// "zoneId" (record routes) URL parameter. Zones without an explicit
// permission list pass through (global RBAC already handled upstream),
// yielding the documented intersection with global RBAC.
func RequireZonePermission(action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			zoneID := chi.URLParam(r, "zoneId")
			if zoneID == "" {
				zoneID = chi.URLParam(r, "id")
			}
			if zoneID != "" {
				allowed, err := getZoneManager().ZonePermissionAllows(
					zoneID, rbac.GetUserID(r.Context()), rbac.GetRoleIDs(r.Context()), action)
				if err != nil {
					response.InternalErrorWithLog(w, "区域权限检查失败", err)
					return
				}
				if !allowed {
					response.Forbidden(w, "区域权限不足")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetCatalogMembers lists the member zones of a catalog zone (RFC 9432).
// GET /api/v1/dns/zones/{id}/catalog/members
func GetCatalogMembers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域 ID")
		return
	}

	zm := getZoneManager()
	z, err := zm.GetZone(id)
	if err != nil {
		response.NotFound(w, "区域不存在")
		return
	}
	if z.Type != string(zone.ZoneTypeCatalog) {
		response.BadRequest(w, "该区域不是 Catalog 区域")
		return
	}

	members, err := zm.ListCatalogMembers(id)
	if err != nil {
		response.InternalErrorWithLog(w, "查询 Catalog 成员失败", err)
		return
	}
	if members == nil {
		members = []string{}
	}

	response.OK(w, members)
}
