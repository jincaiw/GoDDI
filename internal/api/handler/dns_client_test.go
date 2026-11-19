package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dns/client"
	dnsserver "github.com/jasonwa/goddi/internal/dns/server"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
)

// isPrivateIP is the SSRF boundary used when dns.allow_private_upstream is
// off. Pin every address class it must refuse, including ones that the
// built-in predicates alone would miss (unspecified, IPv6 ULA, multicast).
func TestIsPrivateIPCoversAllBlockedClasses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ip    string
		block bool
	}{
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"link-local v4", "169.254.1.1", true},
		{"link-local v6", "fe80::1", true},
		{"link-local multicast v4", "224.0.0.1", true},
		{"link-local multicast v6", "ff02::1", true},
		{"private 10/8", "10.1.2.3", true},
		{"private 172.16/12", "172.20.1.1", true},
		{"private 192.168/16", "192.168.0.1", true},
		{"ipv6 ULA", "fd00::1", true},
		{"ipv6 ULA fc00", "fc00::1", true},
		{"public v4", "8.8.8.8", false},
		{"public v4", "1.1.1.1", false},
		{"public v6", "2001:4860:4860::8888", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("test IP %q did not parse", tc.ip)
			}
			if got := isPrivateIP(ip); got != tc.block {
				t.Fatalf("isPrivateIP(%s) = %v, want %v", tc.ip, got, tc.block)
			}
		})
	}
}

// The literal-IP guard in internal/dns/client is the last active defence when
// allow_private_upstream is on (the default). It must not be weaker than
// isPrivateIP for literal addresses: a deployment that deliberately turns the
// handler guard back on would otherwise still be exposed through the client
// path. Query returns before any network I/O when the guard fires, so this
// exercises both layers without dialing anything.
func TestClientLiteralGuardIsNotWeakerThanHandlerGuard(t *testing.T) {
	for _, addr := range []string{
		"0.0.0.0:53", "[::]:53", "224.0.0.1:53", "[ff02::1]:53",
		"127.0.0.1:53", "10.0.0.1:53", "169.254.1.1:53", "[fd00::1]:53",
	} {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatalf("bad test address %q: %v", addr, err)
		}
		host = strings.Trim(host, "[]")
		if !isPrivateIP(net.ParseIP(host)) {
			t.Fatalf("%s is not blocked by the handler guard", addr)
		}
		_, _, qerr := client.NewDNSClient(time.Millisecond).Query("example.com.", dns.TypeA, addr)
		if qerr == nil || !strings.Contains(qerr.Error(), "blocked address range") {
			t.Fatalf("client guard accepted %q: err=%v", addr, qerr)
		}
	}
}

func TestValidateUpstreamAddressRejectsPrivateLiteralsWhenGuardIsOn(t *testing.T) {
	previous := DNSServices
	DNSServices = &DNSServiceContainer{Config: &config.Config{}}
	// Config.DNS.AllowPrivateUpstream defaults to the zero value (false), so
	// the handler guard is active for this test.
	defer func() { DNSServices = previous }()

	for _, addr := range []string{"127.0.0.1:53", "10.0.0.1:53", "0.0.0.0:53", "[fd00::1]:53"} {
		if err := validateUpstreamAddress(addr); err == nil {
			t.Fatalf("private upstream %q was accepted with the guard on", addr)
		}
	}
	if err := validateUpstreamAddress("8.8.8.8:53"); err != nil {
		t.Fatalf("public upstream refused: %v", err)
	}
}

func TestDNSClientEmptyUpstreamUsesLocalAuthoritativeSnapshot(t *testing.T) {
	db := newRecordOwnershipTestDB(t)
	zoneStore := zone.NewStore(db)
	zoneStore.Load()
	dnsServer := dnsserver.New(&config.Config{}, nil, nil, nil, nil, nil, zoneStore)
	previous := DNSServices
	DNSServices = &DNSServiceContainer{DB: db, ZoneStore: zoneStore, DNSServer: dnsServer, Config: &config.Config{}}
	t.Cleanup(func() {
		zoneStore.ReloadNow()
		DNSServices = previous
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/dns/client", strings.NewReader(`{"name":"dhcp-1.example.com","type":"A"}`))
	rec := httptest.NewRecorder()
	ExecuteDNSQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("local debug query = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data client.QueryResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Source != "local-authoritative" {
		t.Fatalf("source = %q, want local-authoritative", env.Data.Source)
	}
	if len(env.Data.Answers) != 1 || env.Data.Answers[0].Data != "10.0.0.10" {
		t.Fatalf("answers = %+v, want local 10.0.0.10", env.Data.Answers)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "8.8.8.8") {
		t.Fatalf("response still references the old external default: %s", rec.Body.String())
	}
}

func TestDNSClientEmptyUpstreamFailsClosedWithoutLocalServer(t *testing.T) {
	previous := DNSServices
	DNSServices = &DNSServiceContainer{Config: &config.Config{}}
	t.Cleanup(func() { DNSServices = previous })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dns/client", strings.NewReader(`{"name":"www.demo.test","type":"A"}`))
	rec := httptest.NewRecorder()
	ExecuteDNSQuery(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing local server = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "8.8.8.8") {
		t.Fatalf("response still references the old external default: %s", rec.Body.String())
	}
}

func TestMustParseCIDR(t *testing.T) {
	if got := mustParseCIDR("10.0.0.0/8").String(); got != "10.0.0.0/8" {
		t.Fatalf("CIDR = %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("invalid CIDR did not panic")
		}
	}()
	mustParseCIDR("not-a-cidr")
}
