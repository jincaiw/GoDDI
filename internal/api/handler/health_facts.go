package handler

// The DHCP facts producer plane: the observation outbox's lag/gap state as a
// readiness plane. Failed events are retained in the outbox and must never be
// disguised as health (a failed event is a fact that did not leave the
// producer); a pending backlog is degraded (the facts are written but not yet
// consumed); an empty outbox is ok.
//
// The mapping lives here rather than in the wiring so the readiness semantics
// are testable without a DHCP-enabled server.
import "github.com/jincaiw/GoDDI/internal/facts"

func DHCPFactsPlane(status facts.ObservationOutboxStatus, err error) PlaneStatus {
	if err != nil {
		return PlaneStatus{Level: LevelFailing, Reasons: []string{"dhcp_facts_status_unavailable"}}
	}
	switch {
	case status.Failed > 0:
		return PlaneStatus{Level: LevelFailing, Reasons: []string{"dhcp_facts_failed_events"}}
	case status.Pending > 0:
		return PlaneStatus{Level: LevelDegraded, Reasons: []string{"dhcp_facts_backlog"}}
	default:
		return PlaneStatus{Level: LevelOK}
	}
}
