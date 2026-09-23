package dataplane

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"
)

// ResolveIPAMSpaceByIP looks up the most-specific locally replicated IPAM
// subnet. DHCP uses this against its own store so a control-plane outage cannot
// enter the ACK path just to resolve a fact's space identity.
func (s *Store) ResolveIPAMSpaceByIP(ctx context.Context, ip string) (string, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return "", fmt.Errorf("dataplane: invalid IP address %q", ip)
	}
	rows, err := s.QueryContext(ctx, `SELECT space_id, cidr FROM ipam_subnets`)
	if err != nil {
		return "", fmt.Errorf("dataplane: read local IPAM subnet mapping: %w", err)
	}
	type match struct {
		space string
		bits  int
	}
	var best *match
	for rows.Next() {
		var spaceID, cidr string
		if err := rows.Scan(&spaceID, &cidr); err != nil {
			rows.Close()
			return "", fmt.Errorf("dataplane: scan local IPAM subnet mapping: %w", err)
		}
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil || !network.Contains(parsed) {
			continue
		}
		bits, _ := network.Mask.Size()
		if best == nil || bits > best.bits {
			best = &match{space: spaceID, bits: bits}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", fmt.Errorf("dataplane: iterate local IPAM subnet mapping: %w", err)
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	if best == nil || strings.TrimSpace(best.space) == "" {
		return "", sql.ErrNoRows
	}
	return best.space, nil
}
