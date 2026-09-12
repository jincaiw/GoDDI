package config

import "strings"

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
//
// The shape of this struct is the contract in docs/adr/0003-dhcp-ha-contract.md:
// a node is either the primary -- the only author of lease facts -- or a
// standby, which is a pure mirror; the two talk over a direct channel that does
// not pass through the control database; and every duration that decides
// whether the pair is healthy, and therefore whether a binding may be
// acknowledged, is stated here rather than defaulted somewhere in the code.
//
// The peer_token is a shared secret and is deliberately readable from the
// environment (GODDI_DHCP_HA_PEER_TOKEN) so that it does not have to be written
// into config.yaml.
type DHCPHAConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	NodeID  string `yaml:"node_id" json:"node_id"`
	Role    string `yaml:"role" json:"role"` // primary | standby
	// ListenAddr is where this node accepts its peer. Unused when Enabled is
	// false: a disabled node binds no HA port at all.
	ListenAddr  string `yaml:"listen_addr" json:"listen_addr"`
	PeerAddress string `yaml:"peer_address" json:"peer_address"`
	PeerToken   string `yaml:"peer_token" json:"peer_token"`
	// ConfirmTimeout bounds how long a binding waits for the second copy
	// before the client is left unanswered. It must stay below the client's
	// retransmission interval, or the client retransmits while we are still
	// waiting and the load doubles for no gain.
	ConfirmTimeout    string `yaml:"confirm_timeout" json:"confirm_timeout"`
	HeartbeatInterval string `yaml:"heartbeat_interval" json:"heartbeat_interval"`
	// PeerStaleAfter is how long the peer may be silent before this node
	// considers the pair broken. It only ever moves a primary into paused; it
	// is never a reason to promote. See ADR 0003 decision 4.
	PeerStaleAfter string `yaml:"peer_stale_after" json:"peer_stale_after"`
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
//
// Disabled by default, which is the whole of the compatibility promise: an
// installation that does not set dhcp_ha.enabled behaves exactly as it did
// before this contract existed, and binds no HA port.
func DefaultDHCPHAConfig() DHCPHAConfig {
	return DHCPHAConfig{
		Enabled:           false,
		Role:              HARolePrimary,
		ListenAddr:        "0.0.0.0:647",
		ConfirmTimeout:    "2s",
		HeartbeatInterval: "1s",
		PeerStaleAfter:    "5s",
	}
}

// The two roles a node may take in a DHCP HA pair.
const (
	HARolePrimary = "primary"
	HARoleStandby = "standby"
)

// HARoleNormalized returns the configured role in canonical form. An empty
// role reads as primary, which is the role every existing installation is.
func (c DHCPHAConfig) HARoleNormalized() string {
	switch strings.ToLower(strings.TrimSpace(c.Role)) {
	case "", HARolePrimary:
		return HARolePrimary
	case HARoleStandby:
		return HARoleStandby
	default:
		return strings.ToLower(strings.TrimSpace(c.Role))
	}
}

// IsStandby reports whether this node is the mirror rather than the author.
func (c DHCPHAConfig) IsStandby() bool {
	return c.Enabled && c.HARoleNormalized() == HARoleStandby
}
