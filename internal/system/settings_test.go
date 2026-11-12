package system

import (
	"database/sql"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func settingsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:settings_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		description TEXT,
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		t.Fatalf("create system_settings: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func settingValue(t *testing.T, db *sql.DB, key string) string {
	t.Helper()
	var v string
	if err := db.QueryRow(`SELECT value FROM system_settings WHERE key = ?`, key).Scan(&v); err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return v
}

func TestNotifyPreservesUpdateOrder(t *testing.T) {
	m := NewManager(nil)
	var mu sync.Mutex
	got := make([]string, 0, 2)
	done := make(chan struct{})
	m.Subscribe(func(key, value string) {
		mu.Lock()
		got = append(got, key+"="+value)
		if len(got) == 2 {
			close(done)
		}
		mu.Unlock()
	})

	m.notify("first", "1")
	m.notify("second", "2")
	<-done

	mu.Lock()
	defer mu.Unlock()
	if got[0] != "first=1" || got[1] != "second=2" {
		t.Fatalf("notification order = %#v, want first then second", got)
	}
}

// A fresh database must be seeded with the operator's config values, not the
// hard-coded constants. Otherwise applyPersistedSettings replays those
// constants onto a DNS cache that was just built from config.yaml.
func TestEnsureDefaultsSeedsConfigOverrides(t *testing.T) {
	db := settingsTestDB(t)
	m := NewManager(db)

	overrides := map[string]string{
		"dns_cache_serve_stale": "false",
		"dns_cache_stale_ttl":   "3600",
		"dns_cache_prefetch":    "true",
		"dns_cache_min_ttl":     "120",
		"dns_cache_max_ttl":     "7200",
	}
	if err := m.EnsureDefaults(overrides); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}

	for key, want := range overrides {
		if got := settingValue(t, db, key); got != want {
			t.Errorf("%s = %q, want %q (config value)", key, got, want)
		}
	}
	// Keys without an override keep their documented default.
	if got := settingValue(t, db, "server_name"); got != "GoDDI" {
		t.Errorf("server_name = %q, want GoDDI", got)
	}
}

func TestEnsureDefaultsDoesNotOverwriteExistingSettings(t *testing.T) {
	db := settingsTestDB(t)
	m := NewManager(db)

	// A value the operator set through the UI, persisted from an earlier boot.
	if err := m.SetSetting("dns_cache_prefetch", "false", ""); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	// A later config change must not rewrite it: only missing keys are seeded.
	if err := m.EnsureDefaults(map[string]string{"dns_cache_prefetch": "true"}); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}
	if got := settingValue(t, db, "dns_cache_prefetch"); got != "false" {
		t.Errorf("existing setting was overwritten: got %q, want false", got)
	}
	// The other cache keys are still seeded from config on this fresh install.
	if got := settingValue(t, db, "dns_cache_stale_ttl"); got == "" {
		t.Error("missing cache keys were not seeded")
	}
}

func TestEnsureDefaultsWithoutOverridesKeepsDocumentedDefaults(t *testing.T) {
	db := settingsTestDB(t)
	m := NewManager(db)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}
	if got := settingValue(t, db, "dns_cache_prefetch"); got != "false" {
		t.Errorf("default dns_cache_prefetch = %q, want false", got)
	}
	if got := settingValue(t, db, "dns_cache_stale_ttl"); got != "86400" {
		t.Errorf("default dns_cache_stale_ttl = %q, want 86400", got)
	}
}

func TestEnsureDefaultsOverridesAreAppliedInOrder(t *testing.T) {
	db := settingsTestDB(t)
	m := NewManager(db)
	if err := m.EnsureDefaults(
		map[string]string{"dns_cache_min_ttl": "30"},
		map[string]string{"dns_cache_min_ttl": "90", "dns_cache_max_ttl": "600"},
	); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}
	if got := settingValue(t, db, "dns_cache_min_ttl"); got != "90" {
		t.Errorf("later override must win: got %q, want 90", got)
	}
	if got := settingValue(t, db, "dns_cache_max_ttl"); got != "600" {
		t.Errorf("second map's extra key = %q, want 600", got)
	}
}
