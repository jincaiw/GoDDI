package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheAllowListRefusesAnEntryItCannotRead covers the failure that costs the
// most: an entry that does not parse. Both ways of carrying on are silent --
// ignoring it leaves the allowlist wider than the operator believes, and
// refusing every request takes the console down without saying why -- so the
// only useful moment to complain is before the process starts serving.
func TestTheAllowListRefusesAnEntryItCannotRead(t *testing.T) {
	for _, entry := range []string{
		"10.0.0.5",                // a bare address, the most likely typo
		"10.0.0.0/33",             // a prefix that cannot exist
		"192.168.1.1-192.168.1.9", // a range, which is not a CIDR
		"not-a-network",
		"", // an empty element, e.g. from a trailing comma
	} {
		t.Run(entry, func(t *testing.T) {
			cfg := secretConfig(t)
			cfg.Security.AdminAllowCIDRs = []string{"10.20.30.0/24", entry}
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("an unparsable allowlist entry was accepted: %q", entry)
			}
			if !strings.Contains(err.Error(), "admin_allow_cidrs") {
				t.Errorf("the error does not name the setting: %v", err)
			}
		})
	}
}

func TestAValidAllowListIsAccepted(t *testing.T) {
	for _, entries := range [][]string{
		nil,
		{},
		{"10.20.30.0/24"},
		{"10.20.30.7/32", "192.0.2.0/24"},
		{"2001:db8:1234::/48"},
		{"0.0.0.0/0"},
	} {
		cfg := secretConfig(t)
		cfg.Security.AdminAllowCIDRs = entries
		if err := Validate(cfg); err != nil {
			t.Errorf("valid allowlist %v was refused: %v", entries, err)
		}
	}
}

// TestTheShippedConfigLeavesTheAllowListOpen pins the default. A shipped
// configuration that restricted the console would lock out every operator who
// deployed without reading the file.
func TestTheShippedConfigLeavesTheAllowListOpen(t *testing.T) {
	path := filepath.Join("..", "..", shippedConfigFilename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree", path)
	}
	cfg, err := LoadFromYAML(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if len(cfg.Security.AdminAllowCIDRs) != 0 {
		t.Errorf("the shipped config restricts the management API to %v", cfg.Security.AdminAllowCIDRs)
	}
}

func TestTheAllowListReadsFromTheEnvironment(t *testing.T) {
	t.Setenv("GODDI_SECURITY_ADMIN_ALLOW_CIDRS", "10.20.30.0/24, 192.0.2.0/24 ,2001:db8::/32")

	cfg := secretConfig(t)
	ApplyEnvOverrides(cfg)

	want := []string{"10.20.30.0/24", "192.0.2.0/24", "2001:db8::/32"}
	if len(cfg.Security.AdminAllowCIDRs) != len(want) {
		t.Fatalf("allowlist = %v, want %v", cfg.Security.AdminAllowCIDRs, want)
	}
	for i, w := range want {
		if cfg.Security.AdminAllowCIDRs[i] != w {
			t.Errorf("entry %d = %q, want %q (surrounding spaces trimmed)", i, cfg.Security.AdminAllowCIDRs[i], w)
		}
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("the environment-sourced allowlist does not validate: %v", err)
	}
}
