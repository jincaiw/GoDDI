package config

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Verify server defaults
	if cfg.Server.Name != "GoDDI" {
		t.Errorf("Server.Name = %q, want %q", cfg.Server.Name, "GoDDI")
	}
	if cfg.Server.HTTPAddr != ":6080" {
		t.Errorf("Server.HTTPAddr = %q, want %q", cfg.Server.HTTPAddr, ":6080")
	}
	if cfg.Server.Language != "zh-CN" {
		t.Errorf("Server.Language = %q, want %q", cfg.Server.Language, "zh-CN")
	}

	// Verify database defaults
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("Database.Driver = %q, want %q", cfg.Database.Driver, "sqlite")
	}

	// Verify DNS defaults
	if !cfg.DNS.Enabled {
		t.Error("DNS.Enabled should be true by default")
	}
	if cfg.DNS.DefaultTTL != 3600 {
		t.Errorf("DNS.DefaultTTL = %d, want 3600", cfg.DNS.DefaultTTL)
	}

	// Verify cache defaults
	if !cfg.Cache.Enabled {
		t.Error("Cache.Enabled should be true by default")
	}
	if cfg.Cache.MaxEntries != 10000 {
		t.Errorf("Cache.MaxEntries = %d, want 10000", cfg.Cache.MaxEntries)
	}

	// Verify security defaults
	if cfg.Security.JWTSecret != "change-me-in-production" {
		t.Errorf("Security.JWTSecret = %q, want default", cfg.Security.JWTSecret)
	}
	if cfg.Security.LoginRateLimit != 5 {
		t.Errorf("Security.LoginRateLimit = %d, want 5", cfg.Security.LoginRateLimit)
	}

	// Verify log defaults
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.RetentionDays != 30 {
		t.Errorf("Log.RetentionDays = %d, want 30", cfg.Log.RetentionDays)
	}
}

// validTestConfig returns a Config with a proper JWT secret for validation tests.
func validTestConfig() *Config {
	cfg := DefaultConfig()
	cfg.Security.JWTSecret = "test-secret-key-for-validation-testing"
	return cfg
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validTestConfig()
	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate() error = %v for valid config", err)
	}
}

func TestValidate_MissingServerName(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.Name = ""
	err := Validate(cfg)
	if err == nil {
		t.Error("Validate() should return error for missing server name")
	}
}

func TestValidate_MissingHTTPAddr(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.HTTPAddr = ""
	err := Validate(cfg)
	if err == nil {
		t.Error("Validate() should return error for missing HTTP address")
	}
}

func TestValidate_InvalidDatabaseDriver(t *testing.T) {
	cfg := validTestConfig()
	cfg.Database.Driver = "oracle"
	err := Validate(cfg)
	if err == nil {
		t.Error("Validate() should return error for invalid database driver")
	}
}

func TestValidate_MissingJWTSecret(t *testing.T) {
	cfg := validTestConfig()
	cfg.Security.JWTSecret = ""
	err := Validate(cfg)
	if err == nil {
		t.Error("Validate() should return error for missing JWT secret")
	}
}

