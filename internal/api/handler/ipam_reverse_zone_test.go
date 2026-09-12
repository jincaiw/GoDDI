package handler

// The reverse-zone step of the subnet console.
//
// The console used to call this endpoint, throw the answer away and report
// success. Nothing was written: the endpoint computes a name, and creating the
// zone is a separate call with a separate permission. So there are two things
// to hold down here, and only the first belongs in this file.
//
//  1. The response has to carry everything the console needs to turn the
//     computation into an action: the subnet it is about, and every reverse
//     zone the addresses could be published in, most specific first. An
//     endpoint that answered with a bare string left the console no choice but
//     to invent one, which is how it ended up reporting a write it had not
//     made.
//  2. A subnet that is not there is 404, and a subnet whose CIDR yields no
//     reverse zone at all is a refusal rather than an invented name.
//
// The second half of the flow -- that the returned name is accepted by
// POST /dns/zones and lands in dns_zones -- is deliberately not asserted here.
// CreateDNSZone reaches the zone manager through initCachedManagers, a
// sync.Once that binds the manager to whichever database it first sees, so a
// test here would poison every later test in this package that expects its own.
// That half is verified end to end against a running instance instead, which is
// also the only place the two HTTP calls the console makes are actually two.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

type reverseZoneEnvelope struct {
	Data subnet.ReverseZonePlan `json:"data"`
}

func reverseZoneRequest(subnetID string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/ipam/subnets/"+subnetID+"/generate-reverse-zone", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", subnetID)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestTheReverseZoneStepOffersEveryDelegationRatherThanDeciding(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "space-1", "subnet-1", "192.0.2.0/24")
	withIPAMServices(t, &IPAMServiceContainer{
		DB:        db,
		SubnetMgr: subnet.NewManager(db),
	})

	rec := httptest.NewRecorder()
	GenerateReverseZone(rec, reverseZoneRequest("subnet-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var env reverseZoneEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decoding the response: %v (%s)", err, rec.Body.String())
	}

	// The subnet is named, so the console draws the dialog over the row the
	// operator clicked rather than over whatever it happens to have selected.
	if env.Data.SubnetID != "subnet-1" || env.Data.CIDR != "192.0.2.0/24" {
		t.Errorf("the response is about %s/%s, want subnet-1/192.0.2.0/24",
			env.Data.SubnetID, env.Data.CIDR)
	}
	// The most specific delegation leads, because it is the one that matches
	// the subnet's own prefix.
	if env.Data.ZoneName != "2.0.192.in-addr.arpa" {
		t.Errorf("zone_name = %q, want 2.0.192.in-addr.arpa", env.Data.ZoneName)
	}
	// And the less specific ones come with it: an operator delegating
	// 192.0.0.0/16 publishes the /16, and the console cannot offer what the
	// response did not mention.
	want := []string{"2.0.192.in-addr.arpa", "0.192.in-addr.arpa", "192.in-addr.arpa"}
	if len(env.Data.Candidates) != len(want) {
		t.Fatalf("candidates = %v, want %v", env.Data.Candidates, want)
	}
	for i, name := range want {
		if env.Data.Candidates[i] != name {
			t.Errorf("candidate %d = %q, want %q", i, env.Data.Candidates[i], name)
		}
	}
}

func TestAnIPv6SubnetGetsANibbleAlignedDelegation(t *testing.T) {
	db := newControlPlaneTestDB(t)
	seedControlSubnet(t, db, "space-1", "subnet-6", "2001:db8::/32")
	withIPAMServices(t, &IPAMServiceContainer{
		DB:        db,
		SubnetMgr: subnet.NewManager(db),
	})

	rec := httptest.NewRecorder()
	GenerateReverseZone(rec, reverseZoneRequest("subnet-6"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var env reverseZoneEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	// /32 is eight nibbles, and each label is one hex character -- not four.
	if env.Data.ZoneName != "8.b.d.0.1.0.0.2.ip6.arpa" {
		t.Errorf("zone_name = %q, want 8.b.d.0.1.0.0.2.ip6.arpa", env.Data.ZoneName)
	}
}

func TestASubnetThatIsNotThereIsNotFoundRatherThanAnInventedName(t *testing.T) {
	db := newControlPlaneTestDB(t)
	withIPAMServices(t, &IPAMServiceContainer{
		DB:        db,
		SubnetMgr: subnet.NewManager(db),
	})

	rec := httptest.NewRecorder()
	GenerateReverseZone(rec, reverseZoneRequest("no-such-subnet"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
