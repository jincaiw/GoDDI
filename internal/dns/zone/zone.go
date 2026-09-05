package zone

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ZoneType represents the type of a DNS zone.
type ZoneType string

const (
	ZoneTypePrimary   ZoneType = "primary"
	ZoneTypeSecondary ZoneType = "secondary"
	ZoneTypeStub      ZoneType = "stub"
	ZoneTypeForward   ZoneType = "forward"
	ZoneTypeReverse   ZoneType = "reverse"
)

// ValidZoneTypes contains all valid zone types.
var ValidZoneTypes = []ZoneType{ZoneTypePrimary, ZoneTypeSecondary, ZoneTypeStub, ZoneTypeForward, ZoneTypeReverse}

// Zone represents a DNS zone with full metadata.
type Zone struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Enabled        bool      `json:"enabled"`
	DNSSECEnabled  bool      `json:"dnssec_enabled"`
	DefaultTTL     int       `json:"default_ttl"`
	SOA_MName      string    `json:"soa_mname"`
	SOA_RName      string    `json:"soa_rname"`
	Serial         uint32    `json:"serial"`
	Refresh        int       `json:"refresh"`
	Retry          int       `json:"retry"`
	Expire         int       `json:"expire"`
	Minimum        int       `json:"minimum"`
	TransferPolicy string    `json:"transfer_policy,omitempty"`
	UpdatePolicy   string    `json:"update_policy,omitempty"`
	RecordsCount   int       `json:"records_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ZoneOptions contains options for creating or updating a zone.
type ZoneOptions struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Enabled        *bool  `json:"enabled,omitempty"`
	DNSSECEnabled  *bool  `json:"dnssec_enabled,omitempty"`
	DefaultTTL     *int   `json:"default_ttl,omitempty"`
	SOA_MName      string `json:"soa_mname,omitempty"`
	SOA_RName      string `json:"soa_rname,omitempty"`
	Refresh        *int   `json:"refresh,omitempty"`
	Retry          *int   `json:"retry,omitempty"`
	Expire         *int   `json:"expire,omitempty"`
	Minimum        *int   `json:"minimum,omitempty"`
	TransferPolicy string `json:"transfer_policy,omitempty"`
	UpdatePolicy   string `json:"update_policy,omitempty"`
}

// ZoneFilter contains filter options for listing zones.
type ZoneFilter struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Type     string `json:"type,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Name     string `json:"name,omitempty"`
}

// ZoneManager provides CRUD operations for DNS zones.
type ZoneManager struct {
	db        *sql.DB
	zoneStore *Store
}

// NewZoneManager creates a new ZoneManager.
func NewZoneManager(db *sql.DB, zoneStore *Store) *ZoneManager {
	return &ZoneManager{
		db:        db,
		zoneStore: zoneStore,
	}
}

