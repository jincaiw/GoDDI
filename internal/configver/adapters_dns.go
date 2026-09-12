package configver

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/miekg/dns"

	"github.com/jasonwa/goddi/internal/dns/zone"
)

// ZoneReloader is the part of the zone store this adapter needs.
//
// It is an interface rather than *zone.Store so the adapter can be tested
// without a database-backed store, and so that what the adapter depends on is
// legible: it needs the ability to force a reload, nothing else.
type ZoneReloader interface {
	// ReloadNow performs a synchronous full reload. The debounced Reload is
	// not sufficient here -- the release is only "applied" once the data
	// plane can answer from the new content, and a debounced reload leaves a
	// window in which the revision is marked applied while queries are still
	// answered from the old zone.
	ReloadNow()
}

// DNSZoneContent is a complete snapshot of a DNS zone's publishable state.
//
// `serial` is deliberately excluded. The serial advances on every apply, so
// including it would make a rollback to a byte-identical configuration show up
// as a change, and would let a caller try to pin a serial that the zone
// manager then overwrites. The serial is derived state, not configuration.
type DNSZoneContent struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Enabled        bool   `json:"enabled"`
	DNSSECEnabled  bool   `json:"dnssec_enabled"`
	DefaultTTL     int    `json:"default_ttl"`
	SOAMName       string `json:"soa_mname"`
	SOARName       string `json:"soa_rname"`
	Refresh        int    `json:"refresh"`
	Retry          int    `json:"retry"`
	Expire         int    `json:"expire"`
	Minimum        int    `json:"minimum"`
	TransferPolicy string `json:"transfer_policy"`
	UpdatePolicy   string `json:"update_policy"`
}

// DNSZoneAdapter publishes DNS zone configuration.
type DNSZoneAdapter struct {
	store ZoneReloader
}

// NewDNSZoneAdapter returns an adapter that forces a zone-store reload after
// each release. store may be nil, in which case no reload is performed -- only
// appropriate when no resolver is running (tools, tests).
func NewDNSZoneAdapter(store ZoneReloader) *DNSZoneAdapter {
	return &DNSZoneAdapter{store: store}
}

func (*DNSZoneAdapter) Type() ResourceType { return ResourceDNSZone }

func (*DNSZoneAdapter) Validate(content json.RawMessage) error {
	var c DNSZoneContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode zone content: %w", err)
	}

	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if err := validZoneName(c.Name); err != nil {
		return err
	}

	if c.Type == "" {
		return fmt.Errorf("type is required")
	}
	valid := false
	for _, vt := range zone.ValidZoneTypes {
		if string(vt) == c.Type {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown zone type %q", c.Type)
	}

	if strings.TrimSpace(c.SOAMName) == "" {
		return fmt.Errorf("soa_mname is required")
	}
	if strings.TrimSpace(c.SOARName) == "" {
		return fmt.Errorf("soa_rname is required")
	}
	// A zone whose MISMATCH could never be notified or whose timers are
	// negative will be accepted by SQLite and then misbehave at query time.
	// SOA timers are unsigned 32-bit in the wire format, so a negative value
	// is not a stylistic problem: it silently becomes a huge positive one.
	for _, f := range []struct {
		name  string
		value int
	}{
		{"default_ttl", c.DefaultTTL},
		{"refresh", c.Refresh},
		{"retry", c.Retry},
		{"expire", c.Expire},
		{"minimum", c.Minimum},
	} {
		if f.value < 0 {
			return fmt.Errorf("%s must not be negative, got %d", f.name, f.value)
		}
	}

	// ACL columns hold JSON arrays. A malformed value is stored happily and
	// then ignored by the reader, which silently turns a restrictive policy
	// into an unrestricted one -- a security-relevant failure, so it is
	// rejected at publish time.
	for _, f := range []struct{ name, value string }{
		{"transfer_policy", c.TransferPolicy},
		{"update_policy", c.UpdatePolicy},
	} {
		if strings.TrimSpace(f.value) == "" {
			continue
		}
		var probe any
		if err := json.Unmarshal([]byte(f.value), &probe); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", f.name, err)
		}
	}
	return nil
}

func (*DNSZoneAdapter) Exists(q Queryer, id string) (bool, error) {
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

func (*DNSZoneAdapter) Apply(tx *sql.Tx, id string, content json.RawMessage) error {
	var c DNSZoneContent
	if err := json.Unmarshal(content, &c); err != nil {
		return fmt.Errorf("decode zone content: %w", err)
	}

	res, err := tx.Exec(`UPDATE dns_zones SET
			name = ?, type = ?, enabled = ?, dnssec_enabled = ?, default_ttl = ?,
			soa_mname = ?, soa_rname = ?, refresh = ?, retry = ?, expire = ?,
			minimum = ?, transfer_policy = ?, update_policy = ?,
			updated_at = datetime('now')
		WHERE id = ?`,
		c.Name, c.Type, c.Enabled, c.DNSSECEnabled, c.DefaultTTL,
		c.SOAMName, c.SOARName, c.Refresh, c.Retry, c.Expire,
		c.Minimum, c.TransferPolicy, c.UpdatePolicy, id)
	if err != nil {
		return fmt.Errorf("update zone: %w", err)
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return fmt.Errorf("zone %s disappeared before the release was applied", id)
	}
	return nil
}

// validZoneName checks a zone name beyond what dns.IsDomainName reports.
//
// IsDomainName only rejects structural damage: it accepts "not a domain" and
// "a b.test" as valid names. A space survives storage and then never matches a
// query, because queries never contain spaces -- producing a zone that exists,
// is enabled, lists its records, and answers nothing. That failure is silent
// enough to be worth an explicit character check.
//
// Internationalised names are expected in punycode (xn--...), which is ASCII
// and therefore accepted here. Any other non-ASCII input is rejected rather
// than stored as bytes no resolver will ever match.
func validZoneName(name string) error {
	if strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("name %q contains whitespace", name)
	}
	if _, ok := dns.IsDomainName(name); !ok {
		return fmt.Errorf("name %q is not a valid domain name", name)
	}

	trimmed := strings.TrimSuffix(name, ".")
	if trimmed == "" {
		return fmt.Errorf("name %q has no labels", name)
	}
	for _, label := range strings.Split(trimmed, ".") {
		if label == "" {
			return fmt.Errorf("name %q has an empty label", name)
		}
		if len(label) > 63 {
			return fmt.Errorf("name %q has a label longer than 63 characters", name)
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			switch {
			case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			case ch == '-' || ch == '_':
				// Underscore-leading labels are legitimate (for example
				// _msdcs), so only hyphen placement is constrained.
				if ch == '-' && (i == 0 || i == len(label)-1) {
					return fmt.Errorf("name %q has a label starting or ending with '-'", name)
				}
			default:
				return fmt.Errorf("name %q contains an invalid character %q", name, string(ch))
			}
		}
	}
	return nil
}

func (a *DNSZoneAdapter) Notify(string) error {
	if a.store == nil {
		return nil
	}
	a.store.ReloadNow()
	return nil
}
