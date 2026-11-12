package prodgates

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/dhcptx"
	"github.com/jasonwa/goddi/internal/enterprise/dnstransport"
	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/eventlogguard"
	"github.com/jasonwa/goddi/internal/enterprise/generationpointer"
	haregstry "github.com/jasonwa/goddi/internal/enterprise/haregistry"
	"github.com/jasonwa/goddi/internal/enterprise/replicaapply"
	"github.com/jasonwa/goddi/internal/enterprise/replicaread"
	"github.com/jasonwa/goddi/internal/enterprise/replicarpc"
	"github.com/jasonwa/goddi/internal/enterprise/snapshotstage"
	_ "modernc.org/sqlite"
)

// TestProductionWriteGatesRemainClosed pins every public mutation/authority
// entry point to its closed sentinel. These are intentionally ErrNoAuthority
// until independent role, epoch, fencing, generation and trusted-source evidence
// exists. Opening any one of them by accident would be a production safety bug,
// so this test fails loudly instead.
func TestProductionWriteGatesRemainClosed(t *testing.T) {
	ctx := context.Background()
	var nilLog *eventlog.Log

	beginErr := func() error {
		_, err := nilLog.Begin(ctx)
		return err
	}
	readErr := func() error {
		_, err := replicaread.Read(ctx)
		return err
	}

	checks := []struct {
		name string
		got  error
		want error
	}{
		{"snapshotstage.Import", snapshotstage.Import(ctx), snapshotstage.ErrNoAuthority},
		{"snapshotstage.Install", snapshotstage.Install(ctx), snapshotstage.ErrNoAuthority},
		{"replicaapply.Apply", replicaapply.Apply(ctx, eventlog.Event{}), replicaapply.ErrNoAuthority},
		{"replicaapply.Batch", replicaapply.Batch(ctx, nil), replicaapply.ErrNoAuthority},
		{"replicaread.Read", readErr(), replicaread.ErrNoAuthority},
		{"eventlogguard.Migrate", eventlogguard.Migrate(ctx, (*sql.DB)(nil)), eventlogguard.ErrNoAuthority},
		{"generationpointer.Import", generationpointer.Import(ctx), generationpointer.ErrNoAuthority},
		{"generationpointer.Install", generationpointer.Install(ctx), generationpointer.ErrNoAuthority},
		{"generationpointer.Activate", generationpointer.Activate(ctx), generationpointer.ErrNoAuthority},
		{"generationpointer.Switch", generationpointer.Switch(ctx), generationpointer.ErrNoAuthority},
		{"dhcptx.Begin", dhcptx.Begin(ctx), dhcptx.ErrNoAuthority},
		{"dhcptx.Commit", dhcptx.Commit(ctx), dhcptx.ErrNoAuthority},
		{"replicarpc.Start", replicarpc.Start(), replicarpc.ErrNoAuthority},
		{"dnstransport.Start", dnstransport.Start(), dnstransport.ErrNoAuthority},
		{"haregistry.Issue", haregstry.Issue(ctx, (*sql.Tx)(nil), haregstry.IssueRequest{}), haregstry.ErrNoAuthority},
		{"haregistry.Cancel", haregstry.Cancel(ctx, (*sql.Tx)(nil), ""), haregstry.ErrNoAuthority},
		{"haregistry.Complete", haregstry.Complete(ctx, (*sql.Tx)(nil), ""), haregstry.ErrNoAuthority},
		{"eventlog.Begin", beginErr(), eventlog.ErrNoAuthority},
		{"eventlog.RecordApplied", nilLog.RecordApplied(ctx, eventlog.ID{}, eventlog.Position{}, time.Time{}), eventlog.ErrNoAuthority},
	}
	for _, c := range checks {
		if c.got == nil {
			t.Fatalf("%s returned nil; gate is open", c.name)
		}
		if !errors.Is(c.got, c.want) {
			t.Fatalf("%s returned %v; want errors.Is(..., %v)", c.name, c.got, c.want)
		}
	}
}

// TestAuthoritySentinelsAreDistinct ensures each candidate package owns a
// separate ErrNoAuthority value. If two packages shared one instance, a caller
// doing errors.Is(err, otherPkg.ErrNoAuthority) could mistake one module's gate
// for another's and take the wrong fail-closed branch. Distinctness keeps gate
// identity unambiguous.
func TestAuthoritySentinelsAreDistinct(t *testing.T) {
	sentinels := map[string]error{
		"snapshotstage":     snapshotstage.ErrNoAuthority,
		"replicaapply":      replicaapply.ErrNoAuthority,
		"replicaread":       replicaread.ErrNoAuthority,
		"eventlogguard":     eventlogguard.ErrNoAuthority,
		"generationpointer": generationpointer.ErrNoAuthority,
		"dhcptx":            dhcptx.ErrNoAuthority,
		"replicarpc":        replicarpc.ErrNoAuthority,
		"dnstransport":      dnstransport.ErrNoAuthority,
		"haregistry":        haregstry.ErrNoAuthority,
		"eventlog":          eventlog.ErrNoAuthority,
	}
	for nameA, a := range sentinels {
		if a == nil {
			t.Fatalf("%s sentinel is nil", nameA)
		}
		for nameB, b := range sentinels {
			if nameA == nameB {
				continue
			}
			if errors.Is(a, b) {
				t.Fatalf("%s.ErrNoAuthority errors.Is %s.ErrNoAuthority; sentinels must be distinct", nameA, nameB)
			}
		}
	}
}

// A writer_authority row is only one half of the HA contract. The other half
// is an independently validated local role log, epoch, generation and fencing;
// a database row (or a config value, or a TLS identity) must never by itself
// let a node begin a protected write. This test inserts a fully formed row and
// asserts the gates stay closed, with a count query as a positive control that
// the fixture really does contain the row.
func TestADatabaseAuthorityRowAloneDoesNotGrantWriteAuthority(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := eventlog.Init(ctx, db); err != nil {
		t.Fatal(err)
	}

	cluster := eventlog.ID{1}
	holder := eventlog.ID{2}

	// Read side: an empty authority table must fail closed with the same
	// sentinel, not a generic "no rows" error a caller might mistake for a
	// benign empty result.
	if _, err := eventlog.CurrentAuthority(ctx, db, cluster); !errors.Is(err, eventlog.ErrNoAuthority) {
		t.Fatalf("CurrentAuthority on an empty table returned %v, want ErrNoAuthority", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO writer_authority(cluster_id, holder_id, epoch, db_generation, grant_digest, granted_at_unix_ms)
		VALUES (?, ?, 1, 'gen-test', ?, 1)`, cluster[:], holder[:], make([]byte, 32)); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM writer_authority WHERE cluster_id = ?`, cluster[:]).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("fixture has %d authority rows, want 1", rows)
	}

	log := eventlog.NewLog(db, cluster, holder)
	if _, err := log.Begin(ctx); !errors.Is(err, eventlog.ErrNoAuthority) {
		t.Fatalf("Begin with a writer_authority row returned %v, want ErrNoAuthority", err)
	}
	if err := log.RecordApplied(ctx, eventlog.ID{}, eventlog.Position{}, time.Time{}); !errors.Is(err, eventlog.ErrNoAuthority) {
		t.Fatalf("RecordApplied with a writer_authority row returned %v, want ErrNoAuthority", err)
	}
}
