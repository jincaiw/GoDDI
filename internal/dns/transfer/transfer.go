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

	// Read the zone row, records, and SOA from one SQLite snapshot. A full
	// transfer must not pair records from one version with an SOA from another.
	tx, err := h.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin AXFR snapshot: %w", err)
	}
	defer tx.Rollback()

	var zoneID string
	err = tx.QueryRow("SELECT id FROM dns_zones WHERE name = ? AND enabled = 1", zoneName).Scan(&zoneID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found: %s", zoneName)
	}
	if err != nil {
		return nil, fmt.Errorf("querying zone: %w", err)
	}

	// Load all records for the zone.
	rows, err := tx.Query(`
		SELECT name, type, ttl, value, priority, weight, port, tag, flag
		FROM dns_records WHERE zone_id = ? AND enabled = 1
		  AND (expires_at IS NULL OR expires_at > datetime('now'))
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
			return nil, fmt.Errorf("scanning transfer record: %w", err)
		}

		rr := buildTransferRR(name, rtype, ttl, value, priority, weight, port, tag, flag, zoneName)
		if rr == nil {
			return nil, fmt.Errorf("building AXFR record for %s %s: unsupported or invalid record value", name, rtype)
		}
		records = append(records, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating transfer records: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing transfer records: %w", err)
	}

	// Add SOA at start and end (AXFR requirement).
	soaRR := getZoneSOA(tx, zoneID, zoneName)
	if soaRR == nil {
		return nil, fmt.Errorf("zone %s has no SOA", zoneName)
	}
	records = append([]dns.RR{soaRR}, records...)
	records = append(records, soaRR)
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit AXFR snapshot: %w", err)
	}

	return records, nil
}

// HandleIXFR handles an incremental zone transfer (IXFR, RFC 1995) request.
//
// IXFR serves a complete journal chain when it can prove that every serial
// transition from the client's version to the current version is present.
// Missing, malformed, or discontinuous history falls back to AXFR.
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

	tx, err := h.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin IXFR serial snapshot: %w", err)
	}
	var zoneID string
	err = tx.QueryRow("SELECT id FROM dns_zones WHERE name = ? AND enabled = 1", zoneName).Scan(&zoneID)
	if err == sql.ErrNoRows {
		_ = tx.Rollback()
		return nil, fmt.Errorf("zone not found: %s", zoneName)
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("querying zone serial: %w", err)
	}

	currentSOA := getZoneSOA(tx, zoneID, zoneName)
	if currentSOA == nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("zone %s has no SOA", zoneName)
	}
	soa, ok := currentSOA.(*dns.SOA)
	if !ok {
		_ = tx.Rollback()
		return nil, fmt.Errorf("zone %s SOA has an unexpected record type", zoneName)
	}
	currentSerial := soa.Serial

	// RFC 1982 comparison: the half-range case is undefined, so it is handled
	// by falling back to AXFR below rather than guessing which copy is newer.
	distance := currentSerial - serial
	if distance == 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit IXFR serial snapshot: %w", err)
		}
		return []dns.RR{currentSOA}, nil
	}
	if distance > 1<<31 && distance != 1<<31 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit IXFR serial snapshot: %w", err)
		}
		return []dns.RR{currentSOA}, nil
	}
	if distance == 1<<31 {
		_ = tx.Rollback()
		return h.HandleAXFR(zoneName, tsigKeyName)
	}

	ixfrRecords, complete, err := readIXFRJournalTx(tx, zoneID, zoneName, serial, currentSerial, currentSOA)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("reading IXFR journal: %w", err)
	}
	if complete {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit IXFR journal snapshot: %w", err)
		}
		return ixfrRecords, nil
	}
	if err := tx.Rollback(); err != nil {
		return nil, fmt.Errorf("release IXFR serial snapshot: %w", err)
	}

	slog.Info("transfer: serving full AXFR response because a complete IXFR journal chain is unavailable",
		"zone", zoneName, "client_serial", serial, "current_serial", currentSerial)
	return h.HandleAXFR(zoneName, tsigKeyName)
}

type ixfrJournalRow struct {
	serial     uint32
	changeType string
	name       string
	rtype      string
	value      string
	ttl        int
	priority   sql.NullInt64
	weight     sql.NullInt64
	port       sql.NullInt64
	flag       sql.NullInt64
	tag        sql.NullString
}

type ixfrDelta struct {
	serial      uint32
	oldSOA      *dns.SOA
	newSOA      *dns.SOA
	oldSOAIndex int
	newSOAIndex int
	rowCount    int
	delete      []dns.RR
	add         []dns.RR
	bad         bool
}

func readIXFRJournalTx(tx *sql.Tx, zoneID, zoneName string, clientSerial, currentSerial uint32, currentSOA dns.RR) ([]dns.RR, bool, error) {
	rows, err := tx.Query(`SELECT serial, change_type, name, type, value, COALESCE(ttl, 300),
		priority, weight, port, flag, tag FROM dns_zone_changes
		WHERE zone_id = ? ORDER BY rowid LIMIT 100001`, zoneID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var deltas []*ixfrDelta
	var active *ixfrDelta
	rowCount := 0
	for rows.Next() {
		rowCount++
		if rowCount > 100000 {
			return nil, false, nil
		}
		var row ixfrJournalRow
		if err := rows.Scan(&row.serial, &row.changeType, &row.name, &row.rtype, &row.value, &row.ttl,
			&row.priority, &row.weight, &row.port, &row.flag, &row.tag); err != nil {
			return nil, false, err
		}
		if active == nil || active.serial != row.serial {
			active = &ixfrDelta{serial: row.serial}
			deltas = append(deltas, active)
		}
		active.rowCount++
		if strings.EqualFold(row.rtype, "SOA") {
			rr, err := dns.NewRR(fmt.Sprintf("%s %d IN SOA %s", dns.Fqdn(row.name), row.ttl, row.value))
			if err != nil {
				active.bad = true
				continue
			}
			soa, ok := rr.(*dns.SOA)
			if !ok {
				active.bad = true
				continue
			}
			switch row.changeType {
			case "delete":
				if active.oldSOA != nil {
					active.bad = true
				}
				active.oldSOA = soa
				active.oldSOAIndex = active.rowCount
			case "add":
				if active.newSOA != nil {
					active.bad = true
				}
				active.newSOA = soa
				active.newSOAIndex = active.rowCount
			default:
				active.bad = true
			}
			continue
		}
		rr := buildTransferRR(row.name, row.rtype, row.ttl, row.value,
			row.priority, row.weight, row.port, row.tag, row.flag, zoneName)
		if rr == nil {
			active.bad = true
			continue
		}
		switch row.changeType {
		case "delete":
			active.delete = append(active.delete, rr)
		case "add":
			active.add = append(active.add, rr)
		default:
			active.bad = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(deltas) == 0 || len(deltas) > 100000 {
		return nil, false, nil
	}
	// A delta group is usable only when its delimiters agree with both the
	// group's serial and the serial at the end of the preceding transition.
	for _, delta := range deltas {
		if delta.bad || delta.oldSOA == nil || delta.newSOA == nil ||
			delta.newSOA.Serial != delta.serial ||
			delta.oldSOAIndex != 1 || delta.newSOAIndex != delta.rowCount ||
			!strings.EqualFold(delta.oldSOA.Hdr.Name, zoneName) || !strings.EqualFold(delta.newSOA.Hdr.Name, zoneName) {
			delta.bad = true
		}
	}

	for start, delta := range deltas {
		if delta.bad || delta.oldSOA.Serial != clientSerial {
			continue
		}
		chain := []*ixfrDelta{delta}
		expected := delta.newSOA.Serial
		for i := start + 1; i < len(deltas) && expected != currentSerial; i++ {
			next := deltas[i]
			if next.bad || next.oldSOA.Serial != expected {
				break
			}
			chain = append(chain, next)
			expected = next.newSOA.Serial
		}
		if expected != currentSerial {
			continue
		}
		lastSOA := chain[len(chain)-1].newSOA
		if lastSOA.String() != currentSOA.String() {
			continue
		}
		answer := []dns.RR{currentSOA}
		for _, change := range chain {
			answer = append(answer, change.oldSOA)
			answer = append(answer, change.delete...)
			answer = append(answer, change.newSOA)
			answer = append(answer, change.add...)
		}
		answer = append(answer, currentSOA)
		return answer, true, nil
	}
	return nil, false, nil
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

type rowQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func getZoneSOA(queryer rowQueryer, zoneID, zoneName string) dns.RR {
	var soaMName, soaRName string
	var serial, refresh, retry, expire, minimum uint32
	var defaultTTL int

	err := queryer.QueryRow(`
		SELECT soa_mname, soa_rname, serial, refresh, retry, expire, minimum, default_ttl
		FROM dns_zones WHERE id = ?
	`, zoneID).Scan(&soaMName, &soaRName, &serial, &refresh, &retry, &expire, &minimum, &defaultTTL)
	if err != nil {
		return nil
	}
	if defaultTTL < 0 || uint64(defaultTTL) > 2147483647 || minimum > 2147483647 {
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
	if ttl < 0 || uint64(ttl) > 2147483647 {
		return nil
	}

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
