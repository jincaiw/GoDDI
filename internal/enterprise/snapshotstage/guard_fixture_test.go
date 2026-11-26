package snapshotstage

// This independently pinned SQL constructs a guarded reference only in the
// freshly initialized, empty t.TempDir database used by tests. It is neither
// an upgrade of existing databases nor an application installation mechanism.
import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
)

const fixtureGuardSQL = `CREATE TRIGGER writer_authority_rowid_insert_guard BEFORE INSERT ON writer_authority WHEN EXISTS (SELECT 1 FROM writer_authority WHERE rowid=NEW.rowid) BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_CONFLICT'); END;
CREATE TRIGGER writer_authority_rowid_positive_guard AFTER INSERT ON writer_authority WHEN NEW.rowid<=0 BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_NONPOSITIVE'); END;
CREATE TRIGGER writer_authority_rowid_update_guard BEFORE UPDATE ON writer_authority WHEN NEW.rowid IS NOT OLD.rowid BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_IMMUTABLE'); END;
CREATE TRIGGER replica_head_rowid_insert_guard BEFORE INSERT ON replica_head WHEN EXISTS (SELECT 1 FROM replica_head WHERE rowid=NEW.rowid) BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_CONFLICT'); END;
CREATE TRIGGER replica_head_rowid_positive_guard AFTER INSERT ON replica_head WHEN NEW.rowid<=0 BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_NONPOSITIVE'); END;
CREATE TRIGGER replica_head_rowid_update_guard BEFORE UPDATE ON replica_head WHEN NEW.rowid IS NOT OLD.rowid BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_IMMUTABLE'); END;
CREATE TRIGGER replica_event_rowid_insert_guard BEFORE INSERT ON replica_event WHEN EXISTS (SELECT 1 FROM replica_event WHERE rowid=NEW.rowid) BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_CONFLICT'); END;
CREATE TRIGGER replica_event_rowid_positive_guard AFTER INSERT ON replica_event WHEN NEW.rowid<=0 BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_NONPOSITIVE'); END;
CREATE TRIGGER replica_event_rowid_update_guard BEFORE UPDATE ON replica_event WHEN NEW.rowid IS NOT OLD.rowid BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_IMMUTABLE'); END;
CREATE TRIGGER replica_checkpoint_rowid_insert_guard BEFORE INSERT ON replica_checkpoint WHEN EXISTS (SELECT 1 FROM replica_checkpoint WHERE rowid=NEW.rowid) BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_CONFLICT'); END;
CREATE TRIGGER replica_checkpoint_rowid_positive_guard AFTER INSERT ON replica_checkpoint WHEN NEW.rowid<=0 BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_NONPOSITIVE'); END;
CREATE TRIGGER replica_checkpoint_rowid_update_guard BEFORE UPDATE ON replica_checkpoint WHEN NEW.rowid IS NOT OLD.rowid BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_IMMUTABLE'); END;
CREATE TRIGGER idempotency_receipt_rowid_insert_guard BEFORE INSERT ON idempotency_receipt WHEN EXISTS (SELECT 1 FROM idempotency_receipt WHERE rowid=NEW.rowid) BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_CONFLICT'); END;
CREATE TRIGGER idempotency_receipt_rowid_positive_guard AFTER INSERT ON idempotency_receipt WHEN NEW.rowid<=0 BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_NONPOSITIVE'); END;
CREATE TRIGGER idempotency_receipt_rowid_update_guard BEFORE UPDATE ON idempotency_receipt WHEN NEW.rowid IS NOT OLD.rowid BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_IMMUTABLE'); END;
CREATE TRIGGER static_occupancy_audit_rowid_insert_guard BEFORE INSERT ON static_occupancy_audit WHEN EXISTS (SELECT 1 FROM static_occupancy_audit WHERE rowid=NEW.rowid) BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_CONFLICT'); END;
CREATE TRIGGER static_occupancy_audit_rowid_positive_guard AFTER INSERT ON static_occupancy_audit WHEN NEW.rowid<=0 BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_NONPOSITIVE'); END;
CREATE TRIGGER static_occupancy_audit_rowid_update_guard BEFORE UPDATE ON static_occupancy_audit WHEN NEW.rowid IS NOT OLD.rowid BEGIN SELECT RAISE(ABORT,'EVENTLOG_ROWID_IMMUTABLE'); END;
CREATE TRIGGER writer_authority_logical_insert_guard BEFORE INSERT ON writer_authority WHEN EXISTS (SELECT 1 FROM writer_authority t WHERE t.cluster_id=NEW.cluster_id) BEGIN SELECT RAISE(ABORT,'EVENTLOG_KEY_CONFLICT'); END;
CREATE TRIGGER writer_authority_logical_update_guard BEFORE UPDATE ON writer_authority WHEN NEW.cluster_id IS NOT OLD.cluster_id BEGIN SELECT RAISE(ABORT,'EVENTLOG_KEY_IMMUTABLE'); END;
CREATE TRIGGER replica_head_logical_insert_guard BEFORE INSERT ON replica_head WHEN EXISTS (SELECT 1 FROM replica_head t WHERE t.cluster_id=NEW.cluster_id) BEGIN SELECT RAISE(ABORT,'EVENTLOG_KEY_CONFLICT'); END;
CREATE TRIGGER replica_head_logical_update_guard BEFORE UPDATE ON replica_head WHEN NEW.cluster_id IS NOT OLD.cluster_id BEGIN SELECT RAISE(ABORT,'EVENTLOG_KEY_IMMUTABLE'); END;
CREATE TRIGGER replica_checkpoint_logical_insert_guard BEFORE INSERT ON replica_checkpoint WHEN EXISTS (SELECT 1 FROM replica_checkpoint t WHERE t.cluster_id=NEW.cluster_id AND t.source_id=NEW.source_id) BEGIN SELECT RAISE(ABORT,'EVENTLOG_KEY_CONFLICT'); END;
CREATE TRIGGER replica_checkpoint_logical_update_guard BEFORE UPDATE ON replica_checkpoint WHEN NEW.cluster_id IS NOT OLD.cluster_id OR NEW.source_id IS NOT OLD.source_id BEGIN SELECT RAISE(ABORT,'EVENTLOG_KEY_IMMUTABLE'); END;
CREATE TRIGGER writer_authority_cluster_type_guard BEFORE INSERT ON writer_authority WHEN typeof(NEW.cluster_id)<>'blob' OR length(NEW.cluster_id)<>16 BEGIN SELECT RAISE(ABORT,'EVENTLOG_CLUSTER_INVALID'); END;
CREATE TRIGGER replica_head_cluster_type_guard BEFORE INSERT ON replica_head WHEN typeof(NEW.cluster_id)<>'blob' OR length(NEW.cluster_id)<>16 BEGIN SELECT RAISE(ABORT,'EVENTLOG_CLUSTER_INVALID'); END;
CREATE TRIGGER replica_checkpoint_cluster_type_guard BEFORE INSERT ON replica_checkpoint WHEN typeof(NEW.cluster_id)<>'blob' OR length(NEW.cluster_id)<>16 OR typeof(NEW.source_id)<>'blob' OR length(NEW.source_id)<>16 BEGIN SELECT RAISE(ABORT,'EVENTLOG_CLUSTER_INVALID'); END;
CREATE TRIGGER idempotency_receipt_key_type_guard BEFORE INSERT ON idempotency_receipt WHEN typeof(NEW.tenant_id)<>'blob' OR length(NEW.tenant_id)<>16 OR typeof(NEW.principal)<>'text' OR typeof(NEW.method)<>'text' OR typeof(NEW.path)<>'text' OR typeof(NEW.idempotency_key)<>'text' BEGIN SELECT RAISE(ABORT,'EVENTLOG_KEY_INVALID'); END;
`
const fixtureGuardDigest = "1a3a8ade8fbde4efe8d891fec9e8bad467da54bc18ae6efe0e8343a3aeedc644"
const fixtureEventDigest = "ba7fae87d6a4d8706e0564916b372c45ec77aabd61462e7a2f42ac6ea55f8d98"
const fixtureGuardMarker = `CREATE TABLE eventlog_reference_guard_migrations (version INTEGER PRIMARY KEY, base_digest TEXT NOT NULL, guard_digest TEXT NOT NULL) STRICT`

