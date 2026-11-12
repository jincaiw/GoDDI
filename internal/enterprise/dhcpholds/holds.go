// Package dhcpholds is an isolated empty-test-database prototype, not a
// migration, authorization mechanism, DHCP server, or production-ready model.
package dhcpholds

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/ipamstore"
	_ "modernc.org/sqlite"
)

var ErrInvalid = errors.New("dhcpholds: invalid prototype setup or hold")

type Mapping struct{ Scope, Tenant, Space, Subnet string }

// Install installs the prototype in a caller-owned transaction with foreign_keys
// enabled on its physical connection. All ipam_-named main-schema objects and
// all triggers and indexes attached to ipam_ tables must match the reference;
// temporary objects in that scope are rejected. Unrelated objects are allowed.
// No IPAM occupancy may exist. Mappings are fixed and must refer to existing
// subnets. Failure rolls all installation changes back to a savepoint; callers
// still own and must roll back the transaction after any error. This is not an
// upgrade path and does not validate ownership, epoch, or network identity.
func Install(ctx context.Context, tx *sql.Tx, mappings []Mapping) (err error) {
	if tx == nil || len(mappings) == 0 {
		return ErrInvalid
	}
	var fk int
	if err = tx.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		return ErrInvalid
	}
	if err = verifyReference(ctx, tx); err != nil {
		return err
	}
	// SQLite represents an implicit BEFORE INSERT rowid as -1. Existing
	// non-positive IDs must be rejected before the collision guards are added.
	var nonPositive int
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ipam_resource_ids WHERE rowid<=0 UNION ALL SELECT 1 FROM ipam_tenants WHERE rowid<=0 UNION ALL SELECT 1 FROM ipam_spaces WHERE rowid<=0 UNION ALL SELECT 1 FROM ipam_prefixes WHERE rowid<=0 UNION ALL SELECT 1 FROM ipam_pools WHERE rowid<=0 UNION ALL SELECT 1 FROM ipam_occupancies WHERE rowid<=0)`).Scan(&nonPositive)
	if err != nil {
		return err
	}
	if nonPositive != 0 {
		return fmt.Errorf("%w: non-positive IPAM rowid", ErrInvalid)
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM ipam_occupancies").Scan(&count); err != nil || count != 0 {
		return ErrInvalid
	}
	if _, err = tx.ExecContext(ctx, "SAVEPOINT dhcpholds_install"); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if _, e := tx.ExecContext(cleanup, "ROLLBACK TO dhcpholds_install"); e != nil {
				err = errors.Join(err, fmt.Errorf("dhcpholds: rollback savepoint failed: %w", e))
			} else if _, e := tx.ExecContext(cleanup, "RELEASE dhcpholds_install"); e != nil {
				err = errors.Join(err, fmt.Errorf("dhcpholds: release savepoint failed: %w", e))
			}
		}
	}()
	if _, err = tx.ExecContext(ctx, ddl); err != nil {
		return err
	}
	for _, m := range mappings {
		if m.Scope == "" || m.Tenant == "" || m.Space == "" || m.Subnet == "" {
			return ErrInvalid
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO dhcp_scope_map(scope,tenant_id,space_id,subnet_id) VALUES (?,?,?,?)`, m.Scope, m.Tenant, m.Space, m.Subnet); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `CREATE TRIGGER dhcp_map_no_insert BEFORE INSERT ON dhcp_scope_map BEGIN SELECT RAISE(ABORT,'DHCP_SCOPE_IMMUTABLE'); END`); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "RELEASE dhcpholds_install")
	return err
}

