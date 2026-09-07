package middleware

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/rbac"
)

// sensitiveQueryKeys are lower-case query parameter names whose values must
// be redacted from the access log to avoid leaking secrets in URLs.
var sensitiveQueryKeys = map[string]bool{
	"token":    true,
	"key":      true,
	"password": true,
	"secret":   true,
	"api_key":  true,
	"apikey":   true,
}

// redactSensitiveQuery masks the values of any query parameters whose names
// are considered sensitive so they never appear in access logs.
func redactSensitiveQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "[redacted:invalid-query]"
	}
	changed := false
	for k := range values {
		if sensitiveQueryKeys[strings.ToLower(k)] {
			values.Set(k, "[REDACTED]")
			changed = true
		}
	}
	if !changed {
		return rawQuery
	}
	return values.Encode()
}

// Logger is a structured request logging middleware.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			slog.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"query", redactSensitiveQuery(r.URL.RawQuery),
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"request_id", middleware.GetReqID(r.Context()),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}

// CORS adds Cross-Origin Resource Sharing headers with proper whitelist validation.
func CORS(cfg *config.Config) func(http.Handler) http.Handler {
	// Build the whitelist from the configured public_url.
	allowedOrigins := buildCORSWhitelist(cfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Not a cross-origin request; pass through.
				next.ServeHTTP(w, r)
				return
			}

			matchedOrigin := ""
			if isOriginAllowed(origin, allowedOrigins) {
				matchedOrigin = origin
			}

			if matchedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", matchedOrigin)
				// Vary: Origin is required so that caches and downstream
				// proxies do not serve a response produced for one origin
				// to a request from a different origin.
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}
			// If origin is not in the whitelist, do NOT set any CORS headers.

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// buildCORSWhitelist builds the list of allowed origins from config.
// In development mode (localhost/127.0.0.1), all localhost origins on any
// port are allowed. Production mode (GODDI_ENV=production) NEVER enables
// the wildcard localhost dev convenience.
func buildCORSWhitelist(cfg *config.Config) []string {
	origins := []string{cfg.Server.PublicURL}

	// Production environments must never get the dev-mode wildcard.
	if os.Getenv("GODDI_ENV") == "production" {
		return origins
	}

	// Parse PublicURL and compare hostnames. Only treat as "dev" when the
	// configured origin's hostname is exactly localhost or an IPv4 loopback
	// (127.0.0.1). This avoids accidentally enabling dev mode for a
	// hostname that merely contains the substring "localhost".
	pubURL := cfg.Server.PublicURL
	parsed, err := url.Parse(pubURL)
	isDev := false
	if err == nil && parsed.Hostname() != "" {
		host := parsed.Hostname()
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			isDev = true
		}
	}
	if isDev {
		// Allow any localhost origin on any port in development.
		for _, scheme := range []string{"http", "https"} {
			for _, host := range []string{"localhost", "127.0.0.1"} {
				origins = append(origins, scheme+"://"+host)
			}
		}
	}

	return origins
}

// isOriginAllowed checks whether the given origin matches any entry in the whitelist.
// For development mode entries (without port), any port on that host is accepted.
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	originLower := strings.ToLower(origin)

	for _, allowed := range allowedOrigins {
		allowedLower := strings.ToLower(allowed)

		// Exact match.
		if originLower == allowedLower {
			return true
		}

		// Prefix match for development mode: if the allowed entry is a scheme+host
		// WITHOUT a port (e.g. "http://localhost"), then any port on that host is
		// acceptable. If the allowed entry specifies a port, it must match exactly —
		// otherwise any same-host service on a different port would be trusted
		// while Allow-Credentials is on.
		parsed, err := url.Parse(originLower)
		parsedAllowed, errA := url.Parse(allowedLower)
		if err == nil && errA == nil {
			if parsed.Scheme == parsedAllowed.Scheme && parsed.Hostname() == parsedAllowed.Hostname() {
				if parsedAllowed.Port() == "" || parsed.Port() == parsedAllowed.Port() {
					return true
				}
			}
		}
	}

	return false
}

