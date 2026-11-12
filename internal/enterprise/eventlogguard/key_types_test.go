package eventlogguard

import (
	"context"
	"strings"
	"testing"
)

func TestMalformedHistoricalAndNewKeysRejected(t *testing.T) {
	cases := []struct{ name, table, sql string }{
		{"authority null", "writer_authority", strings.Replace(tableCases[0].first, "x'01000000000000000000000000000000'", "NULL", 1)},
		{"authority text", "writer_authority", strings.Replace(tableCases[0].first, "x'01000000000000000000000000000000'", "'abcdefghijklmnop'", 1)},
		{"head null", "replica_head", strings.Replace(tableCases[1].first, "x'01000000000000000000000000000000'", "NULL", 1)},
		{"head text", "replica_head", strings.Replace(tableCases[1].first, "x'01000000000000000000000000000000'", "'abcdefghijklmnop'", 1)},
		{"checkpoint cluster text", "replica_checkpoint", strings.Replace(tableCases[3].first, "x'01000000000000000000000000000000'", "'abcdefghijklmnop'", 1)},
		{"checkpoint source text", "replica_checkpoint", strings.Replace(tableCases[3].first, "x'02000000000000000000000000000000'", "'abcdefghijklmnop'", 1)},
		{"receipt tenant text", "idempotency_receipt", strings.Replace(tableCases[4].first, "x'01000000000000000000000000000000'", "'abcdefghijklmnop'", 1)},
		{"receipt principal blob", "idempotency_receipt", strings.Replace(tableCases[4].first, "'a','POST','/a','a'", "x'61','POST','/a','a'", 1)},
		{"receipt method blob", "idempotency_receipt", strings.Replace(tableCases[4].first, "'a','POST','/a','a'", "'a',x'504F5354','/a','a'", 1)},
		{"receipt path blob", "idempotency_receipt", strings.Replace(tableCases[4].first, "'a','POST','/a','a'", "'a','POST',x'2F61','a'", 1)},
		{"receipt key blob", "idempotency_receipt", strings.Replace(tableCases[4].first, "'a','POST','/a','a'", "'a','POST','/a',x'61'", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := legacy(t)
			if _, err := db.Exec(tc.sql, 100); err != nil {
				t.Fatalf("legacy fixture must demonstrate lax DDL: %v", err)
			}
			if err := migrateFixture(context.Background(), db, hooks{}); err == nil {
				t.Fatal("accepted malformed historical key")
			}
			schemaVersion(t, db, 0)
			var n int
			if err := db.QueryRow(`SELECT count(*) FROM main.` + tc.table + ` WHERE rowid=100`).Scan(&n); err != nil || n != 1 {
				t.Fatal("malformed row was repaired/deleted")
			}
			clean := legacy(t)
			if err := migrateFixture(context.Background(), clean, hooks{}); err != nil {
				t.Fatal(err)
			}
			if _, err := clean.Exec(tc.sql, 100); err == nil {
				t.Fatal("accepted malformed new key")
			}
		})
	}
}
