package client

import (
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// validateUpstreamAddress is the SSRF boundary for the diagnostic DNS client.
// Loopback, link-local and RFC1918 targets must be refused in every spelling
// the caller can produce; public IPs and hostnames must remain usable.
func TestValidateUpstreamAddress(t *testing.T) {
	for _, tc := range []struct {
		name    string
		addr    string
		wantErr bool
	}{
		// Refused: loopback, with and without a port and a trailing FQDN dot.
		{"loopback v4", "127.0.0.1", true},
		{"loopback v4 with port", "127.0.0.1:53", true},
		{"loopback v4 trailing dot", "127.0.0.1.", true},
		{"loopback v6", "::1", true},
		{"loopback v6 with port", "[::1]:53", true},
		// Refused: link-local.
		{"link-local v4", "169.254.1.1", true},
		{"link-local v4 with port", "169.254.1.1:5353", true},
		{"link-local v6", "fe80::1", true},
		// Refused: unspecified ("this host") and link-local multicast. These
		// were accepted before the guard was aligned with handler.isPrivateIP.
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v4 with port", "0.0.0.0:53", true},
		{"unspecified v6", "::", true},
		{"link-local multicast v4", "224.0.0.1", true},
		{"link-local multicast v6", "ff02::1", true},
		// Refused: IPv6 unique-local (ULA).
		{"ula v6", "fd00::1", true},
		// Refused: private ranges.
		{"private 10/8", "10.0.0.1", true},
		{"private 172.16/12", "172.16.5.4", true},
		{"private 192.168/16", "192.168.1.1:53", true},
		// Allowed: public unicast and names.
		{"public v4", "8.8.8.8", false},
		{"public v4 with port", "1.1.1.1:53", false},
		{"public v6", "2001:4860:4860::8888", false},
		{"hostname", "dns.example.com", false},
		{"hostname with port", "dns.example.com:53", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUpstreamAddress(tc.addr)
			if tc.wantErr && err == nil {
				t.Fatalf("blocked address %q was accepted", tc.addr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("allowed address %q was refused: %v", tc.addr, err)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), "blocked address range") {
				t.Fatalf("refusal reason is unclear: %v", err)
			}
		})
	}
}

func TestNewDNSClientAppliesDefaultTimeout(t *testing.T) {
	if got := NewDNSClient(0).timeout; got != 10*time.Second {
		t.Fatalf("default timeout = %v, want 10s", got)
	}
	if got := NewDNSClient(-time.Second).timeout; got != 10*time.Second {
		t.Fatalf("negative timeout = %v, want 10s", got)
	}
	if got := NewDNSClient(2 * time.Second).timeout; got != 2*time.Second {
		t.Fatalf("explicit timeout = %v, want 2s", got)
	}
}

// rrToEntry must extract the RDATA rather than leaking the full presentation
// format (which repeats owner/TTL/class/type) into the UI.
func TestRRToEntryExtractsRData(t *testing.T) {
	h := func(rtype uint16, ttl uint32) dns.RR_Header {
		return dns.RR_Header{Name: "example.test.", Rrtype: rtype, Class: dns.ClassINET, Ttl: ttl}
	}
	for _, tc := range []struct {
		name  string
		rr    dns.RR
		qtype string
		want  string
	}{
		{"A", &dns.A{Hdr: h(dns.TypeA, 300), A: []byte{192, 0, 2, 10}}, "A", "192.0.2.10"},
		{"AAAA", &dns.AAAA{Hdr: h(dns.TypeAAAA, 300), AAAA: []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}}, "AAAA", "2001:db8::1"},
		{"CNAME", &dns.CNAME{Hdr: h(dns.TypeCNAME, 300), Target: "target.test."}, "CNAME", "target.test."},
		{"NS", &dns.NS{Hdr: h(dns.TypeNS, 300), Ns: "ns.test."}, "NS", "ns.test."},
		{"PTR", &dns.PTR{Hdr: h(dns.TypePTR, 300), Ptr: "host.test."}, "PTR", "host.test."},
		{"MX", &dns.MX{Hdr: h(dns.TypeMX, 300), Preference: 10, Mx: "mail.test."}, "MX", "10 mail.test."},
		{"TXT", &dns.TXT{Hdr: h(dns.TypeTXT, 300), Txt: []string{"hello", "world"}}, "TXT", "hello world"},
		{"SRV", &dns.SRV{Hdr: h(dns.TypeSRV, 300), Priority: 1, Weight: 5, Port: 443, Target: "svc.test."}, "SRV", "1 5 443 svc.test."},
		{"CAA", &dns.CAA{Hdr: h(dns.TypeCAA, 300), Flag: 0, Tag: "issue", Value: "ca.example"}, "CAA", `0 issue "ca.example"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := rrToEntry(tc.rr)
			if entry.Name != "example.test." || entry.Type != tc.qtype || entry.TTL != 300 {
				t.Fatalf("header = %+v", entry)
			}
			if entry.Data != tc.want {
				t.Fatalf("data = %q, want %q", entry.Data, tc.want)
			}
			if strings.Contains(entry.Data, "example.test.") && tc.name != "CNAME" && tc.name != "NS" && tc.name != "PTR" {
				t.Fatalf("data contains presentation header: %q", entry.Data)
			}
		})
	}

	t.Run("SOA", func(t *testing.T) {
		rr := &dns.SOA{Hdr: h(dns.TypeSOA, 600), Ns: "ns.test.", Mbox: "hostmaster.test.", Serial: 1, Refresh: 2, Retry: 3, Expire: 4, Minttl: 5}
		entry := rrToEntry(rr)
		if entry.Type != "SOA" || entry.TTL != 600 {
			t.Fatalf("header = %+v", entry)
		}
		if entry.Data != "ns.test. hostmaster.test. 1 2 3 4 5" {
			t.Fatalf("data = %q", entry.Data)
		}
	})

	t.Run("unsupported type falls back to full string", func(t *testing.T) {
		rr := &dns.HINFO{Hdr: h(dns.TypeHINFO, 60), Cpu: "x86", Os: "linux"}
		entry := rrToEntry(rr)
		if entry.Type != "HINFO" || !strings.Contains(entry.Data, "x86") {
			t.Fatalf("fallback = %+v", entry)
		}
	})
}
