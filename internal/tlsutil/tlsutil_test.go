package tlsutil

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
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// A throwaway CA, so the tests can issue real certificates rather than stub
// the parts of TLS that do the checking.
// ---------------------------------------------------------------------------

type testAuthority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestAuthority(t *testing.T, name string) *testAuthority {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	return &testAuthority{
		cert: cert,
		key:  key,
		pem:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
}

// leaf issues a certificate for the given purposes and returns the PEM pair and
// the parsed leaf, whose serial number identifies it in assertions.
func (ca *testAuthority) leaf(t *testing.T, serial int64, usages ...x509.ExtKeyUsage) (certPEM, keyPEM []byte, leaf *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "goddi-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usages,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}
	leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse leaf certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		leaf
}

// writePair puts a certificate pair on disk and returns its paths.
func writePair(t *testing.T, dir string, certPEM, keyPEM []byte) (certFile, keyFile string) {
	t.Helper()
	certFile = filepath.Join(dir, "server.crt")
	keyFile = filepath.Join(dir, "server.key")
	writeFile(t, certFile, certPEM)
	writeFile(t, keyFile, keyPEM)
	return certFile, keyFile
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// advancePushesModTime makes a rewrite visible to a loader that compares
// modification times. Two writes in the same nanosecond are not impossible on a
// filesystem that stores coarse timestamps, and a reload test that silently
// skips the reload would pass for the wrong reason.
func advancePushesModTime(t *testing.T, paths ...string) {
	t.Helper()
	future := time.Now().Add(2 * time.Second)
	for _, path := range paths {
		if err := os.Chtimes(path, future, future); err != nil {
			t.Fatalf("set times on %s: %v", path, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Building the configuration.
// ---------------------------------------------------------------------------

func TestServerConfigRefusesAPairItCannotRead(t *testing.T) {
	dir := t.TempDir()
	_, err := ServerConfig(Options{
		CertFile: filepath.Join(dir, "missing.crt"),
		KeyFile:  filepath.Join(dir, "missing.key"),
	})
	if err == nil {
		t.Fatal("a missing certificate pair was accepted")
	}

	// And one that is present but not a pair. Failing here means the console
	// says so once at boot instead of failing every handshake afterwards.
	authority := newTestAuthority(t, "ca")
	other := newTestAuthority(t, "other")
	certPEM, _, _ := authority.leaf(t, 10, x509.ExtKeyUsageServerAuth)
	_, otherKeyPEM, _ := other.leaf(t, 11, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, dir, certPEM, otherKeyPEM)

	if _, err := ServerConfig(Options{CertFile: certFile, KeyFile: keyFile}); err == nil {
		t.Fatal("a certificate paired with somebody else's key was accepted")
	}
}

func TestServerConfigNamesItsCipherSuitesAndVersion(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	certPEM, keyPEM, _ := authority.leaf(t, 20, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, t.TempDir(), certPEM, keyPEM)

	// Default: 1.2.
	cfg, err := ServerConfig(Options{CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2", cfg.MinVersion)
	}
	if len(cfg.CipherSuites) != len(CipherSuites) {
		t.Fatalf("CipherSuites = %v, want the explicit list", cfg.CipherSuites)
	}
	for i, suite := range CipherSuites {
		if cfg.CipherSuites[i] != suite {
			t.Errorf("suite %d = %#x, want %#x", i, cfg.CipherSuites[i], suite)
		}
	}
	// CBC and static-RSA key exchange are the two things an unaudited TLS 1.2
	// list drifts towards being allowed to use.
	for _, forbidden := range []uint16{
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	} {
		for _, suite := range cfg.CipherSuites {
			if suite == forbidden {
				t.Errorf("cipher suite %#x is offered", forbidden)
			}
		}
	}
	if cfg.GetCertificate == nil {
		t.Error("no certificate source: the configuration cannot serve a handshake")
	}

	// Explicit 1.3.
	cfg, err = ServerConfig(Options{CertFile: certFile, KeyFile: keyFile, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatalf("ServerConfig (1.3): %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %#x, want TLS 1.3", cfg.MinVersion)
	}
}

func TestClientCAFileTurnsOnVerifiedMutualTLS(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	certPEM, keyPEM, _ := authority.leaf(t, 30, x509.ExtKeyUsageServerAuth)
	dir := t.TempDir()
	certFile, keyFile := writePair(t, dir, certPEM, keyPEM)
	caFile := filepath.Join(dir, "ca.crt")
	writeFile(t, caFile, authority.pem)

	cfg, err := ServerConfig(Options{
		CertFile: certFile, KeyFile: keyFile, ClientCAFile: caFile,
	})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
	if cfg.ClientCAs == nil {
		t.Error("ClientCAs is nil: a required client certificate nobody can verify is not mutual TLS")
	}

	// A file with no certificate in it would otherwise leave the pool empty,
	// which verifies nothing while reporting mutual TLS.
	writeFile(t, caFile, []byte("not a certificate\n"))
	if _, err := ServerConfig(Options{
		CertFile: certFile, KeyFile: keyFile, ClientCAFile: caFile,
	}); err == nil {
		t.Error("a client_ca_file with no usable certificate was accepted")
	}
}

// ---------------------------------------------------------------------------
// Reloading.
// ---------------------------------------------------------------------------

func TestAReplacedPairIsServedWithoutARestart(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	dir := t.TempDir()
	certA, keyA, leafA := authority.leaf(t, 100, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, dir, certA, keyA)

	loader, err := NewLoader(certFile, keyFile)
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	if got := serialOf(t, loader); got != leafA.SerialNumber.Int64() {
		t.Fatalf("initial serial = %d, want %d", got, leafA.SerialNumber.Int64())
	}

	certB, keyB, leafB := authority.leaf(t, 200, x509.ExtKeyUsageServerAuth)
	writeFile(t, certFile, certB)
	writeFile(t, keyFile, keyB)
	advancePushesModTime(t, certFile, keyFile)

	if got := serialOf(t, loader); got != leafB.SerialNumber.Int64() {
		t.Errorf("serial after the replacement = %d, want %d: the new certificate was not picked up",
			got, leafB.SerialNumber.Int64())
	}
}

func TestABrokenReplacementKeepsServingTheWorkingCertificate(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	other := newTestAuthority(t, "other")
	dir := t.TempDir()
	certA, keyA, leafA := authority.leaf(t, 300, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, dir, certA, keyA)

	loader, err := NewLoader(certFile, keyFile)
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}

	cases := []struct {
		name    string
		cert    []byte
		key     []byte
		explain string
	}{
		{"a truncated certificate", certA[:40], keyA, "a copy that has not finished"},
		{"a key from another pair", certA, mustKey(t, other, 301), "a mismatched pair"},
		{"garbage in both files", []byte("nonsense\n"), []byte("nonsense\n"), "a bad encoding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, certFile, tc.cert)
			writeFile(t, keyFile, tc.key)
			advancePushesModTime(t, certFile, keyFile)

			// The handshake must keep working: an operator replacing a
			// certificate is not an event that should take the console down,
			// and the old certificate is still valid.
			if got := serialOf(t, loader); got != leafA.SerialNumber.Int64() {
				t.Errorf("serial = %d, want the working %d (%s)", got, leafA.SerialNumber.Int64(), tc.explain)
			}
		})
	}

	// And a good replacement after all that bad luck is still adopted: the
	// failure must not have poisoned the loader.
	certB, keyB, leafB := authority.leaf(t, 400, x509.ExtKeyUsageServerAuth)
	writeFile(t, certFile, certB)
	writeFile(t, keyFile, keyB)
	advancePushesModTime(t, certFile, keyFile)
	if got := serialOf(t, loader); got != leafB.SerialNumber.Int64() {
		t.Errorf("serial after a valid replacement = %d, want %d", got, leafB.SerialNumber.Int64())
	}
}

// TestTheServedCertificateAlwaysMatchesItsOwnKey is the invariant a hot reload
// has to hold under concurrency: half of a replacement is not a certificate.
// The reads and the swap are interleaved by racing the loader against itself,
// which is what a burst of connections during a renewal looks like.
func TestTheServedCertificateAlwaysMatchesItsOwnKey(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	dir := t.TempDir()
	certA, keyA, _ := authority.leaf(t, 500, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, dir, certA, keyA)

	loader, err := NewLoader(certFile, keyFile)
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int64(0); ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			cert, key, _ := authority.leaf(t, 600+i, x509.ExtKeyUsageServerAuth)
			writeFile(t, certFile, cert)
			writeFile(t, keyFile, key)
			advancePushesModTime(t, certFile, keyFile)
		}
	}()

	for i := 0; i < 200; i++ {
		cert, err := loader.Current()
		if err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("Current: %v", err)
		}
		if cert.Leaf == nil || len(cert.Certificate) == 0 {
			close(stop)
			wg.Wait()
			t.Fatal("a certificate without a leaf was served")
		}
		pub, ok := cert.Leaf.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			close(stop)
			wg.Wait()
			t.Fatalf("leaf public key is %T", cert.Leaf.PublicKey)
		}
		served, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
		if !ok {
			close(stop)
			wg.Wait()
			t.Fatalf("served key is %T", cert.PrivateKey)
		}
		// Equal takes a *ecdsa.PublicKey (it type-asserts), so the value
		// embedded in the private key has to be passed by address. Reading
		// .X/.Y directly would still work, but both fields are deprecated in
		// Go 1.26 for exactly this comparison.
		if !pub.Equal(&served.PublicKey) {
			close(stop)
			wg.Wait()
			t.Fatal("the served certificate does not match the served key")
		}
	}
	close(stop)
	wg.Wait()
}

func mustKey(t *testing.T, ca *testAuthority, serial int64) []byte {
	t.Helper()
	_, keyPEM, _ := ca.leaf(t, serial, x509.ExtKeyUsageServerAuth)
	return keyPEM
}

func serialOf(t *testing.T, loader *Loader) int64 {
	t.Helper()
	cert, err := loader.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if cert.Leaf == nil {
		t.Fatal("served certificate has no parsed leaf")
	}
	return cert.Leaf.SerialNumber.Int64()
}

// ---------------------------------------------------------------------------
// Real handshakes.
//
// The unit assertions above say which certificate the loader believes it is
// serving. These say which one a client actually receives.
// ---------------------------------------------------------------------------

// serveTLS starts a listener whose handshake result is reported per
// connection.
//
// The server side is where a mutual-TLS rejection is observable. Under TLS 1.3
// the client sends its Finished before the server has evaluated its
// certificate, so a client whose certificate is refused can complete the
// handshake call and only learn otherwise on the next read -- asserting on the
// client alone would report a rejection as an acceptance.
func serveTLS(t *testing.T, cfg *tls.Config) (addr string, results <-chan error, stop func()) {
	t.Helper()

	listener, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	outcomes := make(chan error, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			tlsConn, ok := conn.(*tls.Conn)
			if !ok {
				_ = conn.Close()
				continue
			}
			select {
			case outcomes <- tlsConn.Handshake():
			default:
			}
			_ = conn.Close()
		}
	}()
	return listener.Addr().String(), outcomes, func() {
		_ = listener.Close()
		<-done
	}
}

// handshakeOutcome reads the server's verdict for one connection.
func handshakeOutcome(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the server reported no handshake result")
		return nil
	}
}

