package dhcp

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// DNSLink handles DHCP to DNS integration.
type DNSLink struct {
	db *sql.DB
}

// NewDNSLink creates a new DNS link manager.
func NewDNSLink(db *sql.DB) *DNSLink {
	return &DNSLink{db: db}
}

// UpdateDNSRecord creates or deletes DNS records for a DHCP lease.
// action: "create" or "delete"
// This method is designed to be called asynchronously (don't block DHCP response).
func (dl *DNSLink) UpdateDNSRecord(l *lease.Lease, action string) error {
	if l.Hostname == "" || l.IPAddress == "" {
		return nil
	}

	// Sanitize hostname: ensure it is a valid DNS name, strip any control
	// characters or backticks that could be used for SQL injection in
	// downstream queries, and limit length to 253 characters per RFC 1035.
	cleaned := sanitizeHostname(l.Hostname)
	if cleaned == "" {
		return nil
	}

	// Find the matching forward zone for the hostname.
	zoneID, zoneName, err := dl.findForwardZone(cleaned)
	if err != nil || zoneID == "" {
		slog.Debug("dhcp_dns_link: no matching forward zone found", "hostname", cleaned)
		return nil
	}

	// Extract the relative name within the zone. We use a strict
	// boundary check: the relative name is the part of the FQDN strictly
	// outside the zone suffix.
	relativeName, ok := relativeNameInZone(cleaned, zoneName)
	if !ok {
		slog.Debug("dhcp_dns_link: hostname does not match zone", "hostname", cleaned, "zone", zoneName)
		return nil
	}

	switch action {
	case "create":
		// Create A record with owner = "dhcp".
		// Only INSERT OR REPLACE if no matching record exists, or the existing record is DHCP-owned.
		_, err := dl.db.Exec(`
			INSERT OR REPLACE INTO dns_records (id, zone_id, name, type, ttl, value, enabled, owner, created_at, updated_at)
			VALUES (
				COALESCE((SELECT id FROM dns_records WHERE zone_id = ? AND name = ? AND type = 'A' AND owner = 'dhcp'), LOWER(HEX(RANDOMBLOB(16)))),
				?, ?, 'A', 300, ?, 1, 'dhcp', datetime('now'), datetime('now')
			)`,
			zoneID, relativeName,
			zoneID, relativeName, l.IPAddress)
		if err != nil {
			slog.Error("dhcp_dns_link: failed to create A record", "error", err)
			return fmt.Errorf("failed to create A record: %w", err)
		}

		// Create PTR record if reverse zone exists.
		if err := dl.createPTRRecord(l); err != nil {
			slog.Error("dhcp_dns_link: failed to create PTR record", "error", err)
		}

		slog.Info("dhcp_dns_link: created DNS records",
			"hostname", cleaned, "ip", l.IPAddress, "zone", zoneName)

	case "delete":
		// Only delete DHCP-owned records.
		dl.deleteDHCPRecord(zoneID, relativeName, "A")

		// Delete PTR record.
		dl.deletePTRRecord(l)

		slog.Info("dhcp_dns_link: deleted DNS records",
			"hostname", cleaned, "ip", l.IPAddress, "zone", zoneName)
	}

	return nil
}

// HandleClientFQDN extracts the desired hostname from DHCP option 81 (Client FQDN).
func HandleClientFQDN(optionData []byte) string {
	if len(optionData) < 3 {
		return ""
	}

	// Option 81 format: flags(1) + RCODE(1) + domain-name in DNS wire format
	// Skip flags and RCODE, parse the domain name.
	domainName := decodeDNSWireFormat(optionData[2:])
	return domainName
}

// findForwardZone finds the best matching forward DNS zone for a hostname.
func (dl *DNSLink) findForwardZone(hostname string) (string, string, error) {
	// Normalize hostname.
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))

	rows, err := dl.db.Query(`
		SELECT id, name FROM dns_zones
		WHERE enabled = 1 AND name NOT LIKE '%in-addr.arpa%'
		ORDER BY LENGTH(name) DESC`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			slog.Warn("dns_link: failed to scan zone row", "error", err)
			continue
		}
		zoneName := strings.ToLower(strings.TrimSuffix(name, "."))
		if strings.HasSuffix(hostname, zoneName) || hostname == zoneName {
			return id, name, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("iterating zones: %w", err)
	}

	return "", "", nil
}

// findReverseZone finds the matching reverse DNS zone for an IP address.
func (dl *DNSLink) findReverseZone(ip string) (string, string, error) {
	reverseName := reverseIP(ip) + ".in-addr.arpa"

	rows, err := dl.db.Query(`
		SELECT id, name FROM dns_zones
		WHERE enabled = 1 AND name LIKE '%in-addr.arpa%'
		ORDER BY LENGTH(name) DESC`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			slog.Warn("dns_link: failed to scan reverse zone row", "error", err)
			continue
		}
		zoneName := strings.ToLower(strings.TrimSuffix(name, "."))
		if strings.HasSuffix(reverseName, zoneName) {
			return id, name, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("iterating reverse zones: %w", err)
	}

	return "", "", nil
}

