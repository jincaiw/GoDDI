package peeridentity

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"testing"
	"time"
)

// Real TLS 1.3 tickets must be read by the client before a subsequent dial.
// A resumed handshake remains an identity candidate, never role authority.
func TestVerifyHandshakeRechecksPolicyOnTLS13Resumption(t *testing.T) {
	for _, kind := range []string{"changed pin", "changed trusted time"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, []string{"node.example.test"}, nil, x509.ExtKeyUsageServerAuth)
			p := policy(f)
			policyNow := now
			roots := x509.NewCertPool()
			roots.AddCert(f.ca)
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			serverCfg := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{f.leaf.Raw}, PrivateKey: f.leafKey}}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Time: func() time.Time { return now }}
			serverDone := make(chan error, 3)
			go func() {
				for i := 0; i < 3; i++ {
					raw, e := listener.Accept()
					if e != nil {
						serverDone <- e
						return
					}
					_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
					s := tls.Server(raw, serverCfg)
					e = s.Handshake()
					if e == nil {
						_, e = s.Write([]byte{42})
					}
					_ = raw.Close()
					serverDone <- e
				}
			}()
			var calls int
			var callbackResumed bool
			var callbackErr error
			cfg := &tls.Config{RootCAs: roots, ServerName: "node.example.test", MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Time: func() time.Time { return now }, ClientSessionCache: tls.NewLRUClientSessionCache(4), VerifyConnection: func(s tls.ConnectionState) error {
				calls++
				callbackResumed = s.DidResume
				_, callbackErr = VerifyHandshake(s, p, policyNow)
				return callbackErr
			}}
			dial := func(expectSuccess bool) (bool, error) {
				raw, e := net.DialTimeout("tcp4", listener.Addr().String(), 5*time.Second)
				if e != nil {
					return false, e
				}
				defer raw.Close()
				_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
				c := tls.Client(raw, cfg)
				e = c.Handshake()
				if e != nil {
					return false, e
				}
				resumed := c.ConnectionState().DidResume
				if expectSuccess {
					var b [1]byte
					if _, e = io.ReadFull(c, b[:]); e != nil {
						return resumed, e
					}
					if b[0] != 42 {
						t.Fatal("bad server byte")
					}
				}
				return resumed, nil
			}
			for i := 1; i <= 2; i++ {
				before := calls
				resumed, e := dial(true)
				if e != nil || calls != before+1 || callbackErr != nil {
					t.Fatalf("handshake %d calls=%d err=%v callbackErr=%v", i, calls, e, callbackErr)
				}
				if resumed != (i == 2) || callbackResumed != (i == 2) {
					t.Fatalf("handshake %d didResume=%v callbackDidResume=%v", i, resumed, callbackResumed)
				}
				select {
				case e = <-serverDone:
					if e != nil {
						t.Fatalf("server %d: %v", i, e)
					}
				case <-time.After(6 * time.Second):
					t.Fatal("server stuck")
				}
			}
			if kind == "changed pin" {
				p.Pin[0] ^= 1
			} else {
				policyNow = now.Add(2 * time.Minute)
			}
			before := calls
			_, err = dial(false)
			if err == nil || calls != before+1 || !callbackResumed || callbackErr == nil {
				t.Fatalf("resumed changed policy accepted: calls=%d resumed=%v cbErr=%v err=%v", calls, callbackResumed, callbackErr, err)
			}
			select {
			case <-serverDone:
			case <-time.After(6 * time.Second):
				t.Fatal("server stuck after rejection")
			}
		})
	}
}
