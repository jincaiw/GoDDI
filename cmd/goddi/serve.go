package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	goddiassets "github.com/jincaiw/GoDDI"
	"github.com/jincaiw/GoDDI/internal/api"
	"github.com/jincaiw/GoDDI/internal/api/handler"
	"github.com/jincaiw/GoDDI/internal/auth"
	"github.com/jincaiw/GoDDI/internal/backup"
	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/jincaiw/GoDDI/internal/configrepl"
	"github.com/jincaiw/GoDDI/internal/configver"
	"github.com/jincaiw/GoDDI/internal/database"
	"github.com/jincaiw/GoDDI/internal/dataplane"
	dhcpinternal "github.com/jincaiw/GoDDI/internal/dhcp"
	"github.com/jincaiw/GoDDI/internal/dhcp/ha"
	"github.com/jincaiw/GoDDI/internal/dhcp/lease"
	"github.com/jincaiw/GoDDI/internal/dhcp/option"
	"github.com/jincaiw/GoDDI/internal/dhcp/reservation"
	"github.com/jincaiw/GoDDI/internal/dhcp/scope"
	dhcpserver "github.com/jincaiw/GoDDI/internal/dhcp/server"
	dnsquerylog "github.com/jincaiw/GoDDI/internal/dns"
	"github.com/jincaiw/GoDDI/internal/dns/cache"
	"github.com/jincaiw/GoDDI/internal/dns/client"
	"github.com/jincaiw/GoDDI/internal/dns/dnssec"
	"github.com/jincaiw/GoDDI/internal/dns/dynamic_update"
	"github.com/jincaiw/GoDDI/internal/dns/filter"
	"github.com/jincaiw/GoDDI/internal/dns/forwarder"
	dnsserver "github.com/jincaiw/GoDDI/internal/dns/server"
	"github.com/jincaiw/GoDDI/internal/dns/transfer"
	"github.com/jincaiw/GoDDI/internal/dns/zone"
	"github.com/jincaiw/GoDDI/internal/facts"
	"github.com/jincaiw/GoDDI/internal/ipam"
	"github.com/jincaiw/GoDDI/internal/ipam/address"
	"github.com/jincaiw/GoDDI/internal/ipam/space"
	"github.com/jincaiw/GoDDI/internal/ipam/subnet"
	applog "github.com/jincaiw/GoDDI/internal/log"
	"github.com/jincaiw/GoDDI/internal/metrics"
	"github.com/jincaiw/GoDDI/internal/rbac"
	"github.com/jincaiw/GoDDI/internal/secretbox"
	"github.com/jincaiw/GoDDI/internal/system"
	"github.com/jincaiw/GoDDI/internal/task"
	"github.com/jincaiw/GoDDI/internal/tlsutil"
)

