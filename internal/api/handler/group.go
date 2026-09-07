package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/audit"
	"github.com/jasonwa/goddi/internal/rbac"
)

// ListGroups handles GET /api/v1/groups
func (h *Handlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	page, pageSize := response.ParsePagination(r)
	offset := (page - 1) * pageSize

	var total int64
	err := h.db.QueryRow(`SELECT COUNT(*) FROM groups`).Scan(&total)
	if err != nil {
		response.InternalError(w, "查询组总数失败")
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, description, created_at, updated_at
		FROM groups ORDER BY name LIMIT ? OFFSET ?`,
		pageSize, offset,
	)
	if err != nil {
		response.InternalError(w, "查询组列表失败")
		return
	}

	// First pass: read all groups into memory, then close rows to release
	// the database connection.  SQLite is configured with MaxOpenConns=1,
	// so a nested query while rows is still open would deadlock.
	type groupRow struct {
		ID          string
		Name        string
		Description string
		CreatedAt   string
		UpdatedAt   string
	}
	var rawGroups []groupRow
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			response.InternalError(w, "扫描组数据失败")
			return
		}
		rawGroups = append(rawGroups, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Error("group handler: failed to iterate groups", "error", err)
		response.InternalError(w, "迭代组数据失败")
		return
	}

	type RoleItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type GroupItem struct {
		ID          string     `json:"id"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Roles       []RoleItem `json:"roles"`
		CreatedAt   string     `json:"created_at"`
		UpdatedAt   string     `json:"updated_at"`
	}

	groups := make([]GroupItem, 0, len(rawGroups))
	for _, g := range rawGroups {
		item := GroupItem{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description,
			CreatedAt:   g.CreatedAt,
			UpdatedAt:   g.UpdatedAt,
		}

		// Fetch roles for this group – safe now because the outer rows is closed.
		roleRows, err := h.db.Query(`
			SELECT r.id, r.name FROM roles r
			JOIN group_roles gr ON r.id = gr.role_id
			WHERE gr.group_id = ?`, g.ID)
		if err == nil {
			for roleRows.Next() {
				var r RoleItem
				if err := roleRows.Scan(&r.ID, &r.Name); err == nil {
					item.Roles = append(item.Roles, r)
				}
			}
			roleRows.Close()
		}

		groups = append(groups, item)
	}

	response.OKPaginated(w, groups, total, page, pageSize)
}

