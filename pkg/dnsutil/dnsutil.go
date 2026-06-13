package dnsutil

import (
	"fmt"
	"net"
	"strings"
)

// FQDN ensures a domain name is fully qualified (ends with a dot).
func FQDN(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "."
	}
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// UnFQDN removes the trailing dot from a domain name.
func UnFQDN(name string) string {
	return strings.TrimSuffix(name, ".")
}

// JoinDomain joins domain parts into an FQDN.
func JoinDomain(parts ...string) string {
	return FQDN(strings.Join(parts, "."))
}

// ReverseIP returns the reverse DNS name for an IPv4 address.
// For example, "192.168.1.1" becomes "1.1.168.192.in-addr.arpa."
// IPv6 addresses must be passed to ReverseIPv6; passing one here returns
// the empty string. If `ip` is not a syntactically valid IPv4 address the
// function returns "" instead of producing a garbage zone name.
func ReverseIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	// Reject IPv6 inputs early so the function does not silently
	// reinterpret an IPv6 string as a dotted-decimal list and emit a
	// meaningless ".in-addr.arpa." zone.
	if parsed.To4() == nil {
		return ""
	}
	parts := strings.Split(ip, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return FQDN(strings.Join(parts, ".") + ".in-addr.arpa")
}

// ReverseIPv6 returns the reverse DNS name for an IPv6 address.
func ReverseIPv6(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	// Ensure we have the 16-byte IPv6 form.
	parsed = parsed.To16()
	if parsed == nil {
		return ""
	}
	// Expand to full hex form and reverse nibble by nibble.
	var reversed strings.Builder
	for i := 15; i >= 0; i-- {
		// Low nibble first, then high nibble (per RFC 3596).
		reversed.WriteByte(hexDigit(parsed[i] & 0x0f))
		reversed.WriteByte('.')
		reversed.WriteByte(hexDigit(parsed[i] >> 4))
		reversed.WriteByte('.')
	}
	return FQDN(reversed.String() + "ip6.arpa")
}

// hexDigit converts a nibble (0-15) to its lowercase hex character.
func hexDigit(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'a' + b - 10
}

// RecordType represents a DNS record type.
type RecordType string

const (
	TypeA      RecordType = "A"
	TypeAAAA   RecordType = "AAAA"
	TypeCNAME  RecordType = "CNAME"
	TypeMX     RecordType = "MX"
	TypeNS     RecordType = "NS"
	TypePTR    RecordType = "PTR"
	TypeSOA    RecordType = "SOA"
	TypeSRV    RecordType = "SRV"
	TypeTXT    RecordType = "TXT"
	TypeCAA    RecordType = "CAA"
	TypeTLSA   RecordType = "TLSA"
	TypeDS     RecordType = "DS"
	TypeDNSKEY RecordType = "DNSKEY"
)

// ValidateRecordType checks if a DNS record type is valid.
func ValidateRecordType(t string) bool {
	switch RecordType(strings.ToUpper(t)) {
	case TypeA, TypeAAAA, TypeCNAME, TypeMX, TypeNS,
		TypePTR, TypeSOA, TypeSRV, TypeTXT, TypeCAA,
		TypeTLSA, TypeDS, TypeDNSKEY:
		return true
	}
	return false
}

// FormatRecordValue formats a DNS record value based on its type.
func FormatRecordValue(recordType, value string, priority, weight, port int) string {
	switch RecordType(strings.ToUpper(recordType)) {
	case TypeMX:
		return fmt.Sprintf("%d %s", priority, FQDN(value))
	case TypeSRV:
		return fmt.Sprintf("%d %d %d %s", priority, weight, port, FQDN(value))
	case TypeCNAME, TypeNS, TypePTR:
		return FQDN(value)
	default:
		return value
	}
}
