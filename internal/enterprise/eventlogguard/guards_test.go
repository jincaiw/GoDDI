package eventlogguard

import (
	"context"
	"strings"
	"testing"
)

type tableCase struct{ name, first, second string }

var tableCases = []tableCase{
	{"writer_authority", `INSERT INTO main.writer_authority(rowid,cluster_id,holder_id,epoch,db_generation,grant_digest,granted_at_unix_ms) VALUES(?,x'01000000000000000000000000000000',x'02000000000000000000000000000000',1,'a',zeroblob(32),1)`, `INSERT INTO main.writer_authority(rowid,cluster_id,holder_id,epoch,db_generation,grant_digest,granted_at_unix_ms) VALUES(?,x'03000000000000000000000000000000',x'04000000000000000000000000000000',1,'b',zeroblob(32),1)`},
	{"replica_head", `INSERT INTO main.replica_head(rowid,cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,x'01000000000000000000000000000000',1,0,zeroblob(32),1,0)`, `INSERT INTO main.replica_head(rowid,cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(?,x'03000000000000000000000000000000',1,0,zeroblob(32),1,0)`},
	{"replica_event", `INSERT INTO main.replica_event(rowid,cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,x'01000000000000000000000000000000',1,1,x'02000000000000000000000000000000',1,1,zeroblob(32),zeroblob(32),x'01')`, `INSERT INTO main.replica_event(rowid,cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,x'03000000000000000000000000000000',1,1,x'04000000000000000000000000000000',1,1,zeroblob(32),zeroblob(32),x'02')`},
	{"replica_checkpoint", `INSERT INTO main.replica_checkpoint(rowid,cluster_id,source_id,epoch,seq,hash,applied_at_unix_ms) VALUES(?,x'01000000000000000000000000000000',x'02000000000000000000000000000000',1,0,zeroblob(32),1)`, `INSERT INTO main.replica_checkpoint(rowid,cluster_id,source_id,epoch,seq,hash,applied_at_unix_ms) VALUES(?,x'03000000000000000000000000000000',x'04000000000000000000000000000000',1,0,zeroblob(32),1)`},
	{"idempotency_receipt", `INSERT INTO main.idempotency_receipt(rowid,tenant_id,principal,method,path,idempotency_key,request_digest,response,created_at_unix_ms,expires_at_unix_ms) VALUES(?,x'01000000000000000000000000000000','a','POST','/a','a',zeroblob(32),x'01',1,2)`, `INSERT INTO main.idempotency_receipt(rowid,tenant_id,principal,method,path,idempotency_key,request_digest,response,created_at_unix_ms,expires_at_unix_ms) VALUES(?,x'03000000000000000000000000000000','b','POST','/b','b',zeroblob(32),x'02',1,2)`},
	{"static_occupancy_audit", `INSERT INTO main.static_occupancy_audit(rowid,cluster_id,epoch,tenant_id,space_id,resource_id,principal,action,request_id,prior_revision,new_revision,result,occurred_at_ms,request_digest) VALUES(?,x'01000000000000000000000000000000',1,'t','s','r','a','ipam.occupancy.create','a',0,1,'SUCCESS',1,zeroblob(32))`, `INSERT INTO main.static_occupancy_audit(rowid,cluster_id,epoch,tenant_id,space_id,resource_id,principal,action,request_id,prior_revision,new_revision,result,occurred_at_ms,request_digest) VALUES(?,x'03000000000000000000000000000000',1,'u','s','r','b','ipam.occupancy.create','b',0,1,'SUCCESS',1,zeroblob(32))`},
}

