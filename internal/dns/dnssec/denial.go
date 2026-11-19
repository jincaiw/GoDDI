package dnssec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/miekg/dns"
)

// SignNegative adds DNSSEC denial-of-existence proof to an authoritative
// negative response. ns is the authority section (normally the zone SOA).
//
// NXDOMAIN gets a covering NSEC (or an RFC 5155 NSEC3 closest-encloser
// proof when the zone has NSEC3 parameters); NODATA gets the record at the
// queried name whose type bitmap omits qtype. The SOA RRset and each denial
// RRset are signed with the active ZSK. Any failure returns ns unchanged:
// an unsigned negative answer is far better than a half-signed one.
func (r *Responder) SignNegative(zoneName, qname string, qtype uint16, nameExists bool, ns []dns.RR) []dns.RR {
	if r == nil || r.db == nil || len(ns) == 0 {
		return ns
	}
	zoneName = dns.Fqdn(zoneName)

	params, useNSEC3, err := r.nsec3Params(zoneName)
	if err != nil {
		return ns // unreadable or invalid parameters: fail closed
	}
	if useNSEC3 {
		return r.signNegativeNSEC3(zoneName, qname, qtype, nameExists, ns, params)
	}
	if _, err := r.activeKey(zoneName, "ZSK"); err != nil {
		return ns
	}

	names, types, err := r.zoneNameMap(zoneName)
	if err != nil || len(names) == 0 {
		return ns
	}
	nsec, err := r.coveringNSEC(zoneName, qname, qtype, nameExists, names, types)
	if err != nil {
		return ns
	}

	// NXDOMAIN needs two proofs (RFC 4035 §3.1.3.2): one NSEC covering the
	// queried name, and one covering the wildcard at the closest encloser so
	// a validator knows no wildcard could have produced the answer. NODATA
	// needs only the NSEC at the existing name.
	nsecRRs := []*dns.NSEC{nsec}
	if !nameExists {
		wildcard := "*." + closestEncloser(qname, names)
		if wildcard != "*." {
			wildcardNSEC, err := r.coveringNSEC(zoneName, wildcard, 0, false, names, types)
			if err == nil && wildcardNSEC.Hdr.Name != nsec.Hdr.Name {
				nsecRRs = append(nsecRRs, wildcardNSEC)
			}
		}
	}

	// SignRRsets returns the input unchanged on failure, so an unchanged
	// length is the fail-closed signal for every RRset.
	signedSOA := r.SignRRsets(zoneName, ns)
	if len(signedSOA) <= len(ns) {
		return ns
	}
	out := make([]dns.RR, 0, len(signedSOA)+2*len(nsecRRs))
	out = append(out, signedSOA...)
	for _, nsecRR := range nsecRRs {
		signedNSEC := r.SignRRsets(zoneName, []dns.RR{nsecRR})
		if len(signedNSEC) < 2 {
			return ns
		}
		out = append(out, signedNSEC...)
	}
	return out
}

// closestEncloser returns the longest existing ancestor of qname from the
// zone name chain (the "closest encloser" of RFC 4592/RFC 4035). Empty means
// qname is outside the zone, which the caller must not sign.
func closestEncloser(qname string, names []string) string {
	labels := canonicalLabels(qname)
	existing := make(map[string]struct{}, len(names))
	for _, n := range names {
		existing[n] = struct{}{}
	}
	for i := 0; i <= len(labels); i++ {
		if len(labels[i:]) == 0 {
			break
		}
		candidate := strings.Join(labels[i:], ".") + "."
		if _, ok := existing[candidate]; ok {
			return candidate
		}
	}
	return ""
}

// zoneNameMap returns every owner name in the zone in canonical DNS order,
// plus the set of RR types present at each name. The apex is always included
// (it carries the SOA and the DNSKEY RRset even with no dns_records row).
func (r *Responder) zoneNameMap(zoneName string) ([]string, map[string][]uint16, error) {
	rows, err := r.db.Query(`
		SELECT r.name, r.type FROM dns_records r
		JOIN dns_zones z ON z.id = r.zone_id
		WHERE z.name = ? AND r.enabled = 1
		  AND (r.expires_at IS NULL OR r.expires_at > datetime('now'))`,
		dns.Fqdn(zoneName))
	if err != nil {
		return nil, nil, fmt.Errorf("reading zone names: %w", err)
	}
	defer rows.Close()

	types := make(map[string][]uint16)
	for rows.Next() {
		var name, typeName string
		if err := rows.Scan(&name, &typeName); err != nil {
			return nil, nil, fmt.Errorf("scanning zone name: %w", err)
		}
		rt, ok := dns.StringToType[strings.ToUpper(typeName)]
		if !ok {
			continue
		}
		name = dns.CanonicalName(name)
		types[name] = append(types[name], rt)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating zone names: %w", err)
	}

	apex := dns.CanonicalName(zoneName)
	types[apex] = append(types[apex], dns.TypeSOA, dns.TypeDNSKEY)
	for name := range types {
		types[name] = append(types[name], dns.TypeNSEC, dns.TypeRRSIG)
	}

	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return canonicalLess(names[i], names[j]) })
	return names, types, nil
}

