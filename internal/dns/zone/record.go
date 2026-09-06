package zone

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Supported record types.
var SupportedRecordTypes = map[string]bool{
	"A": true, "AAAA": true, "NS": true, "SOA": true, "CNAME": true,
	"DNAME": true, "MX": true, "TXT": true, "PTR": true, "SRV": true,
	"CAA": true, "TLSA": true, "SVCB": true, "HTTPS": true, "URI": true,
	"SSHFP": true, "NAPTR": true, "DS": true, "DNSKEY": true, "RRSIG": true,
	"NSEC": true, "NSEC3": true, "NSEC3PARAM": true, "APL": true, "RP": true,
	"HINFO": true, "LOC": true, "SPF": true,
}

// Record represents a DNS resource record.
type Record struct {
	ID        string     `json:"id"`
	ZoneID    string     `json:"zone_id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Value     string     `json:"value"`
	TTL       int        `json:"ttl"`
	Priority  int        `json:"priority,omitempty"`
	Weight    int        `json:"weight,omitempty"`
	Port      int        `json:"port,omitempty"`
	Tag       string     `json:"tag,omitempty"`
	Flag      int        `json:"flag,omitempty"`
	Enabled   bool       `json:"enabled"`
	Comment   string     `json:"comment,omitempty"`
	Tags      string     `json:"tags,omitempty"`
	Owner     string     `json:"owner,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// RecordOptions contains options for creating or updating a record.
type RecordOptions struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      *int   `json:"ttl,omitempty"`
	Priority *int   `json:"priority,omitempty"`
	Weight   *int   `json:"weight,omitempty"`
	Port     *int   `json:"port,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Flag     *int   `json:"flag,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Comment  string `json:"comment,omitempty"`
	Tags     string `json:"tags,omitempty"`
	Owner    string `json:"owner,omitempty"`
	// CreatePTR indicates that a PTR record should be auto-created for A/AAAA records.
	CreatePTR bool `json:"create_ptr,omitempty"`
	// ExpiresAt sets an expiry timestamp for record aging. When it passes,
	// the record stops answering queries and a background task deletes it.
	// nil leaves the value unchanged on update; ClearExpiresAt removes it.
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ClearExpiresAt bool       `json:"clear_expires_at,omitempty"`
}