func TestSixTablesRowIDGuards(t *testing.T) {
	for _, tc := range tableCases {
		t.Run(tc.name, func(t *testing.T) {
			db := legacy(t)
			if _, err := db.Exec(tc.first, 100); err != nil {
				t.Fatal(err)
			}
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatal(err)
			}
			for _, recursive := range []string{"OFF", "ON"} {
				if _, err := db.Exec(`PRAGMA recursive_triggers=` + recursive); err != nil {
					t.Fatal(err)
				}
				for _, alias := range []string{"rowid", "_rowid_", "oid"} {
					for _, mode := range []string{"OR REPLACE", "OR IGNORE"} {
						q := strings.Replace(tc.second, "INSERT INTO", "INSERT "+mode+" INTO", 1)
						q = strings.Replace(q, "(rowid,", "("+alias+",", 1)
						if _, err := db.Exec(q, 100); err == nil || !strings.Contains(err.Error(), "EVENTLOG_ROWID_CONFLICT") {
							t.Fatalf("%s %s %s: %v", recursive, alias, mode, err)
						}
					}
					if _, err := db.Exec(`UPDATE OR REPLACE main.` + tc.name + ` SET ` + alias + `=` + alias + `+900 WHERE rowid=100`); err == nil {
						t.Fatalf("%s rowid update accepted", alias)
					}
				}
			}
			for _, id := range []int64{0, -1, -42} {
				if _, err := db.Exec(tc.second, id); err == nil || !strings.Contains(err.Error(), "EVENTLOG_ROWID_NONPOSITIVE") {
					t.Fatalf("nonpositive %d: %v", id, err)
				}
			}
			var n int
			if err := db.QueryRow(`SELECT count(*) FROM main.` + tc.name + ` WHERE rowid=100`).Scan(&n); err != nil || n != 1 {
				t.Fatalf("old row lost %d %v", n, err)
			}
			if _, err := db.Exec(tc.second, nil); err != nil {
				t.Fatalf("NULL auto rowid: %v", err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM main.` + tc.name + ` WHERE rowid>0`).Scan(&n); err != nil || n != 2 {
				t.Fatalf("positive rows %d %v", n, err)
			}
		})
	}
}
func TestImplicitRowsAndNegativeHistory(t *testing.T) {
	for _, tc := range tableCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, bad := range []int64{0, -1} {
				db := legacy(t)
				if _, err := db.Exec(tc.first, bad); err != nil {
					t.Fatal(err)
				}
				if err := migrateFixture(context.Background(), db, hooks{}); err == nil {
					t.Fatalf("history %d accepted", bad)
				}
				schemaVersion(t, db, 0)
				var n int
				if err := db.QueryRow(`SELECT count(*) FROM main.`+tc.name+` WHERE rowid=?`, bad).Scan(&n); err != nil || n != 1 {
					t.Fatal("bad history modified")
				}
			}
			db := legacy(t)
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatal(err)
			}
			q := strings.Replace(tc.first, "(rowid,", "(", 1)
			q = strings.Replace(q, "VALUES(?,", "VALUES(", 1)
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("implicit: %v", err)
			}
		})
	}
}
func TestKeyCollisionAndCAS(t *testing.T) {
	for _, tc := range tableCases[:] {
		if tc.name != "writer_authority" && tc.name != "replica_head" && tc.name != "replica_checkpoint" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			db := legacy(t)
			if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.first, 100); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.second, 200); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"OR REPLACE", "OR IGNORE"} {
				q := strings.Replace(tc.first, "INSERT INTO", "INSERT "+mode+" INTO", 1)
				if _, err := db.Exec(q, 300); err == nil || !strings.Contains(err.Error(), "EVENTLOG_KEY_CONFLICT") {
					t.Fatalf("%s: %v", mode, err)
				}
			}
			q := strings.Replace(tc.first, "(rowid,", "(", 1)
			q = strings.Replace(q, "VALUES(?,", "VALUES(", 1)
			q += ` ON CONFLICT DO UPDATE SET epoch=excluded.epoch`
			if _, err := db.Exec(q); err == nil || !strings.Contains(err.Error(), "EVENTLOG_KEY_CONFLICT") {
				t.Fatalf("UPSERT: %v", err)
			}
			if _, err := db.Exec(`UPDATE OR REPLACE main.` + tc.name + ` SET cluster_id=x'03000000000000000000000000000000' WHERE rowid=100`); err == nil || !strings.Contains(err.Error(), "EVENTLOG_KEY_IMMUTABLE") {
				t.Fatalf("key update: %v", err)
			}
			if tc.name == "replica_checkpoint" {
				if _, err := db.Exec(`UPDATE OR REPLACE main.replica_checkpoint SET source_id=x'04000000000000000000000000000000' WHERE rowid=100`); err == nil || !strings.Contains(err.Error(), "EVENTLOG_KEY_IMMUTABLE") {
					t.Fatalf("checkpoint source key: %v", err)
				}
			}
			var n int
			if err := db.QueryRow(`SELECT count(*) FROM main.` + tc.name).Scan(&n); err != nil || n != 2 {
				t.Fatalf("lost row %d %v", n, err)
			}
			result, err := db.Exec(`UPDATE main.` + tc.name + ` SET epoch=epoch+1 WHERE rowid=100 AND epoch=1`)
			if err != nil {
				t.Fatal(err)
			}
			affected, err := result.RowsAffected()
			if err != nil || affected != 1 {
				t.Fatalf("CAS %d %v", affected, err)
			}
			if tc.name == "replica_head" || tc.name == "replica_checkpoint" {
				r, e := db.Exec(`UPDATE main.` + tc.name + ` SET seq=seq+1 WHERE rowid=100 AND seq=0`)
				if e != nil {
					t.Fatal(e)
				}
				n, e := r.RowsAffected()
				if e != nil || n != 1 {
					t.Fatalf("head/checkpoint CAS %d %v", n, e)
				}
			}
		})
	}
}
func TestMultirowAtomicAndMaxRowID(t *testing.T) {
	db := legacy(t)
	if err := migrateFixture(context.Background(), db, hooks{}); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO main.replica_head(rowid,cluster_id,epoch,seq,hash,schema_version,advanced_at_unix_ms) VALUES(100,x'01000000000000000000000000000000',1,0,zeroblob(32),1,0),(-1,x'02000000000000000000000000000000',1,0,zeroblob(32),1,0)`)
	if err == nil {
		t.Fatal("invalid batch accepted")
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM main.replica_head`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("partial batch %d %v", n, err)
	}
	if _, err = db.Exec(tableCases[1].first, int64(9223372036854775807)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(tableCases[1].second, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM main.replica_head WHERE rowid>0`).Scan(&n); err != nil || n != 2 {
		t.Fatal("max allocator failed")
	}
}
