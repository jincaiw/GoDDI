package main

import (
	"context"
	"database/sql"

	"github.com/jincaiw/GoDDI/internal/api/handler"
	"github.com/jincaiw/GoDDI/internal/dataplane"
	"github.com/jincaiw/GoDDI/internal/dhcp/ha"
	dhcpserver "github.com/jincaiw/GoDDI/internal/dhcp/server"
	"github.com/jincaiw/GoDDI/internal/ipam"
	"github.com/jincaiw/GoDDI/internal/metrics"
)

// refreshDataPlaneGauges updates D-14/D-20 gauges from the stores that are
// actually present in this process. DNS LKG must not be gated on DHCP: a
// role=dns process owns only dnsdata.db and still has to report what it serves.
func refreshDataPlaneGauges(ctx context.Context, dhcpDB, dnsDB *sql.DB) {
	if dhcpDB != nil {
		if gaps, err := ipam.ProjectionGaps(ctx, dhcpDB); err == nil {
			metrics.DHCPIPAMProjectionGaps.Set(float64(len(gaps)))
		}
		if rev, _, ok, err := dataplane.LKG(dhcpDB, dataplane.DomainDHCP); err == nil {
			metrics.SetDataPlaneLKG(string(dataplane.DomainDHCP), rev, ok)
		}
	}
	if dnsDB != nil {
		if rev, _, ok, err := dataplane.LKG(dnsDB, dataplane.DomainDNS); err == nil {
			metrics.SetDataPlaneLKG(string(dataplane.DomainDNS), rev, ok)
		}
	}
}

func dhcpStoreDB(store *dataplane.Store) *sql.DB {
	if store == nil {
		return nil
	}
	return store.DB
}

func dhcpStartupStatus(s *dhcpserver.Server) dhcpserver.StartupGateStatus {
	if s == nil {
		return dhcpserver.StartupGateStatus{State: "unconfigured", Ready: true}
	}
	return s.StartupStatus()
}

// mergeStartupStatus folds the optional DHCP recovery gate into readiness.
//
// The recovery gate is deliberately reported as a DHCP-plane failure when it
// is configured but not ready: the server must not admit clients until its
// durable projection has been recovered. Only bounded state is exposed in the
// authenticated detail endpoint; the coordinator's error text may contain
// paths or implementation details and is therefore never copied into the
// readiness payload.
func mergeStartupStatus(st *handler.PlaneStatus, status dhcpserver.StartupGateStatus) {
	if st == nil {
		return
	}
	st.Details = mergeDetails(st.Details, map[string]any{
		"startup_recovery_configured": status.Configured,
		"startup_recovery_ready":      status.Ready,
		"startup_recovery_state":      status.State,
		"startup_recovery_last_seq":   status.LastSeq,
	})
	if !status.Configured || status.Ready {
		return
	}

	st.Level = handler.Worse(st.Level, handler.LevelFailing)
	if status.State == "failed" {
		st.Reasons = append(st.Reasons, "startup_recovery_failed")
		return
	}
	st.Reasons = append(st.Reasons, "startup_recovery_not_ready")
}

// mergeHAStatus folds this node's redundancy into the DHCP plane's readiness.
//
// Nothing here produces failing. Only failing is a 503, and every state below
// is one in which the process is doing what it was told to: a paused primary is
// still answering DISCOVER and still honouring releases, a degraded one is
// serving on an operator's permission, and a fenced one is out of service on
// purpose. Reporting any of them as failing would ask an orchestrator to
// restart a process whose problem is not in the process.
func mergeHAStatus(st *handler.PlaneStatus, role string, repl *ha.Replicator, mirror *ha.Mirror) {
	state := currentHAState(role, repl, mirror)
	details := map[string]any{
		"ha_role":  role,
		"ha_state": state.String(),
	}
	if repl != nil {
		details["ha_seq"] = repl.Seq()
		details["ha_acked_seq"] = repl.AckedSeq()
		if up, lastSeen := repl.LinkSnapshot(); up {
			details["ha_peer_last_seen"] = lastSeen
		}
	}
	if mirror != nil {
		details["ha_applied_seq"] = mirror.AppliedSeq()
		if up, lastSeen := mirror.LinkSnapshot(); up {
			details["ha_peer_last_seen"] = lastSeen
		}
	}

	switch state {
	case ha.StatePrimary:
		// A second copy is present and this node may promise. Nothing to add.
	case ha.StatePrimaryDegraded:
		details["ha_redundant"] = false
		st.Level = handler.Worse(st.Level, handler.LevelDegraded)
		st.Reasons = append(st.Reasons, "dhcp_ha_serving_without_a_second_copy")
		if repl != nil {
			details["ha_degraded_reason"] = repl.DegradedReason()
		}
	case ha.StatePaused:
		details["ha_redundant"] = false
		st.Level = handler.Worse(st.Level, handler.LevelDegraded)
		st.Reasons = append(st.Reasons, "dhcp_ha_no_second_copy")
	case ha.StateStandby:
		// Whether the pair is redundant is answered from the primary, which is
		// the node that owns the fact. What a standby can say about itself is
		// whether it can still see the node whose promises it holds -- and
		// losing that is what stops it being useful.
		details["ha_redundant"] = false
		if up, _ := mirror.LinkSnapshot(); !up {
			st.Level = handler.Worse(st.Level, handler.LevelDegraded)
			st.Reasons = append(st.Reasons, "dhcp_ha_primary_unreachable")
		} else {
			details["ha_redundant"] = true
		}
	case ha.StateFenced:
		details["ha_redundant"] = false
		st.Level = handler.Worse(st.Level, handler.LevelDegraded)
		st.Reasons = append(st.Reasons, "dhcp_ha_fenced")
	}
	st.Details = mergeDetails(st.Details, details)
}

