package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/api"
	"github.com/jasonwa/goddi/internal/api/handler"
	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/backup"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/dhcp/option"
	"github.com/jasonwa/goddi/internal/dhcp/reservation"
	"github.com/jasonwa/goddi/internal/dhcp/scope"
	dhcpserver "github.com/jasonwa/goddi/internal/dhcp/server"
	dnsquerylog "github.com/jasonwa/goddi/internal/dns"
	"github.com/jasonwa/goddi/internal/dns/cache"
	"github.com/jasonwa/goddi/internal/dns/client"
	"github.com/jasonwa/goddi/internal/dns/dynamic_update"
	"github.com/jasonwa/goddi/internal/dns/filter"
	"github.com/jasonwa/goddi/internal/dns/forwarder"
	dnsserver "github.com/jasonwa/goddi/internal/dns/server"
	"github.com/jasonwa/goddi/internal/dns/transfer"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/space"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
	applog "github.com/jasonwa/goddi/internal/log"
	"github.com/jasonwa/goddi/internal/metrics"
	"github.com/jasonwa/goddi/internal/rbac"
	"github.com/jasonwa/goddi/internal/system"
	"github.com/jasonwa/goddi/internal/task"
	"github.com/spf13/cobra"
)

var (
	// Build information, set at compile time via ldflags.
	Version   = "0.5.2"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	var configPath string

	rootCmd := &cobra.Command{
		Use:   "goddi",
		Short: "GoDDI - Enterprise DDI Platform (DNS + DHCP + IPAM)",
		Long:  "GoDDI is an enterprise-grade DDI platform providing integrated DNS, DHCP, and IPAM services.",
	}

	// serve subcommand.
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the GoDDI server",
		Long:  "Start the GoDDI server with the specified configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(configPath)
		},
	}
	serveCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")

	// version subcommand.
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("GoDDI Version: %s\n", Version)
			fmt.Printf("Git Commit:    %s\n", GitCommit)
			fmt.Printf("Build Date:    %s\n", BuildDate)
		},
	}

	// migrate subcommand.
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Long:  "Run all pending database migrations.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrations(configPath)
		},
	}
	migrateCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")

	// unlock subcommand: clears login rate-limit lockouts. This exists as a
	// CLI command because a locked-out administrator cannot log in to use the
	// unlock API — the CLI is the only recovery path when lockout is total.
	var unlockUser, unlockIP string
	var unlockAll bool
	unlockCmd := &cobra.Command{
		Use:   "unlock",
		Short: "Clear login rate-limit lockouts",
		Long: "Clear login rate-limit lockouts created by repeated failed logins.\n" +
			"Use --user <name> to unlock one account on every source IP, --user <name> --ip <addr>\n" +
			"to scope it to a single IP, or --all to clear every lockout.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnlock(configPath, unlockUser, unlockIP, unlockAll)
		},
	}
	unlockCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	unlockCmd.Flags().StringVar(&unlockUser, "user", "", "Username to unlock")
	unlockCmd.Flags().StringVar(&unlockIP, "ip", "", "Restrict the unlock to this source IP (requires --user)")
	unlockCmd.Flags().BoolVar(&unlockAll, "all", false, "Clear every lockout for all users")

	rootCmd.AddCommand(serveCmd, versionCmd, migrateCmd, unlockCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runServer initializes and starts the GoDDI server.
func runServer(configPath string) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply environment variable overrides.
	config.ApplyEnvOverrides(cfg)

	// Validate configuration.
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Initialize logger.
	applog.InitLogger(cfg.Log.Level)
	slog.Info("GoDDI starting",
		"version", Version,
		"config", configPath,
	)

	// Initialize Prometheus metrics.
	metrics.InitMetrics()
	slog.Info("Prometheus metrics initialized")

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.Server.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory %s: %w", cfg.Server.DataDir, err)
	}

	// Initialize database.
	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer db.Close()

	if err := db.RunMigrationsFS(goddiassets.Migrations()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Initialize RBAC predefined data (roles, permissions).
	rbacMgr := rbac.NewRBACManager(db.DB)
	if err := rbacMgr.InitializePredefinedData(); err != nil {
		return fmt.Errorf("initializing RBAC data: %w", err)
	}

	// Auto-initialize admin user if needed.
	if err := auth.AutoInitializeAdmin(db.DB); err != nil {
		slog.Warn("auto-initialize admin failed", "error", err)
	}

	// Clean up expired sessions.
	sessMgr := auth.NewSessionManager(db.DB)
	if err := sessMgr.CleanupExpiredSessions(); err != nil {
		slog.Warn("cleanup expired sessions failed", "error", err)
	}

	// --- Initialize Task Manager ---
	taskMgr := task.NewManager(4)
	slog.Info("Task manager initialized", "workers", 4)

	// --- Initialize Backup Manager ---
	backupMgr := backup.NewManager(db.DB, cfg.Server.DataDir, Version)
	slog.Info("Backup manager initialized", "data_dir", cfg.Server.DataDir)

	// --- Initialize System Settings Manager ---
	settingsMgr := system.NewManager(db.DB)
	if err := settingsMgr.EnsureDefaults(); err != nil {
		slog.Warn("failed to ensure default settings", "error", err)
	}
	slog.Info("System settings manager initialized")

	// --- Initialize DNS Components ---

	// DNS Cache.
	var dnsCache *cache.Cache
	var persistentCache *cache.PersistentCache
	if cfg.Cache.Enabled {
		dnsCache = cache.New(cache.Config{
			Enabled:      cfg.Cache.Enabled,
			MaxEntries:   cfg.Cache.MaxEntries,
			MinTTL:       cfg.Cache.MinTTL,
			MaxTTL:       cfg.Cache.MaxTTL,
			NegativeTTL:  cfg.Cache.NegativeTTL,
			ServeStale:   cfg.Cache.ServeStale,
			StaleTTL:     cfg.Cache.StaleTTL,
			Prefetch:     cfg.Cache.Prefetch,
			AutoPrefetch: cfg.Cache.AutoPrefetch,
		})

		// Create PersistentCache if enabled.
		if cfg.Cache.Persistent {
			persistentCache = cache.NewPersistentCache(dnsCache, db.DB)
			slog.Info("DNS persistent cache initialized")
		}

		slog.Info("DNS cache initialized",
			"max_entries", cfg.Cache.MaxEntries,
			"serve_stale", cfg.Cache.ServeStale,
			"persistent", cfg.Cache.Persistent,
		)

		// Publish cache statistics to /metrics (C3): gauges are sampled
		// by the metrics ticker every 10 seconds.
		metrics.RegisterCacheStatsProvider(func() metrics.CacheStatsSample {
			st := dnsCache.Stats()
			return metrics.CacheStatsSample{
				Entries:    st.Entries,
				MaxEntries: st.MaxEntries,
				SizeBytes:  st.SizeBytes,
				Hits:       st.Hits,
				Misses:     st.Misses,
			}
		})
	}

	// DNS Filter Engine.
	filterEngine := filter.NewFilterEngine(cfg.Security.RebindingProtection)

	// Load filter data from database.
	if err := loadFilterData(db.DB, filterEngine); err != nil {
		slog.Warn("failed to load filter data from database", "error", err)
	}

	// Block list URL subscription fetcher (periodic refresh + manual API).
	blockListFetcher := filter.NewBlockListFetcher(db.DB, filterEngine.BlockListMgr)

	// DNS Forwarder Group.
	strategy := forwarder.SelectionStrategy(cfg.Forwarders.Mode)
	fwdGroup := forwarder.NewForwarderGroup(strategy, 5*time.Second)

	// Load forwarders from database.
	if err := loadForwardersFromDB(db.DB, fwdGroup); err != nil {
		slog.Warn("failed to load forwarders from database", "error", err)
	}

	// Also add config-file forwarders if no DB forwarders exist.
	if len(fwdGroup.GetForwarders()) == 0 {
		for i, s := range cfg.Forwarders.Servers {
			fwd := &forwarder.Forwarder{
				ID:       fmt.Sprintf("cfg-%d", i),
				Name:     s.Name,
				Protocol: s.Protocol,
				Address:  s.Address,
				Enabled:  true,
			}
			fwd.SetHealthy(true)
			if err := fwdGroup.AddForwarder(fwd); err != nil {
				slog.Warn("skipping invalid forwarder from config", "name", s.Name, "error", err)
			}
		}
	}

	// Conditional Forwarder Manager.
	condManager := forwarder.NewConditionalForwarderManager(fwdGroup)
	if err := loadConditionalForwardersFromDB(db.DB, condManager, fwdGroup); err != nil {
		slog.Warn("failed to load conditional forwarders from database", "error", err)
	}

	// DNS Query Logger.
	var queryLog *dnsquerylog.QueryLogger
	if cfg.Log.QueryLogEnabled {
		queryLog = dnsquerylog.NewQueryLogger(db.DB, cfg.Log.RetentionDays)
		slog.Info("DNS query logger initialized", "retention_days", cfg.Log.RetentionDays)

		// Publish query-log queue pressure and persistence outcomes to /metrics.
		metrics.RegisterQueryLogStatsProvider(func() metrics.QueryLogStatsSample {
			st := queryLog.Stats()
			return metrics.QueryLogStatsSample{
				QueueDepth:      st.QueueDepth,
				QueueCapacity:   st.QueueCapacity,
				DroppedFull:     st.DroppedFull,
				Written:         st.Written,
				BeginFailures:   st.BeginFailures,
				PrepareFailures: st.PrepareFailures,
				ExecFailures:    st.ExecFailures,
				CommitFailures:  st.CommitFailures,
				CleanupFailures: st.CleanupFailures,
				CleanupDeleted:  st.CleanupDeleted,
				CleanupRuns:     st.CleanupRuns,
				CleanupTimeouts: st.CleanupTimeouts,
			}
		})
	}

	// Publish database/sql pool pressure. Sampling Stats is lock-free and
	// captures all management-plane callers without SQL hot-path instrumentation.
	metrics.RegisterDBStatsProvider(func() metrics.DBStatsSample {
		st := db.DB.Stats()
		return metrics.DBStatsSample{
			OpenConnections:    st.OpenConnections,
			InUse:              st.InUse,
			Idle:               st.Idle,
			MaxOpenConnections: st.MaxOpenConnections,
			WaitCount:          st.WaitCount,
			WaitDuration:       st.WaitDuration,
		}
	})

	// DNS Client (for debug queries).
	dnsClient := client.NewDNSClient(5 * time.Second)

	// DNS Zone Store - load authoritative zones from database.
	zoneStore := zone.NewStore(db.DB)
	slog.Info("DNS zone store initialized", "zones", len(zoneStore.ZoneNames()))

	// Zone / record managers (dynamic updates + record aging + IXFR history).
	zoneMgr := zone.NewZoneManager(db.DB, zoneStore)
	recordMgr := zone.NewRecordManager(db.DB, zoneStore, zoneMgr)

	// DNS Server.
	var dnsSrv *dnsserver.Server
	var rateLimiter *dnsserver.RateLimiter
	if cfg.DNS.Enabled {
		dnsSrv = dnsserver.New(cfg, dnsCache, filterEngine, fwdGroup, condManager, queryLog, zoneStore)

		// Query rate limiting (QPS + RRL). Values can be hot-updated via
		// the dns_rate_limit_qps setting.
		if cfg.DNS.RateLimit.Enabled {
			rateLimiter = dnsserver.NewRateLimiter(
				int64(cfg.DNS.RateLimit.ClientQPS),
				int64(cfg.DNS.RateLimit.ClientBurst),
				int64(cfg.DNS.RateLimit.RRLThreshold),
			)
			dnsSrv.SetRateLimiter(rateLimiter)
		}

		// Zone transfer (AXFR/IXFR) serving.
		dnsSrv.SetAXFRHandler(transfer.NewAXFRHandler(db.DB))

		// RFC 2136 dynamic updates (TSIG-authenticated).
		updateHandler := dynamic_update.NewUpdateHandler(db.DB, zoneStore, zoneMgr, recordMgr)
		updateHandler.SetTSIGSecrets(cfg.DNS.DynamicUpdate.TSIGKeys)
		dnsSrv.SetUpdateHandler(updateHandler)

		// NOTIFY (RFC 1996): primary zones announce serial bumps to the
		// ACL notify targets; inbound NOTIFY triggers a secondary refresh.
		recordMgr.SetNotifyHook(func(zoneName string) {
			transfer.SendNotifyForZone(db.DB, zoneName)
		})
		secondarySync := transfer.NewSecondarySync(db.DB)
		dnsSrv.SetNotifyHandler(func(zoneName string) {
			if err := secondarySync.HandleNotify(zoneName); err != nil {
				slog.Warn("notify: secondary refresh failed", "zone", zoneName, "error", err)
			}
		})
	}

	// Cache prefetch: hot entries whose remaining TTL drops below the
	// threshold trigger a background re-resolution through the server's
	// normal forwarding path.
	if dnsCache != nil && dnsSrv != nil {
		dnsCache.SetPrefetchCallback(dnsSrv.Prefetch)
	}

	// RFC 8945 TSIG keys for signed zone transfers on TCP listeners.
	if dnsSrv != nil {
		if secrets := transfer.TSIGSecretMap(db.DB); secrets != nil {
			dnsSrv.SetTSIGSecretMap(secrets)
			slog.Info("TSIG keys loaded", "count", len(secrets))
		}
	}

	// Initialize API handler services.
	handler.InitDNSServices(&handler.DNSServiceContainer{
		DB:               db.DB,
		Cache:            dnsCache,
		PersistentCache:  persistentCache,
		Filter:           filterEngine,
		Forwarder:        fwdGroup,
		Conditional:      condManager,
		DNSClient:        dnsClient,
		ZoneStore:        zoneStore,
		BlockListFetcher: blockListFetcher,
		DNSServer:        dnsSrv,
		JWTSecret:        cfg.Security.JWTSecret,
		Config:           cfg,
	})

	// --- Initialize DHCP Components ---

	// DHCP Event Logger.
	var dhcpEventLogger *dhcpinternal.EventLogger
	if cfg.DHCP.Enabled {
		dhcpEventLogger = dhcpinternal.NewEventLogger(db.DB)
		slog.Info("DHCP event logger initialized")
	}

	// DHCP Server.
	var dhcpSrv *dhcpserver.Server
	if cfg.DHCP.Enabled {
		dhcpSrv = dhcpserver.New(db.DB, cfg.DHCP.Interfaces, dhcpEventLogger)
		slog.Info("DHCP server initialized", "interfaces", cfg.DHCP.Interfaces)
	}

	// Initialize DHCP API handler services.
	handler.InitDHCPServices(&handler.DHCPServiceContainer{
		DB:          db.DB,
		ScopeMgr:    scope.NewManager(db.DB),
		LeaseMgr:    lease.NewManager(db.DB),
		ReservMgr:   reservation.NewManager(db.DB),
		OptionMgr:   option.NewManager(db.DB),
		EventLogger: dhcpEventLogger,
	})

	// --- Initialize IPAM Components ---

	// Initialize IPAM API handler services.
	handler.InitIPAMServices(&handler.IPAMServiceContainer{
		DB:         db.DB,
		SpaceMgr:   space.NewManager(db.DB),
		SubnetMgr:  subnet.NewManager(db.DB),
		AddressMgr: address.NewManager(db.DB),
	})

	// --- Initialize System API handler services ---
	// Assigned after the runtime settings applier is constructed below. The
	// restore hook calls it to reconcile persisted settings with in-memory DNS
	// components after a successful config-section restore.
	var applyPersistedSettings func() error
	handler.InitSystemServices(&handler.SystemServiceContainer{
		DB:          db.DB,
		SettingsMgr: settingsMgr,
		BackupMgr:   backupMgr,
		TaskMgr:     taskMgr,
		Version:     Version,
		ReloadAfterRestore: func() error {
			zoneStore.Load()
			if dnsCache != nil {
				dnsCache.Flush()
			}
			if err := loadFilterData(db.DB, filterEngine); err != nil {
				return err
			}
			restored := forwarder.NewForwarderGroup(strategy, 5*time.Second)
			if err := loadForwardersFromDB(db.DB, restored); err != nil {
				return err
			}
			conditionals := forwarder.NewConditionalForwarderManager(restored)
			if err := loadConditionalForwardersFromDB(db.DB, conditionals, restored); err != nil {
				return err
			}
			fwdGroup.SetForwarders(restored.GetForwarders())
			condManager.SetConditionals(conditionals.GetConditionals())
			condManager.RebuildGroups(fwdGroup.GetForwarders())
			if applyPersistedSettings != nil {
				if err := applyPersistedSettings(); err != nil {
					return fmt.Errorf("reconciling restored settings: %w", err)
				}
			}
			return nil
		},
	})

	slog.Info("All services initialized (DNS, DHCP, IPAM, System)")

	// --- Apply persisted settings at startup and hot-apply changes ---
	applySetting := func(key, value string) {
		switch key {
		case "dns_recursion":
			if dnsSrv != nil {
				dnsSrv.SetRecursionEnabled(value == "true" || value == "1")
			}
		case "security_rebinding":
			filterEngine.SetRebindingProtection(value == "true" || value == "1")
		case "dns_blocking_enabled":
			filterEngine.SetBlockingEnabled(value == "true" || value == "1")
		case "dns_rate_limit_qps":
			if rateLimiter != nil {
				qps, _ := strconv.Atoi(value)
				burst := qps * 2
				if burst < 10 {
					burst = 10
				}
				rateLimiter.Configure(int64(qps), int64(burst), int64(cfg.DNS.RateLimit.RRLThreshold))
			}
		case "dns_blocklist_refresh_hours":
			if hours, err := strconv.Atoi(value); err == nil && hours > 0 {
				blockListFetcher.SetInterval(time.Duration(hours) * time.Hour)
			}
		case "dns_ecs_mode", "dns_ecs_ipv4_prefix_length", "dns_ecs_ipv6_prefix_length":
			if dnsSrv != nil {
				// The three ECS keys form one config unit; re-read all of
				// them so a single-key update cannot clobber the others.
				mode := currentECSMode(settingsMgr)
				v4 := forwarder.DefaultECSIPv4Prefix
				v6 := forwarder.DefaultECSIPv6Prefix
				if v, err := settingsMgr.GetSetting("dns_ecs_ipv4_prefix_length"); err == nil {
					v4 = parseIntOr(v, forwarder.DefaultECSIPv4Prefix)
				}
				if v, err := settingsMgr.GetSetting("dns_ecs_ipv6_prefix_length"); err == nil {
					v6 = parseIntOr(v, forwarder.DefaultECSIPv6Prefix)
				}
				dnsSrv.SetECSConfig(mode, v4, v6)
			}
		case "dns_dot_config", "dns_doh_config", "dns_doq_config":
			if dnsSrv != nil {
				kind := "dot"
				switch key {
				case "dns_doh_config":
					kind = "doh"
				case "dns_doq_config":
					kind = "doq"
				}
				var lc config.DNSListenerTLSConfig
				if err := json.Unmarshal([]byte(value), &lc); err == nil {
					if err := dnsSrv.ApplyListenerConfig(kind, lc); err != nil {
						slog.Warn("listener replacement failed; retaining active listener", "kind", kind, "error", err)
					}
				} else {
					slog.Warn("invalid persisted listener configuration", "kind", kind, "error", err)
				}
			}
		case "dns_cache_serve_stale", "dns_cache_stale_ttl", "dns_cache_prefetch",
			"dns_cache_min_ttl", "dns_cache_max_ttl":
			if dnsCache != nil {
				// The cache keys form one config unit; re-read them all so a
				// single-key update cannot clobber the others.
				get := func(k, def string) string {
					if v, err := settingsMgr.GetSetting(k); err == nil && v != "" {
						return v
					}
					return def
				}
				serveStale := get("dns_cache_serve_stale", "true") == "true" || get("dns_cache_serve_stale", "true") == "1"
				prefetch := get("dns_cache_prefetch", "false") == "true" || get("dns_cache_prefetch", "false") == "1"
				staleTTL := parseIntOr(get("dns_cache_stale_ttl", "86400"), 86400)
				minTTL := parseIntOr(get("dns_cache_min_ttl", "60"), 60)
				maxTTL := parseIntOr(get("dns_cache_max_ttl", "86400"), 86400)
				dnsCache.Configure(&serveStale, staleTTL, minTTL, maxTTL, &prefetch)
			}
		case "dns_special_zones":
			if dnsSrv != nil {
				dnsSrv.SetSpecialZonesEnabled(value == "true" || value == "1")
			}
		}
	}
	applyPersistedSettings = func() error {
		for _, key := range []string{
			"dns_recursion", "security_rebinding", "dns_blocking_enabled",
			"dns_rate_limit_qps", "dns_blocklist_refresh_hours",
			"dns_ecs_mode", "dns_ecs_ipv4_prefix_length", "dns_ecs_ipv6_prefix_length",
			"dns_dot_config", "dns_doh_config", "dns_doq_config",
			"dns_cache_serve_stale", "dns_cache_stale_ttl", "dns_cache_prefetch",
			"dns_cache_min_ttl", "dns_cache_max_ttl", "dns_special_zones",
		} {
			if v, err := settingsMgr.GetSetting(key); err == nil {
				applySetting(key, v)
			}
		}
		return nil
	}
	if err := applyPersistedSettings(); err != nil {
		return fmt.Errorf("applying persisted settings: %w", err)
	}
	settingsMgr.Subscribe(applySetting)

	// --- Background maintenance loops ---
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	defer backgroundCancel()

	// Block list URL subscription refresh.
	go blockListFetcher.Run(backgroundCtx)

	// Active upstream probes complement passive query accounting. The loop has
	// its own bounded per-probe timeout and exits with the service context.
	if interval := time.Duration(cfg.Forwarders.HealthCheckIntervalSeconds) * time.Second; interval > 0 {
		go fwdGroup.RunHealthChecks(backgroundCtx, interval)
	}

	// Record aging: delete expired records every 10 minutes.
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-backgroundCtx.Done():
				return
			case <-ticker.C:
				if deleted, err := recordMgr.CleanupExpiredRecords(); err == nil && deleted > 0 {
					zoneStore.Reload()
				}
			}
		}
	}()

	// IXFR change history pruning (30-day window).
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-backgroundCtx.Done():
				return
			case <-ticker.C:
				recordMgr.PruneZoneChangeHistory(30)
			}
		}
	}()

	slog.Info("Background maintenance loops started")

	// Build HTTP router.
	router := api.NewRouter(cfg, db)

	// Create HTTP server.
	// The WriteTimeout must be long enough to stream a full backup file
	// download (multi-GB databases are not unusual), so we set it to 10
	// minutes. The ReadTimeout can stay at 15 seconds because all our
	// upload endpoints either enforce their own body-size cap or reject
	// oversized payloads.
	srv := &http.Server{
		Addr:         cfg.Server.HTTPAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}
	if cfg.Server.TLS.Enabled {
		minVersion := uint16(tls.VersionTLS12)
		if cfg.Server.TLS.MinVersion == "1.3" {
			minVersion = tls.VersionTLS13
		}
		srv.TLSConfig = &tls.Config{MinVersion: minVersion}
	}

	// Start HTTP server in a goroutine.
	tlsEnabled := cfg.Server.TLS.Enabled
	serverErr := make(chan error, 1)
	go func() {
		if tlsEnabled {
			slog.Info("HTTPS server listening", "addr", cfg.Server.HTTPAddr,
				"min_tls_version", cfg.Server.TLS.MinVersion)
			if err := srv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
			return
		}
		slog.Info("HTTP server listening", "addr", cfg.Server.HTTPAddr,
			"warning", "TLS disabled: admin credentials and tokens travel in cleartext; enable server.tls or terminate TLS at a reverse proxy")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Start DNS server if enabled.
	if cfg.DNS.Enabled && dnsSrv != nil {
		if err := dnsSrv.Start(context.Background()); err != nil {
			slog.Error("failed to start DNS server, running in degraded mode", "error", err)
			handler.SetDNSDegraded(true)
		}
	}

	// Start DHCP server if enabled.
	if cfg.DHCP.Enabled && dhcpSrv != nil {
		if err := dhcpSrv.Start(context.Background()); err != nil {
			slog.Error("failed to start DHCP server, running in degraded mode", "error", err)
			handler.SetDHCPDegraded(true)
		}
	}

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig.String())
	}

	// Graceful shutdown with a per-step timeout so an earlier slow step
	// cannot exhaust the budget of the later ones (a shared context would
	// already be expired by the time the next Shutdown call runs).
	newCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 30*time.Second)
	}

	// Stop the HTTP server FIRST: while it is running, in-flight requests can
	// still call taskMgr.SubmitTask. Shutting down the task manager before
	// HTTP closes its submission channel, and a SubmitTask on a closed channel
	// panics (send on closed channel).
	slog.Info("shutting down HTTP server...")
	{
		ctx, cancel := newCtx()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
		cancel()
	}

	// Stop DNS server.
	if dnsSrv != nil {
		slog.Info("shutting down DNS server...")
		ctx, cancel := newCtx()
		if err := dnsSrv.Shutdown(ctx); err != nil {
			slog.Error("DNS server shutdown error", "error", err)
		}
		cancel()
	}

	// Stop DHCP server.
	if dhcpSrv != nil {
		slog.Info("shutting down DHCP server...")
		ctx, cancel := newCtx()
		if err := dhcpSrv.Shutdown(ctx); err != nil {
			slog.Error("DHCP server shutdown error", "error", err)
		}
		cancel()
	}

	// Close DHCP event logger.
	if dhcpEventLogger != nil {
		slog.Info("flushing DHCP event logs...")
		dhcpEventLogger.Close()
	}

	// Stop background maintenance loops.
	backgroundCancel()

	// Flush persistent cache to disk.
	if persistentCache != nil {
		slog.Info("flushing DNS persistent cache...")
		persistentCache.Close()
	}

	// Flush query logs.
	if queryLog != nil {
		slog.Info("flushing DNS query logs...")
		queryLog.Close()
	}

	// Flush DNS cache to persistent storage.
	if dnsCache != nil && persistentCache == nil {
		slog.Info("cleaning DNS cache...")
		dnsCache.CleanExpired()
	}

	// Stop task manager (after HTTP so no requests can submit new tasks).
	if taskMgr != nil {
		slog.Info("shutting down task manager...")
		ctx, cancel := newCtx()
		if err := taskMgr.Shutdown(ctx); err != nil {
			slog.Error("task manager shutdown error", "error", err)
		}
		cancel()
	}

	// Stop the metrics uptime ticker last (R5): it must keep publishing
	// uptime while every other component is still shutting down.
	metrics.Shutdown()

	slog.Info("GoDDI stopped gracefully")
	return nil
}

