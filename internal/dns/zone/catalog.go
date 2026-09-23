package zone

// Catalog Zone membership (RFC 9432, Technitium parity). Membership is
// materialised as PTR records owned by "<hash>.members.<catalog-zone>" inside
// the catalog zone itself, so AXFR, the in-memory store and the records UI
// serve catalog membership without special-casing.

import (
	"crypto/sha256"
	"database/sql"
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
	memberValue := strings.TrimSuffix(memberName, ".")

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES (?, ?, ?, 'PTR', ?, 0, 1)
	`, uuid.New().String(), catalog.ID, owner, memberValue)
	if err != nil {
		return err
	}
	serial, err := bumpZoneSerialTx(tx, catalog.ID)
	if err != nil {
		return err
	}
	if err := logChangeTx(tx, catalog.ID, serial, "add", owner, "PTR", memberValue, 0, 0, 0, 0, 0, ""); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if rm := m.recordManagerForClone(); rm != nil {
		rm.notifyPrimary(catalog.ID)
	}
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}
	return nil
}

func addCatalogMembershipTx(tx *sql.Tx, catalogName, memberName string) (string, error) {
	var catalogID string
	if err := tx.QueryRow(`SELECT id FROM dns_zones WHERE name = ? AND type = ?`,
		catalogName, string(ZoneTypeCatalog)).Scan(&catalogID); err != nil {
		return "", err
	}
	owner := catalogMembershipOwner(catalogName, memberName)
	memberValue := strings.TrimSuffix(memberName, ".")
	if _, err := tx.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES (?, ?, ?, 'PTR', ?, 0, 1)
	`, uuid.New().String(), catalogID, owner, memberValue); err != nil {
		return "", err
	}
	serial, err := bumpZoneSerialTx(tx, catalogID)
	if err != nil {
		return "", err
	}
	if err := logChangeTx(tx, catalogID, serial, "add", owner, "PTR", memberValue, 0, 0, 0, 0, 0, ""); err != nil {
		return "", err
	}
	return catalogID, nil
}

// removeCatalogMembership deletes any membership PTR record for memberName
// from all catalog zones (a zone belongs to at most one catalog, but the
// lookup is by RDATA to stay name-agnostic).
func (m *ZoneManager) removeCatalogMembership(memberName string) error {
	rdata := strings.TrimSuffix(strings.ToLower(memberName), ".")
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`
		SELECT r.id, r.zone_id, r.name, r.type, r.value, r.ttl,
			COALESCE(r.priority, 0), COALESCE(r.weight, 0), COALESCE(r.port, 0)
		FROM dns_records r JOIN dns_zones z ON z.id = r.zone_id
		WHERE r.type = 'PTR' AND LOWER(r.value) = ? AND z.type = ?
	`, rdata, string(ZoneTypeCatalog))
	if err != nil {
		return err
	}
	var removed []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.ZoneID, &rec.Name, &rec.Type, &rec.Value, &rec.TTL,
			&rec.Priority, &rec.Weight, &rec.Port); err != nil {
			rows.Close()
			return err
		}
		removed = append(removed, rec)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(removed) == 0 {
		return nil
	}
	if _, err := tx.Exec(`
		DELETE FROM dns_records WHERE type = 'PTR' AND LOWER(value) = ? AND zone_id IN (
			SELECT id FROM dns_zones WHERE type = ?
		)
	`, rdata, string(ZoneTypeCatalog)); err != nil {
		return err
	}
	serialByZone := make(map[string]uint32)
	for _, rec := range removed {
		serial, ok := serialByZone[rec.ZoneID]
		if !ok {
			serial, err = bumpZoneSerialTx(tx, rec.ZoneID)
			if err != nil {
				return err
			}
			serialByZone[rec.ZoneID] = serial
		}
		if err := logChangeTx(tx, rec.ZoneID, serial, "delete", rec.Name, rec.Type, rec.Value,
			rec.TTL, rec.Priority, rec.Weight, rec.Port, rec.Flag, rec.Tag); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if rm := m.recordManagerForClone(); rm != nil {
		for zoneID := range serialByZone {
			rm.notifyPrimary(zoneID)
		}
	}
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}
	return nil
}

