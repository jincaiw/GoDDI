package lease

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/facts"
)

// FactEnvelope builds the migration-era DHCP lease observation fact for a
// mutation. It is an adapter only: the caller supplies the canonical event ID,
// sequence and space ID, and must enqueue the returned envelope in the same
// authoritative transaction as the lease mutation.
func (c MutationCommand) FactEnvelope(eventID, source, spaceID string, sequence int64, occurredAt time.Time) (facts.Envelope, error) {
	if _, err := c.Validate(); err != nil {
		return facts.Envelope{}, err
	}
	if strings.TrimSpace(eventID) == "" || strings.TrimSpace(source) == "" || strings.TrimSpace(spaceID) == "" {
		return facts.Envelope{}, errors.New("lease mutation: fact identity fields are required")
	}
	if sequence <= 0 || occurredAt.IsZero() {
		return facts.Envelope{}, errors.New("lease mutation: canonical sequence and occurred_at are required")
	}
	if c.Kind == MutationOffer && (c.Before != nil || c.After.Generation != 0) {
		return facts.Envelope{}, fmt.Errorf("lease mutation: offer fact requires a generation-0 offer without a prior lease")
	}
	if c.Kind == MutationDecline && c.Before == nil {
		return DeclineTombstoneEnvelope(eventID, source, spaceID, c.After, sequence, occurredAt)
	}
	payloadLease := c.After
	payload := struct {
		Action    string `json:"action"`
		LeaseID   string `json:"lease_id"`
		ScopeID   string `json:"scope_id"`
		SpaceID   string `json:"space_id"`
		IP        string `json:"ip"`
		MAC       string `json:"mac"`
		Hostname  string `json:"hostname"`
		Tombstone bool   `json:"tombstone,omitempty"`
	}{
		Action: mutationFactAction(c.Kind), LeaseID: payloadLease.ID, ScopeID: payloadLease.ScopeID,
		SpaceID: spaceID, IP: payloadLease.IPAddress, MAC: payloadLease.MACAddress, Hostname: payloadLease.Hostname,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return facts.Envelope{}, fmt.Errorf("lease mutation: encode fact payload: %w", err)
	}
	envelope := facts.Envelope{
		EventID: eventID, Version: facts.CurrentEnvelopeVersion, Entity: "dhcp_lease",
		Action: string(c.Kind), Generation: payloadLease.Generation, Sequence: sequence,
		Source: source, OccurredAt: occurredAt.UTC(), PayloadVersion: 1, Payload: encoded,
	}
	if err := envelope.Validate(); err != nil {
		return facts.Envelope{}, err
	}
	return envelope, nil
}

func mutationFactAction(kind MutationKind) string {
	switch kind {
	case MutationActivate:
		return "bind"
	case MutationRenew:
		return "renew"
	case MutationRelease:
		return "release"
	case MutationDecline:
		return "decline"
	case MutationExpire:
		return "expire"
	case MutationOffer:
		return "discover_offer"
	default:
		return ""
	}
}