func TestValidate_InvalidProxyType(t *testing.T) {
	cfg := validTestConfig()
	cfg.Proxy.Enabled = true
	cfg.Proxy.Type = "socks4"
	err := Validate(cfg)
	if err == nil {
		t.Error("Validate() should return error for invalid proxy type")
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := validTestConfig()
	cfg.Log.Level = "verbose"
	err := Validate(cfg)
	if err == nil {
		t.Error("Validate() should return error for invalid log level")
	}
}

func TestValidate_ValidProxyTypes(t *testing.T) {
	t.Parallel()

	for _, proxyType := range []string{"http", "socks5"} {
		cfg := validTestConfig()
		cfg.Proxy.Enabled = true
		cfg.Proxy.Type = proxyType
		err := Validate(cfg)
		if err != nil {
			t.Errorf("Validate() should accept proxy type %q, got error: %v", proxyType, err)
		}
	}
}

func TestValidate_ValidDatabaseDriver(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	cfg.Database.Driver = "sqlite"
	if err := Validate(cfg); err != nil {
		t.Errorf("Validate() should accept database driver %q, got error: %v", cfg.Database.Driver, err)
	}
}

func TestValidate_UnsupportedDatabaseDrivers(t *testing.T) {
	t.Parallel()

	for _, driver := range []string{"mysql", "postgresql"} {
		cfg := validTestConfig()
		cfg.Database.Driver = driver
		if err := Validate(cfg); err == nil {
			t.Errorf("Validate() should reject unsupported database driver %q", driver)
		}
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("GODDI_SERVER_NAME", "TestServer")
	os.Setenv("GODDI_SERVER_HTTP_ADDR", ":9090")
	os.Setenv("GODDI_DNS_ENABLED", "false")
	os.Setenv("GODDI_SECURITY_JWT_SECRET", "my-secret-key")
	os.Setenv("GODDI_LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("GODDI_SERVER_NAME")
		os.Unsetenv("GODDI_SERVER_HTTP_ADDR")
		os.Unsetenv("GODDI_DNS_ENABLED")
		os.Unsetenv("GODDI_SECURITY_JWT_SECRET")
		os.Unsetenv("GODDI_LOG_LEVEL")
	}()

	cfg := DefaultConfig()
	ApplyEnvOverrides(cfg)

	if cfg.Server.Name != "TestServer" {
		t.Errorf("Server.Name = %q, want %q", cfg.Server.Name, "TestServer")
	}
	if cfg.Server.HTTPAddr != ":9090" {
		t.Errorf("Server.HTTPAddr = %q, want %q", cfg.Server.HTTPAddr, ":9090")
	}
	if cfg.DNS.Enabled {
		t.Error("DNS.Enabled should be false after env override")
	}
	if cfg.Security.JWTSecret != "my-secret-key" {
		t.Errorf("Security.JWTSecret = %q, want %q", cfg.Security.JWTSecret, "my-secret-key")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
}

func TestApplyEnvOverrides_BoolValues(t *testing.T) {
	os.Setenv("GODDI_SERVER_DARK_MODE", "true")
	os.Setenv("GODDI_CACHE_ENABLED", "false")
	defer func() {
		os.Unsetenv("GODDI_SERVER_DARK_MODE")
		os.Unsetenv("GODDI_CACHE_ENABLED")
	}()

	cfg := DefaultConfig()
	ApplyEnvOverrides(cfg)

	if !cfg.Server.DarkMode {
		t.Error("Server.DarkMode should be true after env override")
	}
	if cfg.Cache.Enabled {
		t.Error("Cache.Enabled should be false after env override")
	}
}

func TestApplyEnvOverrides_IntValues(t *testing.T) {
	os.Setenv("GODDI_CACHE_MAX_ENTRIES", "50000")
	os.Setenv("GODDI_DNS_DEFAULT_TTL", "7200")
	defer func() {
		os.Unsetenv("GODDI_CACHE_MAX_ENTRIES")
		os.Unsetenv("GODDI_DNS_DEFAULT_TTL")
	}()

	cfg := DefaultConfig()
	ApplyEnvOverrides(cfg)

	if cfg.Cache.MaxEntries != 50000 {
		t.Errorf("Cache.MaxEntries = %d, want 50000", cfg.Cache.MaxEntries)
	}
	if cfg.DNS.DefaultTTL != 7200 {
		t.Errorf("DNS.DefaultTTL = %d, want 7200", cfg.DNS.DefaultTTL)
	}
}

func TestApplyEnvOverrides_InvalidBool(t *testing.T) {
	os.Setenv("GODDI_DNS_ENABLED", "not-a-bool")
	defer os.Unsetenv("GODDI_DNS_ENABLED")

	cfg := DefaultConfig()
	originalEnabled := cfg.DNS.Enabled
	ApplyEnvOverrides(cfg)

	// Invalid bool should not change the value
	if cfg.DNS.Enabled != originalEnabled {
		t.Error("invalid bool env var should not change the config value")
	}
}

func TestApplyEnvOverrides_InvalidInt(t *testing.T) {
	os.Setenv("GODDI_CACHE_MAX_ENTRIES", "not-an-int")
	defer os.Unsetenv("GODDI_CACHE_MAX_ENTRIES")

	cfg := DefaultConfig()
	originalMaxEntries := cfg.Cache.MaxEntries
	ApplyEnvOverrides(cfg)

	// Invalid int should not change the value
	if cfg.Cache.MaxEntries != originalMaxEntries {
		t.Error("invalid int env var should not change the config value")
	}
}

func TestLoadFromYAML_NonexistentFile(t *testing.T) {
	_, err := LoadFromYAML("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("LoadFromYAML() should return error for nonexistent file")
	}
}

func TestLoadFromYAML_ValidFile(t *testing.T) {
	// Create a temporary YAML file
	content := []byte(`
server:
  name: TestGoDDI
  http_addr: ":9999"
  public_url: "http://localhost:9999"
  data_dir: "/tmp/goddi-test"
database:
  driver: sqlite
  dsn: test.db
security:
  jwt_secret: test-secret
`)
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadFromYAML(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadFromYAML() error = %v", err)
	}
	if cfg.Server.Name != "TestGoDDI" {
		t.Errorf("Server.Name = %q, want %q", cfg.Server.Name, "TestGoDDI")
	}
	if cfg.Server.HTTPAddr != ":9999" {
		t.Errorf("Server.HTTPAddr = %q, want %q", cfg.Server.HTTPAddr, ":9999")
	}
}

func TestLoadFromYAML_InvalidYAML(t *testing.T) {
	content := []byte(`invalid: yaml: content: [`)
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write(content)
	tmpFile.Close()

	_, err = LoadFromYAML(tmpFile.Name())
	if err == nil {
		t.Error("LoadFromYAML() should return error for invalid YAML")
	}
}

func TestConfig_String_MasksSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.JWTSecret = "super-secret-key-12345"

	str := cfg.String()

	if strings.Contains(str, "super-secret-key-12345") {
		t.Error("Config.String() should mask JWT secret")
	}
	if !strings.Contains(str, "*************2345") {
		t.Errorf("Config.String() should mask secret but show last 4 chars, got: %s", str)
	}
}

func TestMaskSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"short secret", "abc", "****"},
		{"4 char secret", "abcd", "****"},
		{"5 char secret", "abcde", "*bcde"},
		{"long secret", "super-secret-key-12345", "******************2345"},
		{"empty string", "", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := maskSecret(tt.input)
			if result != tt.expected {
				t.Errorf("maskSecret(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultConfig_Forwarders(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Forwarders.Mode != "sequential" {
		t.Errorf("Forwarders.Mode = %q, want %q", cfg.Forwarders.Mode, "sequential")
	}
	if len(cfg.Forwarders.Servers) != 2 {
		t.Errorf("Forwarders.Servers count = %d, want 2", len(cfg.Forwarders.Servers))
	}
}

func TestDefaultConfig_DNSListeners(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if !cfg.DNS.Listeners.UDP.Enabled {
		t.Error("DNS.Listeners.UDP.Enabled should be true by default")
	}
	if !cfg.DNS.Listeners.TCP.Enabled {
		t.Error("DNS.Listeners.TCP.Enabled should be true by default")
	}
	if cfg.DNS.Listeners.DOT.Enabled {
		t.Error("DNS.Listeners.DOT.Enabled should be false by default")
	}
}

// --- SEC-04 Regression Tests: Default JWT secret rejection ---

func TestValidate_DefaultSecretRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.JWTSecret = "change-me"
	err := Validate(cfg)
	if err == nil {
		t.Error("Validate() should reject 'change-me' default JWT secret")
	}
}

func TestValidate_DefaultSecretAllowedWithEnv(t *testing.T) {
	os.Setenv("GODDI_ALLOW_DEFAULT_SECRET", "true")
	defer os.Unsetenv("GODDI_ALLOW_DEFAULT_SECRET")

	cfg := DefaultConfig()
	cfg.Security.JWTSecret = "change-me"
	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate() should allow default secret when GODDI_ALLOW_DEFAULT_SECRET=true, got error: %v", err)
	}
}

