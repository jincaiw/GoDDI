package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/jasonwa/goddi/internal/dns/filter"
	"github.com/jasonwa/goddi/internal/dns/forwarder"
	dnsserver "github.com/jasonwa/goddi/internal/dns/server"
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
	Version   = "0.1.1"
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

	rootCmd.AddCommand(serveCmd, versionCmd, migrateCmd)

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
	}

	// DNS Filter Engine.
	filterEngine := filter.NewFilterEngine(cfg.Security.RebindingProtection)

	// Load filter data from database.
	if err := loadFilterData(db.DB, filterEngine); err != nil {
		slog.Warn("failed to load filter data from database", "error", err)
	}

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
	}

	// DNS Client (for debug queries).
	dnsClient := client.NewDNSClient(5 * time.Second)

	// Zone Store - load authoritative zones from database.
	zoneStore := zone.NewStore(db.DB)
	slog.Info("DNS zone store initialized", "zones", len(zoneStore.ZoneNames()))

	// DNS Server.
	var dnsSrv *dnsserver.Server
	if cfg.DNS.Enabled {
		dnsSrv = dnsserver.New(cfg, dnsCache, filterEngine, fwdGroup, condManager, queryLog, zoneStore)
	}

	// Initialize API handler services.
	handler.InitDNSServices(&handler.DNSServiceContainer{
		DB:          db.DB,
		Cache:       dnsCache,
		Filter:      filterEngine,
		Forwarder:   fwdGroup,
		Conditional: condManager,
		DNSClient:   dnsClient,
		ZoneStore:   zoneStore,
		JWTSecret:   cfg.Security.JWTSecret,
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
			return nil
		},
	})

	slog.Info("All services initialized (DNS, DHCP, IPAM, System)")

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

	// Start HTTP server in a goroutine.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("HTTP server listening", "addr", cfg.Server.HTTPAddr)
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

	// Graceful shutdown with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop DNS server first.
	if dnsSrv != nil {
		slog.Info("shutting down DNS server...")
		if err := dnsSrv.Shutdown(ctx); err != nil {
			slog.Error("DNS server shutdown error", "error", err)
		}
	}

	// Stop DHCP server.
	if dhcpSrv != nil {
		slog.Info("shutting down DHCP server...")
		if err := dhcpSrv.Shutdown(ctx); err != nil {
			slog.Error("DHCP server shutdown error", "error", err)
		}
	}

	// Close DHCP event logger.
	if dhcpEventLogger != nil {
		slog.Info("flushing DHCP event logs...")
		dhcpEventLogger.Close()
	}

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

	// Stop task manager.
	if taskMgr != nil {
		slog.Info("shutting down task manager...")
		if err := taskMgr.Shutdown(ctx); err != nil {
			slog.Error("task manager shutdown error", "error", err)
		}
	}

	// Shutdown HTTP server.
	slog.Info("shutting down HTTP server...")
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

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
