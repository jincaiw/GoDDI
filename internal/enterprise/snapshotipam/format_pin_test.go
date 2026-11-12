package snapshotipam

import "testing"

// TestCandidateFormatIsPinned guards the experimental eight-table snapshot row
// format against accidental drift. The table order and every column list are the
// wire-visible candidate format documented in FORMAT.md. Any change here must be
// a deliberate, versioned decision (the four-, five-, six- and eight-table
// candidates are mutually incompatible even where all are labelled schema=1), so
// this test forces that review instead of letting code and docs silently diverge.
func TestCandidateFormatIsPinned(t *testing.T) {
	wantOrder := []string{
		"ipam_tenants",
		"ipam_spaces",
		"ipam_prefixes",
		"ipam_pools",
		"ipam_occupancies",
		"ipam_resource_ids",
		"static_occupancy_audit",
		"idempotency_receipt",
	}
	if len(order) != len(wantOrder) {
		t.Fatalf("candidate table order length=%d want %d", len(order), len(wantOrder))
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("candidate table order[%d]=%q want %q", i, order[i], wantOrder[i])
		}
	}

	wantCols := map[string][]string{
		"ipam_tenants":           {"id", "slug", "display_name", "revision"},
		"ipam_spaces":            {"tenant_id", "id", "slug", "display_name", "revision"},
		"ipam_prefixes":          {"tenant_id", "space_id", "id", "parent_id", "family", "prefix_len", "first_ip", "last_ip", "kind", "usage", "site_ref", "label", "revision"},
		"ipam_pools":             {"tenant_id", "space_id", "id", "subnet_id", "family", "pool_kind", "first_ip", "last_ip", "label", "revision"},
		"ipam_occupancies":       {"tenant_id", "space_id", "id", "subnet_id", "family", "address", "kind", "owner_ref", "expires_at_ms", "created_at_ms", "revision"},
		"ipam_resource_ids":      {"kind", "tenant_id", "space_id", "id"},
		"static_occupancy_audit": {"cluster_id", "epoch", "tenant_id", "space_id", "resource_id", "principal", "action", "request_id", "prior_revision", "new_revision", "result", "occurred_at_ms", "request_digest"},
		"idempotency_receipt":    {"tenant_id", "principal", "method", "path", "idempotency_key", "request_digest", "response", "created_at_unix_ms", "expires_at_unix_ms"},
	}
	if len(definitions) != len(wantCols) {
		t.Fatalf("definitions has %d tables want %d", len(definitions), len(wantCols))
	}
	for table, want := range wantCols {
		got, ok := definitions[table]
		if !ok {
			t.Fatalf("missing table %q", table)
		}
		if len(got) != len(want) {
			t.Fatalf("%s column count=%d want %d", table, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s column[%d]=%q want %q", table, i, got[i], want[i])
			}
		}
	}
}
