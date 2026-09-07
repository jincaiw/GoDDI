package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/middleware"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/audit"
	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/rbac"
)

// logAuditError logs (but does not propagate) an audit logging failure.
func logAuditError(action string, err error) {
	if err != nil {
		slog.Error("audit_log_failed", "action", action, "error", err)
	}
}

// Handlers holds all dependencies for API handlers.
type Handlers struct {
	db        *sql.DB
	jwtMgr    *auth.JWTManager
	sessMgr   *auth.SessionManager
	rateLimit *auth.RateLimiter
	totpMgr   *auth.TOTPManager
	tokenMgr  *auth.TokenManager
	rbacMgr   *rbac.RBACManager
	auditMgr  *audit.AuditManager
}

// NewHandlers creates a new Handlers instance with all dependencies.
// loginRateWindowSec is the lockout window in seconds; pass 0 for the
// default (15 minutes).
func NewHandlers(db *sql.DB, jwtSecret string, totpEncryptionKey string, loginRateLimit int, loginRateWindowSec int) *Handlers {
	jwtMgr, jwtErr := auth.NewJWTManager(jwtSecret)
	if jwtErr != nil {
		slog.Error("invalid JWT secret; authentication will reject all tokens", "error", jwtErr)
	}
	sessMgr := auth.NewSessionManager(db)
	rateLimit := auth.NewRateLimiter(db, loginRateLimit, time.Duration(loginRateWindowSec)*time.Second)
	if totpEncryptionKey == "" {
		slog.Warn("TOTP encryption key not configured; deriving from JWT secret. Set security.encryption_key for dedicated key material.")
		totpEncryptionKey = jwtSecret
	}
	totpMgr := auth.NewTOTPManager(db, "GoDDI", totpEncryptionKey)
	tokenMgr := auth.NewTokenManager(db)
	rbacMgr := rbac.NewRBACManager(db)
	auditMgr := audit.NewAuditManager(db)

	// Ensure rate limit table exists
	if err := auth.EnsureRateLimitTable(db); err != nil {
		slog.Error("failed to create login_rate_limits table", "error", err)
	}

	return &Handlers{
		db:        db,
		jwtMgr:    jwtMgr,
		sessMgr:   sessMgr,
		rateLimit: rateLimit,
		totpMgr:   totpMgr,
		tokenMgr:  tokenMgr,
		rbacMgr:   rbacMgr,
		auditMgr:  auditMgr,
	}
}

// --- Auth Handlers ---

// InitAdmin handles POST /api/v1/auth/init
// Creates the first admin user if no users exist yet.
func (h *Handlers) InitAdmin(w http.ResponseWriter, r *http.Request) {
	// Check if already initialized
	if auth.CheckIfInitialized(h.db) {
		response.Conflict(w, "系统已初始化，无法重复创建管理员")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequest(w, "用户名和密码不能为空")
		return
	}

	if req.Email != "" && !emailRegex.MatchString(req.Email) {
		response.BadRequest(w, "邮箱格式无效")
		return
	}

	if err := auth.ValidatePasswordComplexity(req.Password); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	if err := auth.InitializeAdmin(h.db, req.Username, req.Password); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	logAuditError("init_admin", h.auditMgr.Log(audit.LogEntry{
		Username:     req.Username,
		Action:       "init_admin",
		ResourceType: "system",
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	}))

	response.OK(w, map[string]string{"message": "管理员账户创建成功"})
}