// mergeDetails folds this node's redundancy figures into the payload the
// authenticated detail endpoint serves.
func mergeDetails(existing any, extra map[string]any) map[string]any {
	if m, ok := existing.(map[string]any); ok {
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	return extra
}

// dataPlaneStatus renders a runner's readiness for the API layer, including
// the figures the authenticated endpoint serves.
//
// A process that serves a plane but has no runner is reported failing, not
// ok: it has nothing to report, and "ok" from a probe that checked nothing is
// the answer this endpoint exists to avoid.
func dataPlaneStatus(r *dataplane.Runner) handler.PlaneStatus {
	if r == nil {
		return handler.PlaneStatus{
			Level:   handler.LevelFailing,
			Reasons: []string{"data_plane_not_configured"},
		}
	}

	rd := r.Probe()
	st := handler.PlaneStatus{Level: string(rd.Level), Reasons: rd.Reasons}
	st.Details = dataPlaneDetails(r, rd)
	return st
}

// pendingQueues renders the upward queues for the detail endpoint. The names
// come from the data plane rather than from a list here: the detail payload,
// the Prometheus sample and the quota bounds all describe the same set, and a
// second copy is how a queue ends up missing from one of them.
func pendingQueues(status dataplane.Status) map[string]int {
	queues := status.PendingByQueue()
	out := make(map[string]int, len(queues))
	for _, q := range queues {
		out[q.Name] = q.Pending
	}
	return out
}

// dataPlaneDetails is the payload of the authenticated detail endpoint. The
// unauthenticated /ready never builds it.
func dataPlaneDetails(r *dataplane.Runner, rd dataplane.Readiness) map[string]any {
	status := rd.Status
	details := map[string]any{
		"control_reachable": status.ControlReachable,
		"pending":           pendingQueues(status),
		"held_leases":       status.HeldLeases,
		"refused_rows":      status.RefusedRows,
		"probed_at":         rd.At,
	}
	if status.LastError != "" {
		details["last_error"] = status.LastError
	}
	if !status.LastSuccess.IsZero() {
		details["last_success"] = status.LastSuccess
	}
	if n, err := r.Store().FileSize(); err == nil {
		details["store_bytes"] = n
	}
	if len(rd.Quota) > 0 {
		breaches := make([]string, 0, len(rd.Quota))
		for _, b := range rd.Quota {
			breaches = append(breaches, b.String())
		}
		details["over_quota"] = breaches
	}
	return details
}

// dataPlaneSample renders a runner's probe view for Prometheus. It reads the
// same memory-only view the readiness probe does, so the ten-second metrics
// tick never touches the store.
func dataPlaneSample(plane string, r *dataplane.Runner) metrics.DataPlaneSample {
	rd := r.Probe()
	status := rd.Status

	sample := metrics.DataPlaneSample{
		Plane:       plane,
		Level:       string(rd.Level),
		HeldLeases:  status.HeldLeases,
		Pending:     pendingQueues(status),
		RefusedRows: status.RefusedRows,
		QuotaRatio:  map[string]float64{},
	}
	if n, err := r.Store().FileSize(); err == nil {
		sample.StoreBytes = n
	}

	// The ratio is how close each bound is, not only whether it has been
	// crossed: an alert written against "over the bound" fires after the
	// fact, and one written against the ratio can fire before it.
	q := r.Quota()
	if q.MaxLeases > 0 {
		sample.QuotaRatio["leases"] = float64(status.HeldLeases) / float64(q.MaxLeases)
	}
	if q.MaxFileBytes > 0 && sample.StoreBytes > 0 {
		sample.QuotaRatio["store_bytes"] = float64(sample.StoreBytes) / float64(q.MaxFileBytes)
	}
	if q.MaxBacklog > 0 {
		for queue, n := range sample.Pending {
			sample.QuotaRatio["queue_"+queue] = float64(n) / float64(q.MaxBacklog)
		}
	}
	return sample
}
