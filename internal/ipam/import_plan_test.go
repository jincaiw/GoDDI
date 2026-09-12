package ipam

// A bulk import is the one IPAM operation that can do a lot of damage quickly,
// and the preview exists so an operator can look before it runs. That only
// helps if the preview and the import agree, so most of what follows is about
// the two answers being the same answer:
//
//   - the preview must write nothing at all;
//   - a file the preview refuses must be refused by the import, for the same
//     reasons;
//   - a file the preview accepts must import cleanly -- which is not free,
//     because `ipam_addresses` carries a space-wide unique index as well as a
//     per-subnet one, and an import that only looked at its own subnet would
//     report a row as new and then fail the whole transaction on it.

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

const importHeader = "ip_address,status,mac_address,hostname,owner,device,location,description"

func csvData(lines ...string) []byte {
	return []byte(importHeader + "\n" + strings.Join(lines, "\n") + "\n")
}

func countAddresses(t *testing.T, db *sql.DB, subnetID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = ?`, subnetID).Scan(&n); err != nil {
		t.Fatalf("count addresses: %v", err)
	}
	return n
}

// TestAPreviewReportsWhatAnImportWouldDo. Three outcomes from one file: a new
// address, a change to an existing one, and a row that is already exactly what
// it says. Counting all three as "processed" is what makes a report useless
// for deciding whether to run it.
func TestAPreviewReportsWhatAnImportWouldDo(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad-keep", "sp1", "sn1", "192.0.2.5", address.StatusReserved)
	seedAddress(t, db, "ad-move", "sp1", "sn1", "192.0.2.6", address.StatusAvailable)

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", csvData(
		"192.0.2.10,used,,host-a,,,,",  // new
		"192.0.2.5,reserved,,,,,,",     // already exactly this
		"192.0.2.6,static,,host-b,,,,", // status changes
	))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	if report.Total != 3 || report.Creates != 1 || report.Updates != 1 || report.Unchanged != 1 {
		t.Fatalf("total/create/update/unchanged = %d/%d/%d/%d, want 3/1/1/1",
			report.Total, report.Creates, report.Updates, report.Unchanged)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("errors = %v, want none", report.Errors)
	}
	if report.Applied {
		t.Error("a preview reported itself as applied")
	}

	byAction := map[string]ImportPlan{}
	for _, c := range report.Changes {
		byAction[c.IP] = c
	}
	if got := byAction["192.0.2.10"].Action; got != ImportActionCreate {
		t.Errorf("192.0.2.10 = %q, want create", got)
	}
	if got := byAction["192.0.2.5"].Action; got != ImportActionNoop {
		t.Errorf("192.0.2.5 = %q, want unchanged", got)
	}
	moved := byAction["192.0.2.6"]
	if moved.Action != ImportActionUpdate {
		t.Errorf("192.0.2.6 = %q, want update", moved.Action)
	}
	if len(moved.ChangedFields) != 2 ||
		moved.ChangedFields[0] != "status" || moved.ChangedFields[1] != "hostname" {
		t.Errorf("changed fields = %v, want [status hostname]", moved.ChangedFields)
	}
	if moved.PreviousStatus != string(address.StatusAvailable) {
		t.Errorf("previous status = %q", moved.PreviousStatus)
	}
}

// TestAPreviewWritesNothing. The whole promise is that looking costs nothing.
func TestAPreviewWritesNothing(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad-1", "sp1", "sn1", "192.0.2.5", address.StatusReserved)

	before := countAddresses(t, db, "sn1")

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", csvData(
		"192.0.2.10,used,,host-a,,,,", // would be created
		"192.0.2.5,static,,,,,,",      // would be changed
	))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if report.Creates != 1 || report.Updates != 1 {
		t.Fatalf("the preview did not even plan the work: %+v", report)
	}
	if after := countAddresses(t, db, "sn1"); after != before {
		t.Fatalf("address count went %d -> %d: the preview wrote", before, after)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM ipam_addresses WHERE id = 'ad-1'`).Scan(&status); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != string(address.StatusReserved) {
		t.Fatalf("status = %q after a preview, want it untouched", status)
	}
}

