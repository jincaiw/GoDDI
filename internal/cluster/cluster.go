// Package cluster provides multi-node cluster coordination and
// synchronization primitives for GoDDI. The current release ships a
// single-node default; the production multi-node coordinator remains reserved
// until the protocol model is backed by real transport, durable state, fencing,
// and cross-host partition evidence.
package cluster

import "errors"

// ErrNotImplemented is returned by cluster APIs that are reserved for the
// future production multi-node release. The protocol model in this package is
// a testable contract only; handlers must still translate this error into a
// 501 Not Implemented response.
var ErrNotImplemented = errors.New("cluster: multi-node coordination is not implemented in this release")
