// Package dnstransport contains a laboratory-only transport fixture. It does
// not authenticate snapshots or clients, provide authorization, or serve production.
package dnstransport

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/dnsanswer"
	"github.com/miekg/dns"
)

var ErrNoAuthority = errors.New("dnstransport: production serving has no authority")

// Start is intentionally unavailable until a separate authorized production
// transport and activation lifecycle exist. It opens no sockets.
func Start() error { return ErrNoAuthority }

const (
	maxPacket  = 4096
	maxWorkers = 32
	maxFrames  = 8
	deadline   = 500 * time.Millisecond
)

type binding struct {
	source netip.Prefix
	view   string
}

// fixture is package-private and can only wrap already-open laboratory sockets.
// It is not an identity verifier: source addresses and listener placement are
// supplied by a test, and are not evidence of production network authority.
type fixture struct {
	resolver     *dnsanswer.Resolver
	udp          net.PacketConn
	tcp          net.Listener
	bindings     []binding
	mu           sync.Mutex // serializes gate, registrations and every network write
	closed       bool
	conns        map[net.Conn]struct{}
	workers      chan struct{}
	wg           sync.WaitGroup
	beforeAnswer func() // test-only synchronization, configured before start
}

func newFixture(r *dnsanswer.Resolver, udp net.PacketConn, tcp net.Listener, bindings []binding, beforeAnswer func()) (*fixture, error) {
	if r == nil || (udp == nil && tcp == nil) || len(bindings) == 0 {
		return nil, ErrNoAuthority
	}
	copied := append([]binding(nil), bindings...)
	for i, b := range copied {
		if !b.source.IsValid() || b.source != b.source.Masked() || b.view == "" {
			return nil, ErrNoAuthority
		}
		for j := 0; j < i; j++ {
			if b.source.Overlaps(copied[j].source) {
				return nil, ErrNoAuthority
			}
		}
	}
	f := &fixture{resolver: r, udp: udp, tcp: tcp, bindings: copied, workers: make(chan struct{}, maxWorkers), conns: make(map[net.Conn]struct{}), beforeAnswer: beforeAnswer}
	if udp != nil {
		f.wg.Add(1)
		go f.udpLoop()
	}
	if tcp != nil {
		f.wg.Add(1)
		go f.tcpLoop()
	}
	return f, nil
}
func (f *fixture) clientView(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Zone() != "" {
		return ""
	}
	ip = ip.Unmap()
	for _, b := range f.bindings {
		if b.source.Contains(ip) {
			return b.view
		}
	}
	return ""
}
func (f *fixture) register(run func()) bool {
	select {
	case f.workers <- struct{}{}:
	default:
		return false
	}
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		<-f.workers
		return false
	}
	f.wg.Add(1)
	f.mu.Unlock()
	go func() { defer f.wg.Done(); defer func() { <-f.workers }(); run() }()
	return true
}
func (f *fixture) udpLoop() {
	defer f.wg.Done()
	buf := make([]byte, maxPacket+1)
	for {
		_ = f.udp.SetReadDeadline(time.Now().Add(deadline))
		n, addr, err := f.udp.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		if n == 0 || n > maxPacket {
			continue
		}
		view := f.clientView(addr)
		if view == "" {
			continue
		}
		packet := append([]byte(nil), buf[:n]...)
		f.register(func() {
			resp := f.answer(view, packet, true)
			if len(resp) == 0 {
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.closed {
				return
			}
			_ = f.udp.SetWriteDeadline(time.Now().Add(deadline))
			_, _ = f.udp.WriteTo(resp, addr)
		})
	}
}
func (f *fixture) tcpLoop() {
	defer f.wg.Done()
	for {
		c, err := f.tcp.Accept()
		if err != nil {
			return
		}
		view := f.clientView(c.RemoteAddr())
		if view == "" {
			_ = c.Close()
			continue
		}
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			_ = c.Close()
			return
		}
		f.conns[c] = struct{}{}
		f.mu.Unlock()
		if !f.register(func() {
			defer func() { f.mu.Lock(); delete(f.conns, c); f.mu.Unlock(); _ = c.Close() }()
			f.serveConn(c, view)
		}) {
			f.mu.Lock()
			delete(f.conns, c)
			f.mu.Unlock()
			_ = c.Close()
		}
	}
}
func (f *fixture) serveConn(c net.Conn, view string) {
	for n := 0; n < maxFrames; n++ {
		_ = c.SetReadDeadline(time.Now().Add(deadline))
		var h [2]byte
		if _, err := io.ReadFull(c, h[:]); err != nil {
			return
		}
		size := int(binary.BigEndian.Uint16(h[:]))
		if size == 0 || size > maxPacket {
			return
		}
		wire := make([]byte, size)
		if _, err := io.ReadFull(c, wire); err != nil {
			return
		}
		reply := f.answer(view, wire, false)
		if len(reply) == 0 || len(reply) > 65535 {
			return
		}
		frame := make([]byte, 2+len(reply))
		binary.BigEndian.PutUint16(frame[:2], uint16(len(reply)))
		copy(frame[2:], reply)
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return
		}
		_ = c.SetWriteDeadline(time.Now().Add(deadline))
		_, err := writeFull(c, frame)
		f.mu.Unlock()
		if err != nil {
			return
		}
	}
}
func writeFull(w io.Writer, b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		n, e := w.Write(b)
		total += n
		b = b[n:]
		if e != nil {
			return total, e
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}
func (f *fixture) answer(view string, wire []byte, udp bool) []byte {
	var req dns.Msg
	if err := req.Unpack(wire); err != nil {
		return nil
	}
	if f.beforeAnswer != nil {
		f.beforeAnswer()
	}
	out := f.resolver.Answer(view, &req)
	if udp {
		b, err := dnsanswer.PackUDP(&req, out)
		if err != nil || len(b) > 1232 {
			return nil
		}
		return b
	}
	compressed := out.Copy()
	compressed.Compress = true
	b, err := compressed.Pack()
	if err != nil || len(b) > 65535 {
		return nil
	}
	return b
}

// stop returns only after all writes and workers complete. The mutex excludes
// the transition from writes: after it sets closed no writer can begin.
func (f *fixture) stop() {
	f.mu.Lock()
	if !f.closed {
		f.closed = true
		if f.udp != nil {
			_ = f.udp.Close()
		}
		if f.tcp != nil {
			_ = f.tcp.Close()
		}
		for c := range f.conns {
			_ = c.Close()
		}
	}
	f.mu.Unlock()
	f.wg.Wait()
}