// loadFilterData loads block lists, allow rules, and client policies from the database.
func loadFilterData(db *sql.DB, filterEngine *filter.FilterEngine) error {
	if err := handler.LoadBlockListsFromDB(db, filterEngine); err != nil {
		return fmt.Errorf("loading block lists: %w", err)
	}
	if err := handler.LoadAllowRulesFromDB(db, filterEngine); err != nil {
		return fmt.Errorf("loading allow rules: %w", err)
	}
	if err := handler.LoadClientPoliciesFromDB(db, filterEngine); err != nil {
		return fmt.Errorf("loading client policies: %w", err)
	}
	return nil
}

// loadForwardersFromDB loads forwarders from the database into the forwarder group.
func loadForwardersFromDB(db *sql.DB, fwdGroup *forwarder.ForwarderGroup) error {
	rows, err := db.Query("SELECT id, name, protocol, address, enabled, priority FROM dns_forwarders")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, protocol, address string
		var enabled bool
		var priority int
		if err := rows.Scan(&id, &name, &protocol, &address, &enabled, &priority); err != nil {
			slog.Warn("failed to scan forwarder row", "error", err)
			continue
		}

		fwd := &forwarder.Forwarder{
			ID:       id,
			Name:     name,
			Protocol: protocol,
			Address:  address,
			Enabled:  enabled,
			Priority: priority,
		}
		fwd.SetHealthy(true)
		if err := fwdGroup.AddForwarder(fwd); err != nil {
			slog.Warn("skipping invalid forwarder from database", "name", name, "error", err)
		}
	}

	slog.Info("loaded forwarders from database", "count", len(fwdGroup.GetForwarders()))
	return nil
}

