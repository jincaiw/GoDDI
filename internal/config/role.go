package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Roles a GoDDI process can run.
//
// A single binary serves every role, so an operator can start one process per
// plane and give each its own supervision, its own restart policy and its own
// failure domain — or run everything in one process for a lab.
const (
	// RoleAll runs the management plane and both data planes in one process.
	// It is the default and what every installation before this change was
	// doing.
	RoleAll = "all"
	// RoleControl runs the management plane only: API, web console, IPAM,
	// migrations and the configuration replica publisher.
	RoleControl = "control"
	// RoleDNS runs the authoritative DNS listener and its replication from
	// the control database. It serves no management API.
	RoleDNS = "dns"
	// RoleDHCP runs the DHCP listener and its replication from the control
	// database. It serves no management API.
	RoleDHCP = "dhcp"
)

// DataPlaneConfig configures the local stores the data planes own.
//
// The DNS and DHCP roles serve from files they own rather than reading the
// control database on the request path. That is what lets resolution and lease
// renewal continue while the management process is being restarted, upgraded,
// or is simply down: a client asking to renew must not be told to wait for a
// web console.
//
// There is deliberately no switch to turn the split off. A second mode in which
// the data plane reads the control database directly would be a code path that
// production never exercises, and the whole point of this change is that the
// path being exercised is the one that has to survive a control-plane outage.
type DataPlaneConfig struct {
	// LeaseDSN overrides the DHCP data-plane store. Empty means
	// <server.data_dir>/leases.db.
	LeaseDSN string `yaml:"lease_dsn" validate:"omitempty"`
	// ZoneDSN overrides the DNS data-plane store. Empty means
	// <server.data_dir>/dnsdata.db.
	ZoneDSN string `yaml:"zone_dsn" validate:"omitempty"`
	// SyncIntervalSeconds is how often a data plane polls the control
	// database for changes it cannot observe directly. Zero means the default
	// of 1.
	//
	// It is a latency setting on the client's own path, not only a
	// configuration-drift setting. A record derived from a DHCP binding is
	// written by the plane that serves it, so the entry crosses the control
	// database to get there and this interval is paid twice: once for the
	// DHCP plane's push, once for the DNS plane's pull. The hand-off that
	// does not have to wait for a poll -- a change a process makes to its own
	// store -- wakes its loop instead; see dataplane.Runner.Wake.
	SyncIntervalSeconds int `yaml:"sync_interval_seconds" validate:"omitempty,min=1"`

	// Quota bounds the local stores' footprint. Zero values mean the
	// defaults; a negative value disables that bound.
	//
	// Crossing a bound never refuses a client or a write -- see
	// dataplane.Quota -- so these are thresholds for alerting, and they are
	// set where "something is wrong" is more likely than "this is the busy
	// season". A bound that fires during normal operation is one that gets
	// muted, which is worse than not having it.
	Quota QuotaConfig `yaml:"quota"`
}

// QuotaConfig is the operator-facing form of the data-plane bounds.
//
// It stays plain numbers rather than the data plane's own type so that this
// package does not depend on the stores; the wiring layer resolves it.
type QuotaConfig struct {
	// MaxLeases is how many leases a store may hold.
	MaxLeases int `yaml:"max_leases"`
	// MaxStoreMB is how large a store's files may grow, in mebibytes.
	MaxStoreMB int `yaml:"max_store_mb"`
	// MaxBacklog is how many changes a single outbound queue may hold.
	MaxBacklog int `yaml:"max_backlog"`
}

// The defaults the zero values resolve to, and the only place they are
// stated. The lease and backlog ceilings are ADR 0001's documented capacity
// for a two-node deployment (twenty thousand leases); the file ceiling is far
// above what that many leases occupy and exists to catch runaway growth, not
// to size a deployment.
const (
	// DefaultMaxLeases is the lease ceiling.
	DefaultMaxLeases = 20000
	// DefaultMaxStoreMB is the store file ceiling, in mebibytes.
	DefaultMaxStoreMB = 1024
	// DefaultMaxBacklog is the per-queue backlog ceiling.
	DefaultMaxBacklog = 10000
)

// QuotaBounds is the resolved quota: plain numbers, in the units the data
// plane's checks expect.
type QuotaBounds struct {
	// MaxLeases and MaxBacklog are counts. Zero disables the check.
	MaxLeases  int
	MaxBacklog int
	// MaxStoreBytes is the file ceiling in bytes. Zero disables the check.
	MaxStoreBytes int64
}