// TestThePreviewAndTheImportGiveTheSameAnswer is the acceptance case: the same
// file, through both paths, producing the same counts and the same write.
func TestThePreviewAndTheImportGiveTheSameAnswer(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad-1", "sp1", "sn1", "192.0.2.5", address.StatusAvailable)

	data := csvData(
		"192.0.2.10,used,,host-a,,,,",
		"192.0.2.5,static,,host-b,,,,",
		"192.0.2.11,reserved,,,,,,",
	)

	preview, err := NewImportExport(db).PreviewAddressesCSV("sn1", data)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	applied, err := NewImportExport(db).ImportAddressesCSV("sn1", data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !applied.Applied {
		t.Error("the import did not report itself as applied")
	}
	if preview.Creates != applied.Creates || preview.Updates != applied.Updates ||
		preview.Unchanged != applied.Unchanged || preview.Total != applied.Total {
		t.Fatalf("preview %d/%d/%d/%d but import %d/%d/%d/%d",
			preview.Total, preview.Creates, preview.Updates, preview.Unchanged,
			applied.Total, applied.Creates, applied.Updates, applied.Unchanged)
	}
	// Two creates plus the changed row.
	if got := countAddresses(t, db, "sn1"); got != 3 {
		t.Fatalf("address count = %d, want 3", got)
	}
}

// TestThePreviewPredictsTheSpaceWideCollision. The address exists in another
// subnet of the same space, so the insert would violate
// `idx_ipam_addresses_space_ip_unique` -- a constraint the per-subnet index
// says nothing about. A preview that missed this would call the row a create
// and the import would then roll the whole file back.
func TestThePreviewPredictsTheSpaceWideCollision(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedSpaceAndSubnet(t, db, "sp1", "sn2", "198.51.100.0/24")
	seedAddress(t, db, "ad-other", "sp1", "sn2", "192.0.2.99", address.StatusUsed)

	data := csvData("192.0.2.99,used,,host-a,,,,")

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", data)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "already exists in subnet sn2") {
		t.Fatalf("errors = %v, want the space-wide collision named", report.Errors)
	}

	// And the import must refuse for the same reason rather than discovering
	// it at insert time.
	before := countAddresses(t, db, "sn1")
	if _, err := NewImportExport(db).ImportAddressesCSV("sn1", data); !errors.Is(err, ErrImportRejected) {
		t.Fatalf("import err = %v, want ErrImportRejected", err)
	}
	if after := countAddresses(t, db, "sn1"); after != before {
		t.Fatalf("address count went %d -> %d after a refusal", before, after)
	}
}

// TestTheSameAddressInAnotherSpaceIsNotACollision is the control for the case
// above: the unique index is per space, so the same address in a different
// space is a different address and must preview as a create.
func TestTheSameAddressInAnotherSpaceIsNotACollision(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedSpaceAndSubnet(t, db, "sp2", "sn2", "192.0.2.0/24")
	seedAddress(t, db, "ad-other", "sp2", "sn2", "192.0.2.99", address.StatusUsed)

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", csvData("192.0.2.99,used,,host-a,,,,"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("errors = %v, want none: the other space is a different address", report.Errors)
	}
	if report.Creates != 1 {
		t.Fatalf("creates = %d, want 1", report.Creates)
	}
}

// TestAFileThatContradictsItselfIsRefused. The second row for one address would
// overwrite the first, and which one won would depend on the order of the lines.
func TestAFileThatContradictsItselfIsRefused(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", csvData(
		"192.0.2.10,used,,,,,,",
		"192.0.2.10,reserved,,,,,,",
	))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "already appears on line 2") {
		t.Fatalf("errors = %v, want the duplicate row named with the first line", report.Errors)
	}
	if report.Creates != 1 {
		t.Fatalf("creates = %d, want the first row counted once", report.Creates)
	}
}

// TestASingleColumnFileImportsInsteadOfDoingNothing.
//
// encoding/csv fixes the expected field count from the header, so a
// single-column file is one where every row has one field. The previous
// version skipped every such row, which turned "import these addresses" into
// an import of nothing: no error, no rows, and a success message. The status
// column keeps its default instead.
func TestASingleColumnFileImportsInsteadOfDoingNothing(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	data := []byte("ip_address\n192.0.2.10\n192.0.2.11\n")

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", data)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if report.Total != 2 || report.Creates != 2 {
		t.Fatalf("a one-column file produced %d row(s) and %d create(s), want 2 and 2",
			report.Total, report.Creates)
	}
	if report.Changes[0].Status != string(address.StatusUsed) {
		t.Errorf("status = %q, want the default %q",
			report.Changes[0].Status, address.StatusUsed)
	}

	if _, err := NewImportExport(db).ImportAddressesCSV("sn1", data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := countAddresses(t, db, "sn1"); got != 2 {
		t.Fatalf("address count = %d, want 2", got)
	}
}

