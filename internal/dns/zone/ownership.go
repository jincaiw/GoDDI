package zone

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrOwnedByTheDataPlane is returned when the console tries to change a record
// the data plane owns.
//
// A record with authored_locally = 1 was written where it is served from -- by
// a DHCP binding publishing its name, by an RFC 2136 update from an authorised
// client, or by an inbound zone transfer. The control database's copy of it
// exists so the console can show it, and the plane that owns it reports every
// change.
//
// Editing or deleting that copy would therefore report success and change
// nothing about the name being served: the owner is not asked, the next push
// republishes the row, and the operator is left believing a name they can still
// resolve was removed. Refusing the write and saying where the record lives is
// the honest alternative, and it is the same rule already applied to leases.
//
// It is per row rather than per zone on purpose. A zone usually holds both
// kinds of record at once -- the operator's own and the ones DHCP published --
// and a zone-wide switch would either refuse edits to the operator's records or
// silently accept edits to the published ones.
var ErrOwnedByTheDataPlane = errors.New("record is owned by the data plane")

// assertEditable refuses a write to a record the data plane authored.
//
// The read is a single indexed lookup of one column, not a GetRecord: this runs
// on the write paths only, so a listing or a lookup for display is unaffected.
func (m *RecordManager) assertEditable(id string) error {
	if id == "" {
		return fmt.Errorf("record id is required")
	}
	var authoredLocally int
	err := m.db.QueryRow(
		`SELECT authored_locally FROM dns_records WHERE id = ?`, id).Scan(&authoredLocally)
	if err == sql.ErrNoRows {
		// Not this function's error to raise: the caller's own lookup reports
		// the missing record, and saying "not yours" about a row that does not
		// exist would be a worse message.
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking the ownership of record %s: %w", id, err)
	}
	if authoredLocally != 0 {
		return fmt.Errorf("%w: %s", ErrOwnedByTheDataPlane, id)
	}
	return nil
}
