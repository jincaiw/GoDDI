package handler

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func TestListIPAMRetirementAuditReportsPreservedChildren(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := `
	CREATE TABLE ipam_spaces (id TEXT PRIMARY KEY, name TEXT NOT NULL, lifecycle_state TEXT NOT NULL DEFAULT 'active', retired_at DATETIME, retired_by TEXT, retire_reason TEXT);
	CREATE TABLE ipam_subnets (id TEXT PRIMARY KEY, space_id TEXT, name TEXT NOT NULL, cidr TEXT, lifecycle_state TEXT NOT NULL DEFAULT 'active', retired_at DATETIME, retired_by TEXT, retire_reason TEXT);
	CREATE TABLE ipam_addresses (id TEXT PRIMARY KEY, subnet_id TEXT, ip_address TEXT, status TEXT);
	CREATE TABLE ipam_history (id TEXT PRIMARY KEY, subnet_id TEXT, ip_address TEXT, action TEXT);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_spaces (id,name,lifecycle_state,retired_at,retired_by,retire_reason) VALUES ('sp-retired','old-retired-sp','retired','2026-11-17 00:00:00','tester','audit')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_subnets (id,space_id,name,cidr,lifecycle_state,retired_at,retired_by,retire_reason) VALUES ('sn-retired','sp-retired','old-lan','192.0.2.0/24','retired','2026-11-17 00:00:00','tester','audit')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_addresses (id,subnet_id,ip_address,status) VALUES ('a1','sn-retired','192.0.2.10','dhcp')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_history (id,subnet_id,ip_address,action) VALUES ('h1','sn-retired','192.0.2.10','allocate')`); err != nil {
		t.Fatal(err)
	}
	prev := IPAMServices
	defer func() { IPAMServices = prev }()
	IPAMServices = &IPAMServiceContainer{DB: db}

	rec := httptest.NewRecorder()
	ListIPAMRetirementAudit(rec, httptest.NewRequest("GET", "/api/v1/ipam/retirement-audit", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data []RetirementAudit `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("audit entries = %d, want retired space and subnet", len(body.Data))
	}
	var subnet RetirementAudit
	for _, item := range body.Data {
		if item.Kind == "subnet" {
			subnet = item
		}
	}
	if subnet.ID != "sn-retired" || subnet.RetiredBy != "tester" || subnet.Allocated != 1 || subnet.History != 1 {
		t.Fatalf("subnet audit = %+v", subnet)
	}
}
