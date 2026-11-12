package ipamstore

import (
	"context"
	"strings"
	"testing"
)

func TestMarkerBoundedTypesAndVersions(t *testing.T) {
	cases := []struct {
		name string
		rows []struct {
			version any
			digest  any
		}
	}{
		{"oversize", []struct {
			version any
			digest  any
		}{{1, strings.Repeat("x", 2<<20)}}},
		{"blob", []struct {
			version any
			digest  any
		}{{1, []byte(strings.Repeat("a", 64))}}},
		{"negative", []struct {
			version any
			digest  any
		}{{-1, strings.Repeat("a", 64)}}},
		{"text version", []struct {
			version any
			digest  any
		}{{"1", migrationDigests[initialMigrationPath]}}},
		{"extra", []struct {
			version any
			digest  any
		}{{1, migrationDigests[initialMigrationPath]}, {2, migrationDigests[rowidMigrationPath]}, {3, strings.Repeat("a", 64)}, {4, strings.Repeat("b", 2<<20)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			if _, err := db.Exec(`CREATE TABLE enterprise_schema_migrations(version,digest)`); err != nil {
				t.Fatal(err)
			}
			for _, r := range tc.rows {
				if _, err := db.Exec(`INSERT INTO enterprise_schema_migrations VALUES(?,?)`, r.version, r.digest); err != nil {
					t.Fatal(err)
				}
			}
			c, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err = readVersion(context.Background(), c); err == nil {
				t.Fatal("accepted bad marker")
			}
		})
	}
}
