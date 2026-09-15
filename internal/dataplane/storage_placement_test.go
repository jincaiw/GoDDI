package dataplane

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	_ "modernc.org/sqlite"
)

func TestFactsStoragePlacementIsLeaseProducerAndControlConsumer(t *testing.T) {
	store := newStore(t, config.DataPlaneLease)

	var producer, consumer, atomicity, status string
	if err := store.QueryRow(`
		SELECT producer_store, consumer_store, atomicity, status
		FROM facts_storage_placement WHERE id = 1`).Scan(
		&producer, &consumer, &atomicity, &status); err != nil {
		t.Fatalf("read facts storage placement: %v", err)
	}
	if producer != "leases" || consumer != "control" {
		t.Fatalf("placement = %q -> %q, want leases -> control", producer, consumer)
	}
	if atomicity != "per_store_only" {
		t.Fatalf("atomicity = %q, want per_store_only", atomicity)
	}
	if status != "opt_in" {
		t.Fatalf("status = %q, want opt_in", status)
	}
}

func TestFactsStoragePlacementIsAbsentFromControlDatabase(t *testing.T) {
	controlPath := filepath.Join(t.TempDir(), "control.db")
	control, err := sql.Open("sqlite", controlPath)
	if err != nil {
		t.Fatalf("open control database: %v", err)
	}
	defer control.Close()

	var count int
	if err := control.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'facts_storage_placement'`).Scan(&count); err != nil {
		t.Fatalf("inspect control database: %v", err)
	}
	if count != 0 {
		t.Fatal("facts storage placement unexpectedly exists before data-plane migrations")
	}
}
