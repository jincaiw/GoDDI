package handler

import (
	"errors"
	"testing"

	"github.com/jincaiw/GoDDI/internal/facts"
)

func TestDHCPFactsPlaneMapsTheOutboxState(t *testing.T) {
	cases := []struct {
		name   string
		status facts.ObservationOutboxStatus
		err    error
		want   string
		reason string
	}{
		{"empty outbox is ok", facts.ObservationOutboxStatus{}, nil, LevelOK, ""},
		{"pending backlog is degraded", facts.ObservationOutboxStatus{Pending: 3, HeadSequence: 3, FirstOutstandingSequence: 1}, nil, LevelDegraded, "dhcp_facts_backlog"},
		{"failed events are failing", facts.ObservationOutboxStatus{Failed: 1, HeadSequence: 5, FirstOutstandingSequence: 5}, nil, LevelFailing, "dhcp_facts_failed_events"},
		{"status error is failing", facts.ObservationOutboxStatus{}, errors.New("closed"), LevelFailing, "dhcp_facts_status_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DHCPFactsPlane(tc.status, tc.err)
			if got.Level != tc.want {
				t.Fatalf("level = %v, want %v", got.Level, tc.want)
			}
			if tc.reason != "" {
				if len(got.Reasons) == 0 || got.Reasons[0] != tc.reason {
					t.Fatalf("reasons = %v, want [%s]", got.Reasons, tc.reason)
				}
			} else if len(got.Reasons) != 0 {
				t.Fatalf("ok plane must carry no reasons, got %v", got.Reasons)
			}
		})
	}
}
