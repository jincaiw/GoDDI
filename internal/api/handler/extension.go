package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
)

// --- Extension Point API Handlers ---
// All extension APIs return 501 Not Implemented with message
// "此功能将在后续版本中提供"

func notImplementedExtension(w http.ResponseWriter) {
	response.NotImplemented(w, "此功能将在后续版本中提供")
}

// --- DNS Encryption Listeners ---

// ConfigureDoT handles PUT /api/v1/dns/listeners/dot
func ConfigureDoT(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}

// ConfigureDoH handles PUT /api/v1/dns/listeners/doh
func ConfigureDoH(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
}

// ConfigureDoQ handles PUT /api/v1/dns/listeners/doq
func ConfigureDoQ(w http.ResponseWriter, r *http.Request) {
	notImplementedExtension(w)
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
