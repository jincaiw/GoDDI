package replicarpc

// Fixed reference bytes are compiled only in tests; this is not a migration.
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
