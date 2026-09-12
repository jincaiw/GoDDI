package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/transfer"
	"github.com/jasonwa/goddi/internal/secretbox"
)

// --- TSIG Key Management API (RFC 8945, Technitium parity) ---

// tsigSealer returns the sealed-storage helper, or writes a response and
// returns false when the process has no encryption key configured.
//
// This is a server configuration error, so it is reported as one. Falling
// through would either store the secret in the clear or fail with a confusing
// "bad request" blaming the caller.
func tsigSealer(w http.ResponseWriter) (*secretbox.Sealer, bool) {
	if DNSServices == nil || DNSServices.Secrets == nil || !DNSServices.Secrets.Enabled() {
		response.InternalError(w, "服务端未配置加密密钥（security.encryption_key），拒绝存储或读取 TSIG 密钥")
		return nil, false
	}
	return DNSServices.Secrets, true
}

// ListTSIGKeysHandler handles GET /api/v1/dns/tsig/keys
func ListTSIGKeysHandler(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	sealer, ok := tsigSealer(w)
	if !ok {
		return
	}
	keys, err := transfer.ListTSIGKeys(DNSServices.DB, sealer)
	if err != nil {
		response.InternalErrorWithLog(w, "查询TSIG密钥失败", err)
		return
	}
	response.OK(w, keys)
}

// CreateTSIGKeyHandler handles POST /api/v1/dns/tsig/keys
//
// The generated secret is returned in this response and nowhere else: listing
// keys afterwards yields a fingerprint only.
func CreateTSIGKeyHandler(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	sealer, ok := tsigSealer(w)
	if !ok {
		return
	}

	var req struct {
		Name      string `json:"name"`
		Algorithm string `json:"algorithm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.Name == "" {
		response.BadRequest(w, "缺少密钥名称")
		return
	}
	if req.Algorithm == "" {
		req.Algorithm = "hmac-sha256"
	}

	key, err := transfer.CreateTSIGKeyRecord(DNSServices.DB, sealer, uuid.New().String(), req.Name, req.Algorithm)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	response.Created(w, key)
}

// DeleteTSIGKeyHandler handles DELETE /api/v1/dns/tsig/keys/{id}
func DeleteTSIGKeyHandler(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少密钥ID")
		return
	}
	if err := transfer.DeleteTSIGKey(DNSServices.DB, id); err != nil {
		response.InternalErrorWithLog(w, "删除TSIG密钥失败", err)
		return
	}
	response.OKWithMessage(w, "TSIG密钥已删除", nil)
}
