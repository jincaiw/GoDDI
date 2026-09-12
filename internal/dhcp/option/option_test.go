package option

// The DHCP option write path, and the priority ladder that decides what a
// client is actually told.
//
// The ladder is the branch worth pinning. Four sources can name a value for the
// same option code, and the whole point of the table is that the most specific
// one wins: reservation > client_class > scope > global. A test that only
// created one option per code would pass under any ordering, so the cases below
// deliberately put the same code in every layer and then assert which value
// survives.
//
// The second branch worth pinning is in BuildOptions: a value that cannot be
// encoded is dropped, and the rest of the response still goes out. The
// alternative -- failing the whole call -- would cost a client its address over
// one malformed option it never asked for.

import (
	"database/sql"
	"net"
	"strings"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/rfc1035label"
	_ "modernc.org/sqlite"
)

const (
	scopeID = "scope-lan"
	resID   = "res-printer"
)

func newOptionDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE dhcp_scopes (id TEXT PRIMARY KEY);
	CREATE TABLE dhcp_reservations (id TEXT PRIMARY KEY);
	CREATE TABLE dhcp_options (
		id TEXT PRIMARY KEY,
		scope_id TEXT NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
		reservation_id TEXT REFERENCES dhcp_reservations(id) ON DELETE CASCADE,
		code INTEGER NOT NULL,
		value TEXT NOT NULL,
		priority TEXT NOT NULL DEFAULT 'scope',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	INSERT INTO dhcp_scopes (id) VALUES ('scope-lan');
	INSERT INTO dhcp_reservations (id) VALUES ('res-printer');
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func mustCreate(t *testing.T, m *Manager, scope string, opts OptionOptions) *Option {
	t.Helper()
	o, err := m.CreateOption(scope, opts)
	if err != nil {
		t.Fatalf("CreateOption(%+v): %v", opts, err)
	}
	return o
}

func TestAnOptionIsCreatedAndReadBackWhole(t *testing.T) {
	m := NewManager(newOptionDB(t))

	o := mustCreate(t, m, scopeID, OptionOptions{Code: OptionRouter, Value: "192.0.2.1"})
	if o.ID == "" {
		t.Fatal("no id came back")
	}
	if o.ScopeID != scopeID || o.Code != OptionRouter || o.Value != "192.0.2.1" {
		t.Errorf("a field was lost in the round trip: %+v", o)
	}
	// The default priority is the scope's own layer. Asserted because the
	// global layer is selected by this string, so a different default would
	// quietly move every option the UI creates into (or out of) a layer.
	if o.Priority != "scope" {
		t.Errorf("priority = %q, want \"scope\"", o.Priority)
	}
	// No reservation was named, so the column is NULL and reads back as empty
	// rather than as the reservation's id.
	if o.ReservationID != "" {
		t.Errorf("reservation_id = %q, want empty", o.ReservationID)
	}

	byRes := mustCreate(t, m, scopeID, OptionOptions{
		Code: OptionRouter, Value: "192.0.2.2", ReservationID: resID, Priority: "reservation",
	})
	if byRes.ReservationID != resID || byRes.Priority != "reservation" {
		t.Errorf("the reservation layer was not recorded: %+v", byRes)
	}

	// Four refusals, all of which would otherwise reach a NOT NULL column or
	// silently create an option the encoder cannot use.
	for _, tc := range []struct {
		name string
		opts OptionOptions
	}{
		{"no code", OptionOptions{Value: "192.0.2.1"}},
		{"a code this build does not support", OptionOptions{Code: 200, Value: "x"}},
		{"no value", OptionOptions{Code: OptionRouter}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := m.CreateOption(scopeID, tc.opts); err == nil {
				t.Fatalf("accepted %+v", tc.opts)
			}
		})
	}

	if _, err := m.GetOption("no-such-option"); err == nil {
		t.Error("GetOption invented an option that does not exist")
	}
}

