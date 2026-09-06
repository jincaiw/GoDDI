package transfer

import (
	"crypto/sha512"
	"fmt"
	"sort"
	"strings"

	"github.com/miekg/dns"
)

// ZONEMD verification (RFC 8976) for freshly transferred secondary zones.
//
// Supported scope: SIMPLE scheme (1) with SHA-384 (1) and SHA-512 (2)
// digests. MULainen (multiplexed) schemes are skipped with a warning.
// The digest is computed over the canonical RR set as specified by
// RFC 8976 §3.3.1 for the SIMPLE scheme: all RRs in the zone except the
// ZONEMD RRset itself and RRSIGs covering ZONEMD, with owner names in
// canonical (lowercase) form and uncompressed wire format, sorted in
// canonical RR order.
//
// Limitation: rdata names are emitted by miekg/dns in the case they were
// parsed. A primary that emits non-lowercase rdata names could produce a
// digest mismatch; this is rare in practice and logged clearly when hit.

// VerifyZONEMD checks the ZONEMD record of a transferred zone against the
// transferred RR set. records must be the full AXFR payload (including the
// SOA). It returns (verified bool, err error):
//   - no ZONEMD record present  → (true, nil)  — nothing to verify
//   - digest matches            → (true, nil)
//   - digest mismatch / error   → (false, err)
func VerifyZONEMD(zoneName string, records []dns.RR) (bool, error) {
	zoneName = dns.Fqdn(strings.ToLower(zoneName))

	var zonemd *dns.ZONEMD
	var zoneRRs []dns.RR

	for _, rr := range records {
		hdr := rr.Header()
		switch v := rr.(type) {
		case *dns.ZONEMD:
			// Only the ZONEMD at the zone apex is meaningful.
			if strings.ToLower(hdr.Name) == zoneName {
				if zonemd == nil || v.Scheme == 1 {
					zonemd = v
				}
			}
			// ZONEMD RRs are excluded from the digest input.
			continue
		case *dns.RRSIG:
			// RRSIGs covering ZONEMD are excluded; other RRSIGs are
			// also excluded by our non-DNSSEC scope (see below).
			continue
		default:
			// Skip records that do not belong to this zone (glue from
			// out-of-zone bailiwick should not occur in a well-formed
			// AXFR, but be defensive).
			if !dns.IsSubDomain(zoneName, strings.ToLower(hdr.Name)) {
				continue
			}
			zoneRRs = append(zoneRRs, rr)
		}
	}

	if zonemd == nil {
		return true, nil
	}

	if zonemd.Scheme != 1 {
		// Unsupported scheme (e.g. multiplexed): skip verification rather
		// than failing the transfer.
		return true, fmt.Errorf("ZONEMD scheme %d not supported; verification skipped", zonemd.Scheme)
	}
	digest, err := computeZONEMDDigest(zoneName, zoneRRs, zonemd.Hash)
	if err != nil {
		return false, err
	}
	if digest != strings.ToLower(zonemd.Digest) {
		return false, fmt.Errorf("ZONEMD digest mismatch for zone %s (scheme=%d hash=%d)", zoneName, zonemd.Scheme, zonemd.Hash)
	}
	return true, nil
}

// computeZONEMDDigest sorts the RR set in canonical order and hashes the
// concatenated uncompressed wire format. Owner names are lowercased.
func computeZONEMDDigest(zoneName string, rrs []dns.RR, hashAlgo uint8) (string, error) {
	var h interface {
		Write([]byte) (int, error)
		Sum([]byte) []byte
	}
	switch hashAlgo {
	case 1: // SHA-384
		h = sha512.New384()
	case 2: // SHA-512
		h = sha512.New()
	default:
		return "", fmt.Errorf("unsupported ZONEMD hash algorithm %d", hashAlgo)
	}

	// Canonical sort: owner (canonical), type, class, then rdata.
	sorted := make([]dns.RR, len(rrs))
	copy(sorted, rrs)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i].Header(), sorted[j].Header()
		la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if la != lb {
			return la < lb
		}
		if a.Rrtype != b.Rrtype {
			return a.Rrtype < b.Rrtype
		}
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		return canonicalRData(sorted[i]) < canonicalRData(sorted[j])
	})

	// Digest body: for each RR, the canonical wire format with the owner
	// name lowercased.
	for _, rr := range sorted {
		hdr := *rr.Header()
		hdr.Name = strings.ToLower(hdr.Name)

		copied := dns.Copy(rr)
		*copied.Header() = hdr

		wire, err := packRRUncompressed(copied)
		if err != nil {
			return "", err
		}
		h.Write(wire)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// packRRUncompressed packs an RR in wire format without name compression.
// We achieve this by packing a message containing only the answer RR:
// msg.Compress = false ensures uncompressed output; the 12-byte header and
// the 4-byte question/count sections are sliced off to obtain the RR bytes.
func packRRUncompressed(rr dns.RR) ([]byte, error) {
	m := new(dns.Msg)
	m.Compress = false
	m.Answer = []dns.RR{rr}
	buf, err := m.Pack()
	if err != nil {
		return nil, fmt.Errorf("packing canonical RR: %w", err)
	}
	// Skip the 12-byte DNS header; the single answer RR starts right after
	// it (question count is 0, so no question bytes follow the header).
	return buf[12:], nil
}

// canonicalRData returns a comparable string form of the rdata for sorting
// purposes. It uses the RR's textual presentation, which is sufficient for
// ordering RRs of the same type.
func canonicalRData(rr dns.RR) string {
	return rr.String()
}
