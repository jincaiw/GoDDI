package dhcplease

// Feed wiring for the schema-2 snapshot: the lease-table set expressed as
// pipeline schema entries, so a schema-2 NDJSON stream (the eight-table IPAM
// candidate followed by the four lease tables) can flow through one unified
// snapshotrows validator.

import (
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotpipeline"
)

// SchemaTables returns the lease-table set's pipeline schema entries in frozen
// table order, each carrying its own row validator. Composing these after the
// eight-table IPAM schema entries (snapshotipam.New) gives a schema-2
// snapshot one unified validation stream: the manifest lists twelve tables
// and every row is checked against its frozen shape.
//
// The returned slice is freshly built on each call; callers may append,
// reorder or mutate it. The names here must match the manifest exactly —
// snapshotpipeline.New rejects a schema/manifest mismatch.
func SchemaTables() []snapshotpipeline.SchemaTable {
	order := []string{"dhcp_lease", "dhcp_declined", "dhcp_outbox", "dhcp_receipt"}
	out := make([]snapshotpipeline.SchemaTable, 0, len(order))
	for _, name := range order {
		name := name
		out = append(out, snapshotpipeline.SchemaTable{
			Name:        name,
			ValidateRow: func(raw []byte) error { _, err := validateRow(name, raw); return err },
		})
	}
	return out
}
