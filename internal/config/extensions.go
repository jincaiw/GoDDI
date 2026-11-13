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

	// ConfigAuthor, when true, lets this node accept configuration writes
	// (zones, records, scopes, IPAM objects). It is the single-author rule of
	// ADR-0005 in its minimal form: exactly one node in a deployment may
	// author configuration, so two nodes can never publish divergent
	// revisions for the same resource. It defaults to true, which keeps every
	// single-node deployment unchanged.
	//
	// This is NOT the full ADR-0005 coordinator: there is no leader election,
	// quorum or witness here, and the flag is a deployment-time statement by
	// the operator rather than something a node can prove to itself. It
	// removes the "two consoles, two divergent configs" failure mode; it does
	// not remove the need for the coordinator before multi-node config
	// publishing can be called complete.
	ConfigAuthor *bool `yaml:"config_author" json:"config_author"`
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
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"`
	// ListenAddrBackup is a second address this node accepts its peer on (the
	// second cable). The primary listens on every configured address; the
	// mirror uses whichever of its peer paths answers.
	ListenAddrBackup string `yaml:"listen_addr_backup" json:"listen_addr_backup"`
	PeerAddress      string `yaml:"peer_address" json:"peer_address"`
	PeerToken        string `yaml:"peer_token" json:"peer_token"`
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
	// TLS material for the peer channel. When tls_cert_file is set, the HA
	// replication channel uses mutual TLS (the same node certificate is
	// presented on dial and required on accept, verified against tls_ca_file).
	// All three file settings are required together; empty means plaintext +
	// shared token (legacy).
	TLSCertFile   string `yaml:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile    string `yaml:"tls_key_file" json:"tls_key_file"`
	TLSCAFile     string `yaml:"tls_ca_file" json:"tls_ca_file"`
	TLSServerName string `yaml:"tls_server_name" json:"tls_server_name"`

	// PeerAddressBackup is a second, independent path to the same peer
	// (the second cable). The mirror tries PeerAddress first and falls back
	// to this one; the peer is considered reachable while *either* path
	// works, so a single cable or NIC failing does not look like a dead peer.
	// Empty means a single path.
	PeerAddressBackup string `yaml:"peer_address_backup" json:"peer_address_backup"`
	// Priority breaks a tie when both nodes could serve: the higher number is
	// preferred. It does not on its own move a healthy primary; see Preempt.
	Priority int `yaml:"priority" json:"priority"`
	// Preempt, when true, lets a higher-priority node take the primary role
	// back after it rejoins. Default false: a restarted node comes back as a
	// standby and stays there, so a flapping link cannot cause a takeover
	// storm. Taking back the role still requires the epoch/fencing checks.
	Preempt bool `yaml:"preempt" json:"preempt"`
	// AutoFailover, when true, lets the standby promote itself after it has
	// lost the primary on *every* configured path for AutoFailoverWindows
	// consecutive heartbeat windows, provided it has the higher priority.
	// Default false. Promotion is durable (epoch+1) and takes effect on the
	// next process start, so it needs a supervisor that restarts the process.
	// When false (the default) the pair keeps the fail-closed behaviour of
	// ADR-0003/ADR-0010: a standby never promotes itself and only an operator
	// does. Enabling it trades that safety margin for availability and should
	// be a deliberate deployment decision.
	AutoFailover bool `yaml:"auto_failover" json:"auto_failover"`
	// AutoFailoverWindows is how many consecutive failed connection windows
	// the standby waits before promoting. 0 means the built-in default (3).
	AutoFailoverWindows int `yaml:"auto_failover_windows" json:"auto_failover_windows"`
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

// IsConfigAuthor reports whether this node may write configuration. The default
// (unset) is true so existing single-node deployments keep working.
func (c ClusterConfig) IsConfigAuthor() bool {
	if c.ConfigAuthor == nil {
		return true
	}
	return *c.ConfigAuthor
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
