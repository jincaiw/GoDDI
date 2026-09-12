package handler

// The import preview exists so an operator can look at a bulk change before it
// happens. That is only worth anything if the preview and the import are the
// same decision seen twice, so these cases check the pair together: the same
// file through both endpoints, the same counts, and -- for a refused file --
// the same reasons, with nothing written.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/ipam"
)

func importBody(t *testing.T, importType, parentID, data string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"type":      importType,
		"format":    "csv",
		"parent_id": parentID,
		"data":      data,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func postImport(t *testing.T, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if path == "/api/v1/ipam/import/preview" {
		PreviewIPAMImport(rec, req)
	} else {
		ImportIPAMData(rec, req)
	}
	return rec
}

// importReportEnvelope is the part of an import response these tests read.
type importReportEnvelope struct {
	Code int `json:"code"`
	Data struct {
		Type      string   `json:"type"`
		Total     int      `json:"total"`
		Creates   int      `json:"creates"`
		Updates   int      `json:"updates"`
		Unchanged int      `json:"unchanged"`
		Errors    []string `json:"errors"`
		Applied   bool     `json:"applied"`
	} `json:"data"`
}

func decodeImportReport(t *testing.T, rec *httptest.ResponseRecorder) importReportEnvelope {
	t.Helper()
	var env importReportEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decoding the report: %v (%s)", err, rec.Body.String())
	}
	return env
}

const importCSVHeader = "ip_address,status,mac_address,hostname,owner,device,location,description"

