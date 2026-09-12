-- +goose Up
-- Authorship of a DNS record, and the three queues this store owes upward.
--
-- ---------------------------------------------------------------------------
-- 1. authored_locally
-- ---------------------------------------------------------------------------
-- Mirrors the control schema. A row with authored_locally = 1 was written by
-- this data plane -- a DHCP binding publishing its name, an RFC 2136 update
-- from an authorised client, or an inbound zone transfer -- and the control
-- database's copy of it exists only so the console can show it.
--
-- The downward sync uses this column as its boundary: it replaces the rows the
-- control plane authored and leaves the locally authored ones alone. Without
-- that split, every configuration change anywhere would wipe the records this
-- process is serving, which is the whole reason the split exists.
--
-- ---------------------------------------------------------------------------
-- 2. What this store owes upward, and why leases were not enough
-- ---------------------------------------------------------------------------
-- The lease push covers a lease's state. It does not cover:
--
--   dhcp_dns_events  the work the DNS side has not done yet. Until it is copied
--                    up, a DNS process in another file system cannot know a
--                    record is owed, and the split deployment has no DDNS at
--                    all.
--   dhcp_logs        the event log the console shows. It was only ever copied
--                    during the one-off takeover, so in a split deployment the
--                    console's view of DHCP activity stops at the upgrade.
--   dns_records      the rows authored here. The console reads the control
--                    database, so a record this process publishes is invisible
--                    there -- and invisibly absent from the console is worse
--                    than absent, because nothing tells the operator to look.
--
-- Each has its own marker table rather than a shared one, because each has a
-- different key and a different notion of "gone". Markers are what keep the
-- cost proportional to the rate of change instead of to the size of the table:
-- the lease table is only the largest of the three.

ALTER TABLE dns_records ADD COLUMN authored_locally INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_dp_dns_records_authored_locally ON dns_records(authored_locally);

CREATE TABLE IF NOT EXISTS dhcp_dns_event_dirty (
    event_id  INTEGER PRIMARY KEY,
    queued_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_dns_event_dirty_insert
AFTER INSERT ON dhcp_dns_events
BEGIN
    INSERT INTO dhcp_dns_event_dirty (event_id, queued_at)
    VALUES (NEW.id, datetime('now'))
    ON CONFLICT(event_id) DO UPDATE SET queued_at = datetime('now');
END;
-- +goose StatementEnd

-- The retry state is not something the DNS side is told about -- it keeps its
-- own -- but a row whose status changed is a row this store has already sent
-- once, and re-sending it is cheap next to the alternative of a consumer
-- retrying an event the producer has already abandoned.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_dns_event_dirty_update
AFTER UPDATE ON dhcp_dns_events
BEGIN
    INSERT INTO dhcp_dns_event_dirty (event_id, queued_at)
    VALUES (NEW.id, datetime('now'))
    ON CONFLICT(event_id) DO UPDATE SET queued_at = datetime('now');
END;
-- +goose StatementEnd

-- dhcp_logs is append-only: nothing in the codebase updates or deletes a row.
-- A trigger for the other two events would be dead weight that reads as
-- coverage.
CREATE TABLE IF NOT EXISTS dhcp_log_dirty (
    log_id    TEXT PRIMARY KEY,
    queued_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_dhcp_log_dirty_insert
AFTER INSERT ON dhcp_logs
BEGIN
    INSERT INTO dhcp_log_dirty (log_id, queued_at)
    VALUES (NEW.id, datetime('now'))
    ON CONFLICT(log_id) DO UPDATE SET queued_at = datetime('now');
END;
-- +goose StatementEnd

-- Only rows this process authored are owed upward. A control-authored row
-- landing in this table is a downward sync in progress, and pushing it back
-- would be a round trip that tells the control plane nothing.
--
-- Keyed by the record's own id rather than by rowid: the delete trigger has no
-- NEW row to read, so a rowid-keyed marker would have to carry the id beside it
-- and the two could disagree.
CREATE TABLE IF NOT EXISTS dns_record_dirty (
    record_id TEXT PRIMARY KEY,
    deleted   INTEGER NOT NULL DEFAULT 0,
    queued_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_dns_record_dirty_insert
AFTER INSERT ON dns_records
WHEN NEW.authored_locally = 1
BEGIN
    INSERT INTO dns_record_dirty (record_id, deleted, queued_at)
    VALUES (NEW.id, 0, datetime('now'))
    ON CONFLICT(record_id) DO UPDATE SET deleted = 0, queued_at = datetime('now');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_dns_record_dirty_update
AFTER UPDATE ON dns_records
WHEN NEW.authored_locally = 1
BEGIN
    INSERT INTO dns_record_dirty (record_id, deleted, queued_at)
    VALUES (NEW.id, 0, datetime('now'))
    ON CONFLICT(record_id) DO UPDATE SET deleted = 0, queued_at = datetime('now');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_dns_record_dirty_delete
AFTER DELETE ON dns_records
WHEN OLD.authored_locally = 1
BEGIN
    INSERT INTO dns_record_dirty (record_id, deleted, queued_at)
    VALUES (OLD.id, 1, datetime('now'))
    ON CONFLICT(record_id) DO UPDATE SET deleted = 1, queued_at = datetime('now');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_dns_record_dirty_delete;
DROP TRIGGER IF EXISTS trg_dp_dns_record_dirty_update;
DROP TRIGGER IF EXISTS trg_dp_dns_record_dirty_insert;
DROP TRIGGER IF EXISTS trg_dp_dhcp_log_dirty_insert;
DROP TRIGGER IF EXISTS trg_dp_dns_event_dirty_update;
DROP TRIGGER IF EXISTS trg_dp_dns_event_dirty_insert;
DROP TABLE IF EXISTS dns_record_dirty;
DROP TABLE IF EXISTS dhcp_log_dirty;
DROP TABLE IF EXISTS dhcp_dns_event_dirty;
DROP INDEX IF EXISTS idx_dp_dns_records_authored_locally;
ALTER TABLE dns_records DROP COLUMN authored_locally;
