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
	"strings"

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
// previous implementation. Each row is also validated up-front (IP
// parseability, status allowed) so the transaction can fail early with a
// clear error.
func (ie *ImportExport) ImportAddressesCSV(subnetID string, data []byte) error {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true

	// Skip header row.
	if _, err := reader.Read(); err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	tx, err := ie.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var importErrors []string
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

		ip := record[0]
		status := "used"
		if len(record) > 1 && record[1] != "" {
			status = record[1]
		}

		// Validate the IP address and the status value before touching the
		// database. This keeps invalid rows from partially mutating state
		// when the surrounding transaction is rolled back.
		if net.ParseIP(ip) == nil {
			importErrors = append(importErrors, fmt.Sprintf("line %d: invalid IP %q", lineNum, ip))
			continue
		}
		if !validIPAMStatuses[status] {
			importErrors = append(importErrors, fmt.Sprintf("line %d: invalid status %q", lineNum, status))
			continue
		}

		macAddress := ""
		if len(record) > 2 {
			macAddress = record[2]
		}
		hostname := ""
		if len(record) > 3 {
			hostname = record[3]
		}
		owner := ""
		if len(record) > 4 {
			owner = record[4]
		}
		device := ""
		if len(record) > 5 {
			device = record[5]
		}
		location := ""
		if len(record) > 6 {
			location = record[6]
		}
		description := ""
		if len(record) > 7 {
			description = record[7]
		}

		// Check if address already exists. We do this inside the
		// transaction so a concurrent AllocateIP cannot race with us.
		var existingID string
		err = tx.QueryRow(`SELECT id FROM ipam_addresses WHERE subnet_id = ? AND ip_address = ?`, subnetID, ip).Scan(&existingID)
		if err == nil {
			// Update existing.
			if _, err := tx.Exec(`
				UPDATE ipam_addresses SET status=?, mac_address=?, hostname=?, owner=?,
					device=?, location=?, description=?, updated_at=datetime('now')
				WHERE id=?`,
				status, nullIfEmpty(macAddress), nullIfEmpty(hostname),
				nullIfEmpty(owner), nullIfEmpty(device), nullIfEmpty(location),
				nullIfEmpty(description), existingID); err != nil {
				importErrors = append(importErrors, fmt.Sprintf("line %d: failed to update %s: %v", lineNum, ip, err))
			}
		} else if err == sql.ErrNoRows {
			// Create new.
			id := uuid.New().String()
			if _, err := tx.Exec(`
				INSERT INTO ipam_addresses (id, subnet_id, ip_address, status, mac_address, hostname,
					owner, device, location, description, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
				id, subnetID, ip, status, nullIfEmpty(macAddress), nullIfEmpty(hostname),
				nullIfEmpty(owner), nullIfEmpty(device), nullIfEmpty(location), nullIfEmpty(description)); err != nil {
				importErrors = append(importErrors, fmt.Sprintf("line %d: failed to insert %s: %v", lineNum, ip, err))
			}
		} else {
			importErrors = append(importErrors, fmt.Sprintf("line %d: failed to query %s: %v", lineNum, ip, err))
		}
	}

	if len(importErrors) > 0 {
		// Roll back: the deferred Rollback will undo every change made
		// inside this transaction.
		return fmt.Errorf("import rolled back due to %d error(s): %s", len(importErrors), strings.Join(importErrors, "; "))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit import transaction: %w", err)
	}
	return nil
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