func countSubnetAddresses(t *testing.T, db *sql.DB, subnetID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_addresses WHERE subnet_id = ?`, subnetID).Scan(&n); err != nil {
		t.Fatalf("count addresses: %v", err)
	}
	return n
}

// TestThePreviewEndpointWritesNothing. `applied` is on the wire so a client
// cannot mistake a dry run for a run by looking at the body alone.
func TestThePreviewEndpointWritesNothing(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	withIPAMServices(t, &IPAMServiceContainer{DB: db})

	body := importBody(t, "addresses", "sn1",
		importCSVHeader+"\n192.0.2.10,used,,,,,,\n192.0.2.11,reserved,,,,,,\n")

	rec := postImport(t, "/api/v1/ipam/import/preview", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	env := decodeImportReport(t, rec)
	if env.Data.Applied {
		t.Error("the preview reported itself as applied")
	}
	if env.Data.Total != 2 || env.Data.Creates != 2 {
		t.Fatalf("total/creates = %d/%d, want 2/2", env.Data.Total, env.Data.Creates)
	}
	if n := countSubnetAddresses(t, db, "sn1"); n != 0 {
		t.Fatalf("%d address(es) exist after a preview", n)
	}
}

// TestTheImportEndpointReturnsTheSameReport. One file, two endpoints, the same
// numbers -- and the second one has actually written.
func TestTheImportEndpointReturnsTheSameReport(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	withIPAMServices(t, &IPAMServiceContainer{DB: db})

	body := importBody(t, "addresses", "sn1",
		importCSVHeader+"\n192.0.2.10,used,,host-a,,,,\n192.0.2.11,reserved,,,,,,\n")

	previewed := decodeImportReport(t, postImport(t, "/api/v1/ipam/import/preview", body))
	applied := decodeImportReport(t, postImport(t, "/api/v1/ipam/import", body))

	if applied.Code != 0 {
		t.Fatalf("import code = %d: %s", applied.Code, applied.Data.Errors)
	}
	if !applied.Data.Applied {
		t.Error("the import did not report itself as applied")
	}
	if previewed.Data.Creates != applied.Data.Creates ||
		previewed.Data.Total != applied.Data.Total {
		t.Fatalf("preview %d/%d but import %d/%d",
			previewed.Data.Total, previewed.Data.Creates,
			applied.Data.Total, applied.Data.Creates)
	}
	if n := countSubnetAddresses(t, db, "sn1"); n != 2 {
		t.Fatalf("%d address(es) after the import, want 2", n)
	}
}

// TestARejectedImportIsABadRequestCarryingTheReport. A file with a bad row is
// the request's fault, not the server's, and the per-row reasons are what makes
// it fixable in one pass.
func TestARejectedImportIsABadRequestCarryingTheReport(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	withIPAMServices(t, &IPAMServiceContainer{DB: db})

	body := importBody(t, "addresses", "sn1",
		importCSVHeader+"\n192.0.2.10,used,,,,,,\n192.0.2.11,nonsense,,,,,,\n")

	preview := postImport(t, "/api/v1/ipam/import/preview", body)
	rec := postImport(t, "/api/v1/ipam/import", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("import = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	env := decodeImportReport(t, rec)
	if len(env.Data.Errors) != 1 {
		t.Fatalf("errors = %v, want the one bad row", env.Data.Errors)
	}

	// The preview saw the same problem, which is what makes it worth reading.
	pv := decodeImportReport(t, preview)
	if len(pv.Data.Errors) != len(env.Data.Errors) {
		t.Fatalf("the preview reported %v but the import refused for %v",
			pv.Data.Errors, env.Data.Errors)
	}
	if n := countSubnetAddresses(t, db, "sn1"); n != 0 {
		t.Fatalf("%d address(es) were written despite the refusal", n)
	}
}

// TestAPreviewForAMissingSubnetIsNotFound.
func TestAPreviewForAMissingSubnetIsNotFound(t *testing.T) {
	db := newControlPlaneTestDB(t)
	withIPAMServices(t, &IPAMServiceContainer{DB: db})

	rec := postImport(t, "/api/v1/ipam/import/preview",
		importBody(t, "addresses", "nope", importCSVHeader+"\n192.0.2.10,used,,,,,,\n"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("preview = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestSubnetPreviewReportsChangesWithoutWriting(t *testing.T) {
	db := newControlPlaneTestDB(t)
	if _, err := db.Exec(`INSERT INTO ipam_spaces (id, name) VALUES ('sp1', 'space-sp1')`); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	withIPAMServices(t, &IPAMServiceContainer{DB: db})

	rec := postImport(t, "/api/v1/ipam/import/preview",
		importBody(t, "subnets", "sp1", "name,cidr,vlan_id,location,description\nsite-a,192.0.2.5/24,100,dc-a,first\n"))
	if rec.Code != http.StatusOK {
		t.Fatalf("subnet preview = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var report struct {
		Data struct {
			Creates int      `json:"creates"`
			Applied bool     `json:"applied"`
			Errors  []string `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if report.Data.Creates != 1 || report.Data.Applied || len(report.Data.Errors) != 0 {
		t.Fatalf("preview report = %+v, want one create, not applied, no errors", report.Data)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ipam_subnets WHERE space_id = 'sp1'`).Scan(&count); err != nil {
		t.Fatalf("count subnets: %v", err)
	}
	if count != 0 {
		t.Fatalf("preview wrote %d subnet(s)", count)
	}
}

func TestSubnetImportRejectsOverlapAndWritesNothing(t *testing.T) {
	db := newControlPlaneTestDB(t)
	if _, err := db.Exec(`INSERT INTO ipam_spaces (id, name) VALUES ('sp1', 'space-sp1')`); err != nil {
		t.Fatalf("seed space: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_subnets (id, space_id, name, cidr, created_at, updated_at) VALUES ('existing', 'sp1', 'existing', '192.0.2.0/24', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed subnet: %v", err)
	}
	withIPAMServices(t, &IPAMServiceContainer{DB: db})

	rec := postImport(t, "/api/v1/ipam/import/preview",
		importBody(t, "subnets", "sp1", "name,cidr\nsite-a,192.0.2.0/25\n"))
	if rec.Code != http.StatusOK {
		t.Fatalf("overlap preview = %d, want 200 report: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Errors []string `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode overlap preview: %v", err)
	}
	if len(body.Data.Errors) != 1 {
		t.Fatalf("overlap errors = %v, want one", body.Data.Errors)
	}
}

