package address

import (
	"errors"
	"testing"
)

func TestNormalizeIP(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"192.0.2.1", "192.0.2.1", false},
		{"  192.0.2.1  ", "192.0.2.1", false},
		// IPv6 has many spellings of one address. Without canonicalisation the
		// (space_id, ip_address) index cannot tell them apart.
		{"2001:0db8:0000:0000:0000:0000:0000:0001", "2001:db8::1", false},
		{"2001:db8::1", "2001:db8::1", false},
		{"0:0:0:0:0:0:0:1", "::1", false},
		// The IPv4-mapped form must collapse onto the IPv4 address it denotes.
		{"::ffff:192.0.2.5", "192.0.2.5", false},
		// Leading zeros are refused rather than reinterpreted: the string was
		// historically octal, and guessing the base is how one address becomes
		// two.
		{"192.000.002.001", "", true},
		{"192.168.001.001", "", true},
		{"", "", true},
		{"not-an-ip", "", true},
		{"300.1.1.1", "", true},
		{"192.0.2", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeIP(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizeIP(%q) = %q, want an error", c.in, got)
			} else if !errors.Is(err, ErrInvalidIP) {
				t.Errorf("NormalizeIP(%q) error = %v, want ErrInvalidIP", c.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeIP(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseIP_RejectsOutOfSubnet(t *testing.T) {
	if _, _, _, err := ParseIP("192.0.3.1", "192.0.2.0/24"); !errors.Is(err, ErrOutOfSubnet) {
		t.Fatalf("err = %v, want ErrOutOfSubnet", err)
	}
	if _, _, _, err := ParseIP("192.0.2.1", "192.0.2.0/24"); err != nil {
		t.Fatalf("in-subnet address rejected: %v", err)
	}
	// A v4-mapped literal must be checked against the IPv4 subnet after
	// canonicalisation, not rejected as a foreign address family.
	if _, _, _, err := ParseIP("::ffff:192.0.2.9", "192.0.2.0/24"); err != nil {
		t.Fatalf("v4-mapped address rejected: %v", err)
	}
}

func TestCanTransition(t *testing.T) {
	allowed := []struct{ from, to Status }{
		{StatusAvailable, StatusUsed},
		{StatusAvailable, StatusDHCP},
		{StatusAvailable, StatusReserved},
		{StatusAvailable, StatusGateway},
		{StatusDHCP, StatusAvailable},
		{StatusDHCP, StatusStatic},
		{StatusStatic, StatusDHCP},
		{StatusReserved, StatusDHCP},
		{StatusConflict, StatusAvailable},
		{StatusUnknown, StatusGateway},
	}
	for _, c := range allowed {
		if !CanTransition(c.from, c.to) {
			t.Errorf("CanTransition(%s, %s) = false, want true", c.from, c.to)
		}
	}

	refused := []struct{ from, to Status }{
		// Same status is not a transition. Accepting it would let callers
		// record no-op events in the history that look like changes.
		{StatusAvailable, StatusAvailable},
		{StatusDHCP, StatusDHCP},
		// A structural role must not become an ordinary allocation: this is
		// the failure where the address fenced off as the gateway is handed
		// to a DHCP client.
		{StatusGateway, StatusDHCP},
		{StatusGateway, StatusStatic},
		{StatusGateway, StatusUsed},
		{StatusExcluded, StatusDHCP},
		{StatusExcluded, StatusStatic},
		// The reverse: an address already bound to a client must not silently
		// become the subnet's gateway.
		{StatusDHCP, StatusGateway},
		{StatusStatic, StatusGateway},
		{StatusUsed, StatusGateway},
		{StatusReserved, StatusGateway},
		// A reserved address is not given away by a status flip.
		{StatusReserved, StatusExcluded},
		{StatusGateway, StatusReserved},
		// A conflict is resolved explicitly, never by drifting back.
		{StatusConflict, StatusConflict},
	}
	for _, c := range refused {
		if CanTransition(c.from, c.to) {
			t.Errorf("CanTransition(%s, %s) = true, want false", c.from, c.to)
		}
	}
}

func TestStatusProperties(t *testing.T) {
	if !StatusAvailable.Allocatable() {
		t.Error("available must be allocatable")
	}
	for _, s := range []Status{StatusUsed, StatusDHCP, StatusStatic, StatusReserved,
		StatusGateway, StatusExcluded, StatusConflict, StatusUnknown} {
		if s.Allocatable() {
			t.Errorf("%s must not be allocatable", s)
		}
	}
	if !StatusGateway.Structural() || !StatusExcluded.Structural() {
		t.Error("gateway and excluded are structural")
	}
	if StatusDHCP.Structural() {
		t.Error("dhcp is not structural")
	}
	// Every status reachable from the table must itself be a known status, or
	// the table contains a typo that only shows up at runtime.
	for _, from := range Statuses() {
		if !from.Valid() {
			t.Errorf("%q is in allStatuses but not Valid()", from)
		}
		for _, to := range AllowedTransitions(from) {
			if !to.Valid() {
				t.Errorf("transition %s -> %q targets an unknown status", from, to)
			}
		}
	}
}

func TestPlanLeaseObservation(t *testing.T) {
	cases := []struct {
		name        string
		current     Status
		observed    ObservedState
		wantStatus  Status
		wantChanged bool
	}{
		{"unallocated address becomes dhcp", StatusAvailable, ObservedInUse, StatusDHCP, true},
		{"unclassified address becomes dhcp", StatusUnknown, ObservedInUse, StatusDHCP, true},
		{"existing dhcp allocation is left alone", StatusDHCP, ObservedInUse, StatusDHCP, false},
		{"static allocation survives a dhcp renewal", StatusStatic, ObservedInUse, StatusStatic, false},
		// The address was fenced off but a lease went out anyway: both facts
		// are recorded, with the conflict visible.
		{"reserved address handed out is a conflict", StatusReserved, ObservedInUse, StatusConflict, true},
		{"excluded address handed out is a conflict", StatusExcluded, ObservedInUse, StatusConflict, true},
		{"gateway handed out is a conflict", StatusGateway, ObservedInUse, StatusConflict, true},
		{"conflict is not resolved by a lease", StatusConflict, ObservedInUse, StatusConflict, false},
		// A free observation never frees an allocation.
		{"free does not release an allocation", StatusDHCP, ObservedFree, StatusDHCP, false},
		{"free does not change an available address", StatusAvailable, ObservedFree, StatusAvailable, false},
		// A contested report is a second claimant, not a binding.
		{"contested address becomes a conflict", StatusAvailable, ObservedContested, StatusConflict, true},
		{"contested dhcp allocation becomes a conflict", StatusDHCP, ObservedContested, StatusConflict, true},
		{"already conflicted stays conflicted", StatusConflict, ObservedContested, StatusConflict, false},
		// An excluded address is already out of the pool; raising it to
		// conflict would lose the stronger statement.
		{"excluded stays excluded when contested", StatusExcluded, ObservedContested, StatusExcluded, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed, reason := PlanLeaseObservation(c.current, c.observed)
			if got != c.wantStatus || changed != c.wantChanged {
				t.Fatalf("PlanLeaseObservation(%s, %s) = (%s, %v), want (%s, %v)",
					c.current, c.observed, got, changed, c.wantStatus, c.wantChanged)
			}
			if changed && reason == "" {
				t.Error("a changed status must carry a reason for the history")
			}
			if !changed && reason != "" {
				t.Errorf("an unchanged status produced a reason %q", reason)
			}
		})
	}
}

func TestObservedStateValid(t *testing.T) {
	for _, s := range []ObservedState{ObservedUnknown, ObservedInUse, ObservedFree, ObservedContested} {
		if !s.Valid() {
			t.Errorf("%s should be valid", s)
		}
	}
	if ObservedState("nonsense").Valid() {
		t.Error("an unknown observation state was accepted")
	}
}

// TestFencedStatusesAreNarrowerThanNotAllocatable.
//
// The two sets answer different questions and conflating them is a defect in
// both directions. A row marked dhcp inside a pool is the normal life of a
// pool -- the allocator's own lease table is what stops it handing the address
// out twice -- so calling it fenced would make every busy subnet report a
// conflict. A row marked reserved is a decision recorded only in this column,
// which the allocator cannot see, so leaving it out of the set is how a client
// ends up holding a withheld address.
func TestFencedStatusesAreNarrowerThanNotAllocatable(t *testing.T) {
	want := map[Status]bool{
		StatusGateway:  true,
		StatusExcluded: true,
		StatusReserved: true,
		StatusStatic:   true,
		StatusConflict: true,
		// Not fenced, although none of them is allocatable either.
		StatusAvailable: false,
		StatusUsed:      false,
		StatusDHCP:      false,
		StatusUnknown:   false,
	}

	for _, s := range Statuses() {
		expected, listed := want[s]
		if !listed {
			t.Errorf("status %q is not covered by this test's table", s)
			continue
		}
		if got := IsFenced(s); got != expected {
			t.Errorf("IsFenced(%q) = %v, want %v", s, got, expected)
		}
	}

	listed := FencedStatuses()
	if len(listed) != 5 {
		t.Errorf("FencedStatuses() = %v, want the five fenced states", listed)
	}
	// The set is handed to SQL as an IN list and to a switch as a predicate,
	// so a caller must not be able to mutate it.
	listed[0] = StatusAvailable
	if !IsFenced(StatusGateway) {
		t.Error("FencedStatuses returned the set itself, so a caller can disarm it")
	}
}

// TestTheFencedSetIsNotTheAllocatableSet guards the difference the name does
// not make obvious.
func TestTheFencedSetIsNotTheAllocatableSet(t *testing.T) {
	for _, s := range FencedStatuses() {
		if s.Allocatable() {
			t.Errorf("%q is fenced and allocatable at once", s)
		}
	}
	if len(FencedStatuses()) == len(Statuses()) {
		t.Error("every status is fenced, which is the same as having no set")
	}
}
