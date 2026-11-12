package dnswire

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func fixture() Document {
	return Document{Cluster: "11111111-1111-4111-8111-111111111111", Node: "22222222-2222-4222-8222-222222222222", Snapshot: "33333333-3333-4333-8333-333333333333", Version: "1", View: "internal", Zones: []Zone{{Name: "example.", Serial: 7, Records: []Record{
		{Owner: "www.example.", Type: "AAAA", RData: "2001:db8::1", TTL: 60}, {Owner: "example.", Type: "SOA", RData: "ns.example. hostmaster.example. 7 3600 600 86400 300", TTL: 300}, {Owner: "example.", Type: "NS", RData: "ns.example.", TTL: 300}, {Owner: "www.example.", Type: "A", RData: "192.0.2.1", TTL: 60},
	}}}}
}
func TestRoundTripImmutableAndOrder(t *testing.T) {
	d := fixture()
	s, err := Build(d)
	if err != nil {
		t.Fatal(err)
	}
	raw := s.Bytes()
	digest := sha256.Sum256(raw)
	if digest != s.Digest() {
		t.Fatal("digest")
	}
	d.Zones[0].Records[0].RData = "changed"
	raw[0] = 'x'
	if s.Bytes()[0] != '{' {
		t.Fatal("input or returned bytes mutated snapshot")
	}
	decoded, err := Decode(s.Bytes())
	if err != nil || decoded.Digest() != digest {
		t.Fatalf("decode %v", err)
	}
	copy, err := decoded.Document()
	if err != nil {
		t.Fatal(err)
	}
	copy.Zones[0].Records[0].Owner = "changed"
	again, _ := decoded.Document()
	if again.Zones[0].Records[0].Owner == "changed" {
		t.Fatal("document exposed mutable state")
	}
	f := fixture()
	f.Zones[0].Records[0], f.Zones[0].Records[1] = f.Zones[0].Records[1], f.Zones[0].Records[0]
	other, err := Build(f)
	if err != nil || !bytes.Equal(other.Bytes(), decoded.Bytes()) {
		t.Fatal("order not deterministic", err)
	}
}
func TestInvalidSemantics(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Document)
	}{
		{"upper name", func(d *Document) { d.Zones[0].Name = "Example." }}, {"outside zone", func(d *Document) { d.Zones[0].Records[0].Owner = "badexample." }},
		{"wildcard", func(d *Document) { d.Zones[0].Records[0].Owner = "*.example." }}, {"cname", func(d *Document) { d.Zones[0].Records[0].Type = "CNAME" }},
		{"cidr", func(d *Document) { d.Zones[0].Records[3].RData = "192.0.2.1/24" }}, {"mapped", func(d *Document) { d.Zones[0].Records[0].RData = "::ffff:192.0.2.1" }},
		{"ipv6 zone", func(d *Document) { d.Zones[0].Records[0].RData = "fe80::1%en0" }}, {"wrong family", func(d *Document) { d.Zones[0].Records[3].RData = "2001:db8::1" }},
		{"noncanonical ipv6", func(d *Document) { d.Zones[0].Records[0].RData = "2001:0db8::1" }}, {"ttl overflow", func(d *Document) { d.Zones[0].Records[0].TTL = 1 << 31 }},
		{"soa serial", func(d *Document) { d.Zones[0].Serial = 8 }}, {"soa overflow", func(d *Document) { d.Zones[0].Records[1].RData = "ns.example. hostmaster.example. 7 4294967296 1 1 1" }},
		{"duplicate record", func(d *Document) { d.Zones[0].Records = append(d.Zones[0].Records, d.Zones[0].Records[0]) }},
		{"rrset ttl", func(d *Document) {
			r := d.Zones[0].Records[3]
			r.RData = "192.0.2.2"
			r.TTL++
			d.Zones[0].Records = append(d.Zones[0].Records, r)
		}},
		{"duplicate zone", func(d *Document) { d.Zones = append(d.Zones, d.Zones[0]) }}, {"missing NS", func(d *Document) { d.Zones[0].Records = d.Zones[0].Records[:2] }},
		{"uuid", func(d *Document) { d.Cluster = "not-uuid" }}, {"version overflow", func(d *Document) { d.Version = "9223372036854775808" }},
		{"oversized labels", func(d *Document) { d.Zones[0].Records[0].Owner = strings.Repeat("a", 64) + ".example." }},
		{"overlong name", func(d *Document) {
			d.Zones[0].Records[0].Owner = strings.Repeat(strings.Repeat("a", 63)+".", 4) + "example."
		}},
		{"invalid UTF8", func(d *Document) { d.View = "\xff" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture()
			tc.change(&d)
			if _, err := Build(d); err == nil {
				t.Fatal("invalid document accepted")
			}
		})
	}
}
func TestStrictDecode(t *testing.T) {
	s, err := Build(fixture())
	if err != nil {
		t.Fatal(err)
	}
	raw := string(s.Bytes())
	variants := []string{raw + " ", " " + raw, strings.Replace(raw, `"view":"internal"`, `"view":"internal","view":"internal"`, 1), strings.Replace(raw, `"view":"internal"`, `"View":"internal"`, 1), strings.Replace(raw, `"view":"internal"`, `"view":"internal","unknown":true`, 1), strings.Replace(raw, `internal`, `inter\u006eal`, 1), strings.Replace(raw, `internal`, "inter\xffnal", 1), strings.Replace(raw, `"ttl":60`, `"ttl":4294967296`, 1), raw + `{}`}
	for i, v := range variants {
		if _, err := Decode([]byte(v)); err == nil {
			t.Errorf("variant %d accepted", i)
		}
	}
	if _, err := Decode(make([]byte, MaxBytes+1)); err == nil {
		t.Fatal("oversized accepted")
	}
}
func TestBounds(t *testing.T) {
	d := fixture()
	d.Zones = make([]Zone, MaxZones+1)
	if _, err := Build(d); err == nil {
		t.Fatal("too many zones")
	}
	d = fixture()
	d.Zones[0].Records = make([]Record, MaxRecords+1)
	if _, err := Build(d); err == nil {
		t.Fatal("too many records")
	}
	d = fixture()
	d.Zones[0].Records[0].Owner = strings.Repeat("a", 63) + ".example."
	if _, err := Build(d); err != nil {
		t.Fatal("63 byte label", err)
	}
}
func TestConcurrentCopies(t *testing.T) {
	s, err := Build(fixture())
	if err != nil {
		t.Fatal(err)
	}
	want := s.Digest()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				v := s.Bytes()
				v[0] = '!'
				doc, e := s.Document()
				if e != nil || s.Digest() != want {
					t.Error(fmt.Sprint("bad snapshot: ", e))
					return
				}
				doc.Zones[0].Name = "changed"
			}
		}()
	}
	wg.Wait()
}
