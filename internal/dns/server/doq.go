package server

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// DoQ application error codes (RFC 9250 §4.4).
const (
	doqNoError       quic.ApplicationErrorCode = 0x0
	doqInternalError quic.ApplicationErrorCode = 0x1
	doqProtocolError quic.ApplicationErrorCode = 0x2
)

// doqALPN is the ALPN protocol token mandated by RFC 9250 §4.1.
const doqALPN = "doq"

// DoQServer implements a DNS-over-QUIC listener (RFC 9250). Every DNS
// query is carried on its own client-initiated bidirectional QUIC stream
// with a 2-byte big-endian length prefix; the server answers on the same
// stream and closes it.
type DoQServer struct {
	addr    string
	tlsConf *tls.Config
	handler dns.Handler

	// mu guards listener, which is assigned by the serve goroutine but
	// read by Shutdown from the caller's goroutine.
	mu       sync.Mutex
	listener *quic.Listener
	// ctx/cancel are created in the constructor so Shutdown never races
	// with the serve loop's use of s.ctx (the accept loop runs on its own
	// goroutine and Shutdown may be called from any goroutine).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDoQServer creates a DoQ server for the given address. The TLS config
// must carry a certificate; the "doq" ALPN is applied here.
func NewDoQServer(addr string, tlsConf *tls.Config, handler dns.Handler) *DoQServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &DoQServer{
		addr:    addr,
		tlsConf: tlsConf,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// ListenAndServe starts accepting QUIC connections until Shutdown is called.
func (s *DoQServer) ListenAndServe() error {
	conn, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		return fmt.Errorf("doq: listen %s: %w", s.addr, err)
	}
	return s.Serve(conn)
}

// Serve accepts QUIC connections using an already-bound UDP socket. Binding
// is deliberately separate from serving so the caller can make readiness
// depend on every enabled listener being bound successfully.
func (s *DoQServer) Serve(conn net.PacketConn) error {
	tc := s.tlsConf.Clone()
	tc.NextProtos = []string{doqALPN}

	// RFC 9250 §4.2.1: a DoQ server should time out idle connections; a
	// 2x keep-alive window keeps long-lived clients healthy without
	// leaking connection state.
	qcfg := &quic.Config{
		MaxIdleTimeout:  2 * time.Minute,
		KeepAlivePeriod: 30 * time.Second,
	}

	listener, err := quic.Listen(conn, tc, qcfg)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("doq: listen %s: %w", s.addr, err)
	}
	return s.serve(listener)
}

// serve runs the accept loop on an existing QUIC listener. Split from
// ListenAndServe so tests can bind 127.0.0.1:0 and discover the port.
func (s *DoQServer) serve(listener *quic.Listener) error {
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()
	defer listener.Close()

	slog.Info("doq: listener started", "addr", listener.Addr())
	for {
		conn, err := listener.Accept(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return nil // shutting down
			}
			return fmt.Errorf("doq: accept connection: %w", err)
		}
		go s.serveConn(conn)
	}
}

// Shutdown stops the listener and all in-flight streams. Safe to call
// concurrently with the serve loop.
func (s *DoQServer) Shutdown() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()
	if listener != nil {
		return listener.Close()
	}
	return nil
}

// serveConn accepts bidirectional streams on a QUIC connection. Each
// stream carries exactly one DNS query per RFC 9250 §4.2.
func (s *DoQServer) serveConn(conn *quic.Conn) {
	for {
		stream, err := conn.AcceptStream(s.ctx)
		if err != nil {
			return // connection closed or context cancelled
		}
		go s.handleStream(conn, stream)
	}
}

// handleStream reads one length-prefixed DNS query, routes it through the
// shared DNS handler, writes the length-prefixed response and closes the
// stream.
func (s *DoQServer) handleStream(conn *quic.Conn, stream *quic.Stream) {
	defer stream.Close()

	req, err := readDoQMessage(stream)
	if err != nil {
		// Truncated / malformed framing is a protocol violation.
		stream.CancelRead(0)
		_ = conn.CloseWithError(doqProtocolError, "malformed query framing")
		return
	}

	rw := &doqResponseWriter{
		stream:     stream,
		remoteAddr: conn.RemoteAddr(),
		localAddr:  conn.LocalAddr(),
	}
	s.handler.ServeDNS(rw, req)

	if !rw.responded {
		// Handler produced no answer; signal an internal error instead of
		// leaving the client to time out.
		_ = conn.CloseWithError(doqInternalError, "no response generated")
	}
}

// readDoQMessage reads a single 2-byte length-prefixed DNS message from the
// stream (RFC 9250 §4.2.1).
func readDoQMessage(r io.Reader) (*dns.Msg, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("reading message length: %w", err)
	}
	length := binary.BigEndian.Uint16(lenBuf[:])
	if length == 0 {
		return nil, fmt.Errorf("empty DNS message")
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading message body: %w", err)
	}

	m := new(dns.Msg)
	if err := m.Unpack(buf); err != nil {
		return nil, fmt.Errorf("unpacking DNS message: %w", err)
	}
	return m, nil
}

// writeDoQMessage writes a 2-byte length-prefixed DNS message.
func writeDoQMessage(w io.Writer, m *dns.Msg) error {
	wire, err := m.Pack()
	if err != nil {
		return fmt.Errorf("packing DNS message: %w", err)
	}
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(wire)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(wire)
	return err
}

// doqResponseWriter adapts a QUIC stream to dns.ResponseWriter so DoQ
// queries share the exact same handler pipeline as UDP/TCP/DoT/DoH.
type doqResponseWriter struct {
	stream     *quic.Stream
	remoteAddr net.Addr
	localAddr  net.Addr
	responded  bool
}

func (w *doqResponseWriter) LocalAddr() net.Addr  { return w.localAddr }
func (w *doqResponseWriter) RemoteAddr() net.Addr { return w.remoteAddr }

// Network identifies the transport in query logs and metrics.
func (w *doqResponseWriter) Network() string { return "doq" }

func (w *doqResponseWriter) WriteMsg(m *dns.Msg) error {
	w.responded = true
	return writeDoQMessage(w.stream, m)
}

func (w *doqResponseWriter) Write(b []byte) (int, error) {
	w.responded = true
	return w.stream.Write(b)
}

func (w *doqResponseWriter) Close() error { return w.stream.Close() }

func (w *doqResponseWriter) TsigStatus() error   { return nil }
func (w *doqResponseWriter) TsigTimersOnly(bool) {}
func (w *doqResponseWriter) Hijack()             {}
