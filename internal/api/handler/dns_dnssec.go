package handler

// Batch F (Technitium parity): DNSSEC key lifecycle, DS record export and
// NSEC3 parameter endpoints. All endpoints are per-zone and gated by global
// RBAC + per-zone permissions (modify for mutations, view for reads).

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/dnssec"
)

// GetZoneDSRecords returns the DS records for all active KSKs of a zone.
// GET /api/v1/dns/zones/{id}/dnssec/ds
func GetZoneDSRecords(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	dsRecords, err := getDNSSECManager().GetDSRecords(zoneID)
	if err != nil {
		response.InternalErrorWithLog(w, "生成DS记录失败", err)
		return
	}
	if dsRecords == nil {
		dsRecords = []dnssec.DSInfo{}
	}

	response.OK(w, dsRecords)
}

// GenerateDNSSECKey generates a new KSK or ZSK for a zone.
// POST /api/v1/dns/zones/{id}/dnssec/keys  {"key_type":"KSK","algorithm":"ECDSAP256SHA256"}
func GenerateDNSSECKey(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	if !dnssecFeatureEnabled() {
		response.Forbidden(w, "DNSSEC 实验功能未启用；如需体验请在配置中设置 dns.dnssec.enabled=true。"+dnssecExperimentalNote)
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	var req struct {
		KeyType   string `json:"key_type"`
		Algorithm string `json:"algorithm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.KeyType != "KSK" && req.KeyType != "ZSK" {
		response.BadRequest(w, "key_type 必须为 KSK 或 ZSK")
		return
	}

	mgr := getDNSSECManager()
	var (
		key *dnssec.DNSSECKey
		err error
	)
	if req.KeyType == "KSK" {
		key, err = mgr.GenerateKSK(zoneID, req.Algorithm)
	} else {
		key, err = mgr.GenerateZSK(zoneID, req.Algorithm)
	}
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OKWithMessage(w, "DNSSEC 密钥已生成", key)
}

// DeleteDNSSECKey deletes a single DNSSEC key of a zone.
// DELETE /api/v1/dns/zones/{id}/dnssec/keys/{keyId}
func DeleteDNSSECKey(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	if !dnssecFeatureEnabled() {
		response.Forbidden(w, "DNSSEC 实验功能未启用；如需体验请在配置中设置 dns.dnssec.enabled=true。"+dnssecExperimentalNote)
		return
	}

	zoneID := chi.URLParam(r, "id")
	keyID := chi.URLParam(r, "keyId")
	if zoneID == "" || keyID == "" {
		response.BadRequest(w, "缺少区域ID或密钥ID")
		return
	}

	if err := getDNSSECManager().DeleteKey(zoneID, keyID); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OKWithMessage(w, "DNSSEC 密钥已删除", map[string]string{"zone_id": zoneID, "key_id": keyID})
}

// ToggleDNSSECKey enables or disables a single DNSSEC key.
// PUT /api/v1/dns/zones/{id}/dnssec/keys/{keyId}  {"enabled":true}
func ToggleDNSSECKey(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	if !dnssecFeatureEnabled() {
		response.Forbidden(w, "DNSSEC 实验功能未启用；如需体验请在配置中设置 dns.dnssec.enabled=true。"+dnssecExperimentalNote)
		return
	}

	zoneID := chi.URLParam(r, "id")
	keyID := chi.URLParam(r, "keyId")
	if zoneID == "" || keyID == "" {
		response.BadRequest(w, "缺少区域ID或密钥ID")
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if err := getDNSSECManager().SetKeyEnabled(zoneID, keyID, *req.Enabled); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OKWithMessage(w, "DNSSEC 密钥状态已更新", map[string]any{"zone_id": zoneID, "key_id": keyID, "enabled": *req.Enabled})
}

// PromoteDNSSECStandbyKeys activates the newest key of each type and retires
// the previous ones (rotation phase two).
// POST /api/v1/dns/zones/{id}/dnssec/keys/promote
func PromoteDNSSECStandbyKeys(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	if !dnssecFeatureEnabled() {
		response.Forbidden(w, "DNSSEC 实验功能未启用；如需体验请在配置中设置 dns.dnssec.enabled=true。"+dnssecExperimentalNote)
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	if err := getDNSSECManager().PromoteStandbyKeys(zoneID); err != nil {
		response.InternalErrorWithLog(w, "晋升DNSSEC密钥失败", err)
		return
	}

	response.OKWithMessage(w, "备用密钥已晋升为活动密钥", map[string]string{"zone_id": zoneID})
}

// GetZoneNSEC3Params returns the NSEC3 parameters of a zone.
// GET /api/v1/dns/zones/{id}/dnssec/nsec3
func GetZoneNSEC3Params(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	params, err := getDNSSECManager().GetNSEC3Params(zoneID)
	if err != nil {
		response.InternalErrorWithLog(w, "查询NSEC3参数失败", err)
		return
	}

	response.OK(w, params)
}

// SetZoneNSEC3Params updates the NSEC3 parameters of a zone.
// PUT /api/v1/dns/zones/{id}/dnssec/nsec3  {"iterations":0,"salt":"","optout":false}
func SetZoneNSEC3Params(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	if !dnssecFeatureEnabled() {
		response.Forbidden(w, "DNSSEC 实验功能未启用；如需体验请在配置中设置 dns.dnssec.enabled=true。"+dnssecExperimentalNote)
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	var params dnssec.NSEC3Params
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if err := getDNSSECManager().SetNSEC3Params(zoneID, params); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OKWithMessage(w, "NSEC3 参数已更新", params)
}
