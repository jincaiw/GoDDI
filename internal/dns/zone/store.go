package zone

import (
	"database/sql"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// ZoneRecord represents a DNS record stored in the zone.
type ZoneRecord struct {
	ID       string `json:"id"`
	ZoneID   string `json:"zone_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      int    `json:"ttl"`
	Value    string `json:"value"`
	Priority int    `json:"priority,omitempty"`
	Port     int    `json:"port,omitempty"`
	Weight   int    `json:"weight,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Flag     int    `json:"flag,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// zoneData holds the in-memory data for a single zone.
type zoneData struct {
	zone    *Zone
	records map[string][]dns.RR // key: lowercase name -> records
	soa     *dns.SOA
	ns      []*dns.NS
	// special is set for zones that do not participate in authoritative
	// lookups ("allowed" / "blocked"); empty for normal zones.
	special string
}

// Store is an in-memory zone store that loads zones and records from the database.
// It provides fast lookup by zone name and is thread-safe.
type Store struct {
	mu       sync.RWMutex
	zones    map[string]*zoneData // key: lowercase zone name with trailing dot
	db       *sql.DB
	debounce *time.Timer // debounce timer for Reload
}

// NewStore creates a new zone store and loads data from the database.
func NewStore(db *sql.DB) *Store {
	s := &Store{
		zones: make(map[string]*zoneData),
		db:    db,
	}
	if db != nil {
		s.Load()
	}
	return s
}

// Load loads all zones and records from the database.
// If the database query fails, the previously loaded zones are kept so a
// transient DB error cannot wipe authoritative data and break resolution.
func (s *Store) Load() {
	if s.db == nil {
		return
	}

	// Load data outside the lock to avoid blocking readers.
	newZones := s.loadFromDB()

	s.mu.Lock()
	if newZones == nil {
		// loadFromDB signals a query failure with nil: retain the existing
		// map and keep answering authoritatively from the stale data.
		count := len(s.zones)
		s.mu.Unlock()
		slog.Warn("zone_store: reload failed, keeping previous zone data", "count", count)
		return
	}
	s.zones = newZones
	s.mu.Unlock()

	slog.Info("zone_store: loaded zones", "count", len(newZones))
}

// loadFromDB performs all database queries and builds the zone map.
func (s *Store) loadFromDB() map[string]*zoneData {
	// Load zones with full metadata.
	zoneRows, err := s.db.Query(`
		SELECT id, name, type, enabled, dnssec_enabled, default_ttl,
			soa_mname, soa_rname, serial, refresh, retry, expire, minimum,
			transfer_policy, update_policy, acl, COALESCE(catalog, ''), created_at, updated_at
		FROM dns_zones WHERE enabled = 1
	`)
	if err != nil {
		slog.Error("zone_store: failed to load zones", "error", err)
		return nil
	}
	defer zoneRows.Close()

	zoneMap := make(map[string]*Zone) // id -> Zone
	for zoneRows.Next() {
		var z Zone
		var transferPolicy, updatePolicy, aclJSON sql.NullString
		if err := zoneRows.Scan(
			&z.ID, &z.Name, &z.Type, &z.Enabled, &z.DNSSECEnabled, &z.DefaultTTL,
			&z.SOA_MName, &z.SOA_RName, &z.Serial, &z.Refresh, &z.Retry, &z.Expire, &z.Minimum,
			&transferPolicy, &updatePolicy, &aclJSON, &z.Catalog, &z.CreatedAt, &z.UpdatedAt,
		); err != nil {
			slog.Error("zone_store: failed to scan zone", "error", err)
			continue
		}
		if transferPolicy.Valid {
			z.TransferPolicy = transferPolicy.String
		}
		if updatePolicy.Valid {
			z.UpdatePolicy = updatePolicy.String
		}
		if aclJSON.Valid {
			z.ACL = parseZoneACL(aclJSON.String)
		}
		zoneMap[z.ID] = &z
	}
	if err := zoneRows.Err(); err != nil {
		slog.Error("zone_store: failed to iterate zones", "error", err)
	}

	// Load records. Records whose expiry is in the past are skipped so an
	// aged-out record never answers queries even before the cleanup task runs.
	recordRows, err := s.db.Query(`
		SELECT id, zone_id, name, type, ttl, value, priority, port, weight, tag, flag, enabled
		FROM dns_records
		WHERE enabled = 1 AND (expires_at IS NULL OR expires_at > datetime('now'))
	`)
	if err != nil {
		slog.Error("zone_store: failed to load records", "error", err)
		return nil
	}
	defer recordRows.Close()

	// Group records by zone.
	zoneRecords := make(map[string][]ZoneRecord) // zoneID -> records
	for recordRows.Next() {
		var r ZoneRecord
		var priority, port, weight, flag sql.NullInt64
		var tag sql.NullString
		if err := recordRows.Scan(
			&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.TTL, &r.Value,
			&priority, &port, &weight, &tag, &flag, &r.Enabled,
		); err != nil {
			slog.Error("zone_store: failed to scan record", "error", err)
			continue
		}
		if priority.Valid {
			r.Priority = int(priority.Int64)
		}
		if port.Valid {
			r.Port = int(port.Int64)
		}
		if weight.Valid {
			r.Weight = int(weight.Int64)
		}
		if tag.Valid {
			r.Tag = tag.String
		}
		if flag.Valid {
			r.Flag = int(flag.Int64)
		}
		zoneRecords[r.ZoneID] = append(zoneRecords[r.ZoneID], r)
	}
	if err := recordRows.Err(); err != nil {
		slog.Error("zone_store: failed to iterate records", "error", err)
	}

	// Build in-memory zone data.
	newZones := make(map[string]*zoneData)
	for zoneID, z := range zoneMap {
		zd := &zoneData{
			zone:    z,
			records: make(map[string][]dns.RR),
		}

		// Special zone types do not participate in authoritative lookups;
		// they act as allow/block policy zones instead.
		if z.Type == string(ZoneTypeAllowed) || z.Type == string(ZoneTypeBlocked) {
			zd.special = z.Type
		}

		zoneName := dns.Fqdn(strings.ToLower(z.Name))

		// A zone created via the API stores its SOA fields as zone columns
		// (soa_mname, soa_rname, serial, ...) and does not necessarily have
		// an SOA row in dns_records. Synthesize the SOA from the zone
		// metadata so apex SOA queries and negative answers work; an explicit
		// SOA record row still takes precedence.
		if z.SOA_MName != "" && z.SOA_RName != "" {
			zd.soa = &dns.SOA{
				Hdr: dns.RR_Header{
					Name:   zoneName,
					Rrtype: dns.TypeSOA,
					Class:  dns.ClassINET,
					Ttl:    uint32(z.Minimum),
				},
				Ns:      dns.Fqdn(z.SOA_MName),
				Mbox:    dns.Fqdn(z.SOA_RName),
				Serial:  uint32(z.Serial),
				Refresh: uint32(z.Refresh),
				Retry:   uint32(z.Retry),
				Expire:  uint32(z.Expire),
				Minttl:  uint32(z.Minimum),
			}
		}

		for _, r := range zoneRecords[zoneID] {
			rr := buildRR(r, zoneName)
			if rr == nil {
				continue
			}

			name := dns.Fqdn(strings.ToLower(r.Name))
			zd.records[name] = append(zd.records[name], rr)

			// Track SOA and NS separately for zone-level queries.
			switch r.Type {
			case "SOA":
				if soa, ok := rr.(*dns.SOA); ok {
					zd.soa = soa
				}
			case "NS":
				if ns, ok := rr.(*dns.NS); ok {
					zd.ns = append(zd.ns, ns)
				}
			}
		}

		newZones[zoneName] = zd
	}

	return newZones
}

// Reload reloads all zone data from the database with a debounce mechanism.
// If multiple changes happen within 500ms, only one reload is performed.
func (s *Store) Reload() {
	s.mu.Lock()
	if s.debounce != nil {
		s.debounce.Stop()
	}
	s.debounce = time.AfterFunc(500*time.Millisecond, func() {
		s.Load()
	})
	s.mu.Unlock()
}

// Lookup looks up records in a zone by query name and type.
// Returns the matching zone name, answer records, and whether a match was found.
func (s *Store) Lookup(qname string, qtype uint16) (string, []dns.RR, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	qname = dns.Fqdn(strings.ToLower(qname))

	// Try exact zone match first, then parent zones.
	name := qname
	for {
		if zd, ok := s.zones[name]; ok && zd.special == "" {
			answers := s.lookupInZone(zd, qname, qtype)
			if answers != nil {
				return name, answers, true
			}
		}

		// Move up one label.
		idx := strings.IndexByte(name, '.')
		if idx < 0 || idx >= len(name)-1 {
			break
		}
		name = name[idx+1:]

		if name == "." {
			if zd, ok := s.zones["."]; ok {
				answers := s.lookupInZone(zd, qname, qtype)
				if answers != nil {
					return ".", answers, true
				}
			}
			break
		}
	}

	return "", nil, false
}

// MatchingZone returns the most specific local zone that would be
// authoritative for qname (exact or parent match), or "" if none.
func (s *Store) MatchingZone(qname string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name := dns.Fqdn(strings.ToLower(qname))
	for {
		if zd, ok := s.zones[name]; ok && zd.special == "" {
			return name
		}
		idx := strings.IndexByte(name, '.')
		if idx < 0 || idx >= len(name)-1 {
			return ""
		}
		name = name[idx+1:]
		if name == "." {
			if zd, ok := s.zones["."]; ok && zd.special == "" {
				return "."
			}
			return ""
		}
	}
}

// ACLAllows checks the zone-level ACL of the most specific zone matching
// qname for the given kind ("query"/"transfer"/"update") and client IP.
// Returns true when no zone or no ACL restricts the kind.
func (s *Store) ACLAllows(kind, qname, ip string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name := dns.Fqdn(strings.ToLower(qname))
	for {
		if zd, ok := s.zones[name]; ok {
			return zd.zone.ACL.ACLAllows(kind, ip)
		}
		idx := strings.IndexByte(name, '.')
		if idx < 0 || idx >= len(name)-1 {
			return true
		}
		name = name[idx+1:]
		if name == "." {
			if zd, ok := s.zones["."]; ok {
				return zd.zone.ACL.ACLAllows(kind, ip)
			}
			return true
		}
	}
}

// MatchSpecial returns "allowed" or "blocked" when qname falls inside the
// most specific special policy zone of that kind, "" otherwise.
func (s *Store) MatchSpecial(qname string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name := dns.Fqdn(strings.ToLower(qname))
	for {
		if zd, ok := s.zones[name]; ok && zd.special != "" {
			return zd.special
		}
		idx := strings.IndexByte(name, '.')
		if idx < 0 || idx >= len(name)-1 {
			return ""
		}
		name = name[idx+1:]
		if name == "." {
			if zd, ok := s.zones["."]; ok && zd.special != "" {
				return zd.special
			}
			return ""
		}
	}
}

// ForwardTargets returns the upstream addresses configured for the most
// specific forward/stub zone matching qname, plus its kind. Forward zones
// take their upstreams from the ACL Notify list (reused as target list);
// stub zones resolve their NS records' nameservers from the loaded zone.
func (s *Store) ForwardTargets(qname string) (kind string, targets []string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name := dns.Fqdn(strings.ToLower(qname))
	for {
		if zd, ok2 := s.zones[name]; ok2 {
			switch zd.zone.Type {
			case string(ZoneTypeForward):
				if len(zd.zone.ACL.GetNotify()) > 0 {
					return "forward", zd.zone.ACL.GetNotify(), true
				}
			case string(ZoneTypeStub):
				var addrs []string
				for _, ns := range zd.ns {
					host := ns.Ns
					addrs = append(addrs, net.JoinHostPort(strings.TrimSuffix(host, "."), "53"))
				}
				if len(addrs) > 0 {
					return "stub", addrs, true
				}
			}
		}
		idx := strings.IndexByte(name, '.')
		if idx < 0 || idx >= len(name)-1 {
			return "", nil, false
		}
		name = name[idx+1:]
		if name == "." {
			return "", nil, false
		}
	}
}

// ZoneSOA returns the SOA record of the given zone (used in the authority
// section of authoritative negative responses).
func (s *Store) ZoneSOA(zoneName string) dns.RR {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if zd, ok := s.zones[zoneName]; ok && zd.soa != nil {
		return zd.soa
	}
	return nil
}

// NameExists reports whether any record with the exact qname exists in the
// zone (any type). Used to distinguish NODATA from NXDOMAIN.
func (s *Store) NameExists(zoneName, qname string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	zd, ok := s.zones[zoneName]
	if !ok {
		return false
	}
	_, exists := zd.records[dns.Fqdn(strings.ToLower(qname))]
	return exists
}

// lookupInZone searches for matching records in a specific zone.
func (s *Store) lookupInZone(zd *zoneData, qname string, qtype uint16) []dns.RR {
	zoneName := dns.Fqdn(strings.ToLower(zd.zone.Name))

	// SOA query for the zone itself.
	if qtype == dns.TypeSOA && strings.EqualFold(qname, zoneName) {
		if zd.soa != nil {
			return []dns.RR{zd.soa}
		}
		return nil
	}

	// NS query for the zone itself.
	if qtype == dns.TypeNS && strings.EqualFold(qname, zoneName) {
		if len(zd.ns) > 0 {
			result := make([]dns.RR, len(zd.ns))
			for i, ns := range zd.ns {
				result[i] = ns
			}
			return result
		}
		return nil
	}

	// Look up records by name.
	rrs, ok := zd.records[qname]
	if !ok {
		// RFC 4592 wildcard match: try the closest enclosing wildcard first
		// ("*.b.example.com." before "*.example.com.").
		for _, wild := range wildcardCandidates(zoneName, qname) {
			if wrrs, wok := zd.records[wild]; wok {
				rrs = wrrs
				ok = true
				// Present the answer under the queried name, not under the
				// wildcard name that produced it.
				rrs = rewriteRRName(rrs, qname)
				break
			}
		}
	}
	if !ok {
		return nil
	}

	// Filter by type.
	var answers []dns.RR
	for _, rr := range rrs {
		if rr.Header().Rrtype == qtype {
			answers = append(answers, rr)
		}
	}

	// If no specific type match, check for CNAME (unless querying for CNAME).
	if len(answers) == 0 && qtype != dns.TypeCNAME {
		for _, rr := range rrs {
			if rr.Header().Rrtype == dns.TypeCNAME {
				answers = append(answers, rr)
			}
		}
	}

	return answers
}

// wildcardCandidates returns the wildcard names to try for qname inside
// zoneName, ordered from the closest enclosing wildcard to the zone apex
// wildcard (e.g. "a.b.example.com." yields "*.b.example.com." then
// "*.example.com."). Both arguments are FQDNs. Returns nil when qname is not
// inside the zone.
func wildcardCandidates(zoneName, qname string) []string {
	if !strings.HasSuffix(strings.ToLower(qname), strings.ToLower(zoneName)) {
		return nil
	}
	prefix := strings.TrimSuffix(qname, zoneName)
	if prefix == "" {
		return nil
	}
	labels := strings.Split(strings.TrimSuffix(prefix, "."), ".")
	candidates := make([]string, 0, len(labels))
	for i := 0; i < len(labels); i++ {
		rest := strings.Join(labels[i+1:], ".")
		if rest == "" {
			candidates = append(candidates, "*."+zoneName)
			continue
		}
		candidates = append(candidates, "*."+rest+"."+zoneName)
	}
	return candidates
}

// rewriteRRName returns copies of rrs with their owner name set to name, so a
// wildcard answer is presented under the name that was actually queried.
func rewriteRRName(rrs []dns.RR, name string) []dns.RR {
	out := make([]dns.RR, 0, len(rrs))
	for _, rr := range rrs {
		clone := dns.Copy(rr)
		clone.Header().Name = name
		out = append(out, clone)
	}
	return out
}

// GetZoneSOA returns the SOA record for a zone.
func (s *Store) GetZoneSOA(zoneName string) *dns.SOA {
	s.mu.RLock()
	defer s.mu.RUnlock()

	zd, ok := s.zones[dns.Fqdn(strings.ToLower(zoneName))]
	if !ok {
		return nil
	}
	return zd.soa
}

// ZoneNames returns all loaded zone names.
func (s *Store) ZoneNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.zones))
	for name := range s.zones {
		names = append(names, name)
	}
	return names
}