// Create takes a trusted, already resolved scope, never tenant/space from a
// client packet. It reserves the address but proves no permission or lease.
func Create(ctx context.Context, tx *sql.Tx, scope, lease string, family int, address []byte, state string) error {
	if tx == nil {
		return ErrInvalid
	}
	var fk int
	if err := tx.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		return ErrInvalid
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO dhcp_address_holds(scope,tenant_id,space_id,subnet_id,lease_id,family,address,state,revision)
 SELECT scope,tenant_id,space_id,subnet_id,?,?,?,?,1 FROM dhcp_scope_map WHERE scope=?`, lease, family, address, state, scope)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalid
	}
	return nil
}

// Verify every reference object, not merely a user-editable migration marker.
func verifyReference(ctx context.Context, tx *sql.Tx) error {
	paths := ipamstore.MigrationPaths()
	if len(paths) != 2 || paths[0] != "migrations/001_ipam.sql" || paths[1] != "migrations/002_rowid_guards.sql" {
		return ErrInvalid
	}
	if err := ipamstore.VerifyDigests(); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, path := range paths {
		text, e := ipamstore.ContentsOf(path)
		if e != nil {
			return e
		}
		if _, e = db.ExecContext(ctx, text); e != nil {
			return e
		}
	}
	want, err := protectedSchema(ctx, db, "sqlite_schema")
	if err != nil {
		return err
	}
	got, err := protectedSchema(ctx, tx, "sqlite_schema")
	if err != nil {
		return err
	}
	temporary, err := protectedSchema(ctx, tx, "sqlite_temp_schema")
	if err != nil {
		return err
	}
	if len(temporary) != 0 || len(got) != len(want) {
		return fmt.Errorf("%w: unexpected or missing IPAM schema object", ErrInvalid)
	}
	for key, expected := range want {
		if actual, ok := got[key]; !ok || actual != expected {
			return fmt.Errorf("%w: missing or changed reference schema %s", ErrInvalid, key)
		}
	}
	return nil
}

type schemaReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type schemaObject struct{ table, definition string }

// schema is an internal constant selected by verifyReference, never caller input.
func protectedSchema(ctx context.Context, q schemaReader, schema string) (map[string]schemaObject, error) {
	query := `SELECT type,name,tbl_name,coalesce(sql,'') FROM ` + schema + `
	 WHERE lower(substr(name,1,5))='ipam_' OR
	 (type IN ('trigger','index') AND lower(substr(tbl_name,1,5))='ipam_')`
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	objects := make(map[string]schemaObject)
	for rows.Next() {
		var kind, name, table, definition string
		if err := rows.Scan(&kind, &name, &table, &definition); err != nil {
			return nil, err
		}
		objects[kind+":"+name] = schemaObject{table, definition}
	}
	return objects, rows.Err()
}

const ddl = `
CREATE TABLE dhcp_scope_map (
 scope TEXT PRIMARY KEY CHECK(scope<>''), tenant_id TEXT NOT NULL, space_id TEXT NOT NULL, subnet_id TEXT NOT NULL,
 UNIQUE(scope,tenant_id,space_id,subnet_id),
 FOREIGN KEY(tenant_id,space_id,subnet_id) REFERENCES ipam_prefixes(tenant_id,space_id,id) ON DELETE RESTRICT
) STRICT, WITHOUT ROWID;
CREATE TRIGGER dhcp_map_subnet BEFORE INSERT ON dhcp_scope_map BEGIN
 SELECT RAISE(ABORT,'DHCP_SCOPE_INVALID') WHERE EXISTS(SELECT 1 FROM dhcp_scope_map WHERE scope=NEW.scope)
 OR NOT EXISTS(SELECT 1 FROM ipam_prefixes WHERE tenant_id=NEW.tenant_id AND space_id=NEW.space_id AND id=NEW.subnet_id AND kind='SUBNET' AND family=4); END;
CREATE TRIGGER dhcp_map_no_update BEFORE UPDATE ON dhcp_scope_map BEGIN SELECT RAISE(ABORT,'DHCP_SCOPE_IMMUTABLE'); END;
CREATE TRIGGER dhcp_map_no_delete BEFORE DELETE ON dhcp_scope_map BEGIN SELECT RAISE(ABORT,'DHCP_SCOPE_IMMUTABLE'); END;
CREATE TABLE dhcp_address_holds (
 scope TEXT NOT NULL, tenant_id TEXT NOT NULL, space_id TEXT NOT NULL, subnet_id TEXT NOT NULL,
 lease_id TEXT NOT NULL CHECK(lease_id<>''), family INTEGER NOT NULL CHECK(family=4),
 address BLOB NOT NULL CHECK(length(address)=16), state TEXT NOT NULL CHECK(state IN ('OFFER','ACTIVE','QUARANTINE')),
 revision INTEGER NOT NULL DEFAULT 1 CHECK(revision>0),
 PRIMARY KEY(scope,lease_id), UNIQUE(tenant_id,space_id,family,address),
 FOREIGN KEY(scope,tenant_id,space_id,subnet_id) REFERENCES dhcp_scope_map(scope,tenant_id,space_id,subnet_id) ON DELETE RESTRICT,
 CHECK(family<>4 OR substr(address,1,12)=zeroblob(12)),
 CHECK(family<>6 OR substr(address,1,12)<>x'00000000000000000000FFFF')
) STRICT, WITHOUT ROWID;
CREATE TRIGGER dhcp_hold_insert BEFORE INSERT ON dhcp_address_holds BEGIN
 SELECT RAISE(ABORT,'DHCP_HOLD_EXISTS') WHERE EXISTS(SELECT 1 FROM dhcp_address_holds h WHERE (h.scope=NEW.scope AND h.lease_id=NEW.lease_id) OR (h.tenant_id=NEW.tenant_id AND h.space_id=NEW.space_id AND h.family=NEW.family AND h.address=NEW.address));
 SELECT RAISE(ABORT,'DHCP_ADDRESS_OCCUPIED') WHERE EXISTS(SELECT 1 FROM ipam_occupancies a WHERE a.tenant_id=NEW.tenant_id AND a.space_id=NEW.space_id AND a.family=NEW.family AND a.address=NEW.address);
 SELECT RAISE(ABORT,'DHCP_ADDRESS_OUTSIDE_SUBNET') WHERE NOT EXISTS(SELECT 1 FROM ipam_prefixes p WHERE p.tenant_id=NEW.tenant_id AND p.space_id=NEW.space_id AND p.id=NEW.subnet_id AND p.kind='SUBNET' AND p.family=NEW.family AND NEW.address BETWEEN p.first_ip AND p.last_ip AND (p.usage IN ('P2P','HOST') OR (NEW.address<>p.first_ip AND (NEW.family=6 OR NEW.address<>p.last_ip)))); END;
CREATE TRIGGER dhcp_hold_update BEFORE UPDATE ON dhcp_address_holds WHEN NEW.scope IS NOT OLD.scope OR NEW.tenant_id IS NOT OLD.tenant_id OR NEW.space_id IS NOT OLD.space_id OR NEW.subnet_id IS NOT OLD.subnet_id OR NEW.lease_id IS NOT OLD.lease_id OR NEW.family IS NOT OLD.family OR NEW.address IS NOT OLD.address OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 OR NOT ((OLD.state='OFFER' AND NEW.state IN ('ACTIVE','QUARANTINE')) OR (OLD.state='ACTIVE' AND NEW.state='QUARANTINE')) BEGIN SELECT RAISE(ABORT,'DHCP_HOLD_TRANSITION_INVALID'); END;
CREATE TRIGGER dhcp_hold_no_delete BEFORE DELETE ON dhcp_address_holds BEGIN SELECT RAISE(ABORT,'DHCP_HOLD_RELEASE_UNSUPPORTED'); END;
CREATE TRIGGER dhcp_ipam_insert BEFORE INSERT ON ipam_occupancies WHEN EXISTS(SELECT 1 FROM dhcp_address_holds h WHERE h.tenant_id=NEW.tenant_id AND h.space_id=NEW.space_id AND h.family=NEW.family AND h.address=NEW.address) BEGIN SELECT RAISE(ABORT,'DHCP_ADDRESS_HELD'); END;
CREATE TRIGGER dhcp_ipam_update BEFORE UPDATE ON ipam_occupancies WHEN EXISTS(SELECT 1 FROM dhcp_address_holds h WHERE h.tenant_id=NEW.tenant_id AND h.space_id=NEW.space_id AND h.family=NEW.family AND h.address=NEW.address) BEGIN SELECT RAISE(ABORT,'DHCP_ADDRESS_HELD'); END;
`
