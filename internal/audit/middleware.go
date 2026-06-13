package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jasonwa/goddi/internal/rbac"
)

// allowedDetailFields is the whitelist of body field names that the audit
// middleware is allowed to retain in its detail string. Any field not on
// this list is dropped before the detail is persisted. This avoids
// accidentally logging passwords, tokens, or other secrets that an API
// handler may accept in the same request body.
var allowedDetailFields = map[string]bool{
	"name":        true,
	"description": true,
	"enabled":     true,
	"type":        true,
	"ttl":         true,
	"value":       true,
	"ip_address":  true,
	"mac_address": true,
	"action":      true,
}

// routeResourceMap maps a request path prefix (the first segment after
// /api/v1/) to the singular resource type used in audit log entries. The
// keys must be kept in sync with the routes registered in
// internal/api/router.go.
var routeResourceMap = map[string]string{
	"users":                  "user",
	"groups":                 "group",
	"roles":                  "role",
	"permissions":            "permission",
	"tokens":                 "token",
	"logs":                   "log",
	"audit-logs":             "audit",
	"auth":                   "auth",
	"dns":                    "dns",
	"dhcp":                   "dhcp",
	"ipam":                   "ipam",
	"backup":                 "backup",
	"settings":               "settings",
	"tasks":                  "task",
	"dashboard":              "dashboard",
	"system":                 "system",
	"sso":                    "sso",
	"cluster":                "cluster",
	"apps":                   "app",
	"addresses":              "address",
	"client-policies":        "client_policy",
	"block-lists":            "block_list",
	"allowlists":             "allowlist",
	"conditional-forwarders": "conditional_forwarder",
	"forwarders":             "forwarder",
	"leases":                 "lease",
	"reservations":           "reservation",
	"scopes":                 "scope",
	"options":                "option",
	"spaces":                 "space",
	"subnets":                "subnet",
	"records":                "record",
	"zones":                  "zone",
}

// AuditMiddleware automatically logs all write operations (POST, PUT, PATCH, DELETE).
func AuditMiddleware(auditMgr *AuditManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only audit write operations
			method := r.Method
			if method != http.MethodPost && method != http.MethodPut &&
				method != http.MethodPatch && method != http.MethodDelete {
				next.ServeHTTP(w, r)
				return
			}

			// Extract user info from context
			ctx := r.Context()
			userID := rbac.GetUserID(ctx)
			username := rbac.GetUsername(ctx)

			// Determine action and resource type from the request
			action := methodToAction(method)
			resourceType, resourceID := parseResourceFromPath(r.URL.Path)

			// Read a summary of the request body (limited for security).
			// r.Body may already have been wrapped with http.MaxBytesReader
			// by the router; if not, fall back to a 1 MiB cap here so the
			// middleware is safe to use standalone as well.
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			detail := summarizeBody(r)

			// Get source IP and user agent
			sourceIP := getClientIP(r)
			userAgent := r.UserAgent()

			// Wrap the ResponseWriter to capture the status code.
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Process the request
			next.ServeHTTP(ww, r)

			// Determine success based on HTTP status code.
			success := ww.Status() < 400

			// Log the audit entry asynchronously (fire and forget)
			entry := LogEntry{
				UserID:       userID,
				Username:     username,
				Action:       action,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				Detail:       detail,
				SourceIP:     sourceIP,
				UserAgent:    userAgent,
				Success:      success,
			}

			// Best effort logging - don't block the response
			_ = auditMgr.Log(entry)
		})
	}
}

// methodToAction converts an HTTP method to an audit action.
func methodToAction(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

// parseResourceFromPath extracts the resource type and ID from the URL path
// using the explicit routeResourceMap. Unknown prefixes fall back to the
// raw segment (without trailing 's') for forward compatibility, but the
// map is the source of truth.
func parseResourceFromPath(path string) (resourceType, resourceID string) {
	trimmed := strings.TrimPrefix(path, "/api/v1/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}

	// First segment is the resource. Allow longer compound segments to be
	// matched as a whole before falling back to a strip-trailing-'s'
	// heuristic.
	first := parts[0]
	if mapped, ok := routeResourceMap[first]; ok {
		resourceType = mapped
	} else {
		// Fallback: drop a single trailing 's' for plurals.
		if strings.HasSuffix(first, "s") {
			resourceType = first[:len(first)-1]
		} else {
			resourceType = first
		}
	}

	if len(parts) >= 2 && parts[1] != "" {
		// If the second segment is itself a known resource prefix, we
		// have a nested route (e.g. /dns/zones/{id}/records). The "ID"
		// then lives in the third position.
		if _, nested := routeResourceMap[parts[1]]; nested {
			if len(parts) >= 3 {
				resourceID = parts[2]
			}
		} else {
			resourceID = parts[1]
		}
	}

	return resourceType, resourceID
}

// summarizeBody reads a small JSON request body for audit logging, then
// restores it. Only fields on the allowedDetailFields whitelist are
// retained; everything else is dropped. If the body is not valid JSON the
// raw prefix is returned so administrators still have something to look
// at in the audit log.
func summarizeBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}

	const maxAuditBodyBytes = 1 << 20 // 1 MiB upper bound for audit
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAuditBodyBytes))
	if err != nil {
		return ""
	}
	// Restore the body for downstream handlers.
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Try to parse as a JSON object and emit only the whitelisted fields.
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err == nil {
		return encodeWhitelistedDetail(obj)
	}

	// Fall back to a raw text snippet.
	const maxSnippet = 256
	snippet := string(body)
	if len(snippet) > maxSnippet {
		snippet = snippet[:maxSnippet] + "..."
	}
	return snippet
}

// encodeWhitelistedDetail returns a compact, key=value representation of
// only the allowed fields in the supplied JSON object. Fields not on the
// whitelist are omitted entirely so that secrets never reach the log.
func encodeWhitelistedDetail(obj map[string]interface{}) string {
	var b strings.Builder
	first := true
	for _, k := range sortedKeys(obj) {
		if !allowedDetailFields[k] {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(stringifyDetailValue(obj[k]))
	}
	return b.String()
}

func stringifyDetailValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// json.Unmarshal always decodes numbers into float64.
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	default:
		// Fall back to JSON encoding for arrays/objects.
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Stable, deterministic order so the same input produces the same
	// audit detail string.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

// getClientIP extracts the client IP from r.RemoteAddr. It uses
// net.SplitHostPort so that IPv6 addresses (e.g. "[::1]:8080") are split
// correctly. The previous implementation used LastIndex(":") which truncated
// the IP at the first colon and produced garbage for IPv6 clients.
func getClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may already be a bare host (no port); fall back to
		// the raw value.
		return r.RemoteAddr
	}
	return host
}