func TestValidate_ProperSecretAccepted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.JWTSecret = "this-is-a-proper-secret-key-for-testing"
	err := Validate(cfg)
	if err != nil {
		t.Errorf("Validate() should accept a proper secret, got error: %v", err)
	}
}

// --- DNS-02 Regression Tests: Default recursion ACL ---

func TestDefaultConfig_RecursionAllowNets(t *testing.T) {
	cfg := DefaultConfig()

	// Default AllowNets should be RFC1918 private networks
	expectedNets := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fd00::/8"}
	if len(cfg.DNS.Recursion.AllowNets) != len(expectedNets) {
		t.Fatalf("DNS.Recursion.AllowNets length = %d, want %d", len(cfg.DNS.Recursion.AllowNets), len(expectedNets))
	}

	for i, expected := range expectedNets {
		if cfg.DNS.Recursion.AllowNets[i] != expected {
			t.Errorf("DNS.Recursion.AllowNets[%d] = %q, want %q", i, cfg.DNS.Recursion.AllowNets[i], expected)
		}
	}
}

func TestDefaultConfig_RecursionAllowNetsNotOpen(t *testing.T) {
	cfg := DefaultConfig()

	// 0.0.0.0/0 should NOT be in default AllowNets
	for _, net := range cfg.DNS.Recursion.AllowNets {
		if net == "0.0.0.0/0" {
			t.Error("0.0.0.0/0 should NOT be in default DNS.Recursion.AllowNets")
		}
		if net == "::/0" {
			t.Error("::/0 should NOT be in default DNS.Recursion.AllowNets")
		}
	}
}
