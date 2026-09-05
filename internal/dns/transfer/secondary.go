package transfer

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
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
	"github.com/miekg/dns"
)

// SecondarySync manages synchronization of secondary zones from primary servers.
type SecondarySync struct {
	db *sql.DB
}

// NewSecondarySync creates a new SecondarySync.
func NewSecondarySync(db *sql.DB) *SecondarySync {
	return &SecondarySync{db: db}
}

// transferTSIGConfig carries the TSIG credentials parsed from a zone's
// transfer_policy field. The policy string is formatted as:
//
//	<primary-host>[:<port>][|<tsig-key-name>|<tsig-secret>[|<tsig-algorithm>]]
//
// If the TSIG fields are missing, the secondary will refuse to sync
// (fail-closed) per the security review.
type transferTSIGConfig struct {
	primaryAddr string
	tsigName    string
	tsigSecret  string
	tsigAlgo    string
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
	cfg := &transferTSIGConfig{primaryAddr: strings.TrimSpace(parts[0])}
	if cfg.primaryAddr == "" {
		return nil, fmt.Errorf("transfer_policy missing primary address")
	}
	if _, _, err := net.SplitHostPort(cfg.primaryAddr); err != nil {
		cfg.primaryAddr = net.JoinHostPort(cfg.primaryAddr, "53")
	}

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

// syncAllSecondaryZones syncs all secondary zones that need refreshing.
func (s *SecondarySync) syncAllSecondaryZones() {
	rows, err := s.db.Query(`
		SELECT id, name, refresh FROM dns_zones WHERE type = 'secondary' AND enabled = 1
	`)
	if err != nil {
		slog.Error("secondary_sync: failed to query zones", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		var refresh int
		if err := rows.Scan(&id, &name, &refresh); err != nil {
			continue
		}

		// Use the per-zone refresh value (in seconds) to decide when to sync
		// next. Fall back to the SOA record's refresh when the column is
		// unset, and to a safe default of 1 hour if neither is available.
		interval := time.Duration(refresh) * time.Second
		if interval <= 0 {
			interval = s.soaRefreshSeconds(id)
			if interval <= 0 {
				interval = time.Hour
			}
		}

		// Check if refresh is needed based on updated_at and refresh interval.
		var updatedAt time.Time
		err := s.db.QueryRow("SELECT updated_at FROM dns_zones WHERE id = ?", id).Scan(&updatedAt)
		if err != nil {
			continue
		}

		if time.Since(updatedAt) < interval {
			continue
		}

		if err := s.SyncFromPrimary(id); err != nil {
			slog.Error("secondary_sync: failed to sync zone", "zone", name, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("secondary_sync: failed to iterate zones", "error", err)
	}
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
// TSIG authentication.
func (s *SecondarySync) axfrFromPrimary(zoneName string, cfg *transferTSIGConfig) ([]dns.RR, error) {
	t := new(dns.Transfer)
	m := new(dns.Msg)
	m.SetAxfr(zoneName)

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

// insertRR inserts a dns.RR into the database for a zone.
func (s *SecondarySync) insertRR(zoneID string, rr dns.RR, zoneName string) error {
	return s.insertRRTx(s.db, zoneID, rr, zoneName)
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

	_, err := exec.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
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
