package dnstransport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/dnsanswer"
	"github.com/jasonwa/goddi/internal/enterprise/dnswire"
	"github.com/miekg/dns"
)

func testResolver(t *testing.T) *dnsanswer.Resolver {
	t.Helper()
	z := dnswire.Zone{Name: "example.test.", Serial: 1, Records: []dnswire.Record{{Owner: "example.test.", Type: "SOA", RData: "ns.example.test. hostmaster.example.test. 1 1 1 1 1", TTL: 60}, {Owner: "example.test.", Type: "NS", RData: "ns.example.test.", TTL: 60}}}
	for i := 1; i <= 100; i++ {
		z.Records = append(z.Records, dnswire.Record{Owner: "www.example.test.", Type: "A", RData: fmt.Sprintf("192.0.2.%d", i), TTL: 60})
	}
	s, e := dnswire.Build(dnswire.Document{Cluster: "11111111-1111-4111-8111-111111111111", Node: "22222222-2222-4222-8222-222222222222", Snapshot: "33333333-3333-4333-8333-333333333333", Version: "1", View: "internal", Zones: []dnswire.Zone{z}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := dnsanswer.Compile(s)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func sockets(t *testing.T) (net.PacketConn, net.Listener) {
	t.Helper()
	u, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		_ = u.Close()
		t.Fatal(e)
	}
	return u, l
}
func start(t *testing.T, hook func()) (*fixture, net.Addr, net.Addr) {
	t.Helper()
	u, l := sockets(t)
	f, e := newFixture(testResolver(t), u, l, []binding{{source: netip.MustParsePrefix("127.0.0.1/32"), view: "internal"}}, hook)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(f.stop)
	return f, u.LocalAddr(), l.Addr()
}
func wire(t *testing.T) []byte {
	t.Helper()
	q := new(dns.Msg)
	q.SetQuestion("www.example.test.", dns.TypeA)
	b, e := q.Pack()
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestPublicStartAndValidation(t *testing.T) {
	if !errors.Is(Start(), ErrNoAuthority) {
		t.Fatal("Start opened")
	}
	u, l := sockets(t)
	defer u.Close()
	defer l.Close()
	if _, e := newFixture(nil, u, l, nil, nil); !errors.Is(e, ErrNoAuthority) {
		t.Fatal(e)
	}
	r := testResolver(t)
	if _, e := newFixture(r, u, l, []binding{{netip.MustParsePrefix("127.0.0.0/8"), "a"}, {netip.MustParsePrefix("127.0.0.1/32"), "b"}}, nil); !errors.Is(e, ErrNoAuthority) {
		t.Fatal("overlap accepted")
	}
}
func TestLoopbackUDPAndTCP(t *testing.T) {
	_, ua, ta := start(t, nil)
	q := wire(t)
	u, e := net.Dial("udp4", ua.String())
	if e != nil {
		t.Fatal(e)
	}
	defer u.Close()
	_ = u.SetDeadline(time.Now().Add(time.Second))
	if _, e = u.Write(q); e != nil {
		t.Fatal(e)
	}
	buf := make([]byte, 2048)
	n, e := u.Read(buf)
	if e != nil {
		t.Fatal(e)
	}
	var answer dns.Msg
	if e = answer.Unpack(buf[:n]); e != nil {
		t.Fatal(e)
	}
	if n > 512 || !answer.Truncated || !answer.Authoritative {
		t.Fatalf("UDP len=%d msg=%v", n, answer.String())
	}
	c, e := net.Dial("tcp4", ta.String())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	frame := make([]byte, 2+len(q))
	binary.BigEndian.PutUint16(frame, uint16(len(q)))
	copy(frame[2:], q)
	if _, e = c.Write(frame); e != nil {
		t.Fatal(e)
	}
	var h [2]byte
	if _, e = io.ReadFull(c, h[:]); e != nil {
		t.Fatal(e)
	}
	b := make([]byte, int(binary.BigEndian.Uint16(h[:])))
	if _, e = io.ReadFull(c, b); e != nil {
		t.Fatal(e)
	}
	if e = answer.Unpack(b); e != nil {
		t.Fatal(e)
	}
	if answer.Truncated || len(answer.Answer) != 100 || len(b) <= 512 {
		t.Fatalf("TCP length=%d answers=%d", len(b), len(answer.Answer))
	}
}
func TestDeniedSourceAndOversize(t *testing.T) {
	_, ua, ta := start(t, nil)
	remote := ua.(*net.UDPAddr)
	u, e := net.DialUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.2")}, remote)
	if e != nil {
		t.Skipf("loopback alias unavailable: %v", e)
	}
	defer u.Close()
	_ = u.SetDeadline(time.Now().Add(150 * time.Millisecond))
	_, _ = u.Write(wire(t))
	if _, e = u.Read(make([]byte, 2048)); e == nil {
		t.Fatal("untrusted source received response")
	}
	good, e := net.Dial("udp4", ua.String())
	if e != nil {
		t.Fatal(e)
	}
	defer good.Close()
	_ = good.SetDeadline(time.Now().Add(150 * time.Millisecond))
	_, _ = good.Write(make([]byte, maxPacket+1))
	if _, e = good.Read(make([]byte, 2048)); e == nil {
		t.Fatal("oversized UDP answered")
	}
	c, e := net.Dial("tcp4", ta.String())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	_, _ = c.Write([]byte{0x10, 0x01})
	if _, e = c.Read(make([]byte, 1)); e == nil {
		t.Fatal("oversized TCP frame accepted")
	}
	short, e := net.Dial("tcp4", ta.String())
	if e != nil {
		t.Fatal(e)
	}
	defer short.Close()
	_ = short.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = short.Write([]byte{0, 20, 1})
	if _, e = short.Read(make([]byte, 1)); e == nil {
		t.Fatal("short frame accepted")
	}
}
func TestStopDrainsProcessingAndPreventsReply(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	f, ua, _ := start(t, func() { once.Do(func() { close(entered) }); <-release })
	u, e := net.Dial("udp4", ua.String())
	if e != nil {
		t.Fatal(e)
	}
	defer u.Close()
	_ = u.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = u.Write(wire(t))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no handler")
	}
	done := make(chan struct{})
	go func() { f.stop(); close(done) }()
	select {
	case <-done:
		t.Fatal("stop did not drain")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop stuck")
	}
	if _, e = u.Read(make([]byte, 2048)); e == nil {
		t.Fatal("reply after stop")
	}
}
