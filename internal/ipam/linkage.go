package ipam

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/ipam/address"
)

// Linkage handles IPAM integration with DHCP and DNS.
type Linkage struct {
	db      *sql.DB
	addrMgr *address.Manager
}

// NewLinkage creates a new IPAM linkage manager.
func NewLinkage(db *sql.DB) *Linkage {
	return &Linkage{
		db:      db,
		addrMgr: address.NewManager(db),
	}
}

// SyncFromDHCPLease updates IP status from a DHCP lease.
func (l *Linkage) SyncFromDHCPLease(lz *lease.Lease) error {
	// Find the IPAM address by IP.
	// We need to find the subnet that contains this IP first.
	var subnetID string
	err := l.db.QueryRow(`
		SELECT s.id FROM ipam_subnets s
		JOIN ipam_addresses a ON a.subnet_id = s.id
		WHERE a.ip_address = ?`, lz.IPAddress).Scan(&subnetID)
	if err != nil {
		// No matching IPAM address found - not an error, just not tracked.
		return nil
	}

	addr, err := l.addrMgr.GetAddressByIP(subnetID, lz.IPAddress)
	if err != nil || addr == nil {
		return nil
	}

	// Update the address status based on lease.
	status := address.StatusDHCP
	opts := address.AddressOptions{
		Status:     &status,
		MACAddress: lz.MACAddress,
		Hostname:   lz.Hostname,
	}

	_, err = l.addrMgr.UpdateAddress(addr.ID, opts)
	if err != nil {
		return fmt.Errorf("failed to sync DHCP lease to IPAM: %w", err)
	}

	slog.Debug("IPAM: synced DHCP lease", "ip", lz.IPAddress, "mac", lz.MACAddress)
	return nil
}

// SyncFromDNSRecord links an IP to a DNS record.
func (l *Linkage) SyncFromDNSRecord(recordID, ip, hostname string) error {
	// Find the IPAM address by IP.
	var subnetID string
	err := l.db.QueryRow(`
		SELECT s.id FROM ipam_subnets s
		JOIN ipam_addresses a ON a.subnet_id = s.id
		WHERE a.ip_address = ?`, ip).Scan(&subnetID)
	if err != nil {
		return nil
	}

	addr, err := l.addrMgr.GetAddressByIP(subnetID, ip)
	if err != nil || addr == nil {
		return nil
	}

	// Update DNS record link.
	_, err = l.db.Exec(`
		UPDATE ipam_addresses SET dns_record_id=?, hostname=?, updated_at=datetime('now')
		WHERE id=?`, recordID, hostname, addr.ID)
	if err != nil {
		return fmt.Errorf("failed to link DNS record to IPAM: %w", err)
	}

	slog.Debug("IPAM: linked DNS record", "ip", ip, "hostname", hostname)
	return nil
}

// CheckDependencies checks DHCP scopes and DNS zones that depend on a subnet.
func (l *Linkage) CheckDependencies(subnetID string) ([]string, error) {
	var deps []string

	// Check DHCP scopes.
	var cidr string
	err := l.db.QueryRow("SELECT cidr FROM ipam_subnets WHERE id = ?", subnetID).Scan(&cidr)
	if err != nil {
		return nil, fmt.Errorf("subnet not found")
	}

	rows, err := l.db.Query("SELECT id, name FROM dhcp_scopes WHERE subnet = ?", cidr)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				deps = append(deps, fmt.Sprintf("DHCP scope: %s (%s)", name, id))
			}
		}
		if rows.Err() != nil {
			slog.Warn("ipam: failed to iterate DHCP scopes for dependencies", "error", rows.Err())
		}
	}

	// Check DNS zones (reverse zones based on CIDR).
	reverseZoneName := computeReverseZoneName(cidr)
	if reverseZoneName != "" {
		dnsRows, err := l.db.Query("SELECT id, name FROM dns_zones WHERE name = ?", reverseZoneName)
		if err == nil {
			defer dnsRows.Close()
			for dnsRows.Next() {
				var id, name string
				if dnsRows.Scan(&id, &name) == nil {
					deps = append(deps, fmt.Sprintf("DNS zone: %s (%s)", name, id))
				}
			}
		}
	}

	return deps, nil
}

// computeReverseZoneName computes the reverse zone name for a CIDR.
// e.g., "192.168.1.0/24" -> "1.168.192.in-addr.arpa"
func computeReverseZoneName(cidr string) string {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}
	ones, _ := ipNet.Mask.Size()
	ip := ipNet.IP.To4()
	if ip == nil {
		return ""
	}

	switch {
	case ones >= 24:
		return fmt.Sprintf("%d.%d.%d.in-addr.arpa", ip[2], ip[1], ip[0])
	case ones >= 16:
		return fmt.Sprintf("%d.%d.in-addr.arpa", ip[1], ip[0])
	case ones >= 8:
		return fmt.Sprintf("%d.in-addr.arpa", ip[0])
	default:
		return "in-addr.arpa"
	}
}
