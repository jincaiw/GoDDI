package configver

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jasonwa/goddi/internal/dhcp/scope"
)

// DHCPScopeContent is a complete snapshot of a DHCP scope's publishable state.
//
// It is a snapshot, not a patch: every field is present, including zero values.
// Storing a patch would make a revision impossible to read as "this is what the
// scope was", and a diff between two patches would report fields that merely
// happened to be omitted.
//
// Timestamps are excluded. They change on every write, so including them would
// make every publish look like it changed something and would drown the fields
// an operator actually needs to compare.
type DHCPScopeContent struct {
	Name             string `json:"name"`
	Interface        string `json:"interface"`
	Subnet           string `json:"subnet"`
	StartIP          string `json:"start_ip"`
	EndIP            string `json:"end_ip"`
	SubnetMask       string `json:"subnet_mask"`
	Router           string `json:"router"`
	DNSServers       string `json:"dns_servers"`
	NTPServers       string `json:"ntp_servers"`
	DomainName       string `json:"domain_name"`
	LeaseTime        int    `json:"lease_time"`
	MaxLeaseTime     int    `json:"max_lease_time"`
	Enabled          bool   `json:"enabled"`
	PingCheckEnabled bool   `json:"ping_check_enabled"`
	DNSUpdates       bool   `json:"dns_updates"`
	Comment          string `json:"comment"`
}

// DHCPScopeAdapter publishes DHCP scopes.
type DHCPScopeAdapter struct{}

// NewDHCPScopeAdapter returns an adapter for DHCP scope configuration.
func NewDHCPScopeAdapter() *DHCPScopeAdapter { return &DHCPScopeAdapter{} }

func (*DHCPScopeAdapter) Type() ResourceType { return ResourceDHCPScope }

// Validate reuses the scope package's own rules rather than restating them.
//
// A publish that accepts a configuration the storage layer would reject is
// worse than a publish that rejects it: the revision is recorded as applied
// while the resource never actually changed.
func (*DHCPScopeAdapter) Validate(content json.RawMessage) error {
	var c DHCPScopeContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode scope content: %w", err)
	}

	// A snapshot must be complete. ValidateScopeOptions deliberately passes an
	// all-empty options value (it cannot tell "update nothing" from "empty"),
	// so the required fields are checked here.
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Subnet == "" {
		return fmt.Errorf("subnet is required")
	}
	if c.StartIP == "" || c.EndIP == "" {
		return fmt.Errorf("start_ip and end_ip are required")
	}

	if err := scope.ValidateScopeOptions(scope.ScopeOptions{
		Name:         c.Name,
		Subnet:       c.Subnet,
		StartIP:      c.StartIP,
		EndIP:        c.EndIP,
		LeaseTime:    &c.LeaseTime,
		MaxLeaseTime: &c.MaxLeaseTime,
	}); err != nil {
		return err
	}
	return nil
}

func (*DHCPScopeAdapter) Exists(q Queryer, id string) (bool, error) {
	var one int
	err := q.QueryRow(`SELECT 1 FROM dhcp_scopes WHERE id = ?`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (*DHCPScopeAdapter) Apply(tx *sql.Tx, id string, content json.RawMessage) error {
	var c DHCPScopeContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode scope content: %w", err)
	}

	// An empty string is stored as NULL for the optional columns, matching what
	// the scope manager writes. Storing "" instead would make the column
	// inconsistently NULL-or-empty depending on which path last wrote it, and
	// the read path (sql.NullString) would then report "" either way -- a
	// difference invisible until something starts filtering on IS NULL.
	res, err := tx.Exec(`UPDATE dhcp_scopes SET
			name = ?, interface = ?, subnet = ?, start_ip = ?, end_ip = ?,
			subnet_mask = ?, router = ?, dns_servers = ?, ntp_servers = ?,
			domain_name = ?, lease_time = ?, max_lease_time = ?,
			enabled = ?, ping_check_enabled = ?, dns_updates = ?, comment = ?,
			updated_at = datetime('now')
		WHERE id = ?`,
		c.Name, nullIfEmpty(c.Interface), c.Subnet, c.StartIP, c.EndIP,
		nullIfEmpty(c.SubnetMask), nullIfEmpty(c.Router), nullIfEmpty(c.DNSServers),
		nullIfEmpty(c.NTPServers), nullIfEmpty(c.DomainName),
		c.LeaseTime, nullIfZero(c.MaxLeaseTime),
		c.Enabled, c.PingCheckEnabled, c.DNSUpdates, nullIfEmpty(c.Comment), id)
	if err != nil {
		return fmt.Errorf("update scope: %w", err)
	}

	// The row must exist: Exists() ran before the revision was written, and a
	// zero-row update here would mean the scope was deleted in between, in
	// which case the revision must not be reported as applied.
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return fmt.Errorf("scope %s disappeared before the release was applied", id)
	}
	return nil
}

// Notify is a no-op: the DHCP server reads scopes from the database on every
// request (scope.Manager.FindScopeByIP), so a committed write is already
// visible to the data plane. There is no cache to invalidate.
//
// If a scope cache is ever added, this must call into it. The release queue
// exists precisely so that such a change is a one-line edit here instead of a
// silently stale data plane.
func (*DHCPScopeAdapter) Notify(string) error { return nil }

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