// RecordFilter contains filter options for listing records.
type RecordFilter struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	ZoneID   string `json:"zone_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

// RecordManager provides CRUD operations for DNS records.
type RecordManager struct {
	db        *sql.DB
	zoneStore *Store
	zoneMgr   *ZoneManager
	// notifyHook is invoked (in a goroutine by the caller) with the zone
	// name whenever a primary zone's serial is bumped, so NOTIFY (RFC 1996)
	// announcements can be dispatched to configured secondaries.
	notifyHook func(zoneName string)
}

// SetNotifyHook registers the NOTIFY dispatch callback.
func (m *RecordManager) SetNotifyHook(fn func(zoneName string)) {
	m.notifyHook = fn
}

// notifyPrimary fires the notify hook for the given zone when it is a
// primary zone with a serial bump.
func (m *RecordManager) notifyPrimary(zoneID string) {
	if m.notifyHook == nil {
		return
	}
	zoneName := ""
	if z, err := m.zoneMgr.GetZone(zoneID); err == nil {
		if z.Type != string(ZoneTypePrimary) {
			return
		}
		zoneName = z.Name
	}
	if zoneName != "" {
		go m.notifyHook(zoneName)
	}
}

// NewRecordManager creates a new RecordManager.
func NewRecordManager(db *sql.DB, zoneStore *Store, zoneMgr *ZoneManager) *RecordManager {
	return &RecordManager{
		db:        db,
		zoneStore: zoneStore,
		zoneMgr:   zoneMgr,
	}
}

// CreateRecord creates a new DNS record.
func (m *RecordManager) CreateRecord(zoneID string, opts RecordOptions) (*Record, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone id is required")
	}
	if opts.Name == "" {
		return nil, fmt.Errorf("record name is required")
	}
	if opts.Type == "" {
		return nil, fmt.Errorf("record type is required")
	}
	if opts.Value == "" {
		return nil, fmt.Errorf("record value is required")
	}

	// Validate record type.
	if !SupportedRecordTypes[opts.Type] {
		return nil, fmt.Errorf("unsupported record type: %s", opts.Type)
	}

	// Validate record value.
	if err := validateRecordValue(opts.Type, opts.Value, opts.Priority, opts.Weight, opts.Port, opts.Tag, opts.Flag); err != nil {
		return nil, fmt.Errorf("invalid record value: %w", err)
	}

	// Get zone info for name normalization and default TTL.
	zone, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return nil, fmt.Errorf("zone not found: %w", err)
	}

	// Normalize record name.
	name := normalizeRecordName(opts.Name, zone.Name)

	// CNAME uniqueness: a name that has a CNAME record cannot have any other records.
	if opts.Type == "CNAME" {
		var count int
		err := m.db.QueryRow("SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND type != 'CNAME' AND enabled = 1", zoneID, name).Scan(&count)
		if err == nil && count > 0 {
			return nil, fmt.Errorf("CNAME conflict: name %s already has other record types", name)
		}
	} else {
		var count int
		err := m.db.QueryRow("SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND type = 'CNAME' AND enabled = 1", zoneID, name).Scan(&count)
		if err == nil && count > 0 {
			return nil, fmt.Errorf("CNAME conflict: name %s already has a CNAME record", name)
		}
	}

	// Set TTL.
	ttl := zone.DefaultTTL
	if opts.TTL != nil {
		ttl = *opts.TTL
	}

	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}

	id := uuid.New().String()
	_, err = m.db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, enabled, comment, tags, owner, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, zoneID, name, opts.Type, opts.Value, ttl,
		nullInt(opts.Priority), nullInt(opts.Weight), nullInt(opts.Port),
		nullInt(opts.Flag), enabled, opts.Comment, opts.Tags, opts.Owner, opts.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("inserting record: %w", err)
	}

	// Increment zone serial and record the change for IXFR.
	serial, _ := m.zoneMgr.IncrementSerial(zoneID)
	m.logChange(zoneID, serial, "add", name, opts.Type, opts.Value, ttl,
		intOrZero(opts.Priority), intOrZero(opts.Weight), intOrZero(opts.Port))

	// Auto-create PTR record if requested for A/AAAA.
	if opts.CreatePTR && (opts.Type == "A" || opts.Type == "AAAA") {
		if err := m.autoCreatePTR(zoneID, name, opts.Type, opts.Value, ttl); err != nil {
			// Log but don't fail the record creation.
			_ = err
		}
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return m.GetRecord(id)
}

// GetRecord retrieves a record by ID.
func (m *RecordManager) GetRecord(id string) (*Record, error) {
	if id == "" {
		return nil, fmt.Errorf("record id is required")
	}

	var r Record
	var priority, weight, port, flag sql.NullInt64
	var comment, tags, owner sql.NullString
	var expiresAt sql.NullTime

	err := m.db.QueryRow(`
		SELECT id, zone_id, name, type, value, ttl, priority, weight, port, flag,
			enabled, comment, tags, owner, expires_at, created_at, updated_at
		FROM dns_records WHERE id = ?
	`, id).Scan(
		&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.Value, &r.TTL,
		&priority, &weight, &port, &flag, &r.Enabled,
		&comment, &tags, &owner, &expiresAt,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("record not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying record: %w", err)
	}

	if priority.Valid {
		r.Priority = int(priority.Int64)
	}
	if weight.Valid {
		r.Weight = int(weight.Int64)
	}
	if port.Valid {
		r.Port = int(port.Int64)
	}
	if flag.Valid {
		r.Flag = int(flag.Int64)
	}
	if comment.Valid {
		r.Comment = comment.String
	}
	if tags.Valid {
		r.Tags = tags.String
	}
	if owner.Valid {
		r.Owner = owner.String
	}
	if expiresAt.Valid {
		r.ExpiresAt = &expiresAt.Time
	}

	return &r, nil
}

// ListRecords returns a paginated list of records.
func (m *RecordManager) ListRecords(filter RecordFilter) ([]Record, int64, error) {
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

	if filter.ZoneID != "" {
		conditions = append(conditions, "zone_id = ?")
		args = append(args, filter.ZoneID)
	}
	if filter.Name != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}
	if filter.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, filter.Type)
	}
	if filter.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, *filter.Enabled)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total.
	var total int64
	countSQL := "SELECT COUNT(*) FROM dns_records " + whereClause
	err := m.db.QueryRow(countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting records: %w", err)
	}

	// Query page.
	offset := (filter.Page - 1) * filter.PageSize
	querySQL := fmt.Sprintf(`
		SELECT id, zone_id, name, type, value, ttl, priority, weight, port, flag,
			enabled, comment, tags, owner, expires_at, created_at, updated_at
		FROM dns_records %s
		ORDER BY name, type
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs := append(args, filter.PageSize, offset)
	rows, err := m.db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying records: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		var priority, weight, port, flag sql.NullInt64
		var comment, tags, owner sql.NullString
		var expiresAt sql.NullTime
		if err := rows.Scan(
			&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.Value, &r.TTL,
			&priority, &weight, &port, &flag, &r.Enabled,
			&comment, &tags, &owner, &expiresAt,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			continue
		}
		if priority.Valid {
			r.Priority = int(priority.Int64)
		}
		if weight.Valid {
			r.Weight = int(weight.Int64)
		}
		if port.Valid {
			r.Port = int(port.Int64)
		}
		if flag.Valid {
			r.Flag = int(flag.Int64)
		}
		if comment.Valid {
			r.Comment = comment.String
		}
		if tags.Valid {
			r.Tags = tags.String
		}
		if owner.Valid {
			r.Owner = owner.String
		}
		if expiresAt.Valid {
			r.ExpiresAt = &expiresAt.Time
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating records: %w", err)
	}

	return records, total, nil
}

