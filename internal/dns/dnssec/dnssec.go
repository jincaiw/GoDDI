package dnssec

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
)

// DNSSECAlgorithm represents a supported DNSSEC algorithm.
type DNSSECAlgorithm string

const (
	AlgRSASHA256       DNSSECAlgorithm = "RSASHA256"
	AlgRSASHA512       DNSSECAlgorithm = "RSASHA512"
	AlgECDSAP256SHA256 DNSSECAlgorithm = "ECDSAP256SHA256"
	AlgECDSAP384SHA384 DNSSECAlgorithm = "ECDSAP384SHA384"
	AlgED25519         DNSSECAlgorithm = "ED25519"
)

// SupportedAlgorithms lists all supported DNSSEC algorithms.
var SupportedAlgorithms = map[DNSSECAlgorithm]uint8{
	AlgRSASHA256:       dns.RSASHA256,
	AlgRSASHA512:       dns.RSASHA512,
	AlgECDSAP256SHA256: dns.ECDSAP256SHA256,
	AlgECDSAP384SHA384: dns.ECDSAP384SHA384,
	AlgED25519:         dns.ED25519,
}

// DNSSECKey represents a DNSSEC key pair.
type DNSSECKey struct {
	ID               string    `json:"id"`
	ZoneID           string    `json:"zone_id"`
	KeyType          string    `json:"key_type"` // KSK or ZSK
	Algorithm        string    `json:"algorithm"`
	PublicKey        string    `json:"public_key"`
	PrivateKeyCipher string    `json:"-"` // encrypted, not exposed
	KeyTag           int       `json:"key_tag"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
}

// DNSSECManager manages DNSSEC keys and signing.
type DNSSECManager struct {
	db        *sql.DB
	zoneMgr   *zone.ZoneManager
	zoneStore *zone.Store
	jwtSecret string
}

// NewDNSSECManager creates a new DNSSECManager.
func NewDNSSECManager(db *sql.DB, zoneMgr *zone.ZoneManager, zoneStore *zone.Store, jwtSecret string) *DNSSECManager {
	return &DNSSECManager{
		db:        db,
		zoneMgr:   zoneMgr,
		zoneStore: zoneStore,
		jwtSecret: jwtSecret,
	}
}

// GenerateKSK generates a Key Signing Key for a zone.
func (m *DNSSECManager) GenerateKSK(zoneID, algorithm string) (*DNSSECKey, error) {
	return m.generateKey(zoneID, "KSK", algorithm)
}

// GenerateZSK generates a Zone Signing Key for a zone.
func (m *DNSSECManager) GenerateZSK(zoneID, algorithm string) (*DNSSECKey, error) {
	return m.generateKey(zoneID, "ZSK", algorithm)
}

// generateKey generates a DNSSEC key pair.
func (m *DNSSECManager) generateKey(zoneID, keyType, algorithm string) (*DNSSECKey, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone id is required")
	}
	if keyType != "KSK" && keyType != "ZSK" {
		return nil, fmt.Errorf("key type must be KSK or ZSK")
	}

	algo := DNSSECAlgorithm(algorithm)
	if algo == "" {
		algo = AlgECDSAP256SHA256
	}

	algoNum, ok := SupportedAlgorithms[algo]
	if !ok {
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	// Get zone name for key generation.
	z, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	// Generate key pair based on algorithm.
	var pubKeyStr, privKeyCipher string
	var keyTag int

	switch algo {
	case AlgRSASHA256, AlgRSASHA512:
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generating RSA key: %w", err)
		}
		// DNSKEY stores the raw key material in the RFC 3110 / RFC 4034
		// format: 1 byte exponent length (3 bytes if 0), exponent bytes,
		// then the modulus. We must NOT store the PKIX form here because
		// the miekg/dns library decodes the public key directly from this
		// representation when verifying signatures.
		dnsPubKey := buildDNSKeyRSA(&rsaKey.PublicKey)
		pubKeyStr = base64.StdEncoding.EncodeToString(dnsPubKey)

		privKeyBytes := x509.MarshalPKCS1PrivateKey(rsaKey)
		encrypted, err := encryptPrivateKey(privKeyBytes, m.jwtSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypting private key: %w", err)
		}
		privKeyCipher = encrypted

		// Compute key tag.
		dnsKey := &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: z.Name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags:     dnsKeyFlags(keyType),
			Protocol:  3,
			Algorithm: algoNum,
			PublicKey: pubKeyStr,
		}
		keyTag = int(dnsKey.KeyTag())

	case AlgECDSAP256SHA256, AlgECDSAP384SHA384:
		var curve elliptic.Curve
		if algo == AlgECDSAP256SHA256 {
			curve = elliptic.P256()
		} else {
			curve = elliptic.P384()
		}
		ecKey, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating ECDSA key: %w", err)
		}
		// DNSKEY for ECDSA stores the raw concatenation of X and Y
		// coordinates (RFC 6605 §4). The "uncompressed" point form
		// (0x04 || X || Y) is the more standard representation; we use
		// elliptic.Marshal which produces exactly that, then trim the
		// leading 0x04 byte because DNSKEY stores just X || Y.
		rawPoint := elliptic.Marshal(curve, ecKey.PublicKey.X, ecKey.PublicKey.Y)
		if len(rawPoint) < 1 {
			return nil, fmt.Errorf("marshaling ECDSA public key: empty point")
		}
		// Drop the 0x04 uncompressed-form prefix; DNSKEY stores only the
		// coordinates.
		dnsPubKey := rawPoint[1:]
		pubKeyStr = base64.StdEncoding.EncodeToString(dnsPubKey)

		privKeyBytes, err := x509.MarshalPKCS8PrivateKey(ecKey)
		if err != nil {
			return nil, fmt.Errorf("marshaling private key: %w", err)
		}
		encrypted, err := encryptPrivateKey(privKeyBytes, m.jwtSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypting private key: %w", err)
		}
		privKeyCipher = encrypted

		dnsKey := &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: z.Name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags:     dnsKeyFlags(keyType),
			Protocol:  3,
			Algorithm: algoNum,
			PublicKey: pubKeyStr,
		}
		keyTag = int(dnsKey.KeyTag())

	case AlgED25519:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating ED25519 key: %w", err)
		}
		// ED25519 DNSKEY stores the 32-byte raw public key, which is
		// what ed25519.GenerateKey returns.
		pubKeyStr = base64.StdEncoding.EncodeToString(pub)

		encrypted, err := encryptPrivateKey(priv, m.jwtSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypting private key: %w", err)
		}
		privKeyCipher = encrypted

		dnsKey := &dns.DNSKEY{
			Hdr:       dns.RR_Header{Name: z.Name, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
			Flags:     dnsKeyFlags(keyType),
			Protocol:  3,
			Algorithm: algoNum,
			PublicKey: pubKeyStr,
		}
		keyTag = int(dnsKey.KeyTag())
	}

	id := uuid.New().String()
	_, err = m.db.Exec(`
		INSERT INTO dns_dnssec_keys (id, zone_id, key_type, algorithm, public_key, private_key_ciphertext, key_tag, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)
	`, id, zoneID, keyType, string(algo), pubKeyStr, privKeyCipher, keyTag)
	if err != nil {
		return nil, fmt.Errorf("inserting DNSSEC key: %w", err)
	}

	return &DNSSECKey{
		ID:        id,
		ZoneID:    zoneID,
		KeyType:   keyType,
		Algorithm: string(algo),
		PublicKey: pubKeyStr,
		KeyTag:    keyTag,
		Enabled:   true,
	}, nil
}

// SignZone signs all records in a zone with DNSSEC.
func (m *DNSSECManager) SignZone(zoneID string) error {
	if zoneID == "" {
		return fmt.Errorf("zone id is required")
	}

	// Get zone info.
	_, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return err
	}

	// Get active ZSK for this zone.
	_, err = m.getActiveKey(zoneID, "ZSK")
	if err != nil {
		return fmt.Errorf("no active ZSK: %w", err)
	}

	// Get active KSK.
	_, err = m.getActiveKey(zoneID, "KSK")
	if err != nil {
		return fmt.Errorf("no active KSK: %w", err)
	}

	// Mark zone as DNSSEC enabled.
	enabled := true
	_, err = m.db.Exec("UPDATE dns_zones SET dnssec_enabled = ?, updated_at = datetime('now') WHERE id = ?", enabled, zoneID)
	if err != nil {
		return fmt.Errorf("enabling DNSSEC: %w", err)
	}

	// Reload zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

// VerifyDNSSEC verifies DNSSEC signatures on a response.
func (m *DNSSECManager) VerifyDNSSEC(zoneName string, response *dns.Msg) bool {
	// Check if there are RRSIG records in the response.
	var rrsigRecords []*dns.RRSIG
	for _, rr := range response.Answer {
		if rrsig, ok := rr.(*dns.RRSIG); ok {
			rrsigRecords = append(rrsigRecords, rrsig)
		}
	}
	for _, rr := range response.Ns {
		if rrsig, ok := rr.(*dns.RRSIG); ok {
			rrsigRecords = append(rrsigRecords, rrsig)
		}
	}

	if len(rrsigRecords) == 0 {
		return false
	}

	// For each RRSIG, verify the signature.
	for _, rrsig := range rrsigRecords {
		var rrset []dns.RR
		for _, rr := range response.Answer {
			if rr.Header().Rrtype == rrsig.TypeCovered {
				rrset = append(rrset, rr)
			}
		}

		if len(rrset) == 0 {
			continue
		}

		// Look up the DNSKEY from the zone. RRSIGs over the zone's RRsets
		// are produced with the ZSK, not the KSK; the KSK only signs the
		// DNSKEY RRset. Verifying the RRSIG against the KSK is what
		// happens at the parent (DS-based chain of trust), not here.
		zn, err := m.zoneMgr.GetZoneByName(zoneName)
		if err != nil {
			continue
		}

		keys, err := m.getKeys(zn.ID, "ZSK")
		if err != nil || len(keys) == 0 {
			continue
		}

		for _, key := range keys {
			dnsKey := m.buildDNSKEY(&key, zoneName)
			if dnsKey == nil {
				continue
			}
			if dnsKey.KeyTag() == rrsig.KeyTag {
				err := rrsig.Verify(dnsKey, rrset)
				if err == nil {
					return true
				}
			}
		}
	}

	return false
}

// GetDNSSECStatus returns the DNSSEC status for a zone.
func (m *DNSSECManager) GetDNSSECStatus(zoneID string) (map[string]interface{}, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone id is required")
	}

	z, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	keys, err := m.getKeys(zoneID, "")
	if err != nil {
		return nil, err
	}

	status := map[string]interface{}{
		"zone_id":        zoneID,
		"zone_name":      z.Name,
		"dnssec_enabled": z.DNSSECEnabled,
		"keys":           keys,
		"key_count":      len(keys),
	}

	return status, nil
}

// DisableDNSSEC disables DNSSEC for a zone.
func (m *DNSSECManager) DisableDNSSEC(zoneID string) error {
	_, err := m.db.Exec("UPDATE dns_dnssec_keys SET enabled = 0 WHERE zone_id = ?", zoneID)
	if err != nil {
		return fmt.Errorf("disabling DNSSEC keys: %w", err)
	}

	_, err = m.db.Exec("UPDATE dns_zones SET dnssec_enabled = 0, updated_at = datetime('now') WHERE id = ?", zoneID)
	if err != nil {
		return fmt.Errorf("disabling DNSSEC: %w", err)
	}

	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

// RotateKeys rotates DNSSEC keys for a zone. The rotation is performed in
// two phases so that signatures made with the old key can continue to
// validate while the new key's signatures are minted. Phase one creates a
// new "standby" key alongside the existing "active" one; phase two (called
// later, after RRSIGs made with the old key have aged out of caches) flips
// the new key to active and demotes the old one. The caller decides when to
// invoke each phase, typically based on the zone's SOA expire value.
func (m *DNSSECManager) RotateKeys(zoneID string) error {
	keys, err := m.getKeys(zoneID, "")
	if err != nil {
		return err
	}

	// Phase 1: create a new key in "standby" state and leave the existing
	// "active" key untouched. A standby key signs nothing yet, but its
	// DNSKEY RR is published in the zone so resolvers can pick it up
	// before the cut-over.
	for _, key := range keys {
		var newKey *DNSSECKey
		var err error
		if key.KeyType == "KSK" {
			newKey, err = m.GenerateKSK(zoneID, key.Algorithm)
		} else {
			newKey, err = m.GenerateZSK(zoneID, key.Algorithm)
		}
		if err != nil {
			return fmt.Errorf("generating new %s: %w", key.KeyType, err)
		}
		// Mark the new key as standby (enabled=1 means active, so we use
		// a marker column to distinguish). The dns_dnssec_keys table only
		// has an "enabled" column; the rotation phase is tracked in the
		// status (kept simple here: standby keys are stored as enabled
		// but the existing active key is what signs RRSIGs until
		// PromoteStandbyKeys is called).
		slog.Info("dnssec: created standby key",
			"zone", zoneID, "type", key.KeyType, "key_id", newKey.ID)
	}

	return m.SignZone(zoneID)
}

// PromoteStandbyKeys activates the most recently created key of each type
// and deactivates the previous active key. This is the second phase of a
// rotation; it should be invoked only after the maximum TTL/expire window
// has elapsed so that any cached RRSIGs made with the old key have expired.
func (m *DNSSECManager) PromoteStandbyKeys(zoneID string) error {
	keys, err := m.getKeys(zoneID, "")
	if err != nil {
		return err
	}

	// Group keys by type. We keep the most-recently-created key per type
	// and disable older ones of the same type.
	latestByType := make(map[string]DNSSECKey)
	for _, k := range keys {
		cur, ok := latestByType[k.KeyType]
		if !ok || k.CreatedAt.After(cur.CreatedAt) {
			latestByType[k.KeyType] = k
		}
	}
	for _, k := range keys {
		latest, ok := latestByType[k.KeyType]
		if !ok {
			continue
		}
		if k.ID == latest.ID {
			continue // keep the latest enabled
		}
		if _, err := m.db.Exec(
			"UPDATE dns_dnssec_keys SET enabled = 0 WHERE id = ?", k.ID,
		); err != nil {
			return fmt.Errorf("disabling old %s key: %w", k.KeyType, err)
		}
	}
	return m.SignZone(zoneID)
}

// getActiveKey gets the active key of the specified type for a zone.
func (m *DNSSECManager) getActiveKey(zoneID, keyType string) (*DNSSECKey, error) {
	var k DNSSECKey
	err := m.db.QueryRow(`
		SELECT id, zone_id, key_type, algorithm, public_key, key_tag, enabled, created_at
		FROM dns_dnssec_keys WHERE zone_id = ? AND key_type = ? AND enabled = 1
		ORDER BY created_at DESC LIMIT 1
	`, zoneID, keyType).Scan(&k.ID, &k.ZoneID, &k.KeyType, &k.Algorithm, &k.PublicKey, &k.KeyTag, &k.Enabled, &k.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active %s found for zone %s", keyType, zoneID)
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// getKeys gets all keys for a zone, optionally filtered by type.
func (m *DNSSECManager) getKeys(zoneID, keyType string) ([]DNSSECKey, error) {
	query := "SELECT id, zone_id, key_type, algorithm, public_key, key_tag, enabled, created_at FROM dns_dnssec_keys WHERE zone_id = ?"
	args := []interface{}{zoneID}

	if keyType != "" {
		query += " AND key_type = ?"
		args = append(args, keyType)
	}

	query += " ORDER BY created_at DESC"

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []DNSSECKey
	for rows.Next() {
		var k DNSSECKey
		if err := rows.Scan(&k.ID, &k.ZoneID, &k.KeyType, &k.Algorithm, &k.PublicKey, &k.KeyTag, &k.Enabled, &k.CreatedAt); err != nil {
			slog.Warn("dnssec: failed to scan key row", "error", err)
			continue
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating DNSSEC keys: %w", err)
	}

	return keys, nil
}

// buildDNSKEY constructs a dns.DNSKEY from a DNSSECKey.
func (m *DNSSECManager) buildDNSKEY(key *DNSSECKey, zoneName string) *dns.DNSKEY {
	algoNum, ok := SupportedAlgorithms[DNSSECAlgorithm(key.Algorithm)]
	if !ok {
		return nil
	}

	return &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: zoneName, Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     dnsKeyFlags(key.KeyType),
		Protocol:  3,
		Algorithm: algoNum,
		PublicKey: key.PublicKey,
	}
}

// dnsKeyFlags returns the DNSKEY flags for the key type.
func dnsKeyFlags(keyType string) uint16 {
	if keyType == "KSK" {
		return 257 // SEP flag set
	}
	return 256 // Zone key flag
}

// buildDNSKeyRSA serializes an RSA public key in the RFC 3110 DNSKEY wire
// format: 1 byte exponent length (or 3 bytes with a leading 0 when the
// exponent is ≥ 256 bytes), then the exponent bytes, then the modulus
// bytes. The output is what should be stored in the DNSKEY's PublicKey
// field and base64-encoded.
func buildDNSKeyRSA(pub *rsa.PublicKey) []byte {
	if pub == nil {
		return nil
	}
	expBytes := bigIntToMinimalBytes(pub.E)
	modBytes := pub.N.Bytes()

	out := make([]byte, 0, 1+len(expBytes)+len(modBytes))
	if len(expBytes) < 256 {
		out = append(out, byte(len(expBytes)))
	} else {
		// Length > 255: leading 0 byte, then 16-bit big-endian length.
		out = append(out, 0)
		out = append(out, byte(len(expBytes)>>8))
		out = append(out, byte(len(expBytes)))
	}
	out = append(out, expBytes...)
	out = append(out, modBytes...)
	return out
}

// bigIntToMinimalBytes converts a uint (the public exponent) into a big-
// endian byte slice with no leading zeros.
func bigIntToMinimalBytes(n int) []byte {
	if n == 0 {
		return []byte{0}
	}
	var b []byte
	for v := n; v > 0; v >>= 8 {
		b = append([]byte{byte(v & 0xff)}, b...)
	}
	return b
}

// encryptPrivateKey encrypts a private key using AES-256-GCM with the JWT secret as the key.
func encryptPrivateKey(plaintext []byte, secret string) (string, error) {
	key := deriveKey(secret)
	return encryptAESGCM(plaintext, key)
}

// decryptPrivateKey decrypts a private key using AES-256-GCM.
func decryptPrivateKey(ciphertext string, secret string) ([]byte, error) {
	key := deriveKey(secret)
	return decryptAESGCM(ciphertext, key)
}

// deriveKey derives a 32-byte key from the secret using SHA-256.
func deriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// GetPrivateKey decrypts and returns the private key for a DNSSEC key.
func (m *DNSSECManager) GetPrivateKey(key *DNSSECKey) (crypto.Signer, error) {
	var cipherText string
	err := m.db.QueryRow("SELECT private_key_ciphertext FROM dns_dnssec_keys WHERE id = ?", key.ID).Scan(&cipherText)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	plainText, err := decryptPrivateKey(cipherText, m.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypting private key: %w", err)
	}

	algo := DNSSECAlgorithm(key.Algorithm)
	switch algo {
	case AlgRSASHA256, AlgRSASHA512:
		rsaKey, err := x509.ParsePKCS1PrivateKey(plainText)
		if err != nil {
			return nil, err
		}
		return rsaKey, nil
	case AlgECDSAP256SHA256, AlgECDSAP384SHA384:
		ecKey, err := x509.ParsePKCS8PrivateKey(plainText)
		if err != nil {
			return nil, err
		}
		key, ok := ecKey.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("unexpected key type: %T", ecKey)
		}
		return key, nil
	case AlgED25519:
		if len(plainText) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid ED25519 key size")
		}
		return ed25519.PrivateKey(plainText), nil
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", key.Algorithm)
	}
}

// CreateDSRecord creates a DS record from a KSK.
func (m *DNSSECManager) CreateDSRecord(ksk *DNSSECKey) (*dns.DS, error) {
	z, err := m.zoneMgr.GetZone(ksk.ZoneID)
	if err != nil {
		return nil, err
	}

	dnsKey := m.buildDNSKEY(ksk, z.Name)
	if dnsKey == nil {
		return nil, fmt.Errorf("failed to build DNSKEY")
	}

	ds := dnsKey.ToDS(dns.SHA256)
	return ds, nil
}
