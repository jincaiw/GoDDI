// Package auditlog holds the shape of an audit entry written by a data plane,
// and the action names that identify one.
//
// It exists because the writes that need a trail most are the ones with nobody
// watching them, and those happen in three places outside the control plane:
// the dynamic-update handler, the DHCP-to-DNS linkage, and the lease manager.
// The statement they use has to be the same one. The control plane's HTTP
// middleware keeps its own writer; what this package covers is the writes that
// happen where no request and no operator exists.
//
// Entries are appended and never revised. The control database enforces that
// with a trigger (migration 024); the data plane holds a copy that is a queue,
// not an archive.
package auditlog

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// Action names. A constant rather than a literal at each call site, so that a
// query or an alert cannot drift from the writer.
const (
	// ActionDynamicUpdate is an RFC 2136 update accepted from a client.
	ActionDynamicUpdate = "dns_dynamic_update"

	// ActionDDNSRecordCreate is a record the DHCP linkage published for a
	// confirmed binding.
	ActionDDNSRecordCreate = "ddns_record_create"

	// ActionDDNSRecordDelete is a record the DHCP linkage withdrew when a
	// binding ended.
	ActionDDNSRecordDelete = "ddns_record_delete"

	// ActionLeaseBind is an address becoming a confirmed binding: an offer
	// promoted by a REQUEST, or a lease created already confirmed.
	ActionLeaseBind = "dhcp_lease_bind"

	// ActionLeaseRelease is a client handing its address back.
	ActionLeaseRelease = "dhcp_lease_release"

	// ActionLeaseDecline is a client reporting that the address it was given
	// is already in use by something else.
	ActionLeaseDecline = "dhcp_lease_decline"

	// ActionLeaseExpire is the sweep reclaiming an address whose time ran out.
	// There is deliberately no renewal action: extending a binding does not
	// change it, and a trail of every renew would be a trail of nothing.
	ActionLeaseExpire = "dhcp_lease_expire"

	// ActionHADegrade is an operator approving single-copy operation on a DHCP
	// node, or withdrawing that approval.
	//
	// It is an audit action because it is the one thing in the HA contract that
	// no observation can produce: a node that has lost its second copy is
	// indistinguishable, from the outside, from one an operator has excused.
	// Only the trail says which.
	ActionHADegrade = "dhcp_ha_degrade"

	// ActionHATakeover is a standby being promoted to primary.
	ActionHATakeover = "dhcp_ha_takeover"

	// ActionHAFence is a node being taken out of service by an operator.
	ActionHAFence = "dhcp_ha_fence"

	// ActionHARejoin is a fenced node returning to the pair as a standby.
	ActionHARejoin = "dhcp_ha_rejoin"
)

// Resource types.
const (
	ResourceZone   = "zone"
	ResourceRecord = "record"
	ResourceLease  = "lease"

	// ResourceNode is a node's own role: what it is serving as, rather than
	// anything it holds.
	ResourceNode = "node"
)

// Entry is one audit row.
//
// OldValue and NewValue are the before and after state of whatever changed.
// They are empty rather than "unchanged" when the change has no other side: a
// create has no before, a delete has no after. Append turns an empty side into
// NULL so that a reader can tell "there was nothing" from "it was blank".
type Entry struct {
	UserID       string
	Username     string
	Action       string
	ResourceType string
	ResourceID   string
	Detail       string
	OldValue     string
	NewValue     string
}

// Append writes an entry and returns the id it assigned.
//
// The caller decides what a failure means. Neither data-plane writer treats it
// as fatal -- a client's name must still resolve, and an address must still be
// handed out, whether or not the trail could be written -- but both log it, so
// that an incomplete audit is something an operator sees rather than something
// they discover by reading the table.
func Append(db *sql.DB, e Entry) (string, error) {
	id := uuid.New().String()
	if _, err := db.Exec(`
		INSERT INTO audit_logs
			(id, user_id, username, action, resource_type, resource_id,
			 detail, old_value, new_value, success, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, datetime('now'))`,
		id,
		nullIfEmpty(e.UserID),
		nullIfEmpty(e.Username),
		e.Action,
		e.ResourceType,
		nullIfEmpty(e.ResourceID),
		nullIfEmpty(e.Detail),
		nullIfEmpty(e.OldValue),
		nullIfEmpty(e.NewValue),
	); err != nil {
		return "", fmt.Errorf("writing audit entry %s: %w", e.Action, err)
	}
	return id, nil
}

// nullIfEmpty records "there was nothing" as SQL NULL rather than as the empty
// string. SQL can tell the two apart, and every reader checks `.Valid` rather
// than `!= ""`, so a blank stored here would read as a value that exists -- and
// a create would look like a change from something to nothing.
func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
