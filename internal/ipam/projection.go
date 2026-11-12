package ipam

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrProjectionMissing reports that a DHCP scope's subnet has no matching row
// in the DHCP plane's IPAM subnet projection. On the DHCP data plane this means
// the IPAM projection is incomplete/stale for that scope; allocating from it
// could hand out an address the control plane no longer agrees with.
var ErrProjectionMissing = errors.New("ipam: scope subnet has no IPAM projection in the DHCP plane")

// ProjectionGap is a scope whose subnet is missing from the local ipam_subnets
// projection.
type ProjectionGap struct {
	ScopeID   string
	ScopeName string
	Subnet    string
}

// ProjectionGaps lists enabled DHCP scopes whose subnet has no projected
// ipam_subnets row. It is a non-blocking consistency signal: an operator can
// alert on a non-empty list without the detector itself changing allocation.
func ProjectionGaps(ctx context.Context, db *sql.DB) ([]ProjectionGap, error) {
	if db == nil {
		return nil, errors.New("ipam: nil database for projection gaps")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.name, s.subnet
		FROM dhcp_scopes s
		LEFT JOIN ipam_subnets i ON i.cidr = s.subnet
		WHERE s.enabled = 1 AND i.id IS NULL
		ORDER BY s.id`)
	if err != nil {
		return nil, fmt.Errorf("ipam: query projection gaps: %w", err)
	}
	defer rows.Close()
	var out []ProjectionGap
	for rows.Next() {
		var g ProjectionGap
		if err := rows.Scan(&g.ScopeID, &g.ScopeName, &g.Subnet); err != nil {
			return nil, fmt.Errorf("ipam: scan projection gap: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// NewProjectionGuard returns a fail-closed check for the DHCP allocation path:
// it refuses a scope whose subnet has no local projection. It only has an
// effect when an operator opts in (dhcp.require_ipam_projection); by default the
// detector above is the signal and allocation is unchanged.
func NewProjectionGuard(db *sql.DB) func(scopeID, subnet string) error {
	return func(scopeID, subnet string) error {
		if db == nil {
			return errors.New("ipam: nil database for projection guard")
		}
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ipam_subnets WHERE cidr = ?`, subnet).Scan(&n); err != nil {
			return fmt.Errorf("ipam: projection guard query: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: scope=%s subnet=%s", ErrProjectionMissing, scopeID, subnet)
		}
		return nil
	}
}
