package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jincaiw/GoDDI/internal/config"
	"github.com/spf13/cobra"
)

var (
	// Build information, set at compile time via ldflags.
	Version   = "v0.25.1"
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
			"  --accept-gap <n>          the exact number of lease changes the primary handed out that this\n" +
			"                            node does not hold, when there are any. A binding is\n" +
			"                            acknowledged only once the standby has applied it, so those\n" +
			"                            changes were never acknowledged -- but the number says how\n" +
			"                            far the two nodes had drifted, and it has to be read before\n" +
			"                            it is accepted.\n" +
			"  facts gap                cannot be accepted: restore the missing snapshot first.\n" +
			"The promotion takes effect at the next start. A standby runs no DHCP server, so that\n" +
			"is not an interruption of anything being served: it is the start of one.\n" +
			"After the restart this node serves clients from its own configuration: DHCP scope\n" +
			"definitions are NOT replicated by the lease mirror (config coordination across\n" +
			"nodes is ADR-0005, not implemented) -- recreate the scopes on this node before\n" +
			"putting it on the client path. (Container exercise round 247: a DISCOVER after\n" +
			"takeover answered 'no matching scope' until the scope was recreated.)",
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

// ErrEnterpriseUnavailable is a temporary fail-closed boundary. The edition
// selector grants no role, and Enterprise DNS availability is not implemented.
var ErrEnterpriseUnavailable = errors.New("enterprise edition is unavailable; no service or writes started")

func rejectEnterprise(cfg *config.Config) error {
	if err := config.ValidateEdition(cfg); err != nil {
		return err
	}
	if cfg.Server.Edition == "enterprise" {
		return ErrEnterpriseUnavailable
	}
	return nil
}
