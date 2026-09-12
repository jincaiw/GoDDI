package ipam

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

// importQuerier is satisfied by *sql.DB and *sql.Tx.
//
// A preview runs on the pool, an import runs on the transaction it is about to
// write with, and both go through this interface so the decision is made
// against exactly the state the write will see. Reading the plan on one handle
// and writing on another is how a preview reports a row as new and the insert
// then collides.
type importQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// importRow is one parsed, validated address row.
type importRow struct {
	line        int
	ip          string
	status      string
	mac         string
	hostname    string
	owner       string
	device      string
	location    string
	description string
}

// Import actions.
const (
	ImportActionCreate = "create"
	ImportActionUpdate = "update"
	ImportActionNoop   = "unchanged"
)

// ImportPlan is what happens to one row -- or what would happen to it.
type ImportPlan struct {
	Line           int      `json:"line"`
	IP             string   `json:"ip"`
	Action         string   `json:"action"`
	PreviousStatus string   `json:"previous_status,omitempty"`
	Status         string   `json:"status"`
	ChangedFields  []string `json:"changed_fields,omitempty"`
}

// ImportReport is the outcome of an address import: what it did, or what a
// preview would do.
//
// The two share one type deliberately. A preview shaped differently from the
// outcome is a preview that can disagree with it, and the only reason to offer
// one is that the operator can trust what they are looking at.
type ImportReport struct {
	Type     string `json:"type"`
	ParentID string `json:"parent_id"`

	// Total counts the rows that parsed. Skipped lines are not in it: they
	// carry no address, so counting them would inflate the number the
	// operator compares against their file.
	Total     int `json:"total"`
	Creates   int `json:"creates"`
	Updates   int `json:"updates"`
	Unchanged int `json:"unchanged"`

	// Errors are rows that stop the import. They are collected rather than
	// thrown so that one round trip shows every problem with the file, which
	// is the difference between one fix and six.
	Errors []string `json:"errors"`

	Changes []ImportPlan `json:"changes"`
	// Truncated says Changes is a sample. The counts above are always exact,
	// so a caller cannot read a bounded list as the whole story.
	Truncated bool `json:"truncated"`

	// Applied is false for a preview. A caller cannot mistake one for the
	// other by looking at the body.
	Applied bool `json:"applied"`
}

// maxImportPlanRows bounds the per-row detail in a report.
const maxImportPlanRows = 500

// existingAddress is the part of a stored row an import has to compare against.
type existingAddress struct {
	subnetID    string
	status      string
	mac         string
	hostname    string
	owner       string
	device      string
	location    string
	description string
}

// parseAddressRows parses and validates an address CSV.
//
// Reading is separated from writing so the preview and the import agree about
// the file by construction rather than by two implementations happening to
// agree. The header row is consumed, not validated: the column order is
// positional and always has been.
//
// A file with one column is accepted. `csv.Reader` fixes the field count from
// the header, so a single-column file is a file where every row has one field
// -- and the previous version skipped every such row, which turned "import
// these addresses" into an import of nothing with nothing to show for it. The
// status column keeps its default of "used" when it is absent. A *ragged* file
// is still refused: the reader reports the offending line, and a silently
// half-read row is worse than a refusal.
func parseAddressRows(data []byte) (rows []importRow, errs []string, err error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true

	if _, err := reader.Read(); err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

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

		// Normalise before validating: net.ParseIP rejects the leading-zero
		// IPv4 forms outright, and NormalizeIP also collapses the many valid
		// spellings of an IPv6 address to one. Importing "::ffff:192.0.2.1"
		// and "192.0.2.1" as two addresses is how a bulk import creates
		// duplicates the unique index cannot see.
		ip, err := address.NormalizeIP(csvField(record, 0))
		if err != nil {
			errs = append(errs, fmt.Sprintf("line %d: %v", lineNum, err))
			continue
		}

		status := "used"
		if field := csvField(record, 1); field != "" {
			status = field
		}
		// Validate the status before touching the database, so an invalid row
		// is reported as a bad row rather than as a write that failed.
		if !validIPAMStatuses[status] {
			errs = append(errs, fmt.Sprintf("line %d: invalid status %q", lineNum, status))
			continue
		}

		rows = append(rows, importRow{
			line:        lineNum,
			ip:          ip,
			status:      status,
			mac:         csvField(record, 2),
			hostname:    csvField(record, 3),
			owner:       csvField(record, 4),
			device:      csvField(record, 5),
			location:    csvField(record, 6),
			description: csvField(record, 7),
		})
	}
	return rows, errs, nil
}

// csvField reads a column that may be missing from a short row.
//
// Blank lines never reach here -- encoding/csv skips them -- but a record with
// no fields at all would panic on record[0], and "cannot happen" is not a
// reason to leave a panic in a path that reads operator-supplied input.
func csvField(record []string, i int) string {
	if i < len(record) {
		return record[i]
	}
	return ""
}

// addressImport is a parsed file together with what it would do.
type addressImport struct {
	spaceID string
	// writable holds only the rows that planned cleanly and would change
	// something. The write loop iterates this and not the parsed rows, so a
	// rejected row cannot be written by mistake: it is not in the list.
	writable []importRow
	report   *ImportReport
}

