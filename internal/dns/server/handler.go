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
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/metrics"
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

	// Opcode dispatch: RFC 2136 dynamic updates are handled by their own
	// module and never reach the query pipeline.
	if req.Opcode == dns.OpcodeUpdate {
		if h.server.updateHandler == nil {
			resp := new(dns.Msg)
			resp.SetRcode(req, dns.RcodeNotImplemented)
			_ = w.WriteMsg(resp)
			return
		}
		// Zone-level update ACL.
		if h.server.zoneStore != nil && len(req.Question) > 0 &&
			!h.server.zoneStore.ACLAllows(zone.ACLUpdate, qname, clientIP) {
			slog.Warn("dns_handler: dynamic update denied by zone ACL", "zone", qname, "client", clientIP)
			resp := new(dns.Msg)
			resp.SetRcode(req, dns.RcodeRefused)
			_ = w.WriteMsg(resp)
			return
		}
		resp, err := h.server.updateHandler.HandleUpdateFrom(req, clientIP)
		if err != nil {
			slog.Error("dns_handler: dynamic update failed", "error", err)
			resp = new(dns.Msg)
			resp.SetRcode(req, dns.RcodeServerFailure)
		}
		_ = w.WriteMsg(resp)
		return
	}

	// Inbound NOTIFY (RFC 1996): a primary announces a serial bump; trigger
	// a secondary refresh asynchronously and acknowledge with the local SOA.
	if req.Opcode == dns.OpcodeNotify {
		if h.server.notifyHandler != nil && len(req.Question) > 0 {
			zoneName := qname
			go h.server.notifyHandler(zoneName)
		}
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Authoritative = true
		if soa := h.server.zoneSOAFor(qname); soa != nil {
			resp.Answer = []dns.RR{soa}
		}
		_ = w.WriteMsg(resp)
		return
	}

	// Zone transfer requests (AXFR/IXFR) are only meaningful over TCP.
	if qtype == dns.TypeAXFR || qtype == dns.TypeIXFR {
		h.serveZoneTransfer(w, req, qtype == dns.TypeIXFR, clientIP)
		return
	}

	// Query rate limiting (per client). Authoritative and recursive traffic
	// are both subject to it; disabled when no limiter is attached.
	if h.server.rateLimiter != nil && !h.server.rateLimiter.AllowQuery(normalizeClientIP(clientIP)) {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "RATE_LIMITED", start, "", false, false)
		_ = w.WriteMsg(resp)
		return
	}

	slog.Debug("dns_handler: received query",
		"client_ip", clientIP,
		"query_name", qname,
		"query_type", dns.TypeToString[qtype],
		"protocol", proto,
	)

	// Step 1: Check local authoritative zones first.
	// Authoritative answers bypass recursion ACL checks. A zone-level query
	// ACL may refuse the client.
	resp, denied, found := h.server.lookupAuthoritative(qname, qtype, clientIP)
	if denied {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "REFUSED", start, "", false, false)
		_ = w.WriteMsg(resp)
		return
	}
	if found {
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

	// Step 3b: Special policy zones (Technitium-style Allowed/Blocked zones).
	// Allowed zones bypass the blocking pipeline entirely; blocked zones
	// answer NXDOMAIN before any block list evaluation.
	special := ""
	if h.server.zoneStore != nil {
		special = h.server.zoneStore.MatchSpecial(qname)
	}

	// Step 4: Check allow list (whitelist).
	// Step 5: Check block list (security rules).
	var filterResult filter.CheckResult
	if special == "allowed" {
		filterResult.Blocked = false
	} else if special == "blocked" {
		filterResult.Blocked = true
		filterResult.ResponseType = "NXDOMAIN"
	} else {
		filterResult = h.server.filter.Check(qname, clientIP)
	}
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
	// When ECS data is forwarded upstream (passthrough/add mode) the cached
	// answer would be tied to one client's topology, so the cache is bypassed
	// entirely for correctness.
	if h.server.cache != nil && !h.server.ecsCacheBypass() {
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

	resp, fwd, duration, err := h.server.resolveForward(ctx, h.server.PrepareUpstreamMsg(req, clientIPNet))
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

	// Step 9: CNAME Cloaking protection. Skipped while blocking is
	// temporarily disabled — cloaking detection is part of the blocking
	// pipeline.
	if blockingEnabled, _ := h.server.filter.BlockingStatus(); blockingEnabled {
		if h.server.filter.CheckCNAMECloaking(resp) {
			blockedResp := filter.GenerateBlockedResponse(req, "NXDOMAIN", "")
			h.writeQueryLog(clientIP, clientPort, proto, qname, qtype, "CNAME_CLOAKING", start, "", false, true)
			if err := w.WriteMsg(blockedResp); err != nil {
				slog.Debug("dns_handler: write failed", "error", err)
			}
			return
		}
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
	// Feed the in-memory Top-N statistics (dashboard) regardless of whether
	// persistent query logging is enabled. Cheap enough for the hot path.
	metrics.TopStatsGlobal.RecordWithRcode(clientIP, qname, blocked, rcode)

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

// serveZoneTransfer answers inbound AXFR/IXFR requests. Transfers are only
// served over TCP (RFC 5936 §4.2), require the transfer ACL to match the
// client address, and use TSIG when the request carries a signed query.
func (h *DNSHandler) serveZoneTransfer(w dns.ResponseWriter, req *dns.Msg, ixfr bool, clientIP string) {
	if proto := w.RemoteAddr().Network(); proto != "tcp" {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		_ = w.WriteMsg(resp)
		return
	}

	if h.server.axfrHandler == nil || len(req.Question) == 0 {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		_ = w.WriteMsg(resp)
		return
	}

	zoneName := req.Question[0].Name

	// Zone-level transfer ACL; deny by default when a list is configured.
	if h.server.zoneStore != nil && !h.server.zoneStore.ACLAllows(zone.ACLTransfer, zoneName, clientIP) {
		slog.Warn("dns_handler: zone transfer denied by zone ACL", "zone", zoneName, "client", clientIP)
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		_ = w.WriteMsg(resp)
		return
	}

	// ACL check by client address; deny by default.
	if !h.server.axfrHandler.CheckTransferACL(zoneName, clientIP) {
		slog.Warn("dns_handler: zone transfer denied by ACL", "zone", zoneName, "client", clientIP)
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		_ = w.WriteMsg(resp)
		return
	}

	// TSIG key name from the request, if signed.
	tsigKeyName := ""
	if t := req.IsTsig(); t != nil {
		tsigKeyName = t.Hdr.Name
	}

	var (
		rrs []dns.RR
		err error
	)
	if ixfr {
		var clientSerial uint32
		if soa, ok := req.Ns[0].(*dns.SOA); ok {
			clientSerial = soa.Serial
		}
		rrs, err = h.server.axfrHandler.HandleIXFR(zoneName, clientSerial, tsigKeyName)
	} else {
		rrs, err = h.server.axfrHandler.HandleAXFR(zoneName, tsigKeyName)
	}

	if err != nil {
		slog.Warn("dns_handler: zone transfer failed", "zone", zoneName, "ixfr", ixfr, "error", err)
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeRefused)
		_ = w.WriteMsg(resp)
		return
	}

	// Stream the transfer as multiple DNS messages, ~100 RRs each.
	const chunk = 100
	if len(rrs) == 0 {
		resp := new(dns.Msg)
		resp.SetRcode(req, dns.RcodeServerFailure)
		_ = w.WriteMsg(resp)
		return
	}
	for start := 0; start < len(rrs); start += chunk {
		end := start + chunk
		if end > len(rrs) {
			end = len(rrs)
		}
		msg := new(dns.Msg)
		msg.SetReply(req)
		msg.Authoritative = true
		msg.Answer = rrs[start:end]
		if err := w.WriteMsg(msg); err != nil {
			slog.Debug("dns_handler: transfer write failed", "error", err)
			return
		}
	}
}
