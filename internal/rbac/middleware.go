package rbac

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jasonwa/goddi/internal/auth"
)

// Context keys for storing user information.
type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	UsernameKey  contextKey = "username"
	RoleIDsKey   contextKey = "role_ids"
	SessionIDKey contextKey = "session_id"
	ClaimsKey    contextKey = "claims"
	APITokenKey  contextKey = "api_token"
)

// WithUserID adds the user ID to the context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// WithUsername adds the username to the context.
func WithUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, UsernameKey, username)
}

// WithRoleIDs adds the role IDs to the context.
func WithRoleIDs(ctx context.Context, roleIDs []string) context.Context {
	return context.WithValue(ctx, RoleIDsKey, roleIDs)
}

// WithSessionID adds the session ID to the context.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, SessionIDKey, sessionID)
}

// WithClaims adds the JWT claims to the context.
func WithClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, ClaimsKey, claims)
}

// WithAPIToken adds the validated API token to the context.
func WithAPIToken(ctx context.Context, token *auth.APIToken) context.Context {
	return context.WithValue(ctx, APITokenKey, token)
}

// GetUserID retrieves the user ID from the context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}

// GetUsername retrieves the username from the context.
func GetUsername(ctx context.Context) string {
	if v, ok := ctx.Value(UsernameKey).(string); ok {
		return v
	}
	return ""
}

// GetRoleIDs retrieves the role IDs from the context.
func GetRoleIDs(ctx context.Context) []string {
	if v, ok := ctx.Value(RoleIDsKey).([]string); ok {
		return v
	}
	return nil
}

// GetSessionID retrieves the session ID from the context.
func GetSessionID(ctx context.Context) string {
	if v, ok := ctx.Value(SessionIDKey).(string); ok {
		return v
	}
	return ""
}

// GetClaims retrieves the JWT claims from the context.
func GetClaims(ctx context.Context) *auth.Claims {
	if v, ok := ctx.Value(ClaimsKey).(*auth.Claims); ok {
		return v
	}
	return nil
}

// GetAPIToken retrieves the validated API token from the context.
func GetAPIToken(ctx context.Context) *auth.APIToken {
	if v, ok := ctx.Value(APITokenKey).(*auth.APIToken); ok {
		return v
	}
	return nil
}

// RequirePermission returns middleware that checks if the user has the specified permission.
func RequirePermission(rbacMgr *RBACManager, resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				writeJSONError(w, http.StatusUnauthorized, 401, "未认证")
				return
			}

			if apiToken := GetAPIToken(r.Context()); apiToken != nil && !auth.TokenScopeAllows(apiToken.Scope, resource, action) {
				writeJSONError(w, http.StatusForbidden, 403, "API令牌范围不足")
				return
			}

			allowed, err := rbacMgr.CheckPermission(userID, resource, action)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, 500, "权限检查失败")
				return
			}

			if !allowed {
				writeJSONError(w, http.StatusForbidden, 403, "权限不足")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole returns middleware that checks if the user has the specified role.
func RequireRole(rbacMgr *RBACManager, roleName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				writeJSONError(w, http.StatusUnauthorized, 401, "未认证")
				return
			}

			roles, err := rbacMgr.GetUserRoles(userID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, 500, "角色检查失败")
				return
			}

			hasRole := false
			for _, role := range roles {
				if role.Name == roleName {
					hasRole = true
					break
				}
			}

			if !hasRole {
				writeJSONError(w, http.StatusForbidden, 403, "角色不足")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeJSONError writes a JSON error response without importing the api package
// (to avoid circular imports).
func writeJSONError(w http.ResponseWriter, statusCode int, code int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    code,
		"message": message,
	})
}