func initEmptyGuardedFixture(ctx context.Context, db *sql.DB) error {
	if fmt.Sprintf("%x", sha256.Sum256([]byte(fixtureGuardSQL))) != fixtureGuardDigest || fmt.Sprintf("%x", sha256.Sum256([]byte(eventlog.Schema))) != fixtureEventDigest {
		return errFixture
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.ipam_tenants UNION ALL SELECT 1 FROM main.ipam_spaces UNION ALL SELECT 1 FROM main.ipam_prefixes UNION ALL SELECT 1 FROM main.ipam_pools UNION ALL SELECT 1 FROM main.ipam_occupancies UNION ALL SELECT 1 FROM main.ipam_resource_ids UNION ALL SELECT 1 FROM main.writer_authority UNION ALL SELECT 1 FROM main.replica_head UNION ALL SELECT 1 FROM main.replica_event UNION ALL SELECT 1 FROM main.replica_checkpoint UNION ALL SELECT 1 FROM main.idempotency_receipt UNION ALL SELECT 1 FROM main.static_occupancy_audit)`).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return errFixture
	}
	if _, err = tx.ExecContext(ctx, fixtureGuardSQL); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, fixtureGuardMarker); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO main.eventlog_reference_guard_migrations VALUES (1,?,?)`, fixtureEventDigest, fixtureGuardDigest)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errFixture
	}
	return tx.Commit()
}
