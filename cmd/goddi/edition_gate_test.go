package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func editionFixture(t *testing.T, role string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "never-created")
	configPath := filepath.Join(dir, "config.yaml")
	body := fmt.Sprintf("server:\n  edition: enterprise\n  role: %s\n  data_dir: %q\ndatabase:\n  dsn: %q\nsecurity:\n  jwt_secret: 'long-valid-jwt-secret-123456'\n", role, data, filepath.Join(data, "control.db"))
	if err := os.WriteFile(configPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return configPath, data
}
func cleanEditionEnvironment(t *testing.T) {
	t.Helper()
	v, ok := os.LookupEnv("GODDI_SERVER_EDITION")
	_ = os.Unsetenv("GODDI_SERVER_EDITION")
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv("GODDI_SERVER_EDITION", v)
		} else {
			_ = os.Unsetenv("GODDI_SERVER_EDITION")
		}
	})
}
func TestEnterpriseServeRolesFailBeforeSideEffects(t *testing.T) {
	cleanEditionEnvironment(t)
	for _, role := range []string{"all", "control", "dns", "dhcp"} {
		t.Run(role, func(t *testing.T) {
			path, dir := editionFixture(t, role)
			err := runServer(path)
			if !errors.Is(err, ErrEnterpriseUnavailable) {
				t.Fatalf("role %s: %v", role, err)
			}
			if _, err = os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("data directory created: %v", err)
			}
		})
	}
}
func TestEnterpriseCommandsFailBeforeSideEffects(t *testing.T) {
	cleanEditionEnvironment(t)
	path, dir := editionFixture(t, "all")
	commands := []struct {
		name string
		run  func() error
	}{
		{"migrate", func() error { return runMigrations(path) }},
		{"unlock", func() error { return runUnlock(path, "user", "", false) }},
		{"rekey", func() error { return runRekey(path, false) }},
		{"rekey dry run", func() error { return runRekey(path, true) }},
		{"backup", func() error { return runSnapshot(path, filepath.Join(dir, "out"), nil, false) }},
		{"restore", func() error { return runRestore(path, rest{input: "missing", into: filepath.Join(dir, "restore")}) }},
		{"restore verify", func() error { return runRestore(path, rest{input: "missing", verifyOnly: true}) }},
		{"degrade", func() error { return runHADegrade(path, true, false, "test") }},
		{"takeover", func() error { return runHATakeover(path, true, true, 0) }},
		{"fence", func() error { return runHAFence(path, true, "test") }},
		{"rejoin", func() error { return runHARejoin(path, true) }},
	}
	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, ErrEnterpriseUnavailable) {
				t.Fatalf("got %v", err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("data directory created: %v", err)
			}
		})
	}
	if err := runMigrationStatus(path, false); err != nil {
		t.Fatalf("read-only status blocked: %v", err)
	}
	if err := runMigrationStatus(path, true); err == nil || errors.Is(err, ErrEnterpriseUnavailable) {
		t.Fatalf("check should report pending migrations, not an edition gate: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("status created directory: %v", err)
	}
}
