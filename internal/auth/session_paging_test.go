package auth

import (
	"fmt"
	"testing"
	"time"
)

// The session-management page used to load every unexpired session of the
// user in one response. Every login leaves a session behind, so a busy
// instance eventually shipped megabytes and rendered thousands of rows on a
// single page load. These tests pin the paged read that replaced it.
func TestListUserSessionsPagedReturnsOnePageAndTheTotal(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		token_hash TEXT NOT NULL,
		ip_address TEXT NOT NULL,
		user_agent TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	sm := NewSessionManager(db)
	const n = 7
	for i := 0; i < n; i++ {
		_, err := db.Exec(`INSERT INTO sessions
			(id, user_id, token_hash, ip_address, user_agent, expires_at, created_at)
			VALUES (?, 'u1', 'hash', '127.0.0.1', 'test', ?, ?)`,
			fmt.Sprintf("s%02d", i),
			time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			time.Now().UTC().Add(time.Duration(i)*time.Second).Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
	}
	// A second user's sessions must not leak into the page or the total.
	_, err := db.Exec(`INSERT INTO sessions
		(id, user_id, token_hash, ip_address, user_agent, expires_at, created_at)
		VALUES ('other', 'u2', 'hash', '127.0.0.1', 'test', ?, ?)`,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	// An expired session must be counted out.
	_, err = db.Exec(`INSERT INTO sessions
		(id, user_id, token_hash, ip_address, user_agent, expires_at, created_at)
		VALUES ('expired', 'u1', 'hash', '127.0.0.1', 'test', ?, ?)`,
		time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}

	page, total, err := sm.ListUserSessionsPaged("u1", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != n {
		t.Fatalf("total = %d, want %d (the other user's and expired rows are excluded)", total, n)
	}
	if len(page) != 3 {
		t.Fatalf("first page has %d rows, want 3", len(page))
	}
	// Newest first: s06, s05, s04.
	want := []string{"s06", "s05", "s04"}
	for i, s := range page {
		if s.ID != want[i] {
			t.Errorf("page[%d].ID = %q, want %q", i, s.ID, want[i])
		}
		if s.TokenHash != "" {
			t.Error("the token hash must never be exposed by the paged read")
		}
	}

	page2, total2, err := sm.ListUserSessionsPaged("u1", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total2 != n || len(page2) != 3 {
		t.Fatalf("second page: total=%d rows=%d, want total=%d rows=3", total2, len(page2), n)
	}
	if page2[0].ID != "s03" || page2[2].ID != "s01" {
		t.Errorf("second page order wrong: %s..%s", page2[0].ID, page2[2].ID)
	}

	// The last page holds the remainder, and a page past the end is empty
	// rather than an error or a wraparound.
	page3, _, err := sm.ListUserSessionsPaged("u1", 3, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 1 || page3[0].ID != "s00" {
		t.Errorf("last page: %+v, want only s00", page3)
	}
	page4, _, err := sm.ListUserSessionsPaged("u1", 3, 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(page4) != 0 {
		t.Errorf("page past the end returned %d rows, want 0", len(page4))
	}
}
