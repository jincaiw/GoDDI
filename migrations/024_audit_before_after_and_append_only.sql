-- Migration 024: the audit trail learns what changed, and stops being editable.
--
-- ---------------------------------------------------------------------------
-- 1. old_value / new_value
-- ---------------------------------------------------------------------------
-- An audit entry that says "a dynamic update was applied to zone example.test"
-- cannot answer the question anyone asks afterwards: which record, from what to
-- what. The HTTP middleware records a whitelisted excerpt of the *request*
-- because that is all a middleware can see; the data-plane writers hold the
-- rows themselves, so they record both sides.
--
-- NULL means "there was nothing on that side" — a create has no before, a
-- delete has no after. An empty string would read the same in a report and mean
-- something different.
--
-- ---------------------------------------------------------------------------
-- 2. Append-only, enforced by the database
-- ---------------------------------------------------------------------------
-- Nothing in the codebase updates or deletes these rows, and that is a
-- property of the current code rather than of the data. A trigger makes it a
-- property of the database, which is what survives the next refactor.
--
-- Deletion is permitted only while a row exists in audit_maintenance. A
-- genuinely append-only table grows without bound, and "the only way to prune
-- is to drop the trigger by hand" is how a table ends up never being pruned at
-- all. Opening the window is a deliberate act that leaves its own record:
--
--     INSERT INTO audit_maintenance (id, reason, opened_at)
--         VALUES (1, 'retention: older than 365 days', datetime('now'));
--     DELETE FROM audit_logs WHERE created_at < datetime('now', '-365 days');
--     DELETE FROM audit_maintenance WHERE id = 1;
--
-- The window is not enforced to be brief, because a long-running statement
-- would be; it is enforced to be visible.

-- +goose Up
ALTER TABLE audit_logs ADD COLUMN old_value TEXT;
ALTER TABLE audit_logs ADD COLUMN new_value TEXT;

CREATE TABLE IF NOT EXISTS audit_maintenance (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    reason    TEXT NOT NULL,
    opened_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- audit_logs is created in migration 005, which precedes this one; a trigger
-- body resolves the tables it references at creation time.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_audit_logs_no_update
BEFORE UPDATE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit_logs is append-only: an entry that can be edited is not a record of anything');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_audit_logs_no_delete
BEFORE DELETE ON audit_logs
WHEN NOT EXISTS (SELECT 1 FROM audit_maintenance WHERE id = 1)
BEGIN
    SELECT RAISE(ABORT, 'audit_logs is append-only: open a maintenance window to prune');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_audit_logs_no_delete;
DROP TRIGGER IF EXISTS trg_audit_logs_no_update;
DROP TABLE IF EXISTS audit_maintenance;
ALTER TABLE audit_logs DROP COLUMN new_value;
ALTER TABLE audit_logs DROP COLUMN old_value;
