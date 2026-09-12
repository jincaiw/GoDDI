-- +goose Up
-- Data-plane migration 004: the audit rows this plane owes upward.
--
-- ---------------------------------------------------------------------------
-- Why the data plane needs its own audit table at all
-- ---------------------------------------------------------------------------
-- The two writes that most need a trail are the ones with nobody watching
-- them: an RFC 2136 update accepted from a client, and a record the DHCP side
-- published for a binding it confirmed. Both are made by the plane that serves
-- them, against its own store.
--
-- Before this table existed, the RFC 2136 handler wrote to audit_logs anyway --
-- into a store that had no such table. Every one of those writes failed, and
-- every one failed silently, because the handler discards the error after
-- logging it. A control that only ever runs in a configuration where it cannot
-- work is not a control.
--
-- ---------------------------------------------------------------------------
-- Why this copy is a queue and not an archive
-- ---------------------------------------------------------------------------
-- The control database holds the archive, with a trigger that refuses UPDATE
-- and DELETE. This copy carries no such trigger: its rows are written, pushed
-- up and then superseded by nothing, so the only thing a local delete trigger
-- would prevent is the push marking its own work done.
--
-- Columns mirror the control schema, including the old_value / new_value pair
-- migration 024 adds there. A mirror that is a column behind is how a push
-- silently drops the field it was added for.

CREATE TABLE IF NOT EXISTS audit_logs (
    id            TEXT PRIMARY KEY,
    user_id       TEXT,
    username      TEXT,
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT,
    detail        TEXT,
    old_value     TEXT,
    new_value     TEXT,
    source_ip     TEXT,
    user_agent    TEXT,
    success       BOOLEAN DEFAULT TRUE,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- audit_logs is append-only on this side too, which is why the marker table has
-- no delete trigger to go with it: nothing in this process removes a row, and a
-- trigger guarding a statement that does not exist reads as coverage it is not.
CREATE TABLE IF NOT EXISTS audit_log_dirty (
    log_id    TEXT PRIMARY KEY,
    queued_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_audit_log_dirty_insert
AFTER INSERT ON audit_logs
BEGIN
    INSERT INTO audit_log_dirty (log_id, queued_at)
    VALUES (NEW.id, datetime('now'))
    ON CONFLICT(log_id) DO UPDATE SET queued_at = datetime('now');
END;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_dp_audit_log_dirty_queued ON audit_log_dirty(queued_at);

-- +goose Down
DROP INDEX IF EXISTS idx_dp_audit_log_dirty_queued;
DROP TRIGGER IF EXISTS trg_dp_audit_log_dirty_insert;
DROP TABLE IF EXISTS audit_log_dirty;
DROP TABLE IF EXISTS audit_logs;
