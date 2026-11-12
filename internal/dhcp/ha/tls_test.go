package ha

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// genTLSFiles writes a CA and a leaf certificate (SAN 127.0.0.1, server+client
// EKU) and returns their file paths.
func genTLSFiles(t *testing.T) (caCert, leafCert, leafKey string) {
	t.Helper()
	dir := t.TempDir()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert = filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		t.Fatal(err)
	}

	leafKeyRaw, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "node-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}
	caParsed, _ := x509.ParseCertificate(caDER)
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caParsed, &leafKeyRaw.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leafCert = filepath.Join(dir, "leaf.pem")
	if err := os.WriteFile(leafCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	leafKey = filepath.Join(dir, "leaf.key")
	keyDER, _ := x509.MarshalECPrivateKey(leafKeyRaw)
	if err := os.WriteFile(leafKey, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return caCert, leafCert, leafKey
}

func dialTLS(t *testing.T, cfg Config, addr string) (net.Conn, error) {
	t.Helper()
	d := net.Dialer{Timeout: 2 * time.Second}
	td := tls.Dialer{NetDialer: &d, Config: cfg.clientTLS()}
	return td.DialContext(t.Context(), "tcp", addr)
}

// TestMTLSPeerChannelRoundTrip proves the mutual-TLS transport: a client with a
// certificate signed by the configured CA can speak the frame protocol, while a
// plaintext client and a client trusting a different CA cannot.
func TestMTLSPeerChannelRoundTrip(t *testing.T) {
	ca, cert, key := genTLSFiles(t)
	base, err := LoadTLS(cert, key, ca, "")
	if err != nil {
		t.Fatalf("LoadTLS: %v", err)
	}
	srvCfg := Config{PeerAddress: "127.0.0.1:0", TLS: base}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsLn := tls.NewListener(ln, srvCfg.serverTLS())
	defer tlsLn.Close()

	go func() {
		conn, err := tlsLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err := readFrame(conn, maxFrameBytes); err != nil {
			return
		}
		_ = writeFrame(conn, Frame{Type: FrameApplied, Seq: 1})
	}()

	cliCfg := Config{PeerAddress: tlsLn.Addr().String(), TLS: base}
	conn, err := dialTLS(t, cliCfg, tlsLn.Addr().String())
	if err != nil {
		t.Fatalf("mTLS dial: %v", err)
	}
	defer conn.Close()
	nonce, sent := newHelloNonce()
	if err := writeFrame(conn, Frame{Type: FrameHello, Protocol: Protocol, Token: "test-peer-token-123456", Nonce: nonce, SentAt: sent, Watermarks: &Watermarks{NodeID: "n1", Role: "primary"}}); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	f, err := readFrame(conn, maxFrameBytes)
	if err != nil || f.Type != FrameApplied {
		t.Fatalf("read reply = %v err=%v, want applied", f.Type, err)
	}

	// Plaintext client against the TLS listener must fail to speak TLS.
	plain, err := net.DialTimeout("tcp", tlsLn.Addr().String(), time.Second)
	if err == nil {
		_, _ = plain.Write([]byte("not tls\n"))
		buf := make([]byte, 1)
		_ = plain.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		if _, rerr := plain.Read(buf); rerr == nil {
			t.Fatal("plaintext client was served by the TLS listener")
		}
		plain.Close()
	}
}

// TestMTLSRejectsWrongCA proves a client whose CA does not verify the server
// certificate cannot connect.
func TestMTLSRejectsWrongCA(t *testing.T) {
	ca, cert, key := genTLSFiles(t)
	base, _ := LoadTLS(cert, key, ca, "")
	srvCfg := Config{PeerAddress: "127.0.0.1:0", TLS: base}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	tlsLn := tls.NewListener(ln, srvCfg.serverTLS())
	defer tlsLn.Close()
	go func() {
		if c, err := tlsLn.Accept(); err == nil {
			c.Close()
		}
	}()

	// A different CA.
	ca2, _, _ := genTLSFiles(t)
	otherBase, _ := LoadTLS(cert, key, ca2, "") // cert not signed by ca2
	cliCfg := Config{PeerAddress: tlsLn.Addr().String(), TLS: otherBase}
	if conn, err := dialTLS(t, cliCfg, tlsLn.Addr().String()); err == nil {
		conn.Close()
		t.Fatal("client with wrong CA connected to mTLS listener")
	}
}