// UpdateRecord updates an existing record.
func (m *RecordManager) UpdateRecord(id string, opts RecordOptions) (*Record, error) {
	if id == "" {
		return nil, fmt.Errorf("record id is required")
	}

	// Verify record exists.
	existing, err := m.GetRecord(id)
	if err != nil {
		return nil, err
	}

	// Build update query dynamically.
	var setClauses []string
	var args []interface{}

	if opts.Name != "" {
		// Get zone for name normalization.
		zone, zoneErr := m.zoneMgr.GetZone(existing.ZoneID)
		if zoneErr != nil {
			return nil, zoneErr
		}
		name := normalizeRecordName(opts.Name, zone.Name)
		setClauses = append(setClauses, "name = ?")
		args = append(args, name)
	}
	if opts.Type != "" {
		if !SupportedRecordTypes[opts.Type] {
			return nil, fmt.Errorf("unsupported record type: %s", opts.Type)
		}
		setClauses = append(setClauses, "type = ?")
		args = append(args, opts.Type)
	}
	if opts.Value != "" {
		priority := existing.Priority
		weight := existing.Weight
		port := existing.Port
		if opts.Priority != nil {
			priority = *opts.Priority
		}
		if opts.Weight != nil {
			weight = *opts.Weight
		}
		if opts.Port != nil {
			port = *opts.Port
		}
		recType := existing.Type
		if opts.Type != "" {
			recType = opts.Type
		}
		if err := validateRecordValue(recType, opts.Value, &priority, &weight, &port, opts.Tag, &existing.Flag); err != nil {
			return nil, fmt.Errorf("invalid record value: %w", err)
		}
		setClauses = append(setClauses, "value = ?")
		args = append(args, opts.Value)
	}
	if opts.TTL != nil {
		setClauses = append(setClauses, "ttl = ?")
		args = append(args, *opts.TTL)
	}
	if opts.Priority != nil {
		setClauses = append(setClauses, "priority = ?")
		args = append(args, *opts.Priority)
	}
	if opts.Weight != nil {
		setClauses = append(setClauses, "weight = ?")
		args = append(args, *opts.Weight)
	}
	if opts.Port != nil {
		setClauses = append(setClauses, "port = ?")
		args = append(args, *opts.Port)
	}
	if opts.Tag != "" {
		setClauses = append(setClauses, "tag = ?")
		args = append(args, opts.Tag)
	}
	if opts.Flag != nil {
		setClauses = append(setClauses, "flag = ?")
		args = append(args, *opts.Flag)
	}
	if opts.Enabled != nil {
		setClauses = append(setClauses, "enabled = ?")
		args = append(args, *opts.Enabled)
	}
	if opts.Comment != "" {
		setClauses = append(setClauses, "comment = ?")
		args = append(args, opts.Comment)
	}
	if opts.Tags != "" {
		setClauses = append(setClauses, "tags = ?")
		args = append(args, opts.Tags)
	}
	if opts.Owner != "" {
		setClauses = append(setClauses, "owner = ?")
		args = append(args, opts.Owner)
	}

	if len(setClauses) == 0 {
		return existing, nil
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := "UPDATE dns_records SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	_, err = m.db.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("updating record: %w", err)
	}

	// Increment zone serial.
	if _, err := m.zoneMgr.IncrementSerial(existing.ZoneID); err != nil {
		_ = err
	}

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return m.GetRecord(id)
}

