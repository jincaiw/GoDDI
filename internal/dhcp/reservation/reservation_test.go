package reservation

// The reservation write path.
//
// A reservation is the one place an operator names an address by hand, so it is
// also the one place where a wrong answer is attributed to the server rather
// than to the network. Every refusal here is asserted with errors.Is, because
// that is what the handler switches on: ErrDuplicate and ErrOutOfRange become
// 409 and 400, and anything else becomes a 500 that tells the operator to look
// in the log for a mistake they made in a form.
//
// The schema below is copied from migrations/003_init_dhcp_tables.sql --
// including the UNIQUE constraint on mac_address -- so a case that passes here
// is about the same table production has.

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const scopeID = "scope-lan"

func newReservationDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE dhcp_scopes (
		id TEXT PRIMARY KEY,
		subnet TEXT NOT NULL,
		start_ip TEXT NOT NULL,
		end_ip TEXT NOT NULL
	);
	CREATE TABLE dhcp_reservations (
		id TEXT PRIMARY KEY,
		scope_id TEXT NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
		ip_address TEXT NOT NULL,
		mac_address TEXT NOT NULL UNIQUE,
		hostname TEXT,
		description TEXT,
		enabled BOOLEAN DEFAULT TRUE,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	INSERT INTO dhcp_scopes (id, subnet, start_ip, end_ip)
		VALUES ('scope-lan', '192.0.2.0/24', '192.0.2.100', '192.0.2.110');
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestAReservationIsCreatedAndReadBackWhole(t *testing.T) {
	m := NewManager(newReservationDB(t))

	r, err := m.CreateReservation(scopeID, "192.0.2.105", "AA:BB:CC:DD:EE:01", "printer",
		ReservationOptions{Description: "floor 3"})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if r.ID == "" {
		t.Fatal("no id came back")
	}
	if r.ScopeID != scopeID || r.IPAddress != "192.0.2.105" ||
		r.MACAddress != "AA:BB:CC:DD:EE:01" || r.Hostname != "printer" || r.Description != "floor 3" {
		t.Errorf("a field was lost in the round trip: %+v", r)
	}
	// A reservation created without being told is enabled: the operator named
	// an address, and a reservation that arrives switched off would silently
	// fail to do the one thing it was created for.
	if !r.Enabled {
		t.Error("a newly created reservation is disabled")
	}

	// The server looks a reservation up by MAC, so that read has to agree with
	// the one by id.
	byMAC, err := m.GetReservationByMAC("AA:BB:CC:DD:EE:01")
	if err != nil {
		t.Fatalf("GetReservationByMAC: %v", err)
	}
	if byMAC == nil || byMAC.ID != r.ID {
		t.Fatalf("lookup by MAC returned %+v, want the reservation %s", byMAC, r.ID)
	}

	// A MAC with no reservation is not an error -- the caller is the
	// allocation path, and "no static binding" is an ordinary answer.
	if none, err := m.GetReservationByMAC("AA:BB:CC:DD:EE:99"); err != nil || none != nil {
		t.Errorf("an unreserved MAC gave (%+v, %v), want (nil, nil)", none, err)
	}

	// A disabled reservation is still there, but it is not a binding.
	off := false
	if _, err := m.UpdateReservation(r.ID, ReservationOptions{Enabled: &off}); err != nil {
		t.Fatalf("UpdateReservation: %v", err)
	}
	if none, err := m.GetReservationByMAC("AA:BB:CC:DD:EE:01"); err != nil || none != nil {
		t.Errorf("a disabled reservation answered a MAC lookup with (%+v, %v)", none, err)
	}
	if still, err := m.GetReservation(r.ID); err != nil || still.Enabled {
		t.Errorf("the disabled reservation is gone or still enabled: %+v, %v", still, err)
	}
}

func TestAnAddressOutsideItsScopeIsRefused(t *testing.T) {
	m := NewManager(newReservationDB(t))

	for _, tc := range []struct {
		name string
		ip   string
		why  string
	}{
		{"outside the subnet entirely", "198.51.100.5",
			"the scope is 192.0.2.0/24; a name for an address in another subnet is a typo, not a policy"},
		{"inside the subnet but outside the pool", "192.0.2.200",
			"an address the server will never offer and never check for conflicts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := m.CreateReservation(scopeID, tc.ip, "AA:BB:CC:DD:EE:02", "host", ReservationOptions{})
			if !errors.Is(err, ErrOutOfRange) {
				t.Fatalf("err = %v, want ErrOutOfRange (%s)", err, tc.why)
			}
		})
	}

	// And with no scope id there is nothing to check against, which is the
	// documented case for importers that pass a synthetic id.
	if _, err := m.CreateReservation("", "198.51.100.5", "AA:BB:CC:DD:EE:03", "host", ReservationOptions{}); err != nil {
		t.Fatalf("a scope-less reservation was refused: %v", err)
	}
}

