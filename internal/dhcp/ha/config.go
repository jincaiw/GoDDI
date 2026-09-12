package ha

import (
	"fmt"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/config"
)

// Config is the HA settings this package works from, with the durations
// already parsed.
//
// It is a separate type from config.DHCPHAConfig so that this package's
// behaviour cannot depend on a string that has not been checked, and so that a
// unit test can build a pair of nodes without writing YAML.
type Config struct {
	// Enabled is the operator's switch. The mechanism refuses the operator
	// actions when it is false: a node with no second copy to lose and no peer
	// to promote away from has nothing for them to mean.
	Enabled bool
	// NodeID names this node. The two nodes of a pair must differ; the
	// watermarks are exchanged between them and "which node said that" has to
	// have an answer in a log.
	NodeID string
	// Role is primary or standby.
	Role string
	// ListenAddr is where the primary accepts its standby. A standby binds
	// nothing: the standby is the side that dials, so that a standby which
	// restarts re-establishes the mirror promptly, which is the direction the
	// resync matters in.
	ListenAddr string
	// PeerAddress is, on the standby, the primary's listen address. On the
	// primary it is recorded for status only.
	PeerAddress string
	// PeerToken is the shared secret the dialling side presents.
	PeerToken string
	// ConfirmTimeout bounds how long a binding waits for the second copy.
	ConfirmTimeout time.Duration
	// HeartbeatInterval is how often the primary asks for a confirmation when
	// there is nothing to send.
	HeartbeatInterval time.Duration
	// PeerStaleAfter is how long a silent peer takes this node out of the
	// state in which it may promise addresses.
	PeerStaleAfter time.Duration
}

// FromConfig resolves the operator's settings into a Config.
//
// Validation has already run, so an error here means the two disagree about
// what a valid setting is -- which is worth surfacing rather than defaulting.
func FromConfig(c config.DHCPHAConfig) (Config, error) {
	confirm, err := c.HAConfirmTimeout()
	if err != nil {
		return Config{}, err
	}
	heartbeat, err := c.HAHeartbeatInterval()
	if err != nil {
		return Config{}, err
	}
	stale, err := c.HAPeerStaleAfter()
	if err != nil {
		return Config{}, err
	}
	return Config{
		Enabled:           c.Enabled,
		NodeID:            strings.TrimSpace(c.NodeID),
		Role:              c.HARoleNormalized(),
		ListenAddr:        strings.TrimSpace(c.ListenAddr),
		PeerAddress:       strings.TrimSpace(c.PeerAddress),
		PeerToken:         strings.TrimSpace(c.PeerToken),
		ConfirmTimeout:    confirm,
		HeartbeatInterval: heartbeat,
		PeerStaleAfter:    stale,
	}, nil
}

// IsStandby reports whether this node is the mirror.
func (c Config) IsStandby() bool { return c.Role == config.HARoleStandby }

// ForRole returns these settings with the role in force applied.
//
// Config.Role is the configuration's declaration of intent, which is only the
// seed: a promotion or a fence can have changed it since, and the change is
// recorded in the store rather than in the file. A component built for a role
// has to be built for the role that is actually in force, and this is the one
// place that substitution happens -- without it, a node that an operator has
// just promoted still asks for a standby and refuses to start.
//
// The distinction is why the field keeps its name: readers of the configured
// value (a report showing what the file says, an operator action comparing the
// two) want the declaration, and only a constructor wants the substitution.
func (c Config) ForRole(role string) Config {
	c.Role = role
	return c
}

// Describe renders the settings that are safe to log. The token is not one of
// them: a shared secret in a log line is a shared secret in every log archive
// the line reaches.
func (c Config) Describe() string {
	return fmt.Sprintf("role=%s node_id=%s listen=%s peer=%s confirm_timeout=%s heartbeat=%s peer_stale_after=%s",
		c.Role, c.NodeID, c.ListenAddr, c.PeerAddress,
		c.ConfirmTimeout, c.HeartbeatInterval, c.PeerStaleAfter)
}
