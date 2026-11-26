package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jincaiw/GoDDI/internal/dataplane"
	"github.com/jincaiw/GoDDI/internal/metrics"
	dto "github.com/prometheus/client_model/go"
)

// A role=dns process has no DHCP data-plane store. The DNS LKG gauge must
// still be refreshable: otherwise the operator cannot see what revision the
// DNS process will keep serving while control is down.
func TestRefreshDataPlaneGaugesWithoutDHCPStore(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dnsdata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE dataplane_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dataplane_meta (key,value) VALUES (?,?),(?,?)`,
		"lkg_revision:dns", "42", "lkg_applied_at:dns", "2026-11-18T04:00:00Z"); err != nil {
		t.Fatal(err)
	}

	refreshDataPlaneGauges(context.Background(), nil, db)
	gauge := metrics.DataPlaneLKGRevision.WithLabelValues(string(dataplane.DomainDNS))
	metric := &dto.Metric{}
	if err := gauge.Write(metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetGauge().GetValue(); got != 42 {
		t.Fatalf("DNS LKG gauge = %v, want 42 without a DHCP store", got)
	}
	applied := metrics.DataPlaneLKGApplied.WithLabelValues(string(dataplane.DomainDNS))
	if err := applied.Write(metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetGauge().GetValue(); got != 1 {
		t.Fatalf("DNS LKG applied = %v, want 1", got)
	}

	// An empty store must be distinguishable from an applied revision zero.
	empty, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	if _, err := empty.Exec(`CREATE TABLE dataplane_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	refreshDataPlaneGauges(context.Background(), nil, empty)
	if err := applied.Write(metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetGauge().GetValue(); got != 0 {
		t.Fatalf("DNS LKG applied for empty store = %v, want 0", got)
	}
}
