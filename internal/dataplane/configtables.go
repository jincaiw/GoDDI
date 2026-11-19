package dataplane

// ConfigTableSpec describes one replicated configuration table in a form other
// packages (and, for ADR-0013 configuration replication, a peer node) can use
// without seeing the unexported table/apply internals.
//
// It is deliberately a plain value. The cross-node replication and the local
// control->dataplane sync are NOT the same copy: the local one writes a minimal
// projection into leases.db/dnsdata.db (only the columns the packet path
// reads), while the cross-node one replaces whole rows in the follower's
// CONTROL database, which has the full schema. Sharing one column list between
// the two is what caused ipam_subnets.name (NOT NULL, no default) to be omitted
// and made every snapshot apply fail on the follower. See configReplOverrides.
type ConfigTableSpec struct {
	// Name is the SQL table name.
	Name string
	// Columns are the carried columns, in insert order.
	Columns []string
	// Key is the identifying column (conflict target / ordering key).
	Key string
	// Mode is "replace_whole" or "replace_control_authored": how a replica is
	// brought up to date.
	Mode string
}

// Replace modes as exposed to callers outside this package.
const (
	ConfigReplaceWhole           = "replace_whole"
	ConfigReplaceControlAuthored = "replace_control_authored"
)

func (m replaceMode) configName() string {
	switch m {
	case replaceWhole:
		return ConfigReplaceWhole
	case replaceControlAuthored:
		return ConfigReplaceControlAuthored
	default:
		return ""
	}
}

// configReplOverrides carries the full CONTROL-database row for tables whose
// local data-plane projection is intentionally a subset. A follower applies
// snapshots with INSERT OR REPLACE into its control DB, so every NOT NULL
// column without a default must be present or the whole transaction fails.
var configReplOverrides = map[string][]string{
	"ipam_subnets": {
		"id", "space_id", "name", "cidr", "vlan_id", "location", "description",
		"created_at", "updated_at", "lifecycle_state", "retired_at", "retired_by", "retire_reason",
	},
}

// ipamSpacesSpec is the parent of ipam_subnets and must land first: the control
// DB declares ipam_subnets.space_id as a foreign key into ipam_spaces. Without
// it a subnet created under a new address space on the author can never be
// inserted on the follower.
var ipamSpacesSpec = ConfigTableSpec{
	Name:    "ipam_spaces",
	Columns: []string{"id", "name", "description", "created_at", "updated_at", "lifecycle_state", "retired_at", "retired_by", "retire_reason"},
	Key:     "id",
	Mode:    ConfigReplaceWhole,
}

// ConfigTableSpecs returns the configuration tables of the DHCP and DNS domains,
// in parent-first order, for cross-node configuration replication (ADR-0013).
//
// The lease and facts domains are excluded on purpose: leases are runtime state
// replicated by the DHCP HA channel, and facts are a log, not configuration. A
// follower must never copy either of those into its control database.
func ConfigTableSpecs() []ConfigTableSpec {
	out := []ConfigTableSpec{ipamSpacesSpec}
	appendSpecs := func(tables []table) {
		for _, t := range tables {
			cols := append([]string(nil), t.columns...)
			if override, ok := configReplOverrides[t.name]; ok {
				cols = append([]string(nil), override...)
			}
			out = append(out, ConfigTableSpec{
				Name:    t.name,
				Columns: cols,
				Key:     t.key,
				Mode:    t.mode.configName(),
			})
		}
	}
	appendSpecs(dhcpTables)
	appendSpecs(dnsTables)
	return out
}
