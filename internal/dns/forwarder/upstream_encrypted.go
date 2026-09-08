package forwarder

// Encrypted upstream transports (Technitium parity): DNS-over-TLS (RFC 7858),
// DNS-over-HTTPS (RFC 8484) and DNS-over-QUIC (RFC 9250).
//
// Protocol values on Forwarder:
//   - "dot": Address is host[:port], default port 853.
//   - "doh": Address is a full https:// URL (e.g. https://dns.quad9.net/dns-query).
//   - "doq": Address is host[:port], default port 853 (RFC 9250 ALPN "doq").

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

const (
	maxDNSMessageSize = 64 << 10 // 64 KiB, well above the EDNS0 upper bound.
	doqALPN           = "doq"
)

// httpClient caches one *http.Client per forwarder for DoH.
func (f *Forwarder) httpClient(timeout time.Duration) *http.Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.httpC == nil {
		f.httpC = &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
				// DoH servers are IP-pinned or hostname-resolved; reuse the
				// same dialer defaults as net/http but cap idle conns so a
				// burst of queries does not pin sockets forever.
				MaxIdleConns:        8,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: timeout,
			},
		}
	}
	return f.httpC
}

// dohURL validates and returns the DoH endpoint URL.
func (f *Forwarder) dohURL() (string, error) {
	u, err := url.Parse(strings.TrimSpace(f.Address))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("invalid DoH URL %q: must be an https:// URL", f.Address)
	}
	return u.String(), nil
}

// tlsHost returns the host used for TLS ServerName / QUIC dialing.
func tlsHost(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil && h != "" {
		return h
	}
	if strings.HasPrefix(addr, "[") {
		if end := strings.LastIndex(addr, "]"); end > 1 {
			return addr[1:end]
		}
	}
	return addr
}

// defaultPort appends port when addr has none (IPv6-literal aware).
func defaultPort(addr, port string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}
	if strings.HasPrefix(addr, "[") {
		end := strings.LastIndex(addr, "]")
		if end >= 0 && end == len(addr)-1 {
			return addr + ":" + port
		}
		return addr
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return addr + ":" + port
	}
	return addr
}

