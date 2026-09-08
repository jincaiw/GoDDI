package zone

import (
	"fmt"
	"time"
)

// ZoneChangeEntry is a single RR mutation recorded in dns_zone_changes.
// Exposed via GET /api/v1/dns/zones/{id}/history (Technitium Zone History
// parity); the same rows also feed true IXFR (RFC 1995).
type ZoneChangeEntry struct {
	ID         string    `json:"id"`
	ZoneID     string    `json:"zone_id"`
	Serial     uint32    `json:"serial"`
	ChangeType string    `json:"change_type"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Value      string    `json:"value"`
	TTL        int       `json:"ttl"`
	Priority   int       `json:"priority,omitempty"`
	Weight     int       `json:"weight,omitempty"`
	Port       int       `json:"port,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListZoneHistory returns the recorded RR mutations for a zone, newest
// (highest serial) first, paginated.
func (m *ZoneManager) ListZoneHistory(zoneID string, page, pageSize int) ([]ZoneChangeEntry, int64, error) {
	if zoneID == "" {
		return nil, 0, fmt.Errorf("zone id is required")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}

	var total int64
	if err := m.db.QueryRow("SELECT COUNT(*) FROM dns_zone_changes WHERE zone_id = ?", zoneID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting zone history: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := m.db.Query(`
		SELECT id, zone_id, serial, change_type, name, type, value,
			COALESCE(ttl, 0), COALESCE(priority, 0), COALESCE(weight, 0), COALESCE(port, 0), created_at
		FROM dns_zone_changes WHERE zone_id = ?
		ORDER BY serial DESC, created_at DESC
		LIMIT ? OFFSET ?
	`, zoneID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying zone history: %w", err)
	}
	defer rows.Close()

	var entries []ZoneChangeEntry
	for rows.Next() {
		var e ZoneChangeEntry
		if err := rows.Scan(&e.ID, &e.ZoneID, &e.Serial, &e.ChangeType, &e.Name, &e.Type,
			&e.Value, &e.TTL, &e.Priority, &e.Weight, &e.Port, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating zone history: %w", err)
	}

	return entries, total, nil
}