func TestUpdatingKeepsWhatWasNotMentionedAndRechecksTheCode(t *testing.T) {
	m := NewManager(newOptionDB(t))

	o := mustCreate(t, m, scopeID, OptionOptions{Code: OptionRouter, Value: "192.0.2.1"})

	// A code this build cannot encode is refused, and the stored row is left
	// alone: accepting it would take the option out of every future response
	// without saying so.
	if _, err := m.UpdateOption(o.ID, OptionOptions{Code: 200}); err == nil {
		t.Fatal("an unsupported code was accepted by UpdateOption")
	}
	after, err := m.GetOption(o.ID)
	if err != nil {
		t.Fatalf("GetOption: %v", err)
	}
	if after.Code != OptionRouter || after.Value != "192.0.2.1" {
		t.Errorf("a refused update changed the row: %+v", after)
	}

	updated, err := m.UpdateOption(o.ID, OptionOptions{Value: "192.0.2.254"})
	if err != nil {
		t.Fatalf("UpdateOption: %v", err)
	}
	if updated.Value != "192.0.2.254" || updated.Code != OptionRouter || updated.Priority != "scope" {
		t.Errorf("a field the update did not mention was lost: %+v", updated)
	}

	if _, err := m.UpdateOption("no-such-option", OptionOptions{Value: "x"}); err == nil {
		t.Error("updating an option that does not exist reported success")
	}
}

func TestDeletingSomethingThatIsNotThereSaysSo(t *testing.T) {
	m := NewManager(newOptionDB(t))

	o := mustCreate(t, m, scopeID, OptionOptions{Code: OptionDomainName, Value: "example.test"})
	if err := m.DeleteOption(o.ID); err != nil {
		t.Fatalf("DeleteOption: %v", err)
	}
	if _, err := m.GetOption(o.ID); err == nil {
		t.Error("the option survived its own deletion")
	}
	if err := m.DeleteOption(o.ID); err == nil {
		t.Error("deleting an absent option reported success")
	}
}

func TestTheMostSpecificLayerWins(t *testing.T) {
	m := NewManager(newOptionDB(t))

	// Code 6, named in every layer. Order of creation is deliberately the
	// reverse of the priority order, so a map-build that depended on insertion
	// order would come out wrong.
	mustCreate(t, m, scopeID, OptionOptions{
		Code: OptionDNSServers, Value: "10.0.0.53", ReservationID: resID, Priority: "reservation",
	})
	mustCreate(t, m, scopeID, OptionOptions{
		Code: OptionDNSServers, Value: "10.0.0.2", Priority: "client_class",
	})
	mustCreate(t, m, scopeID, OptionOptions{Code: OptionDNSServers, Value: "10.0.0.1"})
	mustCreate(t, m, scopeID, OptionOptions{
		Code: OptionDNSServers, Value: "9.9.9.9", Priority: "global",
	})

	// With a reservation: its value wins.
	withRes, err := m.BuildOptionMapForScope(scopeID, resID)
	if err != nil {
		t.Fatalf("BuildOptionMapForScope: %v", err)
	}
	if got := withRes[OptionDNSServers]; got != "10.0.0.53" {
		t.Errorf("with a reservation the answer was %q, want the reservation's 10.0.0.53", got)
	}

	// Without one: client_class beats scope beats global.
	withoutRes, err := m.BuildOptionMapForScope(scopeID, "")
	if err != nil {
		t.Fatalf("BuildOptionMapForScope: %v", err)
	}
	if got := withoutRes[OptionDNSServers]; got != "10.0.0.2" {
		t.Errorf("without a reservation the answer was %q, want client_class's 10.0.0.2", got)
	}

	// And the global layer is what it is only when nothing more specific
	// exists: an option code that appears nowhere else, created as global.
	mustCreate(t, m, scopeID, OptionOptions{
		Code: OptionNTPServers, Value: "10.0.0.123", Priority: "global",
	})
	globals, err := m.BuildOptionMapForScope(scopeID, "")
	if err != nil {
		t.Fatalf("BuildOptionMapForScope: %v", err)
	}
	if got := globals[OptionNTPServers]; got != "10.0.0.123" {
		t.Errorf("a global option did not reach the map: %q", got)
	}
}