// exchangeDoT exchanges a query over DNS-over-TLS (miekg "tcp-tls" transport;
// ServerName is derived from the dial address automatically).
func (f *Forwarder) exchangeDoT(ctx context.Context, msg *dns.Msg, timeout time.Duration) (*dns.Msg, error) {
	client := f.clientFor("tcp-tls", timeout)
	resp, _, err := client.ExchangeContext(ctx, msg, defaultPort(f.Address, "853"))
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// exchangeDoH exchanges a query over DNS-over-HTTPS (POST, RFC 8484 §4.1).
func (f *Forwarder) exchangeDoH(ctx context.Context, msg *dns.Msg, timeout time.Duration) (*dns.Msg, error) {
	endpoint, err := f.dohURL()
	if err != nil {
		return nil, err
	}

	// RFC 8484: the DNS message ID MUST be 0 on the wire. Query the original
	// message unmodified by packing a copy (the same msg may be shared with
	// concurrent exchanges under parallel selection strategies).
	wire := msg.Copy()
	origID := wire.Id
	wire.Id = 0
	payload, err := wire.Pack()
	if err != nil {
		return nil, fmt.Errorf("packing DoH query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	httpResp, err := f.httpClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH endpoint %s returned HTTP %d", endpoint, httpResp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, maxDNSMessageSize))
	if err != nil {
		return nil, fmt.Errorf("reading DoH response: %w", err)
	}

	reply := new(dns.Msg)
	if err := reply.Unpack(body); err != nil {
		return nil, fmt.Errorf("unpacking DoH response: %w", err)
	}
	// Restore correlation for the caller's QID check.
	reply.Id = origID
	return reply, nil
}

// exchangeDoQ exchanges a query over DNS-over-QUIC (RFC 9250), reusing one
// QUIC connection per upstream until it fails.
func (f *Forwarder) exchangeDoQ(ctx context.Context, msg *dns.Msg, timeout time.Duration) (*dns.Msg, error) {
	resp, err := f.doqExchangeOnce(ctx, msg, timeout)
	if err != nil && f.dropDoQSession() {
		// The cached session went stale (server restart, idle timeout):
		// redial once before giving up.
		return f.doqExchangeOnce(ctx, msg, timeout)
	}
	return resp, err
}

func (f *Forwarder) doqExchangeOnce(ctx context.Context, msg *dns.Msg, timeout time.Duration) (*dns.Msg, error) {
	conn, err := f.doqSession(ctx, timeout)
	if err != nil {
		return nil, err
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("doq: opening stream: %w", err)
	}

	// RFC 9250 §4.2.1: message prefixed with its 2-byte big-endian length.
	wire := msg.Copy()
	origID := wire.Id
	wire.Id = 0
	payload, err := wire.Pack()
	if err != nil {
		return nil, fmt.Errorf("packing DoQ query: %w", err)
	}
	prefixed := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(prefixed, uint16(len(payload)))
	copy(prefixed[2:], payload)

	stream.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := stream.Write(prefixed); err != nil {
		stream.CancelRead(0)
		stream.Close()
		return nil, fmt.Errorf("doq: writing query: %w", err)
	}
	// Half-close signals the end of the query; the server then responds.
	_ = stream.Close()

	stream.SetReadDeadline(time.Now().Add(timeout))
	var lenPrefix [2]byte
	if _, err := io.ReadFull(stream, lenPrefix[:]); err != nil {
		return nil, fmt.Errorf("doq: reading response length: %w", err)
	}
	respLen := binary.BigEndian.Uint16(lenPrefix[:])
	if respLen == 0 || int(respLen) > maxDNSMessageSize {
		return nil, fmt.Errorf("doq: invalid response length %d", respLen)
	}
	body := make([]byte, respLen)
	if _, err := io.ReadFull(stream, body); err != nil {
		return nil, fmt.Errorf("doq: reading response: %w", err)
	}
	_ = stream.Close()

	reply := new(dns.Msg)
	if err := reply.Unpack(body); err != nil {
		return nil, fmt.Errorf("doq: unpacking response: %w", err)
	}
	reply.Id = origID
	return reply, nil
}

// doqSession returns the cached QUIC connection, dialing a new one when needed.
func (f *Forwarder) doqSession(ctx context.Context, timeout time.Duration) (*quic.Conn, error) {
	f.mu.Lock()
	cached := f.doqConn
	f.mu.Unlock()
	if cached != nil {
		select {
		case <-cached.Context().Done():
			// Stale: dial below.
		default:
			return cached, nil
		}
	}

	addr := defaultPort(f.Address, "853")
	tlsConf := &tls.Config{
		ServerName: tlsHost(f.Address),
		NextProtos: []string{doqALPN},
		MinVersion: tls.VersionTLS12,
	}
	qConf := &quic.Config{
		HandshakeIdleTimeout: timeout,
		MaxIdleTimeout:       5 * time.Minute,
		KeepAlivePeriod:      30 * time.Second,
	}

	conn, err := quic.DialAddr(ctx, addr, tlsConf, qConf)
	if err != nil {
		return nil, fmt.Errorf("doq: dialing %s: %w", addr, err)
	}

	f.mu.Lock()
	// Keep the freshly dialed connection; a racing dial's loser is closed.
	if f.doqConn != nil && f.doqConn != conn {
		go f.doqConn.CloseWithError(0, "superseded")
	}
	f.doqConn = conn
	f.mu.Unlock()
	return conn, nil
}

// dropDoQSession discards the cached QUIC connection. It returns true when a
// session existed (so the caller can retry once).
func (f *Forwarder) dropDoQSession() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.doqConn == nil {
		return false
	}
	stale := f.doqConn
	f.doqConn = nil
	go stale.CloseWithError(0, "retry")
	return true
}

// exchangeEncrypted routes a query through the encrypted transport selected by
// the forwarder protocol. proto must be "dot", "doh" or "doq".
func (f *Forwarder) exchangeEncrypted(ctx context.Context, msg *dns.Msg, proto string, timeout time.Duration) (*dns.Msg, error) {
	switch proto {
	case "dot":
		return f.exchangeDoT(ctx, msg, timeout)
	case "doh":
		return f.exchangeDoH(ctx, msg, timeout)
	case "doq":
		return f.exchangeDoQ(ctx, msg, timeout)
	default:
		return nil, fmt.Errorf("unsupported encrypted transport: %s", proto)
	}
}

// isEncryptedProtocol reports whether proto selects an encrypted transport.
func isEncryptedProtocol(proto string) bool {
	switch proto {
	case "dot", "doh", "doq":
		return true
	}
	return false
}

// IsEncryptedProtocol is the exported form used by API handlers.
func IsEncryptedProtocol(proto string) bool {
	return isEncryptedProtocol(proto)
}

// validateEncryptedAddress performs protocol-specific address validation for
// encrypted upstreams (SSRF-safe: host must resolve to a permitted IP).
func validateEncryptedAddress(proto, addr string) error {
	switch proto {
	case "doh":
		u, err := url.Parse(strings.TrimSpace(addr))
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("DoH address must be an https:// URL: %s", addr)
		}
		return validateUpstreamAddress(u.Hostname())
	case "dot", "doq":
		return validateUpstreamAddress(addr)
	}
	return fmt.Errorf("unknown encrypted protocol: %s", proto)
}

// ValidateEncryptedAddress is the exported form used by API handlers.
func ValidateEncryptedAddress(proto, addr string) error {
	return validateEncryptedAddress(proto, addr)
}
