package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/system"
)

// validSettingKeys contains the set of valid setting keys.
var validSettingKeys = map[string]bool{
	"server_name":                 true,
	"server_language":             true,
	"server_dark_mode":            true,
	"dns_default_ttl":             true,
	"dns_recursion":               true,
	"dns_blocking_enabled":        true,
	"dns_rate_limit_qps":          true,
	"dns_blocklist_refresh_hours": true,
	"dns_ecs_mode":                true,
	"dns_ecs_ipv4_prefix_length":  true,
	"dns_ecs_ipv6_prefix_length":  true,
	"dns_cache_serve_stale":       true,
	"dns_cache_stale_ttl":         true,
	"dns_cache_prefetch":          true,
	"dns_cache_min_ttl":           true,
	"dns_cache_max_ttl":           true,
	"dns_special_zones":           true,
	"dns_dot_config":              true,
	"dns_doh_config":              true,
	"dns_doq_config":              true,
	"dhcp_lease_time":             true,
	"ipam_ping_check":             true,
	"ipam_auto_scan":              true,
	"security_rebinding":          true,
	"log_retention_days":          true,
	"backup_auto_enabled":         true,
	"backup_auto_schedule":        true,
	"backup_retention":            true,
}

// --- Settings API Handlers ---

// ListSettingsHandler handles GET /api/v1/settings
func ListSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.SettingsMgr == nil {
		response.InternalError(w, "系统设置服务未初始化")
		return
	}

	settings, err := SystemServices.SettingsMgr.ListSettings()
	if err != nil {
		response.InternalErrorWithLog(w, "查询系统设置失败", err)
		return
	}

	response.OK(w, settings)
}

// UpdateSettingsHandler handles PUT /api/v1/settings (batch update)
func UpdateSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.SettingsMgr == nil {
		response.InternalError(w, "系统设置服务未初始化")
		return
	}

	var settings []system.Setting
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if len(settings) == 0 {
		response.BadRequest(w, "请求数据不能为空")
		return
	}

	// Validate each setting key against the whitelist and enforce the same
	// per-key value constraints as the single-update path (otherwise values
	// like dns_default_ttl="abc" bypass validation via the batch endpoint).
	for _, s := range settings {
		if !validSettingKeys[s.Key] {
			response.BadRequest(w, "无效的设置项: "+s.Key)
			return
		}
		if err := validateSettingValue(s.Key, s.Value); err != nil {
			response.BadRequest(w, err.Error())
			return
		}
	}

	if err := SystemServices.SettingsMgr.BatchUpdateSettings(settings); err != nil {
		response.InternalErrorWithLog(w, "批量更新设置失败", err)
		return
	}

	response.OKWithMessage(w, "设置已更新", nil)
}

// validateSettingValue enforces type/range constraints on a setting update
// so a malformed value cannot crash the server or break a downstream
// component (e.g. a TTL of zero would disable caching). Only the keys
// known to need range validation are checked here; the rest accept any
// non-empty string.
func validateSettingValue(key, value string) error {
	switch key {
	case "dns_default_ttl":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("dns_default_ttl 必须是整数")
		}
		if n < 60 || n > 86400 {
			return fmt.Errorf("dns_default_ttl 必须在 60-86400 之间")
		}
	case "dhcp_lease_time":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("dhcp_lease_time 必须是正整数")
		}
	case "log_retention_days":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("log_retention_days 必须是正整数")
		}
	case "dns_rate_limit_qps":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 1000000 {
			return fmt.Errorf("dns_rate_limit_qps 必须在 0-1000000 之间")
		}
	case "dns_blocklist_refresh_hours":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 720 {
			return fmt.Errorf("dns_blocklist_refresh_hours 必须在 1-720 之间")
		}
	case "dns_ecs_mode":
		switch value {
		case "strip", "passthrough", "add":
		default:
			return fmt.Errorf("dns_ecs_mode 必须是 strip、passthrough 或 add")
		}
	case "dns_ecs_ipv4_prefix_length":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 32 {
			return fmt.Errorf("dns_ecs_ipv4_prefix_length 必须在 0-32 之间")
		}
	case "dns_ecs_ipv6_prefix_length":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 128 {
			return fmt.Errorf("dns_ecs_ipv6_prefix_length 必须在 0-128 之间")
		}
	case "dns_cache_stale_ttl", "dns_cache_min_ttl", "dns_cache_max_ttl":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 604800 {
			return fmt.Errorf("%s 必须在 0-604800 之间", key)
		}
	}
	return nil
}

// UpdateSingleSettingHandler handles PUT /api/v1/settings/{key}
func UpdateSingleSettingHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.SettingsMgr == nil {
		response.InternalError(w, "系统设置服务未初始化")
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequest(w, "缺少设置键名")
		return
	}

	// Validate key is a known setting.
	if !validSettingKeys[key] {
		response.BadRequest(w, "无效的设置项")
		return
	}

	var req struct {
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Value == "" {
		response.BadRequest(w, "设置值不能为空")
		return
	}

	// Per-key value validation.
	if err := validateSettingValue(key, req.Value); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	if err := SystemServices.SettingsMgr.SetSetting(key, req.Value, req.Description); err != nil {
		response.InternalErrorWithLog(w, "更新设置失败", err)
		return
	}

	response.OKWithMessage(w, "设置已更新", map[string]string{"key": key})
}