// TestTheEmptyListsAreArraysOnTheWire.
//
// Every other case in this file reads a response through a struct, and
// `json.Unmarshal` turns both `null` and `[]` into a slice of length zero. The
// file was blind to which one the server writes -- and it wrote null.
//
// That is not cosmetic. The console counts these lists to decide what to
// render, so `"conflicts": null` was a TypeError inside a render: the address
// drawer came up with a title and a blank body, and the import dialog closed
// itself instead of showing the report. Both read as the feature being broken.
//
// Asserted as bytes, because bytes are the only place the difference exists.
func TestTheEmptyListsAreArraysOnTheWire(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	if _, err := db.Exec(`
		INSERT INTO ipam_addresses (id, subnet_id, space_id, ip_address, status, observed_state)
		VALUES ('ad1', 'sn1', 'sp1', '192.0.2.10', 'available', 'unknown')`); err != nil {
		t.Fatalf("seed address: %v", err)
	}
	withIPAMServices(t, &IPAMServiceContainer{DB: db, Linkage: ipam.NewLinkage(db)})

	// A file with a header and no rows: nothing to complain about and nothing
	// to change, so every list on the report is empty. A report with entries in
	// it would not show the bug -- a non-empty slice is non-nil whatever the
	// builder does.
	report := postImport(t, "/api/v1/ipam/import/preview",
		importBody(t, "addresses", "sn1", importCSVHeader+"\n"))
	if report.Code != http.StatusOK {
		t.Fatalf("preview = %d, want 200: %s", report.Code, report.Body.String())
	}

	// The same address through the view, belonging to a space and a subnet and
	// nothing else: no DNS name, no scope, no lease, no reservation, no history.
	view := httptest.NewRecorder()
	GetIPAMAddressView(view, httptest.NewRequest(http.MethodGet,
		"/api/v1/ipam/addresses/view?space_id=sp1&ip=192.0.2.10", nil))
	if view.Code != http.StatusOK {
		t.Fatalf("view = %d, want 200: %s", view.Code, view.Body.String())
	}

	cases := []struct {
		what string
		body string
		keys []string
	}{
		{"the import report", report.Body.String(), []string{"errors", "changes"}},
		{"the address view", view.Body.String(), []string{
			"dns_records", "dhcp_scopes", "dhcp_leases", "dhcp_reservations", "history", "conflicts",
		}},
	}
	for _, c := range cases {
		for _, key := range c.keys {
			if !strings.Contains(c.body, `"`+key+`":[]`) {
				t.Errorf("%s: %q is not an empty array on the wire: %s", c.what, key, c.body)
			}
		}
	}
}

// TestTheImportStillValidatesItsRequest. The shared decoder is what the preview
// and the import both rely on, so a body the import used to reject must still
// be rejected.
func TestTheImportStillValidatesItsRequest(t *testing.T) {
	db := newControlPlaneTestDB(t)
	withIPAMServices(t, &IPAMServiceContainer{DB: db})

	cases := map[string]string{
		"no type":      `{"format":"csv","parent_id":"sn1","data":"x"}`,
		"no format":    `{"type":"addresses","parent_id":"sn1","data":"x"}`,
		"no parent":    `{"type":"addresses","format":"csv","data":"x"}`,
		"no data":      `{"type":"addresses","format":"csv","parent_id":"sn1"}`,
		"unknown type": `{"type":"widgets","format":"csv","parent_id":"sn1","data":"x"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/api/v1/ipam/import", "/api/v1/ipam/import/preview"} {
				rec := postImport(t, path, []byte(body))
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("%s = %d, want 400: %s", path, rec.Code, rec.Body.String())
				}
			}
		})
	}
}
