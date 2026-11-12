package dnssec

import (
	"crypto"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Signature lifetime for RRSIGs minted on the query path. Inception is
// backdated so a validator whose clock is slightly behind still accepts the
// signature; expiration is long enough that a cached answer does not expire
// before its TTL, but short enough that a key retirement eventually ages out.
const (
	signatureInceptionSkew   = int64(3600)
	signatureValiditySeconds = int64(30 * 24 * 3600)
	dnssecKeyRRTTL           = uint32(3600)
)

// Responder produces DNSKEY and RRSIG records for authoritative answers.
//
// It is deliberately fail-closed. Any missing key, undecryptable private key
// or signing error means "no signatures at all", never a partially signed
// answer: a single RRset without its RRSIG makes a validating resolver mark
// the whole zone Bogus, which is worse than serving the zone unsigned. The
// caller is responsible for gating DNSSEC on the request's EDNS DO bit and on
// the experimental configuration switch.
type Responder struct {
	db        *sql.DB
	jwtSecret string
}

// NewResponder builds a signing responder over a database that holds both
// dns_zones and dns_dnssec_keys.
func NewResponder(db *sql.DB, jwtSecret string) *Responder {
	return &Responder{db: db, jwtSecret: jwtSecret}
}

// SignedZone reports whether the zone is marked DNSSEC-enabled and has both
// an active KSK and an active ZSK. A zone missing either is not signed: it
// must keep answering unsigned rather than publishing half a key set.
func (r *Responder) SignedZone(zoneName string) bool {
	if r == nil || r.db == nil {
		return false
	}
	for _, kind := range []string{"KSK", "ZSK"} {
		_, err := r.activeKey(zoneName, kind)
		if err != nil {
			return false
		}
	}
	return true
}

// DNSKEYRRset returns every enabled DNSKEY for the zone plus an RRSIG over
// that RRset made with the active KSK. It returns an error (and no records)
// when the zone is not signed or has no usable KSK.
func (r *Responder) DNSKEYRRset(zoneName string) ([]dns.RR, error) {
	zoneName = dns.Fqdn(zoneName)
	keys, err := r.enabledKeys(zoneName)
	if err != nil {
		return nil, err
	}
	ksk, err := r.activeKey(zoneName, "KSK")
	if err != nil {
		return nil, err
	}
	signer, err := r.signerFor(ksk)
	if err != nil {
		return nil, fmt.Errorf("loading KSK: %w", err)
	}

	rrset := make([]dns.RR, 0, len(keys))
	for i := range keys {
		if rr := buildDNSKEYRecord(&keys[i], zoneName); rr != nil {
			rrset = append(rrset, rr)
		}
	}
	if len(rrset) == 0 {
		return nil, fmt.Errorf("zone %s has no usable DNSKEY records", zoneName)
	}

	rrsig, err := r.sign(rrset, ksk, signer, zoneName)
	if err != nil {
		return nil, err
	}
	return append(rrset, rrsig), nil
}

// SignRRsets returns rrs with an RRSIG appended for each distinct RRset, all
// made with the active ZSK. On any failure it returns the original rrs
// unchanged -- never a subset of signatures.
func (r *Responder) SignRRsets(zoneName string, rrs []dns.RR) []dns.RR {
	if r == nil || r.db == nil || len(rrs) == 0 {
		return rrs
	}
	zoneName = dns.Fqdn(zoneName)
	zsk, err := r.activeKey(zoneName, "ZSK")
	if err != nil {
		return rrs
	}
	signer, err := r.signerFor(zsk)
	if err != nil {
		return rrs
	}

	out := make([]dns.RR, 0, len(rrs)+2)
	out = append(out, rrs...)
	for _, group := range groupRRsets(rrs) {
		// RRSIGs are themselves never signed. Skip an RRset that is already
		// signed so a caller applying this twice does not nest signatures.
		if len(group) == 0 || group[0].Header().Rrtype == dns.TypeRRSIG {
			continue
		}
		rrsig, err := r.sign(group, zsk, signer, zoneName)
		if err != nil {
			return rrs
		}
		out = append(out, rrsig)
	}
	return out
}

func (r *Responder) sign(rrset []dns.RR, key *DNSSECKey, signer crypto.Signer, zoneName string) (*dns.RRSIG, error) {
	now := time.Now().Unix()
	return SignRRset(rrset, key, signer, zoneName, now-signatureInceptionSkew, now+signatureValiditySeconds)
}

// activeKey returns the most recently created enabled key of kind for a zone
// that is marked DNSSEC-enabled.
func (r *Responder) activeKey(zoneName, kind string) (*DNSSECKey, error) {
	var k DNSSECKey
	err := r.db.QueryRow(`
		SELECT k.id, k.zone_id, k.key_type, k.algorithm, k.public_key, k.key_tag, k.enabled, k.created_at
		FROM dns_dnssec_keys k
		JOIN dns_zones z ON z.id = k.zone_id
		WHERE z.name = ? AND z.dnssec_enabled = 1 AND k.key_type = ? AND k.enabled = 1
		ORDER BY k.created_at DESC, k.id DESC LIMIT 1`, dns.Fqdn(zoneName), kind).
		Scan(&k.ID, &k.ZoneID, &k.KeyType, &k.Algorithm, &k.PublicKey, &k.KeyTag, &k.Enabled, &k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("no active %s for %s: %w", kind, zoneName, err)
	}
	return &k, nil
}

// enabledKeys returns all published keys for a signed zone.
func (r *Responder) enabledKeys(zoneName string) ([]DNSSECKey, error) {
	rows, err := r.db.Query(`
		SELECT k.id, k.zone_id, k.key_type, k.algorithm, k.public_key, k.key_tag, k.enabled, k.created_at
		FROM dns_dnssec_keys k
		JOIN dns_zones z ON z.id = k.zone_id
		WHERE z.name = ? AND z.dnssec_enabled = 1 AND k.enabled = 1
		ORDER BY k.created_at, k.id`, dns.Fqdn(zoneName))
	if err != nil {
		return nil, fmt.Errorf("reading DNSSEC keys for %s: %w", zoneName, err)
	}
	defer rows.Close()

	var keys []DNSSECKey
	for rows.Next() {
		var k DNSSECKey
		if err := rows.Scan(&k.ID, &k.ZoneID, &k.KeyType, &k.Algorithm, &k.PublicKey, &k.KeyTag, &k.Enabled, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning DNSSEC key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating DNSSEC keys: %w", err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("zone %s has no published DNSSEC keys", zoneName)
	}
	return keys, nil
}

func (r *Responder) signerFor(key *DNSSECKey) (crypto.Signer, error) {
	var ciphertext string
	if err := r.db.QueryRow(
		`SELECT private_key_ciphertext FROM dns_dnssec_keys WHERE id = ?`, key.ID,
	).Scan(&ciphertext); err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}
	plaintext, err := decryptPrivateKey(ciphertext, r.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypting private key: %w", err)
	}
	return parsePrivateKey(plaintext, key.Algorithm)
}

// buildDNSKEYRecord constructs the DNSKEY RR published for a stored key.
func buildDNSKEYRecord(key *DNSSECKey, zoneName string) *dns.DNSKEY {
	algoNum, ok := SupportedAlgorithms[DNSSECAlgorithm(key.Algorithm)]
	if !ok {
		return nil
	}
	return &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: dns.Fqdn(zoneName), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: dnssecKeyRRTTL},
		Flags:     dnsKeyFlags(key.KeyType),
		Protocol:  3,
		Algorithm: algoNum,
		PublicKey: key.PublicKey,
	}
}

// groupRRsets partitions records into RRsets keyed by owner name, type and
// class. DNS RRSIGs cover exactly one RRset, so signing a mixed answer with a
// single signature would produce an RRSIG that never validates.
func groupRRsets(rrs []dns.RR) [][]dns.RR {
	groups := make([][]dns.RR, 0, len(rrs))
	index := make(map[string]int)
	for _, rr := range rrs {
		h := rr.Header()
		key := strings.ToLower(h.Name) + "|" + fmt.Sprintf("%d", h.Rrtype) + "|" + fmt.Sprintf("%d", h.Class)
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], rr)
	}
	return groups
}
