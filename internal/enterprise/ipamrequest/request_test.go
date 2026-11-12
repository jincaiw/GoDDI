package ipamrequest

import (
	"errors"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/httpcontract"
)

func request() Request {
	return Request{TenantID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", SubnetID: "33333333-3333-4333-8333-333333333333", Address: "2001:db8::1", OwnerRef: "host-1"}
}
func TestEquivalentAddresses(t *testing.T) {
	r := request()
	n, a, e := Normalize(r)
	if e != nil {
		t.Fatal(e)
	}
	r.Address = "2001:0DB8:0000:0000:0000:0000:0000:0001"
	m, b, e := Normalize(r)
	if e != nil || a != b || n.Address != m.Address {
		t.Fatalf("%v %v", m, e)
	}
	r.Address = "192.0.2.1"
	_, a, e = Normalize(r)
	if e != nil {
		t.Fatal(e)
	}
	r.Address = "::192.0.2.1"
	_, b, e = Normalize(r)
	if e != nil || a == b {
		t.Fatalf("families must differ: %v", e)
	}
}
func TestEachSemanticFieldChangesDigest(t *testing.T) {
	base := request()
	_, want, e := Normalize(base)
	if e != nil {
		t.Fatal(e)
	}
	tests := map[string]func(*Request){"tenant": func(r *Request) { r.TenantID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }, "space": func(r *Request) { r.SpaceID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }, "subnet": func(r *Request) { r.SubnetID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }, "address": func(r *Request) { r.Address = "2001:db8::2" }, "owner": func(r *Request) { r.OwnerRef = "host-2" }, "if-match": func(r *Request) { r.IfMatch = []string{`"v1"`} }}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			_, got, e := Normalize(r)
			if e != nil || got == want {
				t.Fatalf("unchanged digest or error: %v", e)
			}
		})
	}
	r := base
	r.IfMatch = []string{`"v1"`}
	_, a, _ := Normalize(r)
	r.IfMatch = []string{`"v2"`}
	_, b, e := Normalize(r)
	if e != nil || a == b {
		t.Fatal("revision omitted")
	}
}
func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Request)
		want   error
	}{
		{"nil uuid", func(r *Request) { r.TenantID = "00000000-0000-0000-0000-000000000000" }, ErrInvalid},
		{"uppercase uuid", func(r *Request) { r.TenantID = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA" }, ErrInvalid},
		{"mapped", func(r *Request) { r.Address = "::ffff:192.0.2.1" }, ErrInvalid},
		{"zone", func(r *Request) { r.Address = "fe80::1%eth0" }, ErrInvalid},
		{"long ip", func(r *Request) { r.Address = strings.Repeat("1", 46) }, ErrInvalid},
		{"empty owner", func(r *Request) { r.OwnerRef = "" }, ErrInvalid},
		{"long owner", func(r *Request) { r.OwnerRef = strings.Repeat("a", 256) }, ErrInvalid},
		{"unicode owner", func(r *Request) { r.OwnerRef = "é" }, ErrInvalid},
		{"control owner", func(r *Request) { r.OwnerRef = "a\n" }, ErrInvalid},
		{"empty If-Match", func(r *Request) { r.IfMatch = []string{""} }, httpcontract.ErrIfMatchInvalid},
		{"weak", func(r *Request) { r.IfMatch = []string{`W/"v1"`} }, httpcontract.ErrIfMatchInvalid},
		{"multiple", func(r *Request) { r.IfMatch = []string{`"v1"`, `"v2"`} }, httpcontract.ErrIfMatchInvalid},
		{"zero", func(r *Request) { r.IfMatch = []string{`"v0"`} }, httpcontract.ErrIfMatchInvalid},
		{"overflow", func(r *Request) { r.IfMatch = []string{`"v9223372036854775808"`} }, httpcontract.ErrIfMatchInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request()
			tc.mutate(&r)
			_, _, e := Normalize(r)
			if !errors.Is(e, tc.want) {
				t.Fatalf("got %v", e)
			}
		})
	}
	r := request()
	r.OwnerRef = strings.Repeat("a", 255)
	r.IfMatch = []string{`"v9223372036854775807"`}
	n, _, e := Normalize(r)
	if e != nil || *n.IfMatchRevision != 9223372036854775807 {
		t.Fatalf("%v %v", n, e)
	}
}
