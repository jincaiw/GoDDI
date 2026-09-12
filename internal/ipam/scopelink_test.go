package ipam

// The scope plan is the moment IPAM and DHCP are compared *before* anything is
// created, so these cases are about the places the two subsystems disagree
// silently: a pool that reaches an address IPAM fenced off, a router nobody
// chose, a duplicate scope.
//
// The optimistic check has two failure modes and both are tested. One is
// accepting a plan whose world moved (a duplicate gets created). The other is
// refusing a plan whose world did not move in any way that matters (operators
// learn to re-preview without reading, and the check becomes decoration), so
// the "does not move" cases are here too.

import (
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

func TestThePlanProposesTheRangeAndSaysWhereTheRouterCameFrom(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.StartIP != "192.0.2.1" || plan.EndIP != "192.0.2.254" {
		t.Errorf("range = %s..%s, want 192.0.2.1..192.0.2.254", plan.StartIP, plan.EndIP)
	}
	// Nobody marked a gateway, so the plan fell back to the first usable
	// address -- and has to say so rather than pass the guess off as a fact.
	if plan.RouterSource != RouterFromConvention || plan.Router != "192.0.2.1" {
		t.Errorf("router = %q (%s), want the conventional 192.0.2.1",
			plan.Router, plan.RouterSource)
	}
	if !hasConflict(plan.Warnings, "no address is marked gateway") {
		t.Errorf("the fallback router was not declared as a fallback: %v", plan.Warnings)
	}
	if plan.Fingerprint == "" {
		t.Error("the plan has no fingerprint, so nothing can be checked against it")
	}
}

func TestAPlanUsesTheGatewaySomebodyMarked(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad-gw", "sp1", "sn1", "192.0.2.254", address.StatusGateway)

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Router != "192.0.2.254" || plan.RouterSource != RouterFromGateway {
		t.Errorf("router = %q (%s), want the marked gateway 192.0.2.254",
			plan.Router, plan.RouterSource)
	}
	if hasConflict(plan.Warnings, "no address is marked gateway") {
		t.Errorf("a named gateway was reported as a guess: %v", plan.Warnings)
	}
	// The gateway sits inside the proposed range, which the scope cannot
	// exclude: worth saying, because ping check is all that protects it.
	if !hasConflict(plan.Warnings, "inside the proposed range") {
		t.Errorf("a gateway inside the pool was not mentioned: %v", plan.Warnings)
	}
}

// TestTwoGatewaysAreARefusalNotAPick. Nothing in the schema stops two rows
// being marked gateway. Silently taking one is how the wrong router ends up on
// every client; the plan says the choice is undecided -- and still picks
// deterministically, so the fingerprint does not wobble between two requests
// that saw the same state.
func TestTwoGatewaysAreARefusalNotAPick(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	// Text order puts 192.0.2.10 before 192.0.2.9; address order does not.
	seedAddress(t, db, "ad-gw-a", "sp1", "sn1", "192.0.2.9", address.StatusGateway)
	seedAddress(t, db, "ad-gw-b", "sp1", "sn1", "192.0.2.10", address.StatusGateway)

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !hasConflict(plan.Conflicts, "2 addresses are marked gateway") {
		t.Errorf("two gateways were not reported as undecided: %v", plan.Conflicts)
	}
	if plan.Router != "192.0.2.9" {
		t.Errorf("router = %q, want 192.0.2.9: the list is ordered by address, not by text",
			plan.Router)
	}
}

// TestThePlanListsTheFencedAddressesTheRangeWouldReach. The list is the part
// an operator has to act on, because a DHCP scope carries no exclusion list:
// the allocator will offer these.
func TestThePlanListsTheFencedAddressesTheRangeWouldReach(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedAddress(t, db, "ad-res", "sp1", "sn1", "192.0.2.50", address.StatusReserved)
	seedAddress(t, db, "ad-exc", "sp1", "sn1", "192.0.2.200", address.StatusExcluded)
	seedAddress(t, db, "ad-out", "sp1", "sn1", "192.0.2.0", address.StatusReserved)
	// Inside the subnet but outside the pool, and not a host address either.
	seedAddress(t, db, "ad-free", "sp1", "sn1", "192.0.2.60", address.StatusAvailable)

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Excluded) != 2 {
		t.Fatalf("excluded = %+v, want the two inside the range", plan.Excluded)
	}
	// Ordered by address, so 192.0.2.50 comes before 192.0.2.200 even though
	// that is not text order.
	if plan.Excluded[0].IP != "192.0.2.50" || plan.Excluded[1].IP != "192.0.2.200" {
		t.Errorf("excluded order = %s, %s", plan.Excluded[0].IP, plan.Excluded[1].IP)
	}
	if !hasConflict(plan.Warnings, "no exclusion list") {
		t.Errorf("the consequence was not spelled out: %v", plan.Warnings)
	}
}

