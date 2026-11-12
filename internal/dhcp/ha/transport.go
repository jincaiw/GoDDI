package ha

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
)

// LoadTLS builds the base peer-channel TLS config from files. The same node
// certificate is used for both dialing and accepting; the peer is verified
// against caFile. Returns nil when certFile is empty (legacy plaintext).
func LoadTLS(certFile, keyFile, caFile, serverName string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" && caFile == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("ha: loading peer certificate: %w", err)
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("ha: reading peer CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("ha: peer CA file %q has no certificates", caFile)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// serverTLS returns the accept-side config (requires and verifies a client
// certificate), or nil when TLS is disabled.
func (c Config) serverTLS() *tls.Config {
	if c.TLS == nil {
		return nil
	}
	cfg := c.TLS.Clone()
	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	cfg.ClientCAs = c.TLS.RootCAs
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}
	return cfg
}

// clientTLS returns the dial-side config, filling ServerName from the config
// override or the peer address host.
func (c Config) clientTLS() *tls.Config {
	if c.TLS == nil {
		return nil
	}
	cfg := c.TLS.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = c.TLSServerName
	}
	if cfg.ServerName == "" {
		if host, _, err := net.SplitHostPort(c.PeerAddress); err == nil {
			cfg.ServerName = host
		}
	}
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}
	return cfg
}
