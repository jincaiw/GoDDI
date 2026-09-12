package ha

import (
	"context"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// Adapter presents the primary's replicator to the DHCP server in the server
// package's own terms.
//
// The two packages do not import each other. The server declares what it needs
// -- "tell me whether I may promise, and make this binding durable elsewhere"
// -- and this adapter is the one place that knows both vocabularies. That is
// what lets the DHCP server be tested with no HA at all, and lets the HA
// machinery be tested without a DHCP server.
type Adapter struct {
	r *Replicator
}

// Adapt wraps a replicator for the DHCP server.
func Adapt(r *Replicator) *Adapter { return &Adapter{r: r} }

// MayBind reports whether a new binding or a renewal may be promised.
func (a *Adapter) MayBind() bool { return a.r.MayBind() }

// Confirm records the binding for the mirror and waits for it to be durable
// there.
func (a *Adapter) Confirm(ctx context.Context, l *lease.Lease) error {
	return a.r.Confirm(ctx, RowOf(l))
}

// Replicate records a lease change without waiting for the mirror. It is used
// for the changes that are not promises.
func (a *Adapter) Replicate(l *lease.Lease) {
	a.r.Replicate(RowOf(l))
}

// RowOf converts a lease into the row that travels between the nodes.
//
// It is a named function rather than a method on the wire type so that the
// conversion appears once: a second, slightly different copy of this mapping
// is how a field stops being replicated without anybody noticing.
func RowOf(l *lease.Lease) LeaseRow {
	if l == nil {
		return LeaseRow{}
	}
	return LeaseRow{
		ID:         l.ID,
		ScopeID:    l.ScopeID,
		IPAddress:  l.IPAddress,
		MACAddress: l.MACAddress,
		Hostname:   l.Hostname,
		ClientID:   l.ClientID,
		LeaseStart: l.LeaseStart,
		LeaseEnd:   l.LeaseEnd,
		Status:     string(l.Status),
		LastSeen:   l.LastSeen,
		Generation: l.Generation,
	}
}
