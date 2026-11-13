package dataplane

// ConfigTableSpec describes one replicated configuration table in a form other
// packages (and, for ADR-0013 configuration replication, a peer node) can use
// without seeing the unexported table/apply internals.
//
// It is deliberately a plain value: the cross-node replication must carry the
// exact same column set and replace mode the local control→dataplane sync uses,
// or the two copies of the schema would drift. Deriving both from the same
// definitions (dhcpTables/dnsTables) is what keeps them in step.
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

// ConfigTableSpecs returns the configuration tables of the DHCP and DNS domains,
// in parent-first order, for cross-node configuration replication (ADR-0013).
//
// The lease and facts domains are excluded on purpose: leases are runtime state
// replicated by the DHCP HA channel, and facts are a log, not configuration. A
// follower must never copy either of those into its control database.
func ConfigTableSpecs() []ConfigTableSpec {
	var out []ConfigTableSpec
	appendSpecs := func(tables []table) {
		for _, t := range tables {
			out = append(out, ConfigTableSpec{
				Name:    t.name,
				Columns: append([]string(nil), t.columns...),
				Key:     t.key,
				Mode:    t.mode.configName(),
			})
		}
	}
	appendSpecs(dhcpTables)
	appendSpecs(dnsTables)
	return out
}