func TestAnAddressCannotBeReservedTwiceInTheSameScope(t *testing.T) {
	m := NewManager(newReservationDB(t))

	first, err := m.CreateReservation(scopeID, "192.0.2.105", "AA:BB:CC:DD:EE:01", "printer", ReservationOptions{})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}

	// Two different MACs on one address is the corruption this refusal exists
	// for: the DHCP server would answer DISCOVER with whichever row it read
	// first, and the two hosts would take turns owning the address.
	if _, err := m.CreateReservation(scopeID, "192.0.2.105", "AA:BB:CC:DD:EE:02", "other", ReservationOptions{}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("a second reservation on the same address gave %v, want ErrDuplicate", err)
	}

	// Switching the first one off does not free the address for a second
	// static claim: a disabled row is not a binding, but the operator has to
	// delete it to say they are done with it.
	off := false
	if _, err := m.UpdateReservation(first.ID, ReservationOptions{Enabled: &off}); err != nil {
		t.Fatalf("UpdateReservation: %v", err)
	}
	if _, err := m.CreateReservation(scopeID, "192.0.2.105", "AA:BB:CC:DD:EE:02", "other", ReservationOptions{}); err != nil {
		t.Fatalf("a disabled reservation kept its address out of use: %v", err)
	}
}

func TestAMacAddressCannotBeReservedTwice(t *testing.T) {
	m := NewManager(newReservationDB(t))

	if _, err := m.CreateReservation(scopeID, "192.0.2.105", "AA:BB:CC:DD:EE:01", "printer", ReservationOptions{}); err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}

	// The UNIQUE constraint on mac_address is what answers here, and the
	// management UI shows this text, so it has to name the address that
	// collided rather than the SQL statement that failed.
	_, err := m.CreateReservation(scopeID, "192.0.2.106", "AA:BB:CC:DD:EE:01", "printer-again", ReservationOptions{})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("a second reservation for one MAC gave %v, want ErrDuplicate", err)
	}
	if !strings.Contains(err.Error(), "AA:BB:CC:DD:EE:01") {
		t.Errorf("the refusal does not name the MAC: %v", err)
	}
}

func TestMissingFieldsAreRefusedRatherThanStoredBlank(t *testing.T) {
	m := NewManager(newReservationDB(t))

	// ip_address and mac_address are NOT NULL in the schema, so without this
	// check the failure would surface as a driver error -- a 500 for what is a
	// missing form field.
	for _, tc := range []struct{ name, ip, mac string }{
		{"no address", "", "AA:BB:CC:DD:EE:01"},
		{"no MAC", "192.0.2.105", ""},
		{"neither", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := m.CreateReservation(scopeID, tc.ip, tc.mac, "host", ReservationOptions{}); !errors.Is(err, ErrInvalidData) {
				t.Fatalf("err = %v, want ErrInvalidData", err)
			}
		})
	}
}

