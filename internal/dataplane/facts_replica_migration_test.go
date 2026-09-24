package dataplane

import (
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

func TestFactsReplicaStagingMigrationIsApplied(t *testing.T) {
	store := newStore(t, config.RoleDHCP)
	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='facts_replica_snapshot_chunks'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("facts replica staging table is absent after data-plane migrations")
	}
}
