package handler

// Cross-page pagination semantics, pinned server-side.
//
// The old console e2e suite carried the regression "server pagination shows
// the next page instead of slicing it twice": a list handler that applies
// LIMIT/OFFSET to an already-limited subquery returns page 1's rows again on
// page 2, and the operator never sees the tail of the table. The bug lives in
// the SQL shape, not in the console, so the pin is the handler reading the
// real migrated schema through ListIPAMSpaces.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/ipam/space"
)

func spacePage(t *testing.T, page, pageSize string) (names []string, total int, pages int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ipam/spaces?page="+page+"&page_size="+pageSize, nil)
	rec := httptest.NewRecorder()
	ListIPAMSpaces(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list page=%s size=%s: status %d: %s", page, pageSize, rec.Code, rec.Body.String())
	}
	var env struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
			Pages int   `json:"pages"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decoding the page: %v (%s)", err, rec.Body.String())
	}
	for _, s := range env.Data {
		names = append(names, s.Name)
	}
	return names, int(env.Meta.Total), env.Meta.Pages
}

func createSpaceViaHandler(t *testing.T, name string) {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"description":"pagination coverage"}`, name)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ipam/spaces", strings.NewReader(body))
	rec := httptest.NewRecorder()
	CreateIPAMSpace(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating space %q: status %d: %s", name, rec.Code, rec.Body.String())
	}
}

// Eleven spaces, page size ten: page 1 holds the first ten and page 2 holds
// exactly the one remaining row — never a re-slice of page 1, and never
// nothing. Page 3 is empty, and the meta block reports the whole table.
func TestListSpacesPageTwoShowsTheNextRowsNotAReSlice(t *testing.T) {
	db := newControlPlaneTestDB(t)
	t.Cleanup(func() { db.Close() })
	withIPAMServices(t, &IPAMServiceContainer{DB: db, SpaceMgr: space.NewManager(db)})

	const count = 11
	var all []string
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("page-cover-%02d", i)
		createSpaceViaHandler(t, name)
		all = append(all, name)
	}

	page1, total, pages := spacePage(t, "1", "10")
	if total != count || pages != 2 {
		t.Fatalf("meta total=%d pages=%d, want %d/2", total, pages, count)
	}
	if len(page1) != 10 {
		t.Fatalf("page 1 has %d rows, want 10", len(page1))
	}

	page2, _, _ := spacePage(t, "2", "10")
	if len(page2) != 1 {
		t.Fatalf("page 2 has %d rows, want exactly the 1 remaining row (a re-slice of page 1 is the old bug)", len(page2))
	}

	seen := make(map[string]bool, len(page1))
	for _, n := range page1 {
		if seen[n] {
			t.Fatalf("page 1 repeats %q", n)
		}
		seen[n] = true
	}
	for _, n := range page2 {
		if seen[n] {
			t.Fatalf("page 2 repeats page 1's %q: the tail of the table is unreachable", n)
		}
		seen[n] = true
	}
	if len(seen) != count {
		t.Fatalf("pages 1+2 cover %d distinct rows, want all %d", len(seen), count)
	}

	page3, _, _ := spacePage(t, "3", "10")
	if len(page3) != 0 {
		t.Fatalf("page 3 has %d rows, want none", len(page3))
	}
}
