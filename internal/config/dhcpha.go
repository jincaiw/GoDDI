package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// The floor on the shared secret that authenticates the HA peer channel.
//
// A three-character token on a channel that can rewrite lease state is not a
// shared secret, it is a formality. The same floor applies to security.jwt_secret
// and security.encryption_key, and it is here for the same reason.
const minHAPeerTokenLength = 16

// HAConfirmTimeout returns the parsed confirm timeout.
func (c DHCPHAConfig) HAConfirmTimeout() (time.Duration, error) {
	return parseHADuration("dhcp_ha.confirm_timeout", c.ConfirmTimeout)
}

// HAHeartbeatInterval returns the parsed heartbeat interval.
func (c DHCPHAConfig) HAHeartbeatInterval() (time.Duration, error) {
	return parseHADuration("dhcp_ha.heartbeat_interval", c.HeartbeatInterval)
}

// HAPeerStaleAfter returns the parsed staleness bound.
func (c DHCPHAConfig) HAPeerStaleAfter() (time.Duration, error) {
	return parseHADuration("dhcp_ha.peer_stale_after", c.PeerStaleAfter)
}

func parseHADuration(setting, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s must be set (e.g. 2s)", setting)
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s is not a duration: %w", setting, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", setting, raw)
	}
	return d, nil
}

// validateDHCPHA refuses a configuration that would produce an instance which
// believes it has redundancy it does not have.
//
// Every check here is fail-closed at startup for the same reason the management
// allowlist and the relay allowlist are: the failure mode of carrying on is
// silent. A node started with dhcp_ha.enabled = true and no reachable peer will
// serve clients, look unhealthy to nobody, and lose addresses on the first
// power cut -- and none of that is visible from the outside. An operator who
// cannot start a node can fix it; an operator who does not know cannot.
//
// A disabled node is not checked at all. Settings left behind from an earlier
// experiment must not be able to stop a node from starting when HA is off, and
// nothing behind this function reads them in that case.
func validateDHCPHA(c *DHCPHAConfig) error {
	if !c.Enabled {
		return nil
	}

	switch c.HARoleNormalized() {
	case HARolePrimary, HARoleStandby:
	default:
		return fmt.Errorf("dhcp_ha.role must be %q or %q, got %q",
			HARolePrimary, HARoleStandby, c.Role)
	}

	if strings.TrimSpace(c.NodeID) == "" {
		// The two nodes must be distinguishable: the watermarks are exchanged
		// between them and "which node was that" has to have an answer.
		return fmt.Errorf("dhcp_ha.node_id must be set when dhcp_ha.enabled is true: " +
			"the two nodes of a pair must be distinguishable by name")
	}

	if err := validateHAPeerAddress("dhcp_ha.listen_addr", c.ListenAddr); err != nil {
		return err
	}
	if err := validateHAPeerAddress("dhcp_ha.peer_address", c.PeerAddress); err != nil {
		return err
	}
	if strings.TrimSpace(c.ListenAddr) == strings.TrimSpace(c.PeerAddress) {
		// A node pointed at itself would confirm every binding against its own
		// store and report a healthy pair. That is the single worst outcome
		// this setting can produce, and it parses perfectly well.
		return fmt.Errorf("dhcp_ha.listen_addr and dhcp_ha.peer_address are both %q: "+
			"a node cannot be its own peer", c.PeerAddress)
	}

	token := strings.TrimSpace(c.PeerToken)
	if token == "" {
		return fmt.Errorf("dhcp_ha.peer_token must be set when dhcp_ha.enabled is true " +
			"(set GODDI_DHCP_HA_PEER_TOKEN rather than writing it into config.yaml)")
	}
	if len(token) < minHAPeerTokenLength {
		return fmt.Errorf("dhcp_ha.peer_token must be at least %d characters long (current: %d): "+
			"it authenticates a channel that can rewrite lease state",
			minHAPeerTokenLength, len(token))
	}

	confirm, err := c.HAConfirmTimeout()
	if err != nil {
		return err
	}
	heartbeat, err := c.HAHeartbeatInterval()
	if err != nil {
		return err
	}
	stale, err := c.HAPeerStaleAfter()
	if err != nil {
		return err
	}

	if confirm >= stale {
		// If the peer is declared stale while a binding is still waiting for
		// it, the node both withholds the ACK and reports the pair broken --
		// and the second of those is the one an operator acts on.
		return fmt.Errorf("dhcp_ha.confirm_timeout (%s) must be shorter than dhcp_ha.peer_stale_after (%s): "+
			"otherwise the peer is declared stale while a binding is still waiting for it", confirm, stale)
	}
	if heartbeat >= stale {
		return fmt.Errorf("dhcp_ha.heartbeat_interval (%s) must be shorter than dhcp_ha.peer_stale_after (%s): "+
			"otherwise the peer reads as stale between two of its own heartbeats", heartbeat, stale)
	}

	return nil
}

func validateHAPeerAddress(setting, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%s must be set when dhcp_ha.enabled is true (host:port)", setting)
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return fmt.Errorf("%s must be host:port, got %q: %w", setting, raw, err)
	}
	// An empty host is allowed and means "every interface", which is what a
	// listen address normally wants. A peer address without a host does not
	// mean anything, so the two settings differ here.
	if setting == "dhcp_ha.peer_address" && strings.TrimSpace(host) == "" {
		return fmt.Errorf("%s must name a host, got %q", setting, raw)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%s has an invalid port %q: expected 1-65535", setting, port)
	}
	return nil
}
