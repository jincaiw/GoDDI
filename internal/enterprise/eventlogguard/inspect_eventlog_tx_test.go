package eventlogguard

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
)

func TestInspectTxAndEventlogSnapshotShareTransaction(t *testing.T) {
	for _, version := range []Version{LegacyReference, HardenedReference} {
		for _, inspectFirst := range []bool{true, false} {
			name := map[Version]string{LegacyReference: "legacy", HardenedReference: "guarded"}[version]
			if inspectFirst {
				name += "/inspect-first"
			} else {
				name += "/event-first"
			}
			t.Run(name, func(t *testing.T) {
				db := legacy(t)
				if version == HardenedReference {
					if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
						t.Fatal(err)
					}
				}
				cluster, node := eventlog.ID{1}, eventlog.ID{2}
				genesis := eventlog.GenesisHash()
				if _, err := db.Exec(`INSERT INTO main.replica_head(cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,1,0,?,1,0)`, cluster[:], genesis[:]); err != nil {
					t.Fatal(err)
				}
				log := eventlog.NewLog(db, cluster, node)
				c := connQueryOnly(t, db)
				var tempCount int
				if err := c.QueryRowContext(context.Background(), `SELECT count(*) FROM temp.sqlite_schema`).Scan(&tempCount); err != nil || tempCount != 0 {
					t.Fatalf("empty temp %d %v", tempCount, err)
				}
				tx, err := c.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				inspect := func() {
					v, e := InspectTx(context.Background(), tx)
					if e != nil || v != version {
						t.Fatalf("inspect version=%v err=%v", v, e)
					}
				}
				read := func() {
					events, head, e := log.ReadSnapshotTx(context.Background(), tx, eventlog.Position{Epoch: 1}, 10)
					if e != nil || len(events) != 0 || head != (eventlog.Position{Epoch: 1}) {
						t.Fatalf("snapshot events=%v head=%v err=%v", events, head, e)
					}
				}
				if inspectFirst {
					inspect()
					read()
				} else {
					read()
					inspect()
				}
				var count int
				if err = tx.QueryRowContext(context.Background(), `SELECT count(*) FROM main.replica_head`).Scan(&count); err != nil || count != 1 {
					t.Fatalf("caller transaction lost %d %v", count, err)
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
