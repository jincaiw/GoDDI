package server

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/miekg/dns"
)

const dohMediaType = "application/dns-message"

// DoHServer serves DNS-over-HTTPS (RFC 8484) on an HTTP(S) listener and
// forwards decoded messages into the regular DNS handler pipeline.
type DoHServer struct {
	addr    string
	tlsCfg  *tls.Config
	handler dns.Handler
	httpSrv *http.Server
}

// NewDoHServer creates a DoH server wrapper around the DNS handler.
func NewDoHServer(addr string, tlsCfg *tls.Config, handler dns.Handler) *DoHServer {
	d := &DoHServer{
		addr:    addr,
		tlsCfg:  tlsCfg,
		handler: handler,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dns-query", d.handleDNSQuery)
	d.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return d
}

// ListenAndServeTLS starts the HTTPS listener.
func (d *DoHServer) ListenAndServeTLS() error {
	return d.httpSrv.ListenAndServeTLS("", "")
}

// ServeTLS serves HTTPS using an already-bound listener. Binding is kept
// outside the serving goroutine so the DNS startup path can report an address
// conflict before it reports the service as ready.
func (d *DoHServer) ServeTLS(listener net.Listener) error {
	return d.httpSrv.ServeTLS(listener, "", "")
}

// Shutdown gracefully stops the HTTP listener.
func (d *DoHServer) Shutdown(ctx context.Context) error {
	return d.httpSrv.Shutdown(ctx)
}

// handleDNSQuery implements the RFC 8484 wire protocol: POST with a
// application/dns-message body, or GET with a base64url ?dns= parameter.
func (d *DoHServer) handleDNSQuery(w http.ResponseWriter, r *http.Request) {
	var wire []byte
	switch r.Method {
	case http.MethodPost:
		if ct := r.Header.Get("Content-Type"); ct != dohMediaType {
			http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if err != nil || len(body) == 0 {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		wire = body
	case http.MethodGet:
		q := r.URL.Query().Get("dns")
		if q == "" {
			http.Error(w, "missing dns parameter", http.StatusBadRequest)
			return
		}
		decoded, err := base64.RawURLEncoding.DecodeString(q)
		if err != nil {
			http.Error(w, "invalid dns parameter", http.StatusBadRequest)
			return
		}
		wire = decoded
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	msg := new(dns.Msg)
	if err := msg.Unpack(wire); err != nil {
		http.Error(w, "malformed DNS message", http.StatusBadRequest)
		return
	}

	// Serve through the normal pipeline; the response is written by the
	// dohResponseWriter below.
	w.Header().Set("Content-Type", dohMediaType)
	d.handler.ServeDNS(&dohResponseWriter{httpW: w, httpR: r}, msg)
}

// dohResponseWriter adapts an http.ResponseWriter to dns.ResponseWriter.
type dohResponseWriter struct {
	httpW http.ResponseWriter
	httpR *http.Request
}

func (w *dohResponseWriter) WriteMsg(m *dns.Msg) error {
	wire, err := m.Pack()
	if err != nil {
		return err
	}
	w.httpW.WriteHeader(http.StatusOK)
	_, err = w.httpW.Write(wire)
	return err
}

func (w *dohResponseWriter) Write(b []byte) (int, error) {
	return w.httpW.Write(b)
}

func (w *dohResponseWriter) Close() error { return nil }

func (w *dohResponseWriter) Hijack() {}

func (w *dohResponseWriter) TsigStatus() error { return nil }

func (w *dohResponseWriter) TsigTimersOnly(bool) {}

func (w *dohResponseWriter) Hijacked() bool { return false }

type dohAddr string

func (a dohAddr) Network() string { return "https" }
func (a dohAddr) String() string  { return string(a) }

func (w *dohResponseWriter) RemoteAddr() net.Addr {
	return dohAddr(hostOnly(w.httpR.RemoteAddr))
}

func (w *dohResponseWriter) LocalAddr() net.Addr {
	return dohAddr(hostOnly(w.httpR.Host))
}

// hostOnly strips the port from a host:port pair, tolerating bad input.
func hostOnly(s string) string {
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return s
}
