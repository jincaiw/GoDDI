// Package snapshotstage reserves a future snapshot installation boundary.
// The only candidate staging implementation lives in tests and cannot be
// invoked by production code. Neither entry point grants authority.
package snapshotstage

import (
	"context"
	"errors"
)

var ErrNoAuthority = errors.New("snapshotstage: no snapshot installation authority")

func Import(context.Context) error  { return ErrNoAuthority }
func Install(context.Context) error { return ErrNoAuthority }
