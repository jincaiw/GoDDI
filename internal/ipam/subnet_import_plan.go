package ipam

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// SubnetImportPlan describes one subnet row and is shared by preview and apply.
type SubnetImportPlan struct {
	Line          int      `json:"line"`
	Name          string   `json:"name"`
	CIDR          string   `json:"cidr"`
	Action        string   `json:"action"`
	PreviousCIDR  string   `json:"previous_cidr,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// SubnetImportReport is the dry-run or applied result of a subnet import.
type SubnetImportReport struct {
	Type      string             `json:"type"`
	ParentID  string             `json:"parent_id"`
	Total     int                `json:"total"`
	Creates   int                `json:"creates"`
	Updates   int                `json:"updates"`
	Unchanged int                `json:"unchanged"`
	Errors    []string           `json:"errors"`
	Changes   []SubnetImportPlan `json:"changes"`
	Truncated bool               `json:"truncated"`
	Applied   bool               `json:"applied"`
}

type subnetImportRow struct {
	line        int
	name        string
	cidr        string
	vlanID      *int
	location    string
	description string
}

type existingSubnet struct {
	id          string
	name        string
	cidr        string
	vlanID      sql.NullInt64
	location    string
	description string
}

type subnetImport struct {
	rows   []subnetImportRow
	report *SubnetImportReport
}

const maxSubnetImportPlanRows = 500

func parseSubnetImportRows(data []byte) ([]subnetImportRow, []string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	if _, err := reader.Read(); err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	var rows []subnetImportRow
	var errs []string
	lineNum := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("CSV parse error at line %d: %w", lineNum+1, err)
		}
		lineNum++
		if len(record) < 2 || (strings.TrimSpace(record[0]) == "" && strings.TrimSpace(record[1]) == "") {
			continue
		}

		name := strings.TrimSpace(record[0])
		cidr := strings.TrimSpace(record[1])
		if name == "" {
			errs = append(errs, fmt.Sprintf("line %d: subnet name is required", lineNum))
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("line %d: invalid CIDR %q", lineNum, cidr))
			continue
		}

		var vlanID *int
		if value := field(record, 2); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 || parsed > 4095 {
				errs = append(errs, fmt.Sprintf("line %d: invalid VLAN ID %q", lineNum, value))
				continue
			}
			vlanID = &parsed
		}
		rows = append(rows, subnetImportRow{
			line: lineNum, name: name, cidr: network.String(), vlanID: vlanID,
			location: field(record, 3), description: field(record, 4),
		})
	}
	return rows, errs, nil
}

func field(record []string, index int) string {
	if index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func (ie *ImportExport) planSubnets(q importQuerier, spaceID string, data []byte) (*subnetImport, error) {
	var exists int
	if err := q.QueryRow("SELECT COUNT(*) FROM ipam_spaces WHERE id = ?", spaceID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("verify space: %w", err)
	}
	if exists == 0 {
		return nil, fmt.Errorf("space not found: %s", spaceID)
	}

	rows, parseErrors, err := parseSubnetImportRows(data)
	if err != nil {
		return nil, err
	}
	existing, err := readExistingSubnets(q, spaceID)
	if err != nil {
		return nil, err
	}

	report := &SubnetImportReport{
		Type: "subnets", ParentID: spaceID, Total: len(rows),
		Errors: nonNil(parseErrors), Changes: []SubnetImportPlan{},
	}
	seenNames := make(map[string]int, len(rows))
	seenNetworks := make([]*net.IPNet, 0, len(rows))
	var writable []subnetImportRow

	for _, row := range rows {
		plan := SubnetImportPlan{Line: row.line, Name: row.name, CIDR: row.cidr, Action: ImportActionCreate}
		if first, duplicate := seenNames[row.name]; duplicate {
			report.Errors = append(report.Errors, fmt.Sprintf("line %d: subnet %q already appears on line %d", row.line, row.name, first))
			continue
		}
		seenNames[row.name] = row.line
		_, network, _ := net.ParseCIDR(row.cidr)

		if prev, found := existing[row.name]; found {
			if prev.cidr != row.cidr || !sameOptionalInt(prev.vlanID, row.vlanID) || prev.location != row.location || prev.description != row.description {
				report.Errors = append(report.Errors, fmt.Sprintf("line %d: subnet %q already exists with different fields", row.line, row.name))
				continue
			}
			plan.Action = ImportActionNoop
			plan.PreviousCIDR = prev.cidr
			report.Unchanged++
			appendSubnetPlan(report, plan)
			continue
		}

		if err := subnetOverlaps(network, existing, seenNetworks); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("line %d: %v", row.line, err))
			continue
		}
		report.Creates++
		writable = append(writable, row)
		seenNetworks = append(seenNetworks, network)
		appendSubnetPlan(report, plan)
	}
	return &subnetImport{rows: writable, report: report}, nil
}

func readExistingSubnets(q importQuerier, spaceID string) (map[string]existingSubnet, error) {
	rows, err := q.Query(`SELECT id, name, cidr, vlan_id, COALESCE(location,''), COALESCE(description,'') FROM ipam_subnets WHERE space_id = ?`, spaceID)
	if err != nil {
		return nil, fmt.Errorf("read existing subnets: %w", err)
	}
	defer rows.Close()
	result := make(map[string]existingSubnet)
	for rows.Next() {
		var item existingSubnet
		if err := rows.Scan(&item.id, &item.name, &item.cidr, &item.vlanID, &item.location, &item.description); err != nil {
			return nil, fmt.Errorf("scan existing subnet: %w", err)
		}
		result[item.name] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing subnets: %w", err)
	}
	return result, nil
}

func subnetOverlaps(network *net.IPNet, existing map[string]existingSubnet, planned []*net.IPNet) error {
	for _, item := range existing {
		_, other, err := net.ParseCIDR(item.cidr)
		if err != nil {
			return fmt.Errorf("existing subnet %q has invalid CIDR %q", item.name, item.cidr)
		}
		if network.Contains(other.IP) || other.Contains(network.IP) {
			return fmt.Errorf("CIDR %q overlaps existing subnet %q (%s)", network.String(), item.name, item.cidr)
		}
	}
	for _, other := range planned {
		if network.Contains(other.IP) || other.Contains(network.IP) {
			return fmt.Errorf("CIDR %q overlaps another row in this file", network.String())
		}
	}
	return nil
}

func sameOptionalInt(existing sql.NullInt64, incoming *int) bool {
	if !existing.Valid && incoming == nil {
		return true
	}
	return existing.Valid && incoming != nil && existing.Int64 == int64(*incoming)
}

func appendSubnetPlan(report *SubnetImportReport, plan SubnetImportPlan) {
	if len(report.Changes) < maxSubnetImportPlanRows {
		report.Changes = append(report.Changes, plan)
	} else {
		report.Truncated = true
	}
}

func (ie *ImportExport) ImportSubnetsCSVReport(spaceID string, data []byte) (*SubnetImportReport, error) {
	return ie.applySubnets(spaceID, data)
}

func (ie *ImportExport) PreviewSubnetsCSV(spaceID string, data []byte) (*SubnetImportReport, error) {
	planned, err := ie.planSubnets(ie.db, spaceID, data)
	if err != nil {
		return nil, err
	}
	return planned.report, nil
}

func (ie *ImportExport) applySubnets(spaceID string, data []byte) (*SubnetImportReport, error) {
	tx, err := ie.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	planned, err := ie.planSubnets(tx, spaceID, data)
	if err != nil {
		return nil, err
	}
	if len(planned.report.Errors) > 0 {
		return planned.report, &ImportRejectedError{SubnetReport: planned.report}
	}
	for _, row := range planned.rows {
		var vlan any
		if row.vlanID != nil {
			vlan = *row.vlanID
		}
		if _, err := tx.Exec(`INSERT INTO ipam_subnets (id, space_id, name, cidr, vlan_id, location, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`, uuid.New().String(), spaceID, row.name, row.cidr, vlan, row.location, row.description); err != nil {
			return planned.report, fmt.Errorf("line %d: failed to import subnet %q: %w", row.line, row.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return planned.report, fmt.Errorf("failed to commit subnet import: %w", err)
	}
	planned.report.Applied = true
	return planned.report, nil
}
