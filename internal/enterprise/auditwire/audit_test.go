package auditwire

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

const golden = `{"version":"1","clusterId":"11111111-1111-4111-8111-111111111111","epoch":"1","tenantId":"22222222-2222-4222-8222-222222222222","spaceId":"33333333-3333-4333-8333-333333333333","resourceId":"44444444-4444-4444-8444-444444444444","principal":"user:1","action":"ipam.occupancy.create","requestId":"55555555-5555-4555-8555-555555555555","priorRevision":"0","newRevision":"1","result":"SUCCESS","occurredAtMs":"100","requestDigest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`

func sample() Audit {
	return Audit{ClusterID: "11111111-1111-4111-8111-111111111111", Epoch: 1, TenantID: "22222222-2222-4222-8222-222222222222", SpaceID: "33333333-3333-4333-8333-333333333333", ResourceID: "44444444-4444-4444-8444-444444444444", Principal: "user:1", RequestID: "55555555-5555-4555-8555-555555555555", OccurredAtMS: 100}
}
func TestGolden(t *testing.T) {
	a := sample()
	b, err := Encode(a)
	if err != nil || string(b) != golden {
		t.Fatalf("%s %v", b, err)
	}
	got, err := Decode([]byte(golden))
	if err != nil || got != a {
		t.Fatalf("%+v %v", got, err)
	}
}
func TestRejectInvalid(t *testing.T) {
	cases := []string{
		strings.Replace(golden, `"version":"1"`, `"version":"2"`, 1),
		strings.Replace(golden, `"version":"1"`, `"version":"1","version":"1"`, 1),
		strings.Replace(golden, `"version":"1"`, `"version":"1","extra":"x"`, 1),
		strings.Replace(golden, `"version":"1"`, `"Version":"1"`, 1),
		strings.Replace(golden, `"epoch":"1"`, `"epoch":"01"`, 1),
		strings.Replace(golden, `"epoch":"1"`, `"epoch":"9223372036854775808"`, 1),
		strings.Replace(golden, `"epoch":"1"`, `"epoch":"0"`, 1),
		strings.Replace(golden, `"occurredAtMs":"100"`, `"occurredAtMs":"-1"`, 1),
		strings.Replace(golden, `"priorRevision":"0"`, `"priorRevision":"1"`, 1),
		strings.Replace(golden, `"newRevision":"1"`, `"newRevision":"2"`, 1),
		strings.Replace(golden, `"result":"SUCCESS"`, `"result":"FAILED"`, 1),
		strings.Replace(golden, `"action":"ipam.occupancy.create"`, `"action":"other"`, 1),
		strings.Replace(golden, `"principal":"user:1"`, `"principal":null`, 1),
		strings.Replace(golden, `"requestDigest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`, `"requestDigest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAB"`, 1),
		strings.Replace(golden, `"requestDigest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`, `"requestDigest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="`, 1),
		strings.Replace(golden, `"clusterId":"11111111-1111-4111-8111-111111111111"`, `"clusterId":"00000000-0000-0000-0000-000000000000"`, 1),
		" " + golden, golden + "\n", string([]byte{0xff}),
	}
	for i, c := range cases {
		if _, err := Decode([]byte(c)); err == nil {
			t.Errorf("accepted %d", i)
		}
	}
	if _, err := Decode(bytes.Repeat([]byte{'x'}, MaxBytes+1)); err == nil {
		t.Fatal("accepted oversized")
	}
}
func TestBoundariesAndBinding(t *testing.T) {
	a := sample()
	a.Epoch = math.MaxInt64
	a.OccurredAtMS = math.MaxInt64
	a.Principal = strings.Repeat("a", 256)
	a.RequestDigest[31] = 255
	b, err := Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decode(b)
	if err != nil || out != a {
		t.Fatalf("%+v %v", out, err)
	}
	for _, mutate := range []func(*Audit){func(a *Audit) { a.Epoch = 0 }, func(a *Audit) { a.OccurredAtMS = -1 }, func(a *Audit) { a.Principal = strings.Repeat("x", 257) }, func(a *Audit) { a.RequestID = "invalid" }} {
		x := sample()
		mutate(&x)
		if _, err := Encode(x); err == nil {
			t.Error("accepted invalid")
		}
	}
	x := sample()
	x.Epoch = 2
	b1, _ := Encode(sample())
	b2, _ := Encode(x)
	if bytes.Equal(b1, b2) {
		t.Fatal("epoch not bound")
	}
}
