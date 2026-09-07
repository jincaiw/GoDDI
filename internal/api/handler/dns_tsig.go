package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/transfer"
)

// --- TSIG Key Management API (RFC 8945, Technitium parity) ---

// ListTSIGKeysHandler handles GET /api/v1/dns/tsig/keys
func ListTSIGKeysHandler(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	keys, err := transfer.ListTSIGKeys(DNSServices.DB)
	if err != nil {
		response.InternalErrorWithLog(w, "查询TSIG密钥失败", err)
		return
	}
	response.OK(w, keys)
}

// CreateTSIGKeyHandler handles POST /api/v1/dns/tsig/keys
func CreateTSIGKeyHandler(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
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

	key, err := transfer.CreateTSIGKeyRecord(DNSServices.DB, uuid.New().String(), req.Name, req.Algorithm)
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
