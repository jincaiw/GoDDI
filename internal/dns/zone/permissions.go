package zone

import (
	"fmt"

	"github.com/google/uuid"
)

// ZonePermission grants one user or group (role) access to a zone.
// Zones without any permission rows keep the global-RBAC-only behavior;
// zones with rows restrict access to the listed principals, intersected
// with global RBAC (Technitium Zone Permissions parity).
type ZonePermission struct {
	ID            string `json:"id"`
	ZoneID        string `json:"zone_id"`
	PrincipalType string `json:"principal_type"` // "user" | "group"
	PrincipalID   string `json:"principal_id"`
	CanView       bool   `json:"can_view"`
	CanModify     bool   `json:"can_modify"`
	CanDelete     bool   `json:"can_delete"`
}

// GetZonePermissions returns the permission rows configured for a zone.
func (m *ZoneManager) GetZonePermissions(zoneID string) ([]ZonePermission, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone id is required")
	}

	rows, err := m.db.Query(`
		SELECT id, zone_id, principal_type, principal_id, can_view, can_modify, can_delete
		FROM dns_zone_permissions WHERE zone_id = ? ORDER BY principal_type, principal_id
	`, zoneID)
	if err != nil {
		return nil, fmt.Errorf("querying zone permissions: %w", err)
	}
	defer rows.Close()

	var perms []ZonePermission
	for rows.Next() {
		var p ZonePermission
		if err := rows.Scan(&p.ID, &p.ZoneID, &p.PrincipalType, &p.PrincipalID,
			&p.CanView, &p.CanModify, &p.CanDelete); err != nil {
			continue
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// SetZonePermissions replaces the full permission list of a zone.
// An empty list clears custom permissions (reverting to global-RBAC-only).
func (m *ZoneManager) SetZonePermissions(zoneID string, perms []ZonePermission) error {
	if zoneID == "" {
		return fmt.Errorf("zone id is required")
	}

	// Validate the zone exists.
	if _, err := m.GetZone(zoneID); err != nil {
		return err
	}

	// Validate principal types before touching the table.
	for _, p := range perms {
		if p.PrincipalType != "user" && p.PrincipalType != "group" {
			return fmt.Errorf("invalid principal_type: %s", p.PrincipalType)
		}
		if p.PrincipalID == "" {
			return fmt.Errorf("principal_id is required")
		}
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM dns_zone_permissions WHERE zone_id = ?", zoneID); err != nil {
		return fmt.Errorf("clearing zone permissions: %w", err)
	}

	for _, p := range perms {
		id := p.ID
		if id == "" {
			id = uuid.New().String()
		}
		if _, err := tx.Exec(`
			INSERT INTO dns_zone_permissions (id, zone_id, principal_type, principal_id, can_view, can_modify, can_delete)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, id, zoneID, p.PrincipalType, p.PrincipalID, p.CanView, p.CanModify, p.CanDelete); err != nil {
			return fmt.Errorf("inserting zone permission: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing zone permissions: %w", err)
	}
	return nil
}

// ZoneHasCustomPermissions reports whether a zone has an explicit permission
// list configured.
func (m *ZoneManager) ZoneHasCustomPermissions(zoneID string) (bool, error) {
	var count int
	err := m.db.QueryRow("SELECT COUNT(*) FROM dns_zone_permissions WHERE zone_id = ?", zoneID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ZonePermissionAllows checks whether the given user (and their group/role
// ids) may perform action ("view"/"modify"/"delete") on the zone. A zone
// without explicit permission rows allows everything (global RBAC governs).
func (m *ZoneManager) ZonePermissionAllows(zoneID, userID string, groupIDs []string, action string) (bool, error) {
	perms, err := m.GetZonePermissions(zoneID)
	if err != nil {
		return false, err
	}
	if len(perms) == 0 {
		return true, nil
	}

	groupSet := make(map[string]bool, len(groupIDs))
	for _, g := range groupIDs {
		groupSet[g] = true
	}

	allowed := false
	for _, p := range perms {
		matches := (p.PrincipalType == "user" && p.PrincipalID == userID) ||
			(p.PrincipalType == "group" && groupSet[p.PrincipalID])
		if !matches {
			continue
		}
		switch action {
		case "view":
			if p.CanView {
				allowed = true
			}
		case "modify":
			if p.CanModify {
				allowed = true
			}
		case "delete":
			if p.CanDelete {
				allowed = true
			}
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}