// Login handles POST /api/v1/auth/login
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTPCode string `json:"totp_code,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequest(w, "用户名和密码不能为空")
		return
	}

	sourceIP := getClientIP(r)
	userAgent := r.UserAgent()

	// Check rate limit
	allowed, err := h.rateLimit.CheckLoginRate(req.Username, sourceIP)
	if err != nil {
		slog.Error("login: rate limit check failed", "username", req.Username, "error", err)
		response.InternalError(w, "限速检查失败")
		return
	}
	if !allowed {
		response.TooManyRequests(w, "登录尝试次数过多，请稍后再试")
		return
	}

	// Look up user
	var userID, passwordHash string
	var enabled bool
	err = h.db.QueryRow(`
		SELECT id, password_hash, enabled FROM users WHERE username = ?`,
		req.Username,
	).Scan(&userID, &passwordHash, &enabled)

	if err == sql.ErrNoRows {
		_ = h.rateLimit.RecordFailedLogin(req.Username, sourceIP)
		logAuditError("login", h.auditMgr.LogLogin("", req.Username, "password", sourceIP, userAgent, false, "用户不存在"))
		// Perform a dummy hash verify to keep the response time constant and
		// to avoid revealing whether the account exists.
		_, _ = auth.VerifyPassword("$argon2id$v=19$m=65536,t=1,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", req.Password)
		response.Unauthorized(w, "用户名或密码错误")
		return
	}
	if err != nil {
		slog.Error("login: query user failed", "error", err)
		response.InternalError(w, "查询用户失败")
		return
	}

	if !enabled {
		_ = h.rateLimit.RecordFailedLogin(req.Username, sourceIP)
		logAuditError("login", h.auditMgr.LogLogin(userID, req.Username, "password", sourceIP, userAgent, false, "用户已禁用"))
		// Still perform a verify to keep timing consistent.
		_, _ = auth.VerifyPassword(passwordHash, req.Password)
		// Uniform error message: do not distinguish "disabled" from "wrong password".
		response.Unauthorized(w, "用户名或密码错误")
		return
	}

	// Verify password
	match, err := auth.VerifyPassword(passwordHash, req.Password)
	if err != nil || !match {
		_ = h.rateLimit.RecordFailedLogin(req.Username, sourceIP)
		logAuditError("login", h.auditMgr.LogLogin(userID, req.Username, "password", sourceIP, userAgent, false, "密码错误"))
		response.Unauthorized(w, "用户名或密码错误")
		return
	}

	// Check TOTP if enabled
	totpEnabled, err := h.totpMgr.IsTOTPEnabled(userID)
	if err != nil {
		slog.Error("login: failed to check TOTP status", "user_id", userID, "error", err)
		response.InternalError(w, "验证双因素认证状态失败")
		return
	}
	if totpEnabled {
		if req.TOTPCode == "" {
			// Count missing TOTP code as a failed login attempt too, so that
			// an attacker who has guessed the password cannot endlessly
			// retry without eventually hitting the rate limiter.
			_ = h.rateLimit.RecordFailedLogin(req.Username, sourceIP)
			logAuditError("login", h.auditMgr.LogLogin(userID, req.Username, "totp", sourceIP, userAgent, false, "需要TOTP验证码"))
			response.Unauthorized(w, "需要双因素认证验证码")
			return
		}

		secret, err := h.totpMgr.GetTOTPSecret(userID)
		if err != nil {
			slog.Error("login: failed to get TOTP secret", "user_id", userID, "error", err)
			response.InternalError(w, "获取双因素认证信息失败")
			return
		}
		if !auth.VerifyTOTP(secret, req.TOTPCode) {
			// Try recovery code
			recoveryOk, err := h.totpMgr.VerifyRecoveryCode(userID, req.TOTPCode)
			if err != nil {
				slog.Error("login: failed to verify recovery code", "user_id", userID, "error", err)
				response.InternalError(w, "验证恢复码失败")
				return
			}
			if !recoveryOk {
				_ = h.rateLimit.RecordFailedLogin(req.Username, sourceIP)
				logAuditError("login", h.auditMgr.LogLogin(userID, req.Username, "totp", sourceIP, userAgent, false, "TOTP验证码错误"))
				response.Unauthorized(w, "验证码错误")
				return
			}
		}
	}

	// Reset rate limit on successful login
	if err := h.rateLimit.ResetLoginAttempts(req.Username, sourceIP); err != nil {
		slog.Error("login: failed to reset rate limit", "username", req.Username, "error", err)
	}

	// Create session
	_, session, err := h.sessMgr.CreateSession(userID, sourceIP, userAgent)
	if err != nil {
		slog.Error("login: create session failed", "user_id", userID, "error", err)
		response.InternalError(w, "创建会话失败")
		return
	}

	// Get user roles
	roles, err := h.rbacMgr.GetUserRoles(userID)
	if err != nil {
		slog.Error("login: get user roles failed", "user_id", userID, "error", err)
		response.InternalError(w, "获取用户角色失败")
		return
	}
	roleIDs := make([]string, 0, len(roles))
	for _, r := range roles {
		roleIDs = append(roleIDs, r.ID)
	}

	// Generate JWT token
	token, err := h.jwtMgr.GenerateToken(userID, req.Username, roleIDs, session.ID)
	if err != nil {
		slog.Error("login: generate token failed", "user_id", userID, "error", err)
		response.InternalError(w, "生成令牌失败")
		return
	}

	// Update last login
	if _, err := h.db.Exec(`UPDATE users SET last_login_at = datetime('now') WHERE id = ?`, userID); err != nil {
		slog.Error("login: update last login failed", "user_id", userID, "error", err)
	}

	// Audit log
	logAuditError("login", h.auditMgr.LogLogin(userID, req.Username, "password", sourceIP, userAgent, true, ""))

	// Issue a CSRF token alongside the JWT so browser-style clients can use
	// it for subsequent mutating requests. The token is derived from the
	// JWT secret via HKDF and is only valid for mutating methods enforced
	// by the CSRF middleware.
	csrfToken := middleware.GenerateCSRFToken(h.jwtMgr.CSRFKey(), session.ID)

	response.OK(w, map[string]interface{}{
		"token":        token,
		"csrf_token":   csrfToken,
		"expires_in":   int(auth.AccessTokenDuration.Seconds()),
		"user_id":      userID,
		"username":     req.Username,
		"totp_enabled": totpEnabled,
	})
}

// Logout handles POST /api/v1/auth/logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token != "" {
		claims, err := h.jwtMgr.ParseToken(token)
		if err == nil && claims.SessionID != "" {
			_ = h.sessMgr.DeleteSession(claims.SessionID)
		}
	}
	response.OK(w, map[string]string{"message": "已登出"})
}

// ListLockouts handles GET /api/v1/auth/lockouts
// Returns the usernames/IPs currently locked out by the login rate limiter,
// so an administrator can tell a mistyped password from an active DoS attempt
// and unlock the affected account.
func (h *Handlers) ListLockouts(w http.ResponseWriter, r *http.Request) {
	entries, err := h.rateLimit.ListLockedEntries()
	if err != nil {
		response.InternalErrorWithLog(w, "查询登录锁定失败", err)
		return
	}
	response.OK(w, entries)
}

// UnlockUser handles POST /api/v1/auth/unlock
// Clears the login rate-limit state for a username (optionally scoped to one
// source IP). This is the API counterpart of the `goddi unlock` CLI command:
// an account locked out by repeated failures cannot log in to unlock itself,
// so an administrator with user:write must be able to do it.
func (h *Handlers) UnlockUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		IP       string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Username == "" {
		response.BadRequest(w, "缺少用户名")
		return
	}

	// Audit the unlock before performing it so the trail survives a failure.
	logAuditError("unlock_user", h.auditMgr.Log(audit.LogEntry{
		Username:     rbac.GetUsername(r.Context()),
		Action:       "unlock_user",
		ResourceType: "user",
		ResourceID:   req.Username,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	}))

	var err error
	var removed int64
	if req.IP != "" {
		err = h.rateLimit.ResetLoginAttempts(req.Username, req.IP)
		removed = 1
	} else {
		removed, err = h.rateLimit.ResetUserAttempts(req.Username)
	}
	if err != nil {
		response.InternalErrorWithLog(w, "解锁失败", err)
		return
	}

	response.OKWithMessage(w, "已解锁", map[string]any{
		"username": req.Username,
		"entries":  removed,
	})
}


// SetupTOTP handles POST /api/v1/auth/totp/setup
func (h *Handlers) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	username := rbac.GetUsername(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	// Refuse to regenerate a secret when TOTP is already active, otherwise
	// a successful /setup would silently invalidate the existing authenticator
	// app configuration. The user must explicitly disable TOTP first.
	enabled, err := h.totpMgr.IsTOTPEnabled(userID)
	if err != nil {
		slog.Error("setup-totp: failed to query TOTP status", "user_id", userID, "error", err)
		response.InternalError(w, "查询TOTP状态失败")
		return
	}
	if enabled {
		response.Conflict(w, "TOTP已经启用，请先禁用后再重新设置")
		return
	}

	secret, url, err := h.totpMgr.GenerateTOTPSecret(userID, username)
	if err != nil {
		response.InternalError(w, "生成TOTP密钥失败")
		return
	}

	response.OK(w, map[string]string{
		"secret": secret,
		"qr_url": url,
	})
}

// VerifyAndEnableTOTP handles POST /api/v1/auth/totp/verify
func (h *Handlers) VerifyAndEnableTOTP(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	// Reject re-verification: enabling an already-enabled TOTP would reissue
	// recovery codes, which the user has no way to know have changed. Force
	// them through the disable flow first.
	enabled, err := h.totpMgr.IsTOTPEnabled(userID)
	if err != nil {
		slog.Error("verify-totp: failed to query TOTP status", "user_id", userID, "error", err)
		response.InternalError(w, "查询TOTP状态失败")
		return
	}
	if enabled {
		response.Conflict(w, "TOTP已经启用")
		return
	}

	var req struct {
		Code   string `json:"code"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if !auth.VerifyTOTP(req.Secret, req.Code) {
		response.BadRequest(w, "验证码错误")
		return
	}

	if err := h.totpMgr.EnableTOTP(userID, req.Secret); err != nil {
		response.InternalError(w, "启用TOTP失败")
		return
	}

	// Generate recovery codes
	codes, err := h.totpMgr.GenerateRecoveryCodes(userID)
	if err != nil {
		response.InternalError(w, "生成恢复码失败")
		return
	}

	// Audit
	logAuditError("enable_totp", h.auditMgr.Log(audit.LogEntry{
		UserID:       userID,
		Username:     rbac.GetUsername(r.Context()),
		Action:       "enable_totp",
		ResourceType: "user",
		ResourceID:   userID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	}))

	response.OK(w, map[string]interface{}{
		"recovery_codes": codes,
		"message":        "双因素认证已启用，请妥善保存恢复码",
	})
}

