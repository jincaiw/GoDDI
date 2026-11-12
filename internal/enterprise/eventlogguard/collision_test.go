package eventlogguard

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

func rowsFingerprint(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	rows, err := db.Query(`SELECT rowid,* FROM main.` + table + ` ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for rows.Next() {
		v := make([]any, len(columns))
		p := make([]any, len(v))
		for i := range v {
			p[i] = &v[i]
		}
		if err = rows.Scan(p...); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&out, "%#v\n", v)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
func TestReplaceCollidingTwoDifferentRowsIsAtomic(t *testing.T) {
	for _, tc := range tableCases {
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
			before := rowsFingerprint(t, db, tc.name)
			q := strings.Replace(tc.second, "INSERT INTO", "INSERT OR REPLACE INTO", 1)
			if _, err := db.Exec(q, 100); err == nil {
				t.Fatal("replacement accepted")
			}
			if after := rowsFingerprint(t, db, tc.name); after != before {
				t.Fatalf("rows changed: before=%s after=%s", before, after)
			}
		})
	}
}
