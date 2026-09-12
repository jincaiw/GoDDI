package transfer

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/metrics"
	"github.com/miekg/dns"
)

// ZoneExpirySink is implemented by the in-memory zone store. It lets the
// secondary sweeper withhold authoritative answers for a zone whose SOA
// EXPIRE has elapsed (RFC 1035 §6.3) and resume them after a successful
// ZoneExpirySink is the slice of the zone store the expiry sweeper needs:
// marking a zone expired withholds it from authoritative answers, and clearing
// the mark resumes them.
type ZoneExpirySink interface {
	MarkZoneExpired(zoneName string)
	ClearZoneExpired(zoneName string)
}

// ZoneReloader is the slice of the zone store an inbound transfer needs.
//
// A transfer replaces a zone's records in the database, and queries are
// answered from memory, so a transfer that is not followed by a reload leaves
// the new data on disk and the old zone in service: the secondary would report
// a new serial in its SOA and serve the previous zone. The service path moved
// to a store this process owns, so this is now the only thing that can make a
// transfer visible.
type ZoneReloader interface {
	ReloadNow()
}

// SecondarySync manages synchronization of secondary zones from primary servers.
type SecondarySync struct {
	db *sql.DB
	// expiry receives expired/refreshed notifications. Optional: when nil the
	// sweeper still records state in the database but cannot gate answering.
	expiry ZoneExpirySink
	// reloader republishes the in-memory zone data after a transfer replaced a
	// zone's records. Optional: when nil the transfer still commits, and the
	// zone stays as it was until something else reloads.
	reloader ZoneReloader
	// now is injectable for tests.
	now func() time.Time
}

// NewSecondarySync creates a new SecondarySync.
func NewSecondarySync(db *sql.DB) *SecondarySync {
	return &SecondarySync{db: db, now: time.Now}
}

// SetZoneStore wires the zone store used to withhold expired zones. Callers
// that never serve authoritative answers (e.g. the API handler performing a
// manual refresh) may leave it unset.
func (s *SecondarySync) SetZoneStore(sink ZoneExpirySink) {
	s.expiry = sink
}

// SetZoneReloader wires the zone store that has to be republished after a
// transfer. Callers that never serve authoritative answers may leave it unset.
func (s *SecondarySync) SetZoneReloader(r ZoneReloader) {
	s.reloader = r
}

func (s *SecondarySync) reloadZones() {
	if s.reloader != nil {
		s.reloader.ReloadNow()
	}
}

func (s *SecondarySync) nowFn() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// transferTSIGConfig carries the TSIG credentials parsed from a zone's
// transfer_policy field. The policy string is formatted as:
//
//	<primary-host>[:<port>][|<tsig-key-name>|<tsig-secret>[|<tsig-algorithm>]]
//
// The primary address may carry a scheme prefix to select the transport:
//
//	tls://host[:port]           XFR-over-TLS (RFC 9103), certificate verified
//	tls-insecure://host[:port]  XFR-over-TLS without certificate verification
//	                              (lab / self-signed primaries only)
//
// The default port is 53 (plain TCP) or 853 (TLS). If the TSIG fields are
// missing, the secondary will refuse to sync (fail-closed) per the
// security review.
type transferTSIGConfig struct {
	primaryAddr string
	useTLS      bool
	tlsInsecure bool
	tsigName    string
	tsigSecret  string
	tsigAlgo    string
}

// parsePrimaryAddress strips a transport scheme prefix from the primary
// address and reports the selected transport.
func parsePrimaryAddress(raw string) (addr string, useTLS, insecure bool, err error) {
	normalized := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(normalized, "tls-insecure://"):
		addr = raw[len("tls-insecure://"):]
		useTLS, insecure = true, true
	case strings.HasPrefix(normalized, "tls://"):
		addr = raw[len("tls://"):]
		useTLS = true
	default:
		addr = raw
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", false, false, fmt.Errorf("empty primary address")
	}
	if _, _, e := net.SplitHostPort(addr); e != nil {
		defPort := "53"
		if useTLS {
			defPort = "853"
		}
		addr = net.JoinHostPort(addr, defPort)
	}
	return addr, useTLS, insecure, nil
}