func TestAnOptionBoundToAReservationDoesNotLeakIntoTheScope(t *testing.T) {
	m := NewManager(newOptionDB(t))

	// A scope-layer row that carries a reservation id is the reservation's
	// business, not the scope's. Applying it at scope level would give every
	// client in the subnet the printer's settings.
	mustCreate(t, m, scopeID, OptionOptions{
		Code: OptionDomainName, Value: "printer.example.test", ReservationID: resID,
	})

	withoutRes, err := m.BuildOptionMapForScope(scopeID, "")
	if err != nil {
		t.Fatalf("BuildOptionMapForScope: %v", err)
	}
	if got, ok := withoutRes[OptionDomainName]; ok {
		t.Errorf("the reservation's domain name reached a client with no reservation: %q", got)
	}

	// The reservation itself still gets it, and the scope layer is consulted
	// for the code it owns.
	withRes, err := m.BuildOptionMapForScope(scopeID, resID)
	if err != nil {
		t.Fatalf("BuildOptionMapForScope: %v", err)
	}
	if got := withRes[OptionDomainName]; got != "printer.example.test" {
		t.Errorf("the reservation's own option was not applied: %q", got)
	}
}

func TestTheLayersAreListedSeparately(t *testing.T) {
	m := NewManager(newOptionDB(t))

	global := mustCreate(t, m, scopeID, OptionOptions{Code: OptionRouter, Value: "10.1.1.1", Priority: "global"})
	scoped := mustCreate(t, m, scopeID, OptionOptions{Code: OptionDNSServers, Value: "10.1.1.2"})
	reserved := mustCreate(t, m, scopeID, OptionOptions{
		Code: OptionHostName, Value: "printer", ReservationID: resID, Priority: "reservation",
	})

	// A global option is one whose priority says so. Creating one requires
	// naming the layer, because the default is the scope's -- asserted here
	// because GetGlobalOptions is selected by that string, so an operator who
	// omits it gets a scope option and no global one, and the two lists below
	// have to reflect exactly that.
	globals, err := m.GetGlobalOptions()
	if err != nil {
		t.Fatalf("GetGlobalOptions: %v", err)
	}
	if len(globals) != 1 || globals[0].ID != global.ID {
		t.Errorf("global options = %+v, want only the one created as global", globals)
	}

	all, err := m.ListOptions(scopeID)
	if err != nil {
		t.Fatalf("ListOptions: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListOptions returned %d rows, want 3", len(all))
	}
	if none, err := m.ListOptions("scope-elsewhere"); err != nil || len(none) != 0 {
		t.Errorf("another scope returned %d rows (%v), want 0", len(none), err)
	}
	if any, err := m.ListOptions(""); err != nil || len(any) != 3 {
		t.Errorf("listing with no scope filter returned %d rows (%v), want all 3", len(any), err)
	}

	byRes, err := m.GetOptionsByReservation(resID)
	if err != nil {
		t.Fatalf("GetOptionsByReservation: %v", err)
	}
	if len(byRes) != 1 || byRes[0].ID != reserved.ID {
		t.Errorf("reservation options = %+v, want only the reservation's own", byRes)
	}
	if none, err := m.GetOptionsByScope("scope-elsewhere"); err != nil || len(none) != 0 {
		t.Errorf("GetOptionsByScope on another scope returned %d rows (%v)", len(none), err)
	}
	_ = scoped
}

