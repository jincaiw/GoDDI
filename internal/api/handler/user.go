package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/audit"
	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/rbac"
)

// emailRegex validates basic email format.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// usernameRegex validates username format (3-50 chars, alphanumeric and underscore only).
var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,50}$`)

// ListUsers handles GET /api/v1/users
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := response.ParsePagination(r)
	offset := (page - 1) * pageSize

	// Count total
	var total int64
	err := h.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total)
	if err != nil {
		response.InternalError(w, "查询用户总数失败")
		return
	}

	// Query users
	rows, err := h.db.Query(`
		SELECT id, username, email, display_name, enabled, must_change_password,
			last_login_at, created_at, updated_at
		FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, offset,
	)
	if err != nil {
		response.InternalError(w, "查询用户列表失败")
		return
	}
	defer rows.Close()

	type UserItem struct {
		ID                 string  `json:"id"`
		Username           string  `json:"username"`
		Email              string  `json:"email"`
		DisplayName        string  `json:"display_name"`
		Enabled            bool    `json:"enabled"`
		MustChangePassword bool    `json:"must_change_password"`
		LastLoginAt        *string `json:"last_login_at,omitempty"`
		CreatedAt          string  `json:"created_at"`
		UpdatedAt          string  `json:"updated_at"`
	}

	var users []UserItem
	for rows.Next() {
		var u UserItem
		var lastLoginAt sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName,
			&u.Enabled, &u.MustChangePassword, &lastLoginAt, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			response.InternalError(w, "扫描用户数据失败")
			return
		}
		if lastLoginAt.Valid {
			u.LastLoginAt = &lastLoginAt.String
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		slog.Error("user handler: failed to iterate users", "error", err)
		response.InternalError(w, "迭代用户数据失败")
		return
	}

	response.OKPaginated(w, users, total, page, pageSize)
}

// CreateUser handles POST /api/v1/users
func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username           string `json:"username"`
		Email              string `json:"email"`
		Password           string `json:"password"`
		DisplayName        string `json:"display_name"`
		Enabled            *bool  `json:"enabled"`
		MustChangePassword *bool  `json:"must_change_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequest(w, "用户名和密码不能为空")
		return
	}

	if !usernameRegex.MatchString(req.Username) {
		response.BadRequest(w, "用户名格式无效，长度需为3-50个字符，仅允许字母、数字和下划线")
		return
	}

	// Validate email format if provided. The DB enforces uniqueness on
	// the `email` column (see migrations/001_init_auth_tables.sql) so we
	// only need to check the format here. An empty value is allowed and
	// stores SQL NULL.
	if req.Email != "" && !emailRegex.MatchString(req.Email) {
		response.BadRequest(w, "邮箱格式无效")
		return
	}

	// Validate password complexity
	if err := auth.ValidatePasswordComplexity(req.Password); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	// Hash password
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.InternalError(w, "密码哈希失败")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	mustChange := false
	if req.MustChangePassword != nil {
		mustChange = *req.MustChangePassword
	}

	id := uuid.New().String()
	_, err = h.db.Exec(`
		INSERT INTO users (id, username, email, password_hash, display_name, enabled, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, req.Username, req.Email, hash, req.DisplayName, enabled, mustChange,
	)
	if err != nil {
		// Distinguish duplicate-username (409) from other insert failures
		// (500) so operators are not misled by a generic conflict message.
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			response.Conflict(w, "用户名或邮箱已存在")
			return
		}
		slog.Error("creating user failed", "error", err)
		response.InternalError(w, "创建用户失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "create",
		ResourceType: "user",
		ResourceID:   id,
		Detail:       "username=" + req.Username,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	user := map[string]interface{}{
		"id":                   id,
		"username":             req.Username,
		"email":                req.Email,
		"display_name":         req.DisplayName,
		"enabled":              enabled,
		"must_change_password": mustChange,
	}

	response.Created(w, user)
}

// GetUser handles GET /api/v1/users/{id}
func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		response.BadRequest(w, "缺少用户ID")
		return
	}

	user, err := h.getUserByID(userID)
	if err != nil {
		response.NotFound(w, "用户不存在")
		return
	}

	// Get user roles
	roles, _ := h.rbacMgr.GetUserRoles(userID)
	user["roles"] = roles

	// Get user groups
	groups, _ := h.getUserGroups(userID)
	user["groups"] = groups

	response.OK(w, user)
}

