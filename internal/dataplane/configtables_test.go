package dataplane

import "testing"

// The cross-node replica must carry the full control row for ipam_subnets.
// The local projection intentionally has only id/space_id/cidr; reusing it for
// cross-node replication omitted name (NOT NULL) and broke every snapshot.
func TestConfigTableSpecsCarryControlSchema(t *testing.T) {
	specs := ConfigTableSpecs()
	var spaces, subnets int = -1, -1
	for i, s := range specs {
		if s.Name == "ipam_spaces" {
			spaces = i
		}
		if s.Name == "ipam_subnets" {
			subnets = i
			want := []string{"id", "space_id", "name", "cidr", "vlan_id", "location", "description", "created_at", "updated_at", "lifecycle_state", "retired_at", "retired_by", "retire_reason"}
			if len(s.Columns) != len(want) {
				t.Fatalf("ipam_subnets columns = %v, want %v", s.Columns, want)
			}
			for j, c := range want {
				if s.Columns[j] != c {
					t.Fatalf("ipam_subnets columns = %v, want %v", s.Columns, want)
				}
			}
		}
	}
	if spaces < 0 {
		t.Fatal("ipam_spaces parent spec missing")
	}
	if subnets < 0 {
		t.Fatal("ipam_subnets spec missing")
	}
	if spaces > subnets {
		t.Fatalf("ipam_spaces (index %d) must be applied before ipam_subnets (index %d)", spaces, subnets)
	}
}

// The local projection must stay minimal: dhcpRunner writes these columns into
// leases.db, whose ipam_subnets table has exactly id/space_id/cidr.
func TestLocalProjectionUnchanged(t *testing.T) {
	for _, tbl := range dhcpTables {
		if tbl.name != "ipam_subnets" {
			continue
		}
		if len(tbl.columns) != 3 || tbl.columns[0] != "id" || tbl.columns[1] != "space_id" || tbl.columns[2] != "cidr" {
			t.Fatalf("local ipam_subnets projection changed: %v", tbl.columns)
		}
		return
	}
	t.Fatal("ipam_subnets missing from dhcpTables")
}
