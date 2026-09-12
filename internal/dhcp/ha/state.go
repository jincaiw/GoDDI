// Package ha implements the DHCP high-availability contract of ADR 0003: a
// single-writer pair in which the primary is the only author of lease facts and
// the standby is a pure mirror.
//
// The package exists because of one sentence in that contract: a client must
// not be told it holds an address until a second machine has it durably too.
// Everything here serves that sentence --
//
//   - the peer channel carries lease facts directly between the two nodes,
//     never through the control database, so a management-plane outage cannot
//     withhold an address (INV-3);
//   - the watermarks are sequence numbers rather than heartbeats, because a
//     two-node cluster cannot tell "the peer is down" from "I can only see my
//     half of the network", and only an operator can (INV-4);
//   - losing the second copy stops the node from making new promises rather
//     than from answering at all (INV-6).
//
// The three load-bearing properties, stated so that a later change can be
// measured against them:
//
//  1. A binding is acknowledged only after the standby reports it has applied
//     the sequence that carries it.
//  2. The standby's content is always a subset of what the primary has held.
//     It applies the primary's rows in the primary's order and never authors
//     one itself, which is what makes "ask the standby for what I lost" safe
//     and the reverse unsafe.
//  3. Nothing in this package promotes a node, or approves running without a
//     second copy, on its own. Both are operator actions.
package ha

// State is what a node believes it is, and the thing an operator is shown.
//
// It is deliberately not a boolean. "Do I have a peer?" has one bit, and every
// failure of a two-node design comes from collapsing the remaining distinctions
// into that bit: a node that cannot see its peer is not the same as a node that
// has been told to carry on alone, and a node that has been fenced is not the
// same as a node that is merely paused.
type State string

const (
	// StateSolo is a node with HA switched off. It is what every installation
	// was before this contract existed, and it is not an error state.
	StateSolo State = "solo"

	// StatePrimary is a node that owns the lease facts and whose peer is
	// current. New bindings and renewals are acknowledged.
	StatePrimary State = "primary"

	// StatePaused is a primary that has lost its second copy and has not been
	// given permission to carry on. It answers nothing that creates or extends
	// a promise. This is the default state of a primary without a peer, not an
	// error state: it is what "no second copy, so no new promise" looks like.
	StatePaused State = "paused"

	// StatePrimaryDegraded is a primary that lost its second copy and was
	// explicitly degraded by an operator. It serves, and it says so
	// everywhere, because the reason it is still serving is a decision
	// somebody made rather than a fact the system observed.
	StatePrimaryDegraded State = "primary-degraded"

	// StateStandby is the mirror. It never serves clients, and it never
	// authors a lease.
	StateStandby State = "standby"

	// StateFenced is a node that does not know whether it is still allowed to
	// serve. It is more conservative than paused: it answers nothing at all,
	// not even the requests it could refuse anyway.
	StateFenced State = "fenced"
)

// ServesClients reports whether the node's DHCP listener should be answering.
//
// paused is included on purpose. A paused node is still reachable and still
// runs the request path; what it withholds are the replies that would create or
// extend a promise, and it decides that per request rather than by refusing to
// listen. Going dark entirely would turn a temporary loss of the second copy
// into every client losing sight of its server.
func (s State) ServesClients() bool {
	switch s {
	case StateSolo, StatePrimary, StatePaused, StatePrimaryDegraded:
		return true
	default:
		return false
	}
}

// MayBind reports whether the node may promise an address or an extension of
// an existing one.
//
// This is the predicate the request path consults, and it is the only place
// that answers it. A paused node may hand out offers -- a reservation is not a
// promise, and it expires -- but it may not answer a REQUEST.
func (s State) MayBind() bool {
	switch s {
	case StateSolo, StatePrimary, StatePrimaryDegraded:
		return true
	default:
		return false
	}
}

// Redundant reports whether the node currently has a second copy of what it
// promises. It is the fact the operator cares about, and it is reported
// separately from the state because "serving" and "redundant" are different
// questions.
func (s State) Redundant() bool {
	return s == StatePrimary || s == StateStandby
}

// String makes the state safe to put in a log line, a probe reason and an
// alert label without a caller remembering to convert it.
func (s State) String() string { return string(s) }
