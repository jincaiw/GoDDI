package api

// Accounts without an email address.
//
// The column is UNIQUE, and SQLite treats two empty strings as duplicates while
// treating every NULL as distinct. The create handler used to pass the request
// field straight through, so the first account with no email was created as ''
// and every account after it was refused with "the username or email already
// exists" -- blaming a username that was free. The handler's own validation
// comment already said an empty value stores SQL NULL; nothing implemented it.

import (
	"net/http"
	"testing"
)

func TestTwoAccountsWithoutAnEmailCanBothExist(t *testing.T) {
	srv, dbh := newSessionTestServer(t)
	adminToken, adminCSRF := initialiseAndLogIn(t, srv, "w14_email_admin", "W14-Admin!Pass42")

	first := createUser(t, srv, adminToken, adminCSRF, "w14_no_mail_a", "W14-NoMail!Pass42")
	// The second one is the whole point: with the empty string written through,
	// this is where the 409 lands.
	second := createUser(t, srv, adminToken, adminCSRF, "w14_no_mail_b", "W14-NoMail!Pass42")

	// The stored value is NULL, not an empty string. An assertion on the API
	// alone would pass for a server that stored '' and simply skipped the
	// unique index, which is not what makes a third account possible.
	var stored int
	if err := dbh.DB.QueryRow(
		"SELECT COUNT(*) FROM users WHERE email IS NULL AND username IN (?, ?)",
		"w14_no_mail_a", "w14_no_mail_b").Scan(&stored); err != nil {
		t.Fatalf("count the accounts with no email: %v", err)
	}
	if stored != 2 {
		t.Errorf("%d of the two accounts are stored with a NULL email, want 2", stored)
	}

	// Both read paths have to survive the NULL: listing scans every row, and
	// the single-user read is a different query.
	if code, body, _ := doJSON(t, "GET", srv.URL+"/api/v1/users", adminToken, "", nil); code != http.StatusOK {
		t.Errorf("listing users with email-less accounts = %d, want 200 (message=%q)", code, body.Message)
	}
	for _, id := range []string{first, second} {
		if code, body, _ := doJSON(t, "GET", srv.URL+"/api/v1/users/"+id, adminToken, "", nil); code != http.StatusOK {
			t.Errorf("reading an account with no email = %d, want 200 (message=%q)", code, body.Message)
		}
	}
}