// CreateGroup handles POST /api/v1/groups
func (h *Handlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "组名称不能为空")
		return
	}

	id := uuid.New().String()
	_, err := h.db.Exec(`
		INSERT INTO groups (id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		id, req.Name, req.Description,
	)
	if err != nil {
		response.Conflict(w, "组名称已存在")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "create",
		ResourceType: "group",
		ResourceID:   id,
		Detail:       "name=" + req.Name,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.Created(w, map[string]interface{}{
		"id":          id,
		"name":        req.Name,
		"description": req.Description,
	})
}

// GetGroup handles GET /api/v1/groups/{id}
func (h *Handlers) GetGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	if groupID == "" {
		response.BadRequest(w, "缺少组ID")
		return
	}

	var id, name, description, createdAt, updatedAt string
	err := h.db.QueryRow(`
		SELECT id, name, description, created_at, updated_at
		FROM groups WHERE id = ?`, groupID,
	).Scan(&id, &name, &description, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		response.NotFound(w, "组不存在")
		return
	}
	if err != nil {
		response.InternalError(w, "查询组失败")
		return
	}

	// Get group roles
	roles, _ := h.getGroupRoles(groupID)

	// Get group members
	members, _ := h.getGroupMembers(groupID)

	response.OK(w, map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
		"roles":       roles,
		"members":     members,
	})
}

// UpdateGroup handles PUT /api/v1/groups/{id}
func (h *Handlers) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	if groupID == "" {
		response.BadRequest(w, "缺少组ID")
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
		response.BadRequest(w, "组名称不能为空")
		return
	}

	// Verify the row exists BEFORE the UPDATE so we can return 404
	// without relying on RowsAffected (which is unreliable under
	// SQLite for some drivers).
	var existing string
	if err := h.db.QueryRow(`SELECT id FROM groups WHERE id = ?`, groupID).Scan(&existing); err != nil {
		if err == sql.ErrNoRows {
			response.NotFound(w, "组不存在")
			return
		}
		response.InternalError(w, "查询组失败")
		return
	}

	_, err := h.db.Exec(`
		UPDATE groups SET name = ?, description = ?, updated_at = datetime('now')
		WHERE id = ?`, req.Name, req.Description, groupID,
	)
	if err != nil {
		response.InternalError(w, "更新组失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "update",
		ResourceType: "group",
		ResourceID:   groupID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "组已更新"})
}

// DeleteGroup handles DELETE /api/v1/groups/{id}
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	if groupID == "" {
		response.BadRequest(w, "缺少组ID")
		return
	}

	// Use a transaction to clean up associations atomically.
	tx, err := h.db.Begin()
	if err != nil {
		response.InternalError(w, "删除组失败")
		return
	}
	defer tx.Rollback()

	// Verify the group exists BEFORE delete so we can return 404 without
	// relying on RowsAffected (which is unreliable under SQLite for some
	// drivers).
	var existing string
	if err := tx.QueryRow(`SELECT id FROM groups WHERE id = ?`, groupID).Scan(&existing); err != nil {
		if err == sql.ErrNoRows {
			response.NotFound(w, "组不存在")
			return
		}
		response.InternalError(w, "查询组失败")
		return
	}

	// Delete user_groups associations.
	if _, err := tx.Exec(`DELETE FROM user_groups WHERE group_id = ?`, groupID); err != nil {
		response.InternalError(w, "删除组关联用户失败")
		return
	}

	// Delete group_roles associations.
	if _, err := tx.Exec(`DELETE FROM group_roles WHERE group_id = ?`, groupID); err != nil {
		response.InternalError(w, "删除组关联角色失败")
		return
	}

	// Delete the group itself.
	if _, err := tx.Exec(`DELETE FROM groups WHERE id = ?`, groupID); err != nil {
		response.InternalError(w, "删除组失败")
		return
	}

	if err := tx.Commit(); err != nil {
		response.InternalError(w, "删除组失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "delete",
		ResourceType: "group",
		ResourceID:   groupID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "组已删除"})
}

// AssignGroupRoles handles POST /api/v1/groups/{id}/roles
func (h *Handlers) AssignGroupRoles(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	if groupID == "" {
		response.BadRequest(w, "缺少组ID")
		return
	}

	var req struct {
		RoleID  string   `json:"role_id"`
		RoleIDs []string `json:"role_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Support both single role_id and batch role_ids.
	roleIDs := req.RoleIDs
	if req.RoleID != "" {
		roleIDs = append(roleIDs, req.RoleID)
	}
	if len(roleIDs) == 0 {
		response.BadRequest(w, "角色ID不能为空")
		return
	}

	for _, roleID := range roleIDs {
		if roleID == "" {
			continue
		}
		if err := h.rbacMgr.AssignRoleToGroup(groupID, roleID); err != nil {
			response.InternalErrorWithLog(w, "分配角色失败", err)
			return
		}
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "assign_role",
		ResourceType: "group",
		ResourceID:   groupID,
		Detail:       "role_ids=" + strings.Join(roleIDs, ","),
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "角色已分配"})
}

// RemoveGroupRole handles DELETE /api/v1/groups/{id}/roles/{roleId}
func (h *Handlers) RemoveGroupRole(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "roleId")
	if groupID == "" || roleID == "" {
		response.BadRequest(w, "缺少组ID或角色ID")
		return
	}

	if err := h.rbacMgr.RemoveRoleFromGroup(groupID, roleID); err != nil {
		response.InternalError(w, "移除角色失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "remove_role",
		ResourceType: "group",
		ResourceID:   groupID,
		Detail:       "role_id=" + roleID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "角色已移除"})
}

// getGroupRoles is a helper to get roles for a group.
func (h *Handlers) getGroupRoles(groupID string) ([]map[string]interface{}, error) {
	rows, err := h.db.Query(`
		SELECT r.id, r.name, r.description, r.is_builtin
		FROM group_roles gr
		JOIN roles r ON gr.role_id = r.id
		WHERE gr.group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []map[string]interface{}
	for rows.Next() {
		var id, name, description string
		var isBuiltin bool
		if err := rows.Scan(&id, &name, &description, &isBuiltin); err != nil {
			return nil, err
		}
		roles = append(roles, map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"is_builtin":  isBuiltin,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group roles: %w", err)
	}

	return roles, nil
}

// getGroupMembers returns all members of a group.
func (h *Handlers) getGroupMembers(groupID string) ([]map[string]interface{}, error) {
	rows, err := h.db.Query(`
		SELECT u.id, u.username, u.display_name
		FROM user_groups ug
		JOIN users u ON ug.user_id = u.id
		WHERE ug.group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []map[string]interface{}
	for rows.Next() {
		var id, username, displayName string
		if err := rows.Scan(&id, &username, &displayName); err != nil {
			return nil, err
		}
		members = append(members, map[string]interface{}{
			"id":           id,
			"username":     username,
			"display_name": displayName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group members: %w", err)
	}

	return members, nil
}