// CreateZone creates a new DNS zone.
func (m *ZoneManager) CreateZone(opts ZoneOptions) (*Zone, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	if opts.Type == "" {
		opts.Type = string(ZoneTypePrimary)
	}
	if !isValidZoneType(opts.Type) {
		return nil, fmt.Errorf("invalid zone type: %s", opts.Type)
	}

	// Normalize zone name: ensure trailing dot for FQDN.
	name := strings.TrimSuffix(strings.ToLower(opts.Name), ".")
	name += "."

	// Check for duplicate.
	var count int
	err := m.db.QueryRow("SELECT COUNT(*) FROM dns_zones WHERE name = ?", name).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("checking duplicate zone: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("zone already exists: %s", name)
	}

	// Set defaults.
	defaultTTL := 3600
	if opts.DefaultTTL != nil {
		defaultTTL = *opts.DefaultTTL
	}
	refresh := 3600
	if opts.Refresh != nil {
		refresh = *opts.Refresh
	}
	retry := 600
	if opts.Retry != nil {
		retry = *opts.Retry
	}
	expire := 86400
	if opts.Expire != nil {
		expire = *opts.Expire
	}
	minimum := 300
	if opts.Minimum != nil {
		minimum = *opts.Minimum
	}
	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}

	// SOA defaults.
	soaMName := opts.SOA_MName
	if soaMName == "" {
		soaMName = "ns1." + name
	}
	if !strings.HasSuffix(soaMName, ".") {
		soaMName += "."
	}
	soaRName := opts.SOA_RName
	if soaRName == "" {
		soaRName = "admin." + name
	}
	if !strings.HasSuffix(soaRName, ".") {
		soaRName += "."
	}

	// Generate serial: YYYYMMDDNN format. We use the global max serial as
	// the lower bound to ensure a freshly-created zone does not collide
	// with serial numbers already issued today.
	serial := generateSerial(m.db)

	id := uuid.New().String()
	_, err = m.db.Exec(`
		INSERT INTO dns_zones (id, name, type, enabled, dnssec_enabled, default_ttl,
			soa_mname, soa_rname, serial, refresh, retry, expire, minimum,
			transfer_policy, update_policy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, name, opts.Type, enabled, false, defaultTTL,
		soaMName, soaRName, serial, refresh, retry, expire, minimum,
		opts.TransferPolicy, opts.UpdatePolicy)
	if err != nil {
		return nil, fmt.Errorf("inserting zone: %w", err)
	}

	zone := &Zone{
		ID:             id,
		Name:           name,
		Type:           opts.Type,
		Enabled:        enabled,
		DNSSECEnabled:  false,
		DefaultTTL:     defaultTTL,
		SOA_MName:      soaMName,
		SOA_RName:      soaRName,
		Serial:         serial,
		Refresh:        refresh,
		Retry:          retry,
		Expire:         expire,
		Minimum:        minimum,
		TransferPolicy: opts.TransferPolicy,
		UpdatePolicy:   opts.UpdatePolicy,
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return zone, nil
}

// GetZone retrieves a zone by ID.
func (m *ZoneManager) GetZone(id string) (*Zone, error) {
	if id == "" {
		return nil, fmt.Errorf("zone id is required")
	}

	var z Zone
	var transferPolicy, updatePolicy sql.NullString
	err := m.db.QueryRow(`
		SELECT id, name, type, enabled, dnssec_enabled, default_ttl,
			soa_mname, soa_rname, serial, refresh, retry, expire, minimum,
			transfer_policy, update_policy, created_at, updated_at
		FROM dns_zones WHERE id = ?
	`, id).Scan(
		&z.ID, &z.Name, &z.Type, &z.Enabled, &z.DNSSECEnabled, &z.DefaultTTL,
		&z.SOA_MName, &z.SOA_RName, &z.Serial, &z.Refresh, &z.Retry, &z.Expire, &z.Minimum,
		&transferPolicy, &updatePolicy, &z.CreatedAt, &z.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying zone: %w", err)
	}

	if transferPolicy.Valid {
		z.TransferPolicy = transferPolicy.String
	}
	if updatePolicy.Valid {
		z.UpdatePolicy = updatePolicy.String
	}

	return &z, nil
}

// GetZoneByName retrieves a zone by name.
func (m *ZoneManager) GetZoneByName(name string) (*Zone, error) {
	if name == "" {
		return nil, fmt.Errorf("zone name is required")
	}
	if !strings.HasSuffix(name, ".") {
		name += "."
	}

	var z Zone
	var transferPolicy, updatePolicy sql.NullString
	err := m.db.QueryRow(`
		SELECT id, name, type, enabled, dnssec_enabled, default_ttl,
			soa_mname, soa_rname, serial, refresh, retry, expire, minimum,
			transfer_policy, update_policy, created_at, updated_at
		FROM dns_zones WHERE name = ?
	`, name).Scan(
		&z.ID, &z.Name, &z.Type, &z.Enabled, &z.DNSSECEnabled, &z.DefaultTTL,
		&z.SOA_MName, &z.SOA_RName, &z.Serial, &z.Refresh, &z.Retry, &z.Expire, &z.Minimum,
		&transferPolicy, &updatePolicy, &z.CreatedAt, &z.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying zone: %w", err)
	}

	if transferPolicy.Valid {
		z.TransferPolicy = transferPolicy.String
	}
	if updatePolicy.Valid {
		z.UpdatePolicy = updatePolicy.String
	}

	return &z, nil
}

// ListZones returns a paginated list of zones.
func (m *ZoneManager) ListZones(filter ZoneFilter) ([]Zone, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	var conditions []string
	var args []interface{}

	if filter.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, filter.Type)
	}
	if filter.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, *filter.Enabled)
	}
	if filter.Name != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total.
	var total int64
	countSQL := "SELECT COUNT(*) FROM dns_zones " + whereClause
	err := m.db.QueryRow(countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting zones: %w", err)
	}

	// Query page.
	offset := (filter.Page - 1) * filter.PageSize
	querySQL := fmt.Sprintf(`
		SELECT id, name, type, enabled, dnssec_enabled, default_ttl,
			soa_mname, soa_rname, serial, refresh, retry, expire, minimum,
			transfer_policy, update_policy, created_at, updated_at,
			(SELECT COUNT(*) FROM dns_records WHERE zone_id = dns_zones.id)
		FROM dns_zones %s
		ORDER BY name
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs := append(args, filter.PageSize, offset)
	rows, err := m.db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying zones: %w", err)
	}
	defer rows.Close()

	var zones []Zone
	for rows.Next() {
		var z Zone
		var transferPolicy, updatePolicy sql.NullString
		if err := rows.Scan(
			&z.ID, &z.Name, &z.Type, &z.Enabled, &z.DNSSECEnabled, &z.DefaultTTL,
			&z.SOA_MName, &z.SOA_RName, &z.Serial, &z.Refresh, &z.Retry, &z.Expire, &z.Minimum,
			&transferPolicy, &updatePolicy, &z.CreatedAt, &z.UpdatedAt,
			&z.RecordsCount,
		); err != nil {
			continue
		}
		if transferPolicy.Valid {
			z.TransferPolicy = transferPolicy.String
		}
		if updatePolicy.Valid {
			z.UpdatePolicy = updatePolicy.String
		}
		zones = append(zones, z)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating zones: %w", err)
	}

	return zones, total, nil
}

