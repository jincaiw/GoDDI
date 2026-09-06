package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// generateDoQTestCert mints a self-signed certificate for the loopback DoQ
// listener used in tests.
func generateDoQTestCert(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "doq-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{doqALPN},
		MinVersion:   tls.VersionTLS12,
	}
}

// stubDoQHandler answers every query with a fixed NOERROR response so the
// round trip exercises the full framing path.
type stubDoQHandler struct{}

func (stubDoQHandler) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   []byte{192, 0, 2, 1},
	})
	_ = w.WriteMsg(resp)
}

// TestDoQRoundTrip starts a real QUIC listener on 127.0.0.1:0, sends one
// RFC 9250 framed query and validates the framed response.
func TestDoQRoundTrip(t *testing.T) {
	tc := generateDoQTestCert(t)
	tc.InsecureSkipVerify = true

	srv := NewDoQServer("127.0.0.1:0", tc, stubDoQHandler{})
	listener, err := quic.ListenAddr("127.0.0.1:0", tc, &quic.Config{
		MaxIdleTimeout:  2 * time.Minute,
		KeepAlivePeriod: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("quic listen: %v", err)
	}
	go func() { _ = srv.serve(listener) }()
	defer func() {
		_ = srv.Shutdown()
	}()

	addr := listener.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := quic.DialAddr(ctx, addr, &tls.Config{
		NextProtos:         []string{doqALPN},
		InsecureSkipVerify: true, //nolint:gosec // self-signed test certificate
	}, &quic.Config{})
	if err != nil {
		t.Fatalf("quic dial: %v", err)
	}
	defer conn.CloseWithError(0, "")

	stream, err := conn.OpenStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("example.test."), dns.TypeA)
	wire, err := req.Pack()
	if err != nil {
		t.Fatalf("pack query: %v", err)
	}
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(wire)))
	if _, err := stream.Write(lenBuf[:]); err != nil {
		t.Fatalf("write length: %v", err)
	}
	if _, err := stream.Write(wire); err != nil {
		t.Fatalf("write query: %v", err)
	}
	// RFC 9250: client half-closes after sending the query.
	_ = stream.Close()

	var respBuf [2]byte
	if _, err := io.ReadFull(stream, respBuf[:]); err != nil {
		t.Fatalf("read response length: %v", err)
	}
	respLen := binary.BigEndian.Uint16(respBuf[:])
	if respLen == 0 {
		t.Fatal("empty response")
	}
	body := make([]byte, respLen)
	if _, err := io.ReadFull(stream, body); err != nil {
		t.Fatalf("read response body: %v", err)
	}

	resp := new(dns.Msg)
	if err := resp.Unpack(body); err != nil {
		t.Fatalf("unpack response: %v", err)
	}
	if resp.Id != req.Id {
		t.Errorf("response ID = %d, want %d", resp.Id, req.Id)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	if a, ok := resp.Answer[0].(*dns.A); !ok || a.A.String() != "192.0.2.1" {
		t.Errorf("unexpected answer record: %v", resp.Answer[0])
	}
}
