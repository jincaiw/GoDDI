package space

// The space write path.
//
// A space is the top of the IPAM tree, and its deletion is the one operation
// here that can remove more than it was asked to: ipam_subnets references
// ipam_spaces with ON DELETE CASCADE, and ipam_addresses references
// ipam_subnets the same way. So the guard in DeleteSpace is not a courtesy
// check -- it is the only thing standing between "delete this empty space" and
// "delete every subnet and address under it". That is why one of the cases
// below is about the guard itself failing to run, rather than about it
// answering wrongly.

import (
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func newSpaceDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	schema := `
	CREATE TABLE ipam_spaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE ipam_subnets (
		id TEXT PRIMARY KEY,
		space_id TEXT NOT NULL REFERENCES ipam_spaces(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		cidr TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestASpaceIsCreatedAndReadBackWhole(t *testing.T) {
	m := NewManager(newSpaceDB(t))

	s, err := m.CreateSpace("campus", "the main site")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if s.ID == "" {
		t.Fatal("no id came back")
	}
	if s.Name != "campus" || s.Description != "the main site" {
		t.Errorf("a field was lost in the round trip: %+v", s)
	}

	// A space with no name cannot be identified in any list, so it is refused
	// before it reaches the NOT NULL column.
	if _, err := m.CreateSpace("", "unnameable"); err == nil {
		t.Error("a nameless space was created")
	}
	// The name is UNIQUE in the schema.
	if _, err := m.CreateSpace("campus", "again"); err == nil {
		t.Error("a second space took the same name")
	}

	if _, err := m.GetSpace("no-such-space"); err == nil {
		t.Error("GetSpace invented a space that does not exist")
	}
}

func TestASpaceThatStillHasSubnetsIsNotDeleted(t *testing.T) {
	db := newSpaceDB(t)
	m := NewManager(db)

	s, err := m.CreateSpace("campus", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ipam_subnets (id, space_id, name, cidr)
		VALUES ('sub-1', ?, 'lan', '192.0.2.0/24')`, s.ID); err != nil {
		t.Fatalf("insert subnet: %v", err)
	}

	err = m.DeleteSpace(s.ID)
	if !errors.Is(err, ErrSpaceInUse) {
		t.Fatalf("err = %v, want ErrSpaceInUse", err)
	}
	// The handler maps that sentinel to 409, so a message that reads like an
	// internal failure would be the wrong one to show.
	if _, err := m.GetSpace(s.ID); err != nil {
		t.Errorf("the space is gone after a refused delete: %v", err)
	}
	if n := countSubnets(t, db, s.ID); n != 1 {
		t.Errorf("%d subnets remain, want 1 -- the cascade ran", n)
	}
}

// TestACheckThatCannotRunIsNotAnEmptyAnswer covers the guard itself.
//
// DeleteSpace used to read the subnet count with the error discarded:
//
//	m.db.QueryRow("SELECT COUNT(*) FROM ipam_subnets WHERE space_id = ?", id).Scan(&subnetCount)
//
// A query that cannot run leaves subnetCount at zero, and zero is also the
// answer for "this space is empty" -- so a check that failed to happen was
// indistinguishable from a check that passed, and the delete went ahead. With
// ON DELETE CASCADE in the schema, going ahead means removing subnets and
// addresses nobody asked to remove.
//
// The case drops the table the guard reads. In production that is a database
// whose migrations are behind the binary, which is exactly when an operator is
// most likely to be deleting things. The rule is the one this project states
// for every allow-list: something that could not be read must not answer the
// way an empty list answers.
func TestACheckThatCannotRunIsNotAnEmptyAnswer(t *testing.T) {
	db := newSpaceDB(t)
	m := NewManager(db)

	s, err := m.CreateSpace("campus", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if _, err := db.Exec("DROP TABLE ipam_subnets"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	if err := m.DeleteSpace(s.ID); err == nil {
		t.Fatal("a delete whose safety check could not run reported success")
	}
	if _, err := m.GetSpace(s.ID); err != nil {
		t.Errorf("the space was removed by a delete that could not check it: %v", err)
	}
}

func TestASpaceWithNoSubnetsIsDeletedAndTheSecondAttemptSaysSo(t *testing.T) {
	db := newSpaceDB(t)
	m := NewManager(db)

	s, err := m.CreateSpace("campus", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if err := m.DeleteSpace(s.ID); err != nil {
		t.Fatalf("DeleteSpace: %v", err)
	}
	if _, err := m.GetSpace(s.ID); err == nil {
		t.Error("the space survived its own deletion")
	}
	// "Removed" and "was never there" have to be tellable apart.
	if err := m.DeleteSpace(s.ID); err == nil {
		t.Error("deleting an absent space reported success")
	}

	// An empty space is reusable territory: deleting it does not reserve its
	// name. Asserted because the opposite would be a surprising rule to
	// discover from a 409 on a name nobody is using.
	if _, err := m.CreateSpace("campus", "rebuilt"); err != nil {
		t.Errorf("the name of a deleted space could not be reused: %v", err)
	}
}

func TestListingCountsTheFilteredSetAndBoundsThePage(t *testing.T) {
	m := NewManager(newSpaceDB(t))

	for _, name := range []string{"campus", "campus-lab", "datacentre"} {
		if _, err := m.CreateSpace(name, ""); err != nil {
			t.Fatalf("CreateSpace(%s): %v", name, err)
		}
	}

	for _, tc := range []struct {
		name   string
		filter SpaceFilter
		want   int64
	}{
		{"everything", SpaceFilter{}, 3},
		{"by a name fragment", SpaceFilter{Name: "campus"}, 2},
		{"by a fragment nothing matches", SpaceFilter{Name: "branch"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := m.ListSpaces(tc.filter)
			if err != nil {
				t.Fatalf("ListSpaces: %v", err)
			}
			if total != tc.want {
				t.Errorf("total = %d, want %d", total, tc.want)
			}
			if int64(len(rows)) != tc.want {
				t.Errorf("returned %d rows, want %d", len(rows), tc.want)
			}
		})
	}

	// The page size is clamped, so one request cannot ask for the whole table.
	rows, _, err := m.ListSpaces(SpaceFilter{PageSize: 5000})
	if err != nil {
		t.Fatalf("ListSpaces: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("returned %d rows, want 3", len(rows))
	}
	// Page 0 is page 1 rather than a negative offset.
	if page0, _, err := m.ListSpaces(SpaceFilter{Page: 0}); err != nil || len(page0) != 3 {
		t.Errorf("page 0 returned %d rows (%v), want 3", len(page0), err)
	}
}

func TestUpdatingKeepsWhatWasNotMentioned(t *testing.T) {
	m := NewManager(newSpaceDB(t))

	s, err := m.CreateSpace("campus", "the main site")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	renamed, err := m.UpdateSpace(s.ID, SpaceOptions{Name: "campus-main"})
	if err != nil {
		t.Fatalf("UpdateSpace: %v", err)
	}
	if renamed.Name != "campus-main" {
		t.Errorf("name = %q, want campus-main", renamed.Name)
	}
	if renamed.Description != "the main site" {
		t.Errorf("a field the update did not mention was lost: %+v", renamed)
	}

	if _, err := m.UpdateSpace("no-such-space", SpaceOptions{Name: "x"}); err == nil {
		t.Error("updating a space that does not exist reported success")
	}
}

// ---- helpers ---------------------------------------------------------------

func countSubnets(t *testing.T, db *sql.DB, spaceID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM ipam_subnets WHERE space_id = ?", spaceID).Scan(&n); err != nil {
		t.Fatalf("count subnets: %v", err)
	}
	return n
}
