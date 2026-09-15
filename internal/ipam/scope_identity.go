package ipam

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
)

// ScopeIdentity is the producer-side identity needed by a unified DHCP fact.
// The lookup is intentionally separate from FactsMutationWriter: the writer
// must receive a resolved SpaceID before starting the lease-store transaction.
type ScopeIdentity struct {
	ScopeID  string
	SpaceID  string
	SubnetID string
	CIDR     string
}

// ScopeIdentityResolver resolves a DHCP scope and address to one stable IPAM
// space identity. Callers must resolve it before opening the lease mutation
// transaction because the IPAM database may be a different SQLite file.
type ScopeIdentityResolver struct {
	db *sql.DB
}

func NewScopeIdentityResolver(db *sql.DB) (*ScopeIdentityResolver, error) {
	if db == nil {
		return nil, fmt.Errorf("ipam scope identity: nil database")
	}
	return &ScopeIdentityResolver{db: db}, nil
}

func (r *ScopeIdentityResolver) Resolve(scopeID, ip string) (ScopeIdentity, error) {
	if r == nil || r.db == nil {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: nil resolver")
	}
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: scope ID is required")
	}
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: invalid IP %q", ip)
	}

	var scopeCIDR string
	if err := r.db.QueryRow(`SELECT subnet FROM dhcp_scopes WHERE id = ?`, scopeID).Scan(&scopeCIDR); err != nil {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: load scope %q: %w", scopeID, err)
	}
	if canonical, _, err := net.ParseCIDR(strings.TrimSpace(scopeCIDR)); err == nil {
		identity, err := r.resolveExact(scopeID, canonical.String())
		if err != nil {
			return ScopeIdentity{}, err
		}
		if identity.SpaceID != "" {
			return identity, nil
		}
	}

	return r.resolveContaining(scopeID, parsed)
}

func (r *ScopeIdentityResolver) resolveExact(scopeID, cidr string) (ScopeIdentity, error) {
	var identity ScopeIdentity
	err := r.db.QueryRow(`
		SELECT id, space_id, cidr FROM ipam_subnets WHERE cidr = ?`, cidr).
		Scan(&identity.SubnetID, &identity.SpaceID, &identity.CIDR)
	if err == sql.ErrNoRows {
		return ScopeIdentity{ScopeID: scopeID}, nil
	}
	if err != nil {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: load exact subnet %q: %w", cidr, err)
	}
	identity.ScopeID = scopeID
	return identity, nil
}

func (r *ScopeIdentityResolver) resolveContaining(scopeID string, ip net.IP) (ScopeIdentity, error) {
	rows, err := r.db.Query(`SELECT id, space_id, cidr FROM ipam_subnets`)
	if err != nil {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: list subnets: %w", err)
	}
	defer rows.Close()

	var best ScopeIdentity
	bestPrefix := -1
	for rows.Next() {
		var candidate ScopeIdentity
		if err := rows.Scan(&candidate.SubnetID, &candidate.SpaceID, &candidate.CIDR); err != nil {
			return ScopeIdentity{}, fmt.Errorf("ipam scope identity: scan subnet: %w", err)
		}
		_, network, err := net.ParseCIDR(strings.TrimSpace(candidate.CIDR))
		if err != nil || !network.Contains(ip) {
			continue
		}
		prefix, _ := network.Mask.Size()
		if prefix > bestPrefix {
			candidate.ScopeID = scopeID
			best = candidate
			bestPrefix = prefix
		}
	}
	if err := rows.Err(); err != nil {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: iterate subnets: %w", err)
	}
	if bestPrefix < 0 {
		return ScopeIdentity{}, fmt.Errorf("ipam scope identity: no subnet contains scope %q address %s", scopeID, ip)
	}
	return best, nil
}
