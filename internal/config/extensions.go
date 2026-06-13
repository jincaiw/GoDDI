package config

// DoTConfig holds DNS-over-TLS configuration.
type DoTConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Address  string `yaml:"address" json:"address"`
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
}

// DoHConfig holds DNS-over-HTTPS configuration.
type DoHConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Address  string `yaml:"address" json:"address"`
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
	Path     string `yaml:"path" json:"path"`
}

// DoQConfig holds DNS-over-QUIC configuration.
type DoQConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Address  string `yaml:"address" json:"address"`
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
}

// SSOConfig holds SSO/OIDC configuration.
type SSOConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	Provider     string `yaml:"provider" json:"provider"`
	Authority    string `yaml:"authority" json:"authority"`
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
	DiscoveryURL string `yaml:"discovery_url" json:"discovery_url"`
	Scopes       string `yaml:"scopes" json:"scopes"`
	AutoRegister bool   `yaml:"auto_register" json:"auto_register"`
	GroupMapping string `yaml:"group_mapping" json:"group_mapping"`
}

// ClusterConfig holds cluster configuration.
type ClusterConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	NodeName     string `yaml:"node_name" json:"node_name"`
	NodeRole     string `yaml:"node_role" json:"node_role"` // primary, secondary
	BindAddr     string `yaml:"bind_addr" json:"bind_addr"`
	Peers        string `yaml:"peers" json:"peers"`
	SyncInterval int    `yaml:"sync_interval" json:"sync_interval"`
}

// PluginConfig holds plugin/app marketplace configuration.
type PluginConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	Registry   string `yaml:"registry" json:"registry"`
	AutoUpdate bool   `yaml:"auto_update" json:"auto_update"`
}

// DHCPHAConfig holds DHCP High Availability configuration.
type DHCPHAConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	Role         string `yaml:"role" json:"role"` // primary, secondary
	PeerAddress  string `yaml:"peer_address" json:"peer_address"`
	PeerPort     int    `yaml:"peer_port" json:"peer_port"`
	SyncInterval int    `yaml:"sync_interval" json:"sync_interval"`
}

// DefaultDoTConfig returns default DoT configuration.
func DefaultDoTConfig() DoTConfig {
	return DoTConfig{
		Enabled: false,
		Address: ":853",
	}
}

// DefaultDoHConfig returns default DoH configuration.
func DefaultDoHConfig() DoHConfig {
	return DoHConfig{
		Enabled: false,
		Address: ":8443",
		Path:    "/dns-query",
	}
}

// DefaultDoQConfig returns default DoQ configuration.
func DefaultDoQConfig() DoQConfig {
	return DoQConfig{
		Enabled: false,
		Address: ":853",
	}
}

// DefaultSSOConfig returns default SSO configuration.
func DefaultSSOConfig() SSOConfig {
	return SSOConfig{
		Enabled:      false,
		Provider:     "oidc",
		Scopes:       "openid,profile,email",
		AutoRegister: false,
	}
}

// DefaultClusterConfig returns default cluster configuration.
func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		Enabled:      false,
		NodeRole:     "primary",
		BindAddr:     ":7946",
		SyncInterval: 30,
	}
}

// DefaultPluginConfig returns default plugin configuration.
func DefaultPluginConfig() PluginConfig {
	return PluginConfig{
		Enabled:    false,
		AutoUpdate: false,
	}
}

// DefaultDHCPHAConfig returns default DHCP HA configuration.
func DefaultDHCPHAConfig() DHCPHAConfig {
	return DHCPHAConfig{
		Enabled:      false,
		Role:         "primary",
		PeerPort:     647,
		SyncInterval: 30,
	}
}
