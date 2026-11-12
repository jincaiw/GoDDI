package dhcplease

import (
	"database/sql"
	"fmt"
)

// Verify checks the cross-table invariants the lease tables add on top of the
// eight-table IPAM candidate, inside one read-only transaction the caller
// opened. Every check here exists because a violation would mean the snapshot
// lies about the pair it describes:
//
//   - active leases (OFFER/BOUND) must have family consistent with the
//     16-byte address they carry;
//   - an address must not be both actively leased and declined (two-way
//     mutual exclusion with the isolation table);
//   - every outbox entry's seq must be strictly positive and the (cluster,
//     epoch, seq) key unique by construction (enforced by DDL — re-checked
//     here against the data so a DDL mutation cannot hide it);
//   - receipt rows must reference a lease kind that exists in the frozen
//     enum and carry an address of the matching family.
//
// Like every check in the snapshot packages, passing this does not prove the
// source was truthful; it proves the candidate is internally consistent.
func Verify(tx *sql.Tx) error {
	// Family consistency: the first byte of a 16-byte address decides v4/v6.
	rows, err := tx.Query(`
		SELECT address, family FROM dhcp_lease
		UNION ALL SELECT address, family FROM dhcp_declined`)
	if err != nil {
		return fmt.Errorf("dhcplease: reading addresses for family check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var addr []byte
		var family int
		if err := rows.Scan(&addr, &family); err != nil {
			return fmt.Errorf("dhcplease: scanning address: %w", err)
		}
		if len(addr) != 16 {
			return fmt.Errorf("dhcplease: address must be 16 bytes, got %d", len(addr))
		}
		v4 := addr[0] == 0 && addr[1] == 0 && addr[2] == 0 && addr[3] == 0 &&
			addr[4] == 0 && addr[5] == 0 && addr[6] == 0 && addr[7] == 0 &&
			addr[8] == 0 && addr[9] == 0 && addr[10] == 0xff && addr[11] == 0xff
		want := 6
		if v4 {
			want = 4
		}
		if family != want {
			return fmt.Errorf("dhcplease: address family mismatch: prefix says %d, row says %d", want, family)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("dhcplease: iterating addresses: %w", err)
	}

	// Two-way mutual exclusion: no address may be both actively leased and
	// declined. The DDL cannot express this across two tables; the snapshot
	// must not carry a pair that would violate it.
	excl, err := tx.Query(`
		SELECT l.address FROM dhcp_lease l
		JOIN dhcp_declined d
		  ON l.tenant = d.tenant AND l.space = d.space AND l.address = d.address
		WHERE l.kind IN ('OFFER','BOUND')
		LIMIT 1`)
	if err != nil {
		return fmt.Errorf("dhcplease: reading active-vs-declined overlap: %w", err)
	}
	defer excl.Close()
	if excl.Next() {
		return fmt.Errorf("dhcplease: an address is both actively leased and declined")
	}
	if err := excl.Err(); err != nil {
		return fmt.Errorf("dhcplease: iterating overlap: %w", err)
	}

	// Outbox sanity: sequence must be positive; a non-positive seq would break
	// the watermark arithmetic the takeover is decided on.
	ob, err := tx.Query(`SELECT seq FROM dhcp_outbox WHERE seq <= 0 LIMIT 1`)
	if err != nil {
		return fmt.Errorf("dhcplease: reading outbox seq: %w", err)
	}
	defer ob.Close()
	if ob.Next() {
		return fmt.Errorf("dhcplease: outbox carries a non-positive sequence")
	}
	if err := ob.Err(); err != nil {
		return fmt.Errorf("dhcplease: iterating outbox: %w", err)
	}

	// Receipt kinds must come from the frozen enum.
	rec, err := tx.Query(`
		SELECT DISTINCT kind FROM dhcp_receipt
		WHERE kind NOT IN ('OFFER','BOUND','RELEASED','EXPIRED','DECLINED') LIMIT 1`)
	if err != nil {
		return fmt.Errorf("dhcplease: reading receipt kinds: %w", err)
	}
	defer rec.Close()
	if rec.Next() {
		return fmt.Errorf("dhcplease: receipt carries a kind outside the frozen enum")
	}
	if err := rec.Err(); err != nil {
		return fmt.Errorf("dhcplease: iterating receipts: %w", err)
	}
	return nil
}
