package dynamic_update

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
)

// UpdateHandler handles RFC 2136 Dynamic DNS Update messages.
type UpdateHandler struct {
	db        *sql.DB
	zoneStore *zone.Store
	zoneMgr   *zone.ZoneManager
	recordMgr *zone.RecordManager

	// tsigSecrets maps a TSIG key name to its base64-encoded shared secret.
	// The set is registered by the server bootstrap so that dynamic updates
	// can be authenticated against the on-wire TSIG record. When a TSIG key
	// is not present in this map, dynamic updates signed with that key are
	// rejected outright (fail-closed).
	mu          sync.RWMutex
	tsigSecrets map[string]string
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(db *sql.DB, zoneStore *zone.Store, zoneMgr *zone.ZoneManager, recordMgr *zone.RecordManager) *UpdateHandler {
	return &UpdateHandler{
		db:          db,
		zoneStore:   zoneStore,
		zoneMgr:     zoneMgr,
		recordMgr:   recordMgr,
		tsigSecrets: make(map[string]string),
	}
}

// SetTSIGSecrets replaces the in-memory TSIG secret table used to authenticate
// inbound dynamic updates.
func (h *UpdateHandler) SetTSIGSecrets(secrets map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tsigSecrets = make(map[string]string, len(secrets))
	for k, v := range secrets {
		h.tsigSecrets[strings.ToLower(k)] = v
	}
}

// lookupTSIGSecret returns the registered secret for the given TSIG key name.
func (h *UpdateHandler) lookupTSIGSecret(keyName string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	secret, ok := h.tsigSecrets[strings.ToLower(keyName)]
	return secret, ok
}

// HandleUpdate processes an RFC 2136 Dynamic DNS Update message.
// The client IP is unknown in this form, so IP-based update policies are
// skipped. Prefer HandleUpdateFrom when the source address is available.
func (h *UpdateHandler) HandleUpdate(msg *dns.Msg) (*dns.Msg, error) {
	return h.HandleUpdateFrom(msg, "")
}

// HandleUpdateFrom processes an RFC 2136 Dynamic DNS Update message from a
// known client IP (used for IP-based update policies).
func (h *UpdateHandler) HandleUpdateFrom(msg *dns.Msg, clientIP string) (*dns.Msg, error) {
	if len(msg.Question) == 0 {
		return h.makeResponse(msg, dns.RcodeFormatError), nil
	}

	// The question section contains the zone to be updated.
	zoneName := msg.Question[0].Name
	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}

	// Find the zone.
	z, err := h.zoneMgr.GetZoneByName(zoneName)
	if err != nil {
		return h.makeResponse(msg, dns.RcodeNotZone), nil
	}

	// Check if zone is updatable (primary zones only).
	if z.Type != "primary" {
		return h.makeResponse(msg, dns.RcodeRefused), nil
	}

	// RFC 2136 dynamic updates require TSIG authentication. Reject messages
	// that arrive without a TSIG RR.
	tsig := msg.IsTsig()
	if tsig == nil {
		return h.makeResponse(msg, dns.RcodeRefused), nil
	}
	// Reject weak / disallowed TSIG algorithms. Only SHA-256 and SHA-512
	// are accepted; the rest (MD5, SHA-1, SHA-224, SHA-384) are refused
	// even if the caller knows the shared secret.
	if !isAllowedTSIGAlgorithm(tsig.Algorithm) {
		slog.Warn("dynamic_update: rejected TSIG algorithm", "algorithm", tsig.Algorithm)
		return h.makeResponse(msg, dns.RcodeRefused), nil
	}
	// The TSIG key must be registered with the server. A registered key
	// implies the caller has proven possession of the matching shared
	// secret out of band, and gives us something to verify the on-wire
	// signature against.
	secret, ok := h.lookupTSIGSecret(tsig.Hdr.Name)
	if !ok {
		slog.Warn("dynamic_update: unknown TSIG key", "key", tsig.Hdr.Name)
		return h.makeResponse(msg, dns.RcodeRefused), nil
	}
	// Re-pack the message into wire format. The TSIG MAC is computed over
	// the message bytes (with the TSIG RR stripped and the TSIG variables
	// appended), which is exactly what dns.TsigVerify handles internally.
	if err := h.verifyTSIG(msg, secret); err != nil {
		slog.Warn("dynamic_update: TSIG verification failed", "key", tsig.Hdr.Name, "error", err)
		return h.makeResponse(msg, dns.RcodeRefused), nil
	}
	if !h.checkUpdatePolicy(z.ID, tsig.Hdr.Name, clientIP) {
		return h.makeResponse(msg, dns.RcodeRefused), nil
	}

	// Verify that all record names touched by the update fall within the
	// zone. This prevents updates from inserting or removing records
	// outside the authoritative zone.
	if err := h.checkZoneBoundary(z.Name, msg.Ns); err != nil {
		return h.makeResponse(msg, dns.RcodeNotZone), nil
	}
	if err := h.checkZoneBoundary(z.Name, msg.Answer); err != nil {
		return h.makeResponse(msg, dns.RcodeNotZone), nil
	}

	// Process prerequisites.
	if err := h.checkPrerequisites(z.ID, msg); err != nil {
		return h.makeResponse(msg, dns.RcodeRefused), nil
	}

	// Apply updates. RFC 2136 §2.4.2 classifies each RR in the Update
	// section by its CLASS, not by its type:
	//   CLASS ANY  + type ANY  -> delete all RRsets at the name
	//   CLASS NONE            -> delete the specific RRset (match by
	//                            name/type, and by rdata when TTL==0)
	//   CLASS == zone class   -> add the RR
	// The previous dispatch on Rrtype never matched real deletions:
	// delete-RRset messages (real type, CLASS NONE) fell into the add
	// branch and TypeAny/TypeNone Rrtypes do not occur in real traffic.
	var applied int
	for _, rr := range msg.Ns {
		hdr := rr.Header()
		switch {
		case hdr.Class == dns.ClassANY && hdr.Rrtype == dns.TypeANY:
			// Delete all records at a name.
			if err := h.deleteAllRecords(z.ID, hdr.Name); err != nil {
				slog.Error("dynamic_update: delete all records failed", "error", err)
				continue
			}
			applied++

		case hdr.Class == dns.ClassNONE:
			// Delete the specific RRset (or single record when TTL==0).
			if err := h.deleteRecord(z.ID, rr); err != nil {
				slog.Error("dynamic_update: delete record failed", "error", err)
				continue
			}
			applied++

		case hdr.Class == dns.ClassINET:
			// Add record.
			if err := h.addRecord(z.ID, rr, z.Name); err != nil {
				slog.Error("dynamic_update: add record failed", "error", err)
				continue
			}
			applied++

		default:
			slog.Warn("dynamic_update: unsupported class in update section",
				"class", hdr.Class, "type", hdr.Rrtype, "name", hdr.Name)
		}
	}

	// Increment zone serial if any updates were applied.
	if applied > 0 {
		if _, err := h.zoneMgr.IncrementSerial(z.ID); err != nil {
			slog.Error("dynamic_update: increment serial failed", "error", err)
		}

		// Reload zone store.
		if h.zoneStore != nil {
			h.zoneStore.Reload()
		}

		// Audit log the update.
		h.auditLog(z.ID, zoneName, applied)
	}

	return h.makeResponse(msg, dns.RcodeSuccess), nil
}

