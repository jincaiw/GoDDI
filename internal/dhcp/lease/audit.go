package lease

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jasonwa/goddi/internal/auditlog"
	"github.com/jasonwa/goddi/internal/metrics"
)

// leaseState is the part of a lease row an audit entry describes.
//
// Both sides of an entry use this same shape, so a reader can compare them
// field by field. The identifying fields are repeated on both sides on purpose:
// an entry that names the address only in the "after" half cannot answer "what
// happened to 192.0.2.5" for a release.
type leaseState struct {
	LeaseID    string      `json:"id"`
	ScopeID    string      `json:"scope_id"`
	IP         string      `json:"ip_address"`
	MAC        string      `json:"mac_address"`
	Hostname   string      `json:"hostname,omitempty"`
	ClientID   string      `json:"client_id,omitempty"`
	Status     LeaseStatus `json:"status"`
	LeaseEnd   string      `json:"lease_end"`
	Generation int64       `json:"generation"`
}

// renderLeaseState renders one side of an entry, or "" when there is no such
// side -- a binding that did not exist before, a state that does not exist
// after. auditlog.Append stores "" as NULL, so "there was nothing" is what a
// reader sees; the distinction from a blank value is not one this program can
// produce, and no reader treats a blank as a value.
func renderLeaseState(l *Lease) string {
	if l == nil {
		return ""
	}
	encoded, err := json.Marshal(leaseState{
		LeaseID:    l.ID,
		ScopeID:    l.ScopeID,
		IP:         l.IPAddress,
		MAC:        l.MACAddress,
		Hostname:   l.Hostname,
		ClientID:   l.ClientID,
		Status:     l.Status,
		LeaseEnd:   l.LeaseEnd,
		Generation: l.Generation,
	})
	if err != nil {
		// A lease holds strings and an int; this cannot fail for any value
		// stored today, and a missing detail is better than a missing entry.
		return ""
	}
	return string(encoded)
}

// The four transitions worth a trail, named here so the action constants stay
// in one file with the writer that uses them.
//
// There is deliberately no renewal among them. A renewal extends a binding
// without changing it: the address, the client and the state are all the same
// afterwards, and the only thing that moved is the row's own lease_end -- which
// is the row. An entry per renewal would be an entry per client per half-life,
// saying nothing.
//
// A status change made from the console is not recorded here either. That one
// has an actor, and the HTTP middleware already writes an entry naming them;
// duplicating it here would produce two rows for one decision.
func (m *Manager) auditBind(before, after *Lease) {
	m.auditTransition(auditlog.ActionLeaseBind, before, after)
}
func (m *Manager) auditRelease(before, after *Lease) {
	m.auditTransition(auditlog.ActionLeaseRelease, before, after)
}
func (m *Manager) auditDecline(before, after *Lease) {
	m.auditTransition(auditlog.ActionLeaseDecline, before, after)
}
func (m *Manager) auditExpire(before, after *Lease) {
	m.auditTransition(auditlog.ActionLeaseExpire, before, after)
}

// auditTransition records one lease state change.
//
// Neither of the two states may be nil-meaningful: a nil before is a lease that
// did not exist (a fresh binding), a nil after is one that does not exist any
// more. A transition where neither side changed is not a transition, and the
// callers do not call it for one -- a renewal extends a binding without
// changing it, and there is no action for it here.
//
// A failure is logged and dropped. The address has already been handed out, and
// refusing it now would tell the client to retry a binding that exists -- the
// same trade the DHCP-to-DNS linkage makes, for the same reason. What makes
// dropping it acceptable is that it is logged: an incomplete trail is something
// an operator can see, not something they discover by reading the table.
func (m *Manager) auditTransition(action string, before, after *Lease) {
	leaseID, scopeID := "", ""
	for _, side := range []*Lease{before, after} {
		if side != nil {
			leaseID, scopeID = side.ID, side.ScopeID
			break
		}
	}

	// The detail is the line an operator reads first; the two value columns are
	// what a diff is computed from. The address and the client are named here
	// rather than left to a JSON parse because "what happened to this address"
	// is the question the entry exists to answer.
	var detail string
	switch {
	case before == nil && after != nil:
		detail = fmt.Sprintf("DHCP lease %s created for %s (%s)", after.Status, after.IPAddress, after.MACAddress)
	case before != nil && after != nil:
		detail = fmt.Sprintf("DHCP lease %s -> %s for %s (%s)",
			before.Status, after.Status, after.IPAddress, after.MACAddress)
	case before != nil:
		detail = fmt.Sprintf("DHCP lease %s withdrawn for %s (%s)", before.Status, before.IPAddress, before.MACAddress)
	}

	if _, err := auditlog.Append(m.db, auditlog.Entry{
		// No operator and no HTTP request: the client is the only actor, and
		// the lease it acted on is the resource.
		UserID:       "system",
		Username:     "dhcp",
		Action:       action,
		ResourceType: auditlog.ResourceLease,
		ResourceID:   leaseID,
		Detail:       detail,
		OldValue:     renderLeaseState(before),
		NewValue:     renderLeaseState(after),
	}); err != nil {
		slog.Error("dhcp: could not write the audit entry for a lease change",
			"lease", leaseID, "scope", scopeID, "action", action, "error", err)
		// Also counted, because this is the failure with no other symptom. The
		// address was handed out, the client is working, and the trail simply
		// has a hole in it -- the one kind of missing data nobody notices by
		// using the system.
		metrics.RecordDBError()
	}
}
