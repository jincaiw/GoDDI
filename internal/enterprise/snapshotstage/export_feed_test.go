package snapshotstage

// The eight-table export/feed closed loop: rows ExportRows emits from the
// staged guarded candidate must pass the snapshotipam validator unchanged —
// the export and the feed are the same contract seen from the writer and the
// reader side. The validator's own strict PK order check runs inside Validate
// for every table, so this also proves the export's ORDER BY matches the
// frozen BINARY order.

import (
	"context"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/snapshotipam"
)

func TestExportedCandidateRowsPassTheValidator(t *testing.T) {
	lines := guardedLines()
	m, tr, chunks := snapshot(t, lines)
	s, err := importFixture(t, context.Background(), m, tr, node(), chunks, fixtureHooks{hardened: true})
	if err != nil {
		t.Fatal(err)
	}
	exported, err := snapshotipam.ExportRows(context.Background(), s.conn)
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if len(exported) == 0 {
		t.Fatal("the export is empty")
	}
	v := snapshotipam.New()
	for i, r := range exported {
		if _, err := v.Validate(r.Table, r.Payload); err != nil {
			t.Fatalf("exported row %d (%s) must pass the validator: %v\npayload: %s", i, r.Table, err, r.Payload)
		}
	}
}
