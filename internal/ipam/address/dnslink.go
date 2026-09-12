package address

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// DNSLink ties one address to one DNS record.
//
// This replaces the single dns_record_id column. A host normally has at least
// two records -- a forward A and a reverse PTR -- and may have several A
// records across zones; with one column the second write overwrote the first
// and the IPAM view then claimed the host had exactly one name. Links are rows,
// so they accumulate instead of replacing each other.
type DNSLink struct {
	ID         string `json:"id"`
	AddressID  string `json:"address_id"`
	RecordID   string `json:"record_id"`
	ZoneID     string `json:"zone_id,omitempty"`
	RecordName string `json:"record_name"`
	RecordType string `json:"record_type"`
	Value      string `json:"value"`
	Source     string `json:"source"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

const dnsLinkColumns = `id, address_id, record_id, zone_id, record_name, record_type, value, source, created_at, updated_at`

// LinkDNSRecord records that a DNS record publishes an address.
//
// VESTIGIAL. Nothing in the runtime calls this, so `ipam_dns_links` stays empty
// in every deployment, and readers of that table get a wrong answer rather than
// a stale one. The 360° view therefore derives its DNS half from `dns_records`
// instead (see ipam.Linkage.publishingRecordsForIP); this function and the
// table are kept only until a migration removes them, and no new reader should
// be written against either.
//
// It is idempotent on (address_id, record_id): re-linking the same pair
// refreshes the stored name and value rather than failing, because the caller
// is usually a reconciler that cannot know whether it already ran.
func (m *Manager) LinkDNSRecord(link DNSLink) error {
	if link.AddressID == "" {
		return fmt.Errorf("link: address_id is required")
	}
	if link.RecordID == "" {
		return fmt.Errorf("link: record_id is required")
	}
	if link.Source == "" {
		link.Source = SourceDNS
	}
	if _, err := m.db.Exec(`
		INSERT INTO ipam_dns_links
			(id, address_id, record_id, zone_id, record_name, record_type, value, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(address_id, record_id) DO UPDATE SET
			zone_id     = excluded.zone_id,
			record_name = excluded.record_name,
			record_type = excluded.record_type,
			value       = excluded.value,
			updated_at  = datetime('now')`,
		uuid.New().String(), link.AddressID, link.RecordID, nullIfEmpty(link.ZoneID),
		link.RecordName, link.RecordType, link.Value, link.Source); err != nil {
		return fmt.Errorf("link DNS record: %w", err)
	}
	return nil
}

// UnlinkDNSRecord removes one address-to-record link.
func (m *Manager) UnlinkDNSRecord(addressID, recordID string) error {
	if _, err := m.db.Exec(
		`DELETE FROM ipam_dns_links WHERE address_id = ? AND record_id = ?`,
		addressID, recordID); err != nil {
		return fmt.Errorf("unlink DNS record: %w", err)
	}
	return nil
}

// UnlinkDNSRecordByRecordID removes every link to a record.
//
// A deleted record cannot be referenced any more, and leaving the link behind
// would make the address's 360 view list a name that no longer resolves --
// the same class of bug as an orphaned reverse entry.
func (m *Manager) UnlinkDNSRecordByRecordID(recordID string) error {
	if _, err := m.db.Exec(`DELETE FROM ipam_dns_links WHERE record_id = ?`, recordID); err != nil {
		return fmt.Errorf("unlink DNS record: %w", err)
	}
	return nil
}

// DNSLinksFor returns every DNS record linked to an address.
func (m *Manager) DNSLinksFor(addressID string) ([]DNSLink, error) {
	rows, err := m.db.Query(
		`SELECT `+dnsLinkColumns+` FROM ipam_dns_links WHERE address_id = ? ORDER BY record_name`, addressID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DNSLink
	for rows.Next() {
		var l DNSLink
		var zoneID sql.NullString
		if err := rows.Scan(&l.ID, &l.AddressID, &l.RecordID, &zoneID, &l.RecordName,
			&l.RecordType, &l.Value, &l.Source, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.ZoneID = zoneID.String
		out = append(out, l)
	}
	return out, rows.Err()
}

// DNSLinksForIP resolves an address by space and IP, then returns its links.
// It returns an empty slice rather than an error when the address has no row:
// "this address has no names" is an answer, not a failure.
func (m *Manager) DNSLinksForIP(spaceID, ip string) ([]DNSLink, error) {
	a, err := m.GetAddressBySpaceIP(spaceID, ip)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}
	return m.DNSLinksFor(a.ID)
}

// OrphanedDNSLinks returns links whose record no longer exists.
//
// VESTIGIAL, and it is important to read that as "this cannot fire" rather
// than "this found nothing". `ipam_dns_links` has no production writer, so the
// result is empty because the table is empty -- not because the links are
// healthy. A caller that reports it as a passing check would be describing a
// property of the code rather than of the deployment, which is why
// /ipam/integrity no longer includes it.
func (m *Manager) OrphanedDNSLinks(limit int) ([]DNSLink, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := m.db.Query(`
		SELECT l.id, l.address_id, l.record_id, l.zone_id, l.record_name,
		       l.record_type, l.value, l.source, l.created_at, l.updated_at
		FROM ipam_dns_links l
		LEFT JOIN dns_records r ON r.id = l.record_id
		WHERE r.id IS NULL
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DNSLink
	for rows.Next() {
		var l DNSLink
		var zoneID sql.NullString
		if err := rows.Scan(&l.ID, &l.AddressID, &l.RecordID, &zoneID, &l.RecordName,
			&l.RecordType, &l.Value, &l.Source, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.ZoneID = zoneID.String
		out = append(out, l)
	}
	return out, rows.Err()
}

// ErrDNSRecordNotFound means a link target does not exist.
var ErrDNSRecordNotFound = errors.New("dns record not found")

// NonCanonicalPublisher is an A/AAAA record whose stored value is not the
// canonical spelling of the address it represents.
type NonCanonicalPublisher struct {
	ID     string `json:"id"`
	ZoneID string `json:"zone_id,omitempty"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	// Canonical is what the value should be for the address to be findable.
	Canonical string `json:"canonical"`
}

// NonCanonicalPublishingRecords finds A/AAAA records that no lookup will find.
//
// The 360° address view answers "which names publish this address" by comparing
// a record's `value` to the address as a string. That comparison is exact, so a
// record storing "192.000.000.001" or "2001:0db8:0000::1" is invisible to the
// view even though a resolver would serve it: the address is published, and the
// view says it has no name.
//
// It is the same hazard as a non-canonical address row, which is why the two
// checks sit next to each other, and it is bounded the same way: the first
// `limit` records are examined, so a clean result means "none among the rows
// examined" rather than a proof about the table.
func (m *Manager) NonCanonicalPublishingRecords(limit int) ([]NonCanonicalPublisher, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := m.db.Query(`
		SELECT id, COALESCE(zone_id, ''), name, type, value
		FROM dns_records
		WHERE type IN ('A', 'AAAA')
		ORDER BY name
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []NonCanonicalPublisher
	for rows.Next() {
		var p NonCanonicalPublisher
		if err := rows.Scan(&p.ID, &p.ZoneID, &p.Name, &p.Type, &p.Value); err != nil {
			return nil, err
		}
		canonical, err := NormalizeIP(p.Value)
		// A value that is not an address at all is a different defect -- the
		// record cannot be served either way -- and reporting it here would
		// blur "unfindable spelling" with "not an address".
		if err != nil || canonical == p.Value {
			continue
		}
		p.Canonical = canonical
		out = append(out, p)
	}
	return out, rows.Err()
}
