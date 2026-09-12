package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func baseConfig() *Config {
	cfg := DefaultConfig()
	cfg.Security.JWTSecret = "a-secret-long-enough"
	cfg.DNS.Enabled = true
	cfg.DHCP.Enabled = true
	return cfg
}

func TestEffectiveRoleDefaultsToAll(t *testing.T) {
	cfg := baseConfig()
	cfg.Server.Role = ""
	if got := cfg.EffectiveRole(); got != RoleAll {
		t.Fatalf("EffectiveRole() = %q, want %q", got, RoleAll)
	}
	cfg.Server.Role = "  ALL "
	if got := cfg.EffectiveRole(); got != RoleAll {
		t.Fatalf("EffectiveRole() = %q, want %q (trimmed and lowercased)", got, RoleAll)
	}
}

// TestRolePlaneMatrix pins which planes each role runs. Getting this wrong in
// either direction is silent: a control-only node that also listens on :53
// fights the real DNS node, and a dns-only node that starts the API exposes the
// management plane on every host running a resolver.
func TestRolePlaneMatrix(t *testing.T) {
	cases := []struct {
		role               string
		control, dns, dhcp bool
	}{
		{RoleAll, true, true, true},
		{RoleControl, true, false, false},
		{RoleDNS, false, true, false},
		{RoleDHCP, false, false, true},
	}
	for _, c := range cases {
		cfg := baseConfig()
		cfg.Server.Role = c.role
		if got := cfg.IsControlPlane(); got != c.control {
			t.Errorf("role %s: IsControlPlane() = %v, want %v", c.role, got, c.control)
		}
		if got := cfg.ServesDNS(); got != c.dns {
			t.Errorf("role %s: ServesDNS() = %v, want %v", c.role, got, c.dns)
		}
		if got := cfg.ServesDHCP(); got != c.dhcp {
			t.Errorf("role %s: ServesDHCP() = %v, want %v", c.role, got, c.dhcp)
		}
	}
}

func TestDataPlaneDSNResolution(t *testing.T) {
	cfg := baseConfig()
	cfg.Server.DataDir = "/var/lib/goddi"

	if got, want := cfg.DataPlaneDSN(DataPlaneLease), "/var/lib/goddi/leases.db"; got != want {
		t.Errorf("lease DSN = %q, want %q", got, want)
	}
	if got, want := cfg.DataPlaneDSN(DataPlaneZone), "/var/lib/goddi/dnsdata.db"; got != want {
		t.Errorf("zone DSN = %q, want %q", got, want)
	}

	cfg.DataPlane.LeaseDSN = "/srv/leases/leases.db"
	cfg.DataPlane.ZoneDSN = " /srv/zones.db "
	if got, want := cfg.DataPlaneDSN(DataPlaneLease), "/srv/leases/leases.db"; got != want {
		t.Errorf("override lease DSN = %q, want %q", got, want)
	}
	if got, want := cfg.DataPlaneDSN(DataPlaneZone), "/srv/zones.db"; got != want {
		t.Errorf("override zone DSN = %q, want %q (trimmed)", got, want)
	}
}

func TestEffectiveSyncInterval(t *testing.T) {
	cfg := baseConfig()
	if got := cfg.EffectiveSyncInterval(); got != DefaultSyncIntervalSeconds {
		t.Errorf("default sync interval = %d, want %d", got, DefaultSyncIntervalSeconds)
	}
	cfg.DataPlane.SyncIntervalSeconds = 30
	if got := cfg.EffectiveSyncInterval(); got != 30 {
		t.Errorf("configured sync interval = %d, want 30", got)
	}
}

// TestTheSyncIntervalDefaultMeetsTheDdnsBound is why the default is not five.
//
// The interval appears in the delay between a DHCP client being answered and
// its own name resolving, because the record is authored by the plane that
// serves it: the entry is pushed up by the DHCP plane and pulled down by the
// DNS plane, and the consumer that turns it into a record drains every second.
// The wait on the near side is not the interval -- a queued change wakes that
// loop -- so the arithmetic is (interval + 1s drain), against the Phase 0 exit
// condition of two seconds.
//
// It is a test rather than a comment because five is the value someone would
// restore as "less chatty" without noticing what it costs: nothing else in the
// suite would fail, and the symptom would appear in production as a name that
// resolves several seconds after the client is online.
func TestTheSyncIntervalDefaultMeetsTheDdnsBound(t *testing.T) {
	const (
		consumerDrainSeconds = 1 // dataplane's DNS consumer drains every second
		ddnsBoundSeconds     = 2 // the Phase 0 exit condition
	)
	worst := DefaultSyncIntervalSeconds + consumerDrainSeconds
	if worst > ddnsBoundSeconds {
		t.Fatalf("default sync interval = %ds: with the consumer's %ds drain a published name "+
			"would take up to %ds, over the %ds the exit condition allows",
			DefaultSyncIntervalSeconds, consumerDrainSeconds, worst, ddnsBoundSeconds)
	}
}

