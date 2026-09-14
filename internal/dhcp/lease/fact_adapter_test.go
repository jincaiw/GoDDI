package lease

import (
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
