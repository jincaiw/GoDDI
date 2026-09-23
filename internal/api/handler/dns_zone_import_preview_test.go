package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/dns/zone"
)

func TestCSVZoneImportDryRunReportsCountsWithoutWriting(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)
	dnsZone, err := cachedZoneMgr.CreateZone(zone.ZoneOptions{Name: "preview.example.test", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"format":"csv","dry_run":true,"content":"name,type,value,ttl,priority,weight,port,flag,tag\nwww,A,192.0.2.25,300,,,,,\n"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones/"+dnsZone.ID+"/import", strings.NewReader(body))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", dnsZone.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	ImportZoneFile(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dry-run status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ZoneID      string                   `json:"zone_id"`
			DryRun      bool                     `json:"dry_run"`
			Valid       bool                     `json:"valid"`
			RecordCount int                      `json:"record_count"`
			Creates     int                      `json:"creates"`
			Unchanged   int                      `json:"unchanged"`
			RecordTypes map[string]int           `json:"record_types"`
			Conflicts   []zone.CSVImportConflict `json:"conflicts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run response: %v (%s)", err, rec.Body.String())
	}
	if envelope.Data.ZoneID != dnsZone.ID || !envelope.Data.DryRun || !envelope.Data.Valid || envelope.Data.RecordCount != 1 ||
		envelope.Data.Creates != 1 || envelope.Data.Unchanged != 0 || envelope.Data.RecordTypes["A"] != 1 {
		t.Fatalf("dry-run response = %+v", envelope.Data)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE zone_id = ?`, dnsZone.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("dry-run wrote %d records", count)
	}
}

func TestCSVZoneImportDryRunReturnsAllConflictRowsWithoutWriting(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	withDNSRecordServices(t, db)
	dnsZone, err := cachedZoneMgr.CreateZone(zone.ZoneOptions{Name: "conflicts.example.test", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}
	ttl := 300
	if _, err := cachedRecordMgr.CreateRecord(dnsZone.ID, zone.RecordOptions{
		Name: "taken", Type: "A", Value: "192.0.2.1", TTL: &ttl,
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"format":"csv","dry_run":true,"content":"name,type,value,ttl,priority,weight,port,flag,tag\ntaken,A,192.0.2.2,600,,,,,\ntaken,CNAME,target.example.test.,300,,,,,\nnew,A,192.0.2.3,300,,,,,\nnew,CNAME,target.example.test.,300,,,,,\nalias,CNAME,one.example.test.,300,,,,,\nalias,CNAME,two.example.test.,300,,,,,\n"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones/"+dnsZone.ID+"/import", strings.NewReader(body))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", dnsZone.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	ImportZoneFile(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("conflict preview status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data struct {
			Valid     bool                     `json:"valid"`
			Conflicts []zone.CSVImportConflict `json:"conflicts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode conflict preview: %v (%s)", err, rec.Body.String())
	}
	if envelope.Data.Valid || len(envelope.Data.Conflicts) != 4 {
		t.Fatalf("conflict preview = %+v, want invalid with four conflicts", envelope.Data)
	}
	want := map[int]string{
		2: "rrset_ttl_mismatch", 3: "cname_exclusive_type",
		5: "cname_exclusive_type", 7: "multiple_cname_targets",
	}
	for _, conflict := range envelope.Data.Conflicts {
		if want[conflict.Row] != conflict.Code {
			t.Errorf("conflict row %d code %q, want %q", conflict.Row, conflict.Code, want[conflict.Row])
		}
		delete(want, conflict.Row)
	}
	if len(want) != 0 {
		t.Errorf("missing conflicts: %+v", want)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE zone_id = ?`, dnsZone.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("conflict preview left %d records, want only the original record", count)
	}
}
