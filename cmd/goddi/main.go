package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/api"
	"github.com/jasonwa/goddi/internal/api/handler"
	"github.com/jasonwa/goddi/internal/auth"
	"github.com/jasonwa/goddi/internal/backup"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/configver"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/jasonwa/goddi/internal/dataplane"
	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
	"github.com/jasonwa/goddi/internal/dhcp/ha"
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
	"github.com/jasonwa/goddi/internal/ipam"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/space"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
	applog "github.com/jasonwa/goddi/internal/log"
	"github.com/jasonwa/goddi/internal/metrics"
	"github.com/jasonwa/goddi/internal/rbac"
	"github.com/jasonwa/goddi/internal/secretbox"
	"github.com/jasonwa/goddi/internal/system"
	"github.com/jasonwa/goddi/internal/task"
	"github.com/jasonwa/goddi/internal/tlsutil"
	"github.com/spf13/cobra"
)

var (
	// Build information, set at compile time via ldflags.
	Version   = "0.8.2"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// IPAM reconciliation cadence. The pass derives the IPAM view from the lease
// replica, so how often it runs is a trade-off between how stale an address
// list may look and how much work an idle console does. Thirty seconds is well
// inside the interval an operator notices, and the pass is bounded per run.
const (
	ipamReconcileEvery = 30 * time.Second
	ipamReconcileLimit = 200
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
	//
	// --status and --check report what is applied and what is pending without
	// doing any of it. They exist because "did that restore land me on a
	// schema this binary can read" is a question an operator asks before the
	// first start, and the answer has to come from somewhere other than
	// running the thing that is in question.
	var migrateStatusOnly, migrateCheckOnly bool
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations, or report their state",
		Long: "Run all pending database migrations.\n" +
			"With --status, report which migrations are applied and which are pending for\n" +
			"every database this deployment owns, without changing anything. With --check,\n" +
			"do the same and exit non-zero when anything is pending or when a database is\n" +
			"ahead of this binary.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if migrateStatusOnly || migrateCheckOnly {
				return runMigrationStatus(configPath, migrateCheckOnly)
			}
			return runMigrations(configPath)
		},
	}
	migrateCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	migrateCmd.Flags().BoolVar(&migrateStatusOnly, "status", false, "Report applied and pending migrations without applying anything")
	migrateCmd.Flags().BoolVar(&migrateCheckOnly, "check", false, "Like --status, but exit non-zero when anything is pending or ahead of this binary")

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

	// rekey subcommand: moves stored secrets onto the configured encryption
	// key. A CLI rather than an endpoint, because the operation has to run
	// while both keys are known and must not be reachable over the network.
	var rekeyDryRun bool
	rekeyCmd := &cobra.Command{
		Use:   "rekey",
		Short: "Re-encrypt stored secrets under security.encryption_key",
		Long: "Move TOTP seeds and TSIG keys onto security.encryption_key.\n" +
			"Run this after setting a dedicated encryption key on an installation that was\n" +
			"encrypting with the JWT secret, otherwise those values can no longer be read.\n" +
			"Use --dry-run to report what would change without writing.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRekey(configPath, rekeyDryRun)
		},
	}
	rekeyCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	rekeyCmd.Flags().BoolVar(&rekeyDryRun, "dry-run", false, "Report what would change without writing")

	// backup subcommand: writes an archive an operator can rebuild an instance
	// from. A CLI rather than an endpoint because its result is a file on the
	// operator's filesystem: an archive the API could write is an archive an
	// attacker holding an admin token could read every password hash out of.
	var backupOutput string
	var backupInclude []string
	var backupPlaintext bool
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Write a consistent, encrypted snapshot of this deployment",
		Long: "Copy every database this deployment owns into one archive.\n" +
			"The databases are copied with SQLite's VACUUM INTO, so each one is a single\n" +
			"consistent instant of a database that was being written to, and the archive\n" +
			"travels with the configuration files, the schema versions and an inventory of\n" +
			"the stored secrets.\n" +
			"The archive is encrypted with security.encryption_key; without it the command\n" +
			"refuses unless --plaintext says otherwise.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSnapshot(configPath, backupOutput, backupInclude, backupPlaintext)
		},
	}
	backupCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "Where to write the archive (required)")
	backupCmd.Flags().StringArrayVar(&backupInclude, "include", nil, "Extra file to place in the archive (repeatable, optional files are skipped when absent)")
	backupCmd.Flags().BoolVar(&backupPlaintext, "plaintext", false, "Write the archive unencrypted, which exposes every stored secret in it")

	// restore subcommand: verifies an archive and either unpacks it or replaces
	// the databases this deployment is serving from.
	var restore rest
	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Verify an archive, unpack it, or restore it over this deployment",
		Long: "Read a snapshot written by `goddi backup`.\n" +
			"Every member is checked against the manifest before anything is written, and\n" +
			"the unpacked files land in a staging directory that is moved into place only\n" +
			"once the whole archive has been read.\n" +
			"Use --verify to check an archive without unpacking it, --into <dir> to unpack\n" +
			"it, or --in-place to replace this deployment's databases -- which takes a copy\n" +
			"of the current ones first and requires the service to be stopped.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(configPath, restore)
		},
	}
	restoreCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	restoreCmd.Flags().StringVarP(&restore.input, "input", "i", "", "The archive to read (required)")
	restoreCmd.Flags().StringVar(&restore.into, "into", "", "Unpack the archive into this directory")
	restoreCmd.Flags().BoolVar(&restore.verifyOnly, "verify", false, "Check the archive and report what it holds, without writing anything")
	restoreCmd.Flags().BoolVar(&restore.replace, "replace", false, "Allow --into to overwrite an existing directory")
	restoreCmd.Flags().BoolVar(&restore.inPlace, "in-place", false, "Replace this deployment's databases with the ones in the archive")
	restoreCmd.Flags().BoolVar(&restore.yes, "yes", false, "Confirm --in-place, having stopped the service")

	// ha subcommand: the operator actions of the DHCP high-availability
	// contract (ADR 0003).
	//
	// They are a CLI rather than endpoints because every one of them is a
	// judgement no observation can make: "carry on without a second copy", "the
	// other node can no longer write". A console that could make those over the
	// network could make them with a stolen token, and what they decide is
	// whether the deployment still has redundancy.
	var haStatusJSON bool

	var haDegradeConfirm, haDegradeUndo bool
	var haDegradeReason string

	var haTakeoverConfirm, haTakeoverOldStopped bool
	var haTakeoverGap int64

	var haFenceConfirm bool
	var haFenceReason string

	var haRejoinConfirm bool

	haCmd := &cobra.Command{
		Use:   "ha",
		Short: "Inspect and change this node's DHCP high-availability role",
		Long: "The operator actions of the DHCP high-availability contract.\n" +
			"Nothing in the server decides any of these on its own. A node that loses its\n" +
			"peer stops promising addresses and says so; only a person can decide that it\n" +
			"should carry on without one, or that it has become the one that should serve.",
	}

	haStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Report this node's recorded redundancy",
		Long: "Report the role this node has been left in and the watermarks its store holds.\n" +
			"The report is read-only: it opens the lease store without creating it, without\n" +
			"applying pragmas and without migrating it, because a report that changes the\n" +
			"thing it describes has answered a different question.\n" +
			"One fact is not in the store and therefore not in this report: whether the peer\n" +
			"is currently reachable. Only the running service observes the link, and it says\n" +
			"so in its log and on /ready.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHAStatus(configPath, haStatusJSON)
		},
	}
	haStatusCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	haStatusCmd.Flags().BoolVar(&haStatusJSON, "json", false, "Print the state as JSON instead of a report")

	haDegradeCmd := &cobra.Command{
		Use:   "degrade",
		Short: "Approve serving without a second copy, or withdraw the approval",
		Long: "Record that this node may acknowledge bindings and renewals even though its\n" +
			"mirror is not current. Without this the node keeps answering DISCOVER and keeps\n" +
			"honouring releases, but withholds every acknowledgement -- which is what a client\n" +
			"experiences as an address it cannot keep.\n" +
			"The permission is recorded in this node's lease store and is picked up by the\n" +
			"running service within one heartbeat interval, so no restart is needed.\n" +
			"It clears itself as soon as the mirror has caught up, and --undo withdraws it\n" +
			"immediately.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHADegrade(configPath, haDegradeConfirm, haDegradeUndo, haDegradeReason)
		},
	}
	haDegradeCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	haDegradeCmd.Flags().BoolVar(&haDegradeConfirm, "confirm", false, "Confirm that this node will acknowledge bindings it cannot recover")
	haDegradeCmd.Flags().BoolVar(&haDegradeUndo, "undo", false, "Withdraw the approval instead of granting it")
	haDegradeCmd.Flags().StringVar(&haDegradeReason, "reason", "", "Why the node is being allowed to run without a second copy")

	haTakeoverCmd := &cobra.Command{
		Use:   "takeover",
		Short: "Promote this standby to primary",
		Long: "Promote this node, a standby, to be the node that owns the leases.\n" +
			"It requires three statements that this node cannot verify for itself:\n" +
			"  --confirm                 the operator means it;\n" +
			"  --old-primary-cannot-write the node that was primary has been stopped, disconnected\n" +
			"                            or fenced. A dead primary and a partitioned one are both\n" +
			"                            silent to this node, so only a person can tell them apart;\n" +
			"  --accept-gap <n>          the exact number of changes the primary handed out that this\n" +
			"                            node does not hold, when there are any. A binding is\n" +
			"                            acknowledged only once the standby has applied it, so those\n" +
			"                            changes were never acknowledged -- but the number says how\n" +
			"                            far the two nodes had drifted, and it has to be read before\n" +
			"                            it is accepted.\n" +
			"The promotion takes effect at the next start. A standby runs no DHCP server, so that\n" +
			"is not an interruption of anything being served: it is the start of one.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHATakeover(configPath, haTakeoverConfirm, haTakeoverOldStopped, haTakeoverGap)
		},
	}
	haTakeoverCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	haTakeoverCmd.Flags().BoolVar(&haTakeoverConfirm, "confirm", false, "Confirm that this node becomes the only author of its leases")
	haTakeoverCmd.Flags().BoolVar(&haTakeoverOldStopped, "old-primary-cannot-write", false, "Confirm that the node which was primary has been stopped, disconnected or fenced")
	haTakeoverCmd.Flags().Int64Var(&haTakeoverGap, "accept-gap", 0, "Accept this exact shortfall against the primary's sequence")

	haFenceCmd := &cobra.Command{
		Use:   "fence",
		Short: "Take this node out of service",
		Long: "Record that this node must not serve clients and must not mirror.\n" +
			"It is the action that makes a takeover from the other node safe: the node that was\n" +
			"primary is the only one that can stop itself.\n" +
			"It takes effect at the next start. Until then the running service is still in the\n" +
			"state its link implies, and a node that has lost its mirror cannot acknowledge a\n" +
			"binding anyway -- the window is one in which it offers and releases, not one in\n" +
			"which it promises.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHAFence(configPath, haFenceConfirm, haFenceReason)
		},
	}
	haFenceCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	haFenceCmd.Flags().BoolVar(&haFenceConfirm, "confirm", false, "Confirm that this node stops serving and stops mirroring")
	haFenceCmd.Flags().StringVar(&haFenceReason, "reason", "", "Why the node is being taken out of service")

	haRejoinCmd := &cobra.Command{
		Use:   "rejoin",
		Short: "Return a fenced node to the pair as a standby",
		Long: "Return this node to the pair. It comes back as a standby, never as a primary: it\n" +
			"has been out of the pair while another node was serving, so the only thing it can\n" +
			"be trusted to do is hold a copy of what that node holds.\n" +
			"Its leases will be replaced by the primary's on the next start.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHARejoin(configPath, haRejoinConfirm)
		},
	}
	haRejoinCmd.Flags().StringVarP(&configPath, "config", "c", "/etc/goddi/config.yaml", "Path to configuration file")
	haRejoinCmd.Flags().BoolVar(&haRejoinConfirm, "confirm", false, "Confirm that this node's leases will be replaced by the primary's")

	haCmd.AddCommand(haStatusCmd, haDegradeCmd, haTakeoverCmd, haFenceCmd, haRejoinCmd)

	rootCmd.AddCommand(serveCmd, versionCmd, migrateCmd, unlockCmd, rekeyCmd, backupCmd, restoreCmd, haCmd)

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

	// Which planes this process runs.
	//
	// The default is still one process doing everything. Separating them is a
	// deployment choice an operator makes when the console being restarted must
	// not interrupt the addresses clients are already using: a data-plane
	// process has no management API and no control-plane database to migrate,
	// so nothing on the console's startup path is on the client's path.
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
		// Stated plainly rather than left for an operator to discover: the
		// role model is in place, but the serving path of a data-plane process
		// still reads the control database. Until it reads its own store
		// instead, a data-plane process needs the control database reachable
		// when it starts and is not yet isolated from it.
		slog.Warn("this role does not yet serve from its own local store",
			"role", role,
			"detail", "reading the local store on the serving path is the next step; the control database must still be reachable")
	}

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
	slog.Info("DNS zone store initialized", "zones", len(zoneStore.ZoneNames()))

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
	//	DDNS    the work the DHCP plane owes: the entries this process turns into
	//	        records. It is a log, not configuration, and it rides on its own
	//	        counter so lease churn cannot look like a zone edit.
	var dnsRunner *dataplane.Runner
	if servesDNS {
		runner := dataplane.NewRunner(
			dataplane.NewReplicator(db.DB, dnsStore),
			dataplane.RunnerConfig{
				Domains: []dataplane.Domain{
					dataplane.DomainDNS,
					dataplane.DomainDHCP,
					dataplane.DomainLeases,
					dataplane.DomainDDNS,
				},
				Interval: time.Duration(cfg.EffectiveSyncInterval()) * time.Second,
				// The records this plane writes for confirmed bindings, and a
				// dynamic update or an inbound transfer, are all authored here.
				// The console reads the control database, so without the push a
				// published name would be invisibly absent from the console --
				// which is worse than absent, because nothing tells an operator
				// to look.
				PushRecords: true,
				Quota:       dataPlaneQuota(cfg),
				OnApplied: func(d dataplane.Domain) {
					if d == dataplane.DomainDNS {
						zoneStore.ReloadNow()
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
	var dnsConsumer *dhcpinternal.DNSConsumer
	if servesDNS {
		link := dhcpinternal.NewDNSLink(dhcpinternal.Same(dnsStore.DB), zoneStore)
		dnsConsumer = dhcpinternal.NewDNSConsumer(dnsStore.DB, link)
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

	// RFC 8945 TSIG keys for signed zone transfers on TCP listeners.
	if dnsSrv != nil {
		secrets, err := transfer.TSIGSecretMap(db.DB, tsigSealer)
		if err != nil {
			// Fail closed and loudly. A map quietly missing a live key would
			// REFUSE every transfer signed with it, with nothing in the log to
			// say why.
			slog.Error("TSIG keys could not be loaded; signed transfers will be refused", "error", err)
		} else if secrets != nil {
			dnsSrv.SetTSIGSecretMap(secrets)
			slog.Info("TSIG keys loaded", "count", len(secrets))
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
	if servesDHCP && !standsAside {
		dhcpSrv = dhcpserver.New(dhcpStore.DB, cfg.DHCP.Interfaces, dhcpEventLogger)
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
		// A queued DNS change wakes the outbox push instead of waiting for the
		// loop's next poll. The consumer is in another process and cannot be
		// woken at all, so this is the one interval of the two the client waits
		// through that the data plane is able to remove.
		if dhcpRunner != nil {
			dhcpSrv.SetOutboxWake(dhcpRunner.Wake)
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

	// Report lease state changes into IPAM. Without this the DHCP server hands
	// out addresses that IPAM still lists as free, which is how the two views
	// drift apart until neither is trusted.
	if dhcpSrv != nil {
		dhcpSrv.SetLeaseObserver(ipamLinkage)
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

	// DHCP configuration replication. The poll is a single indexed read of the
	// control-plane revision counter; while it keeps up, a scope edited in the
	// console reaches the data plane within the interval. It is deliberately not
	// on the request path: a client waits for neither the console nor this loop.
	if dhcpRunner != nil {
		go dhcpRunner.Run(backgroundCtx)
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
	if dnsConsumer != nil {
		go dnsConsumer.Run(backgroundCtx)
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

	// Record aging: delete expired records every 10 minutes.
	if servesDNS {
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
			out = append(out, dataPlaneSample("lease", dhcpRunner))
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
		metrics.RegisterDHCPScopeStatsProvider(func() []metrics.DHCPScopeSample {
			usage, err := dhcpLeaseMgr.ScopeUtilization()
			if err != nil {
				slog.Warn("metrics: could not read DHCP scope utilisation", "error", err)
				return nil
			}
			out := make([]metrics.DHCPScopeSample, 0, len(usage))
			for _, u := range usage {
				out = append(out, metrics.DHCPScopeSample{
					Scope: u.Scope,
					Held:  u.Held,
					Ratio: u.Ratio,
				})
			}
			return out
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
	if haEnabled && haRole != config.HARoleStandby {
		metrics.RegisterDHCPHAStatsProvider(func() []metrics.DHCPHASample {
			state := currentHAState(haRole, haRepl, haMirror)
			return []metrics.DHCPHASample{{
				NodeID:    haCfg.NodeID,
				Redundant: state.Redundant(),
				Promising: state.MayBind(),
			}}
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
// runRekey moves stored secrets onto security.encryption_key.
//
// The previous key material is the JWT secret: that is what this build used
// before a dedicated key existed, and it is the only other key an operator
// could have had. When the two are the same there is nothing to move, and the
// command says so instead of reporting a successful no-op.
func runRekey(configPath string, dryRun bool) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)
	applog.InitLogger(cfg.Log.Level)

	if cfg.Security.EncryptionKey == "" {
		return fmt.Errorf("security.encryption_key is not set, so there is nothing to rekey onto (set GODDI_SECURITY_ENCRYPTION_KEY)")
	}
	if cfg.Security.EncryptionKey == cfg.Security.JWTSecret {
		return fmt.Errorf("security.encryption_key is identical to the JWT secret; set it to a distinct value before rekeying")
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer db.Close()

	targets := secretbox.ControlDatabaseSecrets()
	to := secretbox.New(secretbox.LabelTSIG, cfg.Security.EncryptionKey)

	if dryRun {
		outcomes, err := secretbox.RekeyPlan(db.DB, cfg.Security.JWTSecret, cfg.Security.EncryptionKey, targets)
		if err != nil {
			return fmt.Errorf("planning rekey: %w", err)
		}
		plaintext, err := transfer.CountPlaintextTSIGSecrets(db.DB)
		if err != nil {
			return err
		}
		reportRekey(outcomes, true)
		if plaintext > 0 {
			fmt.Printf("另有 %d 条 TSIG 密钥以明文存储，将在下次启动或本次执行时加密\n", plaintext)
		}
		return nil
	}

	outcomes, err := secretbox.Rekey(db.DB, cfg.Security.JWTSecret, cfg.Security.EncryptionKey, targets)
	if err != nil {
		return err
	}
	plaintext, err := transfer.SealPlaintextTSIGSecrets(db.DB, to)
	if err != nil {
		return err
	}
	reportRekey(outcomes, false)
	if plaintext > 0 {
		fmt.Printf("另有 %d 条 TSIG 密钥从明文转为加密存储\n", plaintext)
	}
	return nil
}

func reportRekey(outcomes []secretbox.RekeyOutcome, dryRun bool) {
	moved := 0
	for _, o := range outcomes {
		verb := "已迁移"
		if dryRun {
			verb = "待迁移"
		}
		fmt.Printf("%s %s：重加密 %d 条，明文加密 %d 条，已是最新 %d 条，空值 %d 条\n",
			verb, o.Table, o.Resealed, o.SealedPlaintext, o.AlreadyCurrent, o.Empty)
		moved += o.Resealed + o.SealedPlaintext
	}
	if dryRun {
		if moved == 0 {
			fmt.Println("没有需要迁移的密文；当前密钥可以读取全部已存储的密钥材料")
		} else {
			fmt.Printf("共 %d 条需要迁移；去掉 --dry-run 执行\n", moved)
		}
		return
	}
	fmt.Printf("完成：共迁移 %d 条\n", moved)
}

// --- backup and restore ---

// rest holds the flags of the restore subcommand. They are read together
// because they constrain one another: --in-place and --into are alternatives,
// and --in-place means nothing without --yes.
type rest struct {
	input      string
	into       string
	verifyOnly bool
	replace    bool
	inPlace    bool
	yes        bool
}

// snapshotTargets names every database this deployment owns.
//
// All three are listed whatever role the process runs, and each data-plane
// store is optional: a host where the DNS plane has never run has no DNS store,
// and a backup that failed for that reason would fail on exactly the hosts with
// nothing to lose. The absence is recorded in the manifest instead, so "there
// was nothing there" and "the archiver left it out" stay distinguishable.
func snapshotTargets(cfg *config.Config) []backup.SnapshotTarget {
	return []backup.SnapshotTarget{
		{Name: "control", DSN: cfg.Database.DSN, HoldsSecrets: true},
		{Name: "dnsdata", DSN: cfg.DataPlaneDSN(config.DataPlaneZone), Optional: true},
		{Name: "leases", DSN: cfg.DataPlaneDSN(config.DataPlaneLease), Optional: true},
	}
}

// runSnapshot writes a restorable archive of everything this deployment owns.
func runSnapshot(configPath, output string, includes []string, plaintext bool) error {
	if strings.TrimSpace(output) == "" {
		return errors.New("--output is required: a snapshot with nowhere to go would have to be discarded")
	}
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)
	applog.InitLogger(cfg.Log.Level)

	// config.Validate is deliberately not called. A backup is most needed when
	// the deployment is already unhealthy, and refusing to take one because an
	// unrelated path in the configuration is wrong is the wrong way round.
	material := securityKeyMaterial(cfg)
	if plaintext {
		if material != "" {
			fmt.Println("注意：已指定 --plaintext；尽管配置中设置了 encryption_key，该归档仍未加密")
		}
		material = ""
	}

	sources := []backup.ConfigSource{{Path: configPath}}
	for _, include := range includes {
		sources = append(sources, backup.ConfigSource{Path: include, Optional: true})
	}

	manifest, err := backup.CreateSnapshot(backup.SnapshotRequest{
		OutputPath:     output,
		Version:        Version,
		Description:    fmt.Sprintf("%s 快照，角色 %s", cfg.Server.Name, cfg.EffectiveRole()),
		Targets:        snapshotTargets(cfg),
		ConfigFiles:    sources,
		KeyMaterial:    material,
		AllowPlaintext: plaintext,
	})
	if err != nil {
		return err
	}
	describeSnapshot(*manifest, output)
	return nil
}

// runRestore verifies an archive and then does one of three things with it.
func runRestore(configPath string, opts rest) error {
	if strings.TrimSpace(opts.input) == "" {
		return errors.New("--input is required")
	}
	if opts.inPlace && strings.TrimSpace(opts.into) != "" {
		return errors.New("--in-place and --into are alternatives; choose one")
	}
	if !opts.inPlace && !opts.verifyOnly && strings.TrimSpace(opts.into) == "" {
		return errors.New("choose what to do with the archive: --verify to check it, --into <dir> to unpack it, or --in-place to replace this deployment's databases")
	}
	if opts.inPlace && !opts.yes {
		return errors.New("--in-place replaces the databases this deployment serves from and the service must be stopped first; re-run with --yes once it is")
	}

	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)
	applog.InitLogger(cfg.Log.Level)
	material := securityKeyMaterial(cfg)

	if opts.verifyOnly {
		manifest, err := backup.InspectSnapshot(opts.input, material)
		if err != nil {
			return err
		}
		describeSnapshot(*manifest, opts.input)
		if mismatched := backup.SnapshotKeyMismatches(*manifest, material); len(mismatched) > 0 {
			return keyMismatchError(mismatched)
		}
		// The fit is reported but not enforced here. --verify answers "is this
		// archive intact", which is a question about the artifact; whether
		// this binary can use it is a question about the target, and it is
		// enforced on the paths that write something.
		fitness, err := assessArchive(*manifest)
		if err != nil {
			return err
		}
		describeArchiveFitness(fitness)
		fmt.Println("校验通过：归档完整，且与本机密钥材料一致。未写入任何文件。")
		return nil
	}

	// An in-place restore unpacks into a temporary directory first. Nothing
	// this deployment serves from is touched until the whole archive has been
	// read and checked.
	extractTo := opts.into
	if opts.inPlace {
		staging, err := os.MkdirTemp("", "goddi-restore-")
		if err != nil {
			return fmt.Errorf("创建临时解包目录：%w", err)
		}
		defer os.RemoveAll(staging)
		extractTo = filepath.Join(staging, "archive")
	}

	result, err := backup.ReadSnapshot(backup.SnapshotReadRequest{
		Path:        opts.input,
		KeyMaterial: material,
		ExtractTo:   extractTo,
		Replace:     opts.replace,
	})
	if err != nil {
		return err
	}
	describeSnapshot(result.Manifest, opts.input)
	if mismatched := backup.SnapshotKeyMismatches(result.Manifest, material); len(mismatched) > 0 {
		return keyMismatchError(mismatched)
	}

	// The gate runs before anything is written and on both writing paths, so
	// an archive this binary cannot read is refused on a host that has no
	// databases yet -- the host a restore is prepared on -- and not only on
	// one where a live database happened to be there to compare against.
	fitness, err := assessArchive(result.Manifest)
	if err != nil {
		return err
	}
	describeArchiveFitness(fitness)
	if err := fitness.refusal(); err != nil {
		if !opts.inPlace {
			// The unpack path reads and extracts in one pass, so by the time
			// the gate can judge the manifest the files are already in the
			// directory the operator named. Saying so is better than a bare
			// refusal that leaves them wondering what landed.
			return fmt.Errorf("%w（归档已解包到 %s 以便排查，内容未被改动）", err, result.ExtractedTo)
		}
		return err
	}

	if !opts.inPlace {
		fmt.Printf("已解包到 %s\n", result.ExtractedTo)
		fmt.Println("把它变成一个实例还差三步：把配置指向解包目录、启动服务让迁移把 schema 补齐、再确认密钥材料是写入该归档时的那一份。")
		return nil
	}
	return restoreInPlace(cfg, configPath, result, material)
}

func keyMismatchError(labels []string) error {
	return fmt.Errorf("归档记录的密钥指纹与本机密钥材料不一致（%s）；请用写入该归档时的密钥材料重试",
		strings.Join(labels, ", "))
}

// archiveFitness is what the running binary can make of an archive.
//
// It is a separate value from the manifest because it describes a relationship
// rather than a fact about the file: the same archive is usable by one binary
// and not by another. Reporting the archive and judging its fit are kept apart
// so `--verify` can describe an archive it would refuse to restore.
type archiveFitness struct {
	// ArchiveVersion is the product version the manifest says wrote it.
	ArchiveVersion string

	// VersionKnown is false when either version is not dotted-numeric, which
	// is the case for a development build. The product comparison is then
	// skipped and said out loud, rather than being treated as agreement.
	VersionKnown bool

	// ProductNewer means the archive was written by a newer build than this
	// one.
	ProductNewer bool

	// ProductOlder means the archive is older, which is the ordinary case.
	ProductOlder bool

	// SchemaAhead names stores whose schema in the archive is newer than this
	// binary ships. This is the precise signal: a database migrated further
	// than the binary understands cannot be read by it.
	SchemaAhead []string

	// SchemaBehind names stores the archive is behind on, which the next
	// service start brings forward.
	SchemaBehind []string

	// UnknownStores names stores in the archive this binary has no migrations
	// for. It cannot be judged, and an unjudgeable store is refused rather
	// than assumed readable.
	UnknownStores []string
}

// assessArchive judges an archive against the running binary.
//
// The comparison is against the migrations this binary carries, not against
// the databases this host is currently serving from. The live comparison that
// used to be here was wrong in both directions: it refused the ordinary case
// of restoring last night's backup over a database that had since been
// migrated further, and on a host with no database yet -- which is exactly the
// host a restore is prepared on -- it checked nothing at all.
func assessArchive(manifest backup.SnapshotManifest) (archiveFitness, error) {
	fitness := archiveFitness{ArchiveVersion: manifest.Version}

	if order, ok := compareVersion(manifest.Version, Version); ok {
		fitness.VersionKnown = true
		fitness.ProductNewer = order > 0
		fitness.ProductOlder = order < 0
	}

	ceiling, err := knownSchemaVersions()
	if err != nil {
		return archiveFitness{}, err
	}
	for _, entry := range manifest.Databases {
		known, ok := ceiling[entry.Name]
		if !ok {
			fitness.UnknownStores = append(fitness.UnknownStores, entry.Name)
			continue
		}
		switch {
		case entry.SchemaVersion > known:
			fitness.SchemaAhead = append(fitness.SchemaAhead,
				fmt.Sprintf("%s（归档 %d，本二进制最高 %d）", entry.Name, entry.SchemaVersion, known))
		case entry.SchemaVersion < known:
			fitness.SchemaBehind = append(fitness.SchemaBehind,
				fmt.Sprintf("%s（归档 %d，本二进制最高 %d）", entry.Name, entry.SchemaVersion, known))
		}
	}
	return fitness, nil
}

// refusal is the error this archive deserves, or nil when this binary can use
// it.
//
// Order matters: the schema is the precise signal, so it is reported before
// the product version. The product version is the one a human reads, and it
// would be unhelpful to lead with it when the schema has already said exactly
// which store is too new.
func (f archiveFitness) refusal() error {
	switch {
	case len(f.SchemaAhead) > 0:
		return fmt.Errorf("归档里有存储的 schema 比本二进制自带的更新（%s）；更旧的二进制读不了更新的 schema，请用写入该归档的同一版本或更新的二进制恢复",
			strings.Join(f.SchemaAhead, "、"))
	case len(f.UnknownStores) > 0:
		return fmt.Errorf("归档含有本二进制没有迁移的存储（%s），无法判断它能否被读取；在无法判断时拒绝继续",
			strings.Join(f.UnknownStores, "、"))
	case f.ProductNewer:
		return fmt.Errorf("归档由 GoDDI %s 写入，本二进制是 %s；请用写入该归档的同一版本或更新的二进制恢复",
			f.ArchiveVersion, Version)
	}
	return nil
}

// describeArchiveFitness states the fit in the report, including when it could
// not be established.
func describeArchiveFitness(fitness archiveFitness) {
	if !fitness.VersionKnown {
		fmt.Printf("版本比较已跳过：归档记录为 %q，本二进制为 %q，其中一个不是点分数字版本。是否可用以 schema 版本为准。\n",
			fitness.ArchiveVersion, Version)
	} else if fitness.ProductOlder {
		fmt.Printf("归档由 GoDDI %s 写入，本二进制是 %s（更旧）。\n", fitness.ArchiveVersion, Version)
	}
	if len(fitness.SchemaBehind) > 0 {
		fmt.Printf("归档的 schema 落后于本二进制：%s。下一次启动服务时迁移会把它前移。\n",
			strings.Join(fitness.SchemaBehind, "、"))
	}
	if len(fitness.SchemaAhead) == 0 && len(fitness.UnknownStores) == 0 && !fitness.ProductNewer {
		fmt.Println("本二进制可以读取该归档。")
	}
}

// compareVersion orders two dotted numeric versions.
//
// It reports false when either side is not dotted numeric -- a build stamped
// "dev", or a version carrying a commit suffix. An unreadable version must be
// visible as "compared nothing" rather than silently treated as agreement:
// the caller says so in the report, and the schema comparison is what carries
// the decision.
func compareVersion(left, right string) (int, bool) {
	a, ok := parseVersion(left)
	if !ok {
		return 0, false
	}
	b, ok := parseVersion(right)
	if !ok {
		return 0, false
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		switch {
		case x < y:
			return -1, true
		case x > y:
			return 1, true
		}
	}
	return 0, true
}

// parseVersion reads a dotted run of numbers. It tolerates a leading "v",
// which is how release tags spell the same version, and refuses anything else
// rather than reading the numeric prefix and guessing at the rest.
func parseVersion(value string) ([]int, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	if value == "" {
		return nil, false
	}
	fields := strings.Split(value, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			return nil, false
		}
		for _, char := range field {
			if char < '0' || char > '9' {
				return nil, false
			}
		}
		number, err := strconv.Atoi(field)
		if err != nil {
			return nil, false
		}
		parts = append(parts, number)
	}
	return parts, true
}

// restoreInPlace replaces this deployment's databases with the ones in an
// archive.
//
// It is deliberately narrow. It replaces database files, which is the part an
// operator cannot do by hand without knowing where every store lives. It does
// not apply the configuration files from the archive, because those describe
// the host the archive came from and writing them here would silently move this
// instance's data directory. It does not run migrations either: the restored
// schema is brought forward by the next service start, which is the same path
// an upgrade takes and therefore the one that is actually exercised.
//
// Everything that can be checked is checked before anything is written, and a
// consistent copy of the current databases is taken first when there is
// anything to copy, so the one failure mode that remains -- the disk filling up
// halfway through -- still has a way back that is stated in the error.
func restoreInPlace(cfg *config.Config, configPath string, result *backup.SnapshotReadResult, material string) error {
	if len(result.Manifest.Databases) == 0 {
		return errors.New("该归档不含任何数据库，无法就地恢复")
	}
	dataDir := strings.TrimSpace(cfg.Server.DataDir)
	if dataDir == "" {
		return errors.New("server.data_dir 未配置，无法确定替换前副本的存放位置")
	}

	// Empty when there was nothing to preserve, which is why the failure
	// message below says so rather than pointing at a file that was never
	// written.
	safety := ""

	// A safety copy exists to preserve what is about to be replaced, so it is
	// taken only when there is something to preserve.
	//
	// This is the "recover onto a new instance" case, which is the reason an
	// archive exists at all. The copy used to be unconditional, and because
	// snapshotTargets marks the control database as mandatory, --in-place on a
	// host that had never run failed before replacing anything -- reporting a
	// missing database the operator never had. The fresh host is where the
	// command matters most and it was the one place it could not run.
	if existing := existingStores(cfg); len(existing) == 0 {
		fmt.Println("目标主机上没有任何库，没有需要留存的替换前副本")
	} else {
		safety = filepath.Join(dataDir, "backups", "prerestore_"+time.Now().Format("20060102_150405")+backup.SnapshotFileExt)
		if _, err := backup.CreateSnapshot(backup.SnapshotRequest{
			OutputPath:     safety,
			Version:        Version,
			Description:    "就地恢复前的自动副本",
			Targets:        existing,
			ConfigFiles:    []backup.ConfigSource{{Path: configPath, Optional: true}},
			KeyMaterial:    material,
			AllowPlaintext: material == "",
		}); err != nil {
			return fmt.Errorf("替换前复制当前数据库失败，已放弃恢复：%w", err)
		}
		fmt.Printf("替换前已留存当前数据库副本：%s\n", safety)
	}

	locations := map[string]string{}
	for _, target := range snapshotTargets(cfg) {
		locations[target.Name] = backup.DSNPath(target.DSN)
	}

	type step struct {
		name  string
		live  string
		image string

		// The two schema versions are carried so the swap is reported with
		// both numbers. Whether this binary can read the archive was already
		// decided, and it was decided against the migrations this binary
		// carries rather than against this file: an archive older than the
		// live database is the ordinary "restore last night's backup" case,
		// and refusing it -- which this code used to do -- was a bug.
		archiveVersion int64
		liveVersion    int64
		hadLive        bool
	}
	plan := make([]step, 0, len(result.Manifest.Databases))
	inArchive := map[string]bool{}
	for _, database := range result.Manifest.Databases {
		inArchive[database.Name] = true
		live, ok := locations[database.Name]
		if !ok || live == "" {
			return fmt.Errorf("归档含有 %s 库，而本部署没有为它配置存放位置", database.Name)
		}
		image := filepath.Join(result.ExtractedTo, filepath.FromSlash(database.Member))
		if _, err := os.Stat(image); err != nil {
			return fmt.Errorf("归档成员 %s 不在解包结果里：%w", database.Member, err)
		}
		entry := step{name: database.Name, live: live, image: image, archiveVersion: database.SchemaVersion}
		if _, err := os.Stat(live); err == nil {
			version, _, err := backup.SchemaVersionOf(live)
			if err != nil {
				return fmt.Errorf("读取当前 %s 库的 schema 版本：%w", database.Name, err)
			}
			entry.liveVersion = version
			entry.hadLive = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("检查当前 %s 库：%w", database.Name, err)
		}
		plan = append(plan, entry)
	}

	for _, entry := range plan {
		if entry.hadLive && entry.liveVersion != entry.archiveVersion {
			fmt.Printf("   %s：本机原为 schema %d，归档为 %d，下一次启动会把它前移到本二进制支持的版本\n",
				entry.name, entry.liveVersion, entry.archiveVersion)
		}
	}

	// A store the archive does not hold is left exactly as it is rather than
	// emptied: "the archive has no lease store" means the host it came from had
	// none, not that this host's should be deleted. Said out loud, because an
	// operator expecting a full replacement would otherwise not notice.
	for name, live := range locations {
		if inArchive[name] || live == "" {
			continue
		}
		if _, err := os.Stat(live); err == nil {
			fmt.Printf("注意：归档不含 %s 库，本机的 %s 保持原样\n", name, live)
		}
	}

	var replaced []string
	for _, entry := range plan {
		if err := replaceDatabaseFile(entry.image, entry.live); err != nil {
			return fmt.Errorf("替换 %s 库失败（已替换：%s；替换前副本：%s）：%w",
				entry.name, orNone(replaced), orAbsent(safety), err)
		}
		replaced = append(replaced, entry.name)
		fmt.Printf("已替换 %-8s ← %s\n", entry.name, entry.live)
	}

	fmt.Println("数据库已替换。归档中的配置文件刻意未应用——它描述的是归档来源主机，请自行比对后再决定是否采用。")
	fmt.Println("下一步：启动服务，让迁移把 schema 补齐到本二进制支持的版本。")
	return nil
}

// existingStores lists the stores this deployment has a file for.
//
// The mandate is dropped on purpose. snapshotTargets marks the control
// database as mandatory because a *backup* that silently omits it is the worst
// possible outcome of a command that reported success. A safety copy is the
// opposite situation: it records what is about to be replaced, and the honest
// answer on a host that has never run is "nothing".
func existingStores(cfg *config.Config) []backup.SnapshotTarget {
	var found []backup.SnapshotTarget
	for _, target := range snapshotTargets(cfg) {
		path := backup.DSNPath(target.DSN)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			// Not there, or not readable: either way there is nothing to
			// preserve, and the replace step reports the real problem if there
			// is one.
			continue
		}
		target.Optional = true
		found = append(found, target)
	}
	return found
}

// replaceDatabaseFile puts a restored database in place of a live one.
//
// The copy lands beside its destination and is renamed onto it, so the swap is
// atomic and cannot cross a filesystem boundary.
//
// The write-ahead log and shared-memory files next to the old database are
// removed first, because they belong to the file being replaced. This matters
// in one case: while another connection is still attached -- which is what an
// operator who ran --in-place without stopping the service leaves behind --
// SQLite cannot remove the log itself, and a log sitting beside a database it
// does not belong to is a corruption waiting for the next reader that adopts
// WAL mode, which the service does on startup. When nothing else is attached
// the log is already checkpointed away by the checks this function's caller
// ran, and the removal is a no-op.
func replaceDatabaseFile(image, live string) error {
	if err := os.MkdirAll(filepath.Dir(live), 0o755); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(live + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 %s%s：%w", live, suffix, err)
		}
	}
	temporary := live + ".restore-new"
	if err := copyRegularFile(image, temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, live); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}

func copyRegularFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func orNone(names []string) string {
	if len(names) == 0 {
		return "无"
	}
	return strings.Join(names, ", ")
}

// orAbsent names a path that may legitimately not exist, so a failure message
// does not send the operator looking for a file that was never written.
func orAbsent(path string) string {
	if strings.TrimSpace(path) == "" {
		return "无（替换前没有任何库）"
	}
	return path
}

// describeSnapshot prints what an archive holds. Every line comes from the
// manifest, so this is also the report of a verify-only read: what the operator
// sees is what the file claims, and the read has already checked every claim.
func describeSnapshot(manifest backup.SnapshotManifest, path string) {
	fmt.Printf("归档：%s\n", path)
	fmt.Printf("  格式版本 %d · 加密 %s · 创建于 %s\n",
		manifest.FormatVersion, manifest.Encryption, manifest.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	if manifest.Description != "" {
		fmt.Printf("  说明：%s\n", manifest.Description)
	}
	fmt.Println("  数据库：")
	for _, database := range manifest.Databases {
		fmt.Printf("    %-8s %10s  schema %-9s 表 %-4d ← %s\n",
			database.Name, humanBytes(database.SizeBytes), revisionLabel(database), database.Tables, database.Source)
	}
	for _, absent := range manifest.Absent {
		fmt.Printf("    %-8s %10s  %s ← %s\n", absent.Name, "不存在", reasonLabel(absent.Reason), absent.Source)
	}
	if len(manifest.ConfigFiles) > 0 {
		fmt.Println("  配置文件：")
		for _, file := range manifest.ConfigFiles {
			fmt.Printf("    %-22s %10s ← %s\n", file.Member, humanBytes(file.SizeBytes), file.Source)
		}
	}
	if len(manifest.Secrets.Inventory) > 0 {
		fmt.Println("  密钥盘点（只计数，不含值）：")
		for _, entry := range manifest.Secrets.Inventory {
			fmt.Printf("    %-24s %5d 条，其中已加密 %d 条\n",
				entry.Table+"."+entry.Column, entry.Rows, entry.Sealed)
		}
	}
	for _, label := range backup.SnapshotKeyLabels(manifest) {
		fmt.Printf("  密钥指纹 %s = %s\n", label, manifest.Secrets.Keys[label])
	}
	if manifest.Secrets.Note != "" {
		fmt.Printf("  注意：%s\n", manifest.Secrets.Note)
	}
}

// revisionLabel distinguishes a database that has never been migrated from one
// sitting at revision zero, which is the difference a restore cares about.
func revisionLabel(database backup.SnapshotDatabase) string {
	if !database.SchemaTablePresent {
		return "无迁移表"
	}
	return strconv.FormatInt(database.SchemaVersion, 10)
}

// reasonLabel renders the manifest's machine-readable reason for a human. The
// manifest keeps the English value: it is a field in a file that may be carried
// to a host where nothing else about this project is installed.
func reasonLabel(reason string) string {
	switch reason {
	case "no such file":
		return "文件不存在"
	default:
		return reason
	}
}

func humanBytes(size int64) string {
	switch {
	case size >= 1<<30:
		return fmt.Sprintf("%.2f GiB", float64(size)/(1<<30))
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// --- DHCP high availability: the operator actions (ADR 0003) ---

// haNode is an opened lease store and the operator view of it.
type haNode struct {
	Operator *ha.Operator
	Config   ha.Config
	Store    *dataplane.Store
}

// Close releases the store. A node that never opened one closes cleanly.
func (n *haNode) Close() {
	if n != nil && n.Store != nil {
		n.Store.Close()
	}
}

// openHANode resolves this node's configuration, locates its lease store and
// opens it.
//
// write chooses how. The difference is the point: the report opens the file
// without creating it, without applying pragmas and without migrating it,
// because a command whose whole output is a statement about what is on disk
// must not be the thing that changes it.
//
// config.Validate is deliberately not called, for the reason the migration
// report gives: the redundancy of a node whose configuration is already suspect
// is exactly what is worth looking at, and refusing to look because something
// unrelated is wrong is the wrong way round. The HA settings themselves are
// still checked, by FromConfig.
func openHANode(configPath string, write bool) (*haNode, error) {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)

	if !cfg.DHCPHA.Enabled {
		return nil, ha.ErrHAOff
	}
	haCfg, err := ha.FromConfig(cfg.DHCPHA)
	if err != nil {
		return nil, fmt.Errorf("reading the DHCP HA settings: %w", err)
	}

	dsn := cfg.DataPlaneDSN(config.DataPlaneLease)
	path := strings.TrimSpace(backup.DSNPath(dsn))
	if path == "" {
		return nil, fmt.Errorf("the DHCP lease store has no file path (%q), and goddi ha acts on the store this node owns", dsn)
	}
	// Checked before anything is opened. Opening a SQLite path creates the
	// file, so a report run against a node that has never served DHCP would
	// otherwise answer a question about a database it had just brought into
	// existence.
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("this node has never opened its DHCP lease store (%s), so it has no redundancy to report or change", path)
		}
		return nil, fmt.Errorf("reading the DHCP lease store %s: %w", path, err)
	}

	var store *dataplane.Store
	if write {
		store, err = dataplane.Open(config.DataPlaneLease, dsn)
	} else {
		store, err = dataplane.OpenReadOnly(config.DataPlaneLease, dsn)
	}
	if err != nil {
		return nil, err
	}
	return &haNode{Operator: ha.NewOperator(haCfg, store), Config: haCfg, Store: store}, nil
}

// haAge renders how long ago something was, in the words an operator reads.
func haAge(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	return time.Since(at).Round(time.Millisecond).String() + " ago"
}

func runHAStatus(configPath string, asJSON bool) error {
	node, err := openHANode(configPath, false)
	if err != nil {
		return err
	}
	defer node.Close()

	status, err := node.Operator.Status()
	if err != nil {
		return err
	}
	if asJSON {
		encoded, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding the status: %w", err)
		}
		fmt.Println(string(encoded))
		return nil
	}

	fmt.Printf("DHCP HA status for %s\n", status.NodeID)
	fmt.Printf("  configured role:  %s\n", status.ConfiguredRole)
	fmt.Printf("  role in force:    %s\n", status.Role)
	fmt.Printf("  state:            %s\n", status.State)
	// Whether the node has a second copy is a statement about the peer link,
	// and the link is not in the store. Reporting "redundant" from here would
	// be the answer nobody checked.
	switch {
	case status.RedundancyKnown && status.Redundant:
		fmt.Printf("  redundant:        yes\n")
	case status.RedundancyKnown:
		fmt.Printf("  redundant:        no\n")
	default:
		fmt.Printf("  redundant:        unknown from this store\n")
	}
	switch status.Role {
	case config.HARolePrimary:
		fmt.Printf("  sequence:         %d handed out, %d confirmed by the mirror\n", status.Seq, status.AckedSeq)
	case config.HARoleStandby:
		fmt.Printf("  applied sequence: %d\n", status.AppliedSeq)
		fmt.Printf("  primary sequence: %d (heard %s)\n", status.PeerSeq, haAge(status.PeerSeqAt))
		fmt.Printf("  shortfall:        %d\n", status.Gap)
	}
	if status.Degraded {
		fmt.Printf("  permission:       running without a second copy, since %s\n", status.DegradedAt.Format(time.RFC3339))
		fmt.Printf("  reason:           %s\n", status.DegradedReason)
	}
	if !status.FencedAt.IsZero() {
		fmt.Printf("  fenced:           since %s\n", status.FencedAt.Format(time.RFC3339))
	}
	if !status.TakeoverAt.IsZero() {
		fmt.Printf("  promoted:         at %s\n", status.TakeoverAt.Format(time.RFC3339))
		if status.AcceptedGap > 0 {
			// The shortfall is named here rather than in the role switch
			// above, because it belongs to the promotion rather than to what
			// the node is now: the applied watermark that made it computable
			// was cleared by the same transaction that recorded this.
			fmt.Printf("  gave up:          %d change(s) the node it replaced had handed out\n", status.AcceptedGap)
		}
	}
	if !status.RedundancyKnown {
		fmt.Println()
		fmt.Println("  Whether this node has a second copy right now is decided by its peer link, and")
		fmt.Println("  only the running service observes that. It reports the answer in its log and")
		fmt.Println("  on /ready; this report covers what the store can establish on its own.")
	}
	return nil
}

func runHADegrade(configPath string, confirm, undo bool, reason string) error {
	node, err := openHANode(configPath, true)
	if err != nil {
		return err
	}
	defer node.Close()

	before, err := node.Operator.Status()
	if err != nil {
		return err
	}
	if err := node.Operator.Degrade(ha.DegradeOptions{Confirmed: confirm, Reason: reason, Undo: undo}); err != nil {
		return err
	}
	after, err := node.Operator.Status()
	if err != nil {
		return err
	}

	if undo {
		fmt.Printf("goddi ha degrade --undo: %s may no longer run without a second copy\n", after.NodeID)
		fmt.Printf("  state in the store: %s\n", after.State)
		fmt.Println("  effect:  within one heartbeat interval the service stops acknowledging REQUESTs,")
		fmt.Println("           until its mirror is current again. It keeps answering DISCOVER, and it")
		fmt.Println("           keeps honouring releases and declines.")
		return nil
	}

	fmt.Printf("goddi ha degrade: %s may run without a second copy\n", after.NodeID)
	fmt.Printf("  state in the store: %s\n", after.State)
	fmt.Println("  effect:  within one heartbeat interval the service acknowledges REQUESTs again,")
	fmt.Println("           and reports degraded -- not failing -- on /ready and in goddi_dhcp_ha_redundant.")
	fmt.Println("  ends:    when the mirror has caught up, or now with --undo.")
	if before.Seq > 0 && before.AckedSeq >= before.Seq {
		fmt.Println()
		fmt.Printf("  note:  the mirror had already confirmed everything this node handed out\n")
		fmt.Printf("         (%d of %d). If it is still reachable, this permission clears itself on\n", before.AckedSeq, before.Seq)
		fmt.Println("         the next confirmation.")
	}
	return nil
}

func runHATakeover(configPath string, confirm, oldStopped bool, acceptGap int64) error {
	node, err := openHANode(configPath, true)
	if err != nil {
		return err
	}
	defer node.Close()

	out, err := node.Operator.Takeover(ha.TakeoverOptions{
		Confirmed:             confirm,
		OldPrimaryCannotWrite: oldStopped,
		AcceptGap:             acceptGap,
	})
	if err != nil {
		return err
	}

	fmt.Printf("goddi ha takeover: %s is now the primary\n", node.Config.NodeID)
	fmt.Printf("  held before:        %d\n", out.AppliedSeq)
	fmt.Printf("  the primary reached %d, shortfall %d\n", out.PeerSeq, out.Gap)
	fmt.Printf("  sequence resumes:   %d\n", out.Seq)
	fmt.Println()
	fmt.Println("  It starts as primary-degraded: a promotion comes with the permission to serve")
	fmt.Println("  alone, because that is what it is for. That marker clears itself once a mirror")
	fmt.Println("  has caught up.")
	fmt.Println()
	fmt.Println("  next: restart the service. A standby runs no DHCP server, so this starts one")
	fmt.Println("        rather than interrupting one.")
	fmt.Println("  old primary: it must be fenced and rejoined (`goddi ha rejoin --confirm`) before")
	fmt.Println("        it may mirror again; its sequence is kept, so a later promotion of it")
	fmt.Println("        still starts above everything either node has used.")
	return nil
}

func runHAFence(configPath string, confirm bool, reason string) error {
	node, err := openHANode(configPath, true)
	if err != nil {
		return err
	}
	defer node.Close()

	if err := node.Operator.Fence(ha.FenceOptions{Confirmed: confirm, Reason: reason}); err != nil {
		return err
	}

	fmt.Printf("goddi ha fence: %s is taken out of service\n", node.Config.NodeID)
	fmt.Println("  next: restart the service. From then on it neither serves clients nor mirrors.")
	fmt.Println("  until then: the running service is in whatever state its link implies, and a")
	fmt.Println("        node that has lost its mirror cannot acknowledge a binding -- so the window")
	fmt.Println("        is one in which it offers and releases, not one in which it promises.")
	fmt.Println("  reverses: goddi ha rejoin --confirm")
	return nil
}

func runHARejoin(configPath string, confirm bool) error {
	node, err := openHANode(configPath, true)
	if err != nil {
		return err
	}
	defer node.Close()

	if err := node.Operator.Rejoin(ha.RejoinOptions{Confirmed: confirm}); err != nil {
		return err
	}

	fmt.Printf("goddi ha rejoin: %s returns to the pair as a standby\n", node.Config.NodeID)
	fmt.Printf("  peer: %s\n", node.Config.PeerAddress)
	fmt.Println("  next: restart the service. Its leases are replaced by the primary's on connect.")
	fmt.Println("  note: it comes back as a mirror, never as a primary. It has been out of the pair")
	fmt.Println("        while another node was serving, so all it can be trusted to hold is a copy.")
	return nil
}

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

	// The control database lives under the data directory, and SQLite cannot
	// create a file in a directory that is not there -- it reports only
	// "unable to open database file". runServer creates that directory, so a
	// host that has been started once never shows the gap; this command is the
	// documented way to prepare a database, so it has to create it too.
	if dir := strings.TrimSpace(cfg.Server.DataDir); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating data directory %s: %w", dir, err)
		}
	}

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

// migrateStore is one database whose schema this deployment owns and whose
// migrations the migrate command has to be able to report on.
type migrateStore struct {
	set  string
	dsn  string
	fsys fs.FS
}

// migrationStores lists the databases this deployment owns.
//
// It mirrors snapshotTargets deliberately: the three stores the archiver
// copies are the three whose schemas have to line up with the binary, and a
// store that could be restored but not reported on would be exactly the gap
// this command exists to close.
func migrationStores(cfg *config.Config) []migrateStore {
	return []migrateStore{
		{set: "control", dsn: cfg.Database.DSN, fsys: goddiassets.Migrations()},
		{set: "dnsdata", dsn: cfg.DataPlaneDSN(config.DataPlaneZone), fsys: dataplane.Migrations()},
		{set: "leases", dsn: cfg.DataPlaneDSN(config.DataPlaneLease), fsys: dataplane.Migrations()},
	}
}

// runMigrationStatus reports every store's migration state and, with check,
// fails when there is anything to apply or anything this build cannot place.
func runMigrationStatus(configPath string, check bool) error {
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)

	// No logger is initialized. This command's output is the report, and log
	// lines interleaved with it would make the report unreadable to both the
	// operator and any script reading it.
	//
	// config.Validate is deliberately not called either, for the same reason
	// the snapshot command skips it: the state of the schema is most worth
	// inspecting on a deployment whose configuration is already suspect, and
	// refusing to look because something unrelated is wrong is the wrong way
	// round.
	fmt.Printf("迁移状态（本二进制 %s）\n", Version)

	toApply, unrecognised, unreadable := 0, 0, 0
	for _, store := range migrationStores(cfg) {
		status := migrationStatusOfStore(store)
		location := status.Path
		if location == "" {
			location = "无存放位置"
		}
		fmt.Printf("  %s  %s\n", status.Set, location)
		fmt.Printf("    %s\n", status.Summary())
		toApply += len(status.Pending)
		unrecognised += len(status.Withdrawn)
		if status.Err != nil {
			unreadable++
		}
	}
	fmt.Printf("\n待应用合计：%d\n", toApply)
	if unrecognised > 0 {
		fmt.Printf("本二进制不认识的版本合计：%d\n", unrecognised)
	}
	if unreadable > 0 {
		fmt.Printf("读取失败的存储合计：%d\n", unreadable)
	}

	if !check {
		return nil
	}
	switch {
	case unreadable > 0:
		return fmt.Errorf("有 %d 个存储无法读取，无法判断迁移状态；请先解决读取失败再运行 migrate", unreadable)
	case unrecognised > 0:
		return fmt.Errorf("有 %d 个已应用的迁移不在本二进制自带之列：该数据库比这个二进制更新，用它继续运行会把 schema 回退到本二进制理解的版本，请改用与其匹配的二进制", unrecognised)
	case toApply > 0:
		return fmt.Errorf("有 %d 个迁移待应用；请运行 goddi migrate 应用它们", toApply)
	}
	fmt.Println("migrate --check：每个存储都不缺迁移，且没有本二进制不认识的版本。")
	return nil
}

// migrationStatusOfStore reads one store's migration state without creating or
// changing anything.
func migrationStatusOfStore(store migrateStore) database.MigrationStatus {
	path := strings.TrimSpace(backup.DSNPath(store.dsn))
	if path == "" {
		status := database.PendingMigrationStatus(store.fsys, store.set)
		status.Configured = false
		return status
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// Never opened: opening a SQLite path creates the file, and a
			// report that brings the database into existence has answered a
			// different question than the one it was asked.
			status := database.PendingMigrationStatus(store.fsys, store.set)
			status.Configured = true
			status.Path = path
			return status
		}
		status := database.PendingMigrationStatus(store.fsys, store.set)
		status.Configured = true
		status.Path = path
		status.Err = err
		return status
	}

	conn, err := sql.Open("sqlite", store.dsn)
	if err != nil {
		status := database.PendingMigrationStatus(store.fsys, store.set)
		status.Configured = true
		status.Path = path
		status.Err = err
		return status
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)

	// No PRAGMAs are applied on this path. `PRAGMA journal_mode` writes to the
	// file header, and the whole job of this command is to report what is on
	// disk rather than to change it.
	status := database.MigrationStatusOf(conn, store.fsys, store.set)
	status.Configured = true
	status.Path = path
	return status
}

// knownSchemaVersions is the highest migration this binary carries for each
// store it knows how to restore.
//
// It returns an error rather than an empty map when a migration set cannot be
// read: a ceiling that is missing because the list failed to load would make
// the restore gate below describe every archive as acceptable, and a gate that
// passes because it could not look is worse than no gate.
func knownSchemaVersions() (map[string]int64, error) {
	sets := []struct {
		set  string
		fsys fs.FS
	}{
		{set: "control", fsys: goddiassets.Migrations()},
		{set: "dnsdata", fsys: dataplane.Migrations()},
		{set: "leases", fsys: dataplane.Migrations()},
	}
	ceiling := make(map[string]int64, len(sets))
	for _, entry := range sets {
		versions, err := database.ShippedMigrations(entry.fsys)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 库自带的迁移列表：%w", entry.set, err)
		}
		if len(versions) == 0 {
			continue
		}
		ceiling[entry.set] = versions[len(versions)-1]
	}
	return ceiling, nil
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

// dataPlaneQuota resolves the configured bounds into the shape the data plane
// checks against. A bound the operator disabled resolves to zero, which is
// how the data plane is told not to look at it.
// securityKeyMaterial resolves what sealed values are encrypted with.
//
// A dedicated security.encryption_key is the intended configuration. Falling
// back to the JWT secret keeps an installation that predates the setting
// running unchanged, and costs coupling: rotating the JWT secret would then
// make every sealed value unreadable, and the only way back is a rekey while
// both keys are known. Validate() refuses the fallback once the deployment
// declares itself production, so the coupling survives only where it is cheap
// to repair.
func securityKeyMaterial(cfg *config.Config) string {
	if cfg.Security.EncryptionKey != "" {
		return cfg.Security.EncryptionKey
	}
	slog.Warn("security.encryption_key is not set; stored secrets are sealed with the JWT secret as key material. Set security.encryption_key and run `goddi rekey` to decouple them.")
	return cfg.Security.JWTSecret
}

func dataPlaneQuota(cfg *config.Config) dataplane.Quota {
	q := cfg.EffectiveQuota()
	return dataplane.Quota{
		MaxLeases:    q.MaxLeases,
		MaxFileBytes: q.MaxStoreBytes,
		MaxBacklog:   q.MaxBacklog,
	}
}

// currentHAState is the redundancy state of this process, derived when it is
// asked rather than recorded at each transition.
//
// One function rather than one per consumer: the probe and the metric answer
// different questions about the same fact, and a second derivation of it is how
// the two come to report different states for one node.
func currentHAState(role string, repl *ha.Replicator, mirror *ha.Mirror) ha.State {
	switch {
	case repl != nil:
		return repl.State()
	case mirror != nil:
		return mirror.State()
	case role == ha.RoleFenced:
		return ha.StateFenced
	default:
		return ha.StateSolo
	}
}

// mergeHAStatus folds this node's redundancy into the DHCP plane's readiness.
//
// Nothing here produces failing. Only failing is a 503, and every state below
// is one in which the process is doing what it was told to: a paused primary is
// still answering DISCOVER and still honouring releases, a degraded one is
// serving on an operator's permission, and a fenced one is out of service on
// purpose. Reporting any of them as failing would ask an orchestrator to
// restart a process whose problem is not in the process.
func mergeHAStatus(st *handler.PlaneStatus, role string, repl *ha.Replicator, mirror *ha.Mirror) {
	state := currentHAState(role, repl, mirror)
	details := map[string]any{
		"ha_role":  role,
		"ha_state": state.String(),
	}
	if repl != nil {
		details["ha_seq"] = repl.Seq()
		details["ha_acked_seq"] = repl.AckedSeq()
		if up, lastSeen := repl.LinkSnapshot(); up {
			details["ha_peer_last_seen"] = lastSeen
		}
	}
	if mirror != nil {
		details["ha_applied_seq"] = mirror.AppliedSeq()
		if up, lastSeen := mirror.LinkSnapshot(); up {
			details["ha_peer_last_seen"] = lastSeen
		}
	}

	switch state {
	case ha.StatePrimary:
		// A second copy is present and this node may promise. Nothing to add.
	case ha.StatePrimaryDegraded:
		details["ha_redundant"] = false
		st.Level = handler.Worse(st.Level, handler.LevelDegraded)
		st.Reasons = append(st.Reasons, "dhcp_ha_serving_without_a_second_copy")
		if repl != nil {
			details["ha_degraded_reason"] = repl.DegradedReason()
		}
	case ha.StatePaused:
		details["ha_redundant"] = false
		st.Level = handler.Worse(st.Level, handler.LevelDegraded)
		st.Reasons = append(st.Reasons, "dhcp_ha_no_second_copy")
	case ha.StateStandby:
		// Whether the pair is redundant is answered from the primary, which is
		// the node that owns the fact. What a standby can say about itself is
		// whether it can still see the node whose promises it holds -- and
		// losing that is what stops it being useful.
		details["ha_redundant"] = false
		if up, _ := mirror.LinkSnapshot(); !up {
			st.Level = handler.Worse(st.Level, handler.LevelDegraded)
			st.Reasons = append(st.Reasons, "dhcp_ha_primary_unreachable")
		} else {
			details["ha_redundant"] = true
		}
	case ha.StateFenced:
		details["ha_redundant"] = false
		st.Level = handler.Worse(st.Level, handler.LevelDegraded)
		st.Reasons = append(st.Reasons, "dhcp_ha_fenced")
	}
	st.Details = mergeDetails(st.Details, details)
}

// mergeDetails folds this node's redundancy figures into the payload the
// authenticated detail endpoint serves.
func mergeDetails(existing any, extra map[string]any) map[string]any {
	if m, ok := existing.(map[string]any); ok {
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	return extra
}

// dataPlaneStatus renders a runner's readiness for the API layer, including
// the figures the authenticated endpoint serves.
//
// A process that serves a plane but has no runner is reported failing, not
// ok: it has nothing to report, and "ok" from a probe that checked nothing is
// the answer this endpoint exists to avoid.
func dataPlaneStatus(r *dataplane.Runner) handler.PlaneStatus {
	if r == nil {
		return handler.PlaneStatus{
			Level:   handler.LevelFailing,
			Reasons: []string{"data_plane_not_configured"},
		}
	}

	rd := r.Probe()
	st := handler.PlaneStatus{Level: string(rd.Level), Reasons: rd.Reasons}
	st.Details = dataPlaneDetails(r, rd)
	return st
}

// pendingQueues renders the upward queues for the detail endpoint. The names
// come from the data plane rather than from a list here: the detail payload,
// the Prometheus sample and the quota bounds all describe the same set, and a
// second copy is how a queue ends up missing from one of them.
func pendingQueues(status dataplane.Status) map[string]int {
	queues := status.PendingByQueue()
	out := make(map[string]int, len(queues))
	for _, q := range queues {
		out[q.Name] = q.Pending
	}
	return out
}

// dataPlaneDetails is the payload of the authenticated detail endpoint. The
// unauthenticated /ready never builds it.
func dataPlaneDetails(r *dataplane.Runner, rd dataplane.Readiness) map[string]any {
	status := rd.Status
	details := map[string]any{
		"control_reachable": status.ControlReachable,
		"pending":           pendingQueues(status),
		"held_leases":       status.HeldLeases,
		"refused_rows":      status.RefusedRows,
		"probed_at":         rd.At,
	}
	if status.LastError != "" {
		details["last_error"] = status.LastError
	}
	if !status.LastSuccess.IsZero() {
		details["last_success"] = status.LastSuccess
	}
	if n, err := r.Store().FileSize(); err == nil {
		details["store_bytes"] = n
	}
	if len(rd.Quota) > 0 {
		breaches := make([]string, 0, len(rd.Quota))
		for _, b := range rd.Quota {
			breaches = append(breaches, b.String())
		}
		details["over_quota"] = breaches
	}
	return details
}

// dataPlaneSample renders a runner's probe view for Prometheus. It reads the
// same memory-only view the readiness probe does, so the ten-second metrics
// tick never touches the store.
func dataPlaneSample(plane string, r *dataplane.Runner) metrics.DataPlaneSample {
	rd := r.Probe()
	status := rd.Status

	sample := metrics.DataPlaneSample{
		Plane:       plane,
		Level:       string(rd.Level),
		HeldLeases:  status.HeldLeases,
		Pending:     pendingQueues(status),
		RefusedRows: status.RefusedRows,
		QuotaRatio:  map[string]float64{},
	}
	if n, err := r.Store().FileSize(); err == nil {
		sample.StoreBytes = n
	}

	// The ratio is how close each bound is, not only whether it has been
	// crossed: an alert written against "over the bound" fires after the
	// fact, and one written against the ratio can fire before it.
	q := r.Quota()
	if q.MaxLeases > 0 {
		sample.QuotaRatio["leases"] = float64(status.HeldLeases) / float64(q.MaxLeases)
	}
	if q.MaxFileBytes > 0 && sample.StoreBytes > 0 {
		sample.QuotaRatio["store_bytes"] = float64(sample.StoreBytes) / float64(q.MaxFileBytes)
	}
	if q.MaxBacklog > 0 {
		for queue, n := range sample.Pending {
			sample.QuotaRatio["queue_"+queue] = float64(n) / float64(q.MaxBacklog)
		}
	}
	return sample
}
