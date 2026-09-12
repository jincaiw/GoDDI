package config

import (
	"strings"
	"testing"
)

// haConfig returns a configuration with a valid HA pair already configured, so
// that each case below changes exactly one thing.
func haConfig(t *testing.T) *Config {
	t.Helper()
	cfg := secretConfig(t)
	cfg.DHCP.Enabled = true
	cfg.DHCPHA = DHCPHAConfig{
		Enabled:           true,
		NodeID:            "dhcp-a",
		Role:              HARolePrimary,
		ListenAddr:        "10.0.0.1:647",
		PeerAddress:       "10.0.0.2:647",
		PeerToken:         "0123456789abcdef",
		ConfirmTimeout:    "2s",
		HeartbeatInterval: "1s",
		PeerStaleAfter:    "5s",
	}
	return cfg
}

// TestAnEnabledPairRefusesToStartWithoutItsOtherHalf is the fail-closed rule of
// ADR 0003 decision 5's configuration section.
//
// A node that comes up with HA switched on and no reachable peer is the worst
// shape this feature can produce: it believes it has redundancy, it has none,
// and nothing it reports says so. The only outcome an operator can act on is a
// refusal to start that names the setting.
func TestAnEnabledPairRefusesToStartWithoutItsOtherHalf(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		setting string
	}{
		{"no node id", func(c *Config) { c.DHCPHA.NodeID = "  " }, "dhcp_ha.node_id"},
		{"no peer address", func(c *Config) { c.DHCPHA.PeerAddress = "" }, "dhcp_ha.peer_address"},
		{"no listen address", func(c *Config) { c.DHCPHA.ListenAddr = "" }, "dhcp_ha.listen_addr"},
		{"peer address is not host:port", func(c *Config) { c.DHCPHA.PeerAddress = "10.0.0.2" }, "dhcp_ha.peer_address"},
		{"listen address has no port", func(c *Config) { c.DHCPHA.ListenAddr = "10.0.0.1" }, "dhcp_ha.listen_addr"},
		{"port out of range", func(c *Config) { c.DHCPHA.PeerAddress = "10.0.0.2:99999" }, "dhcp_ha.peer_address"},
		{"peer address has no host", func(c *Config) { c.DHCPHA.PeerAddress = ":647" }, "dhcp_ha.peer_address"},
		{"no token", func(c *Config) { c.DHCPHA.PeerToken = "" }, "dhcp_ha.peer_token"},
		{"short token", func(c *Config) { c.DHCPHA.PeerToken = "hunter2" }, "dhcp_ha.peer_token"},
		{"unknown role", func(c *Config) { c.DHCPHA.Role = "witness" }, "dhcp_ha.role"},
		{"confirm timeout is not a duration", func(c *Config) { c.DHCPHA.ConfirmTimeout = "2 seconds" }, "dhcp_ha.confirm_timeout"},
		{"heartbeat is not a duration", func(c *Config) { c.DHCPHA.HeartbeatInterval = "soon" }, "dhcp_ha.heartbeat_interval"},
		{"staleness is not a duration", func(c *Config) { c.DHCPHA.PeerStaleAfter = "-1s" }, "dhcp_ha.peer_stale_after"},
		{
			// If the peer is declared stale while a binding is still waiting
			// for it, the node both withholds the ACK and reports the pair
			// broken -- and the second of those is the one an operator acts on.
			"confirm timeout reaches the staleness bound",
			func(c *Config) { c.DHCPHA.ConfirmTimeout = "5s" },
			"dhcp_ha.confirm_timeout",
		},
		{
			"heartbeat reaches the staleness bound",
			func(c *Config) { c.DHCPHA.HeartbeatInterval = "5s" },
			"dhcp_ha.heartbeat_interval",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := haConfig(t)
			tc.mutate(cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("the configuration was accepted; %s was left unusable", tc.setting)
			}
			if !strings.Contains(err.Error(), tc.setting) {
				t.Errorf("the error does not name %s: %v", tc.setting, err)
			}
		})
	}
}

// TestANodeCannotBeItsOwnPeer covers the one misconfiguration that parses
// perfectly and produces a healthy-looking pair of one.
//
// A node pointed at its own listen address would confirm every binding against
// its own store, report `primary`, and lose every address it ever promised on
// the first power cut -- while telling the operator, in every signal it emits,
// that it has a second copy.
func TestANodeCannotBeItsOwnPeer(t *testing.T) {
	cfg := haConfig(t)
	cfg.DHCPHA.PeerAddress = cfg.DHCPHA.ListenAddr
	err := Validate(cfg)
	if err == nil {
		t.Fatal("a node was allowed to be its own peer")
	}
	if !strings.Contains(err.Error(), "own peer") {
		t.Errorf("the error does not explain the problem: %v", err)
	}
}

// TestADisabledNodeIsNotChecked covers the compatibility half of the contract.
//
// Settings left behind from an earlier experiment must not be able to stop a
// node from starting when HA is off, and nothing reads them in that case. The
// shipped default is the first case here: no dhcp_ha block at all.
func TestADisabledNodeIsNotChecked(t *testing.T) {
	cfg := secretConfig(t)
	cfg.DHCP.Enabled = true
	if err := Validate(cfg); err != nil {
		t.Fatalf("the default configuration was refused: %v", err)
	}

	// Every setting present and wrong, HA off: still fine.
	cfg.DHCPHA = DHCPHAConfig{
		Enabled:    false,
		Role:       "witness",
		PeerToken:  "x",
		ListenAddr: "not a host:port",
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("a disabled node was refused because of settings it does not read: %v", err)
	}
}

// TestTheStandbyRoleIsRecognisedAsSuch pins the predicate the wiring uses to
// decide whether this node is a server at all.
func TestTheStandbyRoleIsRecognisedAsSuch(t *testing.T) {
	cfg := haConfig(t)
	cfg.DHCPHA.Role = HARoleStandby
	if err := Validate(cfg); err != nil {
		t.Fatalf("a standby configuration was refused: %v", err)
	}
	if !cfg.DHCPHA.IsStandby() {
		t.Error("a node configured as a standby does not consider itself one")
	}
	if got := cfg.DHCPHA.HARoleNormalized(); got != HARoleStandby {
		t.Errorf("normalised role = %q, want %q", got, HARoleStandby)
	}

	// The default role is primary, which is what every installation predating
	// this setting is.
	legacy := DefaultDHCPHAConfig()
	legacy.Enabled = true
	legacy.Role = ""
	if legacy.IsStandby() {
		t.Error("an unset role was read as standby")
	}
	if got := legacy.HARoleNormalized(); got != HARolePrimary {
		t.Errorf("normalised empty role = %q, want %q", got, HARolePrimary)
	}
}
