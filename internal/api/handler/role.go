package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/audit"
	"github.com/jasonwa/goddi/internal/rbac"
)

// ListRoles handles GET /api/v1/roles
func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.rbacMgr.ListRoles()
	if err != nil {
		response.InternalError(w, "查询角色列表失败")
		return
	}

	// Load permissions for each role
	for i := range roles {
		perms, _ := h.rbacMgr.GetRolePermissions(roles[i].ID)
		roles[i].Permissions = perms
	}

	response.OK(w, roles)
}

// CreateRole handles POST /api/v1/roles
func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "角色名称不能为空")
		return
	}

	role, err := h.rbacMgr.CreateRole(req.Name, req.Description)
	if err != nil {
		response.Conflict(w, "角色名称已存在")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "create",
		ResourceType: "role",
		ResourceID:   role.ID,
		Detail:       "name=" + req.Name,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.Created(w, role)
}

// GetRole handles GET /api/v1/roles/{id}
func (h *Handlers) GetRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	if roleID == "" {
		response.BadRequest(w, "缺少角色ID")
		return
	}

	role, err := h.rbacMgr.GetRole(roleID)
	if err != nil {
		response.NotFound(w, "角色不存在")
		return
	}

	response.OK(w, role)
}

// UpdateRole handles PUT /api/v1/roles/{id}
func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	if roleID == "" {
		response.BadRequest(w, "缺少角色ID")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "角色名称不能为空")
		return
	}

	// Verify the role exists.
	role, err := h.rbacMgr.GetRole(roleID)
	if err != nil {
		response.NotFound(w, "角色不存在")
		return
	}
	if role.IsBuiltin {
		response.BadRequest(w, "内置角色不可修改")
		return
	}

	if err := h.rbacMgr.UpdateRole(roleID, req.Name, req.Description); err != nil {
		response.InternalError(w, "更新角色失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "update",
		ResourceType: "role",
		ResourceID:   roleID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "角色已更新"})
}

// DeleteRole handles DELETE /api/v1/roles/{id}
func (h *Handlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	if roleID == "" {
		response.BadRequest(w, "缺少角色ID")
		return
	}

	// Verify the role exists.
	role, err := h.rbacMgr.GetRole(roleID)
	if err != nil {
		response.NotFound(w, "角色不存在")
		return
	}
	if role.IsBuiltin {
		response.BadRequest(w, "内置角色不可删除")
		return
	}

	// Check if the role is being used by any users or groups.
	var userCount, groupCount int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM user_roles WHERE role_id = ?`, roleID).Scan(&userCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM group_roles WHERE role_id = ?`, roleID).Scan(&groupCount)
	if userCount > 0 || groupCount > 0 {
		response.BadRequest(w, fmt.Sprintf("角色正在被使用（%d个用户，%d个组），无法删除", userCount, groupCount))
		return
	}

	if err := h.rbacMgr.DeleteRole(roleID); err != nil {
		response.InternalError(w, "删除角色失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "delete",
		ResourceType: "role",
		ResourceID:   roleID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "角色已删除"})
}

// AssignRolePermissions handles POST /api/v1/roles/{id}/permissions
func (h *Handlers) AssignRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	if roleID == "" {
		response.BadRequest(w, "缺少角色ID")
		return
	}

	var req struct {
		PermissionID string `json:"permission_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.PermissionID == "" {
		response.BadRequest(w, "权限ID不能为空")
		return
	}

	if err := h.rbacMgr.AssignPermissionToRole(roleID, req.PermissionID); err != nil {
		response.InternalError(w, "分配权限失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "assign_permission",
		ResourceType: "role",
		ResourceID:   roleID,
		Detail:       "permission_id=" + req.PermissionID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "权限已分配"})
}

// RemoveRolePermission handles DELETE /api/v1/roles/{id}/permissions/{permId}
func (h *Handlers) RemoveRolePermission(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	permID := chi.URLParam(r, "permId")
	if roleID == "" || permID == "" {
		response.BadRequest(w, "缺少角色ID或权限ID")
		return
	}

	if err := h.rbacMgr.RemovePermissionFromRole(roleID, permID); err != nil {
		response.InternalError(w, "移除权限失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "remove_permission",
		ResourceType: "role",
		ResourceID:   roleID,
		Detail:       "permission_id=" + permID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "权限已移除"})
}

// ListPermissionsHandler handles GET /api/v1/permissions
func (h *Handlers) ListPermissionsHandler(w http.ResponseWriter, r *http.Request) {
	permissions, err := h.rbacMgr.ListPermissions()
	if err != nil {
		response.InternalError(w, "查询权限列表失败")
		return
	}

	response.OK(w, permissions)
}