func TestAnExistingScopeOnTheSameAddressesIsAConflict(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedScope(t, db, "sc1", "office", "192.0.2.0/24", "192.0.2.10", "192.0.2.100", "", true)

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !hasConflict(plan.Conflicts, `scope "office" already hands out`) {
		t.Errorf("an overlapping live scope was not a conflict: %v", plan.Conflicts)
	}
}

// TestADisabledScopeIsAWarningAndAMismatchedPoolIsAWarning.
//
// A disabled scope hands out nothing, so it is not a reason to refuse;
// enabling it later would be, and that is what the warning is for.
//
// The second case is a scope that declares this subnet but pools addresses
// from another one. It cannot overlap (the two are in different networks), and
// a console grouping scopes by their declared subnet shows it as serving this
// one -- so it is still this plan's business, which is exactly why relevance
// cannot be decided by overlap alone.
func TestADisabledScopeIsAWarningAndAMismatchedPoolIsAWarning(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedScope(t, db, "sc-off", "off", "192.0.2.0/24", "192.0.2.10", "192.0.2.100", "", false)
	seedScope(t, db, "sc-mis", "mismatched", "192.0.2.0/24", "198.51.100.10", "198.51.100.20", "", true)

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Conflicts) != 0 {
		t.Errorf("nothing here is a reason to refuse: %v", plan.Conflicts)
	}
	if !hasConflict(plan.Warnings, `scope "off" is disabled`) {
		t.Errorf("a disabled overlapping scope was not mentioned: %v", plan.Warnings)
	}
	if !hasConflict(plan.Warnings, `scope "mismatched" already serves subnet`) {
		t.Errorf("a scope declaring this subnet with a pool elsewhere was not mentioned: %v",
			plan.Warnings)
	}
}

// TestAScopeElsewhereIsNotThisPlansBusiness. What the plan reads and what the
// fingerprint covers are the same set, so this case is what keeps an unrelated
// change from invalidating a plan.
func TestAScopeElsewhereIsNotThisPlansBusiness(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	seedScope(t, db, "sc-else", "elsewhere", "198.51.100.0/24", "198.51.100.10", "198.51.100.20", "", true)

	link := NewLinkage(db)
	plan, err := link.PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Conflicts) != 0 || len(plan.Warnings) != 1 {
		t.Errorf("a scope on another subnet leaked into the plan: %+v / %+v",
			plan.Conflicts, plan.Warnings)
	}

	// The reporting above would stay quiet about an unrelated scope even if
	// the plan read all of them, because none of the three cases matches it.
	// Relevance is what keeps it out of the fingerprint, so that is asserted
	// directly rather than inferred from the absence of a message.
	all, _, err := link.readScopes()
	if err != nil {
		t.Fatalf("read scopes: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("read %d scopes, want the one seeded", len(all))
	}
	if relevant := relevantScopes(all, plan.CIDR, plan.StartIP, plan.EndIP); len(relevant) != 0 {
		t.Errorf("a scope on another subnet was treated as relevant to this plan: %+v", relevant)
	}
}

// TestThePlanAndTheDraftProposeTheSameRange. The plan is not the thing that
// creates the scope, so the two must agree about the range or the console
// previews one pool and creates another.
func TestThePlanAndTheDraftProposeTheSameRange(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	draft, err := subnet.NewManager(db).GenerateDHCPScope("sn1")
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if plan.StartIP != draft.StartIP || plan.EndIP != draft.EndIP {
		t.Errorf("plan %s..%s but draft %s..%s: the preview and the created scope disagree",
			plan.StartIP, plan.EndIP, draft.StartIP, draft.EndIP)
	}
	if plan.CIDR != draft.Subnet {
		t.Errorf("plan subnet %s but draft subnet %s", plan.CIDR, draft.Subnet)
	}
}

// --- the optimistic check -------------------------------------------------

