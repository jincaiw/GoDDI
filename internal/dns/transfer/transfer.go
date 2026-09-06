package transfer

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strconv"
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

// HandleIXFR handles an incremental zone transfer (IXFR, RFC 1995) request.
//
// Deltas are served from the dns_zone_changes history table, which every
// record mutation writes to. When the requester's serial is current, a
// single SOA is returned; when the history does not reach back far enough
// (client too old or history pruned), the request falls back to a full
// AXFR. Both paths require TSIG authentication.
func (h *AXFRHandler) HandleIXFR(zoneName string, serial uint32, tsigKeyName string) ([]dns.RR, error) {
	if zoneName == "" {
		return nil, fmt.Errorf("zone name is required")
	}

	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}

	// IXFR (like AXFR) requires TSIG authentication.
	if tsigKeyName == "" {
		return nil, fmt.Errorf("IXFR for zone %s requires TSIG authentication", zoneName)
	}
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

	var zoneID string
	var currentSerial uint32
	err = h.db.QueryRow("SELECT id, serial FROM dns_zones WHERE name = ? AND enabled = 1", zoneName).Scan(&zoneID, &currentSerial)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found: %s", zoneName)
	}
	if err != nil {
		return nil, fmt.Errorf("querying zone serial: %w", err)
	}

	currentSOA := h.getZoneSOA(zoneID, zoneName)
	if currentSOA == nil {
		return nil, fmt.Errorf("zone %s has no SOA", zoneName)
	}

	// Client is already up to date: respond with just the current SOA.
	if serial == currentSerial {
		return []dns.RR{currentSOA}, nil
	}

	// Collect change history newer than the client's serial, grouped by
	// the serial that was in effect after the change was applied.
	changes, err := h.loadChanges(zoneID, serial)
	if err != nil {
		return nil, err
	}

	// No usable history (e.g. pruned or brand-new history): fall back to
	// a full AXFR rather than serving a wrong delta.
	if len(changes) == 0 {
		slog.Info("transfer: IXFR history unavailable; falling back to AXFR",
			"zone", zoneName, "client_serial", serial)
		return h.HandleAXFR(zoneName, tsigKeyName)
	}

	// RFC 1995 delta layout:
	//   SOA(current)
	//   for each version (ascending serial):
	//     SOA(version)          — old SOA marking the start of the delta
	//     deletions (CLASS NONE)
	//     additions (CLASS IN)
	//   SOA(current)
	resp := []dns.RR{currentSOA}
	for _, group := range changes {
		oldSOA := h.soaWithSerial(currentSOA, group.serial)
		resp = append(resp, oldSOA)
		for _, ch := range group.deletions {
			ch.Header().Class = dns.ClassNONE
			ch.Header().Ttl = 0
			resp = append(resp, ch)
		}
		resp = append(resp, group.additions...)
	}
	resp = append(resp, currentSOA)
	return resp, nil
}

// ixfrGroup holds the mutations of one serial version.
type ixfrGroup struct {
	serial     uint32
	deletions  []dns.RR
	additions  []dns.RR
}

// loadChanges reads dns_zone_changes newer than clientSerial and converts
// the rows into RR groups per serial version. It returns an empty slice
// when no history covers the requested range.
func (h *AXFRHandler) loadChanges(zoneID string, clientSerial uint32) ([]ixfrGroup, error) {
	rows, err := h.db.Query(`
		SELECT serial, change_type, name, type, value, ttl, priority, weight, port
		FROM dns_zone_changes
		WHERE zone_id = ? AND serial > ?
		ORDER BY serial ASC, created_at ASC
	`, zoneID, clientSerial)
	if err != nil {
		return nil, fmt.Errorf("querying zone changes: %w", err)
	}
	defer rows.Close()

	// Need the zone name to normalize record names.
	var zoneName string
	if err := h.db.QueryRow("SELECT name FROM dns_zones WHERE id = ?", zoneID).Scan(&zoneName); err != nil {
		return nil, fmt.Errorf("querying zone name: %w", err)
	}

	var groups []ixfrGroup
	for rows.Next() {
		var serial uint32
		var changeType, name, rtype, value string
		var ttl int
		var priority, weight, port sql.NullInt64

		if err := rows.Scan(&serial, &changeType, &name, &rtype, &value, &ttl, &priority, &weight, &port); err != nil {
			continue
		}

		rr := buildTransferRR(name, rtype, ttl, value, priority, weight, port, sql.NullString{}, sql.NullInt64{}, zoneName)
		if rr == nil {
			continue
		}

		// Append to the group for this serial (rows arrive in order).
		if len(groups) == 0 || groups[len(groups)-1].serial != serial {
			groups = append(groups, ixfrGroup{serial: serial})
		}
		g := &groups[len(groups)-1]
		if changeType == "delete" {
			g.deletions = append(g.deletions, rr)
		} else {
			g.additions = append(g.additions, rr)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating zone changes: %w", err)
	}
	return groups, nil
}

// soaWithSerial returns a copy of soa with the given serial.
func (h *AXFRHandler) soaWithSerial(soa dns.RR, serial uint32) dns.RR {
	s, ok := soa.(*dns.SOA)
	if !ok {
		return soa
	}
	clone := *s
	clone.Hdr = s.Hdr
	clone.Serial = serial
	return &clone
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
	case "ZONEMD":
		// RFC 8976: value format "serial scheme algorithm digest".
		parts := strings.Fields(value)
		if len(parts) < 4 {
			return nil
		}
		scheme := parseUint8(parts[1])
		algo := parseUint8(parts[2])
		if scheme == 0 || algo == 0 {
			return nil
		}
		return &dns.ZONEMD{Hdr: hdr, Serial: parseUint32(parts[0]), Scheme: scheme, Hash: algo, Digest: parts[3]}
	default:
		return nil
	}
}

// parseUint32 parses a decimal uint32, returning 0 on error.
func parseUint32(s string) uint32 {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

// parseUint8 parses a decimal uint8, returning 0 on error.
func parseUint8(s string) uint8 {
	v, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return 0
	}
	return uint8(v)
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