// TestARaggedFileIsRefusedWithTheLineNumber is the other half of the decision
// above: a file whose rows do not all have the header's column count is not
// tolerated row by row. A half-read row imported quietly is worse than a
// refusal, so the reader's complaint is passed on and it names the line.
func TestARaggedFileIsRefusedWithTheLineNumber(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	// The header has eight columns; the second data row has one.
	data := csvData("192.0.2.10,used,,,,,,", "192.0.2.11")

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", data)
	if report != nil {
		t.Fatalf("a ragged file produced a report: %+v", report)
	}
	if err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("err = %v, want the offending line named", err)
	}

	if _, err := NewImportExport(db).ImportAddressesCSV("sn1", data); err == nil {
		t.Fatal("the import accepted a file the preview refused")
	}
	if got := countAddresses(t, db, "sn1"); got != 0 {
		t.Fatalf("%d address(es) were written from a ragged file", got)
	}
}

// TestReimportingTheSameFileChangesNothing. Rows that are already exactly what
// the file says are counted as unchanged and not written, so importing the same
// export twice is a no-op rather than a rewrite of every row's updated_at.
func TestReimportingTheSameFileChangesNothing(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	data := csvData("192.0.2.10,used,,host-a,,,,")

	first, err := NewImportExport(db).ImportAddressesCSV("sn1", data)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.Creates != 1 || first.Unchanged != 0 {
		t.Fatalf("first import = %+v, want one create", first)
	}

	second, err := NewImportExport(db).ImportAddressesCSV("sn1", data)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.Creates != 0 || second.Updates != 0 || second.Unchanged != 1 {
		t.Fatalf("second import = %d create / %d update / %d unchanged, want 0/0/1",
			second.Creates, second.Updates, second.Unchanged)
	}
	if got := countAddresses(t, db, "sn1"); got != 1 {
		t.Fatalf("address count = %d, want 1", got)
	}
	// The row was not rewritten. `datetime('now')` has one-second resolution,
	// so an untouched row and a rewritten one look identical inside a test
	// unless the stored value is moved out of the way first. (The driver hands
	// DATETIME columns back in RFC3339 whatever was written, hence the prefix.)
	if _, err := db.Exec(
		`UPDATE ipam_addresses SET updated_at = '2000-01-01 00:00:00' WHERE subnet_id = 'sn1'`); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if _, err := NewImportExport(db).ImportAddressesCSV("sn1", data); err != nil {
		t.Fatalf("third import: %v", err)
	}
	var updatedAt string
	if err := db.QueryRow(
		`SELECT updated_at FROM ipam_addresses WHERE subnet_id = 'sn1'`).Scan(&updatedAt); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	if !strings.HasPrefix(updatedAt, "2000-01-01") {
		t.Fatalf("updated_at = %q: an unchanged row was rewritten", updatedAt)
	}
}

// TestABadRowStopsTheWholeFile. Partial application is the outcome a bulk
// import must not produce: the operator cannot tell which half landed.
func TestABadRowStopsTheWholeFile(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	data := csvData(
		"192.0.2.10,used,,,,,,",
		"192.0.2.11,nonsense,,,,,,",
	)

	report, err := NewImportExport(db).ImportAddressesCSV("sn1", data)
	if !errors.Is(err, ErrImportRejected) {
		t.Fatalf("err = %v, want ErrImportRejected", err)
	}
	if report == nil || len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "invalid status") {
		t.Fatalf("the refusal does not carry the per-row reason: %+v", report)
	}
	if report.Applied {
		t.Error("a rejected import reported itself as applied")
	}
	if got := countAddresses(t, db, "sn1"); got != 0 {
		t.Fatalf("%d address(es) were written despite the refusal", got)
	}
}

// TestAPreviewOfAMissingSubnetIsNotFound. The space is looked up first, so a
// preview for a subnet that does not exist is reported as that rather than as
// an empty plan a caller might act on.
func TestAPreviewOfAMissingSubnetIsNotFound(t *testing.T) {
	db := newTestDB(t)

	report, err := NewImportExport(db).PreviewAddressesCSV("nope", csvData("192.0.2.10,used,,,,,,"))
	if report != nil {
		t.Fatalf("a report was produced for a subnet that does not exist: %+v", report)
	}
	if !errors.Is(err, subnet.ErrSubnetNotFound) {
		t.Fatalf("err = %v, want ErrSubnetNotFound", err)
	}
}

// TestAnEmptyPlanIsStillAList. The arrays are what a console iterates; null
// would make an empty result look like a missing one.
func TestAnEmptyPlanIsStillAList(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", []byte(importHeader+"\n"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if report.Changes == nil {
		t.Error("changes is null, want an empty list")
	}
	if report.Total != 0 || report.Creates != 0 || report.Updates != 0 {
		t.Errorf("an empty file produced work: %+v", report)
	}
}