// UpdateUser handles PUT /api/v1/users/{id}
func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		response.BadRequest(w, "缺少用户ID")
		return
	}

	var req struct {
		Email              *string `json:"email"`
		DisplayName        *string `json:"display_name"`
		Enabled            *bool   `json:"enabled"`
		MustChangePassword *bool   `json:"must_change_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Build dynamic UPDATE so that the request body is fully optional (the
	// caller may update only one field). Every column name in `updates` is
	// a hard-coded string constant, and every value is parameterised via
	// the `?` placeholder, so this concatenation is safe from SQL injection.
	// We deliberately do NOT use fmt.Sprintf to splice user input into the
	// query string.
	updates := []string{}
	args := []interface{}{}

	if req.Email != nil {
		if *req.Email != "" && !emailRegex.MatchString(*req.Email) {
			response.BadRequest(w, "邮箱格式无效")
			return
		}
		updates = append(updates, "email = ?")
		args = append(args, *req.Email)
	}
	if req.DisplayName != nil {
		updates = append(updates, "display_name = ?")
		args = append(args, *req.DisplayName)
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		args = append(args, *req.Enabled)
	}
	if req.MustChangePassword != nil {
		updates = append(updates, "must_change_password = ?")
		args = append(args, *req.MustChangePassword)
	}

	if len(updates) == 0 {
		response.BadRequest(w, "没有需要更新的字段")
		return
	}

	updates = append(updates, "updated_at = ?")
	args = append(args, time.Now().Format(time.RFC3339))
	args = append(args, userID)

	query := "UPDATE users SET " + updates[0]
	for i := 1; i < len(updates); i++ {
		query += ", " + updates[i]
	}
	query += " WHERE id = ?"

	// Verify the row exists BEFORE the UPDATE so we can return 404
	// without relying on RowsAffected (which is unreliable under SQLite
	// for some drivers when triggers are in play).
	var existing string
	if err := h.db.QueryRow(`SELECT id FROM users WHERE id = ?`, userID).Scan(&existing); err != nil {
		if err == sql.ErrNoRows {
			response.NotFound(w, "用户不存在")
			return
		}
		response.InternalError(w, "查询用户失败")
		return
	}

	_, err := h.db.Exec(query, args...)
	if err != nil {
		response.InternalError(w, "更新用户失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "update",
		ResourceType: "user",
		ResourceID:   userID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "用户已更新"})
}

// DeleteUser handles DELETE /api/v1/users/{id}
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		response.BadRequest(w, "缺少用户ID")
		return
	}

	// Prevent deleting self
	currentUserID := rbac.GetUserID(r.Context())
	if userID == currentUserID {
		response.BadRequest(w, "不能删除自己")
		return
	}

	// Use a transaction to clean up all related rows atomically. This
	// avoids leaving dangling foreign-key references in sessions,
	// user_groups, user_roles, audit logs, etc.
	tx, err := h.db.Begin()
	if err != nil {
		response.InternalError(w, "删除用户失败")
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		response.InternalError(w, "删除用户会话失败")
		return
	}
	if _, err := tx.Exec(`DELETE FROM user_groups WHERE user_id = ?`, userID); err != nil {
		response.InternalError(w, "删除用户组关联失败")
		return
	}
	if _, err := tx.Exec(`DELETE FROM user_roles WHERE user_id = ?`, userID); err != nil {
		response.InternalError(w, "删除用户角色关联失败")
		return
	}
	if _, err := tx.Exec(`DELETE FROM api_tokens WHERE user_id = ?`, userID); err != nil {
		response.InternalError(w, "删除用户令牌失败")
		return
	}
	if _, err := tx.Exec(`DELETE FROM user_totp_secrets WHERE user_id = ?`, userID); err != nil {
		response.InternalError(w, "删除用户TOTP配置失败")
		return
	}

	// Look up the username BEFORE deleting the user so we can log it
	// in the audit trail.
	var username string
	if err := tx.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&username); err != nil {
		if err == sql.ErrNoRows {
			response.NotFound(w, "用户不存在")
			return
		}
		response.InternalError(w, "查询用户失败")
		return
	}

	result, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		response.InternalError(w, "删除用户失败")
		return
	}
	// SEC: SQLite's RowsAffected() does not always reflect the actual
	// number of rows deleted (it may return 0 for a row that did exist
	// when the WHERE clause matched it, depending on the driver and
	// triggers). We already confirmed existence via the SELECT above,
	// so we don't treat a 0 from RowsAffected as a hard "not found"
	// signal any more.
	rows, _ := result.RowsAffected()
	_ = rows

	if err := tx.Commit(); err != nil {
		response.InternalError(w, "删除用户失败")
		return
	}
	committed = true

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       currentUserID,
		Username:     rbac.GetUsername(r.Context()),
		Action:       "delete",
		ResourceType: "user",
		ResourceID:   userID,
		Detail:       "username=" + username,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "用户已删除"})
}

// AssignUserRoles handles POST /api/v1/users/{id}/roles
func (h *Handlers) AssignUserRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		response.BadRequest(w, "缺少用户ID")
		return
	}

	// The console posts the full role selection as role_ids; role_id is
	// accepted for backwards compatibility with single-value callers.
	var req struct {
		RoleID  string   `json:"role_id"`
		RoleIDs []string `json:"role_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	roleIDs := req.RoleIDs
	if len(roleIDs) == 0 && req.RoleID != "" {
		roleIDs = []string{req.RoleID}
	}
	if len(roleIDs) == 0 {
		response.BadRequest(w, "角色ID不能为空")
		return
	}

	for _, roleID := range roleIDs {
		if err := h.rbacMgr.AssignRole(userID, roleID); err != nil {
			response.InternalError(w, "分配角色失败")
			return
		}
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "assign_role",
		ResourceType: "user",
		ResourceID:   userID,
		Detail:       "role_id=" + req.RoleID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "角色已分配"})
}

