package auditlog

// The data plane's audit writer.
//
// It exists because the writes that need a trail most are the ones with nobody
// watching them: the dynamic-update handler, the DHCP-to-DNS linkage, and the
// lease manager. Three properties carry that weight.
//
//   - "there was nothing" and "it was blank" have to stay distinguishable. A
//     create has no before and a delete has no after, and a report that prints
//     both as an empty string cannot say which of the two a row is.
//   - a write that fails has to come back to the caller. The callers do not
//     treat it as fatal -- a name must still resolve and an address must still
//     be handed out -- but they log it, and that only works if there is
//     something to log.
//   - what was written has to be what is read back. The package's whole reason
//     for existing is that four call sites share one statement; a statement
//     only one of them can read would be worse than four separate ones.

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
)

func newLedger(t *testing.T) *sql.DB {
	t.Helper()

	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(t.TempDir(), "auditlog.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbh.Close() })
	if err := dbh.RunMigrations(filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	return dbh.DB
}

func TestAnEntryIsWrittenWholeAndItsIDComesBack(t *testing.T) {
	db := newLedger(t)

	id, err := Append(db, Entry{
		UserID: "user-1", Username: "operator",
		Action: ActionLeaseBind, ResourceType: ResourceLease, ResourceID: "lease-1",
		Detail: "192.0.2.10", OldValue: "offered", NewValue: "active",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if id == "" {
		t.Fatal("Append returned no id")
	}

	var (
		gotID, action, resourceType, resourceID, detail string
		userID, username, oldValue, newValue            sql.NullString
		success                                         bool
		createdAt                                       string
	)
	if err := db.QueryRow(`
		SELECT id, user_id, username, action, resource_type, resource_id,
			detail, old_value, new_value, success, created_at
		FROM audit_logs WHERE id = ?`, id,
	).Scan(&gotID, &userID, &username, &action, &resourceType, &resourceID,
		&detail, &oldValue, &newValue, &success, &createdAt); err != nil {
		t.Fatalf("read back: %v", err)
	}

	if gotID != id {
		t.Errorf("the row's id is %q, but Append returned %q", gotID, id)
	}
	if action != ActionLeaseBind || resourceType != ResourceLease || resourceID != "lease-1" || detail != "192.0.2.10" {
		t.Errorf("a column was lost: action=%q resource_type=%q resource_id=%q detail=%q",
			action, resourceType, resourceID, detail)
	}
	if userID.String != "user-1" || username.String != "operator" {
		t.Errorf("the actor was lost: user_id=%q username=%q", userID.String, username.String)
	}
	if oldValue.String != "offered" || newValue.String != "active" {
		t.Errorf("the before/after pair was lost: old=%q new=%q", oldValue.String, newValue.String)
	}
	// Everything this package records is something that happened: it is called
	// on the path that did the work, not on the path that refused to. So the
	// flag is not a parameter, and a caller cannot accidentally file a
	// successful write as a failure.
	if !success {
		t.Error("a data-plane entry was recorded as a failure")
	}
	if createdAt == "" {
		t.Error("the entry has no timestamp, so it cannot be placed in a sequence")
	}
}

func TestTheEmptySideOfAChangeIsNullRatherThanBlank(t *testing.T) {
	db := newLedger(t)

	// A create has no before and a delete has no after. Which side is absent is
	// the only thing that distinguishes the two rows once the reader has them,
	// so it has to survive the write.
	createID, err := Append(db, Entry{
		Action: ActionDDNSRecordCreate, ResourceType: ResourceRecord, ResourceID: "r1",
		NewValue: "192.0.2.10",
	})
	if err != nil {
		t.Fatalf("Append (create): %v", err)
	}
	deleteID, err := Append(db, Entry{
		Action: ActionDDNSRecordDelete, ResourceType: ResourceRecord, ResourceID: "r1",
		OldValue: "192.0.2.10",
	})
	if err != nil {
		t.Fatalf("Append (delete): %v", err)
	}
	// A writer with neither side is the ordinary case for an action-only entry,
	// and it must not leave a blank string behind: every reader checks `.Valid`,
	// so a blank would read as a value that exists -- a create would look like a
	// change from something to nothing.
	neitherID, err := Append(db, Entry{
		Action: ActionHATakeover, ResourceType: ResourceNode, ResourceID: "node-1",
	})
	if err != nil {
		t.Fatalf("Append (neither side): %v", err)
	}

	for _, tc := range []struct {
		name    string
		id      string
		wantOld bool // true = the column is NULL
		wantNew bool
	}{
		{name: "a create has no before", id: createID, wantOld: true},
		{name: "a delete has no after", id: deleteID, wantNew: true},
		{name: "an action-only entry has neither", id: neitherID, wantOld: true, wantNew: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var oldNull, newNull bool
			if err := db.QueryRow(
				"SELECT old_value IS NULL, new_value IS NULL FROM audit_logs WHERE id = ?", tc.id,
			).Scan(&oldNull, &newNull); err != nil {
				t.Fatalf("read back: %v", err)
			}
			if oldNull != tc.wantOld {
				t.Errorf("old_value IS NULL = %v, want %v", oldNull, tc.wantOld)
			}
			if newNull != tc.wantNew {
				t.Errorf("new_value IS NULL = %v, want %v", newNull, tc.wantNew)
			}

			// The side that does have a value still has it, so "NULL" here is
			// not simply "the writer drops both".
			var present int
			if err := db.QueryRow(
				"SELECT COUNT(*) FROM audit_logs WHERE id = ? AND (old_value IS NOT NULL OR new_value IS NOT NULL)",
				tc.id,
			).Scan(&present); err != nil {
				t.Fatalf("count the present sides: %v", err)
			}
			wantPresent := 0
			if !tc.wantOld {
				wantPresent++
			}
			if !tc.wantNew {
				wantPresent++
			}
			if present != wantPresent {
				t.Errorf("%d of the two sides carry a value, want %d", present, wantPresent)
			}
		})
	}
}

func TestAnEntryThatCannotBeWrittenSaysWhichOne(t *testing.T) {
	db := newLedger(t)
	if _, err := db.Exec("DROP TABLE audit_logs"); err != nil {
		t.Fatalf("drop audit_logs: %v", err)
	}

	id, err := Append(db, Entry{Action: ActionHAFence, ResourceType: ResourceNode, ResourceID: "node-1"})
	if err == nil {
		t.Fatal("writing to a table that does not exist reported success")
	}
	if id != "" {
		t.Errorf("a failed append returned the id %q", id)
	}
	// The action has to be named. The four call sites are on paths that carry on
	// regardless, so this string is the operator's only clue about which trail
	// has a hole in it.
	if !strings.Contains(err.Error(), ActionHAFence) {
		t.Errorf("the error does not name the action: %v", err)
	}
}