// createPTRRecord creates a PTR record for a lease.
func (dl *DNSLink) createPTRRecord(l *lease.Lease) error {
	zoneID, zoneName, err := dl.findReverseZone(l.IPAddress)
	if err != nil || zoneID == "" {
		return nil // No reverse zone, skip PTR
	}

	reverseName := reverseIP(l.IPAddress) + ".in-addr.arpa"
	relativeName, ok := relativeNameInZone(reverseName, zoneName)
	if !ok {
		return nil
	}

	hostname := l.Hostname
	if !strings.HasSuffix(hostname, ".") {
		hostname += "."
	}

	_, err = dl.db.Exec(`
		INSERT OR REPLACE INTO dns_records (id, zone_id, name, type, ttl, value, enabled, owner, created_at, updated_at)
		VALUES (
			COALESCE((SELECT id FROM dns_records WHERE zone_id = ? AND name = ? AND type = 'PTR' AND owner = 'dhcp'), LOWER(HEX(RANDOMBLOB(16)))),
			?, ?, 'PTR', 300, ?, 1, 'dhcp', datetime('now'), datetime('now')
		)`,
		zoneID, relativeName,
		zoneID, relativeName, hostname)
	if err != nil {
		return fmt.Errorf("failed to create PTR record: %w", err)
	}

	return nil
}

// deletePTRRecord deletes the PTR record for a lease.
func (dl *DNSLink) deletePTRRecord(l *lease.Lease) error {
	zoneID, zoneName, err := dl.findReverseZone(l.IPAddress)
	if err != nil || zoneID == "" {
		return nil
	}

	reverseName := reverseIP(l.IPAddress) + ".in-addr.arpa"
	relativeName, ok := relativeNameInZone(reverseName, zoneName)
	if !ok {
		return nil
	}

	// Only delete DHCP-owned records.
	dl.deleteDHCPRecord(zoneID, relativeName, "PTR")
	return nil
}

// deleteDHCPRecord deletes a DNS record only if it was DHCP-created.
func (dl *DNSLink) deleteDHCPRecord(zoneID, name, recordType string) {
	_, err := dl.db.Exec(`
		DELETE FROM dns_records
		WHERE zone_id = ? AND name = ? AND type = ? AND owner = 'dhcp'`,
		zoneID, name, recordType)
	if err != nil {
		slog.Error("dhcp_dns_link: failed to delete DHCP record", "error", err)
	}
}

// reverseIP converts an IP address to reverse notation (e.g., "1.2.3.4" -> "4.3.2.1").
func reverseIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	parsed = parsed.To4()
	if parsed == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", parsed[3], parsed[2], parsed[1], parsed[0])
}

// decodeDNSWireFormat decodes a DNS wire format domain name.
func decodeDNSWireFormat(data []byte) string {
	var labels []string
	i := 0
	for i < len(data) {
		length := int(data[i])
		if length == 0 {
			break
		}
		i++
		if i+length > len(data) {
			break
		}
		labels = append(labels, string(data[i:i+length]))
		i += length
	}
	return strings.Join(labels, ".")
}

// maxHostLength is the maximum total length of a DNS name per RFC 1035 §2.3.4.
const maxHostLength = 253

// sanitizeHostname normalizes a hostname and rejects inputs that are not safe
// to use in downstream SQL queries. It enforces the 253-character limit,
// strips control characters and quotes, and lowercases the result.
func sanitizeHostname(h string) string {
	h = strings.TrimSpace(h)
	if h == "" || len(h) > maxHostLength {
		return ""
	}
	// Strip trailing dot for normalization.
	h = strings.TrimSuffix(h, ".")
	h = strings.ToLower(h)
	// Filter to the printable ASCII subset allowed by RFC 1035 plus '-'.
	// This also strips quotes and backticks that could enable SQL injection.
	var b strings.Builder
	b.Grow(len(h))
	for i := 0; i < len(h); i++ {
		c := h[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c)
		case c >= '0' && c <= '9':
			b.WriteByte(c)
		case c == '-' || c == '.' || c == '_':
			b.WriteByte(c)
		}
	}
	return b.String()
}

// relativeNameInZone returns the part of hostname that is strictly outside
// the given zone, using a label-boundary check. It returns ok=false when
// hostname does not actually fall within zone.
//
// Examples:
//
//	"host1.example.com.", "example.com." -> "host1", true
//	"example.com.",      "example.com." -> "",     true
//	"evil.com.",         "example.com." -> "",     false  (suffix match but not at label boundary)
func relativeNameInZone(hostname, zone string) (string, bool) {
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	zone = strings.ToLower(strings.TrimSuffix(zone, "."))
	if hostname == "" || zone == "" {
		return "", false
	}
	if hostname == zone {
		return "", true
	}
	suffix := "." + zone
	if !strings.HasSuffix(hostname, suffix) {
		return "", false
	}
	return hostname[:len(hostname)-len(suffix)], true
}
