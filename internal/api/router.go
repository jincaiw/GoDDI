package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jasonwa/goddi/internal/api/handler"
	"github.com/jasonwa/goddi/internal/api/middleware"
	"github.com/jasonwa/goddi/internal/audit"
	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/metrics"
	"github.com/jasonwa/goddi/internal/rbac"
)

// NewRouter creates and configures the main API router.
func NewRouter(cfg *config.Config, db *database.DB) http.Handler {
	r := chi.NewRouter()

	// Create handlers with dependencies.
	h := handler.NewHandlers(db.DB, cfg.Security.JWTSecret, cfg.Security.EncryptionKey, cfg.Security.LoginRateLimit, cfg.Security.LoginRateWindow)

	// Create RBAC manager for middleware.
	rbacMgr := rbac.NewRBACManager(db.DB)

	// Create audit manager for middleware.
	auditMgr := audit.NewAuditManager(db.DB)

	// Create JWT manager for middleware.
	jwtMgr, _ := auth.NewJWTManager(cfg.Security.JWTSecret)

	// Create session manager for JWT session validation.
	sessMgr := auth.NewSessionManager(db.DB)
	tokenMgr := auth.NewTokenManager(db.DB)

	// Global middleware.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(middleware.CORS(cfg))

	// Limit request body size to 10MB to prevent OOM attacks.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
			next.ServeHTTP(w, r)
		})
	})

	// Health check.
	r.Get("/health", handler.Health)

	// Prometheus metrics endpoint.
	if cfg.Metrics.Enabled {
		r.Handle(cfg.Metrics.Path, metrics.PrometheusHandler())
	}

	// API v1 routes.
	r.Route("/api/v1", func(r chi.Router) {
		// Public routes (no auth required).
		r.Group(func(r chi.Router) {
			r.Post("/auth/init", h.InitAdmin)
			r.Post("/auth/login", h.Login)
			r.Post("/auth/refresh", h.RefreshToken)
		})

		// Authenticated routes.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authentication(jwtMgr, sessMgr, tokenMgr, db.DB))
			r.Use(middleware.CSRFProtection(cfg))

			// Auth.
			r.Post("/auth/logout", h.Logout)
			r.Get("/auth/me", h.GetCurrentUser)
			r.Post("/auth/change-password", h.ChangePassword)
			r.Post("/auth/totp/setup", h.SetupTOTP)
			r.Post("/auth/totp/verify", h.VerifyAndEnableTOTP)
			r.Post("/auth/totp/disable", h.DisableTOTPHandler)
			r.Get("/auth/sessions", h.ListSessions)
			r.Delete("/auth/sessions/{id}", h.DeleteSession)

			// Users - with RBAC.
			r.Route("/users", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "user", "read")).Get("/", h.ListUsers)
				r.With(rbac.RequirePermission(rbacMgr, "user", "write")).Post("/", h.CreateUser)
				r.With(rbac.RequirePermission(rbacMgr, "user", "read")).Get("/{id}", h.GetUser)
				r.With(rbac.RequirePermission(rbacMgr, "user", "write")).Put("/{id}", h.UpdateUser)
				r.With(rbac.RequirePermission(rbacMgr, "user", "delete")).Delete("/{id}", h.DeleteUser)
				r.With(rbac.RequirePermission(rbacMgr, "user", "write")).Post("/{id}/roles", h.AssignUserRoles)
				r.With(rbac.RequirePermission(rbacMgr, "user", "write")).Delete("/{id}/roles/{roleId}", h.RemoveUserRole)
				r.With(rbac.RequirePermission(rbacMgr, "user", "write")).Post("/{id}/groups", h.AssignUserGroup)
				r.With(rbac.RequirePermission(rbacMgr, "user", "write")).Delete("/{id}/groups/{groupId}", h.RemoveUserGroup)
			})

			// Groups - with RBAC.
			r.Route("/groups", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "group", "read")).Get("/", h.ListGroups)
				r.With(rbac.RequirePermission(rbacMgr, "group", "write")).Post("/", h.CreateGroup)
				r.With(rbac.RequirePermission(rbacMgr, "group", "read")).Get("/{id}", h.GetGroup)
				r.With(rbac.RequirePermission(rbacMgr, "group", "write")).Put("/{id}", h.UpdateGroup)
				r.With(rbac.RequirePermission(rbacMgr, "group", "delete")).Delete("/{id}", h.DeleteGroup)
				r.With(rbac.RequirePermission(rbacMgr, "group", "write")).Post("/{id}/roles", h.AssignGroupRoles)
				r.With(rbac.RequirePermission(rbacMgr, "group", "write")).Delete("/{id}/roles/{roleId}", h.RemoveGroupRole)
			})

			// Roles - with RBAC.
			r.Route("/roles", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "role", "read")).Get("/", h.ListRoles)
				r.With(rbac.RequirePermission(rbacMgr, "role", "write")).Post("/", h.CreateRole)
				r.With(rbac.RequirePermission(rbacMgr, "role", "read")).Get("/{id}", h.GetRole)
				r.With(rbac.RequirePermission(rbacMgr, "role", "write")).Put("/{id}", h.UpdateRole)
				r.With(rbac.RequirePermission(rbacMgr, "role", "delete")).Delete("/{id}", h.DeleteRole)
				r.With(rbac.RequirePermission(rbacMgr, "role", "write")).Post("/{id}/permissions", h.AssignRolePermissions)
				r.With(rbac.RequirePermission(rbacMgr, "role", "write")).Delete("/{id}/permissions/{permId}", h.RemoveRolePermission)
			})

			// Permissions.
			r.Route("/permissions", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "role", "read")).Get("/", h.ListPermissionsHandler)
			})

			// API Tokens - with RBAC.
			r.Route("/tokens", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "token", "read")).Get("/", h.ListAPITokens)
				r.With(rbac.RequirePermission(rbacMgr, "token", "write")).Post("/", h.CreateAPIToken)
				r.With(rbac.RequirePermission(rbacMgr, "token", "delete")).Delete("/{id}", h.DeleteAPIToken)
			})

			// Audit Logs - with audit middleware.
			r.Route("/logs", func(r chi.Router) {
				r.With(audit.AuditMiddleware(auditMgr)).With(rbac.RequirePermission(rbacMgr, "audit", "read")).Get("/audit", h.ListAuditLogs)
				r.With(rbac.RequirePermission(rbacMgr, "audit", "read")).Get("/login", h.ListLoginHistory)
			})

			// DNS Zones.
			r.Route("/dns/zones", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.ListDNSZones)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/", handler.CreateDNSZone)
				r.Route("/{id}", func(r chi.Router) {
					r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.GetDNSZone)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Put("/", handler.UpdateDNSZone)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/", handler.DeleteDNSZone)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/import", handler.ImportZoneFile)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/export", handler.ExportZoneFile)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/sync", handler.SyncSecondaryZone)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/dnssec", handler.GetDNSSECStatus)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/dnssec/enable", handler.EnableDNSSEC)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/dnssec/disable", handler.DisableDNSSEC)
					r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/dnssec/rotate", handler.RotateDNSSECKeys)
				})
			})

			// DNS Records.
			r.Route("/dns/zones/{zoneId}/records", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.ListDNSRecords)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/", handler.CreateDNSRecord)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/{id}", handler.GetDNSRecord)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Put("/{id}", handler.UpdateDNSRecord)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{id}", handler.DeleteDNSRecord)
			})

			// DNS Records batch operations.
			r.Route("/dns/records", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/batch", handler.BatchCreateRecords)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/batch", handler.BatchDeleteRecords)
			})

			// DNS Forwarders.
			r.Route("/dns/forwarders", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.ListDNSForwarders)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/", handler.CreateDNSForwarder)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Put("/{id}", handler.UpdateDNSForwarder)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{id}", handler.DeleteDNSForwarder)
			})

			// DNS Conditional Forwarders.
			r.Route("/dns/conditional-forwarders", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.ListConditionalForwarders)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/", handler.CreateConditionalForwarder)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Put("/{id}", handler.UpdateConditionalForwarder)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{id}", handler.DeleteConditionalForwarder)
			})

			// DNS Cache.
			r.Route("/dns/cache", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.GetDNSCacheStats)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/", handler.FlushDNSCache)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{name}/{type}", handler.FlushDNSCacheEntry)
			})

			// DNS Client (debug query).
			r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Post("/dns/client", handler.ExecuteDNSQuery)

			// DNS Security - Block Lists.
			r.Route("/dns/security/blocklists", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.ListBlockLists)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/", handler.CreateBlockList)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/{id}", handler.GetBlockList)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Put("/{id}", handler.UpdateBlockList)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{id}", handler.DeleteBlockList)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/{id}/rules", handler.ListBlockRules)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/{id}/rules", handler.AddBlockRule)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{listId}/rules/{ruleId}", handler.DeleteBlockRule)
			})

			// DNS Security - Allow Lists.
			r.Route("/dns/security/allowlists", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.ListAllowRules)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/", handler.AddAllowRule)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{id}", handler.DeleteAllowRule)
			})

			// DNS Security - Client Policies.
			r.Route("/dns/security/policies", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/", handler.ListClientPolicies)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Post("/", handler.CreateClientPolicy)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "write")).Put("/{id}", handler.UpdateClientPolicy)
				r.With(rbac.RequirePermission(rbacMgr, "dns", "delete")).Delete("/{id}", handler.DeleteClientPolicy)
			})

			// DNS Query Logs (new API).
			r.With(rbac.RequirePermission(rbacMgr, "dns", "read")).Get("/logs/dns", handler.ListDNSQueryLogs)

			// DNS Extension Points (501 Not Implemented).
			r.Route("/dns/listeners", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Put("/dot", handler.ConfigureDoT)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Put("/doh", handler.ConfigureDoH)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Put("/doq", handler.ConfigureDoQ)
			})

			// DHCP Scopes.
			r.Route("/dhcp/scopes", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/", handler.ListDHCPScopes)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "write")).Post("/", handler.CreateDHCPScope)
				r.Route("/{id}", func(r chi.Router) {
					r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/", handler.GetDHCPScope)
					r.With(rbac.RequirePermission(rbacMgr, "dhcp", "write")).Put("/", handler.UpdateDHCPScope)
					r.With(rbac.RequirePermission(rbacMgr, "dhcp", "delete")).Delete("/", handler.DeleteDHCPScope)
				})
			})

			// DHCP Leases.
			r.Route("/dhcp/leases", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/", handler.ListDHCPLeases)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/{id}", handler.GetDHCPLease)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "delete")).Delete("/{id}", handler.DeleteDHCPLease)
			})

			// DHCP Reservations.
			r.Route("/dhcp/reservations", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/", handler.ListDHCPReservations)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "write")).Post("/", handler.CreateDHCPReservation)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/{id}", handler.GetDHCPReservation)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "write")).Put("/{id}", handler.UpdateDHCPReservation)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "delete")).Delete("/{id}", handler.DeleteDHCPReservation)
			})

			// DHCP Options.
			r.Route("/dhcp/options", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/", handler.ListDHCPOptions)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "write")).Post("/", handler.CreateDHCPOption)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "write")).Put("/{id}", handler.UpdateDHCPOption)
				r.With(rbac.RequirePermission(rbacMgr, "dhcp", "delete")).Delete("/{id}", handler.DeleteDHCPOption)
			})

			// DHCP HA Extension (501).
			r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/dhcp/ha", handler.GetDHCPHAConfig)

			// DHCP Logs.
			r.With(rbac.RequirePermission(rbacMgr, "dhcp", "read")).Get("/logs/dhcp", handler.ListDHCPLogs)

			// IPAM Spaces.
			r.Route("/ipam/spaces", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/", handler.ListIPAMSpaces)
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Post("/", handler.CreateIPAMSpace)
				r.Route("/{id}", func(r chi.Router) {
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/", handler.GetIPAMSpace)
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Put("/", handler.UpdateIPAMSpace)
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "delete")).Delete("/", handler.DeleteIPAMSpace)
				})
			})

			// IPAM Subnets.
			r.Route("/ipam/subnets", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/", handler.ListIPAMSubnets)
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Post("/", handler.CreateIPAMSubnet)
				r.Route("/{id}", func(r chi.Router) {
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/", handler.GetIPAMSubnet)
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Put("/", handler.UpdateIPAMSubnet)
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "delete")).Delete("/", handler.DeleteIPAMSubnet)
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/stats", handler.GetIPAMSubnetStats)
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Post("/generate-dhcp-scope", handler.GenerateDHCPScope)
					r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Post("/generate-reverse-zone", handler.GenerateReverseZone)
				})
			})

			// IPAM Addresses.
			r.Route("/ipam/addresses", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/", handler.ListIPAMAddresses)
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/{id}", handler.GetIPAMAddress)
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Put("/{id}", handler.UpdateIPAMAddress)
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Post("/allocate", handler.AllocateIP)
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Post("/release", handler.ReleaseIP)
			})

			// IPAM Import/Export.
			r.Route("/ipam", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "write")).Post("/import", handler.ImportIPAMData)
				r.With(rbac.RequirePermission(rbacMgr, "ipam", "read")).Get("/export", handler.ExportIPAMData)
			})

			// Audit Logs (legacy route).
			r.Route("/audit-logs", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "audit", "read")).Get("/", h.ListAuditLogs)
			})

			// --- Phase 7: Dashboard, Backup, Settings, Tasks ---

			// Dashboard.
			r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/dashboard", handler.GetDashboard)

			// Backup.
			r.Route("/backup", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "backup", "read")).Get("/", handler.ListBackupsHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "write")).Post("/", handler.CreateBackupHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "read")).Get("/{id}", handler.GetBackupHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "write")).Post("/{id}/restore", handler.RestoreBackupHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "write")).Delete("/{id}", handler.DeleteBackupHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "read")).Get("/{id}/download", handler.DownloadBackupHandler)
			})

			// System Settings.
			r.Route("/settings", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/", handler.ListSettingsHandler)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Put("/", handler.UpdateSettingsHandler)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Put("/{key}", handler.UpdateSingleSettingHandler)
			})

			// Tasks.
			r.Route("/tasks", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/", handler.ListTasksHandler)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/{id}", handler.GetTaskHandler)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Post("/{id}/cancel", handler.CancelTaskHandler)
			})

			// --- Phase 8: Extension Points (501 Not Implemented) ---

			// SSO.
			r.Route("/sso", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/", handler.GetSSOSettings)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Put("/", handler.UpdateSSOSettings)
			})

			// Cluster.
			r.Route("/cluster", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/", handler.GetClusterNodes)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Post("/", handler.AddClusterNode)
			})

			// App Marketplace.
			r.Route("/apps", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/", handler.ListAppsHandler)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Post("/{id}/install", handler.InstallAppHandler)
			})

			// Legacy system routes (backward compatibility).
			r.Route("/system/settings", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "settings", "read")).Get("/", handler.ListSettingsHandler)
				r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Put("/{key}", handler.UpdateSingleSettingHandler)
			})

			r.Route("/system/backups", func(r chi.Router) {
				r.With(rbac.RequirePermission(rbacMgr, "backup", "read")).Get("/", handler.ListBackupsHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "write")).Post("/", handler.CreateBackupHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "write")).Post("/{id}/restore", handler.RestoreBackupHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "write")).Delete("/{id}", handler.DeleteBackupHandler)
				r.With(rbac.RequirePermission(rbacMgr, "backup", "read")).Get("/{id}/download", handler.DownloadBackupHandler)
			})
		})
	})

	// Static frontend assets (catch-all). Registered after the API routes
	// so any /api/*, /health, /metrics, etc. requests are dispatched by
	// their dedicated handlers. The static handler falls back to
	// index.html for SPA routes (e.g. /login, /dashboard) and serves
	// files from ./web/dist (or the equivalent install path) directly.
	staticHandler := newStaticHandler()
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		staticHandler.ServeHTTP(w, req)
	})

	return r
}
