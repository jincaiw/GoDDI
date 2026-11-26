package dnstransport

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/enterprise/dnsanswer"
	"github.com/jincaiw/GoDDI/internal/enterprise/dnswire"
	"github.com/miekg/dns"
)

func TestEDNSBudgetAndConcurrentUDP(t *testing.T) {
	_, ua, _ := start(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.Dial("udp4", ua.String())
			if err != nil {
				t.Error(err)
				return
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			q := new(dns.Msg)
			q.SetQuestion("www.example.test.", dns.TypeA)
			q.SetEdns0(4096, false)
			raw, err := q.Pack()
			if err != nil {
				t.Error(err)
				return
			}
			if _, err = c.Write(raw); err != nil {
				t.Error(err)
				return
			}
			buf := make([]byte, 2048)
			n, err := c.Read(buf)
			if err != nil {
				t.Error(err)
				return
			}
			var out dns.Msg
			if err = out.Unpack(buf[:n]); err != nil {
				t.Error(err)
				return
			}
			if n > 1232 || !out.Authoritative || !out.Truncated || out.IsEdns0() == nil {
				t.Errorf("invalid EDNS answer: length=%d response=%v", n, out.String())
			}
		}()
	}
	wg.Wait()
}
func TestLongRRSetUDPToTCPAndTCPFrameLimit(t *testing.T) {
	for _, count := range []int{1000, 4200} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			owner := strings.Repeat("a", 60) + "." + strings.Repeat("b", 30) + ".example.test."
			zone := dnswire.Zone{Name: "example.test.", Serial: 1, Records: []dnswire.Record{
				{Owner: "example.test.", Type: "SOA", RData: "ns.example.test. hostmaster.example.test. 1 1 1 1 1", TTL: 60},
				{Owner: "example.test.", Type: "NS", RData: "ns.example.test.", TTL: 60},
			}}
			for i := 0; i < count; i++ {
				zone.Records = append(zone.Records, dnswire.Record{Owner: owner, Type: "A", RData: fmt.Sprintf("10.0.%d.%d", i/256, i%256), TTL: 60})
			}
			snapshot, err := dnswire.Build(dnswire.Document{Cluster: "11111111-1111-4111-8111-111111111111", Node: "22222222-2222-4222-8222-222222222222", Snapshot: "33333333-3333-4333-8333-333333333333", Version: "1", View: "internal", Zones: []dnswire.Zone{zone}})
			if err != nil {
				t.Fatal(err)
			}
			r, err := dnsanswer.Compile(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			u, l := sockets(t)
			f, err := newFixture(r, u, l, []binding{{source: netip.MustParsePrefix("127.0.0.1/32"), view: "internal"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer f.stop()
			q := new(dns.Msg)
			q.SetQuestion(owner, dns.TypeA)
			raw, err := q.Pack()
			if err != nil {
				t.Fatal(err)
			}
			udp, err := net.Dial("udp4", u.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer udp.Close()
			_ = udp.SetDeadline(time.Now().Add(3 * time.Second))
			if _, err = udp.Write(raw); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 1232)
			n, err := udp.Read(buf)
			if err != nil {
				t.Fatal(err)
			}
			var truncated dns.Msg
			if err = truncated.Unpack(buf[:n]); err != nil || !truncated.Truncated || len(truncated.Answer) != 0 {
				t.Fatalf("UDP: %v %v", &truncated, err)
			}
			tcp, err := net.Dial("tcp4", l.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer tcp.Close()
			_ = tcp.SetDeadline(time.Now().Add(3 * time.Second))
			frame := make([]byte, len(raw)+2)
			binary.BigEndian.PutUint16(frame, uint16(len(raw)))
			copy(frame[2:], raw)
			if _, err = writeFull(tcp, frame); err != nil {
				t.Fatal(err)
			}
			var header [2]byte
			_, err = io.ReadFull(tcp, header[:])
			if count == 4200 {
				if err != io.EOF {
					t.Fatalf("oversized response should close connection: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			response := make([]byte, int(binary.BigEndian.Uint16(header[:])))
			if _, err = io.ReadFull(tcp, response); err != nil {
				t.Fatal(err)
			}
			var parsed dns.Msg
			if err = parsed.Unpack(response); err != nil || parsed.Truncated || len(parsed.Answer) != count {
				t.Fatalf("TCP answers=%d truncated=%v error=%v", len(parsed.Answer), parsed.Truncated, err)
			}
		})
	}
}

func TestEmptyResolverNeverServesRecords(t *testing.T) {
	u, l := sockets(t)
	f, err := newFixture(&dnsanswer.Resolver{}, u, l, []binding{{source: netip.MustParsePrefix("127.0.0.1/32"), view: "internal"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	c, err := net.Dial("udp4", u.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err = c.Write(wire(t)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	var out dns.Msg
	if err = out.Unpack(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if out.Rcode != dns.RcodeServerFailure || out.Authoritative || len(out.Answer) != 0 {
		t.Fatalf("empty resolver: %v", out.String())
	}
}
