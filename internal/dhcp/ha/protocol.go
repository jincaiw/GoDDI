package ha

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Protocol is the version of the peer wire format. It is carried in the
// handshake so that two nodes running different builds refuse each other
// instead of exchanging frames one of them reads as something else.
const Protocol = 1

// maxFrameBytes bounds one frame on the wire.
//
// The largest frame in this protocol is a snapshot. The capacity this release
// declares is 20,000 leases, and a lease row serialises to a few hundred bytes,
// so a full snapshot is a few megabytes. The bound is set several times above
// that so that a legitimate snapshot is never the thing that fails, while a
// peer that announces a gigabyte is refused before anything is allocated for
// it. A frame cap that can be raised from the other end of a socket is a
// memory-exhaustion handle.
const maxFrameBytes = 32 << 20 // 32 MiB

// FrameType names a message on the peer channel.
type FrameType string

const (
	// FrameHello opens a connection. It carries the sender's identity and its
	// watermarks, and the token on the side that dialled.
	FrameHello FrameType = "hello"
	// FrameSnapshot is the whole mirror, taken at one sequence. It is sent by
	// the primary on every new connection, which is what makes an incremental
	// catch-up path unnecessary.
	FrameSnapshot FrameType = "snapshot"
	// FrameOps is a batch of lease changes in sequence order.
	FrameOps FrameType = "ops"
	// FrameApplied is the standby's confirmation: everything up to Seq is
	// durably in its store.
	FrameApplied FrameType = "applied"
	// FramePing is the primary asking its peer to confirm. The reply is a
	// FrameApplied rather than a bare acknowledgement, so that a quiet pair
	// still refreshes the primary's view of how far along the mirror is.
	FramePing FrameType = "ping"
)

// Watermarks is what a node reports about how far along it is.
//
// AppliedSeq is "what I have durably". AckedSeq is "what I know my peer has
// durably". They are both facts, and they are the only facts that count: a
// node that has applied everything it was sent knows nothing about what it
// was not sent, and the difference between the two numbers is exactly the set
// of promises that may exist on one side and not the other.
type Watermarks struct {
	NodeID     string `json:"node_id"`
	Role       string `json:"role"`
	AppliedSeq int64  `json:"applied_seq"`
	AckedSeq   int64  `json:"acked_seq"`
	UptimeMS   int64  `json:"uptime_ms"`
}

// Frame is one message. The optional fields are not a union by accident: each
// frame type uses the subset it needs, and a field that is present on the wrong
// type is ignored rather than acted on.
type Frame struct {
	Type     FrameType `json:"type"`
	Protocol int       `json:"protocol,omitempty"`
	// Token is set on the dialling side's hello and nowhere else.
	Token string `json:"token,omitempty"`
	// Watermarks accompanies hello.
	Watermarks *Watermarks `json:"watermarks,omitempty"`
	// Seq is the sequence this frame brings the peer up to. A snapshot and an
	// ops batch both carry the highest sequence they cover; applied carries
	// the highest sequence applied.
	Seq    int64      `json:"seq,omitempty"`
	Leases []LeaseRow `json:"leases,omitempty"`
}

// LeaseRow is one lease as it travels between the nodes.
//
// It is a plain struct rather than a shared domain type for the same reason
// LeaseObserver's parameters are primitives: the peer channel must be able to
// carry what a lease is without either side importing the other's model, so
// that a change to a lease's Go representation is not silently a change to a
// wire format.
type LeaseRow struct {
	ID         string `json:"id"`
	ScopeID    string `json:"scope_id"`
	IPAddress  string `json:"ip_address"`
	MACAddress string `json:"mac_address"`
	Hostname   string `json:"hostname"`
	ClientID   string `json:"client_id"`
	LeaseStart string `json:"lease_start"`
	LeaseEnd   string `json:"lease_end"`
	Status     string `json:"status"`
	LastSeen   string `json:"last_seen"`
	Generation int64  `json:"generation"`
}

// leaseColumns is the column order every statement in this package uses. It is
// one list rather than one per statement because a mismatch between an INSERT's
// column list and its VALUES is invisible to the compiler and to SQLite.
var leaseColumns = []string{
	"id", "scope_id", "ip_address", "mac_address", "hostname", "client_id",
	"lease_start", "lease_end", "status", "last_seen", "generation",
}

func (l LeaseRow) values() []any {
	return []any{
		l.ID, l.ScopeID, l.IPAddress, l.MACAddress, l.Hostname, l.ClientID,
		l.LeaseStart, l.LeaseEnd, l.Status, l.LastSeen, l.Generation,
	}
}

// Protocol errors. They are distinct values rather than formatted strings so
// that the caller can decide whether to retry, and so that a test can assert
// on which one happened.
var (
	// ErrAuthFailed means the peer did not present the shared token.
	ErrAuthFailed = errors.New("ha: peer failed authentication")
	// ErrProtocolMismatch means the peer speaks a different wire version.
	ErrProtocolMismatch = errors.New("ha: peer speaks a different protocol version")
	// ErrRoleMismatch means the peer is not the role this node expects. Two
	// primaries must never exchange lease facts: each would conclude the other
	// is a mirror and neither would stop serving.
	ErrRoleMismatch = errors.New("ha: peer is not the role this node expects")
	// ErrFrameTooLarge means the peer announced a frame beyond the cap.
	ErrFrameTooLarge = errors.New("ha: peer announced a frame larger than the limit")
)

func writeFrame(w io.Writer, f Frame) error {
	body, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("ha: encoding a %s frame: %w", f.Type, err)
	}
	if len(body) > maxFrameBytes {
		return fmt.Errorf("ha: a %s frame of %d bytes exceeds the %d byte limit",
			f.Type, len(body), maxFrameBytes)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("ha: writing a %s frame header: %w", f.Type, err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("ha: writing a %s frame: %w", f.Type, err)
	}
	return nil
}

func readFrame(r io.Reader, limit int) (Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if limit <= 0 || n > uint32(limit) {
		return Frame{}, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return Frame{}, fmt.Errorf("ha: reading a frame body: %w", err)
	}
	var f Frame
	if err := json.Unmarshal(body, &f); err != nil {
		return Frame{}, fmt.Errorf("ha: decoding a frame: %w", err)
	}
	return f, nil
}