// DisableTOTPHandler handles POST /api/v1/auth/totp/disable
func (h *Handlers) DisableTOTPHandler(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	// Disabling an already-disabled TOTP is meaningless and usually signals
	// client confusion / a forged request. Reject it explicitly rather than
	// silently succeeding.
	enabled, err := h.totpMgr.IsTOTPEnabled(userID)
	if err != nil {
		slog.Error("disable-totp: failed to query TOTP status", "user_id", userID, "error", err)
		response.InternalError(w, "查询TOTP状态失败")
		return
	}
	if !enabled {
		response.BadRequest(w, "TOTP未启用，无需禁用")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Verify password before disabling TOTP
	var passwordHash string
	err = h.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&passwordHash)
	if err != nil {
		response.InternalError(w, "查询用户失败")
		return
	}

	match, err := auth.VerifyPassword(passwordHash, req.Password)
	if err != nil || !match {
		response.Unauthorized(w, "密码错误")
		return
	}

	if err := h.totpMgr.DisableTOTP(userID); err != nil {
		response.InternalError(w, "禁用TOTP失败")
		return
	}

	logAuditError("disable_totp", h.auditMgr.Log(audit.LogEntry{
		UserID:       userID,
		Username:     rbac.GetUsername(r.Context()),
		Action:       "disable_totp",
		ResourceType: "user",
		ResourceID:   userID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	}))

	response.OK(w, map[string]string{"message": "双因素认证已禁用"})
}

