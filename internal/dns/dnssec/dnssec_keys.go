package dnssec

// Batch F (Technitium parity): DS record export, key lifecycle management
// and per-zone NSEC3 parameters (RFC 5155). NSEC3 params take effect once
// real zone signing lands; they are persisted on dns_zones.

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

// DSInfo is the parent-side delegation signer data for one KSK.
type DSInfo struct {
	KeyTag     int    `json:"key_tag"`
	Algorithm  string `json:"algorithm"`
	DigestType uint8  `json:"digest_type"` // 2 = SHA-256, 4 = SHA-384
	Digest     string `json:"digest"`      // hex encoded
	KeyFlags   uint16 `json:"key_flags"`
	PublicKey  string `json:"public_key"`
}

// GetDSRecords computes the DS records for every enabled KSK of a zone, the
// data an operator needs to paste into the parent zone or registrar.
func (m *DNSSECManager) GetDSRecords(zoneID string) ([]DSInfo, error) {
	z, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	keys, err := m.getKeys(zoneID, "KSK")
	if err != nil {
		return nil, err
	}

	var out []DSInfo
	for i := range keys {
		if !keys[i].Enabled {
			continue
		}
		key := &keys[i]
		dnsKey := m.buildDNSKEY(key, z.Name)
		if dnsKey == nil {
			continue
		}
		// SHA-256 (digest type 2) is the interoperable default; SHA-384
		// (type 4) for P-384 keys. SHA-1 is deliberately not offered.
		digestType := uint8(dns.SHA256)
		if key.Algorithm == string(AlgECDSAP384SHA384) {
			digestType = uint8(dns.SHA384)
		}
		ds := dnsKey.ToDS(digestType)
		if ds == nil {
			continue
		}
		digest := ds.Digest
		if digest == "" {
			continue
		}
		out = append(out, DSInfo{
			KeyTag:     key.KeyTag,
			Algorithm:  key.Algorithm,
			DigestType: digestType,
			Digest:     strings.ToLower(digest),
			KeyFlags:   dnsKey.Flags,
			PublicKey:  key.PublicKey,
		})
	}
	return out, nil
}

// DeleteKey removes a single DNSSEC key from a zone. The key must belong to
// the given zone; deleting the last active KSK or ZSK is allowed but callers
// (UI/API) are expected to warn — SignZone re-validates key presence.
func (m *DNSSECManager) DeleteKey(zoneID, keyID string) error {
	res, err := m.db.Exec(`DELETE FROM dns_dnssec_keys WHERE id = ? AND zone_id = ?`, keyID, zoneID)
	if err != nil {
		return fmt.Errorf("deleting DNSSEC key: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("key not found in this zone")
	}
	return nil
}

// SetKeyEnabled toggles a single key. Disabled keys do not sign and are not
// published in the DNSKEY RRset.
func (m *DNSSECManager) SetKeyEnabled(zoneID, keyID string, enabled bool) error {
	res, err := m.db.Exec(`UPDATE dns_dnssec_keys SET enabled = ? WHERE id = ? AND zone_id = ?`, enabled, keyID, zoneID)
	if err != nil {
		return fmt.Errorf("updating DNSSEC key: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("key not found in this zone")
	}
	return nil
}

// NSEC3Params holds the per-zone NSEC3 configuration (RFC 5155).
type NSEC3Params struct {
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"` // hex encoded; empty = no salt
	OptOut     bool   `json:"optout"`
}

// GetNSEC3Params returns the NSEC3 parameters of a zone.
func (m *DNSSECManager) GetNSEC3Params(zoneID string) (*NSEC3Params, error) {
	if _, err := m.zoneMgr.GetZone(zoneID); err != nil {
		return nil, err
	}
	var iterations int
	var salt string
	var optOut bool
	err := m.db.QueryRow(
		`SELECT nsec3_iterations, nsec3_salt, nsec3_optout FROM dns_zones WHERE id = ?`, zoneID,
	).Scan(&iterations, &salt, &optOut)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found")
	}
	if err != nil {
		return nil, err
	}
	return &NSEC3Params{Iterations: iterations, Salt: salt, OptOut: optOut}, nil
}

// SetNSEC3Params validates and persists the NSEC3 parameters of a zone.
// RFC 5155 caps iterations at 2500 (recommendation 50 in RFC 9276 era);
// values above 100 are rejected here as a denial-of-service guard.
func (m *DNSSECManager) SetNSEC3Params(zoneID string, params NSEC3Params) error {
	if _, err := m.zoneMgr.GetZone(zoneID); err != nil {
		return err
	}
	if params.Iterations < 0 || params.Iterations > 100 {
		return fmt.Errorf("iterations must be between 0 and 100")
	}
	if params.Salt != "" {
		if _, err := hex.DecodeString(params.Salt); err != nil {
			return fmt.Errorf("salt must be hex encoded")
		}
		if len(params.Salt) > 2*64 {
			return fmt.Errorf("salt too long (max 64 bytes)")
		}
	}
	params.Salt = strings.ToLower(params.Salt)
	_, err := m.db.Exec(
		`UPDATE dns_zones SET nsec3_iterations = ?, nsec3_salt = ?, nsec3_optout = ?, updated_at = datetime('now') WHERE id = ?`,
		params.Iterations, params.Salt, params.OptOut, zoneID,
	)
	return err
}
