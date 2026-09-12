package ipam

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/ipam/address"
)

// ImportExport handles IPAM data import and export.
type ImportExport struct {
	db      *sql.DB
	addrMgr *address.Manager
}

// NewImportExport creates a new import/export handler.
func NewImportExport(db *sql.DB) *ImportExport {
	return &ImportExport{
		db:      db,
		addrMgr: address.NewManager(db),
	}
}

// validIPAMStatuses is the set of values that the CSV status column may
// take. We accept anything outside this set only if it is a non-empty
// string and treat unknown values as a validation error.
var validIPAMStatuses = map[string]bool{
	string(address.StatusAvailable): true,
	string(address.StatusUsed):      true,
	string(address.StatusReserved):  true,
	string(address.StatusDHCP):      true,
	string(address.StatusStatic):    true,
	string(address.StatusGateway):   true,
	string(address.StatusExcluded):  true,
	string(address.StatusConflict):  true,
	string(address.StatusUnknown):   true,
}

// ImportAddressesCSV imports IP addresses from CSV data into a subnet.
// CSV format: ip_address,status,mac_address,hostname,owner,device,location,description
//
// The entire import runs inside a single transaction so that either all rows
// are written or none are, eliminating the partial-import ambiguity of the
// previous implementation. Every row is decided before anything is written
// (see planAddressRows), so a file with a bad row is refused with all the
// reasons at once instead of failing on the first insert.
//
// The returned report is the same type PreviewAddressesCSV returns, with
// Applied set. When rows are rejected the report is returned alongside the
// error, because the refusal is explained per row and not in a sentence.
func (ie *ImportExport) ImportAddressesCSV(subnetID string, data []byte) (*ImportReport, error) {
	tx, err := ie.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// The plan is decided on the transaction handle, never on the pool:
	// reading through the pool while this transaction is open would wait for
	// the connection the transaction holds, with no error and no timeout.
	plan, err := ie.planAddressRows(tx, subnetID, data)
	if err != nil {
		return nil, err
	}
	if len(plan.report.Errors) > 0 {
		return plan.report, &ImportRejectedError{Report: plan.report}
	}

	// Only rows that planned cleanly are in writable, and rows that change
	// nothing are not in it at all.
	for _, row := range plan.writable {
		// One statement instead of a read followed by a write. The upsert is
		// resolved by the (subnet_id, ip_address) index, so a row that appears
		// between the two halves of a read-then-write cannot be double-added.
		if _, err := tx.Exec(`
			INSERT INTO ipam_addresses
				(id, subnet_id, space_id, ip_address, status, mac_address, hostname,
				 owner, device, location, description, observed_state,
				 allocated_at, allocated_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'unknown',
				datetime('now'), 'import', datetime('now'), datetime('now'))
			ON CONFLICT(subnet_id, ip_address) DO UPDATE SET
				status      = excluded.status,
				mac_address = excluded.mac_address,
				hostname    = excluded.hostname,
				owner       = excluded.owner,
				device      = excluded.device,
				location    = excluded.location,
				description = excluded.description,
				updated_at  = datetime('now')`,
			uuid.New().String(), subnetID, plan.spaceID, row.ip, row.status,
			nullIfEmpty(row.mac), nullIfEmpty(row.hostname), nullIfEmpty(row.owner),
			nullIfEmpty(row.device), nullIfEmpty(row.location), nullIfEmpty(row.description)); err != nil {
			return plan.report, fmt.Errorf("line %d: failed to import %s: %w", row.line, row.ip, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return plan.report, fmt.Errorf("failed to commit import transaction: %w", err)
	}
	plan.report.Applied = true
	return plan.report, nil
}

// ExportAddressesCSV exports IP addresses from a subnet as CSV data.
func (ie *ImportExport) ExportAddressesCSV(subnetID string) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header.
	writer.Write([]string{"ip_address", "status", "mac_address", "hostname", "owner", "device", "location", "description"})

	// Query all addresses for the subnet.
	rows, err := ie.db.Query(`
		SELECT ip_address, status, COALESCE(mac_address,''), COALESCE(hostname,''),
			COALESCE(owner,''), COALESCE(device,''), COALESCE(location,''), COALESCE(description,'')
		FROM ipam_addresses WHERE subnet_id = ? ORDER BY ip_address ASC`, subnetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ip, status, mac, hostname, owner, device, location, description string
		if err := rows.Scan(&ip, &status, &mac, &hostname, &owner, &device, &location, &description); err != nil {
			slog.Warn("ipam: failed to scan address row during export", "error", err)
			continue
		}
		writer.Write([]string{ip, status, mac, hostname, owner, device, location, description})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating addresses for export: %w", err)
	}

	writer.Flush()
	return buf.Bytes(), nil
}

// ImportSubnetsCSV imports subnets from CSV data into a space.
// CSV format: name,cidr,vlan_id,location,description
func (ie *ImportExport) ImportSubnetsCSV(spaceID string, data []byte) error {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true

	// Skip header row.
	if _, err := reader.Read(); err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	lineNum := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("CSV parse error at line %d: %w", lineNum+1, err)
		}
		lineNum++

		if len(record) < 2 {
			continue
		}

		name := record[0]
		cidr := record[1]

		// Validate the CIDR before touching the database; an invalid subnet
		// in the store breaks usage stats and DHCP scope generation later.
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("line %d: invalid CIDR %q for subnet %q", lineNum, cidr, name)
		}

		vlanID := ""
		if len(record) > 2 {
			vlanID = record[2]
		}
		location := ""
		if len(record) > 3 {
			location = record[3]
		}
		description := ""
		if len(record) > 4 {
			description = record[4]
		}

		id := uuid.New().String()
		var vlanIDVal interface{}
		if vlanID != "" {
			vlanInt, err := strconv.Atoi(vlanID)
			if err != nil {
				return fmt.Errorf("line %d: invalid VLAN ID %q for subnet %q", lineNum, vlanID, name)
			}
			vlanIDVal = vlanInt
		}

		if _, err = ie.db.Exec(`
			INSERT OR IGNORE INTO ipam_subnets (id, space_id, name, cidr, vlan_id, location, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			id, spaceID, name, cidr, vlanIDVal, location, description); err != nil {
			return fmt.Errorf("line %d: failed to import subnet %q: %w", lineNum, name, err)
		}
	}

	return nil
}

// ExportSubnetsCSV exports subnets from a space as CSV data.
func (ie *ImportExport) ExportSubnetsCSV(spaceID string) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header.
	writer.Write([]string{"name", "cidr", "vlan_id", "location", "description"})

	rows, err := ie.db.Query(`
		SELECT name, cidr, COALESCE(CAST(vlan_id AS TEXT),''), COALESCE(location,''), COALESCE(description,'')
		FROM ipam_subnets WHERE space_id = ? ORDER BY created_at ASC`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name, cidr, vlanID, location, description string
		if err := rows.Scan(&name, &cidr, &vlanID, &location, &description); err != nil {
			slog.Warn("ipam: failed to scan subnet row during export", "error", err)
			continue
		}
		writer.Write([]string{name, cidr, vlanID, location, description})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subnets for export: %w", err)
	}

	writer.Flush()
	return buf.Bytes(), nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
