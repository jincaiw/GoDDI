package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
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
	DataPlane  DataPlaneConfig  `yaml:"dataplane"`
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

	// Env is the deployment's own declaration of what it is. Only the exact
	// value "production" changes behaviour, and only to tighten it: it is what
	// turns the development conveniences (CORS loopback widening, deriving the
	// encryption key from the JWT secret) into startup errors. An empty value
	// is treated as development, so a deployment that never sets it keeps the
	// old, permissive behaviour rather than silently acquiring new failure
	// modes.
	Env string `yaml:"env"`
}

// IsProduction reports whether this deployment declares itself production.
//
// One reader, one value: the previous shape asked the environment directly
// from two different places, so "are we production?" had two answers that
// could disagree.
func (c *Config) IsProduction() bool { return c != nil && c.Env == "production" }

// ServerConfig holds HTTP server configuration.
// ServerTLSConfig holds the TLS settings for the HTTP management plane.
//
// Without TLS the admin UI and API transport credentials, JWTs and API tokens
// in cleartext; enabling it is strongly recommended for any non-localhost
// deployment. When TLS is disabled here, terminate it at a reverse proxy and
// keep the management port off untrusted networks.
type ServerTLSConfig struct {
	// Enabled serves the management UI/API over HTTPS.
	Enabled bool `yaml:"enabled"`
	// CertFile and KeyFile are the PEM-encoded certificate chain and key.
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	// MinVersion is the minimum acceptable TLS version: "1.2" (default) or "1.3".
	MinVersion string `yaml:"min_version"`
	// ClientCAFile turns on mutual TLS: when set, the console requires a client
	// certificate signed by this CA. Off by default -- a browser deployment has
	// no client certificate to present, so enabling it without distributing one
	// locks out every operator.
	ClientCAFile string `yaml:"client_ca_file"`
}

