package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditionConfigurationAndEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, env string
		setEnv          bool
		want            string
		valid           bool
	}{
		{"default", "server: {}\n", "", false, "legacy", true},
		{"explicit enterprise", "server:\n  edition: enterprise\n", "", false, "enterprise", true},
		{"environment tightens", "server: {}\n", "enterprise", true, "enterprise", true},
		{"explicit legacy override", "server:\n  edition: legacy\n", "legacy", true, "legacy", true},
		{"cannot downgrade", "server:\n  edition: enterprise\n", "legacy", true, "", false},
		{"empty env", "server: {}\n", "", true, "", false},
		{"space env", "server: {}\n", " enterprise", true, "", false},
		{"case", "server:\n  edition: Enterprise\n", "", false, "", false},
		{"blank yaml", "server:\n  edition: ''\n", "", false, "", false},
		{"invalid cannot be overridden", "server:\n  edition: oops\n", "legacy", true, "", false},
		{"null yaml", "server:\n  edition: null\n", "", false, "", false},
		{"merged null", "server: {<<: {edition: null}}\n", "", false, "", false},
		{"merged integer", "server: {<<: {edition: 1}}\n", "", false, "", false},
		{"merged enterprise", "server: {<<: {edition: enterprise}}\n", "", false, "enterprise", true},
		{"merged enterprise downgrade", "server: {<<: {edition: enterprise}}\n", "legacy", true, "", false},
		{"root merged enterprise", "<<: {server: {edition: enterprise}}\n", "legacy", true, "", false},
		{"edition alias enterprise", "server: {name: &edition enterprise, edition: *edition}\n", "", false, "enterprise", true},
		{"edition alias null", "server: {name: &edition null, edition: *edition}\n", "", false, "", false},
		{"server alias empty", "log: &servermap {}\nserver: *servermap\n", "", false, "legacy", true},
		{"server alias null edition", "log: &servermap {edition: null}\nserver: *servermap\n", "", false, "", false},
		{"merge overridden", "server: {<<: {edition: null}, edition: enterprise}\n", "", false, "enterprise", true},
		{"legacy then enterprise", "server: {edition: legacy}\n---\nserver: {edition: enterprise}\n", "", false, "", false},
		{"enterprise then legacy", "server: {edition: enterprise}\n---\nserver: {edition: legacy}\n", "", false, "", false},
		{"default then enterprise", "server: {}\n---\nserver: {edition: enterprise}\n", "", false, "", false},
		{"default then enterprise with env", "server: {}\n---\nserver: {edition: enterprise}\n", "legacy", true, "", false},
		{"empty second document", "server: {}\n---\n", "", false, "", false},
		{"invalid trailing document", "server: {}\n---\nserver: [\n", "", false, "", false},
		{"empty first document", "---\n---\nserver: {edition: enterprise}\n", "", false, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.setEnv {
				t.Setenv("GODDI_SERVER_EDITION", tc.env)
			} else {
				old, ok := os.LookupEnv("GODDI_SERVER_EDITION")
				_ = os.Unsetenv("GODDI_SERVER_EDITION")
				t.Cleanup(func() {
					if ok {
						_ = os.Setenv("GODDI_SERVER_EDITION", old)
					} else {
						_ = os.Unsetenv("GODDI_SERVER_EDITION")
					}
				})
			}
			cfg, err := LoadFromYAML(path)
			if err == nil {
				ApplyEnvOverrides(cfg)
				err = ValidateEdition(cfg)
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && cfg.Server.Edition != tc.want {
				t.Fatalf("edition=%q", cfg.Server.Edition)
			}
		})
	}
}