// DeleteRecord deletes a record by ID.
func (m *RecordManager) DeleteRecord(id string) error {
	if id == "" {
		return fmt.Errorf("record id is required")
	}

	// Get record to know which zone to update serial for.
	record, err := m.GetRecord(id)
	if err != nil {
		return err
	}

	_, err = m.db.Exec("DELETE FROM dns_records WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting record: %w", err)
	}

	// Increment zone serial and record the change for IXFR.
	serial, _ := m.zoneMgr.IncrementSerial(record.ZoneID)
	m.logChange(record.ZoneID, serial, "delete", record.Name, record.Type,
		record.Value, record.TTL, record.Priority, record.Weight, record.Port)

	// Reload in-memory zone store.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

// BatchCreateRecords creates multiple records in a zone atomically. If any
// record in the batch fails to insert, the whole batch is rolled back; no
// partial state is left behind. Serial increments are batched into a single
// update at the end of the transaction.
func (m *RecordManager) BatchCreateRecords(zoneID string, records []RecordOptions) ([]Record, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone id is required")
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no records provided")
	}

	// Verify zone exists and read its default TTL up front so each record
	// can be normalized without holding the transaction open.
	zone, err := m.zoneMgr.GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin batch create transaction: %w", err)
	}
	defer tx.Rollback()

	created := make([]Record, 0, len(records))
	for _, opts := range records {
		// Inline the bulk of CreateRecord against the transaction so that
		// validation, normalization, and insert all run with the same
		// connection. The created-at/updated-at timestamps come from the
		// database so that the entire batch shares a consistent clock.
		rec, err := m.insertRecordTx(tx, zone, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create record %s %s: %w", opts.Name, opts.Type, err)
		}
		created = append(created, *rec)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit batch create transaction: %w", err)
	}

	// Reload in-memory zone store once for the whole batch.
	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return created, nil
}

