package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/backup"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
	"github.com/jasonwa/goddi/internal/dhcp/ha"
)

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
	if err := config.ValidateEdition(cfg); err != nil {
		return nil, err
	}
	if write {
		if err := rejectEnterprise(cfg); err != nil {
			return nil, err
		}
	}

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
		fmt.Printf("  facts applied:    %d\n", status.FactsAppliedSeq)
		fmt.Printf("  primary facts:    %d\n", status.PeerFactsSeq)
		fmt.Printf("  facts shortfall:  %d\n", status.FactsGap)
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
	fmt.Printf("  facts watermark:     %d of %d, shortfall %d\n", out.FactsAppliedSeq, out.PeerFactsSeq, out.FactsGap)
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
