-- Isolated reference schema only. Migration runner verifies all preconditions.
BEGIN IMMEDIATE;

CREATE TRIGGER ipam_resource_ids_rowid_insert_guard BEFORE INSERT ON ipam_resource_ids
WHEN EXISTS (SELECT 1 FROM ipam_resource_ids WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_CONFLICT'); END;

CREATE TRIGGER ipam_resource_ids_rowid_positive_guard AFTER INSERT ON ipam_resource_ids
WHEN NEW.rowid <= 0
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_NONPOSITIVE'); END;

CREATE TRIGGER ipam_resource_ids_rowid_update_guard BEFORE UPDATE ON ipam_resource_ids
WHEN NEW.rowid IS NOT OLD.rowid
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_IMMUTABLE'); END;

CREATE TRIGGER ipam_tenants_rowid_insert_guard BEFORE INSERT ON ipam_tenants
WHEN EXISTS (SELECT 1 FROM ipam_tenants WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_CONFLICT'); END;

CREATE TRIGGER ipam_tenants_rowid_positive_guard AFTER INSERT ON ipam_tenants
WHEN NEW.rowid <= 0
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_NONPOSITIVE'); END;

CREATE TRIGGER ipam_tenants_rowid_update_guard BEFORE UPDATE ON ipam_tenants
WHEN NEW.rowid IS NOT OLD.rowid
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_IMMUTABLE'); END;

CREATE TRIGGER ipam_spaces_rowid_insert_guard BEFORE INSERT ON ipam_spaces
WHEN EXISTS (SELECT 1 FROM ipam_spaces WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_CONFLICT'); END;

CREATE TRIGGER ipam_spaces_rowid_positive_guard AFTER INSERT ON ipam_spaces
WHEN NEW.rowid <= 0
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_NONPOSITIVE'); END;

CREATE TRIGGER ipam_spaces_rowid_update_guard BEFORE UPDATE ON ipam_spaces
WHEN NEW.rowid IS NOT OLD.rowid
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_IMMUTABLE'); END;

CREATE TRIGGER ipam_prefixes_rowid_insert_guard BEFORE INSERT ON ipam_prefixes
WHEN EXISTS (SELECT 1 FROM ipam_prefixes WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_CONFLICT'); END;

CREATE TRIGGER ipam_prefixes_rowid_positive_guard AFTER INSERT ON ipam_prefixes
WHEN NEW.rowid <= 0
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_NONPOSITIVE'); END;

CREATE TRIGGER ipam_prefixes_rowid_update_guard BEFORE UPDATE ON ipam_prefixes
WHEN NEW.rowid IS NOT OLD.rowid
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_IMMUTABLE'); END;

CREATE TRIGGER ipam_pools_rowid_insert_guard BEFORE INSERT ON ipam_pools
WHEN EXISTS (SELECT 1 FROM ipam_pools WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_CONFLICT'); END;

CREATE TRIGGER ipam_pools_rowid_positive_guard AFTER INSERT ON ipam_pools
WHEN NEW.rowid <= 0
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_NONPOSITIVE'); END;

CREATE TRIGGER ipam_pools_rowid_update_guard BEFORE UPDATE ON ipam_pools
WHEN NEW.rowid IS NOT OLD.rowid
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_IMMUTABLE'); END;

CREATE TRIGGER ipam_occupancies_rowid_insert_guard BEFORE INSERT ON ipam_occupancies
WHEN EXISTS (SELECT 1 FROM ipam_occupancies WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_CONFLICT'); END;

CREATE TRIGGER ipam_occupancies_rowid_positive_guard AFTER INSERT ON ipam_occupancies
WHEN NEW.rowid <= 0
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_NONPOSITIVE'); END;

CREATE TRIGGER ipam_occupancies_rowid_update_guard BEFORE UPDATE ON ipam_occupancies
WHEN NEW.rowid IS NOT OLD.rowid
BEGIN SELECT RAISE(ABORT, 'IPAM_ROWID_IMMUTABLE'); END;

COMMIT;
