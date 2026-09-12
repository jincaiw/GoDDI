package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheRelayListRefusesAnEntryItCannotRead guards the same failure as the
// management allowlist, with a worse consequence. An entry dropped from a relay
// allowlist makes the list longer -- the one direction an access control must
// never move on its own -- and a relay that should have been refused would be
// served without anyone noticing.
func TestTheRelayListRefusesAnEntryItCannotRead(t *testing.T) {
	for _, entry := range []string{
		"10.0.0.5",                // a bare address, the most likely typo
		"10.0.0.0/33",             // a prefix that cannot exist
		"192.168.1.1-192.168.1.9", // a range, which is not a CIDR
		"not-a-network",
		"", // an empty element, e.g. from a trailing comma
	} {
		t.Run(entry, func(t *testing.T) {
			cfg := secretConfig(t)
			cfg.DHCP.TrustedRelayCIDRs = []string{"198.51.100.0/24", entry}
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("an unparsable relay entry was accepted: %q", entry)
			}
			if !strings.Contains(err.Error(), "trusted_relay_cidrs") {
				t.Errorf("the error does not name the setting: %v", err)
			}
		})
	}
}

func TestAValidRelayListIsAccepted(t *testing.T) {
	for _, entries := range [][]string{
		nil,
		{},
		{"198.51.100.0/24"},
		{"198.51.100.7/32", "203.0.113.0/24"},
		{"2001:db8:1234::/48"},
		{"0.0.0.0/0"},
	} {
		cfg := secretConfig(t)
		cfg.DHCP.TrustedRelayCIDRs = entries
		if err := Validate(cfg); err != nil {
			t.Errorf("valid relay list %v was refused: %v", entries, err)
		}
	}
}

// TestTheShippedConfigLeavesTheRelayListOpen pins the default. Restricting
// relays in the file that ships would break every deployment that has none --
// and one that has them would find out by watching clients go unserved.
func TestTheShippedConfigLeavesTheRelayListOpen(t *testing.T) {
	path := filepath.Join("..", "..", shippedConfigFilename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree", path)
	}
	cfg, err := LoadFromYAML(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if len(cfg.DHCP.TrustedRelayCIDRs) != 0 {
		t.Errorf("the shipped config serves only relays in %v", cfg.DHCP.TrustedRelayCIDRs)
	}
}

func TestTheRelayListReadsFromTheEnvironment(t *testing.T) {
	t.Setenv("GODDI_DHCP_TRUSTED_RELAY_CIDRS", "198.51.100.0/24, 203.0.113.0/24 ,2001:db8::/32")

	cfg := secretConfig(t)
	ApplyEnvOverrides(cfg)

	want := []string{"198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"}
	if len(cfg.DHCP.TrustedRelayCIDRs) != len(want) {
		t.Fatalf("relay list = %v, want %v", cfg.DHCP.TrustedRelayCIDRs, want)
	}
	for i, w := range want {
		if cfg.DHCP.TrustedRelayCIDRs[i] != w {
			t.Errorf("entry %d = %q, want %q (surrounding spaces trimmed)", i, cfg.DHCP.TrustedRelayCIDRs[i], w)
		}
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("the environment-sourced relay list does not validate: %v", err)
	}
}
