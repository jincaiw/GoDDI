// Package sdk provides the Go SDK for building third-party GoDDI
// extensions ("apps"). The current release does not ship any built-in
// apps; the SDK is reserved for the upcoming V0.02 app marketplace.
package sdk

import "errors"

// ErrNotImplemented is returned by SDK APIs that are reserved for the
// upcoming V0.02 app marketplace. Handlers should translate this error
// into a 501 Not Implemented response.
var ErrNotImplemented = errors.New("sdk: app SDK is not implemented in this release")
