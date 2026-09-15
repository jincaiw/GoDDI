package lease

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jasonwa/goddi/internal/facts"
)

// DeclineTombstoneFact is the payload for a DECLINE about an address that had
// no prior lease on this server. The generated conflict lease ID is retained as
// the durable identity of the quarantine row; Tombstone distinguishes it from
// a normal lease transition even though both project to a contested address.
type DeclineTombstoneFact struct {
	Action    string `json:"action"`
	LeaseID   string `json:"lease_id"`
	ScopeID   string `json:"scope_id"`
	SpaceID   string `json:"space_id"`
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Tombstone bool   `json:"tombstone"`
}

// DeclineTombstoneEnvelope builds a unified fact for a durable conflict
// tombstone. It is an adapter only: the caller must create the tombstone and
// supply its stable ID, source, resolved space ID, sequence, and timestamp.
// It does not write the lease row or the outbox and is not connected to the
// default DHCP DECLINE path.
func DeclineTombstoneEnvelope(eventID, source, spaceID string, tombstone *Lease, sequence int64, occurredAt time.Time) (facts.Envelope, error) {
	if strings.TrimSpace(eventID) == "" || strings.TrimSpace(source) == "" || strings.TrimSpace(spaceID) == "" {
		return facts.Envelope{}, errors.New("lease decline tombstone: fact identity fields are required")
	}
	if tombstone == nil || strings.TrimSpace(tombstone.ID) == "" || strings.TrimSpace(tombstone.ScopeID) == "" || strings.TrimSpace(tombstone.IPAddress) == "" || strings.TrimSpace(tombstone.MACAddress) == "" {
		return facts.Envelope{}, errors.New("lease decline tombstone: complete tombstone is required")
	}
	if tombstone.Status != LeaseStatusConflict || tombstone.Generation != 0 {
		return facts.Envelope{}, fmt.Errorf("%w: tombstone must be conflict with generation 0", ErrInvalidMutationState)
	}
	if sequence <= 0 || occurredAt.IsZero() {
		return facts.Envelope{}, errors.New("lease decline tombstone: canonical sequence and occurred_at are required")
	}

	payload, err := json.Marshal(DeclineTombstoneFact{
		Action: "decline", LeaseID: tombstone.ID, ScopeID: tombstone.ScopeID,
		SpaceID: spaceID, IP: tombstone.IPAddress, MAC: tombstone.MACAddress,
		Hostname: tombstone.Hostname, Tombstone: true,
	})
	if err != nil {
		return facts.Envelope{}, fmt.Errorf("lease decline tombstone: encode payload: %w", err)
	}
	event := facts.Envelope{
		EventID: eventID, Version: facts.CurrentEnvelopeVersion, Entity: "dhcp_lease",
		Action: string(MutationDecline), Generation: tombstone.Generation,
		Sequence: sequence, Source: source, OccurredAt: occurredAt.UTC(),
		PayloadVersion: 1, Payload: payload,
	}
	if err := event.Validate(); err != nil {
		return facts.Envelope{}, err
	}
	return event, nil
}
