package configver

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

// IPAMSubnetContent is a complete snapshot of an IPAM subnet's publishable
// state.
//
// space_id is excluded: moving a subnet between spaces is a re-parenting
// operation, not a field edit. It changes which addresses and reservations the
// subnet is related to, so it needs its own dependency checks rather than
// riding along with a configuration publish.
type IPAMSubnetContent struct {
	Name        string `json:"name"`
	CIDR        string `json:"cidr"`
	VLANID      int    `json:"vlan_id"`
	Location    string `json:"location"`
	Description string `json:"description"`
}

// IPAMSubnetAdapter publishes IPAM subnet metadata.
type IPAMSubnetAdapter struct{}

// NewIPAMSubnetAdapter returns an adapter for IPAM subnet configuration.
func NewIPAMSubnetAdapter() *IPAMSubnetAdapter { return &IPAMSubnetAdapter{} }

func (*IPAMSubnetAdapter) Type() ResourceType { return ResourceIPAMSubnet }

func (*IPAMSubnetAdapter) Validate(content json.RawMessage) error {
	var c IPAMSubnetContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode subnet content: %w", err)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(c.CIDR) == "" {
		return fmt.Errorf("cidr is required")
	}
	if _, _, err := net.ParseCIDR(c.CIDR); err != nil {
		return fmt.Errorf("cidr %q is not a valid network: %w", c.CIDR, err)
	}
	// 802.1Q VLAN IDs are 12 bits; 0 and 4095 are reserved. Accepting an
	// out-of-range value would store a VLAN nothing on the wire can use.
	if c.VLANID < 0 || c.VLANID > 4094 {
		return fmt.Errorf("vlan_id must be between 0 and 4094, got %d", c.VLANID)
	}
	return nil
}

func (*IPAMSubnetAdapter) Exists(q Queryer, id string) (bool, error) {
	var one int
	err := q.QueryRow(`SELECT 1 FROM ipam_subnets WHERE id = ?`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (*IPAMSubnetAdapter) Apply(tx *sql.Tx, id string, content json.RawMessage) error {
	var c IPAMSubnetContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode subnet content: %w", err)
	}
	res, err := tx.Exec(`UPDATE ipam_subnets SET
			name = ?, cidr = ?, vlan_id = ?, location = ?, description = ?,
			updated_at = datetime('now')
		WHERE id = ?`,
		c.Name, c.CIDR, nullIfZero(c.VLANID), nullIfEmpty(c.Location),
		nullIfEmpty(c.Description), id)
	if err != nil {
		return fmt.Errorf("update subnet: %w", err)
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return fmt.Errorf("subnet %s disappeared before the release was applied", id)
	}
	return nil
}

// Notify is a no-op: IPAM is read directly from the database, so a committed
// write is already visible.
func (*IPAMSubnetAdapter) Notify(string) error { return nil }
