// Package cluster provides multi-node cluster coordination and
// synchronization primitives for GoDDI. The current release ships a
// single-node default; the multi-node coordinator is reserved for the
// upcoming V0.02 release.
package cluster

import "errors"

// ErrNotImplemented is returned by cluster APIs that are reserved for the
// upcoming V0.02 multi-node release. Handlers should translate this error
// into a 501 Not Implemented response.
var ErrNotImplemented = errors.New("cluster: multi-node coordination is not implemented in this release")
