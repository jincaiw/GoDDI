// Package replicarpc does not provide an authorized replication service.
// Production transport, role and fencing gates have not been implemented.
package replicarpc

import "errors"

var ErrNoAuthority = errors.New("replicarpc: no authorized replication service")

// Start always fails before opening a listener or reading a database.
func Start() error { return ErrNoAuthority }
