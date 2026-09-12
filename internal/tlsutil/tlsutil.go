// Package tlsutil builds the TLS configuration for the management plane.
//
// It exists because two things about a console's certificate are operational
// rather than cryptographic, and both were missing: a renewed certificate
// needed a restart to take effect, and the cipher suites were whatever the
// standard library happened to choose. A certificate that only takes effect at
// the next restart expires during the change window of every deployment whose
// restart is a maintenance event.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Options describes the management plane's TLS.
type Options struct {
	CertFile string
	KeyFile  string
	// MinVersion is tls.VersionTLS12 or tls.VersionTLS13; zero means 1.2.
	MinVersion uint16
	// ClientCAFile, when set, turns on mutual TLS: the console then requires a
	// client certificate signed by this CA. Empty means the client proves
	// itself with a session or a token, as it does over plain HTTP.
	ClientCAFile string
}

// CipherSuites is the explicit list of TLS 1.2 suites this server offers.
//
// It names the ECDHE-AEAD suites the standard library already prefers, so on
// today's Go it changes no behaviour and is not a hardening measure by itself.
// Writing them down is the point: a TLS 1.2 handshake can otherwise be
// negotiated into a suite nobody chose, and an operator auditing the deployment
// should be able to read the list rather than infer it from a version number.
//
// TLS 1.3 suites are deliberately absent. Go does not expose them because the
// specification requires the AEAD suites and forbids the rest; a field here
// would only suggest a choice that does not exist.
var CipherSuites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
}

// ServerConfig builds the TLS configuration for the management plane.
//
// A failure here is a startup failure, not a request failure: the certificate
// pair is read now so that a console which cannot present a certificate says so
// once, at boot, rather than failing every handshake afterwards.
func ServerConfig(opts Options) (*tls.Config, error) {
	if opts.CertFile == "" || opts.KeyFile == "" {
		return nil, fmt.Errorf("tls: cert_file and key_file are both required")
	}

	loader, err := NewLoader(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}

	minVersion := opts.MinVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}

	cfg := &tls.Config{
		MinVersion:   minVersion,
		CipherSuites: CipherSuites,
		// GetCertificate rather than Certificates: the files are re-read when
		// they change, so renewing a certificate is replacing a file.
		GetCertificate: loader.GetCertificate,
	}

	if opts.ClientCAFile != "" {
		pool, err := loadClientCAs(opts.ClientCAFile)
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = pool
		// Verified, not merely requested: a client certificate that nobody
		// checks proves nothing and would read as mTLS on an audit.
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg, nil
}

// loadClientCAs reads the CA bundle a client certificate must chain to.
func loadClientCAs(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tls: reading client_ca_file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("tls: client_ca_file %s contains no usable certificate", path)
	}
	return pool, nil
}

// fileStamp is what "the file changed" means: its modification time and size.
type fileStamp struct {
	modTime time.Time
	size    int64
}

// Loader serves a certificate and key pair, re-reading it when either file
// changes on disk.
//
// It is safe for concurrent use: every connection asks it for a certificate.
type Loader struct {
	certFile string
	keyFile  string

	mu     sync.Mutex
	cert   *tls.Certificate
	stamps [2]fileStamp
}

// NewLoader reads the pair and returns a loader that serves it.
func NewLoader(certFile, keyFile string) (*Loader, error) {
	l := &Loader{certFile: certFile, keyFile: keyFile}
	if _, err := l.reloadLocked(); err != nil {
		return nil, err
	}
	return l, nil
}

// GetCertificate returns the current pair, reloading first if the files moved.
func (l *Loader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return l.Current()
}

// Current returns the certificate to serve.
//
// The files are checked on every call rather than by a background timer. Two
// stat calls per handshake is a cost this surface can carry, and it buys the
// absence of a goroutine whose only job is to notice a file the process cannot
// see changing. What it must not do is break a working handshake because the
// new file is wrong -- a partially copied file, a key that does not match, a
// PEM saved in the wrong encoding. In every one of those the old certificate
// still works, so it stays in service and the failure is logged.
func (l *Loader) Current() (*tls.Certificate, error) {
	want, err := l.stampsOnDisk()
	if err != nil {
		return l.keepServing(err)
	}

	// Fast path: nothing moved. One short lock, no I/O.
	l.mu.Lock()
	if l.cert != nil && l.stamps == want {
		cert := l.cert
		l.mu.Unlock()
		return cert, nil
	}
	l.mu.Unlock()

	// Slow path: reload, with the lock held across the read. Two connections
	// arriving together would otherwise both reload and both believe they were
	// the one that noticed; the lock costs nothing here because it is only
	// contended while a certificate is being replaced, which is once per
	// renewal.
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cert != nil && l.stamps == want {
		return l.cert, nil
	}
	if _, err := l.reloadLocked(); err != nil {
		return l.keepServingLocked(err)
	}
	return l.cert, nil
}

// reloadLocked reads and parses both files, then commits them together.
//
// The caller holds the lock.
func (l *Loader) reloadLocked() (*tls.Certificate, error) {
	// Stamps are taken before the reads, not after. A replacement that lands
	// in between then leaves the recorded stamp older than the file on disk,
	// so the next call sees a difference and tries again; taking them
	// afterwards would record the new file's stamp next to the old file's
	// content and never notice.
	stamps, err := l.stampsOnDisk()
	if err != nil {
		return nil, err
	}

	certPEM, err := os.ReadFile(l.certFile)
	if err != nil {
		return nil, fmt.Errorf("tls: reading %s: %w", l.certFile, err)
	}
	keyPEM, err := os.ReadFile(l.keyFile)
	if err != nil {
		return nil, fmt.Errorf("tls: reading %s: %w", l.keyFile, err)
	}

	// Both are parsed before either is adopted, so a mismatched pair is
	// rejected as a pair rather than half-committed.
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("tls: loading the certificate pair: %w", err)
	}
	if len(cert.Certificate) > 0 {
		cert.Leaf, _ = x509.ParseCertificate(cert.Certificate[0])
	}

	first := l.cert == nil
	l.cert = &cert
	l.stamps = stamps

	if !first {
		var notBefore, notAfter time.Time
		if cert.Leaf != nil {
			notBefore, notAfter = cert.Leaf.NotBefore, cert.Leaf.NotAfter
		}
		slog.Info("TLS certificate reloaded",
			"cert_file", l.certFile, "not_before", notBefore, "not_after", notAfter)
	}
	return l.cert, nil
}

func (l *Loader) stampsOnDisk() ([2]fileStamp, error) {
	certStamp, err := stampOf(l.certFile)
	if err != nil {
		return [2]fileStamp{}, fmt.Errorf("tls: stat %s: %w", l.certFile, err)
	}
	keyStamp, err := stampOf(l.keyFile)
	if err != nil {
		return [2]fileStamp{}, fmt.Errorf("tls: stat %s: %w", l.keyFile, err)
	}
	return [2]fileStamp{certStamp, keyStamp}, nil
}

// keepServing prefers a working handshake over a broken replacement.
func (l *Loader) keepServing(err error) (*tls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.keepServingLocked(err)
}

func (l *Loader) keepServingLocked(err error) (*tls.Certificate, error) {
	if l.cert == nil {
		// Nothing to fall back on: this is the startup path, and the caller
		// reports it as a failure to build the server.
		return nil, err
	}
	slog.Error("TLS certificate could not be reloaded; continuing to serve the previous one",
		"cert_file", l.certFile, "error", err)
	return l.cert, nil
}

func stampOf(path string) (fileStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, err
	}
	return fileStamp{modTime: info.ModTime(), size: info.Size()}, nil
}
