package dnsanswer

import (
	"errors"
	"fmt"

	"github.com/miekg/dns"
)

const maxUDP = 1232

var ErrUDPMessage = errors.New("dnsanswer: invalid or oversized UDP message")

// inspectEDNS accepts exactly one root-owner OPT in Extra, version zero, with
// no options or flags. Neither OPT nor any DNS field authenticates a view.
func inspectEDNS(req *dns.Msg) (int, bool, int) {
	if req == nil || len(req.Answer) != 0 || len(req.Ns) != 0 || len(req.Extra) > 1 {
		return 512, false, dns.RcodeFormatError
	}
	if len(req.Extra) == 0 {
		return 512, false, dns.RcodeSuccess
	}
	opt, ok := req.Extra[0].(*dns.OPT)
	if !ok || opt == nil || opt.Hdr.Name != "." || opt.Hdr.Rrtype != dns.TypeOPT || opt.ExtendedRcode() != 0 {
		return 512, false, dns.RcodeFormatError
	}
	size := int(opt.UDPSize())
	if size < 512 {
		size = 512
	}
	if size > maxUDP {
		size = maxUDP
	}
	if opt.Version() != 0 {
		return size, true, dns.RcodeBadVers
	}
	if len(opt.Option) != 0 || opt.Hdr.Ttl&0xffff != 0 {
		return size, true, dns.RcodeNotImplemented
	}
	return size, true, dns.RcodeSuccess
}

// PackUDP packs an Answer response within the request's actual UDP budget.
// It does not mutate either message. When the packed answer is too large, all
// answer/authority RRsets are omitted together and TC is set, so no partial
// RRset or negative proof is emitted. A caller must use TCP for the full reply.
// It accepts only single-question responses from this isolated handler, with
// at most one response OPT; it cannot be used to authorize or send packets.
func PackUDP(req, resp *dns.Msg) ([]byte, error) {
	if req == nil || resp == nil || len(req.Question) != 1 || len(resp.Question) != 1 || req.Id != resp.Id || req.Question[0] != resp.Question[0] || !resp.Response {
		return nil, ErrUDPMessage
	}
	limit, has, code := inspectEDNS(req)
	if code == dns.RcodeFormatError && resp.Rcode != dns.RcodeFormatError {
		return nil, ErrUDPMessage
	}
	if len(resp.Extra) > 1 {
		return nil, ErrUDPMessage
	}
	if has {
		if len(resp.Extra) != 1 {
			return nil, ErrUDPMessage
		}
		opt, ok := resp.Extra[0].(*dns.OPT)
		if !ok || opt == nil || opt.Hdr.Name != "." || opt.Hdr.Rrtype != dns.TypeOPT || int(opt.UDPSize()) != limit || len(opt.Option) != 0 || opt.Hdr.Ttl&0xffff != 0 {
			return nil, ErrUDPMessage
		}
	} else if len(resp.Extra) != 0 {
		return nil, ErrUDPMessage
	}
	// A finite cap bounds work on externally constructed responses. The resolver
	// itself produces at most one RRset and one SOA, and dnswire caps record count.
	if len(resp.Answer)+len(resp.Ns) > 8192 {
		return nil, ErrUDPMessage
	}
	out := resp.Copy()
	out.Compress = true
	if out.Len() <= 65535 {
		wire, err := out.Pack()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUDPMessage, err)
		}
		if len(wire) <= limit {
			return wire, nil
		}
	}
	out.Answer = nil
	out.Ns = nil
	out.Truncated = true
	wire, err := out.Pack()
	if err != nil || len(wire) > limit {
		return nil, ErrUDPMessage
	}
	return wire, nil
}
