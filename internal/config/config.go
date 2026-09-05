package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration.
type Config struct {
	Server     ServerConfig     `yaml:"server" validate:"required"`
	Database   DatabaseConfig   `yaml:"database" validate:"required"`
	DNS        DNSConfig        `yaml:"dns"`
	Cache      CacheConfig      `yaml:"cache"`
	Forwarders ForwardersConfig `yaml:"forwarders"`
	DHCP       DHCPConfig       `yaml:"dhcp"`
	IPAM       IPAMConfig       `yaml:"ipam"`
	Security   SecurityConfig   `yaml:"security"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	Log        LogConfig        `yaml:"log"`
	SSO        SSOConfig        `yaml:"sso"`
	Cluster    ClusterConfig    `yaml:"cluster"`
	Plugin     PluginConfig     `yaml:"plugin"`
	DHCPHA     DHCPHAConfig     `yaml:"dhcp_ha"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Name      string `yaml:"name" validate:"required"`
	HTTPAddr  string `yaml:"http_addr" validate:"required"`
	PublicURL string `yaml:"public_url" validate:"required,url"`
	DataDir   string `yaml:"data_dir" validate:"required"`
	Language  string `yaml:"language"`
	DarkMode  bool   `yaml:"dark_mode"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Driver string `yaml:"driver" validate:"required,oneof=sqlite"`
	DSN    string `yaml:"dsn" validate:"required"`
}

// DNSConfig holds DNS service configuration.
type DNSConfig struct {
	Enabled    bool               `yaml:"enabled"`
	Domain     string             `yaml:"domain"`
	DefaultTTL int                `yaml:"default_ttl"`
	Recursion  DNSRecursionConfig `yaml:"recursion"`
	Listeners  DNSListenersConfig `yaml:"listeners"`
}

// DNSRecursionConfig holds DNS recursion settings.
type DNSRecursionConfig struct {
	Enabled   bool     `yaml:"enabled"`
	AllowNets []string `yaml:"allow_nets"`
}

// DNSListenersConfig holds all DNS listener configurations.
type DNSListenersConfig struct {
	UDP DNSListenerConfig    `yaml:"udp"`
	TCP DNSListenerConfig    `yaml:"tcp"`
	DOT DNSListenerTLSConfig `yaml:"dot"`
	DOH DNSListenerTLSConfig `yaml:"doh"`
	DOQ DNSListenerTLSConfig `yaml:"doq"`
}

// DNSListenerConfig holds a basic DNS listener configuration.
type DNSListenerConfig struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
}

// DNSListenerTLSConfig holds a DNS listener configuration with TLS.
type DNSListenerTLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Address  string `yaml:"address"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// CacheConfig holds DNS cache configuration.
type CacheConfig struct {
	Enabled        bool   `yaml:"enabled"`
	MaxEntries     int    `yaml:"max_entries"`
	MinTTL         int    `yaml:"min_ttl"`
	MaxTTL         int    `yaml:"max_ttl"`
	NegativeTTL    int    `yaml:"negative_ttl"`
	ServeStale     bool   `yaml:"serve_stale"`
	StaleTTL       int    `yaml:"stale_ttl"`
	Prefetch       bool   `yaml:"prefetch"`
	AutoPrefetch   bool   `yaml:"auto_prefetch"`
	Persistent     bool   `yaml:"persistent"`
	PersistentFile string `yaml:"persistent_file"`
}

// ForwardersConfig holds DNS forwarder configuration.
type ForwardersConfig struct {
	Mode    string            `yaml:"mode" validate:"omitempty,oneof=sequential round_robin random parallel_fastest latency_best health_aware"`
	Servers []ForwarderServer `yaml:"servers"`
}

// ForwarderServer represents a single DNS forwarder server.
type ForwarderServer struct {
	Name     string `yaml:"name"`
	Protocol string `yaml:"protocol" validate:"omitempty,oneof=udp tcp dot doh doq"`
	Address  string `yaml:"address"`
}

// DHCPConfig holds DHCP service configuration.
type DHCPConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Interfaces []string `yaml:"interfaces"`
}

// IPAMConfig holds IPAM configuration.
type IPAMConfig struct {
	PingCheck    bool `yaml:"ping_check"`
	AutoScan     bool `yaml:"auto_scan"`
	ScanInterval int  `yaml:"scan_interval"`
}

