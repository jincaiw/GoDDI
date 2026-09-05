package server

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	dnsquerylog "github.com/jasonwa/goddi/internal/dns"
	"github.com/jasonwa/goddi/internal/dns/filter"
	"github.com/miekg/dns"
)

// DNSHandler implements the dns.Handler interface.
type DNSHandler struct {
	server *Server
}

// ServeDNS handles incoming DNS requests through the full processing pipeline.
func (h *DNSHandler) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	start := time.Now()

	// Extract client IP and protocol.
	clientIP, clientPort := extractClientAddr(w)
	proto := w.RemoteAddr().Network()

	// Only process if there's at least one question.
	if len(req.Question) == 0 {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeFormatError)
		if err := w.WriteMsg(resp); err != nil {
			slog.Debug("dns_handler: write failed", "error", err)
		}
		return
	}

	q := req.Question[0]
	qname := q.Name
	qtype := q.Qtype

	slog.Debug("dns_handler: received query",
		"client_ip", clientIP,
		"query_name", qname,
		"query_type", dns.TypeToString[qtype],
		"protocol", proto,
	)

	// Step 1: Check local authoritative zones first.
	// Authoritative answers bypass recursion ACL checks.
	if resp, found := h.server.lookupAuthoritative(qname, qtype); found {
		// SetReply resets Rcode to NOERROR; preserve the authoritative
		// negative-answer code (NXDOMAIN) computed by the zone lookup.
		rcode := resp.Rcode
		resp.SetReply(req)
		resp.Rcode = rcode
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, dns.RcodeToString[resp.Rcode], start, "", false, false)
		if err := w.WriteMsg(resp); err != nil {
			slog.Debug("dns_handler: write failed", "error", err)
		}
		return
	}

	// Step 2: Check recursion ACL for non-authoritative queries.
	clientIPNet := net.ParseIP(clientIP)
	if clientIPNet == nil {
		clientIPNet = net.ParseIP("0.0.0.0")
	}

	if !h.server.isRecursionAllowed(clientIPNet) {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "REFUSED", start, "", false, false)
		if err := w.WriteMsg(resp); err != nil {
			slog.Debug("dns_handler: write failed", "error", err)
		}
		return
	}

	// Step 3: Match client policy.
	// (handled within filter.Check below)

	// Step 4: Check allow list (whitelist).
	// Step 5: Check block list (security rules).
	filterResult := h.server.filter.Check(qname, clientIP)
	if filterResult.Blocked {
		resp := filter.GenerateBlockedResponse(req, filterResult.ResponseType, filterResult.ResponseData)
		if resp == nil {
			// DROP response type - don't respond at all.
			h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "DROP", start, "", false, true)
			return
		}
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, filterResult.ResponseType, start, "", false, true)
		if err := w.WriteMsg(resp); err != nil {
			slog.Debug("dns_handler: write failed", "error", err)
		}
		return
	}

	// Step 6: Check cache.
	if h.server.cache != nil {
		cachedMsg, hit, _ := h.server.cache.Get(qname, qtype)
		if hit {
			cachedMsg.SetReply(req)
			h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, dns.RcodeToString[cachedMsg.Rcode], start, "", true, false)
			if err := w.WriteMsg(cachedMsg); err != nil {
				slog.Debug("dns_handler: write failed", "error", err)
			}
			return
		}
	}

	// Step 7: Forward to upstream or recursive resolve.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, fwd, duration, err := h.server.resolveForward(ctx, req)
	if err != nil {
		slog.Error("dns_handler: forward failed",
			"query_name", qname,
			"error", err,
		)
		errResp := new(dns.Msg)
		errResp.SetRcode(req, dns.RcodeServerFailure)
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "SERVFAIL", start, "", false, false)
		if err := w.WriteMsg(errResp); err != nil {
			slog.Debug("dns_handler: write failed", "error", err)
		}
		return
	}

	// Step 8: DNS Rebinding protection.
	if h.server.filter.CheckRebindingProtection(resp, clientIP) {
		blockedResp := filter.GenerateBlockedResponse(req, "NXDOMAIN", "")
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "REBINDING", start, "", false, true)
		if err := w.WriteMsg(blockedResp); err != nil {
			slog.Debug("dns_handler: write failed", "error", err)
		}
		return
	}

	// Step 9: CNAME Cloaking protection.
	if h.server.filter.CheckCNAMECloaking(resp) {
		blockedResp := filter.GenerateBlockedResponse(req, "NXDOMAIN", "")
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "CNAME_CLOAKING", start, "", false, true)
		if err := w.WriteMsg(blockedResp); err != nil {
			slog.Debug("dns_handler: write failed", "error", err)
		}
		return
	}

	// Step 10: Cache the response.
	if h.server.cache != nil {
		h.server.cache.Set(qname, qtype, resp)
	}

	// Step 11: Generate response.
	upstream := ""
	if fwd != nil {
		upstream = fwd.Address
	}

	rcode := dns.RcodeToString[resp.Rcode]
	h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, rcode, start, upstream, false, false)

	// Step 12: Write response.
	// Limit response size for UDP. RFC 6891 §6.2.5: when the response is
	// larger than the requestor's UDP payload size, the response must
	// have the TC bit set. The client will then retry over TCP. We must
	// NOT drop the answer/extra sections: keeping them preserves the
	// correct wire-format truncation behavior the client expects.
	if proto == "udp" && resp.Len() > 512 {
		// Check if EDNS0 UDP size is specified.
		if opt := req.IsEdns0(); opt != nil {
			maxSize := int(opt.UDPSize())
			if maxSize > 512 && resp.Len() > maxSize {
				resp.Truncated = true
			}
		} else {
			resp.Truncated = true
		}
	}

	if err := w.WriteMsg(resp); err != nil {
		slog.Debug("dns_handler: write failed", "error", err)
	}

	_ = duration // Duration already logged via query log.
}

// extractClientAddr extracts the client IP and port from the DNS response writer.
func extractClientAddr(w dns.ResponseWriter) (string, int) {
	addr := w.RemoteAddr()
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String(), 0
	}
	return host, mustAtoi(port)
}

// writeQueryLog asynchronously writes a query log entry.
func (h *DNSHandler) writeQueryLog(
	clientIP string, clientPort int, protocol string,
	qname string, qtype uint16, rcode string,
	start time.Time, upstream string, cached bool, blocked bool,
) {
	if h.server.queryLog == nil {
		return
	}

	responseTime := float64(time.Since(start).Microseconds()) / 1000.0 // ms

	h.server.queryLog.Log(dnsquerylog.QueryLogEntry{
		ClientIP:       clientIP,
		ClientPort:     clientPort,
		Protocol:       strings.ToUpper(protocol),
		QueryName:      qname,
		QueryType:      dns.TypeToString[qtype],
		ResponseCode:   rcode,
		ResponseTimeMs: responseTime,
		Upstream:       upstream,
		Cached:         cached,
		Blocked:        blocked,
	})
}

// mustAtoi converts a string to an int. On any parse error the function
// returns 0 and records a warning. The previous implementation silently
// truncated the value at the first non-digit character, which is not what
// a numeric port is allowed to contain.
func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		slog.Warn("dns_handler: parsing numeric value", "value", s, "error", err)
		return 0
	}
	return n
}