func TestAHandshakeServesTheReloadedCertificate(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	dir := t.TempDir()
	certA, keyA, leafA := authority.leaf(t, 700, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, dir, certA, keyA)

	cfg, err := ServerConfig(Options{CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	addr, results, stop := serveTLS(t, cfg)
	defer stop()
	_ = results

	if got := handshakeSerial(t, addr, nil); got != leafA.SerialNumber.Int64() {
		t.Fatalf("client saw serial %d, want %d", got, leafA.SerialNumber.Int64())
	}

	certB, keyB, leafB := authority.leaf(t, 800, x509.ExtKeyUsageServerAuth)
	writeFile(t, certFile, certB)
	writeFile(t, keyFile, keyB)
	advancePushesModTime(t, certFile, keyFile)

	if got := handshakeSerial(t, addr, nil); got != leafB.SerialNumber.Int64() {
		t.Errorf("after replacing the files the client saw serial %d, want %d: "+
			"a renewed certificate still needs a restart", got, leafB.SerialNumber.Int64())
	}
}

func TestMutualTLSAcceptsOnlyCertificatesSignedByTheConfiguredCA(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	stranger := newTestAuthority(t, "stranger")
	dir := t.TempDir()
	certPEM, keyPEM, _ := authority.leaf(t, 900, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, dir, certPEM, keyPEM)
	caFile := filepath.Join(dir, "ca.crt")
	writeFile(t, caFile, authority.pem)

	cfg, err := ServerConfig(Options{
		CertFile: certFile, KeyFile: keyFile, ClientCAFile: caFile,
	})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	addr, results, stop := serveTLS(t, cfg)
	defer stop()

	// No certificate at all.
	conn, err := dialTLS(addr, nil)
	if err == nil {
		conn.Close()
	}
	if err := handshakeOutcome(t, results); err == nil {
		t.Error("a client with no certificate completed a mutual-TLS handshake")
	}

	// A certificate from a CA the server was not told about.
	strangerCertPEM, strangerKeyPEM, _ := stranger.leaf(t, 901, x509.ExtKeyUsageClientAuth)
	conn, err = dialTLS(addr, pairFromPEM(t, strangerCertPEM, strangerKeyPEM))
	if err == nil {
		conn.Close()
	}
	if err := handshakeOutcome(t, results); err == nil {
		t.Error("a client certificate from another CA completed a mutual-TLS handshake")
	}

	// And one it was.
	clientCertPEM, clientKeyPEM, _ := authority.leaf(t, 903, x509.ExtKeyUsageClientAuth)
	conn, err = dialTLS(addr, pairFromPEM(t, clientCertPEM, clientKeyPEM))
	if err != nil {
		t.Fatalf("a client certificate from the configured CA was refused by the client: %v", err)
	}
	defer conn.Close()
	if err := handshakeOutcome(t, results); err != nil {
		t.Errorf("a client certificate from the configured CA was refused by the server: %v", err)
	}
}

func TestATLS13OnlyServerNegotiates13(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	certPEM, keyPEM, _ := authority.leaf(t, 1000, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, t.TempDir(), certPEM, keyPEM)

	cfg, err := ServerConfig(Options{
		CertFile: certFile, KeyFile: keyFile, MinVersion: tls.VersionTLS13,
	})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	addr, results, stop := serveTLS(t, cfg)
	defer stop()

	conn, err := dialTLS(addr, nil)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if err := handshakeOutcome(t, results); err != nil {
		t.Fatalf("TLS 1.3 handshake refused by the server: %v", err)
	}
	defer conn.Close()
	if got := conn.ConnectionState().Version; got != tls.VersionTLS13 {
		t.Errorf("negotiated version = %#x, want TLS 1.3", got)
	}

	// A client capped at 1.2 must be refused rather than downgraded.
	if _, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		MaxVersion:         tls.VersionTLS12,
	}); err == nil {
		t.Error("a TLS 1.2 client completed a handshake with a TLS 1.3-only server")
	}
}