// parseTransferPolicy extracts the primary address and (optional) TSIG
// credentials from a zone's transfer_policy field. A policy without TSIG
// fields is considered insecure and yields a non-nil error so that
// SyncFromPrimary can fail closed.
func parseTransferPolicy(policy string) (*transferTSIGConfig, error) {
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return nil, fmt.Errorf("no primary server configured for zone")
	}

	parts := strings.Split(policy, "|")
	addr, useTLS, tlsInsecure, err := parsePrimaryAddress(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, err
	}
	cfg := &transferTSIGConfig{primaryAddr: addr, useTLS: useTLS, tlsInsecure: tlsInsecure}

	if len(parts) >= 3 {
		cfg.tsigName = strings.TrimSpace(parts[1])
		cfg.tsigSecret = strings.TrimSpace(parts[2])
	}
	if len(parts) >= 4 {
		cfg.tsigAlgo = strings.TrimSpace(parts[3])
	}
	// Default algorithm to SHA-256 when TSIG is configured.
	if cfg.tsigName != "" && cfg.tsigAlgo == "" {
		cfg.tsigAlgo = string(TSIGHMACSHA256)
	}

	// Fail-closed: refuse to sync without TSIG credentials. Unauthenticated
	// AXFR leaks the entire zone to anyone who can reach the primary.
	if cfg.tsigName == "" || cfg.tsigSecret == "" {
		return nil, fmt.Errorf("transfer_policy must include TSIG credentials (key name and secret)")
	}

	return cfg, nil
}

// SyncFromPrimary pulls zone data from the primary server for a secondary zone.
func (s *SecondarySync) SyncFromPrimary(zoneID string) error {
	if zoneID == "" {
		return fmt.Errorf("zone id is required")
	}

	// Get zone info.
	var zoneName, transferPolicy string
	var currentSerial uint32
	err := s.db.QueryRow(`
		SELECT name, COALESCE(transfer_policy, ''), serial FROM dns_zones WHERE id = ? AND type = 'secondary'
	`, zoneID).Scan(&zoneName, &transferPolicy, &currentSerial)
	if err == sql.ErrNoRows {
		return fmt.Errorf("secondary zone not found: %s", zoneID)
	}
	if err != nil {
		return fmt.Errorf("querying zone: %w", err)
	}

	// Parse primary server and TSIG credentials from transfer policy.
	cfg, err := parseTransferPolicy(transferPolicy)
	if err != nil {
		return fmt.Errorf("invalid transfer_policy for zone %s: %w", zoneName, err)
	}

	// Perform AXFR from primary using TSIG.
	records, err := s.axfrFromPrimary(zoneName, cfg)
	if err != nil {
		return fmt.Errorf("AXFR from primary: %w", err)
	}

	// RFC 8976: verify the ZONEMD digest when the zone publishes one. A
	// digest mismatch aborts the transfer so corrupted zone data never
	// replaces the local copy.
	if verified, zerr := VerifyZONEMD(zoneName, records); !verified {
		return fmt.Errorf("ZONEMD verification failed for zone %s: %w", zoneName, zerr)
	} else if zerr != nil {
		slog.Warn("secondary_sync: ZONEMD verification skipped", "zone", zoneName, "reason", zerr)
	}

	// Extract serial from SOA record in the AXFR response.
	var primarySerial uint32
	for _, rr := range records {
		if soa, ok := rr.(*dns.SOA); ok {
			primarySerial = soa.Serial
			break
		}
	}
	if primarySerial == 0 {
		primarySerial = currentSerial + 1
	}

	// Use a transaction: delete old records and insert new ones atomically.
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing records for this zone.
	if _, err := tx.Exec("DELETE FROM dns_records WHERE zone_id = ?", zoneID); err != nil {
		return fmt.Errorf("clearing existing records: %w", err)
	}

	// Insert new records.
	for _, rr := range records {
		if err := s.insertRRTx(tx, zoneID, rr, zoneName); err != nil {
			slog.Warn("secondary_sync: failed to insert record", "error", err)
			continue
		}
	}

	// Update zone serial to the primary's serial.
	if _, err := tx.Exec("UPDATE dns_zones SET serial = ?, updated_at = datetime('now') WHERE id = ?", primarySerial, zoneID); err != nil {
		return fmt.Errorf("updating serial: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Record the successful transfer. This is the clock refresh/expire are
	// measured against, and it must be written only after the new zone data
	// is committed — marking it earlier would claim freshness for data that
	// never landed.
	if _, err := s.db.Exec(`
		UPDATE dns_zones
		SET last_sync_at = datetime('now'),
		    last_sync_attempt_at = datetime('now'),
		    sync_failure_count = 0
		WHERE id = ?
	`, zoneID); err != nil {
		// The zone data is already committed; failing to stamp the clock must
		// not fail the transfer. The next sweep re-stamps it.
		slog.Warn("secondary_sync: failed to record sync timestamp", "zone", zoneName, "error", err)
	}

	// A refreshed zone is no longer expired: resume authoritative answers.
	if s.expiry != nil {
		s.expiry.ClearZoneExpired(zoneName)
	}

	// The zone's records and its serial were both replaced. Queries are
	// answered from memory, so without this the secondary would advertise the
	// primary's serial in its SOA while still serving the zone it replaced.
	s.reloadZones()

	slog.Info("secondary_sync: zone synced from primary", "zone", zoneName, "records", len(records), "serial", primarySerial)
	return nil
}

// HandleNotify handles an incoming NOTIFY message for a secondary zone.
func (s *SecondarySync) HandleNotify(zoneName string) error {
	if zoneName == "" {
		return fmt.Errorf("zone name is required")
	}

	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}

	// Find the secondary zone.
	var zoneID string
	err := s.db.QueryRow("SELECT id FROM dns_zones WHERE name = ? AND type = 'secondary' AND enabled = 1", zoneName).Scan(&zoneID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("secondary zone not found: %s", zoneName)
	}
	if err != nil {
		return fmt.Errorf("querying zone: %w", err)
	}

	// Trigger sync.
	return s.SyncFromPrimary(zoneID)
}

// StartPeriodicSync starts periodic sync for all secondary zones based on SOA
// refresh interval. A short loop with per-zone sleep timers is used so that
// each zone can honor its own refresh value from dns_zones.refresh (or its
// current SOA record) without all zones being forced onto a global cadence.
func (s *SecondarySync) StartPeriodicSync(stopCh <-chan struct{}) {
	// Re-evaluate schedules every minute; individual zones still sleep until
	// their own refresh interval elapses since the last successful sync.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.syncAllSecondaryZones()
		}
	}
}

