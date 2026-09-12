package dhcp

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jasonwa/goddi/internal/auditlog"
)

// ddnsAuditRecord is one dns_records row, reduced to what identifies it. The
// whole row is not useful in an audit trail; the name, the type and the address
// are what an operator would look up afterwards.
type ddnsAuditRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// AuditActions names the two actions the linkage records. Aliased here so that
// a reader of this package does not have to know where the strings live, while
// there is still exactly one definition of each.
const (
	AuditActionRecordCreate = auditlog.ActionDDNSRecordCreate
	AuditActionRecordDelete = auditlog.ActionDDNSRecordDelete
)

// recordsOwnedBy returns the records a binding owns, as of now.
//
// The rows are materialised before the cursor closes. The store runs on a
// single connection, where issuing another statement while a cursor is open on
// the same connection blocks forever rather than returning an error.
func (dl *DNSLink) recordsOwnedBy(ownerRef string) ([]ddnsAuditRecord, error) {
	if ownerRef == "" {
		return nil, nil
	}
	rows, err := dl.stores.DNS.Query(`
		SELECT name, type, value FROM dns_records
		WHERE owner = 'dhcp' AND owner_ref = ?
		ORDER BY type, name`, ownerRef)
	if err != nil {
		return nil, err
	}
	var out []ddnsAuditRecord
	for rows.Next() {
		var r ddnsAuditRecord
		if err := rows.Scan(&r.Name, &r.Type, &r.Value); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}

func encodeAuditRecords(records []ddnsAuditRecord) string {
	if len(records) == 0 {
		return ""
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		// Marshal cannot fail on this shape, but an audit column that silently
		// becomes empty because of a serialisation surprise is worse than a
		// readable fallback.
		return fmt.Sprintf("%d records (not serialisable: %v)", len(records), err)
	}
	return string(encoded)
}

// auditRecordChange appends an entry for a record the linkage published or
// withdrew.
//
// Before and after are read from dns_records rather than reconstructed from the
// event: the event carries a snapshot taken before the write, and the point of
// an audit entry is to say what the database actually held. A create has an
// empty before side and a populated after side; a delete is the other way
// round.
//
// A failure here is logged and then dropped. The linkage exists to make a
// binding resolvable, and refusing to publish a name because the audit row
// could not be written would trade the function an operator depends on for the
// record of it. It is logged at error level precisely so that an incomplete
// audit is something an operator sees rather than something they find out by
// reading the table.
func (dl *DNSLink) auditRecordChange(e DNSEvent, action, oldValue, newValue string) {
	if _, err := auditlog.Append(dl.stores.DNS, auditlog.Entry{
		UserID:       "dhcp",
		Username:     "dhcp",
		Action:       action,
		ResourceType: auditlog.ResourceRecord,
		ResourceID:   e.LeaseID,
		Detail: fmt.Sprintf("hostname=%s ip=%s generation=%d",
			e.Hostname, e.IPAddress, e.Generation),
		OldValue: oldValue,
		NewValue: newValue,
	}); err != nil {
		slog.Error("dhcp_dns_link: failed to write the audit entry for a record change",
			"action", action, "lease_id", e.LeaseID, "ip", e.IPAddress, "error", err)
	}
}