// planAddressRows works out what each row would do, without writing anything.
func (ie *ImportExport) planAddressRows(q importQuerier, subnetID string, data []byte) (*addressImport, error) {
	var spaceID string
	if err := q.QueryRow("SELECT space_id FROM ipam_subnets WHERE id = ?", subnetID).Scan(&spaceID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", subnet.ErrSubnetNotFound, subnetID)
		}
		return nil, fmt.Errorf("read subnet %s: %w", subnetID, err)
	}

	rows, errs, err := parseAddressRows(data)
	if err != nil {
		return nil, err
	}

	existing, err := readSpaceAddresses(q, spaceID)
	if err != nil {
		return nil, err
	}

	report := &ImportReport{
		Type:     "addresses",
		ParentID: subnetID,
		Total:    len(rows),
		Errors:   nonNil(errs),
		Changes:  []ImportPlan{},
	}

	// A file that names one address twice is a file that contradicts itself:
	// the second row would overwrite the first and which one wins would
	// depend on the order of the lines rather than on anything the operator
	// decided.
	firstSeen := make(map[string]int, len(rows))
	var writable []importRow

	for _, row := range rows {
		plan := ImportPlan{
			Line:   row.line,
			IP:     row.ip,
			Status: row.status,
			Action: ImportActionCreate,
		}

		if first, dup := firstSeen[row.ip]; dup {
			report.Errors = append(report.Errors, fmt.Sprintf(
				"line %d: %s already appears on line %d of this file", row.line, row.ip, first))
			continue
		}
		firstSeen[row.ip] = row.line

		if prev, found := existing[row.ip]; found {
			if prev.subnetID != subnetID {
				report.Errors = append(report.Errors, fmt.Sprintf(
					"line %d: %s already exists in subnet %s of this space", row.line, row.ip, prev.subnetID))
				continue
			}
			plan.PreviousStatus = prev.status
			plan.ChangedFields = changedFields(prev, row)
			if len(plan.ChangedFields) == 0 {
				// Nothing to write, and writing anyway would bump updated_at
				// on a row nobody changed -- making a re-import of the same
				// file look like a change to anything watching that column.
				plan.Action = ImportActionNoop
				report.Unchanged++
				planChange(report, plan)
				continue
			}
			plan.Action = ImportActionUpdate
			report.Updates++
		} else {
			report.Creates++
		}

		writable = append(writable, row)
		planChange(report, plan)
	}

	return &addressImport{spaceID: spaceID, writable: writable, report: report}, nil
}

// planChange records one row's outcome, bounded so that a file with a hundred
// thousand lines produces a usable report.
func planChange(report *ImportReport, plan ImportPlan) {
	if len(report.Changes) < maxImportPlanRows {
		report.Changes = append(report.Changes, plan)
		return
	}
	report.Truncated = true
}

// readSpaceAddresses reads every address in a space, keyed by canonical IP.
//
// The key is the address alone, not (subnet, address), because the space-level
// unique index is not the only thing that stops a duplicate:
// `idx_ipam_addresses_space_ip_unique` refuses the same address twice in one
// space whichever subnet it is named in. An import that only looked at its own
// subnet would report a row as new and then fail the whole transaction on the
// insert.
func readSpaceAddresses(q importQuerier, spaceID string) (map[string]existingAddress, error) {
	rows, err := q.Query(`
		SELECT subnet_id, ip_address, status,
		       COALESCE(mac_address, ''), COALESCE(hostname, ''), COALESCE(owner, ''),
		       COALESCE(device, ''), COALESCE(location, ''), COALESCE(description, '')
		FROM ipam_addresses WHERE space_id = ?`, spaceID)
	if err != nil {
		return nil, fmt.Errorf("read addresses of space %s: %w", spaceID, err)
	}
	defer rows.Close()

	out := make(map[string]existingAddress)
	for rows.Next() {
		var ip string
		var e existingAddress
		if err := rows.Scan(&e.subnetID, &ip, &e.status, &e.mac, &e.hostname,
			&e.owner, &e.device, &e.location, &e.description); err != nil {
			return nil, err
		}
		out[ip] = e
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// changedFields lists the columns an import would rewrite.
//
// Distinguishing a no-op from a rewrite is what makes the report usable for
// deciding whether to run the import: "400 rows processed" is not the same
// answer as "400 rows actually change something", and only the second one is
// a reason to be careful.
func changedFields(prev existingAddress, row importRow) []string {
	var out []string
	cmp := func(name, before, after string) {
		if before != after {
			out = append(out, name)
		}
	}
	cmp("status", prev.status, row.status)
	cmp("mac_address", prev.mac, row.mac)
	cmp("hostname", prev.hostname, row.hostname)
	cmp("owner", prev.owner, row.owner)
	cmp("device", prev.device, row.device)
	cmp("location", prev.location, row.location)
	cmp("description", prev.description, row.description)
	return out
}

// PreviewAddressesCSV reports what ImportAddressesCSV would do, without
// writing.
//
// It reads through the same code path the import uses, on the pool rather than
// on a transaction, so the two answers are the same by construction and not by
// two implementations agreeing today.
func (ie *ImportExport) PreviewAddressesCSV(subnetID string, data []byte) (*ImportReport, error) {
	plan, err := ie.planAddressRows(ie.db, subnetID, data)
	if err != nil {
		return nil, err
	}
	return plan.report, nil
}

// ErrImportRejected means the import was refused and nothing was written.
var ErrImportRejected = errors.New("import rejected")

// ImportRejectedError carries the report that explains the refusal, so a
// caller can answer with the per-row reasons instead of a summary of them.
type ImportRejectedError struct {
	Report *ImportReport
}

func (e *ImportRejectedError) Error() string {
	return fmt.Sprintf("%s: %d error(s): %s",
		ErrImportRejected, len(e.Report.Errors), strings.Join(e.Report.Errors, "; "))
}

// Is lets errors.Is(err, ErrImportRejected) succeed.
func (e *ImportRejectedError) Is(target error) bool { return target == ErrImportRejected }
