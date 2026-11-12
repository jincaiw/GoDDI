package dnssec

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/miekg/dns"
)

// nsec3Params holds the per-zone NSEC3 configuration read from dns_zones.
// Salt is hex encoded, lowercase, matching SetNSEC3Params/HashName.
type nsec3Params struct {
	Iterations uint16
	Salt       string
	OptOut     bool
}

// nsec3Params reads and validates the zone's NSEC3 parameters. The second
// return value reports whether NSEC3 is in use.
//
// The schema has no separate "mode" column, so NSEC3 is signalled by any
// non-default parameter (iterations > 0, a non-empty salt, or opt-out set).
// A zone with all three at their defaults uses NSEC. This is the same
// convention the API's SetNSEC3Params writes; an explicit mode column would
// be a schema change and is deliberately out of scope here.
func (r *Responder) nsec3Params(zoneName string) (nsec3Params, bool, error) {
	var iterations int
	var salt string
	var optOut bool
	err := r.db.QueryRow(`
		SELECT nsec3_iterations, nsec3_salt, nsec3_optout FROM dns_zones WHERE name = ?`,
		dns.Fqdn(zoneName),
	).Scan(&iterations, &salt, &optOut)
	if err != nil {
		return nsec3Params{}, false, fmt.Errorf("reading NSEC3 parameters: %w", err)
	}
	// Same bounds as SetNSEC3Params. A corrupted row must not produce a
	// response a validator would reject, so fail closed.
	if iterations < 0 || iterations > 100 {
		return nsec3Params{}, false, fmt.Errorf("invalid NSEC3 iterations %d", iterations)
	}
	if salt != "" {
		if _, err := hex.DecodeString(salt); err != nil {
			return nsec3Params{}, false, fmt.Errorf("invalid NSEC3 salt: %w", err)
		}
		if len(salt) > 2*64 {
			return nsec3Params{}, false, fmt.Errorf("NSEC3 salt too long")
		}
	}
	p := nsec3Params{Iterations: uint16(iterations), Salt: salt, OptOut: optOut}
	used := iterations > 0 || salt != "" || optOut
	return p, used, nil
}

type nsec3Entry struct {
	hash   string // uppercase base32hex of the original owner name
	owner  string // canonical original owner name
	rrName string // hash + "." + zoneName
	types  []uint16
}

// nsec3Chain is the zone's hashed-owner-name chain, sorted by hash (RFC 5155
// §3.2). It is built from the same owner-name/type map as the NSEC chain, so
// the two denial mechanisms cannot disagree about which names exist.
type nsec3Chain struct {
	zoneName string
	params   nsec3Params
	entries  []nsec3Entry
}

