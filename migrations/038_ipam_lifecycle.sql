-- +goose Up
-- IPAM parents gain a lifecycle state. Deleting a subnet or space is now a
-- retire transition: the row stays for FK integrity and snapshot replication,
-- while local-only ipam_addresses/ipam_history rows survive with it.
-- The guards below make a raw DELETE fail closed while children still exist;
-- backup restore deletes children first, so it remains valid.

ALTER TABLE ipam_spaces ADD COLUMN lifecycle_state TEXT NOT NULL DEFAULT 'active';
ALTER TABLE ipam_spaces ADD COLUMN retired_at DATETIME;
ALTER TABLE ipam_spaces ADD COLUMN retired_by TEXT NOT NULL DEFAULT '';
ALTER TABLE ipam_spaces ADD COLUMN retire_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE ipam_subnets ADD COLUMN lifecycle_state TEXT NOT NULL DEFAULT 'active';
ALTER TABLE ipam_subnets ADD COLUMN retired_at DATETIME;
ALTER TABLE ipam_subnets ADD COLUMN retired_by TEXT NOT NULL DEFAULT '';
ALTER TABLE ipam_subnets ADD COLUMN retire_reason TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_ipam_spaces_lifecycle_state ON ipam_spaces(lifecycle_state);
CREATE INDEX IF NOT EXISTS idx_ipam_subnets_lifecycle_state ON ipam_subnets(lifecycle_state);

-- +goose StatementBegin
CREATE TRIGGER ipam_spaces_delete_guard
BEFORE DELETE ON ipam_spaces
WHEN EXISTS (SELECT 1 FROM ipam_subnets WHERE space_id = OLD.id)
BEGIN
  SELECT RAISE(ABORT, 'ipam_spaces must be retired, not deleted while subnets exist');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER ipam_subnets_delete_guard
BEFORE DELETE ON ipam_subnets
WHEN EXISTS (SELECT 1 FROM ipam_addresses WHERE subnet_id = OLD.id)
  OR EXISTS (SELECT 1 FROM ipam_history WHERE subnet_id = OLD.id)
BEGIN
  SELECT RAISE(ABORT, 'ipam_subnets must be retired, not deleted while children exist');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS ipam_subnets_delete_guard;
DROP TRIGGER IF EXISTS ipam_spaces_delete_guard;
DROP INDEX IF EXISTS idx_ipam_subnets_lifecycle_state;
DROP INDEX IF EXISTS idx_ipam_spaces_lifecycle_state;
ALTER TABLE ipam_subnets DROP COLUMN retire_reason;
ALTER TABLE ipam_subnets DROP COLUMN retired_by;
ALTER TABLE ipam_subnets DROP COLUMN retired_at;
ALTER TABLE ipam_subnets DROP COLUMN lifecycle_state;
ALTER TABLE ipam_spaces DROP COLUMN retire_reason;
ALTER TABLE ipam_spaces DROP COLUMN retired_by;
ALTER TABLE ipam_spaces DROP COLUMN retired_at;
ALTER TABLE ipam_spaces DROP COLUMN lifecycle_state;
