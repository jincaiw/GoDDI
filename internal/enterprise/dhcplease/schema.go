// Package dhcplease implements the frozen Enterprise DHCP lease reference
// schema (contracts/enterprise-dhcp-lease.proposal.md, frozen round 256–258)
// and the lease-table portion of the schema-2 snapshot: install/verify the
// reference DDL, encode/decode candidate rows, and check the cross-table
// invariants the lease tables add on top of the eight-table IPAM candidate.
//
// What this package does NOT do: it does not serve DHCP, does not write
// production leases, and does not authorize any snapshot install. The public
// staging/import entry points stay closed (prodgates); this package is the
// frozen schema plus its verification primitives, composed by snapshotstage
// behind the same closed doors.
package dhcplease

import (
	"database/sql"
	"fmt"
)

// SchemaVersion2 is the snapshot schema version that includes the lease
// tables. The eight-table IPAM candidate is schema 1; adding the lease/
// declined/outbox/receipt table set changes the exported table set, which is
// exactly what a version bump exists to declare.
const SchemaVersion2 = 2

// FrozenDDL is the reference schema for the lease table set, frozen per the
// decisions recorded in the contract: address is a 16-byte network-order BLOB
// with an explicit family, expires_ms is nullable for permanent/reservation
// leases, the active-uniqueness is a partial unique index over the live
// kinds, and a dedicated declined table holds isolation entries with a
// two-way mutual exclusion against active leases.
const FrozenDDL = `
CREATE TABLE dhcp_lease(
  tenant TEXT NOT NULL,
  space TEXT NOT NULL,
  address BLOB(16) NOT NULL,
  family INTEGER NOT NULL CHECK (family IN (4,6)),
  lease_id TEXT NOT NULL,
  client_key BLOB NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('OFFER','BOUND','RELEASED','EXPIRED','DECLINED')),
  offer_xid INTEGER NOT NULL CHECK (offer_xid >= 0),
  issued_ms INTEGER NOT NULL CHECK (issued_ms >= 0),
  expires_ms INTEGER CHECK (expires_ms IS NULL OR expires_ms >= issued_ms),
  revision INTEGER NOT NULL CHECK (revision > 0),
  PRIMARY KEY (tenant, space, address),
  UNIQUE (tenant, space, lease_id)
);
CREATE UNIQUE INDEX dhcp_lease_active_client
  ON dhcp_lease(tenant, space, client_key) WHERE kind IN ('OFFER','BOUND');
CREATE TABLE dhcp_declined(
  tenant TEXT NOT NULL,
  space TEXT NOT NULL,
  address BLOB(16) NOT NULL,
  family INTEGER NOT NULL CHECK (family IN (4,6)),
  reason TEXT NOT NULL,
  declined_ms INTEGER NOT NULL CHECK (declined_ms >= 0),
  revision INTEGER NOT NULL CHECK (revision > 0),
  PRIMARY KEY (tenant, space, address)
);
CREATE TABLE dhcp_outbox(
  cluster_id BLOB NOT NULL,
  epoch INTEGER NOT NULL,
  seq INTEGER NOT NULL,
  hash BLOB NOT NULL,
  payload BLOB NOT NULL,
  PRIMARY KEY (cluster_id, epoch, seq)
);
CREATE TABLE dhcp_receipt(
  tenant TEXT NOT NULL,
  space TEXT NOT NULL,
  client_key BLOB NOT NULL,
  xid INTEGER NOT NULL,
  kind TEXT NOT NULL,
  digest BLOB NOT NULL,
  reply_kind TEXT NOT NULL,
  address BLOB(16) NOT NULL,
  expires_ms INTEGER,
  revision INTEGER NOT NULL,
  seq INTEGER NOT NULL,
  PRIMARY KEY (tenant, space, client_key, xid, kind)
);
`

// install applies the frozen DDL inside the caller's transaction. The caller
// owns commit/rollback; this function only runs statements.
func install(tx *sql.Tx) error {
	if _, err := tx.Exec(FrozenDDL); err != nil {
		return fmt.Errorf("dhcplease: installing frozen ddl: %w", err)
	}
	return nil
}