func (r *Responder) nsec3Chain(
	zoneName string, names []string, types map[string][]uint16, p nsec3Params,
) (*nsec3Chain, error) {
	entries := make([]nsec3Entry, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, owner := range names {
		h := strings.ToUpper(dns.HashName(owner, dns.SHA1, p.Iterations, p.Salt))
		if h == "" {
			return nil, fmt.Errorf("hashing owner %q failed", owner)
		}
		if _, exists := seen[h]; exists {
			return nil, fmt.Errorf("NSEC3 hash collision for %q", owner)
		}
		seen[h] = struct{}{}

		// The NSEC3 type bitmap lists the RR types present at the ORIGINAL
		// owner name, not at the hashed name. Drop the NSEC marker that
		// zoneNameMap adds for the NSEC chain, and add NSEC3PARAM at the apex.
		bitmap := make([]uint16, 0, len(types[owner])+2)
		for _, rt := range types[owner] {
			if rt != dns.TypeNSEC {
				bitmap = append(bitmap, rt)
			}
		}
		if owner == dns.CanonicalName(zoneName) {
			bitmap = append(bitmap, dns.TypeNSEC3PARAM)
		}
		sort.Slice(bitmap, func(i, j int) bool { return bitmap[i] < bitmap[j] })
		entries = append(entries, nsec3Entry{
			hash:   h,
			owner:  owner,
			rrName: h + "." + zoneName,
			types:  bitmap,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].hash < entries[j].hash })
	return &nsec3Chain{zoneName: zoneName, params: p, entries: entries}, nil
}

// hashOf returns the hashed owner name for an arbitrary name under the zone's
// NSEC3 parameters.
func (c *nsec3Chain) hashOf(name string) string {
	return strings.ToUpper(dns.HashName(dns.CanonicalName(name), dns.SHA1, c.params.Iterations, c.params.Salt))
}

// rr builds the NSEC3 RR for an entry, excluding one type from the bitmap
// (used for NODATA, where the queried type must not be advertised).
func (c *nsec3Chain) rr(e nsec3Entry, index int, exclude uint16) (*dns.NSEC3, error) {
	next := c.entries[(index+1)%len(c.entries)].hash
	bitmap := make([]uint16, 0, len(e.types))
	for _, rt := range e.types {
		if rt != exclude {
			bitmap = append(bitmap, rt)
		}
	}
	if len(bitmap) == 0 {
		return nil, fmt.Errorf("empty NSEC3 type bitmap for %q", e.owner)
	}
	flags := uint8(0)
	if c.params.OptOut {
		flags = 1 // opt-out flag, RFC 5155 §3.1.2
	}
	// miekg/dns does not derive the length octets while packing: it writes
	// SaltLength and HashLength verbatim. The scanner (scan_rr.go) sets
	// SaltLength to half the hex string length and HashLength to the fixed
	// SHA-1 digest size of 20. An empty salt is written as "-" so the
	// packer skips the salt octets, exactly as the parser expects.
	salt := c.params.Salt
	saltLength := uint8(0)
	if len(salt) > 0 {
		saltLength = uint8(len(salt) / 2)
	} else {
		salt = "-"
	}
	return &dns.NSEC3{
		Hdr:        dns.RR_Header{Name: e.rrName, Rrtype: dns.TypeNSEC3, Class: dns.ClassINET, Ttl: 300},
		Hash:       dns.SHA1,
		Flags:      flags,
		Iterations: c.params.Iterations,
		SaltLength: saltLength,
		Salt:       salt,
		HashLength: 20, // SHA-1 digest length, RFC 5155 §3.1.5
		NextDomain: next,
		TypeBitMap: bitmap,
	}, nil
}

// exact returns the NSEC3 RR whose hash equals H(name): proof that the name
// itself exists (closest-encloser proof), or a NODATA proof when exclude is
// the queried type.
func (c *nsec3Chain) exact(name string, exclude uint16) (*dns.NSEC3, error) {
	h := c.hashOf(name)
	for i, e := range c.entries {
		if e.hash == h {
			return c.rr(e, i, exclude)
		}
	}
	return nil, fmt.Errorf("no NSEC3 RR matches H(%s)", name)
}

// covering returns the NSEC3 RR whose hash interval contains H(name): proof
// that the name (or a wildcard at that level) does not exist.
func (c *nsec3Chain) covering(name string) (*dns.NSEC3, error) {
	h := c.hashOf(name)
	idx := -1
	for i, e := range c.entries {
		if e.hash < h {
			idx = i
		} else {
			break
		}
	}
	if idx < 0 {
		idx = len(c.entries) - 1 // H(name) sorts before every owner: wrap
	}
	return c.rr(c.entries[idx], idx, 0)
}

// nextCloserName returns the name one label longer than the closest encloser
// on the path to qname (RFC 5155 §8.3). Its hash must be covered by an NSEC3
// RR to prove that the queried name does not exist.
func nextCloserName(qname, closestEncloser string) string {
	q := canonicalLabels(qname)
	ce := canonicalLabels(closestEncloser)
	keep := len(ce) + 1
	if len(q) <= keep {
		return dns.CanonicalName(qname)
	}
	return strings.Join(q[len(q)-keep:], ".") + "."
}

// signNegativeNSEC3 produces RFC 5155 denial-of-existence proof. Every failure
// returns ns unchanged: a validator must never see a partly signed answer.
func (r *Responder) signNegativeNSEC3(
	zoneName, qname string, qtype uint16, nameExists bool, ns []dns.RR, params nsec3Params,
) []dns.RR {
	if _, err := r.activeKey(zoneName, "ZSK"); err != nil {
		return ns
	}
	names, types, err := r.zoneNameMap(zoneName)
	if err != nil || len(names) == 0 {
		return ns
	}
	chain, err := r.nsec3Chain(zoneName, names, types, params)
	if err != nil {
		return ns
	}

	var proofs []*dns.NSEC3
	if nameExists {
		// NODATA: the NSEC3 at H(qname) must omit qtype from its bitmap.
		rr, err := chain.exact(qname, qtype)
		if err != nil {
			return ns
		}
		proofs = append(proofs, rr)
	} else {
		ce := closestEncloser(qname, names)
		if ce == "" {
			return ns
		}
		// RFC 5155 §8.9 closest-encloser proof: an exact match on the
		// closest encloser, an NSEC3 covering the next-closer name, and an
		// NSEC3 covering the wildcard at the closest encloser.
		ceRR, err := chain.exact(ce, 0)
		if err != nil {
			return ns
		}
		proofs = append(proofs, ceRR)
		ncRR, err := chain.covering(nextCloserName(qname, ce))
		if err != nil {
			return ns
		}
		proofs = appendUniqueNSEC3(proofs, ncRR)
		wRR, err := chain.covering("*." + ce)
		if err != nil {
			return ns
		}
		proofs = appendUniqueNSEC3(proofs, wRR)
	}

	// SignRRsets returns its input unchanged on failure, so an unchanged
	// length is the fail-closed signal for each RRset.
	signedSOA := r.SignRRsets(zoneName, ns)
	if len(signedSOA) <= len(ns) {
		return ns
	}
	out := make([]dns.RR, 0, len(signedSOA)+2*len(proofs))
	out = append(out, signedSOA...)
	for _, proof := range proofs {
		signed := r.SignRRsets(zoneName, []dns.RR{proof})
		if len(signed) < 2 {
			return ns
		}
		out = append(out, signed...)
	}
	return out
}

func appendUniqueNSEC3(proofs []*dns.NSEC3, rr *dns.NSEC3) []*dns.NSEC3 {
	for _, p := range proofs {
		if p.Hdr.Name == rr.Hdr.Name {
			return proofs
		}
	}
	return append(proofs, rr)
}
