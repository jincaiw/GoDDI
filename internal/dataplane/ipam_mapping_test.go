package dataplane

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

func TestResolveIPAMSpaceByIPPrefersMostSpecificSubnet(t *testing.T) {
	store := newStore(t, config.DataPlaneLease)
	for _, row := range [][3]string{
		{"broad", "space-broad", "192.0.2.0/24"},
		{"narrow", "space-narrow", "192.0.2.64/26"},
	} {
		if _, err := store.Exec(`INSERT INTO ipam_subnets(id, space_id, cidr) VALUES (?, ?, ?)`, row[0], row[1], row[2]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ResolveIPAMSpaceByIP(context.Background(), "192.0.2.70")
	if err != nil || got != "space-narrow" {
		t.Fatalf("resolved space = %q, %v; want most-specific space-narrow", got, err)
	}
	if _, err := store.ResolveIPAMSpaceByIP(context.Background(), "198.51.100.1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unmapped address error = %v, want sql.ErrNoRows", err)
	}
}