// TestTheShippedConfigAgreesWithTheIntervalDefault closes the trap that a
// constant alone cannot: installs run with the file, not with the constant.
// Changing DefaultSyncIntervalSeconds while the shipped config.yaml still says
// five would leave every real deployment on five seconds while this suite
// reported the new value as verified.
func TestTheShippedConfigAgreesWithTheIntervalDefault(t *testing.T) {
	path := filepath.Join("..", "..", "config.yaml")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree", path)
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var shipped Config
	if err := yaml.Unmarshal(raw, &shipped); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if got := shipped.DataPlane.SyncIntervalSeconds; got != DefaultSyncIntervalSeconds {
		t.Fatalf("the shipped config.yaml sets sync_interval_seconds to %d while the default "+
			"is %d: which one an installation gets would depend on whether the file was "+
			"edited, and the tests would verify the wrong one", got, DefaultSyncIntervalSeconds)
	}
}

// TestValidateRoleRefusesNothingToDo turns a silent misconfiguration into a
// startup error: a process told to be a DNS node with DNS switched off would
// otherwise start, report healthy and serve nothing at all.
func TestValidateRoleRefusesNothingToDo(t *testing.T) {
	cfg := baseConfig()
	cfg.Server.Role = RoleDNS
	cfg.DNS.Enabled = false
	if err := ValidateRole(cfg); err == nil {
		t.Fatal("a dns role with dns.enabled=false was accepted")
	}

	cfg = baseConfig()
	cfg.Server.Role = RoleDHCP
	cfg.DHCP.Enabled = false
	if err := ValidateRole(cfg); err == nil {
		t.Fatal("a dhcp role with dhcp.enabled=false was accepted")
	}

	cfg = baseConfig()
	cfg.Server.Role = "resolver"
	if err := ValidateRole(cfg); err == nil {
		t.Fatal("an unknown role was accepted")
	} else if !strings.Contains(err.Error(), "resolver") {
		t.Errorf("error does not name the bad role: %v", err)
	}

	// A data-plane role still needs somewhere to put its store.
	cfg = baseConfig()
	cfg.Server.Role = RoleDHCP
	cfg.Server.DataDir = ""
	if err := ValidateRole(cfg); err == nil {
		t.Fatal("a dhcp role with no data_dir and no lease_dsn was accepted")
	}
	cfg.DataPlane.LeaseDSN = "/srv/leases.db"
	if err := ValidateRole(cfg); err != nil {
		t.Errorf("an explicit lease DSN should satisfy the requirement: %v", err)
	}
}

func TestValidateAcceptsEveryValidRole(t *testing.T) {
	for _, role := range []string{"", RoleAll, RoleControl, RoleDNS, RoleDHCP} {
		cfg := baseConfig()
		cfg.Server.Role = role
		if err := Validate(cfg); err != nil {
			t.Errorf("role %q: Validate() = %v", role, err)
		}
	}
}

// TestEffectiveQuotaResolvesZeroAsDefaultAndNegativeAsOff pins the two
// spellings apart. They are different intentions: an operator who leaves the
// block out gets the documented ceiling, and one who writes -1 has decided
// not to have one. Collapsing them would mean "unset" silently disabling a
// bound, which is the direction an omission should never take.
func TestEffectiveQuotaResolvesZeroAsDefaultAndNegativeAsOff(t *testing.T) {
	def := baseConfig().EffectiveQuota()
	if def.MaxLeases != DefaultMaxLeases {
		t.Errorf("an unset lease ceiling = %d, want %d", def.MaxLeases, DefaultMaxLeases)
	}
	if def.MaxStoreBytes != int64(DefaultMaxStoreMB)<<20 {
		t.Errorf("an unset store ceiling = %d bytes, want %d MiB", def.MaxStoreBytes, DefaultMaxStoreMB)
	}
	if def.MaxBacklog != DefaultMaxBacklog {
		t.Errorf("an unset backlog ceiling = %d, want %d", def.MaxBacklog, DefaultMaxBacklog)
	}

	cfg := baseConfig()
	cfg.DataPlane.Quota = QuotaConfig{MaxLeases: -1, MaxStoreMB: -1, MaxBacklog: -1}
	off := cfg.EffectiveQuota()
	if off.MaxLeases != 0 || off.MaxStoreBytes != 0 || off.MaxBacklog != 0 {
		t.Errorf("a negative bound resolved to %+v, want every bound disabled", off)
	}

	cfg.DataPlane.Quota = QuotaConfig{MaxLeases: 123, MaxStoreMB: 7, MaxBacklog: 45}
	set := cfg.EffectiveQuota()
	if set.MaxLeases != 123 || set.MaxStoreBytes != 7<<20 || set.MaxBacklog != 45 {
		t.Errorf("explicit bounds resolved to %+v, want 123 leases, 7 MiB, 45", set)
	}
}

// TestTheShippedConfigLeavesTheQuotaAtItsDefaults is the same trap the sync
// interval has: the bounds a deployment actually runs with are the ones in
// the file, so a file that pins its own numbers would make every test that
// verifies the constant verify something no installation sees.
func TestTheShippedConfigLeavesTheQuotaAtItsDefaults(t *testing.T) {
	path := filepath.Join("..", "..", "config.yaml")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree", path)
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var shipped Config
	if err := yaml.Unmarshal(raw, &shipped); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	got := shipped.EffectiveQuota()
	want := baseConfig().EffectiveQuota()
	if got != want {
		t.Fatalf("the shipped config.yaml resolves its quota to %+v while the defaults "+
			"are %+v: the file is the source an installation reads", got, want)
	}
}