// checkZoneBoundary ensures that every RR in the supplied slice has a name
// that is within the given zone (or is the zone apex itself). Names that are
// not absolute, not within the zone, or that escape the zone via relative
// references are rejected.
func (h *UpdateHandler) checkZoneBoundary(zoneName string, rrs []dns.RR) error {
	for _, rr := range rrs {
		if rr == nil {
			return fmt.Errorf("nil RR in update")
		}
		name := rr.Header().Name
		if name == "" {
			return fmt.Errorf("empty name in update")
		}
		if !strings.HasSuffix(name, ".") {
			return fmt.Errorf("non-FQDN name in update: %s", name)
		}
		// The zone apex itself is allowed.
		if strings.EqualFold(name, zoneName) {
			continue
		}
		// Otherwise the name must end with "." + zoneName, i.e. be a child
		// label of the zone. We require the dot separator so that
		// "evil.example.com" cannot pass the check for zone "ample.com".
		suffix := "." + zoneName
		if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(suffix)) {
			return fmt.Errorf("name %s is outside zone %s", name, zoneName)
		}
	}
	return nil
}

// checkPrerequisites checks RFC 2136 prerequisites.
func (h *UpdateHandler) checkPrerequisites(zoneID string, msg *dns.Msg) error {
	// Prerequisites are in the Answer section of the update message.
	for _, rr := range msg.Answer {
		switch rr.Header().Rrtype {
		case dns.TypeANY:
			// Name must exist (at least one record).
			name := rr.Header().Name
			var count int
			err := h.db.QueryRow("SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND enabled = 1", zoneID, name).Scan(&count)
			if err != nil || count == 0 {
				return fmt.Errorf("prerequisite not met: name %s must exist", name)
			}

		case dns.TypeNone:
			// Name must not exist.
			name := rr.Header().Name
			var count int
			err := h.db.QueryRow("SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND enabled = 1", zoneID, name).Scan(&count)
			if err == nil && count > 0 {
				return fmt.Errorf("prerequisite not met: name %s must not exist", name)
			}
		}
	}
	return nil
}

// addRecord adds a record from a dynamic update message.
func (h *UpdateHandler) addRecord(zoneID string, rr dns.RR, zoneName string) error {
	hdr := rr.Header()
	name := hdr.Name
	rtype := dns.TypeToString[hdr.Rrtype]
	value := rrValue(rr)
	ttl := int(hdr.Ttl)

	if !zone.SupportedRecordTypes[rtype] {
		return fmt.Errorf("unsupported record type: %s", rtype)
	}

	// Basic record value validation.
	if err := validateRecordValue(rtype, value); err != nil {
		return fmt.Errorf("invalid record value for %s: %w", rtype, err)
	}

	id := uuid.New().String()
	_, err := h.db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, id, zoneID, name, rtype, value, ttl)
	return err
}