// ChangePassword handles POST /api/v1/auth/password/change
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Verify old password
	var passwordHash string
	err := h.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&passwordHash)
	if err != nil {
		response.InternalError(w, "查询用户失败")
		return
	}

	match, err := auth.VerifyPassword(passwordHash, req.OldPassword)
	if err != nil || !match {
		response.Unauthorized(w, "原密码错误")
		return
	}

	// Validate new password complexity
	if err := auth.ValidatePasswordComplexity(req.NewPassword); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	// Hash and update
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.InternalError(w, "密码哈希失败")
		return
	}

	_, err = h.db.Exec(`UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = datetime('now') WHERE id = ?`,
		newHash, userID)
	if err != nil {
		response.InternalError(w, "更新密码失败")
		return
	}

	// Invalidate all other sessions for this user to prevent session theft
	// after a password change. The current session is preserved by excluding
	// the session ID referenced by the request token.
	currentSessionID := rbac.GetSessionID(r.Context())
	if currentSessionID != "" {
		if _, err := h.db.Exec(
			`DELETE FROM sessions WHERE user_id = ? AND id != ?`,
			userID, currentSessionID,
		); err != nil {
			slog.Error("change_password: failed to invalidate other sessions", "user_id", userID, "error", err)
		}
	} else {
		if _, err := h.db.Exec(
			`DELETE FROM sessions WHERE user_id = ?`,
			userID,
		); err != nil {
			slog.Error("change_password: failed to invalidate sessions", "user_id", userID, "error", err)
		}
	}

	logAuditError("change_password", h.auditMgr.Log(audit.LogEntry{
		UserID:       userID,
		Username:     rbac.GetUsername(r.Context()),
		Action:       "change_password",
		ResourceType: "user",
		ResourceID:   userID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	}))

	response.OK(w, map[string]string{"message": "密码已更改"})
}

