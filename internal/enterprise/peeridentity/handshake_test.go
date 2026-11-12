package peeridentity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"testing"
	"time"
)

func sameRootLeaf(t *testing.T, f fixtures, name string, eku x509.ExtKeyUsage) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(99), DNSNames: []string{name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Minute), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{eku}}
	raw, err := x509.CreateCertificate(rand.Reader, tmpl, f.ca, pub, f.caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return leaf, key
}
func TestVerifyHandshakeRealLoopback(t *testing.T) {
	for _, kind := range []string{"certificate pin", "public key pin", "same CA SAN other certificate pin", "same CA SAN other public key pin", "wrong policy SAN", "wrong policy EKU", "wrong trusted time", "wrong policy root", "wrong TLS root", "wrong TLS EKU", "TLS1.2", "insecure skips default chain"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
			p := policy(f)
			presented, key := f.leaf, f.leafKey
			roots := x509.NewCertPool()
			roots.AddCert(f.ca)
			policyNow := now
			version := uint16(tls.VersionTLS13)
			insecure := false
			callback := false
			var callbackErr error
			if kind == "public key pin" || kind == "same CA SAN other public key pin" {
				p.PinType = PublicKeySHA256
				p.Pin = sha256.Sum256(f.leaf.RawSubjectPublicKeyInfo)
			}
			switch kind {
			case "same CA SAN other certificate pin", "same CA SAN other public key pin":
				presented, key = sameRootLeaf(t, f, "node.example.test", x509.ExtKeyUsageServerAuth)
			case "wrong policy SAN":
				p.DNSName = "other.example.test"
			case "wrong policy EKU":
				p.EKU = x509.ExtKeyUsageClientAuth
			case "wrong trusted time":
				policyNow = now.Add(2 * time.Minute)
			case "wrong policy root":
				other := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
				p.RootDER = [][]byte{other.ca.Raw}
			case "wrong TLS root":
				other := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
				roots = x509.NewCertPool()
				roots.AddCert(other.ca)
			case "wrong TLS EKU":
				presented, key = sameRootLeaf(t, f, "node.example.test", x509.ExtKeyUsageClientAuth)
			case "TLS1.2":
				version = tls.VersionTLS12
			case "insecure skips default chain":
				insecure = true
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			serverDone := make(chan error, 1)
			go func() {
				raw, e := listener.Accept()
				if e != nil {
					serverDone <- e
					return
				}
				defer raw.Close()
				_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
				server := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{presented.Raw}, PrivateKey: key}}, MinVersion: version, MaxVersion: version, Time: func() time.Time { return now }})
				serverDone <- server.Handshake()
			}()
			raw, err := net.DialTimeout("tcp4", listener.Addr().String(), 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
			cfg := &tls.Config{RootCAs: roots, ServerName: "node.example.test", MinVersion: version, MaxVersion: version, Time: func() time.Time { return now }, InsecureSkipVerify: insecure, VerifyConnection: func(state tls.ConnectionState) error {
				callback = true
				if state.HandshakeComplete {
					t.Error("VerifyConnection unexpectedly reports complete handshake")
				}
				_, callbackErr = VerifyHandshake(state, p, policyNow)
				return callbackErr
			}}
			client := tls.Client(raw, cfg)
			err = client.Handshake()
			_ = raw.Close()
			select {
			case <-serverDone:
			case <-time.After(6 * time.Second):
				t.Fatal("server handshake did not exit")
			}
			valid := kind == "certificate pin" || kind == "public key pin"
			if valid {
				if err != nil || !callback || callbackErr != nil {
					t.Fatalf("valid handshake: callback=%v callbackErr=%v err=%v", callback, callbackErr, err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid handshake accepted")
			}
			if kind == "wrong TLS root" || kind == "wrong TLS EKU" {
				if callback {
					t.Fatal("default TLS verification should reject before callback")
				}
				return
			}
			if !callback || callbackErr == nil {
				t.Fatalf("expected callback rejection: callback=%v callbackErr=%v err=%v", callback, callbackErr, err)
			}
		})
	}
}
func TestVerifyHandshakeRequiresChainAndOriginalVerifyCompletion(t *testing.T) {
	f := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
	s := state(f)
	s.HandshakeComplete = false
	s.Version = tls.VersionTLS13
	if _, err := Verify(s, policy(f), now); err == nil {
		t.Fatal("Verify accepted unfinished handshake")
	}
	if _, err := VerifyHandshake(s, policy(f), now); err != nil {
		t.Fatal(err)
	}
	s.VerifiedChains = nil
	if _, err := VerifyHandshake(s, policy(f), now); err == nil {
		t.Fatal("accepted missing default verification chain")
	}
	s = state(f)
	s.Version = tls.VersionTLS12
	if _, err := VerifyHandshake(s, policy(f), now); err == nil {
		t.Fatal("accepted TLS1.2")
	}
}
