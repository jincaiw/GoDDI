package configver

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jasonwa/goddi/internal/dns/zone"
)

// DNSRecordsSchema is the version of DNSRecordsContent's shape.
//
// It is stored inside the content rather than beside it, because the content is
// what a diff compares. A later version that adds, renames or reorders fields
// changes the meaning of every stored revision of this type, and a diff between
// a revision of the old shape and one of the new would report the whole record
// set as changed. With the version inside the content that comparison shows one
// extra field -- `schema: 1 -> 2` -- which is the honest reading: the shape
// changed, so read the two sides rather than the field diff.
const DNSRecordsSchema = 1

// DNSRecordContent is one record as configuration.
//
// It is deliberately not zone.Record: that struct carries created_at and
// updated_at, which change on every write, so a publish built from it would
// never be byte-identical to the revision it came from and a rollback would
// look like a change of every record. `id` is carried, though, because it is
// what makes a rollback restore the same rows rather than mint new identities
// that nothing else in the database refers to.
type DNSRecordContent struct {
	// ID is the row's identity. Empty means "this record is new"; the adapter
	// generates one. Supplying an existing id updates that row in place.
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	TTL     int    `json:"ttl"`
	Enabled bool   `json:"enabled"`

	// The optional fields are pointers so that "not set" and "set to zero"
	// stay distinguishable on the way back out: MX with priority 0 is legal,
	// and a record with no priority at all is a different record.
	Priority *int   `json:"priority,omitempty"`
	Weight   *int   `json:"weight,omitempty"`
	Port     *int   `json:"port,omitempty"`
	Flag     *int   `json:"flag,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Comment  string `json:"comment,omitempty"`
	Tags     string `json:"tags,omitempty"`
	Owner    string `json:"owner,omitempty"`
	// ExpiresAt is an RFC3339 instant. It is configuration when an operator
	// sets it by hand, but the aging sweeper also writes it, so a republish
	// that carries a stale expiry shortens or extends a record's life. The
	// adapter cannot tell the two apart and does not guess: what the content
	// says is what is stored.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// DNSRecordsContent is the whole control-plane-authored record set of one zone.
//
// Whole-set rather than per-record, and this is the design decision worth
// stating: the resource a revision is numbered against is the zone, so the
// revision number is the zone's record-set generation. Per-record revisions
// were rejected because the DHCP-to-DNS linkage writes records continuously --
// every confirmed binding publishes one -- so per-record revisions would turn a
// busy pool into unbounded revision churn for records no operator ever typed,
// and "show me the record set as it was before the incident" would be a
// question about the last of thousands of rows rather than one revision.
//
// The set contains only the records the control plane authored. Rows a data
// plane pushed up (authored_locally = 1) are not configuration: they are the
// reporting copy of what a DHCP binding or a dynamic update produced, and
// including them would put machine state into the governance log and let a
// rollback delete records its author still believes it owns.
type DNSRecordsContent struct {
	Schema  int                `json:"schema"`
	Records []DNSRecordContent `json:"records"`
}

// DNSRecordsAdapter publishes the record set of a zone.
//
// Resource ID is the zone id. The zone has to exist: a record set without a
// zone is not a configuration, it is rows pointing at nothing.
type DNSRecordsAdapter struct {
	store ZoneReloader
}

// NewDNSRecordsAdapter returns an adapter that forces a zone-store reload after
// each release. store may be nil, in which case no reload is performed -- only
// appropriate where no resolver is running (tools, tests).
func NewDNSRecordsAdapter(store ZoneReloader) *DNSRecordsAdapter {
	return &DNSRecordsAdapter{store: store}
}

func (*DNSRecordsAdapter) Type() ResourceType { return ResourceDNSRecords }

// Validate checks the record set without touching the database.
//
// Every rule here is one the console's own create path enforces, and they are
// the same rules because a publish writes the same rows: a value the API would
// refuse must not be installable through the governed path, or the governance
// is a way around validation rather than a way to record it.
func (*DNSRecordsAdapter) Validate(content json.RawMessage) error {
	var c DNSRecordsContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode record set: %w", err)
	}
	switch {
	case c.Schema == 0:
		return fmt.Errorf("schema is required")
	case c.Schema != DNSRecordsSchema:
		return fmt.Errorf("schema %d is not supported, this build writes %d", c.Schema, DNSRecordsSchema)
	}

	seen := make(map[string]struct{}, len(c.Records))
	for i, rec := range c.Records {
		where := fmt.Sprintf("records[%d]", i)
		if strings.TrimSpace(rec.Name) == "" {
			return fmt.Errorf("%s: name is required", where)
		}
		if strings.ContainsAny(rec.Name, " \t\r\n") {
			return fmt.Errorf("%s: name %q contains whitespace", where, rec.Name)
		}
		if strings.TrimSpace(rec.Type) == "" {
			return fmt.Errorf("%s: type is required", where)
		}
		if !zone.SupportedRecordTypes[rec.Type] {
			return fmt.Errorf("%s: unsupported record type %q", where, rec.Type)
		}
		if rec.Value == "" {
			return fmt.Errorf("%s: value is required", where)
		}
		if rec.TTL < 0 {
			return fmt.Errorf("%s: ttl must not be negative, got %d", where, rec.TTL)
		}
		if err := zone.ValidateRecordValue(rec.Type, rec.Value, rec.Priority, rec.Weight, rec.Port, rec.Tag, rec.Flag); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if rec.ExpiresAt != "" {
			if _, err := parseRecordInstant(rec.ExpiresAt); err != nil {
				return fmt.Errorf("%s: expires_at: %w", where, err)
			}
		}
		// Two records with the same identity in one set would make the applied
		// result depend on insert order, and a rollback would restore whichever
		// row the loop happened to write last. An id has to appear once.
		if rec.ID != "" {
			key := "id:" + rec.ID
			if _, dup := seen[key]; dup {
				return fmt.Errorf("%s: record id %q appears twice", where, rec.ID)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

func (*DNSRecordsAdapter) Exists(q Queryer, id string) (bool, error) {
	var one int
	err := q.QueryRow(`SELECT 1 FROM dns_zones WHERE id = ?`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Apply replaces the zone's control-authored records with the content's set.
//
// The rows a data plane authored are left alone, and they are left alone for
// the reason the column exists: a DHCP binding's record is not this
// revision's to delete, so a rollback of an operator's mistake cannot silently
// withdraw the A record a client is currently using. Reads that mix the two are
// the resolver's business; a write that mixes them is a bug with a long fuse.
func (*DNSRecordsAdapter) Apply(tx *sql.Tx, id string, content json.RawMessage) error {
	var c DNSRecordsContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode record set: %w", err)
	}
	// The zone's name is needed to normalise the record names the way the
	// create path does -- "www" and "www.example.com" must not become two rows
	// that answer the same query.
	var zoneName string
	err := tx.QueryRow(`SELECT name FROM dns_zones WHERE id = ?`, id).Scan(&zoneName)
	if err == sql.ErrNoRows {
		return fmt.Errorf("zone %s disappeared before the release was applied", id)
	}
	if err != nil {
		return fmt.Errorf("read zone: %w", err)
	}
	before, err := snapshotServedRRs(tx, id)
	if err != nil {
		return fmt.Errorf("snapshot zone records before release: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM dns_records WHERE zone_id = ? AND authored_locally = 0`, id); err != nil {
		return fmt.Errorf("clear the zone's authored records: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO dns_records
			(id, zone_id, name, type, value, ttl, priority, weight, port, flag,
			 enabled, comment, tags, owner, expires_at, authored_locally)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`)
	if err != nil {
		return fmt.Errorf("prepare the record insert: %w", err)
	}
	defer stmt.Close()

	for _, rec := range c.Records {
		rowID := rec.ID
		if rowID == "" {
			rowID = uuid.New().String()
		}
		name := zone.NormalizeRecordName(rec.Name, zoneName)
		expiresAt, err := parseRecordInstant(rec.ExpiresAt)
		if err != nil {
			return fmt.Errorf("record %s: expires_at: %w", rec.Name, err)
		}
		live := rec.Enabled
		if expiryText, ok := expiresAt.(string); ok {
			if expiresAtTime, parseErr := time.Parse("2006-01-02 15:04:05", expiryText); parseErr == nil && !expiresAtTime.After(time.Now()) {
				live = false
			}
		}
		if live {
			if err := zone.ValidateRRsetTTLTx(tx, id, name, rec.Type, rec.TTL, ""); err != nil {
				return fmt.Errorf("record %s %s: %w", rec.Name, rec.Type, err)
			}
		}
		if _, err := stmt.Exec(
			rowID, id, name, rec.Type, rec.Value,
			rec.TTL, intOrNil(rec.Priority), intOrNil(rec.Weight), intOrNil(rec.Port),
			intOrNil(rec.Flag), rec.Enabled, rec.Comment, rec.Tags, rec.Owner, expiresAt,
		); err != nil {
			return fmt.Errorf("insert record %s %s: %w", rec.Name, rec.Type, err)
		}
	}
	after, err := snapshotServedRRs(tx, id)
	if err != nil {
		return fmt.Errorf("snapshot zone records after release: %w", err)
	}
	if !sameServedRRs(before, after) {
		if err := bumpZoneSerialForRelease(tx, id); err != nil {
			return fmt.Errorf("bump zone serial after record release: %w", err)
		}
	}
	return nil
}

type servedRR struct {
	name, rtype, value string
	ttl                int
	priority, weight   int64
	port, flag         int64
	tag                string
}

func snapshotServedRRs(tx *sql.Tx, zoneID string) ([]servedRR, error) {
	rows, err := tx.Query(`
		SELECT name, type, value, ttl, COALESCE(priority, 0), COALESCE(weight, 0),
			COALESCE(port, 0), COALESCE(flag, 0), COALESCE(tag, '')
		FROM dns_records WHERE zone_id = ? AND enabled = 1`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []servedRR
	for rows.Next() {
		var rr servedRR
		if err := rows.Scan(&rr.name, &rr.rtype, &rr.value, &rr.ttl, &rr.priority, &rr.weight,
			&rr.port, &rr.flag, &rr.tag); err != nil {
			return nil, err
		}
		records = append(records, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].name != records[j].name {
			return records[i].name < records[j].name
		}
		if records[i].rtype != records[j].rtype {
			return records[i].rtype < records[j].rtype
		}
		if records[i].value != records[j].value {
			return records[i].value < records[j].value
		}
		if records[i].ttl != records[j].ttl {
			return records[i].ttl < records[j].ttl
		}
		if records[i].priority != records[j].priority {
			return records[i].priority < records[j].priority
		}
		if records[i].weight != records[j].weight {
			return records[i].weight < records[j].weight
		}
		if records[i].port != records[j].port {
			return records[i].port < records[j].port
		}
		if records[i].flag != records[j].flag {
			return records[i].flag < records[j].flag
		}
		return records[i].tag < records[j].tag
	})
	return records, nil
}

func sameServedRRs(a, b []servedRR) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func bumpZoneSerialForRelease(tx *sql.Tx, zoneID string) error {
	var current uint32
	if err := tx.QueryRow("SELECT serial FROM dns_zones WHERE id = ?", zoneID).Scan(&current); err != nil {
		return err
	}
	next := zone.NextSerial(current)
	res, err := tx.Exec(`UPDATE dns_zones SET serial = ?, updated_at = datetime('now') WHERE id = ? AND serial = ?`, next, zoneID, current)
	if err != nil {
		return err
	}
	if affected, err := res.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return fmt.Errorf("zone serial changed concurrently")
	}
	return nil
}

func (a *DNSRecordsAdapter) Notify(string) error {
	if a.store == nil {
		return nil
	}
	a.store.ReloadNow()
	return nil
}

// ReadDNSRecordsContent returns the zone's current record set in the shape this
// adapter publishes.
//
// It exists so that a caller building a publish does not have to reconstruct
// the content by hand and get the field set or the ordering wrong: a content
// that differs only in the way it was assembled produces a diff of every
// record, which is exactly the reviewability this package is for. Callers that
// want to change one record read this, change the one entry, and publish.
//
// The order is deterministic (by name, type, value, id) because the content is
// compared byte for byte after canonicalisation: an unstable order would make
// two identical configurations hash differently.
func ReadDNSRecordsContent(q Queryer, zoneID string) (json.RawMessage, error) {
	rows, err := q.Query(`
		SELECT id, name, type, value, ttl, priority, weight, port, flag, enabled,
		       comment, tags, owner, COALESCE(expires_at, '')
		FROM dns_records
		WHERE zone_id = ? AND authored_locally = 0
		ORDER BY name, type, value, id`, zoneID)
	if err != nil {
		return nil, fmt.Errorf("read the zone's records: %w", err)
	}
	defer rows.Close()

	content := DNSRecordsContent{Schema: DNSRecordsSchema, Records: []DNSRecordContent{}}
	for rows.Next() {
		var rec DNSRecordContent
		var comment, tags, owner, expiresAt sql.NullString
		if err := rows.Scan(
			&rec.ID, &rec.Name, &rec.Type, &rec.Value, &rec.TTL,
			&rec.Priority, &rec.Weight, &rec.Port, &rec.Flag, &rec.Enabled,
			&comment, &tags, &owner, &expiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan a record: %w", err)
		}
		rec.Comment, rec.Tags, rec.Owner = comment.String, tags.String, owner.String
		rec.ExpiresAt = expiresAt.String
		content.Records = append(content.Records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the zone's records: %w", err)
	}
	out, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("encode the record set: %w", err)
	}
	return out, nil
}

func intOrNil(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// parseRecordInstant accepts the two shapes an instant arrives in: SQLite's own
// `YYYY-MM-DD HH:MM:SS` from a column that was read straight out of the
// database, and RFC3339 from a caller. Both are stored back in SQLite's shape,
// so a record's expiry compares with datetime('now') rather than with a string
// that sorts differently.
func parseRecordInstant(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return t.UTC().Format("2006-01-02 15:04:05"), nil
		}
	}
	return nil, fmt.Errorf("%q is not a timestamp", raw)
}