// BatchDeleteRecords deletes multiple records by IDs atomically. The
// deletion and the corresponding serial increment happen in a single
// transaction so that a partial delete cannot be observed.
func (m *RecordManager) BatchDeleteRecords(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("no record ids provided")
	}

	// Pre-validate that every ID resolves to a real record before we open
	// a transaction. The validation reads use the connection pool and are
	// fast; doing them up front means the transaction itself only does
	// the actual deletes and the single serial bump.
	deleted := make([]Record, 0, len(ids))
	for _, id := range ids {
		rec, err := m.GetRecord(id)
		if err != nil {
			return fmt.Errorf("failed to delete record %s: %w", id, err)
		}
		deleted = append(deleted, *rec)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin batch delete transaction: %w", err)
	}
	defer tx.Rollback()

	for _, rec := range deleted {
		if _, err := tx.Exec("DELETE FROM dns_records WHERE id = ?", rec.ID); err != nil {
			return fmt.Errorf("deleting record %s: %w", rec.ID, err)
		}
	}

	// One serial bump per zone, regardless of how many records were
	// removed from that zone. This keeps the SOA counter advancing in
	// step with the public view of the zone and avoids the noise of
	// bumping the serial once per record.
	zoneBumped := make(map[string]struct{}, len(deleted))
	for _, rec := range deleted {
		if _, ok := zoneBumped[rec.ZoneID]; ok {
			continue
		}
		zoneBumped[rec.ZoneID] = struct{}{}

		var currentSerial uint32
		if err := tx.QueryRow("SELECT serial FROM dns_zones WHERE id = ?", rec.ZoneID).Scan(&currentSerial); err != nil {
			return fmt.Errorf("querying serial for zone %s: %w", rec.ZoneID, err)
		}
		newSerial := m.zoneMgr.generateSerialForZone(currentSerial)
		if newSerial <= currentSerial {
			newSerial = currentSerial + 1
		}
		if _, err := tx.Exec(
			"UPDATE dns_zones SET serial = ?, updated_at = datetime('now') WHERE id = ?",
			newSerial, rec.ZoneID,
		); err != nil {
			return fmt.Errorf("updating serial for zone %s: %w", rec.ZoneID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch delete transaction: %w", err)
	}

	// Record one change-history batch per zone for IXFR.
	zoneSerial := make(map[string]uint32, len(zoneBumped))
	for zoneID := range zoneBumped {
		serial, _ := m.zoneMgr.IncrementSerial(zoneID)
		zoneSerial[zoneID] = serial
	}
	for _, rec := range deleted {
		m.logChange(rec.ZoneID, zoneSerial[rec.ZoneID], "delete", rec.Name, rec.Type,
			rec.Value, rec.TTL, rec.Priority, rec.Weight, rec.Port)
	}

	if m.zoneStore != nil {
		m.zoneStore.Reload()
	}

	return nil
}

// insertRecordTx is the transactional variant of CreateRecord. It assumes
// the caller has already opened tx and is responsible for committing or
// rolling back. The caller is also responsible for the in-memory zone
// store reload.
func (m *RecordManager) insertRecordTx(tx *sql.Tx, zone *Zone, opts RecordOptions) (*Record, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("record name is required")
	}
	if opts.Type == "" {
		return nil, fmt.Errorf("record type is required")
	}
	if opts.Value == "" {
		return nil, fmt.Errorf("record value is required")
	}
	if !SupportedRecordTypes[opts.Type] {
		return nil, fmt.Errorf("unsupported record type: %s", opts.Type)
	}
	if err := validateRecordValue(opts.Type, opts.Value, opts.Priority, opts.Weight, opts.Port, opts.Tag, opts.Flag); err != nil {
		return nil, fmt.Errorf("invalid record value: %w", err)
	}

	name := normalizeRecordName(opts.Name, zone.Name)

	// CNAME uniqueness check, scoped to the same transaction so that a
	// concurrent CreateRecord (which would normally use m.db directly)
	// cannot race us between the read and the insert.
	if opts.Type == "CNAME" {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND type != 'CNAME' AND enabled = 1",
			zone.ID, name,
		).Scan(&count); err == nil && count > 0 {
			return nil, fmt.Errorf("CNAME conflict: name %s already has other record types", name)
		}
	} else {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM dns_records WHERE zone_id = ? AND name = ? AND type = 'CNAME' AND enabled = 1",
			zone.ID, name,
		).Scan(&count); err == nil && count > 0 {
			return nil, fmt.Errorf("CNAME conflict: name %s already has a CNAME record", name)
		}
	}

	ttl := zone.DefaultTTL
	if opts.TTL != nil {
		ttl = *opts.TTL
	}
	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}

	id := uuid.New().String()
	_, err := tx.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, enabled, comment, tags, owner, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, zone.ID, name, opts.Type, opts.Value, ttl,
		nullInt(opts.Priority), nullInt(opts.Weight), nullInt(opts.Port),
		nullInt(opts.Flag), enabled, opts.Comment, opts.Tags, opts.Owner, opts.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("inserting record: %w", err)
	}

	return &Record{
		ID:       id,
		ZoneID:   zone.ID,
		Name:     name,
		Type:     opts.Type,
		Value:    opts.Value,
		TTL:      ttl,
		Priority: intOrZero(opts.Priority),
		Weight:   intOrZero(opts.Weight),
		Port:     intOrZero(opts.Port),
		Tag:      opts.Tag,
		Flag:     intOrZero(opts.Flag),
		Enabled:  enabled,
		Comment:  opts.Comment,
		Tags:     opts.Tags,
		Owner:    opts.Owner,
	}, nil
}