// ListSessions handles GET /api/v1/auth/sessions
func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	sessions, err := h.sessMgr.ListUserSessions(userID)
	if err != nil {
		response.InternalError(w, "查询会话失败")
		return
	}

	response.OK(w, sessions)
}

// DeleteSession handles DELETE /api/v1/auth/sessions/{id}
func (h *Handlers) DeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		response.BadRequest(w, "缺少会话ID")
		return
	}

	currentUserID := rbac.GetUserID(r.Context())
	if currentUserID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	// Fetch the session to verify ownership.
	session, err := h.sessMgr.GetSessionByID(sessionID)
	if err != nil {
		response.NotFound(w, "会话不存在")
		return
	}

	// Check if the current user owns the session or is an admin.
	if session.UserID != currentUserID {
		isAdmin, err := h.rbacMgr.CheckPermission(currentUserID, "user", "delete")
		if err != nil || !isAdmin {
			response.Forbidden(w, "无权删除此会话")
			return
		}
	}

	if err := h.sessMgr.DeleteSession(sessionID); err != nil {
		response.InternalError(w, "删除会话失败")
		return
	}

	logAuditError("delete_session", h.auditMgr.Log(audit.LogEntry{
		UserID:       rbac.GetUserID(r.Context()),
		Username:     rbac.GetUsername(r.Context()),
		Action:       "delete_session",
		ResourceType: "session",
		ResourceID:   sessionID,
		SourceIP:     getClientIP(r),
		UserAgent:    r.UserAgent(),
		Success:      true,
	}))

	response.OK(w, map[string]string{"message": "会话已删除"})
}

// GetCurrentUser handles GET /api/v1/auth/me
func (h *Handlers) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := rbac.GetUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "未认证")
		return
	}

	user, err := h.getUserByID(userID)
	if err != nil {
		response.NotFound(w, "用户不存在")
		return
	}

	// Get user roles
	roles, err := h.rbacMgr.GetUserRoles(userID)
	if err != nil {
		slog.Error("get_current_user: get user roles failed", "user_id", userID, "error", err)
		response.InternalError(w, "获取用户角色失败")
		return
	}
	user["roles"] = roles

	// Get user permissions
	perms, err := h.rbacMgr.GetUserPermissions(userID)
	if err != nil {
		slog.Error("get_current_user: get user permissions failed", "user_id", userID, "error", err)
		response.InternalError(w, "获取用户权限失败")
		return
	}
	user["permissions"] = perms

	response.OK(w, user)
}

