package transfer

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// AXFRHandler handles full zone transfer requests.
type AXFRHandler struct {
	db *sql.DB
}

// NewAXFRHandler creates a new AXFR handler.
func NewAXFRHandler(db *sql.DB) *AXFRHandler {
	return &AXFRHandler{db: db}
}

// HandleAXFR handles a full zone transfer (AXFR) request.
// It returns all records in the zone as a slice of dns.RR.
func (h *AXFRHandler) HandleAXFR(zoneName string, tsigKeyName string) ([]dns.RR, error) {
	if zoneName == "" {
		return nil, fmt.Errorf("zone name is required")
	}

	// Ensure FQDN format.
	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}

	// Check transfer ACL.
	if tsigKeyName != "" {
		// TSIG key provided; verify it's authorized.
		var count int
		err := h.db.QueryRow(`
			SELECT COUNT(*) FROM dns_zone_transfer zt
			JOIN dns_zones z ON zt.zone_id = z.id
			WHERE z.name = ? AND zt.tsig_key_name = ?
		`, zoneName, tsigKeyName).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("checking transfer ACL: %w", err)
		}
		if count == 0 {
			return nil, fmt.Errorf("transfer not authorized for zone %s with key %s", zoneName, tsigKeyName)
		}
	} else {
		// No TSIG key; deny by default. Zone transfers without authentication are unsafe.
		return nil, fmt.Errorf("zone transfer requires TSIG authentication for zone %s", zoneName)
	}

	// Get zone ID.
	var zoneID string
	err := h.db.QueryRow("SELECT id FROM dns_zones WHERE name = ? AND enabled = 1", zoneName).Scan(&zoneID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found: %s", zoneName)
	}
	if err != nil {
		return nil, fmt.Errorf("querying zone: %w", err)
	}

	// Load all records for the zone.
	rows, err := h.db.Query(`
		SELECT name, type, ttl, value, priority, weight, port, tag, flag
		FROM dns_records WHERE zone_id = ? AND enabled = 1
		ORDER BY name, type
	`, zoneID)
	if err != nil {
		return nil, fmt.Errorf("querying records: %w", err)
	}
	defer rows.Close()

	var records []dns.RR
	for rows.Next() {
		var name, rtype, value string
		var ttl int
		var priority, weight, port, flag sql.NullInt64
		var tag sql.NullString

		if err := rows.Scan(&name, &rtype, &ttl, &value, &priority, &weight, &port, &tag, &flag); err != nil {
			continue
		}

		rr := buildTransferRR(name, rtype, ttl, value, priority, weight, port, tag, flag, zoneName)
		if rr != nil {
			records = append(records, rr)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating transfer records: %w", err)
	}

	// Add SOA at start and end (AXFR requirement).
	soaRR := h.getZoneSOA(zoneID, zoneName)
	if soaRR != nil {
		records = append([]dns.RR{soaRR}, records...)
		records = append(records, soaRR)
	}

	return records, nil
}

// HandleIXFR handles an incremental zone transfer (IXFR) request.
//
// The implementation is intentionally minimal: true IXFR diffing (computing
// the SOA deltas between the requester's serial and the current serial) is
// deferred. Until that is available, we require an authenticated request
// (via TSIG) before we can serve the zone at all. If the requester supplies
// a TSIG key, we fall back to a full AXFR; otherwise we return REFUSED, as
// the security review requires authenticated zone transfers.
func (h *AXFRHandler) HandleIXFR(zoneName string, serial uint32, tsigKeyName string) ([]dns.RR, error) {
	if zoneName == "" {
		return nil, fmt.Errorf("zone name is required")
	}

	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}

	// Get current zone serial.
	var currentSerial uint32
	err := h.db.QueryRow("SELECT serial FROM dns_zones WHERE name = ? AND enabled = 1", zoneName).Scan(&currentSerial)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found: %s", zoneName)
	}
	if err != nil {
		return nil, fmt.Errorf("querying zone serial: %w", err)
	}

	// If serials match, no transfer needed.
	if currentSerial == serial {
		return nil, nil
	}

	// We cannot serve a real IXFR diff yet, so either we sign the response
	// (refuse) or we fall back to AXFR. The fallback must still go through
	// the TSIG-authenticated path.
	if tsigKeyName == "" {
		return nil, fmt.Errorf("IXFR for zone %s requires TSIG authentication", zoneName)
	}
	slog.Warn("transfer: true IXFR diffing not implemented; falling back to AXFR",
		"zone", zoneName)
	return h.HandleAXFR(zoneName, tsigKeyName)
}

// SendNotify sends a DNS NOTIFY message to the specified targets.
//
// TSIG credentials are optional but recommended: most modern primaries will
// ignore unauthenticated NOTIFY messages. The caller passes a single
// (key, secret, algorithm) tuple that is applied to every outgoing NOTIFY.
// If no credentials are provided, NOTIFY messages are still sent (a log
// line is recorded) so that operators can see why a remote may have
// rejected the notification.
func SendNotify(zoneName string, targets []string) error {
	return SendNotifyWithTSIG(zoneName, targets, "", "", "")
}

