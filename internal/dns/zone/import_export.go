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

// CSVImportPreview summarizes a fully parsed CSV import without writing it.
type CSVImportPreview struct {
	RecordCount int                 `json:"record_count"`
	Creates     int                 `json:"creates"`
	Unchanged   int                 `json:"unchanged"`
	RecordTypes map[string]int      `json:"record_types"`
	Valid       bool                `json:"valid"`
	Conflicts   []CSVImportConflict `json:"conflicts,omitempty"`
}

// CSVImportConflict identifies an import row that cannot be applied without
// violating an existing or incoming RRset invariant.
type CSVImportConflict struct {
	Row     int    `json:"row"`
	Owner   string `json:"owner"`
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CSVImportConflictError indicates that a syntactically valid import contains
// one or more records that conflict with the target zone or each other.
type CSVImportConflictError struct {
	Conflicts []CSVImportConflict
}

func (e *CSVImportConflictError) Error() string {
	if e == nil || len(e.Conflicts) == 0 {
		return "CSV import contains conflicts"
	}
	return fmt.Sprintf("CSV import contains %d conflict(s)", len(e.Conflicts))
}

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
		flag     *int
		tag      string
	}

	var records []importRR
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		hdr := rr.Header()

		// Skip SOA records - they are managed by the zone itself.
		if hdr.Rrtype == dns.TypeSOA {
			continue
		}
		if !recordNameInZone(hdr.Name, zone.Name) {
			return fmt.Errorf("zone-file record owner %q is outside zone %q", hdr.Name, zone.Name)
		}

		name := hdr.Name
		// Strip trailing dot for storage.
		name = strings.TrimSuffix(name, ".")
		if strings.EqualFold(name+".", zone.Name) || name == "@" {
			name = zone.Name
		}

		rtype := dns.TypeToString[hdr.Rrtype]
		if rtype == "" {
			return fmt.Errorf("zone-file import contains an unsupported RR type %d", hdr.Rrtype)
		}
		value := rrToString(rr)
		if value == "" && rtype != "TXT" {
			return fmt.Errorf("zone-file import cannot represent %s record at %s", rtype, hdr.Name)
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
		if caa, ok := rr.(*dns.CAA); ok {
			flag := int(caa.Flag)
			rec.flag = &flag
			rec.tag = caa.Tag
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
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, tag, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, rec := range records {
		priority, weight, port, flag := intOrZero(rec.priority), intOrZero(rec.weight), intOrZero(rec.port), intOrZero(rec.flag)
		if err := validateRecordTTL(rec.ttl); err != nil {
			return fmt.Errorf("zone-file import: %w", err)
		}
		if err := validateRecordValue(rec.rtype, rec.value, &priority, &weight, &port, rec.tag, &flag); err != nil {
			return fmt.Errorf("zone-file import: invalid %s record at %s: %w", rec.rtype, rec.name, err)
		}
		if err := ValidateRRsetTTLTx(tx, zoneID, rec.name, rec.rtype, rec.ttl, ""); err != nil {
			return fmt.Errorf("zone-file import: %w", err)
		}
		var cnameConflicts int
		if rec.rtype == "CNAME" {
			if err := tx.QueryRow(`SELECT COUNT(*) FROM dns_records
				WHERE zone_id = ? AND LOWER(name) = LOWER(?) AND enabled = 1
				AND (expires_at IS NULL OR julianday(expires_at) > julianday('now')) AND type != 'CNAME'`,
				zoneID, rec.name).Scan(&cnameConflicts); err != nil {
				return fmt.Errorf("zone-file import: checking CNAME exclusivity: %w", err)
			}
			if cnameConflicts == 0 {
				if err := tx.QueryRow(`SELECT COUNT(*) FROM dns_records
					WHERE zone_id = ? AND LOWER(name) = LOWER(?) AND enabled = 1
					AND (expires_at IS NULL OR julianday(expires_at) > julianday('now'))
					AND type = 'CNAME' AND LOWER(value) != LOWER(?)`,
					zoneID, rec.name, rec.value).Scan(&cnameConflicts); err != nil {
					return fmt.Errorf("zone-file import: checking CNAME targets: %w", err)
				}
			}
		} else if err := tx.QueryRow(`SELECT COUNT(*) FROM dns_records
			WHERE zone_id = ? AND LOWER(name) = LOWER(?) AND enabled = 1
			AND (expires_at IS NULL OR julianday(expires_at) > julianday('now')) AND type = 'CNAME'`,
			zoneID, rec.name).Scan(&cnameConflicts); err != nil {
			return fmt.Errorf("zone-file import: checking CNAME exclusivity: %w", err)
		}
		if cnameConflicts > 0 {
			return fmt.Errorf("zone-file import: CNAME conflict at %s", rec.name)
		}
		_, err := stmt.Exec(rec.id, zoneID, rec.name, rec.rtype, rec.value, rec.ttl,
			nullInt(rec.priority), nullInt(rec.weight), nullInt(rec.port), nullInt(rec.flag), rec.tag)
		if err != nil {
			return fmt.Errorf("insert record %s: %w", rec.id, err)
		}
	}
	if len(records) > 0 {
		serial, err := bumpZoneSerialTx(tx, zoneID)
		if err != nil {
			return fmt.Errorf("bumping zone serial: %w", err)
		}
		for _, rec := range records {
			if err := logChangeTx(tx, zoneID, serial, "add", rec.name, rec.rtype, rec.value, rec.ttl,
				intOrZero(rec.priority), intOrZero(rec.weight), intOrZero(rec.port), intOrZero(rec.flag), rec.tag); err != nil {
				return fmt.Errorf("journaling imported record %s: %w", rec.id, err)
			}
		}
		if err := logCurrentSOAStateTx(tx, zoneID, serial); err != nil {
			return fmt.Errorf("journaling updated SOA: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	if len(records) > 0 {
		m.notifyPrimary(zoneID)
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
	return m.importRecordsCSV(zoneID, csvData, false, nil)
}

// PreviewRecordsCSV validates a CSV import and reports its size without writing records.
func (m *RecordManager) PreviewRecordsCSV(zoneID string, csvData []byte) (CSVImportPreview, error) {
	var preview CSVImportPreview
	err := m.importRecordsCSV(zoneID, csvData, true, &preview)
	return preview, err
}

func (m *RecordManager) importRecordsCSV(zoneID string, csvData []byte, dryRun bool, preview *CSVImportPreview) error {
	if zoneID == "" {
		return fmt.Errorf("zone id is required")
	}

	// Verify zone exists.
	zone, err := m.zoneMgr.GetZone(zoneID)
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
		row      int
		name     string
		rtype    string
		value    string
		ttl      int
		priority int
		weight   int
		port     int
		flag     int
		tag      string
	}

	var records []csvRecord
	rowNumber := 1 // The header occupies the first CSV row.
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading CSV record: %w", err)
		}
		rowNumber++

		// CSV format: name, type, value, ttl, priority, weight, port, flag, tag.
		if len(record) < 3 {
			return fmt.Errorf("CSV record must contain at least name, type, and value")
		}
		if len(record) > 9 {
			return fmt.Errorf("CSV record has %d columns; at most 9 are supported", len(record))
		}

		name := normalizeRecordName(record[0], zone.Name)
		if !recordNameInZone(name, zone.Name) {
			return fmt.Errorf("CSV record owner %q is outside zone %q", record[0], zone.Name)
		}
		rtype := strings.ToUpper(record[1])
		value := record[2]
		if rtype == "NAPTR" {
			value = normalizeNAPTRValue(value)
		}
		ttl := 3600
		priority := 0
		weight := 0
		port := 0
		flag := 0
		tag := ""

		if len(record) > 3 && record[3] != "" {
			v, err := strconv.Atoi(record[3])
			if err != nil || v < 0 {
				return fmt.Errorf("invalid CSV TTL %q", record[3])
			}
			ttl = v
		}
		if err := validateRecordTTL(ttl); err != nil {
			return fmt.Errorf("invalid CSV TTL: %w", err)
		}
		if len(record) > 4 && record[4] != "" {
			v, err := strconv.Atoi(record[4])
			if err != nil {
				return fmt.Errorf("invalid CSV priority %q", record[4])
			}
			priority = v
		}
		if len(record) > 5 && record[5] != "" {
			v, err := strconv.Atoi(record[5])
			if err != nil {
				return fmt.Errorf("invalid CSV weight %q", record[5])
			}
			weight = v
		}
		if len(record) > 6 && record[6] != "" {
			v, err := strconv.Atoi(record[6])
			if err != nil {
				return fmt.Errorf("invalid CSV port %q", record[6])
			}
			port = v
		}
		if !SupportedRecordTypes[rtype] {
			return fmt.Errorf("CSV contains unsupported record type %q", rtype)
		}
		if rtype == "CAA" {
			if len(record) < 9 {
				return fmt.Errorf("CSV CAA record requires flag and tag columns")
			}
			flag, err = strconv.Atoi(record[7])
			if err != nil || flag < 0 || flag > 255 || strings.TrimSpace(record[8]) == "" {
				return fmt.Errorf("CSV CAA record requires a flag from 0 to 255 and a non-empty tag")
			}
			// Validate against the same CAA tag/value rules as API writes.
			tag = record[8]
		}
		if err := validateRecordValue(rtype, value, &priority, &weight, &port, tag, &flag); err != nil {
			return fmt.Errorf("invalid CSV %s record: %w", rtype, err)
		}

		records = append(records, csvRecord{
			id:       uuid.New().String(),
			row:      rowNumber,
			name:     name,
			rtype:    rtype,
			value:    value,
			ttl:      ttl,
			priority: priority,
			weight:   weight,
			port:     port,
			flag:     flag,
			tag:      tag,
		})
	}
	// Classify duplicates and CNAME conflicts against one database snapshot,
	// then insert the accepted rows in that same transaction.
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	existingRows, err := tx.Query(`
		SELECT name, type, value, ttl, COALESCE(priority, 0), COALESCE(weight, 0),
			COALESCE(port, 0), COALESCE(flag, 0), COALESCE(tag, '')
		FROM dns_records WHERE zone_id = ? AND enabled = 1
			AND (expires_at IS NULL OR julianday(expires_at) > julianday('now'))`, zoneID)
	if err != nil {
		return fmt.Errorf("querying existing records: %w", err)
	}
	existing := make(map[string]int)
	ownerTypes := make(map[string]map[string]struct{})
	rrsetTTLs := make(map[string]map[int]struct{})
	for existingRows.Next() {
		var name, rtype, value, tag string
		var ttl, priority, weight, port, flag int
		if err := existingRows.Scan(&name, &rtype, &value, &ttl, &priority, &weight, &port, &flag, &tag); err != nil {
			existingRows.Close()
			return fmt.Errorf("scanning existing record: %w", err)
		}
		owner := dns.Fqdn(strings.ToLower(name))
		existing[csvRecordIdentity(owner, rtype, value, priority, weight, port, flag, tag)] = ttl
		if ownerTypes[owner] == nil {
			ownerTypes[owner] = make(map[string]struct{})
		}
		ownerTypes[owner][rtype] = struct{}{}
		rrsetKey := csvRRsetIdentity(owner, rtype)
		if rrsetTTLs[rrsetKey] == nil {
			rrsetTTLs[rrsetKey] = make(map[int]struct{})
		}
		rrsetTTLs[rrsetKey][ttl] = struct{}{}
	}
	if err := existingRows.Err(); err != nil {
		existingRows.Close()
		return fmt.Errorf("reading existing records: %w", err)
	}
	if err := existingRows.Close(); err != nil {
		return fmt.Errorf("closing existing records: %w", err)
	}

	if preview != nil {
		preview.RecordCount = len(records)
		preview.RecordTypes = make(map[string]int)
		preview.Creates = 0
		preview.Unchanged = 0
		for _, rec := range records {
			preview.RecordTypes[rec.rtype]++
		}
	}
	creates := make([]csvRecord, 0, len(records))
	var conflicts []CSVImportConflict
	for _, rec := range records {
		owner := dns.Fqdn(strings.ToLower(rec.name))
		types := ownerTypes[owner]
		rrsetKey := csvRRsetIdentity(owner, rec.rtype)
		conflicted := false
		for existingTTL := range rrsetTTLs[rrsetKey] {
			if existingTTL != rec.ttl {
				conflicts = append(conflicts, CSVImportConflict{
					Row: rec.row, Owner: owner, Type: rec.rtype, Code: "rrset_ttl_mismatch",
					Message: fmt.Sprintf("RRset TTL is %d; incoming record uses %d", existingTTL, rec.ttl),
				})
				conflicted = true
				break
			}
		}
		if rec.rtype == "CNAME" {
			for existingType := range types {
				if existingType != "CNAME" {
					conflicts = append(conflicts, CSVImportConflict{
						Row: rec.row, Owner: owner, Type: rec.rtype, Code: "cname_exclusive_type",
						Message: fmt.Sprintf("CNAME conflicts with %s at this owner", existingType),
					})
					conflicted = true
					break
				}
			}
		} else if _, hasCNAME := types["CNAME"]; hasCNAME {
			conflicts = append(conflicts, CSVImportConflict{
				Row: rec.row, Owner: owner, Type: rec.rtype, Code: "cname_exclusive_type",
				Message: fmt.Sprintf("%s conflicts with CNAME at this owner", rec.rtype),
			})
			conflicted = true
		}
		if conflicted {
			continue
		}
		identity := csvRecordIdentity(owner, rec.rtype, rec.value, rec.priority, rec.weight, rec.port, rec.flag, rec.tag)
		if previousTTL, duplicate := existing[identity]; duplicate {
			if previousTTL != rec.ttl {
				conflicts = append(conflicts, CSVImportConflict{
					Row: rec.row, Owner: owner, Type: rec.rtype, Code: "duplicate_ttl_mismatch",
					Message: fmt.Sprintf("duplicate value uses TTL %d; incoming record uses %d", previousTTL, rec.ttl),
				})
				continue
			}
			if preview != nil {
				preview.Unchanged++
			}
			continue
		}
		if rec.rtype == "CNAME" && len(types) > 0 {
			conflicts = append(conflicts, CSVImportConflict{
				Row: rec.row, Owner: owner, Type: rec.rtype, Code: "multiple_cname_targets",
				Message: "owner has more than one distinct CNAME target",
			})
			continue
		}
		if types == nil {
			types = make(map[string]struct{})
			ownerTypes[owner] = types
		}
		types[rec.rtype] = struct{}{}
		if rrsetTTLs[rrsetKey] == nil {
			rrsetTTLs[rrsetKey] = make(map[int]struct{})
		}
		rrsetTTLs[rrsetKey][rec.ttl] = struct{}{}
		existing[identity] = rec.ttl
		creates = append(creates, rec)
	}
	if preview != nil {
		preview.Creates = len(creates)
		preview.Valid = len(conflicts) == 0
		preview.Conflicts = append([]CSVImportConflict(nil), conflicts...)
	}
	if len(conflicts) > 0 {
		return &CSVImportConflictError{Conflicts: conflicts}
	}
	if dryRun {
		return nil
	}
	records = creates

	stmt, err := tx.Prepare(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, tag, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, rec := range records {
		_, err := stmt.Exec(rec.id, zoneID, rec.name, rec.rtype, rec.value, rec.ttl, rec.priority, rec.weight, rec.port, rec.flag, rec.tag)
		if err != nil {
			return fmt.Errorf("insert record %s: %w", rec.id, err)
		}
	}
	if len(records) > 0 {
		serial, err := bumpZoneSerialTx(tx, zoneID)
		if err != nil {
			return fmt.Errorf("bumping zone serial: %w", err)
		}
		for _, rec := range records {
			if err := logChangeTx(tx, zoneID, serial, "add", rec.name, rec.rtype, rec.value, rec.ttl,
				rec.priority, rec.weight, rec.port, rec.flag, rec.tag); err != nil {
				return fmt.Errorf("journaling imported record %s: %w", rec.id, err)
			}
		}
		if err := logCurrentSOAStateTx(tx, zoneID, serial); err != nil {
			return fmt.Errorf("journaling updated SOA: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	if len(records) > 0 {
		m.notifyPrimary(zoneID)
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

func csvRecordIdentity(name, rtype, value string, priority, weight, port, flag int, tag string) string {
	return fmt.Sprintf("%q|%q|%q|%d|%d|%d|%d|%q",
		dns.Fqdn(strings.ToLower(name)), strings.ToUpper(rtype), value,
		priority, weight, port, flag, tag)
}

func csvRRsetIdentity(owner, rtype string) string {
	return strings.ToLower(dns.Fqdn(owner)) + "\x00" + strings.ToUpper(rtype)
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
	zone, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	w := csv.NewWriter(&sb)
	if err := w.Write([]string{"name", "type", "value", "ttl", "priority", "weight", "port", "flag", "tag"}); err != nil {
		return nil, err
	}

	for _, r := range records {
		name, ok := zoneRelativeRecordName(r.Name, zone.Name)
		if !ok {
			return nil, fmt.Errorf("record owner %q is outside zone %q", r.Name, zone.Name)
		}
		if err := w.Write([]string{name, r.Type, r.Value, strconv.Itoa(r.TTL), strconv.Itoa(r.Priority),
			strconv.Itoa(r.Weight), strconv.Itoa(r.Port), strconv.Itoa(r.Flag), r.Tag}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}

	return []byte(sb.String()), nil
}

func zoneRelativeRecordName(name, zoneName string) (string, bool) {
	name = dns.Fqdn(strings.ToLower(name))
	zoneName = dns.Fqdn(strings.ToLower(zoneName))
	if name == zoneName {
		return "@", true
	}
	if !recordNameInZone(name, zoneName) {
		return "", false
	}
	relative := strings.TrimSuffix(name, "."+strings.TrimSuffix(zoneName, "."))
	return strings.TrimSuffix(relative, "."), true
}

func recordNameInZone(name, zoneName string) bool {
	name = dns.Fqdn(strings.ToLower(name))
	zoneName = dns.Fqdn(strings.ToLower(zoneName))
	return name == zoneName || strings.HasSuffix(name, "."+zoneName)
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
		return strings.Join([]string{
			quoteDNSCharacterString(v.Flags), quoteDNSCharacterString(v.Service),
			quoteDNSCharacterString(v.Regexp), v.Replacement,
		}, " ")
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

func quoteDNSCharacterString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20 || c >= 0x7f:
			fmt.Fprintf(&b, "\\%03d", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
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
	case *dns.NAPTR:
		priority = int(v.Order)
		weight = int(v.Preference)
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