// RemoveUserRole handles DELETE /api/v1/users/{id}/roles/{roleId}
func (h *Handlers) RemoveUserRole(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "roleId")
	if userID == "" || roleID == "" {
		response.BadRequest(w, "缺少用户ID或角色ID")
		return
	}

	if err := h.rbacMgr.RemoveRole(userID, roleID); err != nil {
		response.InternalError(w, "移除角色失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "remove_role",
		ResourceType: "user",
		ResourceID:   userID,
		Detail:       "role_id=" + roleID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "角色已移除"})
}

// AssignUserGroup handles POST /api/v1/users/{id}/groups
func (h *Handlers) AssignUserGroup(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		response.BadRequest(w, "缺少用户ID")
		return
	}

	var req struct {
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.GroupID == "" {
		response.BadRequest(w, "组ID不能为空")
		return
	}

	_, err := h.db.Exec(`
		INSERT OR IGNORE INTO user_groups (user_id, group_id, created_at)
		VALUES (?, ?, datetime('now'))`, userID, req.GroupID)
	if err != nil {
		response.InternalError(w, "分配组失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "assign_group",
		ResourceType: "user",
		ResourceID:   userID,
		Detail:       "group_id=" + req.GroupID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "组已分配"})
}

// RemoveUserGroup handles DELETE /api/v1/users/{id}/groups/{groupId}
func (h *Handlers) RemoveUserGroup(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	groupID := chi.URLParam(r, "groupId")
	if userID == "" || groupID == "" {
		response.BadRequest(w, "缺少用户ID或组ID")
		return
	}

	_, err := h.db.Exec(`DELETE FROM user_groups WHERE user_id = ? AND group_id = ?`, userID, groupID)
	if err != nil {
		response.InternalError(w, "移除组失败")
		return
	}

	_ = h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "remove_group",
		ResourceType: "user",
		ResourceID:   userID,
		Detail:       "group_id=" + groupID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	})

	response.OK(w, map[string]string{"message": "组已移除"})
}

// getUserGroups returns the groups a user belongs to.
func (h *Handlers) getUserGroups(userID string) ([]map[string]interface{}, error) {
	rows, err := h.db.Query(`
		SELECT g.id, g.name, g.description, g.created_at
		FROM user_groups ug
		JOIN groups g ON ug.group_id = g.id
		WHERE ug.user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []map[string]interface{}
	for rows.Next() {
		var id, name, description, createdAt string
		if err := rows.Scan(&id, &name, &description, &createdAt); err != nil {
			return nil, err
		}
		groups = append(groups, map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"created_at":  createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user groups: %w", err)
	}

	return groups, nil
}
