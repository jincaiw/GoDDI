package lease

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestMutationCommandFactEnvelopeUsesCanonicalIdentityAndAction(t *testing.T) {
	before := &Lease{ID: "lease-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa:bb", Status: LeaseStatusOffered, Generation: 0}
	after := &Lease{ID: "lease-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa:bb", Hostname: "host", Status: LeaseStatusActive, Generation: 1}
	cmd := MutationCommand{Kind: MutationActivate, Before: before, After: after}
	event, err := cmd.FactEnvelope("event-1", "dhcp", "space-1", 7, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if event.EventID != "event-1" || event.Sequence != 7 || event.Generation != 1 || event.Action != string(MutationActivate) {
		t.Fatalf("envelope identity = %+v", event)
	}
	if string(event.Payload) == "" || event.Source != "dhcp" || event.Entity != "dhcp_lease" {
		t.Fatalf("envelope metadata = %+v", event)
	}
}

func TestMutationCommandFactEnvelopeBuildsOfferFact(t *testing.T) {
	offer := &Lease{
		ID: "offer-1", ScopeID: "scope-1", IPAddress: "192.0.2.10",
		MACAddress: "aa:bb", Hostname: "host", Status: LeaseStatusOffered,
		Generation: 0,
	}
	event, err := (MutationCommand{Kind: MutationOffer, After: offer}).FactEnvelope(
		"event-offer-1", "dhcp-node-a", "space-1", 3,
		time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("build offer envelope: %v", err)
	}
	if event.Action != string(MutationOffer) || event.Generation != 0 || event.Sequence != 3 {
		t.Fatalf("offer envelope = %+v", event)
	}
	var payload struct {
		Action   string `json:"action"`
		LeaseID  string `json:"lease_id"`
		ScopeID  string `json:"scope_id"`
		SpaceID  string `json:"space_id"`
		IP       string `json:"ip"`
		MAC      string `json:"mac"`
		Hostname string `json:"hostname"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode offer payload: %v", err)
	}
	if payload.Action != string(MutationOffer) || payload.LeaseID != offer.ID ||
		payload.ScopeID != offer.ScopeID || payload.SpaceID != "space-1" ||
		payload.IP != offer.IPAddress || payload.MAC != offer.MACAddress ||
		payload.Hostname != offer.Hostname {
		t.Fatalf("offer payload = %+v", payload)
	}
}

func TestMutationCommandFactEnvelopeRejectsInvalidOfferShape(t *testing.T) {
	base := &Lease{ID: "offer-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa:bb", Status: LeaseStatusOffered, Generation: 0}
	cases := []struct {
		name   string
		before *Lease
		after  *Lease
	}{
		{"prior lease", &Lease{ID: "offer-1", Status: LeaseStatusReleased}, base},
		{"nonzero generation", nil, &Lease{ID: base.ID, ScopeID: base.ScopeID, IPAddress: base.IPAddress, MACAddress: base.MACAddress, Status: base.Status, Generation: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (MutationCommand{Kind: MutationOffer, Before: tc.before, After: tc.after}).FactEnvelope("event", "node", "space", 1, time.Now().UTC())
			if err == nil {
				t.Fatal("invalid offer shape accepted")
			}
		})
	}
}

func TestMutationCommandFactEnvelopeRejectsMissingCanonicalFields(t *testing.T) {
	cmd := MutationCommand{Kind: MutationRenew, Before: &Lease{ID: "lease-1", Status: LeaseStatusActive, Generation: 1}, After: &Lease{ID: "lease-1", Status: LeaseStatusActive, Generation: 2}}
	cases := []struct {
		name     string
		eventID  string
		source   string
		spaceID  string
		sequence int64
	}{
		{"event id", "", "dhcp", "space-1", 1},
		{"source", "event-1", "", "space-1", 1},
		{"space", "event-1", "dhcp", "", 1},
		{"sequence", "event-1", "dhcp", "space-1", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cmd.FactEnvelope(tc.eventID, tc.source, tc.spaceID, tc.sequence, time.Now().UTC())
			if err == nil || !errors.Is(err, ErrInvalidMutation) && tc.name != "sequence" {
				if err == nil {
					t.Fatal("missing canonical field was accepted")
				}
			}
		})
	}
}
