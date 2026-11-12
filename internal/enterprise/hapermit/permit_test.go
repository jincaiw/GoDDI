package hapermit

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

const golden = `{"anchorEpoch":"1","anchorHash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","anchorSeq":"0","candidateId":"33333333-3333-4333-8333-333333333333","clusterId":"11111111-1111-4111-8111-111111111111","fencingDigest":"AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI","kind":"takeover-permit","newEpoch":"2","nonce":"44444444-4444-4444-8444-444444444444","notAfterMs":"200","notBeforeMs":"100","oldEpoch":"1","planDigest":"AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE","predecessorId":"22222222-2222-4222-8222-222222222222","requester":"requester","version":"1"}`
const sig1 = "QWamKQE9JG8sqBxq2Q9rob-qRzYfypbglKTwk1MAl6u-NuyndBDrJUXkVlY1Z6QmoMlt8ies5Uw-xee2JI6XAA"
const sig2 = "9KOGMoFVbnPx-0R_BOz-LdApCFdsjaFuy40z0NZckA_7FrqoaQup3eqzad7HqLAFpCd_iDdMoqBHKt4zghpRBw"

func fixture() ([]Approval, map[string]Approver) {
	p1, _ := hex.DecodeString("8a88e3dd7409f195fd52db2d3cba5d72ca6709bf1d94121bf3748801b40f6f5c")
	p2, _ := hex.DecodeString("8139770ea87d175f56a35466c34c7ecccb8d8a91b4ee37a25df60f5b8fc9b394")
	return []Approval{{"key1", sig1}, {"key2", sig2}}, map[string]Approver{"key1": {p1, "alice", "approver"}, "key2": {p2, "bob", "approver"}}
}
func resign(raw string) []Approval {
	out := make([]Approval, 2)
	for i := range out {
		seed := make([]byte, 32)
		for j := range seed {
			seed[j] = byte(i + 1)
		}
		k := ed25519.NewKeyFromSeed(seed)
		sig := ed25519.Sign(k, append([]byte(domain), []byte(raw)...))
		out[i] = Approval{"key" + string(rune('1'+i)), base64.RawURLEncoding.EncodeToString(sig)}
	}
	return out
}
func TestGolden(t *testing.T) {
	a, k := fixture()
	if err := Verify([]byte(golden), a, k, Bounds{100, 199}); err != nil {
		t.Fatal(err)
	}
	if err := Verify([]byte(golden), a, k, Bounds{100, 200}); err == nil {
		t.Fatal("upper bound is exclusive")
	}
}
func TestResignedInvalidBodies(t *testing.T) {
	cases := map[string]string{
		"wrong kind":         strings.Replace(golden, "takeover-permit", "freeze-proof", 1),
		"unknown version":    strings.Replace(golden, `"version":"1"`, `"version":"2"`, 1),
		"unknown field":      strings.Replace(golden, `"version":"1"`, `"version":"1","x":"y"`, 1),
		"duplicate":          strings.Replace(golden, `"version":"1"`, `"version":"1","version":"1"`, 1),
		"case alias":         strings.Replace(golden, `"version"`, `"Version"`, 1),
		"whitespace":         " " + golden,
		"invalid UTF8":       golden + string([]byte{0xff}),
		"bom":                "\xef\xbb\xbf" + golden,
		"zero epoch":         strings.Replace(golden, `"oldEpoch":"1"`, `"oldEpoch":"0"`, 1),
		"leading zero":       strings.Replace(golden, `"newEpoch":"2"`, `"newEpoch":"02"`, 1),
		"overflow":           strings.Replace(golden, `"newEpoch":"2"`, `"newEpoch":"9223372036854775808"`, 1),
		"epoch not newer":    strings.Replace(golden, `"newEpoch":"2"`, `"newEpoch":"1"`, 1),
		"bad anchor epoch":   strings.Replace(golden, `"anchorEpoch":"1"`, `"anchorEpoch":"2"`, 1),
		"bad anchor hash":    strings.Replace(golden, `"anchorSeq":"0"`, `"anchorSeq":"1"`, 1),
		"bad digest padding": strings.Replace(golden, `AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE`, `AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=`, 1),
		"same node":          strings.Replace(golden, `33333333-3333-4333-8333-333333333333`, `22222222-2222-4222-8222-222222222222`, 1),
		"empty nonce":        strings.Replace(golden, `44444444-4444-4444-8444-444444444444`, `00000000-0000-0000-0000-000000000000`, 1),
	}
	_, keys := fixture()
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Verify([]byte(raw), resign(raw), keys, Bounds{100, 199}); err == nil {
				t.Fatal("accepted invalid body")
			}
		})
	}
}
func TestApprovals(t *testing.T) {
	for name, mutate := range map[string]func(*[]Approval, map[string]Approver){
		"missing":        func(a *[]Approval, _ map[string]Approver) { *a = (*a)[:1] },
		"reversed":       func(a *[]Approval, _ map[string]Approver) { (*a)[0], (*a)[1] = (*a)[1], (*a)[0] },
		"duplicate key":  func(a *[]Approval, _ map[string]Approver) { (*a)[1] = (*a)[0] },
		"bad signature":  func(a *[]Approval, _ map[string]Approver) { (*a)[0].Signature = sig2 },
		"huge signature": func(a *[]Approval, _ map[string]Approver) { (*a)[0].Signature = strings.Repeat("A", 1<<20) },
		"unknown key":    func(a *[]Approval, _ map[string]Approver) { (*a)[1].KeyID = "key3" },
		"wrong purpose":  func(_ *[]Approval, k map[string]Approver) { v := k["key2"]; v.Purpose = "other"; k["key2"] = v },
		"same person":    func(_ *[]Approval, k map[string]Approver) { v := k["key2"]; v.Principal = "alice"; k["key2"] = v },
		"self approval":  func(_ *[]Approval, k map[string]Approver) { v := k["key2"]; v.Principal = "requester"; k["key2"] = v },
		"same key material": func(_ *[]Approval, k map[string]Approver) {
			v := k["key2"]
			v.PublicKey = k["key1"].PublicKey
			k["key2"] = v
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, k := fixture()
			mutate(&a, k)
			if Verify([]byte(golden), a, k, Bounds{100, 199}) == nil {
				t.Fatal("accepted approval")
			}
		})
	}
}
func TestTimeBounds(t *testing.T) {
	a, k := fixture()
	for _, b := range []Bounds{{99, 100}, {100, 200}, {150, 149}, {-1, 101}, {201, 202}} {
		if Verify([]byte(golden), a, k, b) == nil {
			t.Fatalf("accepted %+v", b)
		}
	}
}
