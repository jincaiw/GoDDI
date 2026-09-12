package subnet

// Where a subnet's addresses would be published in reverse DNS.
//
// The console turns this into a real zone, so the plan is the difference
// between an operator choosing a delegation and the console reporting that it
// created one. Two things are pinned here: the list is ordered most specific
// first and carries the alternatives, and the bare roots are not on it.

import (
	"errors"
	"testing"
)

func TestThePlanOffersTheDelegationsButNotTheRoot(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)

	s, err := m.CreateSubnet("sp1", "office", "192.0.2.0/24", SubnetOptions{})
	if err != nil {
		t.Fatalf("CreateSubnet: %v", err)
	}

	plan, err := m.PlanReverseZone(s.ID)
	if err != nil {
		t.Fatalf("PlanReverseZone: %v", err)
	}
	if plan.SubnetID != s.ID || plan.CIDR != "192.0.2.0/24" {
		t.Errorf("the plan is about %s/%s, want the subnet it was asked about", plan.SubnetID, plan.CIDR)
	}

	want := []string{"2.0.192.in-addr.arpa", "0.192.in-addr.arpa", "192.in-addr.arpa"}
	if len(plan.Candidates) != len(want) {
		t.Fatalf("candidates = %v, want %v", plan.Candidates, want)
	}
	for i, name := range want {
		if plan.Candidates[i] != name {
			t.Errorf("candidate %d = %q, want %q", i, plan.Candidates[i], name)
		}
	}
	// The most specific one is the single answer the older method gives, and it
	// is the one the console preselects.
	if plan.ZoneName != want[0] {
		t.Errorf("zone_name = %q, want the most specific candidate %q", plan.ZoneName, want[0])
	}

	// What must not be on the list is the root of the reverse tree. It is a
	// legitimate answer to "which zones could contain these addresses", which
	// is what ReverseZoneCandidates is for -- but creating in-addr.arpa locally
	// takes over reverse resolution for everything, and every IPv4 subnet would
	// otherwise end with it as the last option.
	for _, name := range plan.Candidates {
		if name == "in-addr.arpa" {
			t.Errorf("candidates = %v, want the reverse tree's root left out", plan.Candidates)
		}
	}
}

func TestAnIPv6SubnetIsOfferedNibbleAlignedDelegationsWithoutTheRoot(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)

	s, err := m.CreateSubnet("sp1", "v6", "2001:db8::/32", SubnetOptions{})
	if err != nil {
		t.Fatalf("CreateSubnet: %v", err)
	}
	plan, err := m.PlanReverseZone(s.ID)
	if err != nil {
		t.Fatalf("PlanReverseZone: %v", err)
	}

	// /32 is eight nibbles, and each label is one hex character -- not four.
	if plan.ZoneName != "8.b.d.0.1.0.0.2.ip6.arpa" {
		t.Errorf("zone_name = %q, want 8.b.d.0.1.0.0.2.ip6.arpa", plan.ZoneName)
	}
	// Eight nibbles down to one, and then the root, which is dropped. Without
	// the filter this would be nine entries and the last one would be ip6.arpa.
	if len(plan.Candidates) != 8 {
		t.Errorf("candidates = %v, want the eight nibble prefixes", plan.Candidates)
	}
	if last := plan.Candidates[len(plan.Candidates)-1]; last != "2.ip6.arpa" {
		t.Errorf("the least specific candidate is %q, want 2.ip6.arpa -- ip6.arpa is not a delegation", last)
	}
}

func TestTheSingleAnswerAndThePlanCannotDrift(t *testing.T) {
	db := newTestDB(t)
	seedSpace(t, db, "sp1")
	m := NewManager(db)

	// Two callers, one decision. GenerateReverseZone is what the old console
	// used and PlanReverseZone is what the new one uses; if the two ever picked
	// differently, the name shown to the operator would not be the name the
	// other path would create.
	for i, cidr := range []string{"198.51.0.0/16", "10.0.0.0/8", "2001:db8::/32"} {
		s, err := m.CreateSubnet("sp1", "r"+itoa(i), cidr, SubnetOptions{})
		if err != nil {
			t.Fatalf("create %s: %v", cidr, err)
		}
		single, err := m.GenerateReverseZone(s.ID)
		if err != nil {
			t.Fatalf("GenerateReverseZone(%s): %v", cidr, err)
		}
		plan, err := m.PlanReverseZone(s.ID)
		if err != nil {
			t.Fatalf("PlanReverseZone(%s): %v", cidr, err)
		}
		if single != plan.ZoneName {
			t.Errorf("%s: GenerateReverseZone = %q but the plan leads with %q", cidr, single, plan.ZoneName)
		}
	}
}

func TestAPlanForASubnetThatIsNotThereIsNotFound(t *testing.T) {
	db := newTestDB(t)
	m := NewManager(db)

	// Not an invented name and not a generic failure: the console has to be able
	// to tell a stale row from a data problem, because only one of them is worth
	// refreshing the page for.
	if _, err := m.PlanReverseZone("no-such-subnet"); !errors.Is(err, ErrSubnetNotFound) {
		t.Fatalf("err = %v, want ErrSubnetNotFound", err)
	}
}
