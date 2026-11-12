// Package eventlogguard reserves a future eventlog reference migration boundary.
// It grants no role and performs no migration in application builds.
package eventlogguard

import (
	"context"
	"database/sql"
	"errors"
)

var ErrNoAuthority = errors.New("eventlogguard: no migration authority")

// Migrate remains closed, including for nil arguments and canceled contexts.
func Migrate(context.Context, *sql.DB) error { return ErrNoAuthority }