// GetDB returns the underlying database connection.
func (s *Store) GetDB() *sql.DB {
	return s.db
}

// buildRR constructs a dns.RR from a ZoneRecord.
func buildRR(r ZoneRecord, zoneName string) dns.RR {
	ttl := uint32(r.TTL)
	if ttl == 0 {
		ttl = 3600
	}

	name := r.Name
	if name == "" || name == "@" {
		name = zoneName
	}
	name = dns.Fqdn(name)

	qtype, ok := dns.StringToType[r.Type]
	if !ok {
		return nil
	}
	hdr := dns.RR_Header{
		Name:   name,
		Rrtype: qtype,
		Class:  dns.ClassINET,
		Ttl:    ttl,
	}

	switch r.Type {
	case "A":
		ip := net.ParseIP(r.Value)
		if ip == nil {
			return nil
		}
		return &dns.A{Hdr: hdr, A: ip}

	case "AAAA":
		ip := net.ParseIP(r.Value)
		if ip == nil {
			return nil
		}
		return &dns.AAAA{Hdr: hdr, AAAA: ip}

	case "CNAME":
		return &dns.CNAME{Hdr: hdr, Target: dns.Fqdn(r.Value)}

	case "DNAME":
		return &dns.DNAME{Hdr: hdr, Target: dns.Fqdn(r.Value)}

	case "MX":
		return &dns.MX{Hdr: hdr, Preference: uint16(r.Priority), Mx: dns.Fqdn(r.Value)}

	case "TXT":
		return &dns.TXT{Hdr: hdr, Txt: splitTXT(r.Value)}

	case "SRV":
		return &dns.SRV{
			Hdr:      hdr,
			Priority: uint16(r.Priority),
			Weight:   uint16(r.Weight),
			Port:     uint16(r.Port),
			Target:   dns.Fqdn(r.Value),
		}

	case "PTR":
		return &dns.PTR{Hdr: hdr, Ptr: dns.Fqdn(r.Value)}

	case "NS":
		return &dns.NS{Hdr: hdr, Ns: dns.Fqdn(r.Value)}

	case "SOA":
		parts := strings.Fields(r.Value)
		if len(parts) < 7 {
			return nil
		}
		serial := mustUint32(parts[2])
		refresh := mustUint32(parts[3])
		retry := mustUint32(parts[4])
		expire := mustUint32(parts[5])
		minttl := mustUint32(parts[6])
		return &dns.SOA{
			Hdr:     hdr,
			Ns:      dns.Fqdn(parts[0]),
			Mbox:    dns.Fqdn(parts[1]),
			Serial:  serial,
			Refresh: refresh,
			Retry:   retry,
			Expire:  expire,
			Minttl:  minttl,
		}

	case "CAA":
		return &dns.CAA{
			Hdr:   hdr,
			Flag:  uint8(r.Flag),
			Tag:   r.Tag,
			Value: r.Value,
		}

	case "NAPTR":
		return &dns.NAPTR{
			Hdr:         hdr,
			Order:       uint16(r.Priority),
			Preference:  uint16(r.Weight),
			Flags:       "",
			Service:     "",
			Regexp:      "",
			Replacement: dns.Fqdn(r.Value),
		}

	case "SSHFP":
		parts := strings.Fields(r.Value)
		if len(parts) < 3 {
			return nil
		}
		return &dns.SSHFP{
			Hdr:         hdr,
			Algorithm:   uint8(mustUint32(parts[0])),
			Type:        uint8(mustUint32(parts[1])),
			FingerPrint: parts[2],
		}

	case "TLSA":
		parts := strings.Fields(r.Value)
		if len(parts) < 4 {
			return nil
		}
		return &dns.TLSA{
			Hdr:          hdr,
			Usage:        uint8(mustUint32(parts[0])),
			Selector:     uint8(mustUint32(parts[1])),
			MatchingType: uint8(mustUint32(parts[2])),
			Certificate:  parts[3],
		}

	case "DS":
		parts := strings.Fields(r.Value)
		if len(parts) < 4 {
			return nil
		}
		return &dns.DS{
			Hdr:        hdr,
			KeyTag:     uint16(mustUint32(parts[0])),
			Algorithm:  uint8(mustUint32(parts[1])),
			DigestType: uint8(mustUint32(parts[2])),
			Digest:     parts[3],
		}

	case "DNSKEY":
		parts := strings.Fields(r.Value)
		if len(parts) < 4 {
			return nil
		}
		return &dns.DNSKEY{
			Hdr:       hdr,
			Flags:     uint16(mustUint32(parts[0])),
			Protocol:  uint8(mustUint32(parts[1])),
			Algorithm: uint8(mustUint32(parts[2])),
			PublicKey: parts[3],
		}

	case "URI":
		return &dns.URI{
			Hdr:      hdr,
			Priority: uint16(r.Priority),
			Weight:   uint16(r.Weight),
			Target:   r.Value,
		}

	case "RP":
		parts := strings.Fields(r.Value)
		if len(parts) < 2 {
			return nil
		}
		return &dns.RP{
			Hdr:  hdr,
			Mbox: dns.Fqdn(parts[0]),
			Txt:  dns.Fqdn(parts[1]),
		}

	case "SVCB", "HTTPS":
		// RFC 9460 presentation format: "priority target [key=value ...]".
		// Delegate to miekg's parser so SvcParams are handled correctly.
		rr, err := dns.NewRR(name + " " + strconv.FormatUint(uint64(ttl), 10) + " IN " + r.Type + " " + r.Value)
		if err != nil {
			return nil
		}
		return rr

	default:
		// Generic fallback (RFC 3597): parse the record from presentation
		// format so less common types (HINFO, LOC, SPF, unknown types
		// stored as \# ...) still answer authoritatively. Technitium
		// supports unknown record types the same way.
		rr, err := dns.NewRR(name + " " + strconv.FormatUint(uint64(ttl), 10) + " IN " + r.Type + " " + r.Value)
		if err != nil {
			return nil
		}
		return rr
	}
}

// splitTXT splits a TXT record value into 255-byte chunks.
func splitTXT(s string) []string {
	if len(s) <= 255 {
		return []string{s}
	}
	var chunks []string
	for len(s) > 255 {
		chunks = append(chunks, s[:255])
		s = s[255:]
	}
	if len(s) > 0 {
		chunks = append(chunks, s)
	}
	return chunks
}

// mustUint32 parses a string as uint32, returning 0 on error.
func mustUint32(s string) uint32 {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}
