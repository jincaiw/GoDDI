package dynamic_update

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/auditlog"
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

	// Parse and validate the entire update section before touching the
	// database. RFC 2136 §3.4.1.1 requires a malformed update to be rejected
	// with FORMERR and no side effects; the previous implementation applied
	// RRs one by one and skipped individual failures, leaving the zone in a
	// half-updated state that no client could observe consistently.
	ops, rcode := planUpdateOps(msg.Ns)
	if rcode != dns.RcodeSuccess {
		slog.Warn("dynamic_update: rejected update section",
			"zone", zoneName, "rcode", dns.RcodeToString[rcode])
		return h.makeResponse(msg, rcode), nil
	}

	// Read the records at the names this update touches, before and after
	// applying it. The ops carry the names, so the snapshot is bounded by the
	// update rather than by the zone -- a zone-wide read on every update would
	// cost more than the update itself.
	//
	// A failure to snapshot degrades the audit entry, not the update: the
	// client's change has already been committed by the time the "after" read
	// runs, and refusing to report success would tell it to retry a change that
	// already happened.
	before := h.snapshotRecords(z.ID, ops)

	// Apply prerequisites, changes and the SOA serial bump as one unit.
	rcode = h.applyAtomic(z.ID, msg, ops)
	if rcode != dns.RcodeSuccess {
		slog.Warn("dynamic_update: update rejected",
			"zone", zoneName, "rcode", dns.RcodeToString[rcode], "ops", len(ops))
		return h.makeResponse(msg, rcode), nil
	}

	if len(ops) > 0 {
		// Only publish the new data after the durable commit: reloading the
		// in-memory store first could serve records that a later rollback
		// erased.
		if h.zoneStore != nil {
			h.zoneStore.Reload()
		}
		h.auditLog(z.ID, zoneName, tsig.Hdr.Name, before, h.snapshotRecords(z.ID, ops), len(ops))
	}

	return h.makeResponse(msg, dns.RcodeSuccess), nil
}