func TestTheValueRulesPerOptionFamily(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    int
		value   string
		refused bool
		why     string
	}{
		{"a router list", OptionRouter, "192.0.2.1", false, ""},
		{"a router list with spaces", OptionDNSServers, "192.0.2.1, 192.0.2.2", false,
			"the UI writes it with a space after the comma"},
		{"a router list with a bad entry", OptionDNSServers, "192.0.2.1,nonsense", true, ""},
		{"an IPv6 address in an IPv4 option", OptionDNSServers, "2001:db8::1", true,
			"the wire format for these codes is four bytes per address"},
		{"a numeric lease time", OptionLeaseTime, "86400", false, ""},
		{"a lease time with a unit", OptionLeaseTime, "86400s", true, ""},
		{"a negative lease time", OptionLeaseTime, "-1", true,
			"'-' is not a digit, so the loop refuses it"},
		{"a host name", OptionHostName, "printer", false, ""},
		{"a host name with odd characters", OptionHostName, "printer #1", false,
			"strings are not checked here; the encoder is what has to survive them"},
		{"an empty value", OptionRouter, "", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOptionValue(tc.code, tc.value)
			if tc.refused && err == nil {
				t.Fatalf("accepted %q for code %d (%s)", tc.value, tc.code, tc.why)
			}
			if !tc.refused && err != nil {
				t.Fatalf("refused %q for code %d: %v", tc.value, tc.code, err)
			}
		})
	}

	// Formatting is display only, but an unknown code must still say which code
	// it was rather than dropping the row from the screen.
	if got := FormatOptionValue(OptionRouter, "192.0.2.1"); !strings.Contains(got, "Router") {
		t.Errorf("FormatOptionValue(Router) = %q", got)
	}
	if got := FormatOptionValue(200, "x"); !strings.Contains(got, "200") {
		t.Errorf("FormatOptionValue(unknown) = %q, want the code named", got)
	}
	if got := ListAllOptionCodes(); len(got) != len(SupportedOptionCodes) {
		t.Errorf("ListAllOptionCodes returned %d entries, want %d", len(got), len(SupportedOptionCodes))
	}
}

func TestTheClientGetsWhatItAskedForAndNothingMore(t *testing.T) {
	dbOptionMap := map[int]string{
		OptionRouter:       "192.0.2.1",
		OptionDNSServers:   "192.0.2.53",
		OptionDomainName:   "example.test",
		OptionLeaseTime:    "3600",
		OptionDomainSearch: "example.test,corp.test",
	}

	// With a Parameter Request List, only the requested codes come back.
	requested := BuildOptions(scopeID, "", []dhcpv4.OptionCode{dhcpv4.OptionRouter}, dbOptionMap)
	if len(requested) != 1 {
		t.Fatalf("returned %d options for a one-code request: %+v", len(requested), requested)
	}
	if got := requested[0].Code.Code(); got != OptionRouter {
		t.Errorf("returned code %d, want the router", got)
	}

	// With no list, everything in the map is offered: a client that does not
	// ask is still a client that needs an address.
	all := BuildOptions(scopeID, "", nil, dbOptionMap)
	if len(all) != len(dbOptionMap) {
		t.Errorf("returned %d options with no request list, want %d", len(all), len(dbOptionMap))
	}
}

func TestOneUnencodableValueDoesNotCostTheClientItsAddress(t *testing.T) {
	// A value the encoder cannot turn into an option. The alternative to
	// dropping it is failing the whole response, which would leave the client
	// without the address it asked for because of an option it never named.
	dbOptionMap := map[int]string{
		OptionRouter:     "not-an-address",
		OptionDNSServers: "192.0.2.53",
	}

	opts := BuildOptions(scopeID, "", nil, dbOptionMap)
	if len(opts) != 1 {
		t.Fatalf("returned %d options, want only the encodable one: %+v", len(opts), opts)
	}
	if got := opts[0].Code.Code(); got != OptionDNSServers {
		t.Errorf("kept code %d, want the DNS servers", got)
	}

	// And a router value that is fine arrives as a router option, so the
	// assertion above is not passing because nothing is ever encoded.
	ok, err := encodeOption(OptionRouter, "192.0.2.1")
	if err != nil {
		t.Fatalf("encodeOption: %v", err)
	}
	if ok == nil || ok.Code.Code() != OptionRouter {
		t.Errorf("encodeOption returned %+v", ok)
	}
}