func TestAMoveIsValidatedBeforeAnythingIsWritten(t *testing.T) {
	db := newReservationDB(t)
	m := NewManager(db)

	r, err := m.CreateReservation(scopeID, "192.0.2.105", "AA:BB:CC:DD:EE:01", "printer",
		ReservationOptions{Description: "floor 3"})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	other, err := m.CreateReservation(scopeID, "192.0.2.106", "AA:BB:CC:DD:EE:02", "scanner", ReservationOptions{})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}

	// Out of range: refused, and the stored row is untouched. A partial update
	// that half-applied would leave the reservation describing an address the
	// server will never hand out.
	if _, err := m.UpdateReservation(r.ID, ReservationOptions{IPAddress: "192.0.2.200"}); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("moving an address out of the pool gave %v, want ErrOutOfRange", err)
	}
	if after, err := m.GetReservation(r.ID); err != nil || after.IPAddress != "192.0.2.105" {
		t.Errorf("address = %q after a refused move, want 192.0.2.105 (%v)", after.IPAddress, err)
	}
	if _, err := m.UpdateReservation(r.ID, ReservationOptions{IPAddress: "not-an-ip"}); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("an unparseable address gave %v, want ErrInvalidData", err)
	}

	// Onto the other reservation's address.
	if _, err := m.UpdateReservation(r.ID, ReservationOptions{IPAddress: other.IPAddress}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("moving onto a reserved address gave %v, want ErrDuplicate", err)
	}
	// Onto the other reservation's MAC.
	if _, err := m.UpdateReservation(r.ID, ReservationOptions{MACAddress: other.MACAddress}); err == nil {
		t.Fatal("two reservations ended up on one MAC")
	}

	// A field the update did not mention survives, and a mention changes it.
	moved, err := m.UpdateReservation(r.ID, ReservationOptions{IPAddress: "192.0.2.107", Hostname: "printer-3f"})
	if err != nil {
		t.Fatalf("UpdateReservation: %v", err)
	}
	if moved.IPAddress != "192.0.2.107" || moved.Hostname != "printer-3f" {
		t.Errorf("the update did not take: %+v", moved)
	}
	if moved.MACAddress != "AA:BB:CC:DD:EE:01" || moved.Description != "floor 3" {
		t.Errorf("a field the update did not mention was lost: %+v", moved)
	}

	if _, err := m.UpdateReservation("no-such-reservation", ReservationOptions{Hostname: "x"}); err == nil {
		t.Error("updating a reservation that does not exist reported success")
	}
}

func TestListingCountsTheFilteredSetAndBoundsThePage(t *testing.T) {
	m := NewManager(newReservationDB(t))

	for _, tc := range []struct{ ip, mac, host string }{
		{"192.0.2.100", "AA:BB:CC:DD:EE:01", "printer-a"},
		{"192.0.2.101", "AA:BB:CC:DD:EE:02", "printer-b"},
		{"192.0.2.102", "AA:BB:CC:DD:EE:03", "scanner"},
	} {
		if _, err := m.CreateReservation(scopeID, tc.ip, tc.mac, tc.host, ReservationOptions{}); err != nil {
			t.Fatalf("CreateReservation(%s): %v", tc.host, err)
		}
	}

	for _, tc := range []struct {
		name   string
		filter ReservationFilter
		want   int64
	}{
		{"everything", ReservationFilter{}, 3},
		{"by scope", ReservationFilter{ScopeID: scopeID}, 3},
		{"by scope that has none", ReservationFilter{ScopeID: "scope-elsewhere"}, 0},
		{"by MAC", ReservationFilter{MACAddress: "AA:BB:CC:DD:EE:02"}, 1},
		{"by address", ReservationFilter{IPAddress: "192.0.2.102"}, 1},
		{"by a hostname fragment", ReservationFilter{Hostname: "printer"}, 2},
		{"by two filters together", ReservationFilter{ScopeID: scopeID, Hostname: "scanner"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := m.ListReservations(tc.filter)
			if err != nil {
				t.Fatalf("ListReservations: %v", err)
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
	rows, _, err := m.ListReservations(ReservationFilter{PageSize: 5000})
	if err != nil {
		t.Fatalf("ListReservations: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("returned %d rows, want 3", len(rows))
	}
	// Page 0 is page 1 rather than a negative offset.
	if page0, _, err := m.ListReservations(ReservationFilter{Page: 0}); err != nil || len(page0) != 3 {
		t.Errorf("page 0 returned %d rows (%v), want 3", len(page0), err)
	}
}

func TestDeletingSomethingThatIsNotThereSaysSo(t *testing.T) {
	m := NewManager(newReservationDB(t))

	r, err := m.CreateReservation(scopeID, "192.0.2.105", "AA:BB:CC:DD:EE:01", "printer", ReservationOptions{})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if err := m.DeleteReservation(r.ID); err != nil {
		t.Fatalf("DeleteReservation: %v", err)
	}
	if _, err := m.GetReservation(r.ID); err == nil {
		t.Error("the reservation survived its own deletion")
	}
	// Deleting twice is a refusal rather than a silent success: the caller has
	// to be able to tell "removed" from "was never there".
	if err := m.DeleteReservation(r.ID); err == nil {
		t.Error("deleting an absent reservation reported success")
	}
}