// JWTAuth validates JWT tokens and sets user context.
// If sessionMgr is not nil, it also validates that the session referenced by
// the token still exists and has not expired (prevents use of JWT after logout).
func JWTAuth(jwtMgr *auth.JWTManager, sessionMgr *auth.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				writeAuthError(w, "缺少认证令牌")
				return
			}

			claims, err := jwtMgr.ParseToken(token)
			if err != nil {
				writeAuthError(w, "无效的认证令牌")
				return
			}

			// Validate session if a SessionManager is provided.
			if sessionMgr != nil && claims.SessionID != "" {
				session, err := sessionMgr.GetSessionByID(claims.SessionID)
				if err != nil {
					writeAuthError(w, "会话不存在或已失效")
					return
				}
				if time.Now().After(session.ExpiresAt) {
					_ = sessionMgr.DeleteSession(claims.SessionID)
					writeAuthError(w, "会话已过期")
					return
				}
			}

			// Set user context
			ctx := r.Context()
			ctx = rbac.WithUserID(ctx, claims.UserID)
			ctx = rbac.WithUsername(ctx, claims.Username)
			ctx = rbac.WithRoleIDs(ctx, claims.RoleIDs)
			ctx = rbac.WithSessionID(ctx, claims.SessionID)
			ctx = rbac.WithClaims(ctx, claims)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Authentication accepts either a session-backed JWT or a GoDDI API token.