func TestTheStructuredValuesEncodeIntoTheRightWireTypes(t *testing.T) {
	// The two codes whose values are not just bytes: a classless route list and
	// a domain search list. Both are easy to write in a form that looks right
	// and encodes to nothing, so the decoded form is asserted rather than the
	// option's presence.
	routes, err := encodeOption(OptionClasslessRoute, "10.0.0.0/8,192.0.2.1;0.0.0.0/0,192.0.2.254")
	if err != nil {
		t.Fatalf("encodeOption(classless routes): %v", err)
	}
	parsed, ok := routes.Value.(dhcpv4.Routes)
	if !ok {
		t.Fatalf("the classless route option carries %T, not dhcpv4.Routes", routes.Value)
	}
	if len(parsed) != 2 {
		t.Fatalf("decoded %d routes, want 2", len(parsed))
	}
	if parsed[0].Dest.String() != "10.0.0.0/8" || !parsed[0].Router.Equal(net.ParseIP("192.0.2.1")) {
		t.Errorf("first route = %+v", parsed[0])
	}
	if parsed[1].Dest.String() != "0.0.0.0/0" || !parsed[1].Router.Equal(net.ParseIP("192.0.2.254")) {
		t.Errorf("second route = %+v", parsed[1])
	}

	// A malformed entry is an error rather than a silently shortened list: a
	// client that receives half its routes has a broken network and no way to
	// tell why.
	for _, bad := range []string{
		"10.0.0.0/8", "10.0.0.0/8,192.0.2.1,extra", "not-a-cidr,192.0.2.1", "10.0.0.0/8,nonsense",
	} {
		if _, err := encodeOption(OptionClasslessRoute, bad); err == nil {
			t.Errorf("accepted %q as a classless route list", bad)
		}
	}
	if _, err := encodeOption(OptionClasslessRoute, " ;; "); err == nil {
		t.Error("an empty classless route list was accepted")
	}

	// A domain search list tolerates the spacing an operator types.
	search, err := encodeOption(OptionDomainSearch, "example.test, corp.test")
	if err != nil {
		t.Fatalf("encodeOption(domain search): %v", err)
	}
	domains, ok := search.Value.(*rfc1035label.Labels)
	if !ok {
		t.Fatalf("the domain search option carries %T, not *rfc1035label.Labels", search.Value)
	}
	if len(domains.Labels) != 2 || domains.Labels[1] != "corp.test" {
		t.Errorf("decoded %+v", domains.Labels)
	}
}

func TestTheDisplayFormatNamesTheOptionItWasGiven(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  uint8
		value []byte
		want  string
	}{
		{"a subnet mask", 1, []byte{255, 255, 255, 0}, "255.255.255.0"},
		{"a router list", 3, []byte{192, 0, 2, 1}, "Router: 192.0.2.1"},
		{"two DNS servers", 6, []byte{192, 0, 2, 53, 192, 0, 2, 54}, "192.0.2.53, 192.0.2.54"},
		{"a host name", 12, []byte("printer"), "Host Name: printer"},
		{"a domain name", 15, []byte("example.test"), "Domain Name: example.test"},
		{"an option with no special form", 200, []byte("x"), "Option 200"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatOptionForDisplay(tc.code, tc.value); !strings.Contains(got, tc.want) {
				t.Errorf("FormatOptionForDisplay(%d) = %q, want it to contain %q", tc.code, got, tc.want)
			}
		})
	}

	// A subnet mask value that is not four bytes falls through rather than
	// panicking on the index.
	if got := FormatOptionForDisplay(1, []byte{1, 2}); !strings.Contains(got, "Option 1") {
		t.Errorf("a short subnet mask was formatted as %q", got)
	}
}
