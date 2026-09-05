package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/audit"
	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/rbac"
)

// ListAPITokens handles GET /api/v1/tokens
func (h *Handlers) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	tokens, err := h.tokenMgr.ListTokens(userID)
	if err != nil {
		response.InternalError(w, "查询令牌列表失败")
		return
	}

	response.OK(w, tokens)
}

// CreateAPIToken handles POST /api/v1/tokens
func (h *Handlers) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	var req struct {
		Name           string   `json:"name"`
		Scope          string   `json:"scope"`
		Description    string   `json:"description"`
		ExpiresAt      *string  `json:"expires_at,omitempty"`
		IsSingleUse    bool     `json:"is_single_use"`
		IsReadonly     bool     `json:"is_readonly"`
		IPRestrictions []string `json:"ip_restrictions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "令牌名称不能为空")
		return
	}

	var opts auth.TokenOptions
	opts.IsSingleUse = req.IsSingleUse
	opts.IsReadonly = req.IsReadonly
	opts.IPRestrictions = req.IPRestrictions

	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			response.BadRequest(w, "无效的过期时间格式，请使用RFC3339格式")
			return
		}
		opts.ExpiresAt = &t
	}

	token, fullToken, err := h.tokenMgr.CreateToken(userID, req.Name, req.Scope, opts)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidTokenScope) || strings.Contains(err.Error(), "invalid IP restriction") || strings.Contains(err.Error(), "expires_at must be in the future") {
			response.BadRequest(w, err.Error())
			return
		}
		response.InternalError(w, "创建令牌失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       userID,
		Username:     rbac.GetUsername(r.Context()),
		Action:       "create",
		ResourceType: "token",
		ResourceID:   token.ID,
		Detail:       "name=" + req.Name,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	// Return the full token ONLY once
	response.Created(w, map[string]interface{}{
		"id":            token.ID,
		"name":          token.Name,
		"token":         fullToken, // This is the only time the full token is shown
		"token_prefix":  token.TokenPrefix,
		"scope":         token.Scope,
		"is_single_use": token.IsSingleUse,
		"is_readonly":   token.IsReadonly,
		"expires_at":    token.ExpiresAt,
		"created_at":    token.CreatedAt,
		"message":       "请妥善保存此令牌，系统不会再次显示完整令牌",
	})
}

// DeleteAPIToken handles DELETE /api/v1/tokens/{id}
func (h *Handlers) DeleteAPIToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "id")
	if tokenID == "" {
		response.BadRequest(w, "缺少令牌ID")
		return
	}

	// Verify the token belongs to the current user
	userID := rbac.GetUserID(r.Context())
	token, err := h.tokenMgr.GetTokenByID(tokenID)
	if err != nil {
		response.NotFound(w, "令牌不存在")
		return
	}

	// Only the owner or an admin can revoke a token
	if token.UserID != userID {
		// Check if user has admin role
		isAdmin, _ := h.rbacMgr.CheckPermission(userID, "token", "delete")
		if !isAdmin {
			response.Forbidden(w, "无权撤销此令牌")
			return
		}
	}

	if err := h.tokenMgr.RevokeToken(tokenID); err != nil {
		response.InternalError(w, "撤销令牌失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       userID,
		Username:     rbac.GetUsername(r.Context()),
		Action:       "revoke",
		ResourceType: "token",
		ResourceID:   tokenID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "令牌已撤销"})
}