// SecurityConfig holds security-related configuration.
type SecurityConfig struct {
	JWTSecret           string `yaml:"jwt_secret" validate:"required"`
	EncryptionKey       string `yaml:"encryption_key"`
	LoginRateLimit      int    `yaml:"login_rate_limit"`
	LoginRateWindow     int    `yaml:"login_rate_window_seconds"` // lockout window in seconds (0 = default 900s)
	TOTPEnabled         bool   `yaml:"totp_enabled"`
	RebindingProtection bool   `yaml:"rebinding_protection"`
}

// ProxyConfig holds proxy configuration.
type ProxyConfig struct {
	Enabled bool   `yaml:"enabled"`
	Type    string `yaml:"type" validate:"omitempty,oneof=http socks5"`
	Address string `yaml:"address"`
}

// MetricsConfig holds Prometheus metrics configuration.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level           string `yaml:"level" validate:"omitempty,oneof=debug info warn error"`
	QueryLogEnabled bool   `yaml:"query_log_enabled"`
	RetentionDays   int    `yaml:"retention_days"`
}

// LoadFromYAML loads configuration from a YAML file.
func LoadFromYAML(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return cfg, nil
}

// ApplyEnvOverrides applies environment variable overrides to the configuration.
// Environment variables use the prefix GODDI_ with underscore-separated paths.
// For example: GODDI_SERVER_HTTP_ADDR, GODDI_DATABASE_DRIVER, GODDI_DNS_ENABLED
func ApplyEnvOverrides(cfg *Config) {
	setEnvString("GODDI_SERVER_NAME", &cfg.Server.Name)
	setEnvString("GODDI_SERVER_HTTP_ADDR", &cfg.Server.HTTPAddr)
	setEnvString("GODDI_SERVER_PUBLIC_URL", &cfg.Server.PublicURL)
	setEnvString("GODDI_SERVER_DATA_DIR", &cfg.Server.DataDir)
	setEnvString("GODDI_SERVER_LANGUAGE", &cfg.Server.Language)
	setEnvBool("GODDI_SERVER_DARK_MODE", &cfg.Server.DarkMode)

	setEnvString("GODDI_DATABASE_DRIVER", &cfg.Database.Driver)
	setEnvString("GODDI_DATABASE_DSN", &cfg.Database.DSN)

	setEnvBool("GODDI_DNS_ENABLED", &cfg.DNS.Enabled)
	setEnvString("GODDI_DNS_DOMAIN", &cfg.DNS.Domain)
	setEnvInt("GODDI_DNS_DEFAULT_TTL", &cfg.DNS.DefaultTTL)

	setEnvBool("GODDI_CACHE_ENABLED", &cfg.Cache.Enabled)
	setEnvInt("GODDI_CACHE_MAX_ENTRIES", &cfg.Cache.MaxEntries)

	setEnvBool("GODDI_DHCP_ENABLED", &cfg.DHCP.Enabled)

	setEnvString("GODDI_SECURITY_JWT_SECRET", &cfg.Security.JWTSecret)
	setEnvString("GODDI_SECURITY_ENCRYPTION_KEY", &cfg.Security.EncryptionKey)
	setEnvInt("GODDI_SECURITY_LOGIN_RATE_LIMIT", &cfg.Security.LoginRateLimit)
	setEnvBool("GODDI_SECURITY_TOTP_ENABLED", &cfg.Security.TOTPEnabled)
	setEnvBool("GODDI_SECURITY_REBINDING_PROTECTION", &cfg.Security.RebindingProtection)

	setEnvBool("GODDI_PROXY_ENABLED", &cfg.Proxy.Enabled)
	setEnvString("GODDI_PROXY_TYPE", &cfg.Proxy.Type)
	setEnvString("GODDI_PROXY_ADDRESS", &cfg.Proxy.Address)

	setEnvBool("GODDI_METRICS_ENABLED", &cfg.Metrics.Enabled)
	setEnvString("GODDI_METRICS_PATH", &cfg.Metrics.Path)

	setEnvString("GODDI_LOG_LEVEL", &cfg.Log.Level)
	setEnvBool("GODDI_LOG_QUERY_LOG_ENABLED", &cfg.Log.QueryLogEnabled)
	setEnvInt("GODDI_LOG_RETENTION_DAYS", &cfg.Log.RetentionDays)
}

