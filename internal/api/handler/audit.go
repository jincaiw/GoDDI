package handler

import (
	"net/http"
	"time"

	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/audit"
)

// ListAuditLogs handles GET /api/v1/logs/audit
func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	page, pageSize := response.ParsePagination(r)

	filter := audit.AuditFilter{
		UserID:       r.URL.Query().Get("user_id"),
		ResourceType: r.URL.Query().Get("resource_type"),
		Action:       r.URL.Query().Get("action"),
		Page:         page,
		PageSize:     pageSize,
	}

	if startTime := r.URL.Query().Get("start_time"); startTime != "" {
		t, err := time.Parse(time.RFC3339, startTime)
		if err == nil {
			filter.StartTime = &t
		}
	}

	if endTime := r.URL.Query().Get("end_time"); endTime != "" {
		t, err := time.Parse(time.RFC3339, endTime)
		if err == nil {
			filter.EndTime = &t
		}
	}

	logs, total, err := h.auditMgr.QueryLogs(filter)
	if err != nil {
		response.InternalError(w, "查询审计日志失败")
		return
	}

	response.OKPaginated(w, logs, total, page, pageSize)
}

// ListLoginHistory handles GET /api/v1/logs/login
func (h *Handlers) ListLoginHistory(w http.ResponseWriter, r *http.Request) {
	page, pageSize := response.ParsePagination(r)
	userID := r.URL.Query().Get("user_id")

	history, total, err := h.auditMgr.QueryLoginHistory(userID, page, pageSize)
	if err != nil {
		response.InternalError(w, "查询登录历史失败")
		return
	}

	response.OKPaginated(w, history, total, page, pageSize)
}
