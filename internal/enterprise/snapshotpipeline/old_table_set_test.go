package snapshotpipeline

// Old isolated four-, five- and six-table snapshot candidates are not a
// subset of the frozen eight-table format, even though all were labelled
// schema=1. They must be refused at the pipeline boundary rather than padded
// with zero-row tables, which would silently invent ledger history. See
// docs/audit/SNAPSHOT-MIGRATION-PATH.md.

import (
	"fmt"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/snapshotverify"
)

func eightTableSchema() []SchemaTable {
	return []SchemaTable{
		{Name: "ipam_tenants"},
		{Name: "ipam_spaces"},
		{Name: "ipam_prefixes"},
		{Name: "ipam_pools"},
		{Name: "ipam_occupancies"},
		{Name: "ipam_resource_ids"},
		{Name: "static_occupancy_audit"},
		{Name: "idempotency_receipt"},
	}
}

func TestOldFourFiveSixTableSetsAreRefused(t *testing.T) {
	m, trust, transportLimits, rowLimits, _, _ := fixture()
	schema := eightTableSchema()
	rowLimits.Tables = len(schema)

	for _, n := range []int{4, 5, 6} {
		t.Run(fmt.Sprintf("tables=%d", n), func(t *testing.T) {
			bad := m
			bad.Tables = make([]snapshotverify.TableCount, n)
			for i := range bad.Tables {
				bad.Tables[i] = snapshotverify.TableCount{Name: schema[i].Name, Rows: 0}
			}
			if _, err := New(bad, trust, transportLimits, rowLimits, schema, 100); err == nil {
				t.Fatalf("a %d-table manifest was accepted against the frozen 8-table schema", n)
			}
		})
	}
}

// Positive control: the complete eight-table set (including zero-row tables)
// is accepted by New. This proves the refusals above are about the table set,
// not a broken fixture. It does not establish authority to install anything.
func TestCompleteEightTableSetIsAcceptedByNew(t *testing.T) {
	m, trust, transportLimits, rowLimits, _, _ := fixture()
	schema := eightTableSchema()
	rowLimits.Tables = len(schema)

	good := m
	good.Tables = make([]snapshotverify.TableCount, len(schema))
	for i, table := range schema {
		good.Tables[i] = snapshotverify.TableCount{Name: table.Name, Rows: 0}
	}
	// An empty snapshot has no chunks and no declared bytes/rows.
	good.Chunks = nil
	good.TotalBytes = 0

	if _, err := New(good, trust, transportLimits, rowLimits, schema, 100); err != nil {
		t.Fatalf("complete 8-table manifest rejected: %v", err)
	}
}