// validateRecordValue performs basic format validation on record values.
func validateRecordValue(rtype, value string) error {
	if value == "" {
		return fmt.Errorf("empty value")
	}
	switch rtype {
	case "A":
		if ip := net.ParseIP(value); ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid IPv4 address: %s", value)
		}
	case "AAAA":
		if ip := net.ParseIP(value); ip == nil || ip.To4() != nil {
			return fmt.Errorf("invalid IPv6 address: %s", value)
		}
	case "CNAME", "DNAME", "NS", "PTR", "MX":
		if value == "." {
			return nil
		}
		// Must be a valid domain name.
		if len(value) < 1 || len(value) > 253 {
			return fmt.Errorf("domain name too long or too short")
		}
	}
	return nil
}

// deleteRecord deletes a specific record from a dynamic update message.
func (h *UpdateHandler) deleteRecord(zoneID string, rr dns.RR) error {
	hdr := rr.Header()
	name := hdr.Name
	rtype := dns.TypeToString[hdr.Rrtype]
	value := rrValue(rr)

	_, err := h.db.Exec(`
		DELETE FROM dns_records WHERE zone_id = ? AND name = ? AND type = ? AND value = ?
	`, zoneID, name, rtype, value)
	return err
}

// deleteAllRecords deletes all records at a name.
func (h *UpdateHandler) deleteAllRecords(zoneID string, name string) error {
	_, err := h.db.Exec("DELETE FROM dns_records WHERE zone_id = ? AND name = ?", zoneID, name)
	return err
}

// checkUpdatePolicy checks if a dynamic update is allowed by policy.
func (h *UpdateHandler) checkUpdatePolicy(zoneID string, tsigKeyName string, clientIP string) bool {
	rows, err := h.db.Query(`
		SELECT pattern, action, tsig_key_name FROM dns_dynamic_update_policies
		WHERE zone_id = ? AND enabled = 1
	`, zoneID)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var pattern, action, keyName string
		if err := rows.Scan(&pattern, &action, &keyName); err != nil {
			continue
		}

		// Check TSIG key match.
		if keyName != "" && tsigKeyName != "" {
			if !strings.EqualFold(keyName, tsigKeyName) {
				continue
			}
		}

		// Check IP match if pattern is a CIDR.
		if pattern != "" && clientIP != "" {
			_, ipNet, err := net.ParseCIDR(pattern)
			if err == nil {
				ip := net.ParseIP(clientIP)
				if ip != nil && ipNet.Contains(ip) {
					return action == "allow"
				}
			}
		}

		// If no specific pattern, allow by default.
		if pattern == "" {
			return action == "allow"
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("dynamic_update: failed to iterate policy rows", "error", err)
	}

	// No matching policy found; deny by default.
	return false
}

// makeResponse creates a response message for a dynamic update.
func (h *UpdateHandler) makeResponse(req *dns.Msg, rcode int) *dns.Msg {
	resp := new(dns.Msg)
	resp.SetRcode(req, rcode)
	resp.Authoritative = true
	return resp
}

// isAllowedTSIGAlgorithm reports whether the TSIG algorithm on the wire is
// one we accept. We accept only SHA-256 and SHA-512.
func isAllowedTSIGAlgorithm(algo string) bool {
	switch algo {
	case dns.HmacSHA256, dns.HmacSHA512:
		return true
	}
	return false
}

// verifyTSIG re-packs the message to recover the wire bytes and then asks
// miekg/dns to verify the TSIG MAC. The message must carry exactly one TSIG
// RR (in the Additional section), which the caller is expected to enforce.
func (h *UpdateHandler) verifyTSIG(msg *dns.Msg, secret string) error {
	buf, err := msg.Pack()
	if err != nil {
		return fmt.Errorf("packing DNS message: %w", err)
	}
	if err := dns.TsigVerify(buf, secret, "", false); err != nil {
		return err
	}
	return nil
}

// auditLog writes an audit log entry for a dynamic update.
func (h *UpdateHandler) auditLog(zoneID, zoneName string, count int) {
	slog.Info("dynamic_update: applied updates",
		"zone_id", zoneID,
		"zone_name", zoneName,
		"count", count,
		"timestamp", time.Now().UTC(),
	)

	// Write to audit log in database.
	id := uuid.New().String()
	_, _ = h.db.Exec(`
		INSERT INTO audit_logs (id, user_id, action, resource, resource_id, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
	`, id, "system", "dns_dynamic_update", "zone", zoneID,
		fmt.Sprintf("Dynamic update on zone %s: %d changes applied", zoneName, count))
}

// rrValue extracts the value from a dns.RR.
func rrValue(rr dns.RR) string {
	switch v := rr.(type) {
	case *dns.A:
		return v.A.String()
	case *dns.AAAA:
		return v.AAAA.String()
	case *dns.CNAME:
		return v.Target
	case *dns.MX:
		return v.Mx
	case *dns.TXT:
		return strings.Join(v.Txt, "")
	case *dns.SRV:
		return v.Target
	case *dns.PTR:
		return v.Ptr
	case *dns.NS:
		return v.Ns
	default:
		return ""
	}
}