// recordState is one dns_records row, reduced to what identifies it.
type recordState struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// snapshotRecords renders the records at the names an update touches, as a
// JSON array, or "" when there are none.
func (h *UpdateHandler) snapshotRecords(zoneID string, ops []updateOp) string {
	names := make([]string, 0, len(ops))
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		if op.name == "" || seen[op.name] {
			continue
		}
		seen[op.name] = true
		names = append(names, op.name)
	}
	if len(names) == 0 {
		return ""
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	args := make([]any, 0, len(names)+1)
	args = append(args, zoneID)
	for _, n := range names {
		args = append(args, n)
	}
	// h.db is the DNS plane's own store and runs on a single connection: every
	// row is read into the slice before the cursor closes, so the audit write
	// that follows on the same connection cannot find it still open.
	rows, err := h.db.Query(
		`SELECT name, type, value FROM dns_records
		 WHERE zone_id = ? AND name IN (`+placeholders+`)
		 ORDER BY name, type, value`, args...)
	if err != nil {
		slog.Error("dynamic_update: could not read records for the audit entry", "error", err)
		return ""
	}
	states := make([]recordState, 0, len(names))
	for rows.Next() {
		var s recordState
		if err := rows.Scan(&s.Name, &s.Type, &s.Value); err != nil {
			slog.Error("dynamic_update: scanning records for the audit entry", "error", err)
			rows.Close()
			return ""
		}
		states = append(states, s)
	}
	if err := rows.Err(); err != nil {
		slog.Error("dynamic_update: iterating records for the audit entry", "error", err)
	}
	rows.Close()

	if len(states) == 0 {
		return ""
	}
	encoded, err := json.Marshal(states)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// updateOpKind enumerates the operations an RFC 2136 update section can carry.
type updateOpKind int

const (
	opDeleteName  updateOpKind = iota // CLASS ANY  + type ANY: drop every RRset at a name
	opDeleteRRset                     // CLASS ANY  + type X  : drop the RRset of type X
	opDeleteRR                        // CLASS NONE + type X  : drop one RR matched by rdata
	opAdd                             // CLASS == zone class  : add an RR to its RRset
)

// updateOp is one validated mutation from the update section.
type updateOp struct {
	kind  updateOpKind
	name  string
	rtype string
	value string
	ttl   int
}

// dynamicUpdateTypes is the set of record types this handler can encode
// losslessly from the wire form into the stored (type, value) pair. MX and SRV
// are deliberately absent: their preference/weight/port live in companion
// columns that the update path cannot populate, so storing them would silently
// produce records with zeroed priorities.
var dynamicUpdateTypes = map[string]bool{
	"A": true, "AAAA": true, "NS": true, "CNAME": true,
	"DNAME": true, "TXT": true, "PTR": true,
}

// planUpdateOps validates the RFC 2136 update section and converts it into a
// list of operations. Nothing is written here: any invalid or unsupported RR
// aborts the whole update, so a rejected message can never change the zone.
//
// Classification is by CLASS, per RFC 2136 §2.4.2:
//
//	CLASS ANY  + type ANY -> delete every RRset at the name
//	CLASS ANY  + type X   -> delete the whole RRset of type X
//	CLASS NONE + type X   -> delete a single RR identified by its rdata
//	CLASS IN              -> add the RR
func planUpdateOps(rrs []dns.RR) ([]updateOp, int) {
	ops := make([]updateOp, 0, len(rrs))

	for _, rr := range rrs {
		if rr == nil {
			return nil, dns.RcodeFormatError
		}
		hdr := rr.Header()
		if hdr.Name == "" {
			return nil, dns.RcodeFormatError
		}
		rtype := dns.TypeToString[hdr.Rrtype]

		switch hdr.Class {
		case dns.ClassANY:
			if hdr.Rrtype == dns.TypeANY {
				ops = append(ops, updateOp{kind: opDeleteName, name: hdr.Name})
				continue
			}
			if rtype == "" {
				return nil, dns.RcodeFormatError
			}
			ops = append(ops, updateOp{kind: opDeleteRRset, name: hdr.Name, rtype: rtype})

		case dns.ClassNONE:
			// A value-dependent delete must name a concrete type; TYPE ANY
			// is meaningless here (RFC 2136 §2.4.2).
			if hdr.Rrtype == dns.TypeANY || rtype == "" {
				return nil, dns.RcodeFormatError
			}
			value := rrValue(rr)
			if value == "" {
				return nil, dns.RcodeFormatError
			}
			ops = append(ops, updateOp{kind: opDeleteRR, name: hdr.Name, rtype: rtype, value: value})

		case dns.ClassINET:
			if !dynamicUpdateTypes[rtype] {
				// Either the zone cannot store this type, or the update path
				// cannot encode it faithfully. Report NOTIMP instead of
				// writing a truncated value.
				return nil, dns.RcodeNotImplemented
			}
			value := rrValue(rr)
			if err := validateRecordValue(rtype, value); err != nil {
				return nil, dns.RcodeFormatError
			}
			ops = append(ops, updateOp{
				kind: opAdd, name: hdr.Name, rtype: rtype,
				value: value, ttl: int(hdr.Ttl),
			})

		default:
			return nil, dns.RcodeFormatError
		}
	}

	return ops, dns.RcodeSuccess
}

// applyAtomic runs the prerequisite check, the update section and the SOA
// serial bump inside a single database transaction. RFC 2136 §3.4.2 requires
// the update section to take effect as a unit: any failure rolls the whole
// update back and the client receives SERVFAIL, never a partial change.
//
// Every statement goes through tx and never through h.db: the SQLite pool is
// capped at a single connection, so a nested query on h.db while this
// transaction holds that connection would block forever rather than error.
func (h *UpdateHandler) applyAtomic(zoneID string, msg *dns.Msg, ops []updateOp) int {
	tx, err := h.db.Begin()
	if err != nil {
		slog.Error("dynamic_update: begin transaction failed", "error", err)
		return dns.RcodeServerFailure
	}
	// Rollback is a no-op once the transaction has been committed.
	defer func() { _ = tx.Rollback() }()

	if rc := h.checkPrerequisites(tx, zoneID, msg); rc != dns.RcodeSuccess {
		return rc
	}

	// A prerequisite-only update is legal and must not bump the serial.
	if len(ops) == 0 {
		return dns.RcodeSuccess
	}

	changes, rc, err := applyOps(tx, zoneID, ops)
	if err != nil {
		slog.Error("dynamic_update: applying update section failed", "error", err)
		return dns.RcodeServerFailure
	}
	if rc != dns.RcodeSuccess {
		return rc
	}
	if len(changes) == 0 {
		// Every operation was a no-op (e.g. re-adding an identical RR);
		// leave the serial alone so secondaries are not woken for nothing.
		return dns.RcodeSuccess
	}

	newSerial, err := nextZoneSerial(tx, zoneID)
	if err != nil {
		slog.Error("dynamic_update: serial bump failed", "error", err)
		return dns.RcodeServerFailure
	}

	if err := logChanges(tx, zoneID, newSerial, changes); err != nil {
		slog.Error("dynamic_update: zone change journal write failed", "error", err)
		return dns.RcodeServerFailure
	}

	if err := tx.Commit(); err != nil {
		slog.Error("dynamic_update: commit failed", "error", err)
		return dns.RcodeServerFailure
	}
	return dns.RcodeSuccess
}

// change is one journal entry destined for dns_zone_changes, used to answer
// IXFR (RFC 1995) requests.
type change struct {
	changeType string
	name       string
	rtype      string
	value      string
	ttl        int
}

// applyOps executes the validated operations inside tx and returns the journal
// entries describing what actually changed. The returned rcode is the RFC 2136
// rcode to report to the client (the caller rolls the transaction back); a
// non-nil error means the database itself failed.
func applyOps(tx *sql.Tx, zoneID string, ops []updateOp) ([]change, int, error) {
	var changes []change

	for _, op := range ops {
		switch op.kind {
		case opDeleteName:
			existing, err := matchingRecords(tx, zoneID, op.name, "")
			if err != nil {
				return nil, 0, err
			}
			if _, err := tx.Exec(
				"DELETE FROM dns_records WHERE zone_id = ? AND name = ?", zoneID, op.name); err != nil {
				return nil, 0, err
			}
			changes = append(changes, existing...)

		case opDeleteRRset:
			existing, err := matchingRecords(tx, zoneID, op.name, op.rtype)
			if err != nil {
				return nil, 0, err
			}
			if _, err := tx.Exec(
				"DELETE FROM dns_records WHERE zone_id = ? AND name = ? AND type = ?",
				zoneID, op.name, op.rtype); err != nil {
				return nil, 0, err
			}
			changes = append(changes, existing...)

		case opDeleteRR:
			existing, err := matchingRecords(tx, zoneID, op.name, op.rtype)
			if err != nil {
				return nil, 0, err
			}
			for _, c := range existing {
				if c.value != op.value {
					continue
				}
				changes = append(changes, c)
			}
			if _, err := tx.Exec(
				"DELETE FROM dns_records WHERE zone_id = ? AND name = ? AND type = ? AND value = ?",
				zoneID, op.name, op.rtype, op.value); err != nil {
				return nil, 0, err
			}

		case opAdd:
			added, rc, err := addRecord(tx, zoneID, op)
			if err != nil {
				return nil, 0, err
			}
			if rc != dns.RcodeSuccess {
				return nil, rc, nil
			}
			if added {
				changes = append(changes, change{
					changeType: "add", name: op.name,
					rtype: op.rtype, value: op.value, ttl: op.ttl,
				})
			}
		}
	}

	return changes, dns.RcodeSuccess, nil
}

// matchingRecords materialises the records a delete operation will remove, so
// the IXFR journal can describe each RR individually. The rows are fully read
// and closed before the caller issues its DELETE: the single-connection SQLite
// pool deadlocks if a nested statement runs while a cursor is still open.
func matchingRecords(tx *sql.Tx, zoneID, name, rtype string) ([]change, error) {
	query := "SELECT name, type, value, ttl FROM dns_records WHERE zone_id = ? AND name = ?"
	args := []interface{}{zoneID, name}
	if rtype != "" {
		query += " AND type = ?"
		args = append(args, rtype)
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []change
	for rows.Next() {
		var c change
		if err := rows.Scan(&c.name, &c.rtype, &c.value, &c.ttl); err != nil {
			return nil, err
		}
		c.changeType = "delete"
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// addRecord inserts one RR from an update operation. It reports added=false
// when the exact RR already exists (an RRset is a set, so the add is a no-op)
// and returns YXRRSET when the name would end up with both a CNAME and another
// RRset, which DNS forbids (RFC 1034 §3.6.2, enforced by RFC 2136 §3.4.2.2).
func addRecord(tx *sql.Tx, zoneID string, op updateOp) (bool, int, error) {
	exists, err := rdataExists(tx, zoneID, op.name, op.rtype, op.value)
	if err != nil {
		return false, 0, err
	}
	if exists {
		return false, dns.RcodeSuccess, nil
	}

	otherTypes, cnameCount, err := recordKindsAtName(tx, zoneID, op.name)
	if err != nil {
		return false, 0, err
	}
	if op.rtype == "CNAME" {
		// At most one CNAME per name, and nothing else alongside it.
		if cnameCount > 0 || otherTypes > 0 {
			return false, dns.RcodeYXRrset, nil
		}
	} else if cnameCount > 0 {
		return false, dns.RcodeYXRrset, nil
	}

	// authored_locally marks the row as this plane's, which is what the
	// downward sync uses as its boundary: the row is not in the control plane's
	// gift, so a configuration change elsewhere must not withdraw a name a
	// client was authorised to add.
	if _, err := tx.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled, authored_locally)
		VALUES (?, ?, ?, ?, ?, ?, 1, 1)
	`, uuid.New().String(), zoneID, op.name, op.rtype, op.value, op.ttl); err != nil {
		return false, 0, err
	}
	return true, dns.RcodeSuccess, nil
}

// recordKindsAtName reports how many records at a name are non-CNAME and how
// many are CNAME. Both counts are needed to enforce CNAME exclusivity.
func recordKindsAtName(tx *sql.Tx, zoneID, name string) (otherTypes, cnameCount int, err error) {
	err = tx.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN type <> 'CNAME' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type =  'CNAME' THEN 1 ELSE 0 END), 0)
		FROM dns_records WHERE zone_id = ? AND name = ?`, zoneID, name,
	).Scan(&otherTypes, &cnameCount)
	return otherTypes, cnameCount, err
}

// nextZoneSerial bumps and returns the zone's SOA serial inside tx, using the
// same YYYYMMDDNN sequence as ZoneManager.IncrementSerial so dynamic updates
// and record edits advance a single monotonic series for the zone.
func nextZoneSerial(tx *sql.Tx, zoneID string) (uint32, error) {
	var current uint32
	if err := tx.QueryRow("SELECT serial FROM dns_zones WHERE id = ?", zoneID).Scan(&current); err != nil {
		return 0, fmt.Errorf("querying serial: %w", err)
	}

	next := zone.NextSerial(current)
	if next <= current {
		next = current + 1
	}

	if _, err := tx.Exec(
		"UPDATE dns_zones SET serial = ?, updated_at = datetime('now') WHERE id = ?",
		next, zoneID); err != nil {
		return 0, fmt.Errorf("updating serial: %w", err)
	}
	return next, nil
}

// logChanges appends the applied mutations to the zone change journal. It runs
// in the same transaction as the records themselves: an IXFR client must never
// see a serial whose changes were rolled back.
func logChanges(tx *sql.Tx, zoneID string, serial uint32, changes []change) error {
	for _, c := range changes {
		if _, err := tx.Exec(`
			INSERT INTO dns_zone_changes (id, zone_id, serial, change_type, name, type, value, ttl, priority, weight, port)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL)
		`, uuid.New().String(), zoneID, serial, c.changeType, c.name, c.rtype, c.value, c.ttl); err != nil {
			return err
		}
	}
	return nil
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

// checkPrerequisites evaluates the RFC 2136 §3.2 prerequisite section, which is
// carried in the Answer section of the update message. Like the update section,
// prerequisites are classified by CLASS rather than by type:
//
//	CLASS ANY  + type ANY -> the name is in use
//	CLASS ANY  + type X   -> an RRset of type X exists at the name
//	CLASS NONE + type ANY -> the name is not in use
//	CLASS NONE + type X   -> no RRset of type X exists at the name
//
// The previous implementation dispatched on Rrtype (TypeANY/TypeNone), values
// that never appear in a real prerequisite RR: the "name must not exist" branch
// was unreachable and every other prerequisite was silently ignored, so clients
// relying on "test and set" semantics could clobber each other's records.
//
// It returns the RFC 2136 rcode to send (RcodeSuccess when satisfied) and uses
// tx exclusively, since the caller holds the only SQLite connection.
func (h *UpdateHandler) checkPrerequisites(tx *sql.Tx, zoneID string, msg *dns.Msg) int {
	for _, rr := range msg.Answer {
		hdr := rr.Header()
		name := hdr.Name
		rtype := dns.TypeToString[hdr.Rrtype]

		switch hdr.Class {
		case dns.ClassANY:
			if hdr.Rrtype == dns.TypeANY {
				inUse, err := nameInUse(tx, zoneID, name)
				if err != nil {
					slog.Error("dynamic_update: prerequisite query failed", "error", err)
					return dns.RcodeServerFailure
				}
				if !inUse {
					return dns.RcodeNameError // NXDOMAIN
				}
				continue
			}
			if rtype == "" {
				return dns.RcodeFormatError
			}
			// A non-empty rdata makes the prerequisite value-dependent:
			// that exact RR must be present.
			if value := rrValue(rr); value != "" {
				exists, err := rdataExists(tx, zoneID, name, rtype, value)
				if err != nil {
					slog.Error("dynamic_update: prerequisite query failed", "error", err)
					return dns.RcodeServerFailure
				}
				if !exists {
					return dns.RcodeNXRrset
				}
				continue
			}
			exists, err := rrsetExists(tx, zoneID, name, rtype)
			if err != nil {
				slog.Error("dynamic_update: prerequisite query failed", "error", err)
				return dns.RcodeServerFailure
			}
			if !exists {
				return dns.RcodeNXRrset
			}

		case dns.ClassNONE:
			if hdr.Rrtype == dns.TypeANY {
				inUse, err := nameInUse(tx, zoneID, name)
				if err != nil {
					slog.Error("dynamic_update: prerequisite query failed", "error", err)
					return dns.RcodeServerFailure
				}
				if inUse {
					return dns.RcodeYXDomain
				}
				continue
			}
			if rtype == "" {
				return dns.RcodeFormatError
			}
			if value := rrValue(rr); value != "" {
				exists, err := rdataExists(tx, zoneID, name, rtype, value)
				if err != nil {
					slog.Error("dynamic_update: prerequisite query failed", "error", err)
					return dns.RcodeServerFailure
				}
				if exists {
					return dns.RcodeYXRrset
				}
				continue
			}
			exists, err := rrsetExists(tx, zoneID, name, rtype)
			if err != nil {
				slog.Error("dynamic_update: prerequisite query failed", "error", err)
				return dns.RcodeServerFailure
			}
			if exists {
				return dns.RcodeYXRrset
			}

		default:
			// A prerequisite must be CLASS ANY or CLASS NONE; anything else
			// is malformed (RFC 2136 §3.2).
			return dns.RcodeFormatError
		}
	}
	return dns.RcodeSuccess
}

// nameInUse reports whether the name has at least one enabled record.
func nameInUse(tx *sql.Tx, zoneID, name string) (bool, error) {
	var count int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND enabled = 1",
		zoneID, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// rrsetExists reports whether an enabled RRset of the given type exists.
func rrsetExists(tx *sql.Tx, zoneID, name, rtype string) (bool, error) {
	var count int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND type = ? AND enabled = 1",
		zoneID, name, rtype).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// rdataExists reports whether the exact (name, type, value) record exists.
func rdataExists(tx *sql.Tx, zoneID, name, rtype, value string) (bool, error) {
	var count int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND type = ? AND value = ? AND enabled = 1",
		zoneID, name, rtype, value).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
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
// The entry is written through the shared data-plane writer, which is what
// gives it the before/after columns migration 024 added. The previous version
// wrote its own statement, and the columns it named did not exist, so every
// dynamic update was silently un-audited.
func (h *UpdateHandler) auditLog(zoneID, zoneName, keyName, before, after string, count int) {
	slog.Info("dynamic_update: applied updates",
		"zone_id", zoneID,
		"zone_name", zoneName,
		"count", count,
		"keys", keyName,
		"timestamp", time.Now().UTC(),
	)

	if _, err := auditlog.Append(h.db, auditlog.Entry{
		// The TSIG key name is the only identity an RFC 2136 client presents.
		// Recording "system" here would discard the one fact the entry has.
		UserID:       "system",
		Username:     keyName,
		Action:       auditlog.ActionDynamicUpdate,
		ResourceType: auditlog.ResourceZone,
		ResourceID:   zoneID,
		Detail:       fmt.Sprintf("Dynamic update on zone %s: %d changes applied", zoneName, count),
		OldValue:     before,
		NewValue:     after,
	}); err != nil {
		slog.Error("dynamic_update: failed to write audit log",
			"zone_id", zoneID, "error", err)
	}
}

// rrValue extracts the stored value from a dns.RR. It returns "" for the
// value-independent forms used by the prerequisite and delete-RRset sections
// (where the RDATA is empty), which callers rely on to tell the two forms
// apart; the nil guards keep a partially-populated RR from panicking.
func rrValue(rr dns.RR) string {
	switch v := rr.(type) {
	case *dns.A:
		if v.A == nil {
			return ""
		}
		return v.A.String()
	case *dns.AAAA:
		if v.AAAA == nil {
			return ""
		}
		return v.AAAA.String()
	case *dns.CNAME:
		return v.Target
	case *dns.DNAME:
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
