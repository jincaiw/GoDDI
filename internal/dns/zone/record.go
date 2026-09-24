package zone

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/miekg/dns"
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
	// Overwrite replaces any existing record with the same name and type in
	// the zone instead of adding a second one (Technitium parity).
	Overwrite bool `json:"overwrite,omitempty"`
	// ExpiryTTL sets the record to expire this many seconds from now
	// (Technitium expiryTtl parity). Ignored when ExpiresAt is set.
	ExpiryTTL *int `json:"expiry_ttl,omitempty"`
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

// NotifyPrimaryZone announces a committed change made outside RecordManager.
// The caller must invoke it only after the zone's serial and data commit.
func (m *RecordManager) NotifyPrimaryZone(zoneID string) {
	m.notifyPrimary(zoneID)
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

	// Set TTL.
	ttl := zone.DefaultTTL
	if opts.TTL != nil {
		ttl = *opts.TTL
	}
	if err := validateRecordTTL(ttl); err != nil {
		return nil, err
	}

	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}

	// Resolve record expiry (Technitium expiryTtl parity): when set, the
	// record ages out expiryTtl seconds from now.
	expiresAt := opts.ExpiresAt
	if expiresAt == nil && opts.ExpiryTTL != nil && *opts.ExpiryTTL > 0 {
		t := time.Now().Add(time.Duration(*opts.ExpiryTTL) * time.Second)
		expiresAt = &t
	}

	// Resolve the optional reverse-zone target before opening the write
	// transaction. The database uses a single SQLite connection, so querying
	// the zone manager from inside the transaction would deadlock.
	var ptrZone *Zone
	var ptrOwner string
	if opts.CreatePTR && (opts.Type == "A" || opts.Type == "AAAA") {
		ptrZone, ptrOwner, err = m.findReverseZone(opts.Type, opts.Value)
		if err != nil {
			return nil, fmt.Errorf("finding reverse zone for automatic PTR: %w", err)
		}
	}

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin record create transaction: %w", err)
	}
	defer tx.Rollback()
	if enabled {
		if err := validateCNAMEExclusivityTx(tx, zoneID, name, opts.Type, opts.Value, ""); err != nil {
			return nil, err
		}
	}

	// Overwrite and its deletions are part of the same zone version as the add.
	var replaced []Record
	if opts.Overwrite {
		rows, queryErr := tx.Query(`SELECT id, zone_id, name, type, value, ttl,
			COALESCE(priority, 0), COALESCE(weight, 0), COALESCE(port, 0),
			COALESCE(flag, 0), COALESCE(tag, '')
			FROM dns_records WHERE zone_id = ? AND name = ? AND type = ?`, zoneID, name, opts.Type)
		if queryErr != nil {
			return nil, fmt.Errorf("querying overwritten records: %w", queryErr)
		}
		for rows.Next() {
			var rec Record
			if scanErr := rows.Scan(&rec.ID, &rec.ZoneID, &rec.Name, &rec.Type, &rec.Value, &rec.TTL,
				&rec.Priority, &rec.Weight, &rec.Port, &rec.Flag, &rec.Tag); scanErr != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning overwritten record: %w", scanErr)
			}
			replaced = append(replaced, rec)
		}
		if queryErr = rows.Err(); queryErr != nil {
			rows.Close()
			return nil, fmt.Errorf("iterating overwritten records: %w", queryErr)
		}
		if queryErr = rows.Close(); queryErr != nil {
			return nil, fmt.Errorf("closing overwritten records: %w", queryErr)
		}
		if _, queryErr = tx.Exec("DELETE FROM dns_records WHERE zone_id = ? AND name = ? AND type = ?", zoneID, name, opts.Type); queryErr != nil {
			return nil, fmt.Errorf("overwriting existing record: %w", queryErr)
		}
	}
	if enabled && !opts.Overwrite {
		if err := ValidateRRsetTTLTx(tx, zoneID, name, opts.Type, ttl, ""); err != nil {
			return nil, err
		}
	}

	id := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, tag, enabled, comment, tags, owner, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, zoneID, name, opts.Type, opts.Value, ttl,
		nullInt(opts.Priority), nullInt(opts.Weight), nullInt(opts.Port),
		nullInt(opts.Flag), opts.Tag, enabled, opts.Comment, opts.Tags, opts.Owner, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("inserting record: %w", err)
	}

	serial, err := bumpZoneSerialTx(tx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("bumping zone serial: %w", err)
	}
	for _, rec := range replaced {
		if err := logChangeTx(tx, rec.ZoneID, serial, "delete", rec.Name, rec.Type, rec.Value, rec.TTL, rec.Priority, rec.Weight, rec.Port, rec.Flag, rec.Tag); err != nil {
			return nil, fmt.Errorf("journaling overwritten record: %w", err)
		}
	}
	if err := logChangeTx(tx, zoneID, serial, "add", name, opts.Type, opts.Value, ttl,
		intOrZero(opts.Priority), intOrZero(opts.Weight), intOrZero(opts.Port), intOrZero(opts.Flag), opts.Tag); err != nil {
		return nil, fmt.Errorf("journaling created record: %w", err)
	}
	ptrCreated := false
	if ptrZone != nil {
		ptrCreated, err = m.createPTRTx(tx, ptrZone, ptrOwner, name, ttl)
		if err != nil {
			return nil, fmt.Errorf("creating automatic PTR record: %w", err)
		}
	}
	if err := logCurrentSOAStateTx(tx, zoneID, serial); err != nil {
		return nil, fmt.Errorf("journaling updated SOA: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit record create transaction: %w", err)
	}
	m.notifyPrimary(zoneID)
	if ptrCreated {
		m.notifyPrimary(ptrZone.ID)
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
	var tag, comment, tags, owner sql.NullString
	var expiresAt sql.NullTime

	err := m.db.QueryRow(`
	SELECT id, zone_id, name, type, value, ttl, priority, weight, port, flag, COALESCE(tag, ''),
			enabled, comment, tags, owner, expires_at, created_at, updated_at
		FROM dns_records WHERE id = ?
	`, id).Scan(
		&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.Value, &r.TTL,
		&priority, &weight, &port, &flag, &tag, &r.Enabled,
		&comment, &tags, &owner, &expiresAt,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("record not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying record: %w", err)
	}
	if r.Type == "NAPTR" {
		r.Value = normalizeNAPTRValue(r.Value)
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
	if tag.Valid {
		r.Tag = tag.String
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

func getRecordTx(tx *sql.Tx, id string) (*Record, error) {
	var r Record
	var priority, weight, port, flag sql.NullInt64
	var tag, comment, tags, owner sql.NullString
	var expiresAt sql.NullTime
	err := tx.QueryRow(`
	SELECT id, zone_id, name, type, value, ttl, priority, weight, port, flag, COALESCE(tag, ''),
			enabled, comment, tags, owner, expires_at, created_at, updated_at
		FROM dns_records WHERE id = ?
	`, id).Scan(&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.Value, &r.TTL,
		&priority, &weight, &port, &flag, &tag, &r.Enabled,
		&comment, &tags, &owner, &expiresAt, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("record not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying record: %w", err)
	}
	if r.Type == "NAPTR" {
		r.Value = normalizeNAPTRValue(r.Value)
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
	if tag.Valid {
		r.Tag = tag.String
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
		SELECT id, zone_id, name, type, value, ttl, priority, weight, port, flag, COALESCE(tag, ''),
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
		var tag, comment, tags, owner sql.NullString
		var expiresAt sql.NullTime
		if err := rows.Scan(
			&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.Value, &r.TTL,
			&priority, &weight, &port, &flag, &tag, &r.Enabled,
			&comment, &tags, &owner, &expiresAt,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			continue
		}
		if r.Type == "NAPTR" {
			r.Value = normalizeNAPTRValue(r.Value)
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
		if tag.Valid {
			r.Tag = tag.String
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

	if err := m.assertEditable(id); err != nil {
		return nil, err
	}

	// Verify record exists.
	existing, err := m.GetRecord(id)
	if err != nil {
		return nil, err
	}

	// Build update query dynamically.
	var setClauses []string
	var args []interface{}
	var normalizedName string

	if opts.Name != "" {
		// Get zone for name normalization.
		zone, zoneErr := m.zoneMgr.GetZone(existing.ZoneID)
		if zoneErr != nil {
			return nil, zoneErr
		}
		name := normalizeRecordName(opts.Name, zone.Name)
		normalizedName = name
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
		tag := existing.Tag
		if opts.Tag != "" {
			tag = opts.Tag
		}
		flag := existing.Flag
		if opts.Flag != nil {
			flag = *opts.Flag
		}
		if err := validateRecordValue(recType, opts.Value, &priority, &weight, &port, tag, &flag); err != nil {
			return nil, fmt.Errorf("invalid record value: %w", err)
		}
		setClauses = append(setClauses, "value = ?")
		args = append(args, opts.Value)
	}
	if opts.TTL != nil {
		if err := validateRecordTTL(*opts.TTL); err != nil {
			return nil, err
		}
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
	// Record aging on update (Technitium parity).
	if opts.ExpiryTTL != nil && opts.ExpiresAt == nil {
		if *opts.ExpiryTTL > 0 {
			t := time.Now().Add(time.Duration(*opts.ExpiryTTL) * time.Second)
			setClauses = append(setClauses, "expires_at = ?")
			args = append(args, t)
		}
	}
	if opts.ExpiresAt != nil {
		setClauses = append(setClauses, "expires_at = ?")
		args = append(args, *opts.ExpiresAt)
	}
	if opts.ClearExpiresAt {
		setClauses = append(setClauses, "expires_at = NULL")
	}

	if len(setClauses) == 0 {
		return existing, nil
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := "UPDATE dns_records SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin record update transaction: %w", err)
	}
	defer tx.Rollback()
	current, err := getRecordTx(tx, id)
	if err != nil {
		return nil, err
	}
	existing = current
	if opts.Value != "" {
		effectiveType := existing.Type
		if opts.Type != "" {
			effectiveType = opts.Type
		}
		priority, weight, port := existing.Priority, existing.Weight, existing.Port
		if opts.Priority != nil {
			priority = *opts.Priority
		}
		if opts.Weight != nil {
			weight = *opts.Weight
		}
		if opts.Port != nil {
			port = *opts.Port
		}
		tag := existing.Tag
		if opts.Tag != "" {
			tag = opts.Tag
		}
		flag := existing.Flag
		if opts.Flag != nil {
			flag = *opts.Flag
		}
		if err := validateRecordValue(effectiveType, opts.Value, &priority, &weight, &port, tag, &flag); err != nil {
			return nil, fmt.Errorf("invalid record value: %w", err)
		}
	}
	updatedName := existing.Name
	if opts.Name != "" {
		updatedName = normalizedName
	}
	updatedType := existing.Type
	if opts.Type != "" {
		updatedType = opts.Type
	}
	updatedTTL := existing.TTL
	if opts.TTL != nil {
		updatedTTL = *opts.TTL
	}
	updatedEnabled := existing.Enabled
	if opts.Enabled != nil {
		updatedEnabled = *opts.Enabled
	}
	updatedValue := existing.Value
	if opts.Value != "" {
		updatedValue = opts.Value
	}
	if updatedEnabled && (updatedEnabled != existing.Enabled || !strings.EqualFold(updatedName, existing.Name) ||
		!strings.EqualFold(updatedType, existing.Type) || updatedValue != existing.Value) {
		if err := validateCNAMEExclusivityTx(tx, existing.ZoneID, updatedName, updatedType, updatedValue, id); err != nil {
			return nil, err
		}
	}
	if updatedEnabled && (updatedEnabled != existing.Enabled || !strings.EqualFold(updatedName, existing.Name) ||
		!strings.EqualFold(updatedType, existing.Type) || updatedTTL != existing.TTL) {
		if err := ValidateRRsetTTLTx(tx, existing.ZoneID, updatedName, updatedType, updatedTTL, id); err != nil {
			return nil, err
		}
	}
	result, err := tx.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("updating record: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return nil, fmt.Errorf("checking updated record: %w", affectedErr)
		}
		return nil, fmt.Errorf("record changed concurrently or no longer exists: %s", id)
	}
	updated := *existing
	if opts.Name != "" {
		updated.Name = normalizedName
	}
	if opts.Type != "" {
		updated.Type = opts.Type
	}
	if opts.Value != "" {
		updated.Value = opts.Value
	}
	if opts.TTL != nil {
		updated.TTL = *opts.TTL
	}
	if opts.Priority != nil {
		updated.Priority = *opts.Priority
	}
	if opts.Weight != nil {
		updated.Weight = *opts.Weight
	}
	if opts.Port != nil {
		updated.Port = *opts.Port
	}
	if opts.Tag != "" {
		updated.Tag = opts.Tag
	}
	if opts.Flag != nil {
		updated.Flag = *opts.Flag
	}
	if opts.Enabled != nil {
		updated.Enabled = *opts.Enabled
	}
	serial, err := bumpZoneSerialTx(tx, existing.ZoneID)
	if err != nil {
		return nil, fmt.Errorf("bumping zone serial: %w", err)
	}
	if err := logChangeTx(tx, existing.ZoneID, serial, "delete", existing.Name, existing.Type, existing.Value,
		existing.TTL, existing.Priority, existing.Weight, existing.Port, existing.Flag, existing.Tag); err != nil {
		return nil, fmt.Errorf("journaling prior record: %w", err)
	}
	if err := logChangeTx(tx, updated.ZoneID, serial, "add", updated.Name, updated.Type, updated.Value,
		updated.TTL, updated.Priority, updated.Weight, updated.Port, updated.Flag, updated.Tag); err != nil {
		return nil, fmt.Errorf("journaling updated record: %w", err)
	}
	if err := logCurrentSOAStateTx(tx, existing.ZoneID, serial); err != nil {
		return nil, fmt.Errorf("journaling updated SOA: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit record update transaction: %w", err)
	}

	m.notifyPrimary(existing.ZoneID)

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

	if err := m.assertEditable(id); err != nil {
		return err
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin record delete transaction: %w", err)
	}
	defer tx.Rollback()
	record, err := getRecordTx(tx, id)
	if err != nil {
		return err
	}
	result, err := tx.Exec("DELETE FROM dns_records WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting record: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return fmt.Errorf("checking deleted record: %w", affectedErr)
		}
		return fmt.Errorf("record changed concurrently or no longer exists: %s", id)
	}

	serial, err := bumpZoneSerialTx(tx, record.ZoneID)
	if err != nil {
		return fmt.Errorf("bumping zone serial: %w", err)
	}
	if err := logChangeTx(tx, record.ZoneID, serial, "delete", record.Name, record.Type,
		record.Value, record.TTL, record.Priority, record.Weight, record.Port, record.Flag, record.Tag); err != nil {
		return fmt.Errorf("journaling deleted record: %w", err)
	}
	if err := logCurrentSOAStateTx(tx, record.ZoneID, serial); err != nil {
		return fmt.Errorf("journaling updated SOA: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit record delete transaction: %w", err)
	}
	m.notifyPrimary(record.ZoneID)

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
	serial, err := bumpZoneSerialTx(tx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("bumping zone serial: %w", err)
	}
	for _, rec := range created {
		if err := logChangeTx(tx, rec.ZoneID, serial, "add", rec.Name, rec.Type, rec.Value,
			rec.TTL, rec.Priority, rec.Weight, rec.Port, rec.Flag, rec.Tag); err != nil {
			return nil, fmt.Errorf("journaling created record: %w", err)
		}
	}
	if err := logCurrentSOAStateTx(tx, zoneID, serial); err != nil {
		return nil, fmt.Errorf("journaling updated SOA: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit batch create transaction: %w", err)
	}
	m.notifyPrimary(zoneID)

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
		if err := m.assertEditable(id); err != nil {
			return err
		}
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

	for i := range deleted {
		rec, currentErr := getRecordTx(tx, deleted[i].ID)
		if currentErr != nil {
			return fmt.Errorf("reading record %s in delete transaction: %w", deleted[i].ID, currentErr)
		}
		deleted[i] = *rec
		rec = &deleted[i]
		result, err := tx.Exec("DELETE FROM dns_records WHERE id = ?", rec.ID)
		if err != nil {
			return fmt.Errorf("deleting record %s: %w", rec.ID, err)
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
			if affectedErr != nil {
				return fmt.Errorf("checking deletion of record %s: %w", rec.ID, affectedErr)
			}
			return fmt.Errorf("record changed concurrently or no longer exists: %s", rec.ID)
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

		beforeSOA, err := ReadSOAHistoryStateTx(tx, rec.ZoneID)
		if err != nil {
			return fmt.Errorf("querying SOA for zone %s: %w", rec.ZoneID, err)
		}
		currentSerial := beforeSOA.Serial
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
		if err := LogSOARecordTx(tx, rec.ZoneID, newSerial, "delete", beforeSOA); err != nil {
			return fmt.Errorf("journaling prior SOA for zone %s: %w", rec.ZoneID, err)
		}
		for _, deletedRec := range deleted {
			if deletedRec.ZoneID != rec.ZoneID {
				continue
			}
			if err := logChangeTx(tx, deletedRec.ZoneID, newSerial, "delete", deletedRec.Name, deletedRec.Type,
				deletedRec.Value, deletedRec.TTL, deletedRec.Priority, deletedRec.Weight, deletedRec.Port, deletedRec.Flag, deletedRec.Tag); err != nil {
				return fmt.Errorf("journaling deleted record %s: %w", deletedRec.ID, err)
			}
		}
		beforeSOA.Serial = newSerial
		if err := LogSOARecordTx(tx, rec.ZoneID, newSerial, "add", beforeSOA); err != nil {
			return fmt.Errorf("journaling updated SOA for zone %s: %w", rec.ZoneID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch delete transaction: %w", err)
	}

	for zoneID := range zoneBumped {
		m.notifyPrimary(zoneID)
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
	ttl := zone.DefaultTTL
	if opts.TTL != nil {
		ttl = *opts.TTL
	}
	if err := validateRecordTTL(ttl); err != nil {
		return nil, err
	}
	enabled := true
	if opts.Enabled != nil {
		enabled = *opts.Enabled
	}
	if enabled {
		if err := validateCNAMEExclusivityTx(tx, zone.ID, name, opts.Type, opts.Value, ""); err != nil {
			return nil, err
		}
	}
	if enabled {
		if err := ValidateRRsetTTLTx(tx, zone.ID, name, opts.Type, ttl, ""); err != nil {
			return nil, err
		}
	}

	id := uuid.New().String()
	_, err := tx.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, priority, weight, port, flag, tag, enabled, comment, tags, owner, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, zone.ID, name, opts.Type, opts.Value, ttl,
		nullInt(opts.Priority), nullInt(opts.Weight), nullInt(opts.Port),
		nullInt(opts.Flag), opts.Tag, enabled, opts.Comment, opts.Tags, opts.Owner, opts.ExpiresAt)
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

// ValidateRRsetTTLTx rejects a write that would give a live, enabled RRset
// more than one TTL. Call it in the same transaction as the record mutation.
func ValidateRRsetTTLTx(tx *sql.Tx, zoneID, name, rtype string, ttl int, excludeID string) error {
	query := `SELECT MIN(ttl) FROM dns_records
		WHERE zone_id = ? AND LOWER(RTRIM(name, '.')) = LOWER(RTRIM(?, '.')) AND UPPER(type) = UPPER(?) AND enabled = 1
		AND (expires_at IS NULL OR julianday(expires_at) > julianday('now')) AND ttl != ?`
	args := []any{zoneID, name, rtype, ttl}
	if excludeID != "" {
		query += " AND id != ?"
		args = append(args, excludeID)
	}
	var conflictingTTL sql.NullInt64
	if err := tx.QueryRow(query, args...).Scan(&conflictingTTL); err != nil {
		return fmt.Errorf("checking RRset TTL consistency: %w", err)
	}
	if conflictingTTL.Valid {
		return fmt.Errorf("RRset TTL conflict: %s records at %s use TTL %d; requested TTL is %d",
			strings.ToUpper(rtype), dns.Fqdn(name), conflictingTTL.Int64, ttl)
	}
	return nil
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

func validateRecordTTL(ttl int) error {
	if ttl < 0 || uint64(ttl) > 2147483647 {
		return fmt.Errorf("TTL must be between 0 and 2147483647 seconds")
	}
	return nil
}

func validateCNAMEExclusivityTx(tx *sql.Tx, zoneID, name, rtype, value, excludeID string) error {
	query := `SELECT COUNT(*) FROM dns_records
		WHERE zone_id = ? AND LOWER(RTRIM(name, '.')) = LOWER(RTRIM(?, '.')) AND enabled = 1
		AND (expires_at IS NULL OR julianday(expires_at) > julianday('now')) AND type = 'CNAME'
		AND (? = '' OR id != ?)`
	args := []any{zoneID, name, excludeID, excludeID}
	if rtype == "CNAME" {
		query = `SELECT COUNT(*) FROM dns_records
			WHERE zone_id = ? AND LOWER(RTRIM(name, '.')) = LOWER(RTRIM(?, '.')) AND enabled = 1
			AND (expires_at IS NULL OR julianday(expires_at) > julianday('now')) AND type != 'CNAME'
			AND (? = '' OR id != ?)`
	}
	var conflicts int
	if err := tx.QueryRow(query, args...).Scan(&conflicts); err != nil {
		return fmt.Errorf("checking CNAME exclusivity: %w", err)
	}
	if conflicts > 0 {
		return fmt.Errorf("CNAME conflict at %s", name)
	}
	if rtype != "CNAME" {
		return nil
	}
	query = `SELECT COUNT(*) FROM dns_records
		WHERE zone_id = ? AND LOWER(RTRIM(name, '.')) = LOWER(RTRIM(?, '.')) AND enabled = 1
		AND (expires_at IS NULL OR julianday(expires_at) > julianday('now'))
		AND type = 'CNAME' AND LOWER(value) != LOWER(?)`
	args = []any{zoneID, name, value}
	if excludeID != "" {
		query += ` AND id != ?`
		args = append(args, excludeID)
	}
	if err := tx.QueryRow(query, args...).Scan(&conflicts); err != nil {
		return fmt.Errorf("checking CNAME targets: %w", err)
	}
	if conflicts > 0 {
		return fmt.Errorf("multiple CNAME targets at %s", name)
	}
	return nil
}

// ValidateCNAMEExclusivityTx applies the same DNS owner-name CNAME invariant
// to transactional writers outside the zone RecordManager.
func ValidateCNAMEExclusivityTx(tx *sql.Tx, zoneID, name, rtype, value, excludeID string) error {
	return validateCNAMEExclusivityTx(tx, zoneID, name, rtype, value, excludeID)
}

// SOAHistoryState contains the SOA fields needed to preserve a synthesized
// zone-apex SOA across a metadata update.
type SOAHistoryState struct {
	Name, MName, RName string
	Serial             uint32
	TTL                int
	Refresh, Retry     int
	Expire, Minimum    int
}

func ReadSOAHistoryStateTx(tx *sql.Tx, zoneID string) (SOAHistoryState, error) {
	var state SOAHistoryState
	err := tx.QueryRow(`SELECT name, soa_mname, soa_rname, serial, default_ttl,
		refresh, retry, expire, minimum FROM dns_zones WHERE id = ?`, zoneID).Scan(
		&state.Name, &state.MName, &state.RName, &state.Serial, &state.TTL,
		&state.Refresh, &state.Retry, &state.Expire, &state.Minimum)
	return state, err
}

// LogSOARecordTx appends one synthesized SOA RR to a zone change sequence.
// Callers that journal a content mutation should write the old SOA delete,
// then RR deletes/adds, then the new SOA add, all at the new serial.
func LogSOARecordTx(tx *sql.Tx, zoneID string, serial uint32, changeType string, state SOAHistoryState) error {
	value := fmt.Sprintf("%s %s %d %d %d %d %d",
		dns.Fqdn(state.MName), dns.Fqdn(state.RName), state.Serial,
		state.Refresh, state.Retry, state.Expire, state.Minimum)
	return logChangeTx(tx, zoneID, serial, changeType, dns.Fqdn(state.Name), "SOA",
		value, state.TTL, 0, 0, 0, 0, "")
}

// LogSOAChangeTx records the prior and new synthesized SOA at one committed
// serial. It is for transactional writers outside ZoneManager; the change
// history is not sufficient to enable IXFR until every serial-changing writer
// emits ordered SOA delimiters and protocol tests validate the full sequence.
func LogSOAChangeTx(tx *sql.Tx, zoneID string, serial uint32, before, after SOAHistoryState) error {
	if err := LogSOARecordTx(tx, zoneID, serial, "delete", before); err != nil {
		return fmt.Errorf("journaling prior SOA: %w", err)
	}
	if err := LogSOARecordTx(tx, zoneID, serial, "add", after); err != nil {
		return fmt.Errorf("journaling updated SOA: %w", err)
	}
	return nil
}

// bumpZoneSerialTx advances a zone serial within the caller's mutation
// transaction, so records, SOA serial, and history either all commit or none do.
func bumpZoneSerialTx(tx *sql.Tx, zoneID string) (uint32, error) {
	before, err := ReadSOAHistoryStateTx(tx, zoneID)
	if err != nil {
		return 0, fmt.Errorf("querying serial: %w", err)
	}
	return bumpZoneSerialWithSOAStateTx(tx, zoneID, before)
}

func bumpZoneSerialWithSOAStateTx(tx *sql.Tx, zoneID string, before SOAHistoryState) (uint32, error) {
	current := before.Serial
	next := NextSerial(current)
	result, err := tx.Exec("UPDATE dns_zones SET serial = ?, updated_at = datetime('now') WHERE id = ? AND serial = ?", next, zoneID, current)
	if err != nil {
		return 0, fmt.Errorf("updating serial: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking serial update: %w", err)
	}
	if rows != 1 {
		return 0, fmt.Errorf("serial changed concurrently")
	}
	if err := LogSOARecordTx(tx, zoneID, next, "delete", before); err != nil {
		return 0, fmt.Errorf("journaling prior SOA: %w", err)
	}
	return next, nil
}

func logCurrentSOAStateTx(tx *sql.Tx, zoneID string, serial uint32) error {
	state, err := ReadSOAHistoryStateTx(tx, zoneID)
	if err != nil {
		return err
	}
	state.Serial = serial
	return LogSOARecordTx(tx, zoneID, serial, "add", state)
}

func logChangeTx(tx *sql.Tx, zoneID string, serial uint32, changeType, name, rtype, value string, ttl, priority, weight, port, flag int, tag string) error {
	if serial == 0 {
		return fmt.Errorf("zone serial must be nonzero")
	}
	_, err := tx.Exec(`
		INSERT INTO dns_zone_changes (id, zone_id, serial, change_type, name, type, value, ttl, priority, weight, port, flag, tag)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, uuid.New().String(), zoneID, serial, changeType, name, rtype, value, ttl,
		nullInt(&priority), nullInt(&weight), nullInt(&port), nullInt(&flag), tag)
	return err
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
	tx, err := m.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin expired record cleanup transaction: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, zone_id, name, type, value, ttl,
		COALESCE(priority, 0), COALESCE(weight, 0), COALESCE(port, 0)
		FROM dns_records WHERE expires_at IS NOT NULL AND expires_at <= datetime('now')`)
	if err != nil {
		return 0, fmt.Errorf("querying expired records: %w", err)
	}
	type expiredRecord struct {
		id string
		Record
	}
	var expired []expiredRecord
	for rows.Next() {
		var rec expiredRecord
		if err := rows.Scan(&rec.id, &rec.ZoneID, &rec.Name, &rec.Type, &rec.Value, &rec.TTL,
			&rec.Priority, &rec.Weight, &rec.Port); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning expired record: %w", err)
		}
		expired = append(expired, rec)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterating expired records: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("closing expired records: %w", err)
	}
	if len(expired) == 0 {
		return 0, nil
	}
	for _, rec := range expired {
		if _, err := tx.Exec("DELETE FROM dns_records WHERE id = ?", rec.id); err != nil {
			return 0, fmt.Errorf("deleting expired record %s: %w", rec.id, err)
		}
	}
	zoneSerial := make(map[string]uint32)
	for _, rec := range expired {
		serial, ok := zoneSerial[rec.ZoneID]
		if !ok {
			serial, err = bumpZoneSerialTx(tx, rec.ZoneID)
			if err != nil {
				return 0, fmt.Errorf("bumping zone serial for expired records: %w", err)
			}
			zoneSerial[rec.ZoneID] = serial
		}
		if err := logChangeTx(tx, rec.ZoneID, serial, "delete", rec.Name, rec.Type, rec.Value,
			rec.TTL, rec.Priority, rec.Weight, rec.Port, rec.Flag, rec.Tag); err != nil {
			return 0, fmt.Errorf("journaling expired record %s: %w", rec.id, err)
		}
	}
	for zoneID, serial := range zoneSerial {
		if err := logCurrentSOAStateTx(tx, zoneID, serial); err != nil {
			return 0, fmt.Errorf("journaling updated SOA for expired records: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit expired record cleanup: %w", err)
	}
	for zoneID := range zoneSerial {
		m.notifyPrimary(zoneID)
	}
	deleted := int64(len(expired))
	slog.Info("record: expired records cleaned up", "deleted", deleted)
	return deleted, nil
}

// findReverseZone resolves the reverse zone and relative owner for an address.
// It runs before the caller's write transaction to avoid a nested query on
// SQLite's single connection.
func (m *RecordManager) findReverseZone(recordType, value string) (*Zone, string, error) {
	var ptrName string
	var err error

	switch recordType {
	case "A":
		ptrName, err = ipv4ToPTR(value)
	case "AAAA":
		ptrName, err = ipv6ToPTR(value)
	default:
		return nil, "", fmt.Errorf("PTR creation is unsupported for record type %s", recordType)
	}
	if err != nil {
		return nil, "", err
	}

	// Find the reverse zone that contains this PTR name.
	reverseZones, _, err := m.zoneMgr.ListZones(ZoneFilter{Type: "reverse", PageSize: 100})
	if err != nil {
		return nil, "", fmt.Errorf("listing reverse zones: %w", err)
	}

	for _, rz := range reverseZones {
		if strings.HasSuffix(strings.ToLower(ptrName), strings.ToLower(rz.Name)) {
			relativeName := ptrName[:len(ptrName)-len(rz.Name)]
			if relativeName == "" {
				relativeName = "@"
			}
			return &rz, relativeName, nil
		}
	}

	// No reverse zone is configured for this address.
	return nil, "", nil
}

// createPTRTx inserts a PTR and journals its serial in the caller's
// transaction. It reports whether a new PTR was created.
func (m *RecordManager) createPTRTx(tx *sql.Tx, reverseZone *Zone, owner, target string, ttl int) (bool, error) {
	owner = normalizeRecordName(strings.TrimSuffix(owner, "."), reverseZone.Name)
	if err := validateCNAMEExclusivityTx(tx, reverseZone.ID, owner, "PTR", target, ""); err != nil {
		return false, fmt.Errorf("automatic PTR conflicts with CNAME: %w", err)
	}
	if err := ValidateRRsetTTLTx(tx, reverseZone.ID, owner, "PTR", ttl, ""); err != nil {
		return false, err
	}
	res, err := tx.Exec(`
		INSERT OR IGNORE INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES (?, ?, ?, 'PTR', ?, ?, 1)
	`, uuid.New().String(), reverseZone.ID, owner, target, ttl)
	if err != nil {
		return false, fmt.Errorf("inserting PTR record: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking PTR insertion: %w", err)
	}
	if affected == 0 {
		return false, nil
	}
	serial, err := bumpZoneSerialTx(tx, reverseZone.ID)
	if err != nil {
		return false, fmt.Errorf("bumping reverse zone serial: %w", err)
	}
	if err := logChangeTx(tx, reverseZone.ID, serial, "add", owner, "PTR", target, ttl, 0, 0, 0, 0, ""); err != nil {
		return false, fmt.Errorf("journaling PTR record: %w", err)
	}
	if err := logCurrentSOAStateTx(tx, reverseZone.ID, serial); err != nil {
		return false, fmt.Errorf("journaling updated reverse SOA: %w", err)
	}
	return true, nil
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
//
// The stored zone name is an FQDN (trailing dot), so all comparisons strip the
// trailing dot first; otherwise a fully qualified input such as
// "www.example.com." for zone "example.com." would fail the suffix test and be
// treated as a relative name, producing "www.example.com.example.com.".
func normalizeRecordName(name, zoneName string) string {
	if name == "" || name == "@" {
		return zoneName
	}
	zone := strings.TrimSuffix(strings.ToLower(zoneName), ".")
	// Compare without a single trailing dot, but remember whether the caller
	// supplied an absolute name so the stored value stays absolute.
	trimmed := strings.TrimSuffix(name, ".")
	lower := strings.ToLower(trimmed)
	if lower == zone {
		return zoneName
	}
	// Already fully qualified inside this zone (e.g. "www.example.com").
	if strings.HasSuffix(lower, "."+zone) {
		if strings.HasSuffix(name, ".") {
			return name
		}
		return name + "."
	}
	// Absolute name outside of this zone is stored verbatim.
	if strings.HasSuffix(name, ".") {
		return name
	}
	// Relative name: append zone name.
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
		if *flag < 0 || *flag > 255 {
			return fmt.Errorf("CAA flag must be between 0 and 255")
		}
		if tag == "" {
			return fmt.Errorf("CAA record requires a tag")
		}
		if tag != "issue" && tag != "issuewild" && tag != "iodef" {
			return fmt.Errorf("CAA tag must be issue, issuewild, or iodef")
		}
	case "NAPTR":
		if priority == nil || *priority < 0 || *priority > 65535 {
			return fmt.Errorf("NAPTR record requires a valid order (0-65535)")
		}
		if weight == nil || *weight < 0 || *weight > 65535 {
			return fmt.Errorf("NAPTR record requires a valid preference (0-65535)")
		}
		if _, err := dns.NewRR("naptr.invalid. 0 IN NAPTR " + strconv.Itoa(*priority) + " " + strconv.Itoa(*weight) + " " + value); err != nil {
			return fmt.Errorf("invalid NAPTR RDATA: %w", err)
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

func normalizeNAPTRValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, `"`) {
		return value
	}
	// Older releases stored only the replacement name. Their other RDATA
	// strings were already emitted as empty, so make that legacy wire form
	// explicit while preserving the behavior of existing zones.
	return `"" "" "" ` + value
}

// nullInt returns a nullable int value.
func nullInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// NormalizeRecordName turns a record name into the form this package stores.
//
// It is exported so that another writer of dns_records -- the configuration
// publishing path is one -- cannot invent its own idea of what "www" means.
// Two writers with two normalisations produce two rows that answer the same
// query, and the resolver then has to pick one.
func NormalizeRecordName(name, zoneName string) string {
	return normalizeRecordName(name, zoneName)
}

// ValidateRecordValue applies the per-type value rules this package enforces
// when a record is created.
//
// Exported for the same reason: a second writer that skips these checks can
// store a value the API would have refused, and an unusable value does not fail
// where it is written -- it fails in the resolver, as a record that exists,
// lists, and answers nothing.
func ValidateRecordValue(rtype, value string, priority, weight, port *int, tag string, flag *int) error {
	return validateRecordValue(rtype, value, priority, weight, port, tag, flag)
}