// loadConditionalForwardersFromDB loads conditional forwarders from the database.
func loadConditionalForwardersFromDB(db *sql.DB, condManager *forwarder.ConditionalForwarderManager, fwdGroup *forwarder.ForwarderGroup) error {
	rows, err := db.Query("SELECT id, domain, forwarder_ids, enabled FROM dns_conditional_forwarders")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, domain, forwarderIDsJSON string
		var enabled bool
		if err := rows.Scan(&id, &domain, &forwarderIDsJSON, &enabled); err != nil {
			continue
		}

		// Parse forwarder IDs from JSON.
		var fwdIDs []string
		if forwarderIDsJSON != "" && forwarderIDsJSON != "null" {
			if err := json.Unmarshal([]byte(forwarderIDsJSON), &fwdIDs); err != nil {
				slog.Warn("failed to parse forwarder IDs JSON", "id", id, "error", err)
			}
		}

		cf := &forwarder.ConditionalForwarder{
			ID:           id,
			Domain:       domain,
			ForwarderIDs: fwdIDs,
			Enabled:      enabled,
		}
		condManager.AddConditional(cf)
	}

	// Rebuild forwarder groups for all conditional forwarders.
	allFwds := fwdGroup.GetForwarders()
	condManager.RebuildGroups(allFwds)

	slog.Info("loaded conditional forwarders from database", "count", len(condManager.GetConditionals()))
	return nil
}

