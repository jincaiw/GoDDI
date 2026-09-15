package lease

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDeclineTombstoneEnvelopeBuildsContestedFact(t *testing.T) {
	tombstone := &Lease{ID: "tomb-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa:bb", Status: LeaseStatusConflict, Generation: 0}
	event, err := DeclineTombstoneEnvelope("event-tomb-1", "dhcp-node-a", "space-1", tombstone, 9, time.Date(2026, 9, 14, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	if event.Entity != "dhcp_lease" || event.Action != string(MutationDecline) || event.Generation != 0 || event.Sequence != 9 {
		t.Fatalf("event = %+v", event)
	}
	var payload DeclineTombstoneFact
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.LeaseID != "tomb-1" || payload.SpaceID != "space-1" || payload.IP != "192.0.2.10" || !payload.Tombstone {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestDeclineTombstoneEnvelopeRejectsNonTombstoneState(t *testing.T) {
	cases := []struct {
		name  string
		lease *Lease
	}{
		{"nil", nil},
		{"active", &Lease{ID: "l1", ScopeID: "s1", IPAddress: "192.0.2.1", MACAddress: "aa", Status: LeaseStatusActive}},
		{"wrong generation", &Lease{ID: "l1", ScopeID: "s1", IPAddress: "192.0.2.1", MACAddress: "aa", Status: LeaseStatusConflict, Generation: 1}},
		{"missing ip", &Lease{ID: "l1", ScopeID: "s1", MACAddress: "aa", Status: LeaseStatusConflict}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DeclineTombstoneEnvelope("event", "node", "space", tc.lease, 1, time.Now().UTC()); err == nil {
				t.Fatal("invalid tombstone accepted")
			}
		})
	}
}

func TestDeclineTombstoneEnvelopeRequiresIdentityAndOrdering(t *testing.T) {
	tombstone := &Lease{ID: "tomb-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa", Status: LeaseStatusConflict}
	cases := []struct {
		name, eventID, source, space string
		sequence                     int64
	}{
		{"missing event", "", "node", "space", 1},
		{"missing source", "event", "", "space", 1},
		{"missing space", "event", "node", "", 1},
		{"invalid sequence", "event", "node", "space", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DeclineTombstoneEnvelope(tc.eventID, tc.source, tc.space, tombstone, tc.sequence, time.Now().UTC()); err == nil {
				t.Fatal("invalid identity or sequence accepted")
			}
		})
	}
}
