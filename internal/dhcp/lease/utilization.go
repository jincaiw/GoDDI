package lease

import (
	"encoding/binary"
	"fmt"
	"net"
)

// ScopeUsage is one DHCP scope's address utilisation.
//
// Held is what the pool cannot hand out: active bindings, outstanding offers
// and conflict quarantines. It is counted through heldStatuses so this number
// and the allocator cannot disagree about what "in use" means -- an address
// that the allocator refuses but this figure ignores would be a pool that
// reports capacity it does not have.
type ScopeUsage struct {
	// Scope is the scope identifier. It is the metric label because names are
	// neither unique nor stable, and a renamed scope must not become a second
	// time series.
	Scope string
	// Held is the number of addresses held in the pool.
	Held int
	// Pool is the number of addresses the pool spans.
	Pool int
	// Ratio is Held over Pool. Zero for a pool with no addresses.
	Ratio float64
}

// ScopeUtilization reports every enabled scope's utilisation in one pass.
//
// Disabled scopes are left out. They allocate nothing, so a ratio for one is
// not a capacity signal -- an alert written against it would fire on a scope
// that was deliberately switched off.
//
// The count is a single grouped statement rather than one query per scope: a
// tick that runs every ten seconds on a management database with one connection
// must not become an N+1 on the request path.
func (m *Manager) ScopeUtilization() ([]ScopeUsage, error) {
	placeholders, args := heldStatusPlaceholders()

	query := `
		SELECT s.id, s.start_ip, s.end_ip,
		       (SELECT COUNT(*) FROM dhcp_leases l
		         WHERE l.scope_id = s.id AND l.status IN (` + placeholders + `)) AS held
		FROM dhcp_scopes s
		WHERE s.enabled = 1
		ORDER BY s.id`

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading scope utilisation: %w", err)
	}
	defer rows.Close()

	var out []ScopeUsage
	for rows.Next() {
		var id, startIP, endIP string
		var held int
		if err := rows.Scan(&id, &startIP, &endIP, &held); err != nil {
			return nil, fmt.Errorf("scanning scope utilisation: %w", err)
		}
		pool := poolSize(startIP, endIP)
		usage := ScopeUsage{Scope: id, Held: held, Pool: pool}
		if pool > 0 {
			usage.Ratio = float64(held) / float64(pool)
		}
		out = append(out, usage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading scope utilisation: %w", err)
	}
	return out, nil
}

// poolSize counts the addresses from start to end inclusive.
//
// A range that does not parse, is not IPv4, or runs backwards yields zero
// rather than a guess. Zero makes the ratio zero, which understates a full
// pool; a made-up denominator would overstate an empty one, and the second
// mistake is the one that silences the alert.
func poolSize(startIP, endIP string) int {
	start := net.ParseIP(startIP)
	end := net.ParseIP(endIP)
	if start == nil || end == nil {
		return 0
	}
	start4, end4 := start.To4(), end.To4()
	if start4 == nil || end4 == nil {
		return 0
	}
	first := binary.BigEndian.Uint32(start4)
	last := binary.BigEndian.Uint32(end4)
	if last < first {
		return 0
	}
	// Counted in 64 bits so the whole address space does not wrap to zero.
	return int(uint64(last) - uint64(first) + 1)
}
