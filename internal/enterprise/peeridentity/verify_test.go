package peeridentity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

const clusterID = "11111111-1111-4111-8111-111111111111"
const nodeID = "22222222-2222-4222-8222-222222222222"

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type fixtures struct {
	ca      *x509.Certificate
	caKey   ed25519.PrivateKey
	leaf    *x509.Certificate
	leafKey ed25519.PrivateKey
}

func newFixture(t *testing.T, names []string, uris []string, eku x509.ExtKeyUsage) fixtures {
	t.Helper()
	caPub, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caT := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test ca"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caT, caT, caPub, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	lt := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "ignored"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Minute), DNSNames: names, KeyUsage: x509.KeyUsageDigitalSignature}
	if eku >= 0 {
		lt.ExtKeyUsage = []x509.ExtKeyUsage{eku}
	}
	for _, s := range uris {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		lt.URIs = append(lt.URIs, u)
	}
	der, err := x509.CreateCertificate(rand.Reader, lt, ca, pub, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return fixtures{ca, caKey, leaf, key}
}
func state(f fixtures) tls.ConnectionState {
	return tls.ConnectionState{HandshakeComplete: true, PeerCertificates: []*x509.Certificate{f.leaf}, VerifiedChains: [][]*x509.Certificate{{f.leaf, f.ca}}}
}
func policy(f fixtures) Policy {
	return Policy{ClusterID: clusterID, NodeID: nodeID, DNSName: "node.example.test", PinType: CertificateSHA256, Pin: sha256.Sum256(f.leaf.Raw), RootDER: [][]byte{f.ca.Raw}, EKU: x509.ExtKeyUsageServerAuth}
}

func TestVerifyBinding(t *testing.T) {
	f := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
	s, p := state(f), policy(f)
	id, err := Verify(s, p, now)
	if err != nil || id.ClusterID != clusterID || id.NodeID != nodeID {
		t.Fatalf("%+v %v", id, err)
	}
	p.PinType = PublicKeySHA256
	p.Pin = sha256.Sum256(f.leaf.RawSubjectPublicKeyInfo)
	if _, err := Verify(s, p, now); err != nil {
		t.Fatal(err)
	}
}
func TestRejectInvalid(t *testing.T) {
	f := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
	replacement := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
	tests := map[string]func(*tls.ConnectionState, *Policy, *time.Time){
		"no handshake": func(s *tls.ConnectionState, _ *Policy, _ *time.Time) { s.HandshakeComplete = false },
		"no peer":      func(s *tls.ConnectionState, _ *Policy, _ *time.Time) { s.PeerCertificates = nil },
		"nil peer":     func(s *tls.ConnectionState, _ *Policy, _ *time.Time) { s.PeerCertificates = []*x509.Certificate{nil} },
		"no chains":    func(s *tls.ConnectionState, _ *Policy, _ *time.Time) { s.VerifiedChains = nil },
		"empty chain":  func(s *tls.ConnectionState, _ *Policy, _ *time.Time) { s.VerifiedChains = [][]*x509.Certificate{{}} },
		"fake chain": func(s *tls.ConnectionState, _ *Policy, _ *time.Time) {
			s.VerifiedChains = [][]*x509.Certificate{{replacement.leaf, replacement.ca}}
		},
		"untrusted fake state": func(s *tls.ConnectionState, p *Policy, _ *time.Time) {
			s.PeerCertificates = []*x509.Certificate{replacement.leaf}
			s.VerifiedChains = [][]*x509.Certificate{{replacement.leaf, replacement.ca}}
			p.Pin = sha256.Sum256(replacement.leaf.Raw)
		},
		"replacement leaf": func(s *tls.ConnectionState, _ *Policy, _ *time.Time) {
			s.PeerCertificates = []*x509.Certificate{replacement.leaf}
		},
		"wrong san":     func(_ *tls.ConnectionState, p *Policy, _ *time.Time) { p.DNSName = "other.example.test" },
		"wrong pin":     func(_ *tls.ConnectionState, p *Policy, _ *time.Time) { p.Pin[0] ^= 1 },
		"wrong eku":     func(_ *tls.ConnectionState, p *Policy, _ *time.Time) { p.EKU = x509.ExtKeyUsageClientAuth },
		"no roots":      func(_ *tls.ConnectionState, p *Policy, _ *time.Time) { p.RootDER = nil },
		"bad root":      func(_ *tls.ConnectionState, p *Policy, _ *time.Time) { p.RootDER = [][]byte{f.leaf.Raw} },
		"expired":       func(_ *tls.ConnectionState, _ *Policy, t *time.Time) { *t = now.Add(2 * time.Minute) },
		"not yet valid": func(_ *tls.ConnectionState, _ *Policy, t *time.Time) { *t = now.Add(-2 * time.Minute) },
		"zero time":     func(_ *tls.ConnectionState, _ *Policy, t *time.Time) { *t = time.Time{} },
		"zero node": func(_ *tls.ConnectionState, p *Policy, _ *time.Time) {
			p.NodeID = "00000000-0000-0000-0000-000000000000"
		},
		"uppercase uuid": func(_ *tls.ConnectionState, p *Policy, _ *time.Time) {
			p.NodeID = strings.ToUpper("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
		},
		"wildcard": func(_ *tls.ConnectionState, p *Policy, _ *time.Time) { p.DNSName = "*.example.test" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s, p, tm := state(f), policy(f), now
			mutate(&s, &p, &tm)
			if _, err := Verify(s, p, tm); err == nil {
				t.Fatal("accepted invalid state")
			}
		})
	}
}
func TestEKUAndCertificateBudgets(t *testing.T) {
	for _, eku := range []x509.ExtKeyUsage{-1, x509.ExtKeyUsageAny} {
		f := newFixture(t, []string{"node.example.test"}, nil, eku)
		if _, err := Verify(state(f), policy(f), now); err == nil {
			t.Fatalf("accepted EKU %v", eku)
		}
	}
	f := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
	cases := map[string]func(*tls.ConnectionState, *Policy){
		"peer count": func(s *tls.ConnectionState, _ *Policy) {
			s.PeerCertificates = make([]*x509.Certificate, maxCertificates+1)
		},
		"chain count": func(s *tls.ConnectionState, _ *Policy) { s.VerifiedChains = make([][]*x509.Certificate, maxChains+1) },
		"chain length": func(s *tls.ConnectionState, _ *Policy) {
			s.VerifiedChains[0] = make([]*x509.Certificate, maxCertificates+1)
		},
		"root count": func(_ *tls.ConnectionState, p *Policy) { p.RootDER = make([][]byte, maxCertificates+1) },
		"certificate size": func(s *tls.ConnectionState, _ *Policy) {
			s.PeerCertificates = []*x509.Certificate{{Raw: make([]byte, maxCertificateBytes+1)}}
		},
		"aggregate size": func(s *tls.ConnectionState, p *Policy) {
			large := make([]byte, maxCertificateBytes)
			p.RootDER = make([][]byte, maxCertificates)
			for i := range p.RootDER {
				p.RootDER[i] = large
			}
			s.PeerCertificates = []*x509.Certificate{{Raw: []byte{1}}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, p := state(f), policy(f)
			mutate(&s, &p)
			if _, err := Verify(s, p, now); err == nil {
				t.Fatal("accepted excessive inputs")
			}
		})
	}
}

func TestSANProfiles(t *testing.T) {
	f := newFixture(t, []string{"node.example.test", "other.example.test"}, nil, x509.ExtKeyUsageServerAuth)
	if _, err := Verify(state(f), policy(f), now); err == nil {
		t.Fatal("multiple SANs")
	}
	uri := "spiffe://example.test/cluster/" + clusterID + "/node/" + nodeID
	f = newFixture(t, nil, []string{uri}, x509.ExtKeyUsageServerAuth)
	p := policy(f)
	p.DNSName = ""
	p.SPIFFEURI = uri
	if _, err := Verify(state(f), p, now); err != nil {
		t.Fatal(err)
	}
	p.SPIFFEURI = strings.Replace(uri, nodeID, clusterID, 1)
	if _, err := Verify(state(f), p, now); err == nil {
		t.Fatal("wrong URI node")
	}
}

func TestRealTLSConnection(t *testing.T) {
	f := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
	roots := x509.NewCertPool()
	roots.AddCert(f.ca)
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	server := tls.Server(a, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{f.leaf.Raw}, PrivateKey: f.leafKey}}, MinVersion: tls.VersionTLS12, Time: func() time.Time { return now }})
	client := tls.Client(b, &tls.Config{RootCAs: roots, ServerName: "node.example.test", MinVersion: tls.VersionTLS12, Time: func() time.Time { return now }})
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Handshake() }()
	if err := client.Handshake(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(client.ConnectionState(), policy(f), now); err != nil {
		t.Fatal(err)
	}
}