// coveringNSEC builds the NSEC that proves a negative answer:
//   - NXDOMAIN: the NSEC whose owner immediately precedes qname, with
//     NextDomain being the following name (the chain wraps at the apex).
//   - NODATA: the NSEC at qname itself, with qtype absent from the bitmap.
func (r *Responder) coveringNSEC(
	zoneName, qname string, qtype uint16, nameExists bool, names []string, types map[string][]uint16,
) (*dns.NSEC, error) {
	q := dns.CanonicalName(qname)
	owner := q
	if !nameExists {
		idx := -1
		for i, name := range names {
			if !canonicalLess(name, q) {
				break
			}
			idx = i
		}
		if idx < 0 {
			idx = len(names) - 1 // qname sorts before the apex: wrap to the last NSEC
		}
		owner = names[idx]
	}

	// A wildcard-synthesized name exists but has no exact owner row; its
	// proof belongs to the wildcard that produced the answer. Without this,
	// NODATA for a qtype miss under a wildcard fails closed to an unsigned
	// SOA instead of producing a verifiable denial.
	if nameExists {
		if _, exact := types[owner]; !exact {
			if wild := wildcardOwnerForNODATA(q, names); wild != "" {
				owner = wild
			}
		}
	}

	ownerIdx := -1
	for i, name := range names {
		if name == owner {
			ownerIdx = i
			break
		}
	}
	if ownerIdx < 0 {
		return nil, fmt.Errorf("NSEC owner %q is not in the zone name chain", owner)
	}
	next := names[(ownerIdx+1)%len(names)]

	// Copy before filtering: types[owner] is shared with the name map.
	bitmap := append([]uint16(nil), types[owner]...)
	if nameExists {
		filtered := bitmap[:0]
		for _, t := range bitmap {
			if t != qtype {
				filtered = append(filtered, t)
			}
		}
		bitmap = filtered
	}
	sort.Slice(bitmap, func(i, j int) bool { return bitmap[i] < bitmap[j] })
	deduped := bitmap[:0]
	for i, t := range bitmap {
		if i == 0 || t != bitmap[i-1] {
			deduped = append(deduped, t)
		}
	}
	if len(deduped) == 0 {
		return nil, fmt.Errorf("empty NSEC type bitmap for %q", owner)
	}

	return &dns.NSEC{
		Hdr:        dns.RR_Header{Name: owner, Rrtype: dns.TypeNSEC, Class: dns.ClassINET, Ttl: 300},
		NextDomain: next,
		TypeBitMap: deduped,
	}, nil
}

// canonicalLess implements the canonical DNS name order of RFC 4034 §6.1:
// labels are compared from the most significant (rightmost) to the least,
// bytewise after case folding; a shorter name sorts before its descendants.
func canonicalLess(a, b string) bool {
	la, lb := canonicalLabels(a), canonicalLabels(b)
	ia, ib := len(la)-1, len(lb)-1
	for ia >= 0 && ib >= 0 {
		if la[ia] != lb[ib] {
			return la[ia] < lb[ib]
		}
		ia--
		ib--
	}
	return len(la) < len(lb)
}

// wildcardOwnerForNODATA returns the closest wildcard owner present in the
// zone name chain for a synthesized name, or "" when no wildcard applies.
func wildcardOwnerForNODATA(qname string, names []string) string {
	existing := make(map[string]struct{}, len(names))
	for _, name := range names {
		existing[name] = struct{}{}
	}
	labels := canonicalLabels(qname)
	for i := 0; i < len(labels); i++ {
		candidate := "*." + strings.Join(labels[i+1:], ".") + "."
		if _, ok := existing[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func canonicalLabels(name string) []string {
	n := dns.CanonicalName(name)
	n = strings.TrimSuffix(n, ".")
	if n == "" {
		return nil
	}
	return strings.Split(n, ".")
}