type ServerConfig struct {
	Name      string          `yaml:"name" validate:"required"`
	HTTPAddr  string          `yaml:"http_addr" validate:"required"`
	PublicURL string          `yaml:"public_url" validate:"required,url"`
	DataDir   string          `yaml:"data_dir" validate:"required"`
	Language  string          `yaml:"language"`
	DarkMode  bool            `yaml:"dark_mode"`
	TLS       ServerTLSConfig `yaml:"tls"`
	// Role selects which planes this process runs. Empty means "all", which
	// is the single-process deployment every existing installation has.
	Role string `yaml:"role" validate:"omitempty,oneof=all control dns dhcp"`
	// ExposeOpenAPI controls whether /api/v1/openapi.json is served.
	// It defaults to true (the schema contains no secrets and the web
	// console tooling uses it); set it to false on hardened deployments
	// to reduce the API surface visible to unauthenticated probes.
	ExposeOpenAPI bool `yaml:"expose_openapi"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Driver string `yaml:"driver" validate:"required,oneof=sqlite"`
	DSN    string `yaml:"dsn" validate:"required"`
}

// DNSConfig holds DNS service configuration.
// DNSSECConfig holds the DNSSEC feature gate.
//
// DNSSEC is currently experimental: enabling it on a zone marks the zone as
// DNSSEC-enabled and generates keys, but the authoritative answers do not yet
// carry RRSIG/DNSKEY records. Presenting that as production-ready DNSSEC would
// make validating resolvers treat the zone as Bogus, so the feature is gated
// off by default and must be opted into explicitly.
type DNSSECConfig struct {
	// Enabled allows zones to be marked DNSSEC-enabled. Defaults to false
	// until real signing (RRSIG/NSEC) is implemented.
	Enabled bool `yaml:"enabled"`
}

// DNSConfig holds DNS server configuration.
type DNSConfig struct {
	Enabled       bool                   `yaml:"enabled"`
	Domain        string                 `yaml:"domain"`
	DefaultTTL    int                    `yaml:"default_ttl"`
	Recursion     DNSRecursionConfig     `yaml:"recursion"`
	Listeners     DNSListenersConfig     `yaml:"listeners"`
	RateLimit     DNSRateLimitConfig     `yaml:"rate_limit"`
	DynamicUpdate DNSDynamicUpdateConfig `yaml:"dynamic_update"`
	DNSSEC        DNSSECConfig           `yaml:"dnssec"`
	// AllowPrivateUpstream permits upstream forwarders and debug queries to
	// target loopback/private/link-local addresses. It defaults to true so
	// forwarding to internal resolvers (a core enterprise DDI use case) keeps
	// working; set it to false on internet-facing deployments to close the
	// remaining SSRF surface.
	AllowPrivateUpstream bool `yaml:"allow_private_upstream"`
}

// DNSRateLimitConfig holds DNS query/response rate limiting configuration.
type DNSRateLimitConfig struct {
	// Enabled switches the whole rate limiting module on/off.
	Enabled bool `yaml:"enabled"`
	// ClientQPS is the sustained per-client query limit (0 = unlimited).
	ClientQPS int `yaml:"client_qps"`
	// ClientBurst is the per-client token bucket capacity (0 = ClientQPS).
	ClientBurst int `yaml:"client_burst"`
	// RRLThreshold is the per-second identical-response threshold across
	// all clients (0 = response rate limiting disabled).
	RRLThreshold int `yaml:"rrl_threshold"`
}

// DNSDynamicUpdateConfig holds RFC 2136 dynamic update configuration.
type DNSDynamicUpdateConfig struct {
	// TSIGKeys maps TSIG key names to base64 shared secrets. Updates are
	// only accepted when signed with a registered key.
	TSIGKeys map[string]string `yaml:"tsig_keys"`
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
	Mode                       string            `yaml:"mode" validate:"omitempty,oneof=sequential round_robin random parallel_fastest latency_best health_aware"`
	HealthCheckIntervalSeconds int               `yaml:"health_check_interval_seconds" validate:"gte=0,lte=3600"`
	Servers                    []ForwarderServer `yaml:"servers"`
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
	// TrustedRelayCIDRs restricts which relay agents may forward requests to
	// this server, by the giaddr they set. Empty means no restriction.
	//
	// This is not a client allowlist and cannot be one: a client chooses its
	// own MAC address, so listing addresses would exclude honest hardware and
	// stop nobody who had decided to lie. A relay is a device the operator
	// owns, and its giaddr reaches the server only if it sent the packet.
	TrustedRelayCIDRs []string `yaml:"trusted_relay_cidrs"`
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
	// TrustedProxyCIDRs lists reverse-proxy source networks whose forwarding
	// headers may identify the original client. Empty means direct-peer mode:
	// X-Forwarded-For/X-Real-IP are ignored.
	TrustedProxyCIDRs []string `yaml:"trusted_proxy_cidrs"`
	// AdminAllowCIDRs restricts the management API to these source networks.
	// Empty means no restriction, which is what a direct deployment needs.
	//
	// It deliberately does not cover /health or /ready: an orchestrator whose
	// probe is refused reads the instance as unhealthy and restarts it, so a
	// mistyped entry here would take down the process it was meant to protect.
	// The console is the surface this guards.
	//
	// Entries are CIDRs, the same rule as trusted_proxy_cidrs: a bare address
	// is rejected at startup rather than guessed at.
	AdminAllowCIDRs []string `yaml:"admin_allow_cidrs"`
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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return cfg, nil
}

// ApplyEnvOverrides applies environment variable overrides to the configuration.
// Environment variables use the prefix GODDI_ with underscore-separated paths.
// For example: GODDI_SERVER_HTTP_ADDR, GODDI_DATABASE_DRIVER, GODDI_DNS_ENABLED
func ApplyEnvOverrides(cfg *Config) {
	setEnvString("GODDI_ENV", &cfg.Env)
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
	setEnvBool("GODDI_DNS_RATE_LIMIT_ENABLED", &cfg.DNS.RateLimit.Enabled)
	setEnvInt("GODDI_DNS_RATE_LIMIT_CLIENT_QPS", &cfg.DNS.RateLimit.ClientQPS)
	setEnvInt("GODDI_DNS_RATE_LIMIT_RRL_THRESHOLD", &cfg.DNS.RateLimit.RRLThreshold)
	setEnvBool("GODDI_DNS_DNSSEC_ENABLED", &cfg.DNS.DNSSEC.Enabled)
	setEnvBool("GODDI_DNS_ALLOW_PRIVATE_UPSTREAM", &cfg.DNS.AllowPrivateUpstream)
	setEnvStringSlice("GODDI_SECURITY_TRUSTED_PROXY_CIDRS", &cfg.Security.TrustedProxyCIDRs)
	setEnvStringSlice("GODDI_SECURITY_ADMIN_ALLOW_CIDRS", &cfg.Security.AdminAllowCIDRs)
	setEnvStringSlice("GODDI_DHCP_TRUSTED_RELAY_CIDRS", &cfg.DHCP.TrustedRelayCIDRs)
	// Listener address overrides (useful for CI and multi-instance hosts
	// where binding the default :53 requires root).
	setEnvString("GODDI_DNS_LISTENERS_UDP_ADDR", &cfg.DNS.Listeners.UDP.Address)
	setEnvString("GODDI_DNS_LISTENERS_TCP_ADDR", &cfg.DNS.Listeners.TCP.Address)

	setEnvBool("GODDI_SERVER_TLS_ENABLED", &cfg.Server.TLS.Enabled)
	setEnvBool("GODDI_SERVER_EXPOSE_OPENAPI", &cfg.Server.ExposeOpenAPI)
	setEnvString("GODDI_SERVER_TLS_CERT_FILE", &cfg.Server.TLS.CertFile)
	setEnvString("GODDI_SERVER_TLS_KEY_FILE", &cfg.Server.TLS.KeyFile)

	setEnvBool("GODDI_CACHE_ENABLED", &cfg.Cache.Enabled)
	setEnvInt("GODDI_CACHE_MAX_ENTRIES", &cfg.Cache.MaxEntries)

	setEnvBool("GODDI_DHCP_ENABLED", &cfg.DHCP.Enabled)

	// The HA peer token is a shared secret, so it is read from the environment
	// as well as from the file. Every other dhcp_ha setting is deployment
	// shape rather than key material and has no environment form: an operator
	// reading config.yaml should be able to see what the pair is.
	setEnvString("GODDI_DHCP_HA_PEER_TOKEN", &cfg.DHCPHA.PeerToken)
	setEnvBool("GODDI_DHCP_HA_ENABLED", &cfg.DHCPHA.Enabled)
	setEnvString("GODDI_DHCP_HA_NODE_ID", &cfg.DHCPHA.NodeID)
	setEnvString("GODDI_DHCP_HA_ROLE", &cfg.DHCPHA.Role)
	setEnvString("GODDI_DHCP_HA_LISTEN_ADDR", &cfg.DHCPHA.ListenAddr)
	setEnvString("GODDI_DHCP_HA_PEER_ADDRESS", &cfg.DHCPHA.PeerAddress)
	setEnvString("GODDI_DHCP_HA_CONFIRM_TIMEOUT", &cfg.DHCPHA.ConfirmTimeout)
	setEnvString("GODDI_DHCP_HA_HEARTBEAT_INTERVAL", &cfg.DHCPHA.HeartbeatInterval)
	setEnvString("GODDI_DHCP_HA_PEER_STALE_AFTER", &cfg.DHCPHA.PeerStaleAfter)

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

	// The encryption key seals TOTP seeds and TSIG keys at rest. Deriving it
	// from the JWT secret works, and is what every installation predating this
	// setting does, but it means rotating the JWT secret destroys every sealed
	// value. Development keeps that fallback; a deployment that declares itself
	// production has to say what its key material is.
	if cfg.Security.EncryptionKey == "" {
		if cfg.IsProduction() {
			return fmt.Errorf("security.encryption_key must be set when env is production: without it, stored secrets are sealed with the JWT secret and rotating that secret makes them unreadable (set GODDI_SECURITY_ENCRYPTION_KEY, then run `goddi rekey`)")
		}
	} else {
		if cfg.Security.EncryptionKey == cfg.Security.JWTSecret {
			return fmt.Errorf("security.encryption_key must differ from security.jwt_secret; two purposes sharing one secret means a leak of either compromises both")
		}
		if len(cfg.Security.EncryptionKey) < 16 {
			return fmt.Errorf("security.encryption_key must be at least 16 characters long (current: %d)", len(cfg.Security.EncryptionKey))
		}
	}

	// TLS: when the management plane is served over HTTPS the certificate and
	// key must be present, and the minimum version must be a known value.
	for _, cidr := range cfg.Security.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("security.trusted_proxy_cidrs contains invalid CIDR %q: %w", cidr, err)
		}
	}

	// Refused at startup, not at request time. An entry that does not parse
	// cannot be honoured, and the two ways to react to that are both bad
	// silently: ignore it and the allowlist is wider than intended, or refuse
	// everything and the console is unreachable. Saying so here is the only
	// outcome an operator can act on.
	for _, cidr := range cfg.Security.AdminAllowCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("security.admin_allow_cidrs contains invalid CIDR %q: %w (a bare address must be written as a network, e.g. 10.0.0.5/32)", cidr, err)
		}
	}

	// Same rule as the management allowlist, and for the same reason: an entry
	// that does not parse cannot be honoured, and both ways of carrying on are
	// silent. A relay that should have been refused is the worse of the two.
	for _, cidr := range cfg.DHCP.TrustedRelayCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("dhcp.trusted_relay_cidrs contains invalid CIDR %q: %w (a bare address must be written as a network, e.g. 10.0.0.5/32)", cidr, err)
		}
	}

	if err := validateDHCPHA(&cfg.DHCPHA); err != nil {
		return err
	}

	if cfg.Server.TLS.Enabled {
		if cfg.Server.TLS.CertFile == "" || cfg.Server.TLS.KeyFile == "" {
			return fmt.Errorf("server.tls.enabled is true but server.tls.cert_file / server.tls.key_file are not set")
		}
		files := []string{cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile}
		if cfg.Server.TLS.ClientCAFile != "" {
			files = append(files, cfg.Server.TLS.ClientCAFile)
		}
		for _, f := range files {
			if _, err := os.Stat(f); err != nil {
				return fmt.Errorf("server.tls: cannot read %s: %w", f, err)
			}
		}
		switch cfg.Server.TLS.MinVersion {
		case "", "1.2", "1.3":
		default:
			return fmt.Errorf("server.tls.min_version must be \"1.2\" or \"1.3\" (got %q)", cfg.Server.TLS.MinVersion)
		}
	} else if cfg.Server.TLS.ClientCAFile != "" {
		return fmt.Errorf("server.tls.client_ca_file is set but server.tls.enabled is false: there is no TLS handshake for a client certificate to appear in")
	}

	if err := ValidateRole(cfg); err != nil {
		return err
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
			// Served by default; hardened deployments may switch it off.
			ExposeOpenAPI: true,
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
			RateLimit: DNSRateLimitConfig{
				Enabled:      true,
				ClientQPS:    0,
				ClientBurst:  0,
				RRLThreshold: 0,
			},
			DynamicUpdate: DNSDynamicUpdateConfig{
				TSIGKeys: map[string]string{},
			},
			// Off until real signing (RRSIG/NSEC) is implemented.
			DNSSEC: DNSSECConfig{Enabled: false},
			// Keep enterprise internal-resolver forwarding working by default.
			AllowPrivateUpstream: true,
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
			Mode:                       "health_aware",
			HealthCheckIntervalSeconds: 30,
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
			TrustedProxyCIDRs:   nil,
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

func setEnvStringSlice(key string, target *[]string) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	*target = out
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