// secondaryZone is a materialized secondary zone row plus the derived
// intervals the sweeper needs.
type secondaryZone struct {
	id         string
	name       string
	refresh    time.Duration
	expire     time.Duration
	lastSyncAt sql.NullString
}

// ZoneRefreshHealth is one secondary zone's refresh state.
type ZoneRefreshHealth struct {
	// Zone is the zone name.
	Zone string
	// Failures is the consecutive failed refresh count; zero after a success.
	Failures int
	// HasSynced is false when the zone has never completed a transfer.
	HasSynced bool
	// LastSync is the completion time of the last successful transfer.
	LastSync time.Time
}

// SecondaryZoneHealth reports the refresh state of every enabled secondary
// zone.
//
// Read from the zone table rather than accumulated alongside the sweep. The
// sweep is one goroutine that a restart replaces, and a failure count kept only
// in memory would reset to zero at exactly the moment someone restarted the
// service to clear the problem it was counting.
//
// The rows are materialized before this returns. The SQLite pool is capped at
// one connection (internal/database), so a cursor left open while the caller
// does anything else blocks forever on the connection this query holds.
func SecondaryZoneHealth(db *sql.DB) ([]ZoneRefreshHealth, error) {
	rows, err := db.Query(`
		SELECT name, sync_failure_count, CAST(last_sync_at AS TEXT)
		FROM dns_zones
		WHERE type = 'secondary' AND enabled = 1
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("reading secondary zone health: %w", err)
	}
	defer rows.Close()

	var out []ZoneRefreshHealth
	for rows.Next() {
		var name string
		var failures int
		var lastSync sql.NullString
		if err := rows.Scan(&name, &failures, &lastSync); err != nil {
			return nil, fmt.Errorf("scanning secondary zone health: %w", err)
		}

		h := ZoneRefreshHealth{Zone: name, Failures: failures}
		if lastSync.Valid && strings.TrimSpace(lastSync.String) != "" {
			at, err := parseSyncTime(lastSync.String)
			if err != nil {
				// The failure count for this zone is still true and is still
				// reported. The timestamp is not turned into the epoch, which
				// would read as "synced in 1970" -- a stale-zone alarm about a
				// row whose problem is the row.
				slog.Warn("secondary_sync: unreadable last_sync_at",
					"zone", name, "value", lastSync.String)
			} else {
				h.LastSync, h.HasSynced = at, true
			}
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading secondary zone health: %w", err)
	}
	return out, nil
}

// parseSyncTime reads the layouts last_sync_at holds: SQLite's datetime('now')
// form, and RFC3339 for anything written by a Go caller. A value that is
// neither is an error, not a zero time.
func parseSyncTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if at, err := time.Parse(layout, trimmed); err == nil {
			return at.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised sync timestamp %q", raw)
}

// listSecondaryZones reads the secondary zones into memory and closes the
// cursor before any further query runs.
//
// The rows MUST be materialized rather than processed inside the loop: the
// SQLite pool is capped at a single connection
// (internal/database/database.go — SetMaxOpenConns(1)), so issuing any nested
// query while the cursor is open blocks forever waiting for the very
// connection the cursor holds. That is a hang, not an error the caller can
// retry, and it is why the per-zone lookups below are done after this
// function returns.
func (s *SecondarySync) listSecondaryZones() ([]secondaryZone, error) {
	rows, err := s.db.Query(`
		SELECT id, name, COALESCE(refresh, 0), COALESCE(expire, 0), last_sync_at
		FROM dns_zones WHERE type = 'secondary' AND enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []secondaryZone
	for rows.Next() {
		var z secondaryZone
		var refresh, expire int
		if err := rows.Scan(&z.id, &z.name, &refresh, &expire, &z.lastSyncAt); err != nil {
			continue
		}
		z.refresh = time.Duration(refresh) * time.Second
		z.expire = time.Duration(expire) * time.Second
		zones = append(zones, z)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return zones, nil
}

// syncAllSecondaryZones syncs all secondary zones that need refreshing and
// parks any whose SOA EXPIRE has elapsed.
func (s *SecondarySync) syncAllSecondaryZones() {
	zones, err := s.listSecondaryZones()
	if err != nil {
		slog.Error("secondary_sync: failed to list secondary zones", "error", err)
		return
	}

	for _, z := range zones {
		// Expiry is evaluated before the refresh check so an unreachable
		// primary parks the zone on the sweep where EXPIRE elapses, instead
		// of continuing to serve stale data for another refresh interval.
		s.enforceExpiry(z)

		interval := z.refresh
		if interval <= 0 {
			// Fall back to the SOA record's refresh, then to a safe default.
			interval = s.soaRefreshSeconds(z.id)
			if interval <= 0 {
				interval = time.Hour
			}
		}

		last, ok := parseSQLiteTime(z.lastSyncAt)
		if !ok {
			// Never successfully synced: attempt immediately rather than
			// waiting a full interval before the first pull.
			if err := s.SyncFromPrimary(z.id); err != nil {
				s.recordFailure(z, err)
			}
			continue
		}

		if s.nowFn().Sub(last) < interval {
			continue
		}

		if err := s.SyncFromPrimary(z.id); err != nil {
			s.recordFailure(z, err)
		}
	}
}

// recordFailure logs a failed refresh and advances the retry bookkeeping.
func (s *SecondarySync) recordFailure(z secondaryZone, syncErr error) {
	slog.Error("secondary_sync: failed to sync zone", "zone", z.name, "error", syncErr)
	if _, err := s.db.Exec(`
		UPDATE dns_zones
		SET last_sync_attempt_at = datetime('now'),
		    sync_failure_count = sync_failure_count + 1
		WHERE id = ?
	`, z.id); err != nil {
		slog.Warn("secondary_sync: failed to record sync failure", "zone", z.name, "error", err)
		metrics.RecordDBError()
	}
}

// enforceExpiry parks a secondary zone whose last successful transfer is older
// than the zone's SOA EXPIRE.
//
// RFC 1035 §6.3: once the expire interval elapses without a refresh, the
// secondary must stop using its local copy. The copy is not merely stale — the
// primary may have reassigned names it still holds, so answering from it can
// return an address that now belongs to a different host.
func (s *SecondarySync) enforceExpiry(z secondaryZone) {
	if z.expire <= 0 {
		return
	}

	last, ok := parseSQLiteTime(z.lastSyncAt)
	if !ok {
		// No successful transfer on record: there is no data whose freshness
		// can be vouched for, so the zone is withheld from the start.
		s.markExpired(z.name)
		return
	}

	if s.nowFn().Sub(last) > z.expire {
		slog.Warn("secondary_sync: zone exceeded SOA EXPIRE",
			"zone", z.name, "last_sync_at", last.Format(time.RFC3339), "expire", z.expire)
		s.markExpired(z.name)
	}
}

func (s *SecondarySync) markExpired(zoneName string) {
	if s.expiry == nil {
		return
	}
	s.expiry.MarkZoneExpired(zoneName)
}

// sqliteTimeLayout describes one accepted timestamp encoding. SQLite's
// datetime('now') writes UTC as "2006-01-02 15:04:05" with no zone suffix,
// while some drivers emit RFC 3339 instead; both must be accepted.
type sqliteTimeLayout struct {
	layout  string
	hasZone bool
}

var sqliteTimeLayouts = []sqliteTimeLayout{
	{layout: "2006-01-02 15:04:05", hasZone: false},
	{layout: "2006-01-02 15:04:05.999999999", hasZone: false},
	{layout: "2006-01-02T15:04:05", hasZone: false},
	{layout: "2006-01-02T15:04:05.999999999", hasZone: false},
	{layout: "2006-01-02T15:04:05Z07:00", hasZone: true},
	{layout: "2006-01-02T15:04:05.999999999Z07:00", hasZone: true},
	{layout: "2006-01-02 15:04:05Z07:00", hasZone: true},
}

// parseSQLiteTime parses a timestamp column into a UTC time. Zone-less values
// are interpreted as UTC to match SQLite's datetime('now'). It reports false
// for NULL or unrecognized input so callers can distinguish "never happened"
// from a real timestamp.
func parseSQLiteTime(v sql.NullString) (time.Time, bool) {
	if !v.Valid {
		return time.Time{}, false
	}
	raw := strings.TrimSpace(v.String)
	if raw == "" {
		return time.Time{}, false
	}
	for _, candidate := range sqliteTimeLayouts {
		parsed, err := time.Parse(candidate.layout, raw)
		if err != nil {
			continue
		}
		if !candidate.hasZone {
			// Zone-less text from datetime('now') is UTC. Parsing it as UTC
			// yields the correct instant regardless of the local zone, so the
			// arithmetic below stays correct on a non-UTC host.
			parsed = parsed.UTC()
		}
		return parsed.UTC(), true
	}
	slog.Warn("secondary_sync: unrecognized timestamp format", "value", raw)
	return time.Time{}, false
}

// soaRefreshSeconds returns the refresh value of the zone's SOA record (in
// seconds) when available, or 0 when no SOA is stored. It is used as a
// fallback when the dns_zones.refresh column is zero.
func (s *SecondarySync) soaRefreshSeconds(zoneID string) time.Duration {
	var soa string
	err := s.db.QueryRow(`
		SELECT value FROM dns_records
		WHERE zone_id = ? AND type = 'SOA' AND enabled = 1
		LIMIT 1
	`, zoneID).Scan(&soa)
	if err != nil || soa == "" {
		return 0
	}
	// SOA value is stored in the canonical "ns mbox serial refresh retry expire minimum" format.
	parts := strings.Fields(soa)
	if len(parts) < 7 {
		return 0
	}
	var refresh uint64
	for i := 0; i < len(parts[3]); i++ {
		c := parts[3][i]
		if c < '0' || c > '9' {
			return 0
		}
		refresh = refresh*10 + uint64(c-'0')
	}
	return time.Duration(refresh) * time.Second
}

// axfrFromPrimary performs an AXFR transfer from the primary server using
// TSIG authentication. XFR-over-TLS (RFC 9103) is used when the policy's
// primary address carries a tls:// or tls-insecure:// scheme.
func (s *SecondarySync) axfrFromPrimary(zoneName string, cfg *transferTSIGConfig) ([]dns.RR, error) {
	t := new(dns.Transfer)
	m := new(dns.Msg)
	m.SetAxfr(zoneName)

	if cfg.useTLS {
		t.TLS = &tls.Config{
			ServerName:         hostOnly(cfg.primaryAddr),
			InsecureSkipVerify: cfg.tlsInsecure, //nolint:gosec // explicit opt-in via tls-insecure:// scheme
			MinVersion:         tls.VersionTLS12,
		}
	}

	// Attach TSIG credentials to the AXFR request. miekg/dns looks the secret
	// up in TsigSecret by zone name (lowercase, FQDN). The key name from the
	// TSIG RR on the wire is matched separately; the algorithm is taken
	// from the TSIG RR or from the TsigProvider.
	t.TsigSecret = map[string]string{
		strings.ToLower(dns.Fqdn(zoneName)): cfg.tsigSecret,
	}
	t.TsigProvider = tsigProvider{
		key:    cfg.tsigName,
		secret: cfg.tsigSecret,
		algo:   defaultAlgoMap(cfg.tsigAlgo),
	}

	c, err := t.In(m, cfg.primaryAddr)
	if err != nil {
		return nil, fmt.Errorf("initiating AXFR: %w", err)
	}

	var records []dns.RR
	for env := range c {
		if env.Error != nil {
			return records, env.Error
		}
		records = append(records, env.RR...)
	}

	return records, nil
}

// hostOnly strips the port from a host:port address, used as the TLS
// ServerName for XFR-over-TLS certificate verification.
func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil && host != "" {
		return host
	}
	return addr
}

// defaultAlgoMap returns the canonical algorithm name to use for the given
// configured algorithm string. The "algo" parameter accepts either the local
// TSIGAlgorithm constants or the canonical miekg names.
func defaultAlgoMap(algo string) string {
	switch algo {
	case string(TSIGHMACSHA512), dns.HmacSHA512:
		return dns.HmacSHA512
	default:
		return dns.HmacSHA256
	}
}

// tsigProvider implements dns.TsigProvider with a fixed algorithm. It is used
// to attach TSIG credentials to outbound zone transfer requests.
type tsigProvider struct {
	key    string
	secret string
	algo   string
}

func (p tsigProvider) Generate(msg []byte, t *dns.TSIG) ([]byte, error) {
	t.Algorithm = p.algo
	return computeTSIGMAC(msg, t, p.secret, p.algo)
}

func (p tsigProvider) Verify(msg []byte, t *dns.TSIG) error {
	t.Algorithm = p.algo
	expected, err := computeTSIGMAC(msg, t, p.secret, p.algo)
	if err != nil {
		return err
	}
	got, err := hex.DecodeString(t.MAC)
	if err != nil {
		return err
	}
	if !hmacEqual(expected, got) {
		return dns.ErrSig
	}
	return nil
}

// computeTSIGMAC computes the RFC 8945 TSIG MAC for the given TSIG RR and
// message bytes. The implementation follows the same structure as miekg/dns
// internal tsigBuffer, using the configured algorithm. It returns the raw
// MAC bytes (before hex encoding).
func computeTSIGMAC(msg []byte, t *dns.TSIG, secret, algo string) ([]byte, error) {
	rawSecret, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, err
	}
	macFunc, err := tsigHash(algo)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(macFunc, rawSecret)
	mac.Write(msg)
	return mac.Sum(nil), nil
}

