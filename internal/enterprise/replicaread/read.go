// Package replicaread reserves a future replication-read boundary. It does
// not provide a production read service or confer any role or authority.
package replicaread

import (
	"context"
	"errors"
)

var ErrNoAuthority = errors.New("replicaread: no authorized replication read service")

// Read remains closed until independent identity, role, fencing and other
// production gates exist. No caller-supplied proof can open this boundary.
func Read(context.Context) ([]byte, error) { return nil, ErrNoAuthority }
