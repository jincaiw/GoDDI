package zone

// Catalog Zone membership (RFC 9432, Technitium parity). Membership is
// materialised as PTR records owned by "<hash>.members.<catalog-zone>" inside
// the catalog zone itself, so AXFR, the in-memory store and the records UI
// serve catalog membership without special-casing.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// validateCatalogMembership checks that the target catalog exists, really is
// a catalog zone, and that the joining zone type is eligible.
func (m *ZoneManager) validateCatalogMembership(zoneType, catalogName string) error {
	if zoneType == string(ZoneTypeAllowed) || zoneType == string(ZoneTypeBlocked) {
		return fmt.Errorf("special zones cannot join a catalog")
	}
	catalog, err := m.GetZoneByName(catalogName)
	if err != nil {
		return fmt.Errorf("catalog zone not found: %s", catalogName)
	}
	if catalog.Type != string(ZoneTypeCatalog) {
		return fmt.Errorf("zone %s is not a catalog zone", catalogName)
	}
	return nil
}

// catalogMembershipOwner returns the RFC 9432 owner name for a member zone:
// the first 8 hex chars of the SHA-256 of the member name, under "members."
// inside the catalog zone. Stored without a trailing dot, matching the
// normalizeRecordName convention used by all other records.
func catalogMembershipOwner(catalogName, memberName string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSuffix(memberName, "."))))
	return hex.EncodeToString(sum[:4]) + ".members." + strings.TrimSuffix(strings.ToLower(catalogName), ".")
}

// addCatalogMembership inserts the membership PTR record into the catalog
// zone and bumps its serial so secondaries pick the change up via IXFR.
func (m *ZoneManager) addCatalogMembership(catalogName, memberName string) error {
	catalog, err := m.GetZoneByName(catalogName)
	if err != nil {
		return err
	}
	owner := catalogMembershipOwner(catalogName, memberName)

	_, err = m.db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES (?, ?, ?, 'PTR', ?, 0, 1)
	`, uuid.New().String(), catalog.ID, owner, strings.TrimSuffix(memberName, "."))
	if err != nil {
		return err
	}
	serial, _ := m.IncrementSerial(catalog.ID)
	if rm := m.recordManagerForClone(); rm != nil {
		rm.logChange(catalog.ID, serial, "add", owner, "PTR", memberName, 0, 0, 0, 0)
	}
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}
	return nil
}

// removeCatalogMembership deletes any membership PTR record for memberName
// from all catalog zones (a zone belongs to at most one catalog, but the
// lookup is by RDATA to stay name-agnostic).
func (m *ZoneManager) removeCatalogMembership(memberName string) error {
	rdata := strings.TrimSuffix(strings.ToLower(memberName), ".")
	res, err := m.db.Exec(`
		DELETE FROM dns_records
		WHERE type = 'PTR' AND LOWER(value) = ? AND zone_id IN (
			SELECT id FROM dns_zones WHERE type = ?
		)
	`, rdata, string(ZoneTypeCatalog))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	// Bump serials of every catalog zone that lost a member.
	rows, err := m.db.Query(`SELECT id FROM dns_zones WHERE type = ?`, string(ZoneTypeCatalog))
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		serial, _ := m.IncrementSerial(id)
		if rm := m.recordManagerForClone(); rm != nil {
			rm.logChange(id, serial, "delete", "", "PTR", memberName, 0, 0, 0, 0)
		}
	}
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}
	return nil
}

// ListCatalogMembers returns the member zone names of a catalog zone as
// recorded by its membership PTR records.
func (m *ZoneManager) ListCatalogMembers(catalogZoneID string) ([]string, error) {
	rows, err := m.db.Query(`
		SELECT value FROM dns_records
		WHERE zone_id = ? AND type = 'PTR' AND name LIKE '%.members.%'
		ORDER BY value
	`, catalogZoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err == nil {
			members = append(members, v)
		}
	}
	return members, rows.Err()
}