// runMigrations runs database migrations only.
func runMigrations(configPath string) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	config.ApplyEnvOverrides(cfg)

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	applog.InitLogger(cfg.Log.Level)

	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer db.Close()

	if err := db.RunMigrationsFS(goddiassets.Migrations()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Also initialize RBAC data during migration.
	rbacMgr := rbac.NewRBACManager(db.DB)
	if err := rbacMgr.InitializePredefinedData(); err != nil {
		return fmt.Errorf("initializing RBAC data: %w", err)
	}

	// Auto-initialize admin user if needed.
	if err := auth.AutoInitializeAdmin(db.DB); err != nil {
		slog.Warn("auto-initialize admin failed", "error", err)
	}

	slog.Info("migrations completed successfully")
	return nil
}

// runUnlock clears login rate-limit lockouts. It intentionally does NOT need
// HTTP credentials: the whole point is to recover an administrator account
// that is locked out and therefore cannot call the unlock API.
func runUnlock(configPath, username, ip string, all bool) error {
	if !all && username == "" {
		return fmt.Errorf("specify --user <name> (optionally with --ip <addr>) or --all")
	}
	if all && (username != "" || ip != "") {
		return fmt.Errorf("--all cannot be combined with --user/--ip")
	}
	if ip != "" && username == "" {
		return fmt.Errorf("--ip requires --user")
	}

	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	config.ApplyEnvOverrides(cfg)

	applog.InitLogger(cfg.Log.Level)

	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer db.Close()

	if err := auth.EnsureRateLimitTable(db.DB); err != nil {
		return fmt.Errorf("ensuring rate limit table: %w", err)
	}

	rl := auth.NewRateLimiter(db.DB, cfg.Security.LoginRateLimit, time.Duration(cfg.Security.LoginRateWindow)*time.Second)

	switch {
	case all:
		removed, err := rl.ResetAllLockouts()
		if err != nil {
			return err
		}
		fmt.Printf("已清除所有登录锁定（%d 条记录）\n", removed)
	case ip != "":
		if err := rl.ResetLoginAttempts(username, ip); err != nil {
			return err
		}
		fmt.Printf("已解锁 %s（来源 %s）\n", username, ip)
	default:
		entries, err := rl.ListLockedEntries()
		if err != nil {
			return err
		}
		removed, err := rl.ResetUserAttempts(username)
		if err != nil {
			return err
		}
		fmt.Printf("已解锁 %s（清除 %d 条记录）\n", username, removed)
		for _, e := range entries {
			if e.Username == username {
				fmt.Printf("  - 曾被锁定: %s@%s，尝试 %d 次，锁定至 %s\n",
					e.Username, e.IP, e.Attempts, e.LockedUntil.Format("2006-01-02 15:04:05"))
			}
		}
	}

	// Show remaining lockouts so the operator can confirm the state.
	remaining, err := rl.ListLockedEntries()
	if err == nil && len(remaining) > 0 {
		fmt.Printf("当前仍有 %d 个锁定：\n", len(remaining))
		for _, e := range remaining {
			fmt.Printf("  - %s@%s，锁定至 %s\n", e.Username, e.IP, e.LockedUntil.Format("2006-01-02 15:04:05"))
		}
	}

	return nil
}

func loadRuntimeConfig(configPath string) (*config.Config, error) {
	cfg, err := config.LoadFromYAML(configPath)
	if err == nil {
		return cfg, nil
	}
	if configPath == "/etc/goddi/config.yaml" && errors.Is(err, os.ErrNotExist) {
		slog.Info("default config file not found; using built-in defaults and environment overrides")
		return config.DefaultConfig(), nil
	}
	return nil, err
}

// currentECSMode reads the persisted ECS mode setting, falling back to
// "strip" when unset. Used when applying any of the three ECS keys so the
// other members of the config unit keep their stored values.
func currentECSMode(mgr *system.Manager) string {
	if mgr == nil {
		return "strip"
	}
	if v, err := mgr.GetSetting("dns_ecs_mode"); err == nil && v != "" {
		return v
	}
	return "strip"
}

// parseIntOr parses value as an int, returning fallback on error.
func parseIntOr(value string, fallback int) int {
	if n, err := strconv.Atoi(value); err == nil {
		return n
	}
	return fallback
}
