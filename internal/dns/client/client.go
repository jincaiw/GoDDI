package client

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// QueryResult holds the result of a DNS query for debugging.
type QueryResult struct {
	QueryName    string        `json:"query_name"`
	QueryType    string        `json:"query_type"`
	Upstream     string        `json:"upstream"`
	ResponseCode string        `json:"response_code"`
	Answers      []AnswerEntry `json:"answers"`
	Additional   []AnswerEntry `json:"additional"`
	Authority    []AnswerEntry `json:"authority"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
}

// AnswerEntry represents a single DNS answer record.
type AnswerEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	TTL  uint32 `json:"ttl"`
	Data string `json:"data"`
}

// DNSClient is a DNS query debug tool.
type DNSClient struct {
	timeout time.Duration
}

// NewDNSClient creates a new DNS client for debugging.
func NewDNSClient(timeout time.Duration) *DNSClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &DNSClient{timeout: timeout}
}

// Query executes a DNS query and returns detailed results for debugging.
func (c *DNSClient) Query(name string, qtype uint16, upstream string) (*QueryResult, time.Duration, error) {
	if upstream == "" {
		upstream = "8.8.8.8:53"
	}

	if err := validateUpstreamAddress(upstream); err != nil {
		return &QueryResult{
			QueryName: name,
			QueryType: dns.TypeToString[qtype],
			Upstream:  upstream,
			Error:     err.Error(),
		}, 0, err
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	msg.RecursionDesired = true

	client := &dns.Client{
		Net:          "udp",
		ReadTimeout:  c.timeout,
		WriteTimeout: c.timeout,
	}

	start := time.Now()
	resp, _, err := client.Exchange(msg, upstream)
	duration := time.Since(start)

	if err != nil {
		return &QueryResult{
			QueryName: name,
			QueryType: dns.TypeToString[qtype],
			Upstream:  upstream,
			Duration:  duration,
			Error:     err.Error(),
		}, duration, err
	}

	if resp == nil {
		return &QueryResult{
			QueryName: name,
			QueryType: dns.TypeToString[qtype],
			Upstream:  upstream,
			Duration:  duration,
			Error:     "nil response",
		}, duration, fmt.Errorf("nil response from upstream")
	}

	// TCP fallback if response was truncated.
	if resp.Truncated {
		tcpClient := &dns.Client{
			Net:          "tcp",
			ReadTimeout:  c.timeout,
			WriteTimeout: c.timeout,
		}
		start = time.Now()
		tcpResp, _, tcpErr := tcpClient.Exchange(msg, upstream)
		duration += time.Since(start)
		if tcpErr != nil {
			return &QueryResult{
				QueryName: name,
				QueryType: dns.TypeToString[qtype],
				Upstream:  upstream,
				Duration:  duration,
				Error:     tcpErr.Error(),
			}, duration, tcpErr
		}
		if tcpResp == nil {
			return &QueryResult{
				QueryName: name,
				QueryType: dns.TypeToString[qtype],
				Upstream:  upstream,
				Duration:  duration,
				Error:     "nil TCP response",
			}, duration, fmt.Errorf("nil TCP response from upstream")
		}
		resp = tcpResp
	}

	result := &QueryResult{
		QueryName:    name,
		QueryType:    dns.TypeToString[qtype],
		Upstream:     upstream,
		ResponseCode: dns.RcodeToString[resp.Rcode],
		Duration:     duration,
	}

	// Parse answer section.
	for _, rr := range resp.Answer {
		result.Answers = append(result.Answers, rrToEntry(rr))
	}

	// Parse additional section.
	for _, rr := range resp.Extra {
		result.Additional = append(result.Additional, rrToEntry(rr))
	}

	// Parse authority section.
	for _, rr := range resp.Ns {
		result.Authority = append(result.Authority, rrToEntry(rr))
	}

	return result, duration, nil
}

// validateUpstreamAddress rejects loopback, link-local, and private IPs to
// mitigate SSRF. It accepts either a host:port form or a bare IP literal.
func validateUpstreamAddress(addr string) error {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	} else {
		// Strip a single trailing dot used in FQDN form (e.g. "127.0.0.1.").
		host = strings.TrimSuffix(addr, ".")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
			return fmt.Errorf("blocked address range: %s", addr)
		}
	}
	return nil
}

// rrToEntry converts a dns.RR to an AnswerEntry.
func rrToEntry(rr dns.RR) AnswerEntry {
	hdr := rr.Header()
	entry := AnswerEntry{
		Name: hdr.Name,
		Type: dns.TypeToString[hdr.Rrtype],
		TTL:  hdr.Ttl,
		Data: rr.String(),
	}

	// Extract the data portion after the header.
	// The String() format is: "name TTL class type data"
	// We want just the data part.
	switch v := rr.(type) {
	case *dns.A:
		entry.Data = v.A.String()
	case *dns.AAAA:
		entry.Data = v.AAAA.String()
	case *dns.CNAME:
		entry.Data = v.Target
	case *dns.MX:
		entry.Data = fmt.Sprintf("%d %s", v.Preference, v.Mx)
	case *dns.NS:
		entry.Data = v.Ns
	case *dns.PTR:
		entry.Data = v.Ptr
	case *dns.SOA:
		entry.Data = fmt.Sprintf("%s %s %d %d %d %d %d",
			v.Ns, v.Mbox, v.Serial, v.Refresh, v.Retry, v.Expire, v.Minttl)
	case *dns.SRV:
		entry.Data = fmt.Sprintf("%d %d %d %s", v.Priority, v.Weight, v.Port, v.Target)
	case *dns.TXT:
		entry.Data = strings.Join(v.Txt, " ")
	case *dns.CAA:
		entry.Data = fmt.Sprintf("%d %s \"%s\"", v.Flag, v.Tag, v.Value)
	default:
		// Fallback to full string representation.
		entry.Data = rr.String()
	}

	return entry
}