// TestATLS12HandshakeUsesOneOfTheNamedSuites closes the loop on the explicit
// list: the suites are not merely configured, they are what gets negotiated.
func TestATLS12HandshakeUsesOneOfTheNamedSuites(t *testing.T) {
	authority := newTestAuthority(t, "ca")
	certPEM, keyPEM, _ := authority.leaf(t, 1100, x509.ExtKeyUsageServerAuth)
	certFile, keyFile := writePair(t, t.TempDir(), certPEM, keyPEM)

	cfg, err := ServerConfig(Options{CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	addr, results, stop := serveTLS(t, cfg)
	defer stop()

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		MaxVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("TLS 1.2 handshake: %v", err)
	}
	defer conn.Close()

	if err := handshakeOutcome(t, results); err != nil {
		t.Fatalf("TLS 1.2 handshake refused by the server: %v", err)
	}
	state := conn.ConnectionState()
	if state.Version != tls.VersionTLS12 {
		t.Fatalf("version = %#x, want TLS 1.2", state.Version)
	}
	for _, suite := range CipherSuites {
		if state.CipherSuite == suite {
			return
		}
	}
	t.Errorf("negotiated cipher suite %#x is not in the configured list", state.CipherSuite)
}

func handshakeSerial(t *testing.T, addr string, clientCert *tls.Certificate) int64 {
	t.Helper()
	conn, err := dialTLS(addr, clientCert)
	if err != nil {
		t.Fatalf("handshake with %s: %v", addr, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		t.Fatal("the server presented no certificate")
	}
	return certs[0].SerialNumber.Int64()
}

func dialTLS(addr string, clientCert *tls.Certificate) (*tls.Conn, error) {
	cfg := &tls.Config{InsecureSkipVerify: true}
	if clientCert != nil {
		cfg.Certificates = []tls.Certificate{*clientCert}
	}
	return tls.Dial("tcp", addr, cfg)
}

func pairFromPEM(t *testing.T, certPEM, keyPEM []byte) *tls.Certificate {
	t.Helper()
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build client pair: %v", err)
	}
	return &pair
}