func removeCatalogMembershipTx(tx *sql.Tx, memberName string) (map[string]struct{}, error) {
	rdata := strings.TrimSuffix(strings.ToLower(memberName), ".")
	rows, err := tx.Query(`
		SELECT r.id, r.zone_id, r.name, r.type, r.value, r.ttl,
			COALESCE(r.priority, 0), COALESCE(r.weight, 0), COALESCE(r.port, 0)
		FROM dns_records r JOIN dns_zones z ON z.id = r.zone_id
		WHERE r.type = 'PTR' AND LOWER(r.value) = ? AND z.type = ?`,
		rdata, string(ZoneTypeCatalog))
	if err != nil {
		return nil, err
	}
	var records []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.ZoneID, &rec.Name, &rec.Type, &rec.Value, &rec.TTL,
			&rec.Priority, &rec.Weight, &rec.Port); err != nil {
			rows.Close()
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return map[string]struct{}{}, nil
	}
	if _, err := tx.Exec(`DELETE FROM dns_records WHERE type = 'PTR' AND LOWER(value) = ? AND zone_id IN (
		SELECT id FROM dns_zones WHERE type = ?
	)`, rdata, string(ZoneTypeCatalog)); err != nil {
		return nil, err
	}
	serialByZone := make(map[string]uint32)
	for _, rec := range records {
		serial, ok := serialByZone[rec.ZoneID]
		if !ok {
			serial, err = bumpZoneSerialTx(tx, rec.ZoneID)
			if err != nil {
				return nil, err
			}
			serialByZone[rec.ZoneID] = serial
		}
		if err := logChangeTx(tx, rec.ZoneID, serial, "delete", rec.Name, rec.Type, rec.Value,
			rec.TTL, rec.Priority, rec.Weight, rec.Port, rec.Flag, rec.Tag); err != nil {
			return nil, err
		}
	}
	changed := make(map[string]struct{}, len(serialByZone))
	for zoneID := range serialByZone {
		changed[zoneID] = struct{}{}
	}
	return changed, nil
}

// renameCatalogMemberTx updates the catalog PTR RDATA for a member zone name
// within the caller's transaction and returns the catalog zones whose serials
// were advanced.
func renameCatalogMemberTx(tx *sql.Tx, oldName, newName string) (map[string]struct{}, error) {
	oldValue := strings.TrimSuffix(strings.ToLower(oldName), ".")
	newValue := strings.TrimSuffix(newName, ".")
	rows, err := tx.Query(`
		SELECT r.id, r.zone_id, r.name, r.type, r.value, r.ttl,
			COALESCE(r.priority, 0), COALESCE(r.weight, 0), COALESCE(r.port, 0)
		FROM dns_records r JOIN dns_zones z ON z.id = r.zone_id
		WHERE r.type = 'PTR' AND LOWER(r.value) = ? AND z.type = ?`,
		oldValue, string(ZoneTypeCatalog))
	if err != nil {
		return nil, err
	}
	var records []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.ZoneID, &rec.Name, &rec.Type, &rec.Value, &rec.TTL,
			&rec.Priority, &rec.Weight, &rec.Port); err != nil {
			rows.Close()
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	serialByZone := make(map[string]uint32)
	for _, rec := range records {
		serial, ok := serialByZone[rec.ZoneID]
		if !ok {
			serial, err = bumpZoneSerialTx(tx, rec.ZoneID)
			if err != nil {
				return nil, err
			}
			serialByZone[rec.ZoneID] = serial
		}
		if _, err := tx.Exec(`UPDATE dns_records SET value = ?, updated_at = datetime('now') WHERE id = ?`, newValue, rec.ID); err != nil {
			return nil, err
		}
		if err := logChangeTx(tx, rec.ZoneID, serial, "delete", rec.Name, rec.Type, rec.Value,
			rec.TTL, rec.Priority, rec.Weight, rec.Port, rec.Flag, rec.Tag); err != nil {
			return nil, err
		}
		if err := logChangeTx(tx, rec.ZoneID, serial, "add", rec.Name, rec.Type, newValue,
			rec.TTL, rec.Priority, rec.Weight, rec.Port, rec.Flag, rec.Tag); err != nil {
			return nil, err
		}
	}
	changed := make(map[string]struct{}, len(serialByZone))
	for zoneID := range serialByZone {
		changed[zoneID] = struct{}{}
	}
	return changed, nil
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
