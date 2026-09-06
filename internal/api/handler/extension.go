package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/config"
)

// --- Extension Point API Handlers ---
// SSO / Cluster / Apps / DoQ remain extension stubs (501).

func notImplementedExtension(w http.ResponseWriter) {
	response.NotImplemented(w, "此功能将在后续版本中提供")
}

// --- DNS Encryption Listeners ---

// listenerSettingKeys maps listener kind -> persisted setting key.
var listenerSettingKeys = map[string]string{
	"dot": "dns_dot_config",
	"doh": "dns_doh_config",
	"doq": "dns_doq_config",
}

// configureListener is the shared implementation behind ConfigureDoT and
// ConfigureDoH. It validates the request, persists the config as a JSON
// setting, and hot-restarts the listener.
func configureListener(w http.ResponseWriter, r *http.Request, kind string) {
	if DNSServices == nil || DNSServices.DNSServer == nil || SystemServices == nil || SystemServices.SettingsMgr == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req config.DNSListenerTLSConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Enabled {
		if req.Address == "" {
			response.BadRequest(w, "启用监听时必须提供监听地址")
			return
		}
		if req.CertFile == "" || req.KeyFile == "" {
			response.BadRequest(w, "TLS 监听必须提供证书与私钥文件路径")
			return
		}
		for _, f := range []string{req.CertFile, req.KeyFile} {
			if _, err := os.Stat(f); err != nil {
				response.BadRequest(w, fmt.Sprintf("TLS 文件不可访问: %s", f))
				return
			}
		}
	}

	// Persist for restarts.
	payload, err := json.Marshal(req)
	if err != nil {
		response.InternalError(w, "序列化监听配置失败")
		return
	}
	settingKey := listenerSettingKeys[kind]
	if err := SystemServices.SettingsMgr.SetSetting(settingKey, string(payload), kind+" listener config"); err != nil {
		response.InternalError(w, "保存监听配置失败: "+err.Error())
		return
	}

	// Apply at runtime (hot restart).
	if err := DNSServices.DNSServer.SetListenerConfig(kind, req); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if err := DNSServices.DNSServer.RestartListener(kind); err != nil {
		// The config is persisted but the listener failed to start; surface
		// the error so the operator can fix cert paths etc.
		response.InternalError(w, "监听器重启失败: "+err.Error())
		return
	}

	response.OKWithMessage(w, "监听配置已更新", map[string]any{"kind": kind, "enabled": req.Enabled})
}

// ConfigureDoT handles PUT /api/v1/dns/listeners/dot
func ConfigureDoT(w http.ResponseWriter, r *http.Request) {
	configureListener(w, r, "dot")
}

// ConfigureDoH handles PUT /api/v1/dns/listeners/doh
func ConfigureDoH(w http.ResponseWriter, r *http.Request) {
	configureListener(w, r, "doh")
}

// ConfigureDoQ handles PUT /api/v1/dns/listeners/doq (RFC 9250 listener).
func ConfigureDoQ(w http.ResponseWriter, r *http.Request) {
	configureListener(w, r, "doq")
}

// --- SSO ---

// GetSSOSettings handles GET /api/v1/sso
func GetSSOSettings(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}

// UpdateSSOSettings handles PUT /api/v1/sso
func UpdateSSOSettings(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}

// --- Cluster ---

// GetClusterNodes handles GET /api/v1/cluster
func GetClusterNodes(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}

// AddClusterNode handles POST /api/v1/cluster
func AddClusterNode(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}

// --- App Marketplace ---

// ListAppsHandler handles GET /api/v1/apps
func ListAppsHandler(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}

// InstallAppHandler handles POST /api/v1/apps/{id}/install
func InstallAppHandler(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "id")
	notImplementedExtension(w)
}

// --- DHCP HA ---

// GetDHCPHAConfig handles GET /api/v1/dhcp/ha
func GetDHCPHAConfig(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}