// TestAPlanIsRefusedOnceThePoolMoves is the reason the fingerprint exists: a
// scope created between the preview and the confirmation turns the approved
// plan into a duplicate.
func TestAPlanIsRefusedOnceThePoolMoves(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	link := NewLinkage(db)

	plan, err := link.PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := link.CheckDHCPScopePlan("sn1", plan.Fingerprint); err != nil {
		t.Fatalf("a plan was refused against the state it was just built from: %v", err)
	}

	seedScope(t, db, "sc-new", "new", "192.0.2.0/24", "192.0.2.10", "192.0.2.100", "", true)

	err = link.CheckDHCPScopePlan("sn1", plan.Fingerprint)
	if !errors.Is(err, ErrDHCPScopePlanStale) {
		t.Fatalf("check = %v, want ErrDHCPScopePlanStale", err)
	}
	var stale *PlanStaleError
	if !errors.As(err, &stale) {
		t.Fatalf("the refusal does not carry the two fingerprints: %v", err)
	}
	if stale.Expected != plan.Fingerprint || stale.Actual == plan.Fingerprint || stale.Actual == "" {
		t.Errorf("fingerprints = %q -> %q, want the previewed one and a different current one",
			stale.Expected, stale.Actual)
	}
}

// TestAPlanIsRefusedWhenAnAddressIsFenced. The other way the approved plan
// stops being the plan: somebody fences an address inside the range.
func TestAPlanIsRefusedWhenAnAddressIsFenced(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	link := NewLinkage(db)

	plan, err := link.PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	seedAddress(t, db, "ad-new", "sp1", "sn1", "192.0.2.77", address.StatusStatic)

	if err := link.CheckDHCPScopePlan("sn1", plan.Fingerprint); !errors.Is(err, ErrDHCPScopePlanStale) {
		t.Fatalf("check = %v, want ErrDHCPScopePlanStale", err)
	}
}

// TestTheCheckDoesNotFireForChangesNobodyReviewed. The dangerous direction of
// an optimistic check is the other one: a fingerprint over "everything about
// the subnet" refuses for a lease that came and went, and a check that refuses
// for reasons the operator cannot see is a check they learn to work around.
func TestTheCheckDoesNotFireForChangesNobodyReviewed(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	link := NewLinkage(db)

	plan, err := link.PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	// An address handed out by DHCP inside the range: the normal life of a
	// pool, and not something the plan's decision hangs on.
	seedAddress(t, db, "ad-live", "sp1", "sn1", "192.0.2.30", address.StatusDHCP)
	// A scope on a different subnet, which cannot become this plan's problem.
	seedScope(t, db, "sc-else", "elsewhere", "198.51.100.0/24", "198.51.100.10", "198.51.100.20", "", true)
	// A fenced address just outside the pool. The boundary is the point: the
	// sibling case in TestAPlanIsRefusedWhenAnAddressIsFenced fences an
	// address inside the range and must be refused, so together the two pin
	// where the range ends.
	seedAddress(t, db, "ad-out", "sp1", "sn1", "192.0.2.255", address.StatusReserved)

	if err := link.CheckDHCPScopePlan("sn1", plan.Fingerprint); err != nil {
		t.Errorf("a plan was refused for a change that cannot alter it: %v", err)
	}
}

// TestAnUnknownSubnetIsNotFoundRatherThanAPlan. A plan for a subnet that does
// not exist must be reported as a missing subnet, not as a plan with an empty
// range that a caller might act on.
func TestAnUnknownSubnetIsNotFoundRatherThanAPlan(t *testing.T) {
	db := newTestDB(t)

	plan, err := NewLinkage(db).PlanDHCPScope("nope")
	if plan != nil {
		t.Fatalf("a plan was produced for a subnet that does not exist: %+v", plan)
	}
	if !errors.Is(err, subnet.ErrSubnetNotFound) {
		t.Fatalf("err = %v, want ErrSubnetNotFound", err)
	}
}

// TestAnIPv6SubnetHasNoDHCPPlan. DHCPv6 is out of scope, so the answer is a
// refusal a handler can turn into a 400 -- not an IPv4 plan computed from an
// IPv6 address.
func TestAnIPv6SubnetHasNoDHCPPlan(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "2001:db8::/64")

	plan, err := NewLinkage(db).PlanDHCPScope("sn1")
	if plan != nil {
		t.Fatalf("an IPv6 subnet produced a plan: %+v", plan)
	}
	if !errors.Is(err, subnet.ErrDHCPv6Unsupported) {
		t.Fatalf("err = %v, want ErrDHCPv6Unsupported", err)
	}
}