// EffectiveQuota resolves the configured bounds against the defaults.
//
// A zero value means "unset, use the default" and a negative one means
// "disabled". The two are kept distinct because they are different
// intentions: an operator who leaves the block out gets the documented
// ceiling, and one who writes -1 has decided not to have one.
func (c *Config) EffectiveQuota() QuotaBounds {
	resolve := func(v, def int) int {
		switch {
		case v == 0:
			return def
		case v < 0:
			return 0
		default:
			return v
		}
	}

	q := c.DataPlane.Quota
	return QuotaBounds{
		MaxLeases:     resolve(q.MaxLeases, DefaultMaxLeases),
		MaxStoreBytes: int64(resolve(q.MaxStoreMB, DefaultMaxStoreMB)) << 20,
		MaxBacklog:    resolve(q.MaxBacklog, DefaultMaxBacklog),
	}
}

// DefaultSyncIntervalSeconds is used when SyncIntervalSeconds is unset.
//
// One second, not five, because this is one of the two terms bounding how long
// a client waits for its own name to resolve, and the other term -- the DNS
// plane's consumer, which drains every second -- is not configurable. The
// arithmetic is what the exit criterion is checked against: a wake-up push
// (~0) + this interval (≤1s) + the consumer's drain (≤1s) puts a published
// name under two seconds, where a five-second default would put it under
// eleven.
//
// The cost of a poll is a handful of indexed reads on tables that are almost
// always empty, measured in
// TestTheCostOfAnIdlePassAtTheDocumentedLeaseCeiling; it is the reason this
// can be short.
const DefaultSyncIntervalSeconds = 1

// DataPlaneRole is the role a local store belongs to.
type DataPlaneRole string

const (
	// DataPlaneLease is the DHCP data-plane store.
	DataPlaneLease DataPlaneRole = "lease"
	// DataPlaneZone is the DNS data-plane store.
	DataPlaneZone DataPlaneRole = "zone"
)

// EffectiveRole returns the configured role, defaulting to RoleAll.
func (c *Config) EffectiveRole() string {
	role := strings.ToLower(strings.TrimSpace(c.Server.Role))
	if role == "" {
		return RoleAll
	}
	return role
}

// IsControlPlane reports whether this process serves the management API.
func (c *Config) IsControlPlane() bool {
	switch c.EffectiveRole() {
	case RoleAll, RoleControl:
		return true
	default:
		return false
	}
}

// ServesDNS reports whether this process runs the DNS data plane.
func (c *Config) ServesDNS() bool {
	switch c.EffectiveRole() {
	case RoleAll, RoleDNS:
		return true
	default:
		return false
	}
}

// ServesDHCP reports whether this process runs the DHCP data plane.
func (c *Config) ServesDHCP() bool {
	switch c.EffectiveRole() {
	case RoleAll, RoleDHCP:
		return true
	default:
		return false
	}
}

// EffectiveSyncInterval returns the configured poll interval in seconds.
func (c *Config) EffectiveSyncInterval() int {
	if c.DataPlane.SyncIntervalSeconds <= 0 {
		return DefaultSyncIntervalSeconds
	}
	return c.DataPlane.SyncIntervalSeconds
}

// DataPlaneDSN resolves the store location for a role. An explicit override in
// the configuration wins; otherwise the file lives next to the rest of the
// instance's state, so a backup that covers the data directory covers the data
// plane's memory of what it was doing.
func (c *Config) DataPlaneDSN(role DataPlaneRole) string {
	switch role {
	case DataPlaneLease:
		if dsn := strings.TrimSpace(c.DataPlane.LeaseDSN); dsn != "" {
			return dsn
		}
		return filepath.Join(c.Server.DataDir, "leases.db")
	case DataPlaneZone:
		if dsn := strings.TrimSpace(c.DataPlane.ZoneDSN); dsn != "" {
			return dsn
		}
		return filepath.Join(c.Server.DataDir, "dnsdata.db")
	default:
		return ""
	}
}

// ValidateRole rejects a role that would leave the process with nothing to do,
// and one that would have a data plane depend on a control database it was
// never told about.
func ValidateRole(c *Config) error {
	role := c.EffectiveRole()
	switch role {
	case RoleAll, RoleControl, RoleDNS, RoleDHCP:
	default:
		return fmt.Errorf("server.role %q is not one of all, control, dns, dhcp", c.Server.Role)
	}

	// A DNS-only process with DNS disabled, or a DHCP-only process with DHCP
	// disabled, would start, report healthy and serve nothing. Refusing at
	// startup turns a silent misconfiguration into an error the operator sees
	// immediately.
	if role == RoleDNS && !c.DNS.Enabled {
		return fmt.Errorf("server.role is %q but dns.enabled is false: the process would serve nothing", role)
	}
	if role == RoleDHCP && !c.DHCP.Enabled {
		return fmt.Errorf("server.role is %q but dhcp.enabled is false: the process would serve nothing", role)
	}

	if !c.IsControlPlane() {
		if strings.TrimSpace(c.DataPlane.LeaseDSN) == "" && strings.TrimSpace(c.Server.DataDir) == "" {
			return fmt.Errorf("server.data_dir is required to place the local data-plane stores")
		}
	}
	return nil
}