// SendNotifyWithTSIG sends a DNS NOTIFY message to the specified targets,
// signing the query with the supplied TSIG credentials when they are
// provided.
func SendNotifyWithTSIG(zoneName string, targets []string, tsigKey, tsigSecret, tsigAlgo string) error {
	if zoneName == "" || len(targets) == 0 {
		return fmt.Errorf("zone name and targets are required")
	}

	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}

	if tsigKey == "" || tsigSecret == "" {
		slog.Warn("transfer: sending NOTIFY without TSIG; recipient may ignore",
			"zone", zoneName)
	}

	algo := dns.HmacSHA256
	switch tsigAlgo {
	case dns.HmacSHA512, string(TSIGHMACSHA512):
		algo = dns.HmacSHA512
	case dns.HmacSHA256, string(TSIGHMACSHA256), "":
		algo = dns.HmacSHA256
	}

	var lastErr error
	for _, target := range targets {
		// Parse host:port. strings.Contains(addr, ":") would misclassify a
		// bare IPv6 literal (e.g. "2001:db8::1") as already having a port.
		addr := target
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "53")
		}

		notify := new(dns.Msg)
		notify.SetNotify(zoneName)

		client := new(dns.Client)
		if tsigKey != "" && tsigSecret != "" {
			client.TsigProvider = tsigProvider{
				key:    tsigKey,
				secret: tsigSecret,
				algo:   algo,
			}
			notify.SetTsig(tsigKey, algo, 300, time.Now().Unix())
		}

		_, _, err := client.Exchange(notify, addr)
		if err != nil {
			lastErr = fmt.Errorf("sending NOTIFY to %s: %w", target, err)
			slog.Warn("transfer: NOTIFY failed", "target", target, "error", err)
			continue
		}
	}

	return lastErr
}

// CheckTransferACL checks if a client IP is allowed to transfer a zone.
func (h *AXFRHandler) CheckTransferACL(zoneName string, clientIP string) bool {
	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}

	rows, err := h.db.Query(`
		SELECT zt.allowed_cidr FROM dns_zone_transfer zt
		JOIN dns_zones z ON zt.zone_id = z.id
		WHERE z.name = ?
	`, zoneName)
	if err != nil {
		return false
	}
	defer rows.Close()

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			continue
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipNet.Contains(ip) {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("transfer: failed to iterate ACL entries", "error", err)
	}

	// If no ACL entries exist, deny by default.
	return false
}

// getZoneSOA builds the SOA RR for a zone from the database.
func (h *AXFRHandler) getZoneSOA(zoneID, zoneName string) dns.RR {
	var soaMName, soaRName string
	var serial, refresh, retry, expire, minimum uint32
	var defaultTTL int

	err := h.db.QueryRow(`
		SELECT soa_mname, soa_rname, serial, refresh, retry, expire, minimum, default_ttl
		FROM dns_zones WHERE id = ?
	`, zoneID).Scan(&soaMName, &soaRName, &serial, &refresh, &retry, &expire, &minimum, &defaultTTL)
	if err != nil {
		return nil
	}

	return &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   zoneName,
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
			Ttl:    uint32(defaultTTL),
		},
		Ns:      dns.Fqdn(soaMName),
		Mbox:    dns.Fqdn(soaRName),
		Serial:  serial,
		Refresh: refresh,
		Retry:   retry,
		Expire:  expire,
		Minttl:  minimum,
	}
}

// buildTransferRR constructs a dns.RR from database fields for zone transfer.
func buildTransferRR(name, rtype string, ttl int, value string,
	priority, weight, port sql.NullInt64, tag sql.NullString, flag sql.NullInt64, zoneName string) dns.RR {

	if name == "" || name == "@" {
		name = zoneName
	}
	if !strings.HasSuffix(name, ".") {
		name = name + "." + zoneName
	}
	name = dns.Fqdn(name)

	qtype, ok := dns.StringToType[rtype]
	if !ok {
		return nil
	}

	ttlU := uint32(ttl)
	if ttlU == 0 {
		ttlU = 3600
	}

	hdr := dns.RR_Header{
		Name:   name,
		Rrtype: qtype,
		Class:  dns.ClassINET,
		Ttl:    ttlU,
	}

	p := 0
	if priority.Valid {
		p = int(priority.Int64)
	}
	w := 0
	if weight.Valid {
		w = int(weight.Int64)
	}
	pt := 0
	if port.Valid {
		pt = int(port.Int64)
	}
	f := 0
	if flag.Valid {
		f = int(flag.Int64)
	}
	t := ""
	if tag.Valid {
		t = tag.String
	}

	switch rtype {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil {
			return nil
		}
		return &dns.A{Hdr: hdr, A: ip}
	case "AAAA":
		ip := net.ParseIP(value)
		if ip == nil {
			return nil
		}
		return &dns.AAAA{Hdr: hdr, AAAA: ip}
	case "CNAME":
		return &dns.CNAME{Hdr: hdr, Target: dns.Fqdn(value)}
	case "MX":
		return &dns.MX{Hdr: hdr, Preference: uint16(p), Mx: dns.Fqdn(value)}
	case "TXT":
		return &dns.TXT{Hdr: hdr, Txt: splitTXTValue(value)}
	case "SRV":
		return &dns.SRV{Hdr: hdr, Priority: uint16(p), Weight: uint16(w), Port: uint16(pt), Target: dns.Fqdn(value)}
	case "PTR":
		return &dns.PTR{Hdr: hdr, Ptr: dns.Fqdn(value)}
	case "NS":
		return &dns.NS{Hdr: hdr, Ns: dns.Fqdn(value)}
	case "CAA":
		return &dns.CAA{Hdr: hdr, Flag: uint8(f), Tag: t, Value: value}
	default:
		return nil
	}
}

// splitTXTValue splits a TXT value into 255-byte chunks.
func splitTXTValue(s string) []string {
	if len(s) <= 255 {
		return []string{s}
	}
	var chunks []string
	for len(s) > 255 {
		chunks = append(chunks, s[:255])
		s = s[255:]
	}
	if len(s) > 0 {
		chunks = append(chunks, s)
	}
	return chunks
}