func tsigHash(algo string) (func() hash.Hash, error) {
	switch algo {
	case dns.HmacSHA256:
		return sha256.New, nil
	case dns.HmacSHA512:
		return sha512.New, nil
	default:
		return nil, dns.ErrKeyAlg
	}
}

func hmacEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}

// insertRRTx inserts a dns.RR into the database using the given executor (tx or db).
func (s *SecondarySync) insertRRTx(exec executor, zoneID string, rr dns.RR, zoneName string) error {
	hdr := rr.Header()

	// Skip SOA records (managed by zone).
	if hdr.Rrtype == dns.TypeSOA {
		return nil
	}

	name := hdr.Name
	// Make relative to zone if possible.
	if strings.HasSuffix(strings.ToLower(name), strings.ToLower(zoneName)) {
		name = name[:len(name)-len(zoneName)]
		name = strings.TrimSuffix(name, ".")
		if name == "" {
			name = zoneName
		}
	}

	rtype := dns.TypeToString[hdr.Rrtype]
	value := rrValue(rr)

	// Extract the per-type numeric fields stored in dedicated columns; the
	// value column alone cannot represent MX preference, SRV priority /
	// weight / port or CAA flag, and dropping them silently corrupts the
	// zone after a transfer.
	var priority, weight, port, caaFlag interface{}
	switch v := rr.(type) {
	case *dns.MX:
		priority = int(v.Preference)
	case *dns.SRV:
		priority = int(v.Priority)
		weight = int(v.Weight)
		port = int(v.Port)
	case *dns.CAA:
		caaFlag = int(v.Flag)
	}

	// The records of a transferred zone are authored here, not at the control
	// plane: they are the primary's data, adopted by this process because it is
	// the one that serves them. authored_locally is what keeps a configuration
	// sync from withdrawing them, and what carries them up for the console.
	_, err := exec.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, enabled, authored_locally)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1)
	`, uuid.New().String(), zoneID, name, rtype, value, int(hdr.Ttl), priority, weight, port, caaFlag)
	return err
}

// executor is a minimal interface satisfied by both *sql.DB and *sql.Tx.
type executor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// rrValue extracts the value string from a dns.RR.
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
	case *dns.CAA:
		return v.Value
	default:
		return ""
	}
}
