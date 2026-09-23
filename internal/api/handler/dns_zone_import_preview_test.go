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
	zone, err := cachedZoneMgr.CreateZone(zone.ZoneOptions{Name: "preview.example.test", Type: string(zone.ZoneTypePrimary)})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"format":"csv","dry_run":true,"content":"name,type,value,ttl,priority,weight,port,flag,tag\nwww,A,192.0.2.25,300,,,,,\n"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dns/zones/"+zone.ID+"/import", strings.NewReader(body))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", zone.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	ImportZoneFile(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dry-run status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ZoneID      string         `json:"zone_id"`
			DryRun      bool           `json:"dry_run"`
			RecordCount int            `json:"record_count"`
			RecordTypes map[string]int `json:"record_types"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run response: %v (%s)", err, rec.Body.String())
	}
	if envelope.Data.ZoneID != zone.ID || !envelope.Data.DryRun || envelope.Data.RecordCount != 1 || envelope.Data.RecordTypes["A"] != 1 {
		t.Fatalf("dry-run response = %+v", envelope.Data)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE zone_id = ?`, zone.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("dry-run wrote %d records", count)
	}
}