// Validate validates the configuration using go-playground/validator.
func Validate(cfg *Config) error {
	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// SEC-01: Reject default JWT secrets unless explicitly allowed for development.
	defaultSecrets := map[string]bool{
		"change-me":               true,
		"change-me-in-production": true,
	}
	if defaultSecrets[cfg.Security.JWTSecret] {
		if os.Getenv("GODDI_ALLOW_DEFAULT_SECRET") != "true" {
			return fmt.Errorf("security.jwt_secret must be changed from the default value; set GODDI_ALLOW_DEFAULT_SECRET=true to allow defaults in development")
		}
	}

	// Warn if the JWT secret is too short.
	if len(cfg.Security.JWTSecret) < 16 {
		if os.Getenv("GODDI_ALLOW_DEFAULT_SECRET") != "true" {
			return fmt.Errorf("security.jwt_secret must be at least 16 characters long (current: %d); set GODDI_ALLOW_DEFAULT_SECRET=true to allow in development", len(cfg.Security.JWTSecret))
		}
		slog.Warn("JWT secret is shorter than 16 characters, which may be insecure", "length", len(cfg.Security.JWTSecret))
	}

	return nil
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Name:      "GoDDI",
			HTTPAddr:  ":6080",
			PublicURL: "http://localhost:6080",
			DataDir:   "./data",
			Language:  "zh-CN",
			DarkMode:  false,
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "./data/goddi.db",
		},
		DNS: DNSConfig{
			Enabled:    true,
			Domain:     "local",
			DefaultTTL: 3600,
			Recursion: DNSRecursionConfig{
				Enabled:   true,
				AllowNets: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fd00::/8"},
			},
			Listeners: DNSListenersConfig{
				UDP: DNSListenerConfig{Enabled: true, Address: ":53"},
				TCP: DNSListenerConfig{Enabled: true, Address: ":53"},
				DOT: DNSListenerTLSConfig{Enabled: false, Address: ":853"},
				DOH: DNSListenerTLSConfig{Enabled: false, Address: ":8443"},
				DOQ: DNSListenerTLSConfig{Enabled: false, Address: ":853"},
			},
		},
		Cache: CacheConfig{
			Enabled:        true,
			MaxEntries:     10000,
			MinTTL:         60,
			MaxTTL:         86400,
			NegativeTTL:    300,
			ServeStale:     true,
			StaleTTL:       3600,
			Prefetch:       true,
			AutoPrefetch:   false,
			Persistent:     false,
			PersistentFile: "",
		},
		Forwarders: ForwardersConfig{
			Mode: "sequential",
			Servers: []ForwarderServer{
				{Name: "google", Protocol: "udp", Address: "8.8.8.8:53"},
				{Name: "cloudflare", Protocol: "udp", Address: "1.1.1.1:53"},
			},
		},
		DHCP: DHCPConfig{
			Enabled:    true,
			Interfaces: []string{},
		},
		IPAM: IPAMConfig{
			PingCheck:    true,
			AutoScan:     false,
			ScanInterval: 3600,
		},
		Security: SecurityConfig{
			JWTSecret:           "change-me-in-production",
			EncryptionKey:       "",
			LoginRateLimit:      5,
			TOTPEnabled:         true,
			RebindingProtection: true,
		},
		Proxy: ProxyConfig{
			Enabled: false,
			Type:    "http",
			Address: "",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		Log: LogConfig{
			Level:           "info",
			QueryLogEnabled: true,
			RetentionDays:   30,
		},
		SSO:     DefaultSSOConfig(),
		Cluster: DefaultClusterConfig(),
		Plugin:  DefaultPluginConfig(),
		DHCPHA:  DefaultDHCPHAConfig(),
	}
}

// Helper functions for environment variable overrides.

func setEnvString(key string, target *string) {
	if val := os.Getenv(key); val != "" {
		*target = val
	}
}

func setEnvBool(key string, target *bool) {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			*target = b
		}
	}
}

func setEnvInt(key string, target *int) {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			*target = i
		}
	}
}

// String returns a safe string representation of the config (secrets masked).
func (c *Config) String() string {
	clone := *c
	clone.Security.JWTSecret = maskSecret(clone.Security.JWTSecret)
	clone.Security.EncryptionKey = maskSecret(clone.Security.EncryptionKey)
	out, _ := yaml.Marshal(&clone)
	return string(out)
}

func maskSecret(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}
