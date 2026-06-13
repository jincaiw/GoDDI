package zone

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/miekg/dns"
)

// ImportZoneFile parses a BIND zone file and imports records into the specified zone.
func (m *RecordManager) ImportZoneFile(zoneID string, content string) error {
	if zoneID == "" {
		return fmt.Errorf("zone id is required")
	}
	if content == "" {
		return fmt.Errorf("zone file content is empty")
	}

	// Verify zone exists.
	zone, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return err
	}

	// Parse the zone file using miekg/dns zone parser.
	zp := dns.NewZoneParser(strings.NewReader(content), zone.Name, "")

	// Collect all records first, then insert in a transaction.
	type importRR struct {
		id       string
		name     string
		rtype    string
		value    string
		ttl      int
		priority *int
		weight   *int
		port     *int
	}

	var records []importRR
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		hdr := rr.Header()

		// Skip SOA records - they are managed by the zone itself.
		if hdr.Rrtype == dns.TypeSOA {
			continue
		}

		name := hdr.Name
		// Strip trailing dot for storage.
		name = strings.TrimSuffix(name, ".")
		if strings.EqualFold(name+".", zone.Name) || name == "@" {
			name = zone.Name
		}

		rtype := dns.TypeToString[hdr.Rrtype]
		value := rrToString(rr)
		if value == "" {
			continue
		}

		id := uuid.New().String()
		priority, weight, port := extractRRMeta(rr)

		rec := importRR{
			id:    id,
			name:  name,
			rtype: rtype,
			value: value,
			ttl:   int(hdr.Ttl),
		}
		if priority != 0 {
			rec.priority = &priority
		}
		if weight != 0 {
			rec.weight = &weight
		}
		if port != 0 {
			rec.port = &port
		}
		records = append(records, rec)
	}

	if err := zp.Err(); err != nil {
		return fmt.Errorf("zone file parse error: %w", err)
	}

	// Insert all records in a single transaction.
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, rec := range records {
		_, err := stmt.Exec(rec.id, zoneID, rec.name, rec.rtype, rec.value, rec.ttl,
			nullInt(rec.priority), nullInt(rec.weight), nullInt(rec.port))
		if err != nil {
			return fmt.Errorf("insert record %s: %w", rec.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Increment zone serial.
	if _, err := m.zoneMgr.IncrementSerial(zoneID); err != nil {
		_ = err
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

// ExportZoneFile generates a BIND zone file format for the specified zone.
func (m *RecordManager) ExportZoneFile(zoneID string) (string, error) {
	if zoneID == "" {
		return "", fmt.Errorf("zone id is required")
	}

	zone, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	// Write $ORIGIN and $TTL directives.
	sb.WriteString(fmt.Sprintf("$ORIGIN %s\n", zone.Name))
	sb.WriteString(fmt.Sprintf("$TTL %d\n", zone.DefaultTTL))
	sb.WriteString("\n")

	// Write SOA record.
	sb.WriteString(fmt.Sprintf("@\tIN\tSOA\t%s %s (\n", zone.SOA_MName, zone.SOA_RName))
	sb.WriteString(fmt.Sprintf("\t\t%d\t; serial\n", zone.Serial))
	sb.WriteString(fmt.Sprintf("\t\t%d\t; refresh\n", zone.Refresh))
	sb.WriteString(fmt.Sprintf("\t\t%d\t; retry\n", zone.Retry))
	sb.WriteString(fmt.Sprintf("\t\t%d\t; expire\n", zone.Expire))
	sb.WriteString(fmt.Sprintf("\t\t%d\t; minimum\n", zone.Minimum))
	sb.WriteString("\t)\n\n")

	// Write all records.
	records, _, err := m.ListRecords(RecordFilter{ZoneID: zoneID, PageSize: 10000})
	if err != nil {
		return "", fmt.Errorf("listing records: %w", err)
	}

	for _, r := range records {
		name := r.Name
		// Use relative name if it matches zone.
		if strings.EqualFold(name, zone.Name) {
			name = "@"
		} else if strings.HasSuffix(strings.ToLower(name), strings.ToLower(zone.Name)) {
			name = name[:len(name)-len(zone.Name)]
			name = strings.TrimSuffix(name, ".")
		}

		sb.WriteString(fmt.Sprintf("%s\t%d\tIN\t%s\t%s\n", name, r.TTL, r.Type, formatRecordValue(r)))
	}

	return sb.String(), nil
}

// ImportRecordsCSV imports records from CSV data into the specified zone.
func (m *RecordManager) ImportRecordsCSV(zoneID string, csvData []byte) error {
	if zoneID == "" {
		return fmt.Errorf("zone id is required")
	}

	// Verify zone exists.
	_, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return err
	}

	r := csv.NewReader(strings.NewReader(string(csvData)))
	// Skip header row.
	if _, err := r.Read(); err != nil {
		return fmt.Errorf("reading CSV header: %w", err)
	}

	// Collect all records first, then insert in a transaction.
	type csvRecord struct {
		id       string
		name     string
		rtype    string
		value    string
		ttl      int
		priority int
		weight   int
		port     int
	}

	var records []csvRecord
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// CSV format: name, type, value, ttl, priority, weight, port
		if len(record) < 3 {
			continue
		}

		name := record[0]
		rtype := strings.ToUpper(record[1])
		value := record[2]
		ttl := 3600
		priority := 0
		weight := 0
		port := 0

		if len(record) > 3 && record[3] != "" {
			if v, err := strconv.Atoi(record[3]); err == nil {
				ttl = v
			}
		}
		if len(record) > 4 && record[4] != "" {
			if v, err := strconv.Atoi(record[4]); err == nil {
				priority = v
			}
		}
		if len(record) > 5 && record[5] != "" {
			if v, err := strconv.Atoi(record[5]); err == nil {
				weight = v
			}
		}
		if len(record) > 6 && record[6] != "" {
			if v, err := strconv.Atoi(record[6]); err == nil {
				port = v
			}
		}

		if !SupportedRecordTypes[rtype] {
			continue
		}

		records = append(records, csvRecord{
			id:       uuid.New().String(),
			name:     name,
			rtype:    rtype,
			value:    value,
			ttl:      ttl,
			priority: priority,
			weight:   weight,
			port:     port,
		})
	}

	// Insert all records in a single transaction.
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, rec := range records {
		_, err := stmt.Exec(rec.id, zoneID, rec.name, rec.rtype, rec.value, rec.ttl, rec.priority, rec.weight, rec.port)
		if err != nil {
			return fmt.Errorf("insert record %s: %w", rec.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Increment zone serial.
	if _, err := m.zoneMgr.IncrementSerial(zoneID); err != nil {
		_ = err
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

// ExportRecordsCSV exports records from the specified zone as CSV data.
func (m *RecordManager) ExportRecordsCSV(zoneID string) ([]byte, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone id is required")
	}

	records, _, err := m.ListRecords(RecordFilter{ZoneID: zoneID, PageSize: 10000})
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("name,type,value,ttl,priority,weight,port\n")

	for _, r := range records {
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%d,%d,%d,%d\n",
			r.Name, r.Type, r.Value, r.TTL, r.Priority, r.Weight, r.Port))
	}

	return []byte(sb.String()), nil
}

// rrToString converts a dns.RR to a string value suitable for database storage.
func rrToString(rr dns.RR) string {
	switch v := rr.(type) {
	case *dns.A:
		return v.A.String()
	case *dns.AAAA:
		return v.AAAA.String()
	case *dns.CNAME:
		return v.Target
	case *dns.DNAME:
		return v.Target
	case *dns.MX:
		return v.Mx
	case *dns.TXT:
		return strings.Join(v.Txt, "")
	case *dns.SRV:
		return v.Target
	case *dns.PTR:
		return v.Ptr
	case *dns.NS:
		return v.Ns
	case *dns.CAA:
		return v.Value
	case *dns.NAPTR:
		return v.Replacement
	case *dns.URI:
		return v.Target
	case *dns.SSHFP:
		return fmt.Sprintf("%d %d %s", v.Algorithm, v.Type, v.FingerPrint)
	case *dns.TLSA:
		return fmt.Sprintf("%d %d %d %s", v.Usage, v.Selector, v.MatchingType, v.Certificate)
	case *dns.DS:
		return fmt.Sprintf("%d %d %d %s", v.KeyTag, v.Algorithm, v.DigestType, v.Digest)
	case *dns.DNSKEY:
		return fmt.Sprintf("%d %d %d %s", v.Flags, v.Protocol, v.Algorithm, v.PublicKey)
	default:
		return ""
	}
}

// extractRRMeta extracts priority, weight, port from a dns.RR.
func extractRRMeta(rr dns.RR) (priority, weight, port int) {
	switch v := rr.(type) {
	case *dns.MX:
		priority = int(v.Preference)
	case *dns.SRV:
		priority = int(v.Priority)
		weight = int(v.Weight)
		port = int(v.Port)
	case *dns.URI:
		priority = int(v.Priority)
		weight = int(v.Weight)
	}
	return
}

// formatRecordValue formats a record value for zone file output.
func formatRecordValue(r Record) string {
	switch r.Type {
	case "MX":
		return fmt.Sprintf("%d %s", r.Priority, r.Value)
	case "SRV":
		return fmt.Sprintf("%d %d %d %s", r.Priority, r.Weight, r.Port, r.Value)
	case "CAA":
		return fmt.Sprintf("%d %s \"%s\"", r.Flag, r.Tag, r.Value)
	case "NAPTR":
		return fmt.Sprintf("%d %d %s", r.Priority, r.Weight, r.Value)
	case "URI":
		return fmt.Sprintf("%d %d \"%s\"", r.Priority, r.Weight, r.Value)
	case "TXT":
		return fmt.Sprintf("\"%s\"", strings.ReplaceAll(r.Value, "\"", "\\\""))
	default:
		return r.Value
	}
}