// intOrZero dereferences an *int, returning 0 for nil. Used when building
// in-memory Record values from RecordOptions where the caller has supplied
// nil for unset fields.
func intOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// logChange records one RR mutation into dns_zone_changes for true IXFR
// (RFC 1995) incremental transfer. Change history failure is logged but
// never fails the mutation: IXFR clients fall back to AXFR when history
// is incomplete.
func (m *RecordManager) logChange(zoneID string, serial uint32, changeType, name, rtype, value string, ttl, priority, weight, port int) {
	if serial == 0 {
		return
	}
	_, err := m.db.Exec(`
		INSERT INTO dns_zone_changes (id, zone_id, serial, change_type, name, type, value, ttl, priority, weight, port)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, uuid.New().String(), zoneID, serial, changeType, name, rtype, value, ttl,
		nullInt(&priority), nullInt(&weight), nullInt(&port))
	if err != nil {
		slog.Warn("record: failed to write zone change history", "zone_id", zoneID, "error", err)
	}

	// NOTIFY secondaries about the serial bump (primary zones only; the
	// hook is a no-op when unset or the zone has no notify targets).
	m.notifyPrimary(zoneID)
}

// PruneZoneChangeHistory removes change rows older than the given number of
// days. IXFR clients older than the retained window fall back to AXFR.
func (m *RecordManager) PruneZoneChangeHistory(days int) {
	if days <= 0 {
		return
	}
	res, err := m.db.Exec(
		"DELETE FROM dns_zone_changes WHERE created_at < datetime('now', ?)",
		fmt.Sprintf("-%d days", days),
	)
	if err != nil {
		slog.Warn("record: failed to prune zone change history", "error", err)
		return
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		slog.Debug("record: pruned zone change history", "deleted", rows)
	}
}

// CleanupExpiredRecords deletes records whose expiry has passed and returns
// how many were removed. Intended to run periodically (record aging).
func (m *RecordManager) CleanupExpiredRecords() (int64, error) {
	res, err := m.db.Exec(
		"DELETE FROM dns_records WHERE expires_at IS NOT NULL AND expires_at <= datetime('now')",
	)
	if err != nil {
		return 0, fmt.Errorf("deleting expired records: %w", err)
	}
	deleted, _ := res.RowsAffected()
	if deleted > 0 {
		slog.Info("record: expired records cleaned up", "deleted", deleted)
	}
	return deleted, nil
}

// autoCreatePTR automatically creates a PTR record in the reverse zone for an A/AAAA record.
func (m *RecordManager) autoCreatePTR(zoneID string, name string, recordType string, value string, ttl int) error {
	var ptrName string
	var err error

	if recordType == "A" {
		ptrName, err = ipv4ToPTR(value)
		if err != nil {
			return err
		}
	} else {
		ptrName, err = ipv6ToPTR(value)
		if err != nil {
			return err
		}
	}

	// Find the reverse zone that contains this PTR name.
	reverseZones, _, err := m.zoneMgr.ListZones(ZoneFilter{Type: "reverse", PageSize: 100})
	if err != nil {
		return fmt.Errorf("finding reverse zones: %w", err)
	}

	for _, rz := range reverseZones {
		if strings.HasSuffix(ptrName, rz.Name) {
			// Found the reverse zone. Create PTR record.
			relativeName := strings.TrimSuffix(ptrName, rz.Name)
			if relativeName == "" {
				relativeName = "@"
			}

			id := uuid.New().String()
			_, err = m.db.Exec(`
				INSERT OR IGNORE INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
				VALUES (?, ?, ?, 'PTR', ?, ?, 1)
			`, id, rz.ID, relativeName, name, ttl)
			if err != nil {
				return fmt.Errorf("creating PTR record: %w", err)
			}

			// Increment reverse zone serial.
			if _, err := m.zoneMgr.IncrementSerial(rz.ID); err != nil {
				_ = err
			}

			return nil
		}
	}

	// No reverse zone found; skip PTR creation silently.
	return nil
}

// ipv4ToPTR converts an IPv4 address to a PTR record name.
func ipv4ToPTR(ipStr string) (string, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", ipStr)
	}
	ip = ip.To4()
	if ip == nil {
		return "", fmt.Errorf("not an IPv4 address: %s", ipStr)
	}
	// Reverse octets.
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.", ip[3], ip[2], ip[1], ip[0]), nil
}

// ipv6ToPTR converts an IPv6 address to a PTR record name.
func ipv6ToPTR(ipStr string) (string, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", ipStr)
	}
	ip = ip.To16()
	if ip == nil {
		return "", fmt.Errorf("not an IPv6 address: %s", ipStr)
	}
	// Expand to full hex nibbles, reversed.
	var nibbles []string
	for i := 15; i >= 0; i-- {
		nibbles = append(nibbles,
			fmt.Sprintf("%x", ip[i]&0x0f),
			fmt.Sprintf("%x", (ip[i]>>4)&0x0f),
		)
	}
	return strings.Join(nibbles, ".") + ".ip6.arpa.", nil
}

// normalizeRecordName normalizes a record name relative to a zone.
func normalizeRecordName(name, zoneName string) string {
	if name == "" || name == "@" {
		return zoneName
	}
	// If the name is already an FQDN matching the zone, return as-is.
	if strings.EqualFold(name, zoneName) || strings.EqualFold(name+".", zoneName) {
		return zoneName
	}
	// If the name ends with the zone name, it's already fully qualified.
	if strings.HasSuffix(strings.ToLower(name), strings.ToLower(zoneName)) {
		if !strings.HasSuffix(name, ".") {
			return name + "."
		}
		return name
	}
	// Relative name: append zone name.
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "." + zoneName
}

// validateRecordValue validates the value for a given record type.
func validateRecordValue(rtype, value string, priority, weight, port *int, tag string, flag *int) error {
	switch rtype {
	case "A":
		if ip := net.ParseIP(value); ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid IPv4 address: %s", value)
		}
	case "AAAA":
		if ip := net.ParseIP(value); ip == nil || ip.To16() == nil || ip.To4() != nil {
			return fmt.Errorf("invalid IPv6 address: %s", value)
		}
	case "MX":
		if priority == nil || *priority < 0 || *priority > 65535 {
			return fmt.Errorf("MX record requires a valid priority (0-65535)")
		}
	case "SRV":
		if priority == nil || *priority < 0 || *priority > 65535 {
			return fmt.Errorf("SRV record requires a valid priority (0-65535)")
		}
		if weight == nil {
			return fmt.Errorf("SRV record requires weight")
		}
		if port == nil || *port < 0 || *port > 65535 {
			return fmt.Errorf("SRV record requires a valid port (0-65535)")
		}
	case "CAA":
		if flag == nil {
			return fmt.Errorf("CAA record requires a flag")
		}
		if tag == "" {
			return fmt.Errorf("CAA record requires a tag")
		}
		if tag != "issue" && tag != "issuewild" && tag != "iodef" {
			return fmt.Errorf("CAA tag must be issue, issuewild, or iodef")
		}
	case "CNAME":
		if value == "" {
			return fmt.Errorf("CNAME target cannot be empty")
		}
	case "NS":
		if value == "" {
			return fmt.Errorf("NS target cannot be empty")
		}
	case "PTR":
		if value == "" {
			return fmt.Errorf("PTR target cannot be empty")
		}
	}
	return nil
}

// nullInt returns a nullable int value.
func nullInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}