// RefreshToken handles POST /api/v1/auth/refresh
func (h *Handlers) RefreshToken(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		response.Unauthorized(w, "缺少令牌")
		return
	}

	claims, err := h.jwtMgr.ParseToken(token)
	if err != nil {
		response.Unauthorized(w, "无效的令牌")
		return
	}

	// SEC-05: Validate that the session still exists and has not expired.
	if claims.SessionID != "" {
		session, err := h.sessMgr.GetSessionByID(claims.SessionID)
		if err != nil {
			response.Unauthorized(w, "会话不存在或已过期")
			return
		}
		if time.Now().After(session.ExpiresAt) {
			// Clean up expired session.
			if delErr := h.sessMgr.DeleteSession(claims.SessionID); delErr != nil {
				slog.Error("refresh: failed to delete expired session", "session_id", claims.SessionID, "error", delErr)
			}
			response.Unauthorized(w, "会话已过期")
			return
		}
	}

	// Reload the user from the database to confirm the account is still
	// enabled. Without this check, a disabled/deleted user could keep
	// using a previously issued token.
	var enabled bool
	if err := h.db.QueryRow(
		`SELECT enabled FROM users WHERE id = ?`,
		claims.UserID,
	).Scan(&enabled); err != nil {
		if err == sql.ErrNoRows {
			response.Unauthorized(w, "用户不存在")
			return
		}
		slog.Error("refresh: failed to load user", "user_id", claims.UserID, "error", err)
		response.InternalError(w, "查询用户失败")
		return
	}
	if !enabled {
		response.Unauthorized(w, "账户已被禁用")
		return
	}

	// Generate a new token with the same claims.
	newToken, err := h.jwtMgr.GenerateToken(claims.UserID, claims.Username, claims.RoleIDs, claims.SessionID)
	if err != nil {
		slog.Error("refresh: generate token failed", "user_id", claims.UserID, "error", err)
		response.InternalError(w, "生成令牌失败")
		return
	}

	// Issue a fresh CSRF token alongside the new JWT, bound to the same
	// session carried by the refreshed claims.
	csrfToken := middleware.GenerateCSRFToken(h.jwtMgr.CSRFKey(), claims.SessionID)

	response.OK(w, map[string]interface{}{
		"token":      newToken,
		"csrf_token": csrfToken,
		"expires_in": int(auth.AccessTokenDuration.Seconds()),
	})
}

// getUserByID is a helper to get user data by ID.
func (h *Handlers) getUserByID(userID string) (map[string]interface{}, error) {
	var username, email, displayName string
	var enabled, mustChangePassword bool
	var lastLoginAt, createdAt, updatedAt sql.NullString

	err := h.db.QueryRow(`
		SELECT username, email, display_name, enabled, must_change_password,
			last_login_at, created_at, updated_at
		FROM users WHERE id = ?`, userID,
	).Scan(&username, &email, &displayName, &enabled, &mustChangePassword,
		&lastLoginAt, &createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	user := map[string]interface{}{
		"id":                   userID,
		"username":             username,
		"email":                email,
		"display_name":         displayName,
		"enabled":              enabled,
		"must_change_password": mustChangePassword,
	}

	if lastLoginAt.Valid {
		user["last_login_at"] = lastLoginAt.String
	}
	if createdAt.Valid {
		user["created_at"] = createdAt.String
	}
	if updatedAt.Valid {
		user["updated_at"] = updatedAt.String
	}

	return user, nil
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

// getClientIP extracts the client IP from the request.
// Uses r.RemoteAddr as the primary source (always trustworthy).
// Only considers X-Forwarded-For / X-Real-IP if the request comes from a trusted proxy.
// For now, we log a warning when these headers are present but not from a trusted proxy.
func getClientIP(r *http.Request) string {
	// Always trust RemoteAddr as it is set by the server.
	remoteAddr := r.RemoteAddr

	// Check if X-Forwarded-For or X-Real-IP headers are present.
	xff := r.Header.Get("X-Forwarded-For")
	xri := r.Header.Get("X-Real-IP")

	if xff != "" || xri != "" {
		// Log a warning: these headers are present but we cannot trust them
		// without verifying the request comes from a trusted proxy.
		// In a future version, a trusted proxy IP list should be configured.
		slog.Warn("X-Forwarded-For or X-Real-IP header present but not from a verified trusted proxy; using RemoteAddr",
			"remote_addr", remoteAddr,
			"x_forwarded_for", xff,
			"x_real_ip", xri,
		)
	}

	// Strip the port from RemoteAddr (e.g. "192.168.1.10:52344") so that
	// per-IP rate limiting keys are stable across connections. Without this,
	// every new TCP connection gets a different key and brute-force locking
	// never triggers.
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// RemoteAddr may already be a bare IP (or a malformed address);
		// fall back to the raw value.
		host = remoteAddr
	}

	return host
}