// API tokens are header credentials, so validated API-token requests do not
// need browser CSRF tokens. Read-only tokens are rejected for mutations.
func Authentication(jwtMgr *auth.JWTManager, sessionMgr *auth.SessionManager, tokenMgr *auth.TokenManager, db *sql.DB) func(http.Handler) http.Handler {
	jwtAuth := JWTAuth(jwtMgr, sessionMgr)

	return func(next http.Handler) http.Handler {
		jwtHandler := jwtAuth(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if !strings.HasPrefix(token, "goddi_") {
				jwtHandler.ServeHTTP(w, r)
				return
			}

			apiToken, err := tokenMgr.ValidateAPIToken(token, requestIP(r))
			if err != nil {
				writeAuthError(w, "无效的API令牌")
				return
			}
			if apiToken.IsReadonly && isMutationMethod(r.Method) {
				writeForbiddenError(w, "只读API令牌不能执行修改操作")
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") && r.URL.Path != "/api/v1/auth/me" {
				writeForbiddenError(w, "账户安全操作需要登录会话")
				return
			}

			var username string
			var enabled bool
			if err := db.QueryRow(`SELECT username, enabled FROM users WHERE id = ?`, apiToken.UserID).Scan(&username, &enabled); err != nil || !enabled {
				writeAuthError(w, "API令牌所属用户不存在或已禁用")
				return
			}

			ctx := r.Context()
			ctx = rbac.WithUserID(ctx, apiToken.UserID)
			ctx = rbac.WithUsername(ctx, username)
			ctx = rbac.WithAPIToken(ctx, apiToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requestIP(r *http.Request) string {
	host := r.RemoteAddr
	if parsedHost, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = parsedHost
	}
	return host
}

// csrfKey derives a deterministic, cryptographically-separated CSRF key from
// the JWT secret using HKDF-SHA256. This avoids any direct reuse of the JWT
// signing key for HMAC verification of CSRF tokens.
func csrfKey(jwtSecret string) []byte {
	key, err := hkdf.Key(sha256.New, []byte(jwtSecret), []byte("goddi-csrf-v1"), "csrf-auth", 32)
	if err != nil {
		// hkdf.Key should not fail with our parameters; fall back to a
		// SHA-256 derived key to avoid panicking.
		sum := sha256.Sum256([]byte(jwtSecret + "goddi-csrf-v1"))
		return sum[:]
	}
	return key
}

// isMutationMethod reports whether an HTTP method is state-changing and
// therefore must be protected by CSRF validation.
func isMutationMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// CSRFProtection provides CSRF protection for browser requests.
// It validates the X-CSRF-Token header using HMAC-based token generation
// derived from the JWT secret via HKDF-SHA256. The check is applied to
// every mutating method (POST/PUT/PATCH/DELETE) regardless of the
// authentication scheme, so an attacker cannot bypass it by attaching an
// Authorization: Bearer header to a forged request.
func CSRFProtection(cfg *config.Config) func(http.Handler) http.Handler {
	// Derive a CSRF-specific key with HKDF so the CSRF key is cryptographically
	// separated from the JWT signing key.
	csrfKeyBytes := csrfKey(cfg.Security.JWTSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF for safe methods.
			if !isMutationMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			if rbac.GetAPIToken(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}

			// All mutating requests (including those carrying an Authorization:
			// Bearer header) must present a valid CSRF token. This closes the
			// previous bypass where any request starting with "Bearer " was
			// allowed through without CSRF validation.
			csrfToken := r.Header.Get("X-CSRF-Token")
			if csrfToken == "" {
				slog.Warn("CSRF token missing for mutating request",
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
				)
				writeAuthError(w, "缺少CSRF令牌")
				return
			}

			// Validate the CSRF token using HMAC. The token is bound to the
			// authenticated session ID so a token issued for one session
			// cannot be replayed under another session (e.g. after logout
			// and re-login as a different user).
			if !validateCSRFToken(csrfKeyBytes, csrfToken, rbac.GetSessionID(r.Context())) {
				slog.Warn("Invalid CSRF token",
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
				)
				writeAuthError(w, "无效的CSRF令牌")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// validateCSRFToken validates a CSRF token using HMAC-SHA256.
// Token format: timestamp:signature
// The signature is HMAC-SHA256(key, sessionID+":"+timestamp) so tokens are
// bound to the authenticated session and cannot be replayed across sessions.
// An empty sessionID is tolerated for requests without a session claim
// (legacy tokens), in which case the binding material is simply empty.
func validateCSRFToken(key []byte, token, sessionID string) bool {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return false
	}

	timestampStr, signature := parts[0], parts[1]

	// Verify the HMAC signature over sessionID + ":" + timestamp.
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(sessionID))
	mac.Write([]byte(":"))
	mac.Write([]byte(timestampStr))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison to prevent timing attacks.
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSig)) != 1 {
		return false
	}

	// Parse and validate the timestamp (token valid for 24 hours).
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	if now-timestamp > 86400 || timestamp > now+300 { // 24h expiry, 5min clock skew
		return false
	}

	return true
}

// GenerateCSRFToken generates a new CSRF token for use in browser sessions.
// Token format: timestamp:HMAC-SHA256(key, sessionID+":"+timestamp)
// The token is bound to the given session ID; validateCSRFToken enforces the
// same binding during verification.
func GenerateCSRFToken(key []byte, sessionID string) string {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(sessionID))
	mac.Write([]byte(":"))
	mac.Write([]byte(timestamp))
	signature := hex.EncodeToString(mac.Sum(nil))
	return timestamp + ":" + signature
}

// extractToken extracts the JWT token from the Authorization header and
// performs basic shape validation to prevent unbounded input from being
// handed to the JWT parser.
func extractToken(r *http.Request) string {
	token := extractBearerToken(r)
	if token == "" {
		return ""
	}
	// Cap the token length to avoid handing a multi-megabyte blob to the
	// JWT parser. A typical HS256 JWT is well under 4 KiB.
	if len(token) > 4096 {
		return ""
	}
	// JWTs consist of three base64url segments separated by dots. Reject
	// anything that does not match the expected shape early.
	if strings.Count(token, ".") != 2 {
		return ""
	}
	return token
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	token := strings.TrimSpace(authHeader[len("Bearer "):])
	if token == "" || len(token) > 4096 {
		return ""
	}
	return token
}

// writeAuthError writes an authentication error response.
func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    401,
		"message": message,
	})
}

func writeForbiddenError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    403,
		"message": message,
	})
}
