// Package runtime provides the sandboxed execution environment for
// third-party GoDDI extensions ("apps"). The current release does not
// ship any built-in apps; the runtime is reserved for the upcoming
// V0.02 app marketplace.
package runtime

import "errors"

// ErrNotImplemented is returned by runtime APIs that are reserved for the
// upcoming V0.02 app marketplace. Handlers should translate this error
// into a 501 Not Implemented response.
var ErrNotImplemented = errors.New("runtime: app runtime is not implemented in this release")
