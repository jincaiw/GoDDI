package receiptwire

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
)

const golden = `{"version":"1","tenantId":"11111111-1111-4111-8111-111111111111","principal":"user:1","method":"POST","path":"/api/addresses","key":"22222222-2222-4222-8222-222222222222","requestDigest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","status":201,"etag":"\"v1\"","body":"AP8","createdAtMs":"100","expiresAtMs":"200"}`

func sample() Value {
	return Value{Scope: idempotency.Scope{Tenant: [16]byte(uuid.MustParse("11111111-1111-4111-8111-111111111111")), Principal: "user:1", Method: "POST", Path: "/api/addresses", Key: "22222222-2222-4222-8222-222222222222"}, Receipt: idempotency.Receipt{Response: idempotency.Response{Status: 201, ETag: `"v1"`, Body: []byte{0, 255}}, CreatedAtMS: 100, ExpiresAtMS: 200}}
}
func TestGoldenAndRoundTrip(t *testing.T) {
	v := sample()
	b, err := Encode(v)
	if err != nil || string(b) != golden {
		t.Fatalf("golden: %s %v", b, err)
	}
	got, err := Decode([]byte(golden))
	if err != nil || got.Scope != v.Scope || got.RequestDigest != v.RequestDigest || got.Receipt.CreatedAtMS != 100 || got.Receipt.ExpiresAtMS != 200 || got.Receipt.Response.Status != 201 || got.Receipt.Response.ETag != v.Receipt.Response.ETag || !bytes.Equal(got.Receipt.Response.Body, v.Receipt.Response.Body) {
		t.Fatalf("round trip: %+v %v", got, err)
	}
}
func TestRejectCorruption(t *testing.T) {
	cases := []string{
		strings.Replace(golden, `"version":"1"`, `"version":"2"`, 1),
		strings.Replace(golden, `"version":"1"`, `"version":"1","version":"1"`, 1),
		strings.Replace(golden, `"version":"1"`, `"version":"1","extra":"x"`, 1),
		strings.Replace(golden, `"version":"1"`, `"Version":"1"`, 1),
		strings.Replace(golden, `"body":"AP8"`, `"body":"AP8="`, 1),
		strings.Replace(golden, `"body":"AP8"`, `"body":"AP9"`, 1),
		strings.Replace(golden, `"createdAtMs":"100"`, `"createdAtMs":"0100"`, 1),
		strings.Replace(golden, `"expiresAtMs":"200"`, `"expiresAtMs":"9223372036854775808"`, 1),
		strings.Replace(golden, `"expiresAtMs":"200"`, `"expiresAtMs":"100"`, 1),
		strings.Replace(golden, `"status":201`, `"status":99`, 1),
		strings.Replace(golden, `"status":201`, `"status":600`, 1),
		strings.Replace(golden, `"status":201`, `"status":null`, 1),
		strings.Replace(golden, `"path":"/api/addresses"`, `"path":"/api/%61ddresses"`, 1),
		strings.Replace(golden, `"tenantId":"11111111-1111-4111-8111-111111111111"`, `"tenantId":"00000000-0000-0000-0000-000000000000"`, 1),
		" " + golden, golden + `{}`, golden + "\n", string([]byte{0xff}),
	}
	for i, c := range cases {
		if _, err := Decode([]byte(c)); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}
func TestScopeAndDigestAreBound(t *testing.T) {
	v := sample()
	b, _ := Encode(v)
	cases := []func(*Value){func(v *Value) { v.Scope.Tenant[15]++ }, func(v *Value) { v.Scope.Principal = "user:2" }, func(v *Value) { v.Scope.Method = "PUT" }, func(v *Value) { v.Scope.Path = "/other" }, func(v *Value) { v.Scope.Key = "33333333-3333-4333-8333-333333333333" }, func(v *Value) { v.RequestDigest[0] = 1 }}
	for i, f := range cases {
		x := sample()
		f(&x)
		next, err := Encode(x)
		if err != nil || bytes.Equal(b, next) {
			t.Errorf("scope %d: %v", i, err)
		}
		got, err := Decode(next)
		if err != nil || got.Scope != x.Scope || got.RequestDigest != x.RequestDigest {
			t.Errorf("roundtrip %d: %v", i, err)
		}
	}
}
func TestSizeAndStatusLimits(t *testing.T) {
	v := sample()
	v.Receipt.Response.Body = make([]byte, idempotency.MaxBodyBytes)
	b, err := Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil || len(got.Receipt.Response.Body) != idempotency.MaxBodyBytes {
		t.Fatalf("large body: %v", err)
	}
	v.Receipt.Response.Body = make([]byte, idempotency.MaxBodyBytes+1)
	if _, err = Encode(v); err == nil {
		t.Fatal("oversized body")
	}
	if _, err = Decode(bytes.Repeat([]byte{'x'}, MaxEncodedBytes+1)); err == nil {
		t.Fatal("oversized JSON")
	}
	for _, status := range []int{99, 600} {
		v = sample()
		v.Receipt.Response.Status = status
		if _, err = Encode(v); err == nil {
			t.Fatalf("status %d", status)
		}
	}
	for _, status := range []int{100, 599} {
		v = sample()
		v.Receipt.Response.Status = status
		if _, err = Encode(v); err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
	}
}
