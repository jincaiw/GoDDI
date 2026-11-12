package hapermit

import (
	"errors"
	"strings"
	"testing"
)

func expectedFixture() Expected {
	var plan, fence [32]byte
	for i := range plan {
		plan[i] = 1
		fence[i] = 2
	}
	return Expected{ClusterID: "11111111-1111-4111-8111-111111111111", PredecessorID: "22222222-2222-4222-8222-222222222222", CandidateID: "33333333-3333-4333-8333-333333333333", OldEpoch: 1, NewEpoch: 2, AnchorEpoch: 1, AnchorSeq: 0, PlanDigest: plan, FencingDigest: fence, Nonce: "44444444-4444-4444-8444-444444444444", Requester: "requester"}
}
func TestVerifyExpectedGoldenAndIsolation(t *testing.T) {
	a, k := fixture()
	expected := expectedFixture()
	raw := []byte(golden)
	claims, err := VerifyExpected(raw, a, k, Bounds{100, 199}, expected)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Expected != expected || claims.NotBeforeMS != 100 || claims.NotAfterMS != 200 {
		t.Fatalf("claims %+v", claims)
	}
	raw[0] = 'x'
	expected.PlanDigest[0] = 9
	a[0].Signature = "bad"
	if claims.PlanDigest[0] != 1 || claims.ClusterID != "11111111-1111-4111-8111-111111111111" {
		t.Fatal("claims alias inputs")
	}
	// The verifier is stateless and cannot detect repeated nonce use.
	a, k = fixture()
	if _, err := VerifyExpected([]byte(golden), a, k, Bounds{100, 199}, expectedFixture()); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyExpected([]byte(golden), a, k, Bounds{100, 199}, expectedFixture()); err != nil {
		t.Fatal(err)
	}
}
func TestVerifyExpectedMismatchEveryField(t *testing.T) {
	tests := map[string]func(*Expected){
		"cluster":     func(e *Expected) { e.ClusterID = "22222222-2222-4222-8222-222222222222" },
		"predecessor": func(e *Expected) { e.PredecessorID = "11111111-1111-4111-8111-111111111111" },
		"candidate":   func(e *Expected) { e.CandidateID = "11111111-1111-4111-8111-111111111111" },
		"old epoch":   func(e *Expected) { e.OldEpoch++ }, "new epoch": func(e *Expected) { e.NewEpoch++ },
		"anchor epoch": func(e *Expected) { e.AnchorEpoch++ }, "anchor seq": func(e *Expected) { e.AnchorSeq++ },
		"anchor hash": func(e *Expected) { e.AnchorHash[0] = 1 }, "plan": func(e *Expected) { e.PlanDigest[0] ^= 1 },
		"fencing": func(e *Expected) { e.FencingDigest[0] ^= 1 }, "nonce": func(e *Expected) { e.Nonce = "55555555-5555-4555-8555-555555555555" },
		"requester": func(e *Expected) { e.Requester = "other" }, "noncanonical": func(e *Expected) { e.ClusterID = strings.ToUpper("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa") },
		"zero context": func(e *Expected) { *e = Expected{} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			a, k := fixture()
			e := expectedFixture()
			mutate(&e)
			claims, err := VerifyExpected([]byte(golden), a, k, Bounds{100, 199}, e)
			if !errors.Is(err, ErrInvalid) || claims != (Claims{}) {
				t.Fatalf("got %+v %v", claims, err)
			}
		})
	}
}
func TestEmptyEpochZeroAnchor(t *testing.T) {
	raw := strings.Replace(golden, `"anchorEpoch":"1"`, `"anchorEpoch":"2"`, 1)
	raw = strings.Replace(raw, `"oldEpoch":"1"`, `"oldEpoch":"2"`, 1)
	raw = strings.Replace(raw, `"newEpoch":"2"`, `"newEpoch":"3"`, 1)
	a, keys := resign(raw), func() map[string]Approver { _, k := fixture(); return k }()
	e := expectedFixture()
	e.OldEpoch, e.NewEpoch, e.AnchorEpoch = 2, 3, 2
	if _, err := VerifyExpected([]byte(raw), a, keys, Bounds{100, 199}, e); err != nil {
		t.Fatal(err)
	}
	e.AnchorHash[0] = 1
	if _, err := VerifyExpected([]byte(raw), a, keys, Bounds{100, 199}, e); !errors.Is(err, ErrInvalid) {
		t.Fatalf("untrusted anchor accepted: %v", err)
	}
}

func TestVerifyExpectedRejectsBadSignatureAndTime(t *testing.T) {
	a, k := fixture()
	a[0].Signature = sig2
	if c, err := VerifyExpected([]byte(golden), a, k, Bounds{100, 199}, expectedFixture()); !errors.Is(err, ErrInvalid) || c != (Claims{}) {
		t.Fatalf("%+v %v", c, err)
	}
	a, k = fixture()
	if c, err := VerifyExpected([]byte(golden), a, k, Bounds{100, 200}, expectedFixture()); !errors.Is(err, ErrInvalid) || c != (Claims{}) {
		t.Fatalf("%+v %v", c, err)
	}
}
