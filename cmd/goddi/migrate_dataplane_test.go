package main

// These tests pin the W12-b contract documented in
// docs/实施计划-v0.6.0.md and docs/runbook/灾备恢复.md: `goddi migrate`
// prepares only the control store. The data-plane stores are created and
// migrated by `serve` on its first start, so that `migrate --status/--check`
// can inspect a deployment without creating databases. The `--check` error
// text must tell the operator that rather than sending them back to a
// `migrate` that cannot apply those pending migrations.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func migrateContractFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	configPath := filepath.Join(dir, "config.yaml")
	body := "server:\n" +
		"  name: migrate-contract-test\n" +
		"  http_addr: \"127.0.0.1:0\"\n" +
		"  data_dir: \"" + data + "\"\n" +
		"database:\n" +
		"  dsn: \"" + filepath.Join(dir, "control.db") + "\"\n" +
		"security:\n" +
		"  jwt_secret: 'long-valid-jwt-secret-123456'\n"
	if err := os.WriteFile(configPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return configPath, data
}

func TestMigratePreparesOnlyTheControlStore(t *testing.T) {
	path, dataDir := migrateContractFixture(t)
	if err := runMigrations(path); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "control.db")); err != nil {
		t.Fatalf("control database was not created: %v", err)
	}
	for _, name := range []string{"dnsdata.db", "leases.db"} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); !os.IsNotExist(err) {
			t.Fatalf("migrate created data-plane store %s: %v", name, err)
		}
	}
}

func TestMigrationCheckExplainsWhereDataPlaneStoresComeFrom(t *testing.T) {
	path, _ := migrateContractFixture(t)
	if err := runMigrations(path); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err := runMigrationStatus(path, true)
	if err == nil {
		t.Fatal("--check should fail while data-plane migrations are pending")
	}
	for _, want := range []string{"goddi serve", "不会创建数据面库", "dnsdata/leases"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("--check error %q does not explain %q", err, want)
		}
	}
}
