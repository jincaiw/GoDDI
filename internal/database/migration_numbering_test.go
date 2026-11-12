package database

// Migration numbering is an implicit contract with goose: a file named
// 033_x.sql after 033_y.sql silently replaces one of the two, and a gap in the
// sequence means an older database can never reach a known schema version.
// The expand/contract rules are pinned elsewhere (migration_contract_test.go);
// this file pins the sequence itself across every shipped migration set.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var numberedMigrationRe = regexp.MustCompile(`^(\d{3})_.+\.sql$`)

type migrationSet struct {
	dir        string
	goose      bool
	minVersion int
}

func migrationSets() []migrationSet {
	return []migrationSet{
		{dir: filepath.Join("..", "..", "migrations"), goose: true, minVersion: 1},
		{dir: filepath.Join("..", "dataplane", "migrations"), goose: true, minVersion: 1},
		// The IPAM reference migrations are plain SQL applied by ipamstore,
		// not goose, so they have no Up/Down markers.
		{dir: filepath.Join("..", "enterprise", "ipamstore", "migrations"), goose: false, minVersion: 1},
	}
}

func TestMigrationNumberingIsContiguousAcrossAllSets(t *testing.T) {
	for _, set := range migrationSets() {
		t.Run(filepath.ToSlash(set.dir), func(t *testing.T) {
			entries, err := os.ReadDir(set.dir)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[int]string{}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
					continue
				}
				m := numberedMigrationRe.FindStringSubmatch(e.Name())
				if m == nil {
					t.Fatalf("migration file %q does not follow NNN_name.sql", e.Name())
				}
				n, err := strconv.Atoi(m[1])
				if err != nil {
					t.Fatal(err)
				}
				if prev, dup := seen[n]; dup {
					t.Fatalf("duplicate migration number %03d: %q and %q", n, prev, e.Name())
				}
				seen[n] = e.Name()

				body, err := os.ReadFile(filepath.Join(set.dir, e.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if len(strings.TrimSpace(string(body))) == 0 {
					t.Fatalf("migration %q is empty", e.Name())
				}
				if set.goose {
					for _, marker := range []string{"-- +goose Up", "-- +goose Down"} {
						if !strings.Contains(string(body), marker) {
							t.Fatalf("migration %q is missing %q; goose would not apply it as intended", e.Name(), marker)
						}
					}
				}
			}

			if len(seen) == 0 {
				t.Fatal("no migrations found")
			}
			nums := make([]int, 0, len(seen))
			for n := range seen {
				nums = append(nums, n)
			}
			sort.Ints(nums)
			for i, n := range nums {
				want := set.minVersion + i
				if n != want {
					t.Fatalf("migration numbers are not contiguous: at position %d got %03d, want %03d (files: %v)",
						i, n, want, nums)
				}
			}
			// Guard against a new set silently starting at 0 or jumping ahead.
			if nums[0] != set.minVersion {
				t.Fatalf("first migration is %03d, want %03d", nums[0], set.minVersion)
			}
			t.Logf("%03d..%03d (%d files)", nums[0], nums[len(nums)-1], len(nums))
		})
	}
}