// UpdateZone updates an existing zone.
func (m *ZoneManager) UpdateZone(id string, opts ZoneOptions) (*Zone, error) {
	if id == "" {
		return nil, fmt.Errorf("zone id is required")
	}

	// Verify zone exists.
	existing, err := m.GetZone(id)
	if err != nil {
		return nil, err
	}

	// Build update query dynamically.
	var setClauses []string
	var args []interface{}

	if opts.Name != "" && opts.Name != existing.Name {
		name := strings.TrimSuffix(strings.ToLower(opts.Name), ".")
		name += "."
		setClauses = append(setClauses, "name = ?")
		args = append(args, name)
		existing.Name = name
	}
	if opts.Type != "" {
		if !isValidZoneType(opts.Type) {
			return nil, fmt.Errorf("invalid zone type: %s", opts.Type)
		}
		setClauses = append(setClauses, "type = ?")
		args = append(args, opts.Type)
		existing.Type = opts.Type
	}
	if opts.Enabled != nil {
		setClauses = append(setClauses, "enabled = ?")
		args = append(args, *opts.Enabled)
		existing.Enabled = *opts.Enabled
	}
	if opts.DNSSECEnabled != nil {
		setClauses = append(setClauses, "dnssec_enabled = ?")
		args = append(args, *opts.DNSSECEnabled)
		existing.DNSSECEnabled = *opts.DNSSECEnabled
	}
	if opts.DefaultTTL != nil {
		setClauses = append(setClauses, "default_ttl = ?")
		args = append(args, *opts.DefaultTTL)
		existing.DefaultTTL = *opts.DefaultTTL
	}
	if opts.SOA_MName != "" {
		soaMName := opts.SOA_MName
		if !strings.HasSuffix(soaMName, ".") {
			soaMName += "."
		}
		setClauses = append(setClauses, "soa_mname = ?")
		args = append(args, soaMName)
		existing.SOA_MName = soaMName
	}
	if opts.SOA_RName != "" {
		soaRName := opts.SOA_RName
		if !strings.HasSuffix(soaRName, ".") {
			soaRName += "."
		}
		setClauses = append(setClauses, "soa_rname = ?")
		args = append(args, soaRName)
		existing.SOA_RName = soaRName
	}
	if opts.Refresh != nil {
		setClauses = append(setClauses, "refresh = ?")
		args = append(args, *opts.Refresh)
		existing.Refresh = *opts.Refresh
	}
	if opts.Retry != nil {
		setClauses = append(setClauses, "retry = ?")
		args = append(args, *opts.Retry)
		existing.Retry = *opts.Retry
	}
	if opts.Expire != nil {
		setClauses = append(setClauses, "expire = ?")
		args = append(args, *opts.Expire)
		existing.Expire = *opts.Expire
	}
	if opts.Minimum != nil {
		setClauses = append(setClauses, "minimum = ?")
		args = append(args, *opts.Minimum)
		existing.Minimum = *opts.Minimum
	}
	if opts.TransferPolicy != "" {
		setClauses = append(setClauses, "transfer_policy = ?")
		args = append(args, opts.TransferPolicy)
		existing.TransferPolicy = opts.TransferPolicy
	}
	if opts.UpdatePolicy != "" {
		setClauses = append(setClauses, "update_policy = ?")
		args = append(args, opts.UpdatePolicy)
		existing.UpdatePolicy = opts.UpdatePolicy
	}

	if len(setClauses) == 0 {
		return existing, nil
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := "UPDATE dns_zones SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	_, err = m.db.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("updating zone: %w", err)
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return m.GetZone(id)
}

// DeleteZone deletes a zone by ID.
func (m *ZoneManager) DeleteZone(id string) error {
	if id == "" {
		return fmt.Errorf("zone id is required")
	}

	// Verify zone exists.
	_, err := m.GetZone(id)
	if err != nil {
		return err
	}

	// Use a transaction to ensure atomic deletion of records and zone.
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete records first (cascade should handle this, but be explicit).
	if _, err := tx.Exec("DELETE FROM dns_records WHERE zone_id = ?", id); err != nil {
		return fmt.Errorf("deleting zone records: %w", err)
	}

	// Delete zone.
	if _, err := tx.Exec("DELETE FROM dns_zones WHERE id = ?", id); err != nil {
		return fmt.Errorf("deleting zone: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

// IncrementSerial increments the SOA serial for a zone.
// The read-modify-write runs inside a transaction guarded by a conditional
// UPDATE: two concurrent increments could otherwise compute the same new
// serial (violating SOA monotonicity), with the loser silently ignored.
func (m *ZoneManager) IncrementSerial(zoneID string) (uint32, error) {
	tx, err := m.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current serial.
	var currentSerial uint32
	err = tx.QueryRow("SELECT serial FROM dns_zones WHERE id = ?", zoneID).Scan(&currentSerial)
	if err != nil {
		return 0, fmt.Errorf("querying serial: %w", err)
	}

	// Use a serial that is always strictly greater than the current one.
	// We compare against the zone's own serial (not the global max) so that
	// concurrent updates to unrelated zones cannot block us, but we still
	// guarantee monotonic ordering for this zone.
	newSerial := m.generateSerialForZone(currentSerial)
	if newSerial <= currentSerial {
		newSerial = currentSerial + 1
	}

	result, err := tx.Exec(
		"UPDATE dns_zones SET serial = ?, updated_at = datetime('now') WHERE id = ? AND serial = ?",
		newSerial, zoneID, currentSerial)
	if err != nil {
		return 0, fmt.Errorf("updating serial: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		// A concurrent writer changed the serial between our read and
		// write; report failure so the caller can retry.
		return 0, fmt.Errorf("serial changed concurrently, retry needed")
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing serial update: %w", err)
	}

	return newSerial, nil
}

// generateSerialForZone returns the next SOA serial in the YYYYMMDDNN format
// that is guaranteed to be greater than currentSerial. It uses the current
// UTC day as the high bits and starts the NN counter at 1; if today's
// serial has already been bumped past today*100+1, the counter is
// incremented above the per-zone current value. A persistence layer is not
// required: serial ordering is enforced by the "must be > currentSerial"
// check performed by the caller, which matches the behavior operators
// expect from a zone that has been reloaded after a crash.
func (m *ZoneManager) generateSerialForZone(currentSerial uint32) uint32 {
	now := time.Now().UTC()
	base := uint32(now.Year()*10000+int(now.Month())*100+now.Day()) * 100

	// New day: start the counter at 1.
	if currentSerial < base {
		return base + 1
	}

	// Same day: increment past the current value. The caller's
	// "newSerial <= currentSerial" check ensures the value is strictly
	// greater, so we do not need a separate table to track the counter.
	return currentSerial + 1
}

// isValidZoneType checks if a zone type is valid.
func isValidZoneType(t string) bool {
	for _, vt := range ValidZoneTypes {
		if string(vt) == t {
			return true
		}
	}
	return false
}

// generateSerial generates a DNS SOA serial number in YYYYMMDDNN format.
// The counter is persisted in the dns_zones table: a new day always
// restarts at 1, and within a day the serial is always strictly greater
// than the current maximum already issued. This avoids the well-known
// issue where two updates within the same day would otherwise share the
// same serial (because the previous implementation simply returned the
// current day's base + 1 regardless of how many updates had already
// been issued).
func generateSerial(db *sql.DB) uint32 {
	now := time.Now().UTC()
	base := uint32(now.Year()*10000+int(now.Month())*100+now.Day()) * 100

	if db == nil {
		// No database available: fall back to the day-only form.
		return base + 1
	}

	// Look up the largest serial that has been issued for the current
	// day. If we are past the first update of a new day, the day base
	// itself will be greater than every stored serial, so the SELECT
	// returns 0 and we start the counter at 1.
	var maxSerial uint32
	row := db.QueryRow("SELECT COALESCE(MAX(serial), 0) FROM dns_zones WHERE serial >= ? AND serial < ?", base, base+100)
	if err := row.Scan(&maxSerial); err != nil {
		// On error, fall back to base + 1 to avoid handing out a serial
		// that is clearly out of the day's range.
		return base + 1
	}
	if maxSerial < base {
		return base + 1
	}
	return maxSerial + 1
}
