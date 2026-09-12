package server

import (
	"encoding/hex"
	"fmt"
)

// relayAgentInfo is the small, auditable subset of RFC 3046 that the DHCP
// server uses for relay attribution. Unknown sub-options are accepted and
// retained only as a count; their contents are not trusted for policy.
type relayAgentInfo struct {
	CircuitID      []byte
	RemoteID       []byte
	UnknownSubopts int
}

// parseRelayAgentInfo parses the raw payload of DHCP option 82. The upstream
// DHCP library intentionally returns nil for malformed relay options, which
// cannot distinguish an absent option from a malformed one. The server must
// fail closed for a present but malformed Option 82, so parse the TLV payload
// here and preserve that distinction.
func parseRelayAgentInfo(data []byte) (relayAgentInfo, error) {
	var info relayAgentInfo
	seenCircuit := false
	seenRemote := false

	for offset := 0; offset < len(data); {
		if len(data)-offset < 2 {
			return relayAgentInfo{}, fmt.Errorf("truncated sub-option header at byte %d", offset)
		}
		code := data[offset]
		length := int(data[offset+1])
		offset += 2
		if length > len(data)-offset {
			return relayAgentInfo{}, fmt.Errorf("sub-option %d length %d exceeds remaining %d bytes", code, length, len(data)-offset)
		}
		value := append([]byte(nil), data[offset:offset+length]...)
		offset += length

		switch code {
		case 1: // Agent Circuit ID, RFC 3046 section  sub-option 1.
			if seenCircuit {
				return relayAgentInfo{}, fmt.Errorf("duplicate circuit-id sub-option")
			}
			if len(value) == 0 {
				return relayAgentInfo{}, fmt.Errorf("circuit-id sub-option is empty")
			}
			seenCircuit = true
			info.CircuitID = value
		case 2: // Agent Remote ID, RFC 3046 sub-option 2.
			if seenRemote {
				return relayAgentInfo{}, fmt.Errorf("duplicate remote-id sub-option")
			}
			if len(value) == 0 {
				return relayAgentInfo{}, fmt.Errorf("remote-id sub-option is empty")
			}
			seenRemote = true
			info.RemoteID = value
		default:
			info.UnknownSubopts++
		}
	}

	if len(info.CircuitID) == 0 {
		return relayAgentInfo{}, fmt.Errorf("circuit-id sub-option is required")
	}
	return info, nil
}

func relayAgentInfoLogValue(value []byte) string {
	// Relay identifiers are opaque and may contain control characters or PII.
	// Log bounded hex rather than raw strings, and never log more than 64 bytes.
	if len(value) > 64 {
		value = value[:64]
	}
	return hex.EncodeToString(value)
}