// runServer initializes and starts the GoDDI server.
func runServer(configPath string) error {
	// reloadDHCPIndex republishes the WAL lease index after the DHCP replica
	// changes (see the OnApplied hook below). It is assigned once the WAL store
	// exists.
	reloadDHCPIndex := func() {}
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

	if err := rejectEnterprise(cfg); err != nil {
		return err
	}

	// Which planes this process runs.
	//
	// The default is still one process doing everything. A split role removes
	// unrelated listeners, but is not yet full process fault isolation: data
	// plane roles still open and migrate the control database. The note below
	// makes that startup dependency visible without implying that the serving
	// path reads the control database: both data planes serve from their own
	// local stores and keep answering while the console is down.
	role := cfg.EffectiveRole()
	controlPlane := cfg.IsControlPlane()
	servesDNS := cfg.ServesDNS() && cfg.DNS.Enabled
	servesDHCP := cfg.ServesDHCP() && cfg.DHCP.Enabled

	// Initialize logger.
	applog.InitLogger(cfg.Log.Level)
	slog.Info("GoDDI starting",
		"version", Version,
		"config", configPath,
		"role", role,
		"management_api", controlPlane,
		"serves_dns", servesDNS,
		"serves_dhcp", servesDHCP,
	)

	if !controlPlane {
		// The DNS and DHCP data planes serve from their own local stores: a
		// control-plane restart does not stop resolution or lease renewal.
		// They still open and migrate the control database at boot to seed the
		// replicator, so that database must be reachable when the process
		// starts. This is a startup dependency, not a serving-path one.
		slog.Warn("data-plane role needs the control database at startup",
			"role", role,
			"detail", "the serving path uses the local data-plane store; the control database is needed to seed/replicate it at boot")
	}

	// Initialize Prometheus metrics.
	metrics.InitMetrics()
	slog.Info("Prometheus metrics initialized")

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.Server.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory %s: %w", cfg.Server.DataDir, err)
	}

	// Refuse to run a newer SQLite control schema under an older binary.
	if err := requireControlMigrationCompatibility(cfg); err != nil {
		return err
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
	// Seed the cache keys from config.yaml rather than the hard-coded
	// defaultSettings constants. applyPersistedSettings replays every row in
	// system_settings onto dnsCache a few lines below; if a fresh database had
	// been seeded with the constants, that replay would overwrite the operator's
	// explicit `cache:` values. Only missing keys are seeded, so a value later
	// changed through the UI keeps precedence over config.
	if err := settingsMgr.EnsureDefaults(cacheSettingOverrides(cfg.Cache)); err != nil {
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

	// Publish the age of the newest successful backup of each type. Read from
	// the job table on each tick rather than cached, so a restart does not make
	// a working backup schedule look lapsed. A type with no successful backup
	// produces no sample, which is what the never-backed-up alert is written
	// against; a read failure produces none either, so one SQL error per tick
	// does not become a claim about a backup that may well exist.
	metrics.RegisterBackupStatsProvider(func() []metrics.BackupStatsSample {
		last, err := backupMgr.LastSuccessfulBackups()
		if err != nil {
			slog.Warn("metrics: could not read the last successful backups", "error", err)
			return nil
		}
		out := make([]metrics.BackupStatsSample, 0, len(last))
		for backupType, at := range last {
			out = append(out, metrics.BackupStatsSample{Type: backupType, LastSuccess: at})
		}
		return out
	})

	// DNS Client (for debug queries).
	dnsClient := client.NewDNSClient(5 * time.Second)

	// --- DNS data plane ---
	//
	// A DNS process serves zones and records from a store it owns, and treats
	// the control database as a source of configuration it copies in on a poll.
	// That is what lets resolution continue while the management process is
	// restarting: the alternative is a resolver whose answer depends on a
	// database another process is free to take away.
	//
	// A process that does not serve DNS keeps reading the control database,
	// which is where it authors zones and records. There is deliberately no
	// "single database" shortcut: a second code path in which the data plane
	// reads the control database directly would be the path production never
	// exercises, and the whole point of the split is that the path being
	// exercised is the one that has to survive a control-plane outage.
	var dnsStore *dataplane.Store
	dnsDataDB := db.DB
	if servesDNS {
		zoneDSN := cfg.DataPlaneDSN(config.DataPlaneZone)
		store, err := dataplane.Open(config.DataPlaneZone, zoneDSN)
		if err != nil {
			return fmt.Errorf("opening the DNS data-plane store %s: %w", zoneDSN, err)
		}
		defer store.Close()
		dnsStore = store
		dnsDataDB = store.DB
		slog.Info("DNS data-plane store opened", "role", role, "dsn", zoneDSN)
	}

	// DNS Zone Store - load authoritative zones from the database the DNS plane
	// serves from.
	zoneStore := zone.NewStore(dnsDataDB)
	defer zoneStore.Close()
	slog.Info("DNS zone store initialized", "zones", len(zoneStore.ZoneNames()))
	// D-14: the DNS plane's last-known-good revision, so an operator can see
	// what this node will keep serving while the control plane is unreachable.
	if rev, at, ok, lerr := dataplane.LKG(dnsDataDB, dataplane.DomainDNS); lerr == nil {
		metrics.SetDataPlaneLKG(string(dataplane.DomainDNS), rev, ok)
		if ok {
			slog.Info("DNS data plane LKG", "revision", rev, "applied_at", at.UTC().Format("2006-01-02T15:04:05Z"))
		}
	}

	// Zone / record managers (dynamic updates + record aging + IXFR history).
	// These write where the DNS plane serves from, so a dynamic update and an
	// inbound transfer land next to the data they change.
	zoneMgr := zone.NewZoneManager(dnsDataDB, zoneStore)
	recordMgr := zone.NewRecordManager(dnsDataDB, zoneStore, zoneMgr)

	// DNS Server.
	var dnsSrv *dnsserver.Server
	var rateLimiter *dnsserver.RateLimiter
	// Declared here so the periodic secondary refresh can be started after the
	// background context exists, further down.
	var secondarySync *transfer.SecondarySync
	if servesDNS {
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

		// Zone transfer (AXFR/IXFR) serving. The handler reads the zone, its
		// records and its change journal from the store this process serves
		// from: a transfer of anything else would hand a secondary a zone the
		// resolver is not answering with.
		dnsSrv.SetAXFRHandler(transfer.NewAXFRHandler(dnsDataDB))

		// Experimental DNSSEC signing. Gated off by default: the responder
		// reads the replicated dns_dnssec_keys from this same data-plane DB,
		// so signing never depends on the management database being reachable.
		// NSEC and NSEC3 denial proof are implemented; the zone still needs
		// a DS at the parent, so this must not be presented as a complete
		// DNSSEC implementation yet.
		if cfg.DNS.DNSSEC.Enabled {
			dnsSrv.SetDNSSECResponder(dnssec.NewResponder(dnsDataDB, cfg.Security.JWTSecret))
			slog.Info("DNSSEC signing responder enabled (experimental: no parent DS chain of trust)")
		}

		// RFC 2136 dynamic updates (TSIG-authenticated).
		updateHandler := dynamic_update.NewUpdateHandler(dnsDataDB, zoneStore, zoneMgr, recordMgr)
		updateHandler.SetTSIGSecrets(cfg.DNS.DynamicUpdate.TSIGKeys)
		dnsSrv.SetUpdateHandler(updateHandler)

		// NOTIFY (RFC 1996): primary zones announce serial bumps to the
		// ACL notify targets; inbound NOTIFY triggers a secondary refresh.
		recordMgr.SetNotifyHook(func(zoneName string) {
			transfer.SendNotifyForZone(dnsDataDB, zoneName)
		})
		secondarySync = transfer.NewSecondarySync(dnsDataDB)
		// Give the sweeper the zone store so a zone past its SOA EXPIRE can be
		// withheld from authoritative answers instead of serving stale data.
		secondarySync.SetZoneStore(zoneStore)
		// And the reloader, so a completed transfer becomes the zone being
		// served rather than the one it replaced.
		secondarySync.SetZoneReloader(zoneStore)
		dnsSrv.SetNotifyHandler(func(zoneName string) {
			if err := secondarySync.HandleNotify(zoneName); err != nil {
				slog.Warn("notify: secondary refresh failed", "zone", zoneName, "error", err)
			}
		})
	}

	// --- Initialize the DNS data plane's replication ---

	// Four things have to reach this store, and the order they are listed in is
	// part of the design:
	//
	//	DNS     zones and records. The zone store is republished whenever this
	//	        one is applied: queries are answered from memory, so records
	//	        that arrive without a reload are names that exist and do not
	//	        resolve.
	//	DHCP    scopes. Not served here, but a client's short hostname can only
	//	        be qualified with the scope's domain.
	//	Leases  the lease replica. The sweep compares records against it, and a
	//	        create is published more precisely when it can see the binding.
	//	        Listed after scopes and before the outbox, and it is applied in
	//	        slice order, so a lease is in place before the entry that refers
	//	        to it is consumed.
	var dnsRunner *dataplane.Runner
	var dnsLink *dhcpinternal.DNSLink
	if servesDNS {
		// An all-in-one process (which owns the lease store) already relays
		// facts straight into this store's inbox; copying them again from the
		// control database would collide on the unique sequence. Only a split
		// DNS process -- which has no lease store -- needs the control-side
		// facts replica domain.
		dnsDomains := []dataplane.Domain{
			dataplane.DomainDNS,
			dataplane.DomainDHCP,
			dataplane.DomainLeases,
		}
		if !servesDHCP {
			dnsDomains = append(dnsDomains, dataplane.DomainFacts)
		}
		// The DHCP lease store may be opened later in this function than the
		// runner, so the hook is registered as a variable instead of a direct
		// reference.
		runner := dataplane.NewRunner(
			dataplane.NewReplicator(db.DB, dnsStore),
			dataplane.RunnerConfig{
				Domains:  dnsDomains,
				Interval: time.Duration(cfg.EffectiveSyncInterval()) * time.Second,
				// The records this plane writes for confirmed bindings, and a
				// dynamic update or an inbound transfer, are all authored here.
				// The console reads the control database, so without the push a
				// published name would be invisibly absent from the console --
				// which is worse than absent, because nothing tells an operator
				// to look.
				PushRecords: true,
				Quota:       dataPlaneQuota(cfg),
				OnApplied: func(d dataplane.Domain, changedPrimaryZones []string) {
					if d == dataplane.DomainDHCP {
						// A scope added, edited or removed through the console
						// reaches this plane through the sync loop, not the WAL.
						// Without republishing the pool snapshot the store keeps
						// answering "scope not found in memory index" until a
						// restart, which looks like a broken console.
						reloadDHCPIndex()
					}
					if d == dataplane.DomainDNS {
						zoneStore.ReloadNow()
						if len(changedPrimaryZones) > 0 {
							zonesToNotify := append([]string(nil), changedPrimaryZones...)
							go func() {
								for _, zoneName := range zonesToNotify {
									transfer.SendNotifyForZone(dnsDataDB, zoneName)
								}
							}()
						}
					}
				},
			})
		if err := runner.Prime(context.Background()); err != nil {
			return fmt.Errorf("priming the DNS data plane: %w", err)
		}
		dnsRunner = runner
	}

	// The consumer of that queue: the half of the DHCP-to-DNS linkage that
	// writes records.
	//
	// It runs here, with the records, rather than in the DHCP server, because a
	// withdrawal has to be decided by the process that was serving the name.
	// Its input is this store's replica of the queue, so the DHCP plane may be
	// restarting while these entries are still applied.
	if servesDNS {
		dnsLink = dhcpinternal.NewDNSLink(dhcpinternal.Same(dnsStore.DB), zoneStore)
		dnsLink.SetNotifyHook(func(zoneName string) {
			transfer.SendNotifyForZone(dnsDataDB, zoneName)
		})
	}

	// Cache prefetch: hot entries whose remaining TTL drops below the
	// threshold trigger a background re-resolution through the server's
	// normal forwarding path.
	if dnsCache != nil && dnsSrv != nil {
		dnsCache.SetPrefetchCallback(dnsSrv.Prefetch)
	}

	// Secrets that must not sit in the database in the clear are sealed under
	// their own label. Two labels with the same key material still derive two
	// different keys, so a TOTP seed can never be replayed as a TSIG key.
	tsigSealer := secretbox.New(secretbox.LabelTSIG, securityKeyMaterial(cfg))

	// Migration 023 added the sealed column; rows written before it still hold
	// the secret in the clear. The move runs here, next to the database, and
	// not beside the DNS listener: it is a property of the data, not of whether
	// this particular process happens to be running a DNS server. Gating it on
	// the listener left plaintext in place for as long as DNS was disabled.
	if sealedCount, err := transfer.SealPlaintextTSIGSecrets(db.DB, tsigSealer); err != nil {
		slog.Error("failed to seal plaintext TSIG secrets; signed transfers may fail", "error", err)
	} else if sealedCount > 0 {
		slog.Warn("sealed TSIG secrets that were stored in the clear", "count", sealedCount)
	}

	// RFC 8945 TSIG keys for signed zone transfers and RFC 2136 updates on
	// the DNS listeners. The listener needs the dynamic-update keys too: it
	// verifies the TSIG MAC against the raw wire bytes, which the update
	// handler cannot reproduce by re-packing a compressed message.
	if dnsSrv != nil {
		secrets, err := transfer.TSIGSecretMap(db.DB, tsigSealer)
		if err != nil {
			// Fail closed and loudly. A map quietly missing a live key would
			// REFUSE every transfer signed with it, with nothing in the log to
			// say why.
			slog.Error("TSIG keys could not be loaded; signed transfers will be refused", "error", err)
		} else {
			if len(cfg.DNS.DynamicUpdate.TSIGKeys) > 0 {
				if secrets == nil {
					secrets = make(map[string]string, len(cfg.DNS.DynamicUpdate.TSIGKeys))
				}
				for name, secret := range cfg.DNS.DynamicUpdate.TSIGKeys {
					key := strings.ToLower(name)
					if _, exists := secrets[key]; !exists {
						secrets[key] = secret
					}
				}
			}
			if len(secrets) > 0 {
				dnsSrv.SetTSIGSecretMap(secrets)
				names := make([]string, 0, len(secrets))
				for n := range secrets {
					names = append(names, n)
				}
				slog.Info("TSIG keys loaded", "count", len(secrets), "names", names)
			}
		}
	}

	// Initialize API handler services.
	//
	// These containers exist only to serve the management API. A data-plane
	// process builds no router, so populating them would be dead wiring that
	// reads as if an endpoint exists.
	if controlPlane {
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
			Secrets:          tsigSealer,
			Config:           cfg,
		})
	}

	// --- Initialize the DHCP data plane ---

	// A DHCP process serves from a store it owns. The scopes it hands addresses
	// out of arrive by replication; the leases it issues are written here and
	// nowhere else. Nothing on the request path waits for the control database,
	// which is what makes "the console is being restarted" and "clients cannot
	// renew their addresses" two different events.
	var dhcpStore *dataplane.Store
	var dhcpRunner *dataplane.Runner
	var haRepl *ha.Replicator
	var haMirror *ha.Mirror
	// autoFailoverCh carries "this standby decided to promote" to the shutdown
	// path. Promotion is durable but takes effect on the next start, so the
	// process exits and a supervisor restarts it into the primary role. Nil
	// unless ha.auto_failover is enabled.
	var autoFailoverCh chan struct{}

	// The role this node runs as is resolved before anything is built from it.
	//
	// The configuration says what the deployment intends a node to be; the
	// lease store says what an operator has since decided -- a promotion after
	// a takeover, or a fence. The two are allowed to disagree, and the recorded
	// one wins, which is what stops a node an operator fenced from coming back
	// as a primary on the next restart.
	//
	// standsAside covers everything that is not the author: a mirror serves
	// nobody because its leases are somebody else's, and a fenced node serves
	// nobody because an operator has said it must not. Both are switched off
	// here rather than being prevented later by a check inside them.
	var haCfg ha.Config
	haEnabled := servesDHCP && cfg.DHCPHA.Enabled
	haRole := ""
	standsAside := false
	if haEnabled {
		resolved, err := ha.FromConfig(cfg.DHCPHA)
		if err != nil {
			return fmt.Errorf("reading the DHCP HA settings: %w", err)
		}
		haCfg = resolved
		if cfg.DHCPHA.TLSCertFile != "" {
			tlsCfg, err := ha.LoadTLS(cfg.DHCPHA.TLSCertFile, cfg.DHCPHA.TLSKeyFile, cfg.DHCPHA.TLSCAFile, cfg.DHCPHA.TLSServerName)
			if err != nil {
				return fmt.Errorf("loading the DHCP HA peer TLS material: %w", err)
			}
			haCfg.TLS = tlsCfg
			haCfg.TLSServerName = cfg.DHCPHA.TLSServerName
			slog.Info("DHCP HA peer channel uses mutual TLS")
		}
	}

	if servesDHCP {
		leaseDSN := cfg.DataPlaneDSN(config.DataPlaneLease)
		store, err := dataplane.Open(config.DataPlaneLease, leaseDSN)
		if err != nil {
			return fmt.Errorf("opening the lease store %s: %w", leaseDSN, err)
		}
		defer store.Close()
		dhcpStore = store
		slog.Info("DHCP lease store opened", "role", role, "dsn", leaseDSN)

		if haEnabled {
			if haRole, err = ha.EffectiveRole(haCfg, store); err != nil {
				return fmt.Errorf("resolving the DHCP HA role: %w", err)
			}
			// A node that was primary, went down, and comes back after its peer
			// was promoted would otherwise come up as a second writer: both
			// sides listen, neither dials, and no epoch comparison ever runs.
			// Probing the peer once at startup closes that window. An
			// unreachable peer changes nothing -- the node starts and then runs
			// paused, because it has no second copy.
			if haRole == config.HARolePrimary {
				localEpoch, eerr := ha.OwnerEpoch(store)
				if eerr != nil {
					return fmt.Errorf("reading the owner epoch: %w", eerr)
				}
				peerEpoch, reached, perr := ha.ProbePeerEpoch(haCfg, localEpoch)
				if perr != nil {
					slog.Warn("DHCP HA: could not probe the peer at startup", "error", perr)
				}
				if reached && peerEpoch > localEpoch {
					if derr := ha.RecordSuperseded(store, peerEpoch); derr != nil {
						return fmt.Errorf("recording that this node was superseded: %w", derr)
					}
					slog.Error("DHCP HA: a peer owns a newer owner epoch; this node will not serve as primary",
						"local_epoch", localEpoch, "peer_epoch", peerEpoch)
					haRole = config.HARoleStandby
				}
			}
			standsAside = haRole != config.HARolePrimary
			slog.Info("DHCP HA enabled", "settings", haCfg.Describe(), "role_in_force", haRole)
		}

		// Prime decides whether this process may serve at all: a DHCP server
		// with no scopes accepts nobody while looking healthy, so an
		// unreachable control database with an empty local store is fatal while
		// one with a store behind it is not.
		runner := dataplane.NewRunner(
			dataplane.NewReplicator(db.DB, store),
			dataplane.RunnerConfig{
				Domains:  []dataplane.Domain{dataplane.DomainDHCP},
				Interval: time.Duration(cfg.EffectiveSyncInterval()) * time.Second,
				// Lease changes, the event log and the DNS outbox are all
				// pushed up from here: the console reads the lease replica, and
				// the DNS plane -- a different process, in a different file
				// system -- has no other way to learn that a record is owed.
				//
				// Unless this node is not the author. A mirror's leases are
				// somebody else's: pushing them up would report another
				// machine's bindings as this one's, and would race the
				// primary's own push to the same rows. A fenced node is not
				// serving, so it has nothing current to report either.
				PushLeases: !standsAside,
				Quota:      dataPlaneQuota(cfg),
			})
		if err := runner.Prime(context.Background()); err != nil {
			return fmt.Errorf("priming the DHCP data plane: %w", err)
		}
		dhcpRunner = runner
	}

	if haEnabled {
		// Every component below is built for the role in force rather than for
		// the role the file asks for. They have diverged whenever an operator
		// has promoted or fenced this node, and building for the file's role is
		// how a node that was just promoted comes up as a mirror instead.
		effective := haCfg.ForRole(haRole)
		switch haRole {
		case ha.RoleFenced:
			// A fenced node runs neither half. Saying so at startup is the
			// whole of its behaviour: the state exists so that an operator who
			// has taken a node out of a pair can see that the node knows.
			slog.Warn("DHCP HA: this node is fenced; it will not serve clients and will not mirror",
				"node_id", haCfg.NodeID, "state", ha.StateFenced)
		case config.HARoleStandby:
			mirror, err := ha.NewMirror(effective, dhcpStore)
			if err != nil {
				return fmt.Errorf("opening the DHCP HA mirror: %w", err)
			}
			haMirror = mirror
			if effective.AutoFailover {
				autoFailoverCh = make(chan struct{}, 1)
				effectiveCopy := effective
				storeCopy := dhcpStore
				mirror.SetAutoPromote(func() bool {
					// Record the promotion durably (epoch+1) with the same
					// guarded action an operator runs. The shortfall is the
					// gap the standby already knows; unacknowledged changes
					// cannot contain an acknowledged binding.
					op := ha.NewOperator(effectiveCopy, storeCopy)
					st, serr := op.Status()
					var gap int64
					if serr == nil {
						gap = st.Gap
					}
					out, terr := op.Takeover(ha.TakeoverOptions{
						Confirmed: true, OldPrimaryCannotWrite: true, AcceptGap: gap,
					})
					if terr != nil {
						slog.Error("HA: auto-failover promotion was refused; will keep trying", "error", terr)
						return false
					}
					slog.Error("HA: auto-failover promoted this node; restarting to take effect",
						"epoch", out.Epoch, "seq", out.Seq)
					select {
					case autoFailoverCh <- struct{}{}:
					default:
					}
					return true
				})
			}
			// The redundancy state goes in the startup log, not only in a
			// metric and not only when it changes. A node that comes up
			// without the second copy it was configured for has to say so
			// where somebody is already looking.
			slog.Warn("DHCP HA: this node is a standby; it will not serve clients",
				"node_id", haCfg.NodeID, "mirror_applied_seq", mirror.AppliedSeq(),
				"peer", haCfg.PeerAddress, "state", mirror.State())
		default:
			repl, err := ha.NewReplicator(effective, dhcpStore)
			if err != nil {
				return fmt.Errorf("opening the DHCP HA replicator: %w", err)
			}
			haRepl = repl
			// A primary starts paused unless an operator has already approved
			// running alone. Until its mirror has answered, this node has no
			// second copy and must not promise addresses -- and the
			// alternative, starting in a state that permits them, is a window
			// on every restart in which a binding can be acknowledged with
			// nowhere to recover it from.
			slog.Info("DHCP HA: this node is a primary",
				"node_id", haCfg.NodeID, "state", repl.State(),
				"listen", haCfg.ListenAddr, "seq", repl.Seq(), "acked_seq", repl.AckedSeq(),
				"degraded", repl.Degraded())
		}
	} else if servesDHCP {
		// The redundancy state is printed whether or not HA is on. "Am I
		// redundant?" is a question with an answer on every installation, and
		// the answer for one that never configured a pair is solo -- not an
		// absence of information.
		slog.Info("DHCP HA is off; this node serves without a second copy", "state", ha.StateSolo)
	}

	// --- Initialize DHCP Components ---

	// DHCP Event Logger. The event log is written from the request path, so it
	// belongs on the request path's database: a control database that is locked
	// or unreachable must not be able to slow a DISCOVER down.
	var dhcpEventLogger *dhcpinternal.EventLogger
	if servesDHCP {
		dhcpEventLogger = dhcpinternal.NewEventLogger(dhcpStore.DB)
		slog.Info("DHCP event logger initialized")
	}

	// DHCP Server.
	//
	// It is handed one database and no zone store. Turning a binding into a
	// record, and withdrawing it again, belongs to the plane that serves the
	// name; what this server does is record that the update is owed -- durably,
	// before the client is told it may use the address. A DHCP-only process
	// therefore holds no handle to a zone table it must not write to.
	var dhcpSrv *dhcpserver.Server
	var walLeaseStore *lease.WALLeaseStore
	if servesDHCP && !standsAside {
		if cfg.DHCP.LeaseStore == "wal" {
			// HA-safe shape: a standby sets standsAside and never reaches this
			// block (it does not construct the DHCP server; the mirror writes its
			// SQLite lease DB directly). A primary may run the WAL store; on a
			// takeover the promoted node restarts into the primary role and the
			// WAL store loads its index from the already-mirrored SQLite snapshot.
			walPath := strings.TrimSpace(cfg.DHCP.LeaseWALPath)
			if walPath == "" {
				walPath = filepath.Join(cfg.Server.DataDir, "leases.wal")
			}
			walSource := strings.TrimSpace(cfg.DHCPHA.NodeID)
			if walSource == "" {
				host, herr := os.Hostname()
				if herr != nil || strings.TrimSpace(host) == "" {
					walSource = "dhcp-node"
				} else {
					walSource = host
				}
			}
			store, serr := lease.OpenWALLeaseStore(context.Background(), lease.WALLeaseStoreConfig{
				Path: walPath, DB: dhcpStore.DB, Capacity: 1024, Source: walSource,
				HashChain: cfg.DHCP.FactsHashChain,
				ResolveSpaceID: func(scopeID, ip string) (string, error) {
					return dhcpStore.ResolveIPAMSpaceByIP(context.Background(), ip)
				},
			})
			if serr != nil {
				return fmt.Errorf("opening WAL lease store: %w", serr)
			}
			walLeaseStore = store
			reloadDHCPIndex = func() {
				if err := store.ReloadFromDB(context.Background()); err != nil {
					slog.Warn("dataplane: could not republish the lease index after a DHCP change", "error", err)
				}
			}
			dhcpSrv = dhcpserver.NewWithLeaseStore(store, dhcpStore.DB, cfg.DHCP.Interfaces, dhcpEventLogger)
			slog.Info("DHCP lease store: event-sourced WAL", "wal", walPath)
		} else {
			dhcpSrv = dhcpserver.New(dhcpStore.DB, cfg.DHCP.Interfaces, dhcpEventLogger)
		}
		// The second copy, when there is one. It is installed before Start so
		// that no REQUEST can arrive in a window where the binding would be
		// acknowledged without it.
		if haRepl != nil {
			dhcpSrv.SetLeaseReplicator(ha.Adapt(haRepl))
		}
		// Set before Start, not after: a relay allowlist that arrives once
		// packets are already being served is a window in which the
		// restriction is not in force.
		dhcpSrv.SetTrustedRelays(cfg.DHCP.TrustedRelayCIDRs)
		// Same "before Start" rule as the relay allowlist: a renewal policy
		// that arrived after packets were flowing would leave a window in
		// which neither the old nor the new rule was in force.
		dhcpSrv.SetRenewOutOfPoolPolicy(cfg.DHCP.RenewOutOfPoolPolicy)
		if cfg.DHCP.RequireIPAMProjection {
			dhcpSrv.SetScopeProjectionGuard(ipam.NewProjectionGuard(dhcpStore.DB))
			slog.Info("DHCP: IPAM projection guard enabled (require_ipam_projection)")
		}
		// D-09 non-blocking signal: scopes whose subnet has no local IPAM
		// projection. Allocation is unchanged unless require_ipam_projection
		// (the guard above) is set; this only makes the gap visible.
		if gaps, gerr := ipam.ProjectionGaps(context.Background(), dhcpStore.DB); gerr == nil {
			metrics.DHCPIPAMProjectionGaps.Set(float64(len(gaps)))
			if len(gaps) > 0 {
				slog.Warn("DHCP: scopes missing local IPAM projection", "count", len(gaps))
			}
		}
		// D-14: expose the last-known-good data-plane revision this node has
		// durably applied, so operators can see what config it will serve if
		// the control plane is unreachable.
		if rev, at, ok, lerr := dataplane.LKG(dhcpStore.DB, dataplane.DomainDHCP); lerr == nil {
			metrics.SetDataPlaneLKG(string(dataplane.DomainDHCP), rev, ok)
			if ok {
				slog.Info("DHCP data plane LKG", "revision", rev, "applied_at", at.UTC().Format("2006-01-02T15:04:05Z"))
			}
		}
		slog.Info("DHCP server initialized",
			"interfaces", cfg.DHCP.Interfaces, "lease_store", dhcpStore.DSN())
	}

	// Initialize DHCP API handler services.
	//
	// The console's lease view is a replica when the data plane is a different
	// process: it is refreshed by the upward push, and a lease released from the
	// console would be overwritten by the next push from the plane that actually
	// owns the row. That is a silent lie rather than a bug in the UI, so a
	// replica-only view refuses the write and says why.
	//
	// When this process is the data plane too, the console is given the
	// authoritative store and nothing changes for the operator.
	if controlPlane {
		leaseMgrDB, leaseViewIsReplica := db.DB, true
		if dhcpStore != nil {
			leaseMgrDB, leaseViewIsReplica = dhcpStore.DB, false
		}
		if leaseViewIsReplica {
			slog.Info("DHCP console: lease view is a replica; lease changes are refused here",
				"reason", "lease state is owned by the data plane, which runs in another process")
		}
		handler.InitDHCPServices(&handler.DHCPServiceContainer{
			DB:               db.DB,
			ScopeMgr:         scope.NewManager(db.DB),
			LeaseMgr:         lease.NewManager(leaseMgrDB),
			MutationOwner:    dhcpSrv,
			ReservMgr:        reservation.NewManager(db.DB),
			OptionMgr:        option.NewManager(db.DB),
			EventLogger:      dhcpEventLogger,
			LeasesAreReplica: leaseViewIsReplica,
		})
	}

	// --- Initialize IPAM Components ---

	// The linkage is the bridge between IPAM and the other two subsystems. It
	// is constructed before the DHCP server so the server can report lease
	// activity into IPAM, and before the API so the 360° address view has
	// something to compare the IPAM record against.
	ipamLinkage := ipam.NewLinkage(db.DB)
	var ipamFactsPipeline *ipam.FactsPipeline
	// The consumer-side assembly: the inbox lives with the projection (the
	// control database) and the data-plane relay copies the lease plane's
	// producer outbox into it. The relay runs wherever the lease database
	// lives (the all-in-one and the DHCP plane); the IPAM consumer runs
	// wherever the control database lives (the all-in-one and the control
	// plane) — in a split deployment each half runs in its own process and
	// the trigger-driven facts revision keeps the DNS plane's replica current.
	var dhcpFactsProducerOutbox *facts.ObservationOutbox
	if dhcpStore != nil {
		dhcpFactsProducerOutbox, _ = facts.NewObservationOutbox(dhcpStore.DB)
	}
	if controlPlane {
		controlInbox, err := facts.NewObservationOutbox(db.DB)
		if err != nil {
			return fmt.Errorf("opening control-side DHCP facts inbox: %w", err)
		}
		var relay *facts.Relay
		if dhcpFactsProducerOutbox != nil {
			relay, err = facts.NewRelay(dhcpFactsProducerOutbox, controlInbox, db.DB)
			if err != nil {
				return fmt.Errorf("initializing the DHCP facts relay: %w", err)
			}
		}
		ipamFactsPipeline, err = ipam.NewFactsPipeline(ipamLinkage, controlInbox, ipam.FactsPipelineOptions{
			Enabled: true,
			Relay:   relay,
		})
		if err != nil {
			return fmt.Errorf("initializing IPAM facts consumer: %w", err)
		}
	}

	// Initialize IPAM API handler services.
	if controlPlane {
		handler.InitIPAMServices(&handler.IPAMServiceContainer{
			DB:         db.DB,
			SpaceMgr:   space.NewManager(db.DB),
			SubnetMgr:  subnet.NewManager(db.DB),
			AddressMgr: address.NewManager(db.DB),
			Linkage:    ipamLinkage,
		})
	}

	// Keep the legacy observer for deployments with an unmapped local DHCP
	// address. Mapped mutations are represented in the durable facts stream and
	// projected atomically by the control-side consumer.
	if dhcpSrv != nil {
		// The WAL lease store derives facts from its own durable events, so the
		// synchronous facts writer seam and the best-effort observer are not
		// installed; the outbox/metrics/readiness below still read the same
		// producer table the store writes.
		if walLeaseStore == nil {
			dhcpSrv.SetLeaseObserver(ipamLinkage)
		}
		// Standbys and fenced nodes serve no DHCP requests and only receive the
		// primary's facts stream. A primary, including one promoted by takeover,
		// commits facts with its lease mutations and gates REQUEST/renewal ACKs on
		// both HA watermarks.
		if !haEnabled || haRole == config.HARolePrimary {
			allocator, err := facts.NewSequenceAllocator(dhcpStore.DB)
			if err != nil {
				return fmt.Errorf("initializing DHCP fact sequence allocator: %w", err)
			}
			var outbox *facts.ObservationOutbox
			if cfg.DHCP.FactsHashChain {
				outbox, err = facts.NewObservationOutboxStamped(dhcpStore.DB, "dhcp_lease_facts")
			} else {
				outbox, err = facts.NewObservationOutbox(dhcpStore.DB)
			}
			if err != nil {
				return fmt.Errorf("initializing DHCP fact outbox: %w", err)
			}
			metrics.RegisterFactsProducerStatsProvider(func() []metrics.FactsProducerSample {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				status, err := outbox.Status(ctx)
				if err != nil {
					slog.Warn("metrics: could not read DHCP facts producer status", "error", err)
					return nil
				}
				return []metrics.FactsProducerSample{{
					Domain: "ipam", Pending: status.Pending, Failed: status.Failed,
					HeadSequence:             status.HeadSequence,
					FirstOutstandingSequence: status.FirstOutstandingSequence,
				}}
			})
			_ = allocator
			// Only the SQLite store uses the synchronous facts writer seam. The
			// WAL store emits facts from its own projection, so installing this
			// seam as well would double-publish every mutation.
			if walLeaseStore == nil {
				writer, werr := lease.NewFactsMutationWriter(lease.NewManager(dhcpStore.DB), allocator, outbox)
				if werr != nil {
					return fmt.Errorf("initializing DHCP fact mutation writer: %w", werr)
				}
				if dhcpRunner != nil {
					writer.WithPostCommitWake(dhcpRunner.Wake)
				}
				source := strings.TrimSpace(cfg.DHCPHA.NodeID)
				if source == "" {
					host, herr := os.Hostname()
					if herr != nil || strings.TrimSpace(host) == "" {
						source = "dhcp-node"
					} else {
						source = host
					}
				}
				dhcpSrv.SetLeaseFactsMutation(&dhcpserver.LeaseFactsMutationConfig{
					Caller: writer, Source: source,
					ResolveSpaceID: func(_ string, ip string) (string, error) {
						return dhcpStore.ResolveIPAMSpaceByIP(context.Background(), ip)
					},
				})
				// RELEASE routes through the same facts writer so the write and
				// its durable fact commit in one transaction.
				dhcpSrv.SetReleaseFactsMutation(&dhcpserver.ReleaseFactsMutationConfig{
					Caller: writer, Source: source,
					ResolveSpaceID: func(scopeID, ip string) (string, error) {
						return dhcpStore.ResolveIPAMSpaceByIP(context.Background(), ip)
					},
				})
			}
			// The producer-side lag/gap plane: failed events are retained in
			// the outbox and must never be disguised as health, a pending
			// backlog is degraded (the facts are not consumed yet), and an
			// empty outbox is ok. This is the observation opt-in, so the plane
			// only exists when the observation actually runs.
			handler.SetPlaneProbe("dhcp_facts", handler.NewCachedProbe(5*time.Second, func() handler.PlaneStatus {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				return handler.DHCPFactsPlane(outbox.Status(ctx))
			}).Probe)
		}
		slog.Info("IPAM: DHCP lease observation enabled")
	}

	// --- Initialize configuration publishing ---

	// Every governed change becomes an immutable revision, so a wrong SOA
	// contact or pool range can be diffed and rolled back instead of
	// remembered. The zone store is handed to the DNS adapter because the
	// resolver answers from memory: without a forced reload a published zone
	// change would sit in the database and never be served.
	//
	// Publishing is a management-plane activity: it is the console that turns a
	// scope edit into a revision and a release. A data-plane process consumes
	// the result through replication instead of publishing alongside it.
	var configSvc *configver.Service
	if controlPlane {
		configSvc = configver.NewService(db.DB)
		for _, adapter := range []configver.Adapter{
			configver.NewDHCPScopeAdapter(),
			configver.NewDNSZoneAdapter(zoneStore),
			configver.NewDNSRecordsAdapter(zoneStore),
			configver.NewIPAMSubnetAdapter(),
		} {
			if err := configSvc.Register(adapter); err != nil {
				slog.Warn("config publishing: adapter not registered", "type", adapter.Type(), "error", err)
			}
		}
		handler.InitConfigVersionService(configSvc)
	}

	// --- Initialize System API handler services ---
	// Assigned after the runtime settings applier is constructed below. The
	// restore hook calls it to reconcile persisted settings with in-memory DNS
	// components after a successful config-section restore.
	var applyPersistedSettings func() error
	if controlPlane {
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
	}

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
	// D-20: refresh the LKG/projection gauges periodically (they are also set
	// once at startup); the ticker is bounded by the background context.
	if dhcpStore != nil || dnsDataDB != nil {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-backgroundCtx.Done():
					return
				case <-ticker.C:
					refreshDataPlaneGauges(backgroundCtx, dhcpStoreDB(dhcpStore), dnsDataDB)
				}
			}
		}()
	}
	if ipamFactsPipeline != nil {
		if err := ipamFactsPipeline.Start(backgroundCtx); err != nil {
			return fmt.Errorf("starting IPAM facts consumer: %w", err)
		}
	}

	// DHCP configuration replication. The poll is a single indexed read of the
	// control-plane revision counter; while it keeps up, a scope edited in the
	// console reaches the data plane within the interval. It is deliberately not
	// on the request path: a client waits for neither the console nor this loop.
	if dhcpRunner != nil {
		go dhcpRunner.Run(backgroundCtx)
	}

	// ADR-0013 configuration replication, follower side. When enabled and this
	// node is not the config author, pull the author's configuration snapshot
	// and apply it to this node's control database so a promoted standby has the
	// scopes and zones to serve. After each apply, wake the data-plane sync so
	// the change reaches dnsdata.db/leases.db without waiting a full interval.
	//
	// A snapshot is a full replace, so an apply that lands is complete: a missed
	// pull is corrected by the next one. Failures are logged and retried; the
	// follower keeps serving whatever it already has.
	if cfg.Cluster.ConfigReplication && !cfg.Cluster.IsConfigAuthor() {
		puller := configrepl.NewPuller(db.DB,
			configrepl.PullerConfig{
				SourceURL: cfg.Cluster.ConfigReplicationSource,
				Token:     cfg.Cluster.ConfigReplicationToken,
				Interval:  time.Duration(cfg.Cluster.ConfigReplicationInterval) * time.Second,
			},
			dataplane.ConfigTableSpecs(),
			func() {
				if dnsRunner != nil {
					dnsRunner.Wake()
				}
				if dhcpRunner != nil {
					dhcpRunner.Wake()
				}
			})
		go func() {
			slog.Info("config replication: follower puller starting",
				"source", cfg.Cluster.ConfigReplicationSource)
			if err := puller.Run(backgroundCtx); err != nil {
				slog.Error("config replication: puller stopped", "error", err)
			}
		}()
	}

	// The DHCP high-availability pair. Both halves run off the request path
	// except for the one thing that is on it: a binding waits for the mirror
	// to have it, and that wait is bounded by dhcp_ha.confirm_timeout.
	//
	// The channel is direct between the two nodes and is not carried by the
	// control database. That is the whole point of it: an acknowledgement that
	// had to travel through the management plane would make a console restart
	// into a DHCP outage, which is the failure this contract exists to prevent.
	if haRepl != nil {
		go func() {
			if err := haRepl.Run(backgroundCtx); err != nil {
				slog.Error("DHCP HA: the primary's peer listener stopped", "error", err)
			}
		}()
	}
	if haMirror != nil {
		go func() {
			if err := haMirror.Run(backgroundCtx); err != nil {
				slog.Error("DHCP HA: the mirror stopped", "error", err)
			}
		}()
	}

	// DNS configuration replication, and the consumer of the queue the DHCP
	// plane pushes up. Both are off the request path: a query is answered from
	// memory, so a poll that is late costs a name that resolves to yesterday's
	// address, not a query that fails.
	if dnsRunner != nil {
		go dnsRunner.Run(backgroundCtx)
	}

	// The DNS side of the unified facts chain: the relay copies the lease
	// plane's facts into this store's inbox, and the DNS facts consumer
	// applies them to the records with the same idempotent ApplyOne the
	// former legacy queue used. The unified facts chain is now the only DNS
	// producer. This assembly lives where both planes are
	// in one process; a split-role deployment relays the inbox through the
	// control database the way the legacy queue does (the follow-up).
	if servesDNS {
		// The DNS plane consumes its local inbox copy. In the all-in-one the
		// relay fills it from the lease plane in-process; in a split build the
		// DomainFacts replication fills it from the control inbox, so the
		// consumer assembles either way.
		dnsInbox, err := facts.NewObservationOutbox(dnsStore.DB)
		if err != nil {
			return fmt.Errorf("opening the DNS facts inbox: %w", err)
		}
		dnsFactsConsumer, err := dhcpinternal.NewDNSFactsConsumer(dnsLink, dnsInbox)
		if err != nil {
			return fmt.Errorf("initializing the DNS facts consumer: %w", err)
		}
		if dhcpFactsProducerOutbox != nil {
			dnsRelay, err := facts.NewRelay(dhcpFactsProducerOutbox, dnsInbox, dnsStore.DB)
			if err != nil {
				return fmt.Errorf("initializing the DNS facts relay: %w", err)
			}
			go func() {
				if err := dnsRelay.Run(backgroundCtx, time.Second, func(err error) {
					slog.Warn("dns facts relay: tick failed; the durable state is safe and will retry", "error", err)
				}); err != nil && !errors.Is(err, context.Canceled) {
					slog.Warn("dns facts relay: loop ended", "error", err)
				}
			}()
		}
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-backgroundCtx.Done():
					return
				case <-ticker.C:
					if _, err := dnsFactsConsumer.Replay(backgroundCtx, 100); err != nil {
						slog.Warn("dns facts consumer: replay error; facts stay pending for replay", "error", err)
					}
				}
			}
		}()
		slog.Info("DNS: unified facts consumer enabled")
	}

	// IPAM reconciliation.
	//
	// When the data plane is this process, every lease change is reported as it
	// happens. When it is not, the observation never arrives and the IPAM view
	// would simply stay empty. It is derived here from the lease replica
	// instead, which also repairs any observation lost to a crash between the
	// lease commit and the report -- the gap that made the reconciler exist in
	// the first place.
	if controlPlane {
		go func() {
			ticker := time.NewTicker(ipamReconcileEvery)
			defer ticker.Stop()
			for {
				select {
				case <-backgroundCtx.Done():
					return
				case <-ticker.C:
					res, err := ipamLinkage.Reconcile(ipamReconcileLimit)
					if err != nil {
						slog.Warn("IPAM: reconciliation failed", "error", err)
						continue
					}
					if res.Observed > 0 || res.Released > 0 {
						slog.Info("IPAM: reconciliation updated addresses",
							"observed", res.Observed, "released", res.Released,
							"scanned", res.Scanned)
					}
				}
			}
		}()
	}

	// Block list URL subscription refresh. The control plane owns the
	// database, so it is the one that goes out and refreshes subscriptions; a
	// data plane receives the result through replication.
	if controlPlane {
		go blockListFetcher.Run(backgroundCtx)
	}

	// Release worker for published configuration. It runs here rather than at
	// publish time so that a release written before a crash is completed after
	// the restart, instead of being lost with the process.
	if configSvc != nil {
		go configSvc.Run(backgroundCtx, configver.ReleaseInterval)
		if pending, failed, err := configSvc.OutboxStats(); err == nil && (pending > 0 || failed > 0) {
			// Surfaced at startup because a non-zero failed count means a
			// configuration is recorded but not in service -- an operator should
			// not have to go looking for that.
			slog.Warn("configuration releases pending at startup", "pending", pending, "failed", failed)
		}
	}

	// Active upstream probes complement passive query accounting. The loop has
	// its own bounded per-probe timeout and exits with the service context.
	if interval := time.Duration(cfg.Forwarders.HealthCheckIntervalSeconds) * time.Second; interval > 0 && servesDNS {
		go fwdGroup.RunHealthChecks(backgroundCtx, interval)
	}

	// Record aging: expire served snapshots at the exact record deadline and
	// persist the deletion/serial change promptly afterward.
	if servesDNS {
		go func() {
			ticker := time.NewTicker(time.Minute)
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
	}

	// Secondary zone maintenance: refresh zones whose SOA refresh interval has
	// elapsed, and withhold any zone that has passed its SOA EXPIRE. Without
	// this loop the sweeper is dead code and a secondary zone silently serves
	// whatever copy it last managed to transfer, forever.
	if secondarySync != nil {
		go secondarySync.StartPeriodicSync(backgroundCtx.Done())
	}

	slog.Info("Background maintenance loops started")

	// Readiness probes: what /ready reports.
	//
	// Each plane answers for what it is accountable for and the process
	// reports the worst of them. Registration happens before the listeners
	// are started, so a listener that never comes up is reported through the
	// same path as everything else instead of only in the startup log --
	// the closures read the two variables the start attempts below fill in.
	var dnsStartErr, dhcpStartErr error

	if controlPlane {
		// The control plane keeps no local copy, so the only honest question
		// is whether its database answers. Cached, because /ready is
		// unauthenticated: a ping per probe would let anyone take the single
		// connection the request path needs.
		handler.SetPlaneProbe("control", handler.NewCachedProbe(5*time.Second, func() handler.PlaneStatus {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := db.DB.PingContext(ctx); err != nil {
				return handler.PlaneStatus{
					Level:   handler.LevelFailing,
					Reasons: []string{"control_database_unreachable"},
				}
			}
			return handler.PlaneStatus{Level: handler.LevelOK}
		}).Probe)
		if ipamFactsPipeline != nil {
			handler.SetPlaneProbe("ipam_facts", handler.NewCachedProbe(5*time.Second, func() handler.PlaneStatus {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				status, err := ipamFactsPipeline.Status(ctx)
				if err != nil {
					return handler.PlaneStatus{Level: handler.LevelFailing, Reasons: []string{"ipam_facts_status_unavailable"}}
				}
				switch status.Consumer.Readiness() {
				case ipam.FactsReadinessOK:
					return handler.PlaneStatus{Level: handler.LevelOK}
				case ipam.FactsReadinessDegraded:
					return handler.PlaneStatus{Level: handler.LevelDegraded, Reasons: []string{"ipam_facts_backlog"}}
				default:
					return handler.PlaneStatus{Level: handler.LevelFailing, Reasons: []string{"ipam_facts_consumer_unavailable"}}
				}
			}).Probe)
		}
	}
	if ipamFactsPipeline != nil {
		metrics.RegisterFactsConsumerStatsProvider(func() []metrics.FactsConsumerSample {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			status, err := ipamFactsPipeline.Status(ctx)
			if err != nil {
				slog.Warn("metrics: could not read IPAM facts consumer status", "error", err)
				return nil
			}
			return []metrics.FactsConsumerSample{{
				Domain: ipam.IPAMFactsConsumerDomain, Pending: int64(status.Consumer.Pending),
				Failed: int64(status.Consumer.Failed), Applied: status.Consumer.LastApplied,
				Lag: int64(status.Consumer.Lag), Gap: status.Consumer.Gap,
				Readiness: status.Consumer.Readiness(),
			}}
		})
	}

	if servesDNS {
		handler.SetPlaneProbe("dns", func() handler.PlaneStatus {
			if dnsStartErr != nil {
				return handler.PlaneStatus{
					Level:   handler.LevelFailing,
					Reasons: []string{"dns_listener_failed_to_start"},
				}
			}
			return dataPlaneStatus(dnsRunner)
		})
	}

	if servesDHCP {
		handler.SetPlaneProbe("dhcp", func() handler.PlaneStatus {
			if dhcpStartErr != nil {
				return handler.PlaneStatus{
					Level:   handler.LevelFailing,
					Reasons: []string{"dhcp_listener_failed_to_start"},
				}
			}
			st := dataPlaneStatus(dhcpRunner)
			mergeStartupStatus(&st, dhcpStartupStatus(dhcpSrv))
			if haEnabled {
				// Losing the second copy is a caveat on top of whatever the
				// data plane reports, not a replacement for it. Both are true
				// at once and an operator needs both.
				mergeHAStatus(&st, haRole, haRepl, haMirror)
			}
			return st
		})
	}

	metrics.RegisterDataPlaneStatsProvider(func() []metrics.DataPlaneSample {
		var out []metrics.DataPlaneSample
		if dnsRunner != nil {
			out = append(out, dataPlaneSample("zone", dnsRunner))
		}
		if dhcpRunner != nil {
			sample := dataPlaneSample("lease", dhcpRunner)
			startup := dhcpStartupStatus(dhcpSrv)
			sample.StartupConfigured = startup.Configured
			sample.StartupReady = startup.Ready
			sample.StartupState = startup.State
			sample.StartupLastSeq = startup.LastSeq
			if startup.Configured && !startup.Ready {
				sample.Level = handler.Worse(sample.Level, handler.LevelFailing)
			}
			out = append(out, sample)
		}
		return out
	})

	// Publish DHCP pool utilisation.
	//
	// Sampled from the store that owns the leases, and only by the process that
	// serves DHCP. A process reading a replica would report a figure that is
	// behind by a push interval, and would keep reporting a scope that the
	// plane which owns it has already emptied.
	//
	// A failed read leaves the series at its previous value rather than
	// publishing a zero. Zero here means "an empty pool", which is the reading
	// that would silence a full-pool alert; a stale figure at least stays in
	// the range the last successful look measured.
	if dhcpStore != nil {
		dhcpLeaseMgr := lease.NewManager(dhcpStore.DB)
		metrics.RegisterDHCPScopeStatsProvider(func() ([]metrics.DHCPScopeSample, bool) {
			usage, err := dhcpLeaseMgr.ScopeUtilization()
			if err != nil {
				slog.Warn("metrics: could not read DHCP scope utilisation", "error", err)
				return nil, false
			}
			out := make([]metrics.DHCPScopeSample, 0, len(usage))
			for _, u := range usage {
				out = append(out, metrics.DHCPScopeSample{
					Scope: u.Scope,
					Held:  u.Held,
					Ratio: u.Ratio,
				})
			}
			return out, true
		})
	}

	// Publish this node's DHCP redundancy.
	//
	// Reported by the node that owns the fact. A primary knows whether its
	// mirror is current; a fenced node knows it is neither redundant nor
	// promising. A standby reports nothing, because its answer is the same fact
	// seen from the other side -- and publishing it from both nodes would give
	// one fact two authors, which is how the two come to disagree.
	//
	// Nothing is published at all when HA is off. The redundancy series must be
	// absent on an installation that never had a second copy, not zero: zero is
	// the alarm.
	//
	// The standby publishes too: it is the node that dials the peer, so its
	// peer_up / peer_last_seen_seconds are the primary evidence that the second
	// cable/peer is alive, and its own Redundant/Promising describe its side of
	// the pair. Sequence fields simply stay zero because it authors nothing.
	if haEnabled {
		metrics.RegisterDHCPHAStatsProvider(func() []metrics.DHCPHASample {
			state := currentHAState(haRole, haRepl, haMirror)
			sample := metrics.DHCPHASample{
				NodeID:    haCfg.NodeID,
				Redundant: state.Redundant(),
				Promising: state.MayBind(),
			}
			if haRepl != nil {
				sample.Sequence = haRepl.Seq()
				sample.AcknowledgedSequence = haRepl.AckedSeq()
				peer := haRepl.PeerWatermarks()
				sample.PeerAppliedSequence = peer.AppliedSeq
				sample.PeerAppliedFactsSequence = peer.FactsAppliedSeq
				sample.FactsAcknowledgedSequence = haRepl.FactsAckedSeq()
				factsSequence, err := haRepl.FactsSequence()
				if err != nil {
					slog.Warn("metrics: could not read DHCP HA facts sequence", "error", err)
					return nil
				}
				sample.FactsSequence = factsSequence
				sample.PeerUp, sample.PeerLastSeen = haRepl.LinkSnapshot()
			}
			if haMirror != nil {
				sample.PeerUp, sample.PeerLastSeen = haMirror.LinkSnapshot()
			}
			return []metrics.DHCPHASample{sample}
		})
	}

	// GET /api/v1/dhcp/ha answers the question "what is this node's HA state?"
	// from the same facts `goddi ha status` and /ready use. Read-only: the
	// provider never mutates anything and never exposes the peer token.
	if haEnabled {
		handler.RegisterDHCPHAStatusProvider(func() map[string]any {
			state := currentHAState(haRole, haRepl, haMirror)
			out := map[string]any{
				"ha_enabled":      true,
				"node_id":         haCfg.NodeID,
				"configured_role": haCfg.Role,
				"role_in_force":   haRole,
				"state":           state.String(),
				"redundant":       state.Redundant(),
				"promising":       state.MayBind(),
			}
			if haRepl != nil {
				out["sequence"] = haRepl.Seq()
				out["acknowledged_sequence"] = haRepl.AckedSeq()
				peer := haRepl.PeerWatermarks()
				out["peer_applied_sequence"] = peer.AppliedSeq
				out["peer_applied_facts_sequence"] = peer.FactsAppliedSeq
			}
			if haMirror != nil {
				out["applied_sequence"] = haMirror.AppliedSeq()
				if up, lastSeen := haMirror.LinkSnapshot(); up {
					out["peer_last_seen"] = lastSeen
				}
			}
			return out
		})
	}

	// Publish secondary-zone refresh health, from the database the sweeper
	// writes to. Reading the control database instead would report the state of
	// the configuration rather than the state of the transfers, and the two
	// diverge precisely when a transfer is failing.
	if servesDNS {
		metrics.RegisterSecondaryZoneStatsProvider(func() []metrics.SecondaryZoneSample {
			health, err := transfer.SecondaryZoneHealth(dnsDataDB)
			if err != nil {
				slog.Warn("metrics: could not read secondary zone health", "error", err)
				return nil
			}
			out := make([]metrics.SecondaryZoneSample, 0, len(health))
			for _, h := range health {
				out = append(out, metrics.SecondaryZoneSample{
					Zone:      h.Zone,
					Failures:  h.Failures,
					HasSynced: h.HasSynced,
					LastSync:  h.LastSync,
				})
			}
			return out
		})
	}

	// Build HTTP router.
	//
	// The management API is the control plane. A data-plane process builds no
	// router and binds no listening socket, which is what makes "the console is
	// being restarted" and "clients cannot get an address" two different
	// events.
	var srv *http.Server
	serverErr := make(chan error, 1)
	if controlPlane {
		router := api.NewRouter(cfg, db)

		// Create HTTP server.
		// The WriteTimeout must be long enough to stream a full backup file
		// download (multi-GB databases are not unusual), so we set it to 10
		// minutes. The ReadTimeout can stay at 15 seconds because all our
		// upload endpoints either enforce their own body-size cap or reject
		// oversized payloads.
		srv = &http.Server{
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
			// Built here, not inside the goroutine below: a certificate that
			// cannot be read is a startup failure, and a failure reported from
			// a goroutine is a process that appears to be serving.
			tlsCfg, err := tlsutil.ServerConfig(tlsutil.Options{
				CertFile:     cfg.Server.TLS.CertFile,
				KeyFile:      cfg.Server.TLS.KeyFile,
				MinVersion:   minVersion,
				ClientCAFile: cfg.Server.TLS.ClientCAFile,
			})
			if err != nil {
				return fmt.Errorf("building the management TLS configuration: %w", err)
			}
			srv.TLSConfig = tlsCfg
			if cfg.Server.TLS.ClientCAFile != "" {
				slog.Info("management TLS requires a client certificate",
					"client_ca_file", cfg.Server.TLS.ClientCAFile)
			}
		}

		// Start HTTP server in a goroutine.
		tlsEnabled := cfg.Server.TLS.Enabled
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
	} else {
		slog.Info("management API not served by this process", "role", role,
			"note", "this process runs only the data plane; the console is a separate process")
	}

	// Start DNS server if enabled.
	if servesDNS && dnsSrv != nil {
		if err := dnsSrv.Start(context.Background()); err != nil {
			// Recorded rather than marked here: the probe registered above
			// turns it into a failing readiness, which is what an operator
			// and an orchestrator both read.
			dnsStartErr = err
			slog.Error("failed to start DNS server, running in degraded mode", "error", err)
		}
	}

	// Start DHCP server if enabled.
	if servesDHCP && dhcpSrv != nil {
		if err := dhcpSrv.Start(context.Background()); err != nil {
			dhcpStartErr = err
			slog.Error("failed to start DHCP server, running in degraded mode", "error", err)
		}
	}

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-autoFailoverCh:
		slog.Error("HA: shutting down for an auto-failover promotion; a supervisor must restart this process into the primary role")
		// Fall through to the graceful shutdown below; the process exits 0 and
		// the supervisor (systemd Restart=always / container restart policy)
		// brings it back, where EffectiveRole reads the recorded primary role.
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
	if srv != nil {
		slog.Info("shutting down HTTP server...")
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
	if walLeaseStore != nil {
		slog.Info("shutting down WAL lease store...")
		ctx, cancel := newCtx()
		if err := walLeaseStore.Close(ctx); err != nil {
			slog.Error("WAL lease store shutdown error (durable events replay on start)", "error", err)
		}
		cancel()
	}
	if ipamFactsPipeline != nil {
		slog.Info("shutting down IPAM facts consumer...")
		ctx, cancel := newCtx()
		if err := ipamFactsPipeline.Stop(ctx); err != nil {
			slog.Error("IPAM facts consumer shutdown error", "error", err)
		}
		cancel()
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
