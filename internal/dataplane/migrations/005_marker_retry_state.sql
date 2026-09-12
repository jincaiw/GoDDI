-- +goose Up
-- Data-plane migration 005: what happened when a queued change was refused.
--
-- ---------------------------------------------------------------------------
-- The defect
-- ---------------------------------------------------------------------------
-- Every upward queue was written the same way: read a batch of markers, copy
-- the rows up inside one transaction, and return on the first error. The
-- markers are only cleared after the transaction commits, so a single row the
-- control database will not accept -- one lease whose scope has not arrived
-- yet, say -- left every marker in the batch in place. The next pass read the
-- same batch, hit the same row and returned at the same place. The queue did
-- not stall one row: it stalled every row behind it, permanently, while the
-- runner reported it once a second as a database error.
--
-- ---------------------------------------------------------------------------
-- What is added, and what deliberately is not
-- ---------------------------------------------------------------------------
-- Three columns, on each of the six marker tables, describing the last refusal
-- for the row the marker names:
--
--   attempts        how many consecutive refusals this row has collected
--   next_attempt_at when it becomes eligible again; NULL is "now"
--   last_error      what the control database said
--
-- No trigger is touched. The eleven triggers that write these tables say one
-- thing -- "this row changed, it is owed again" -- and the retry state says
-- something else, about delivery. Making the trigger's ON CONFLICT clause also
-- reset a retry counter would couple the statement that observes a change to
-- the machinery that paces its delivery, and the next person to add a marker
-- table would have to know that.
--
-- The three columns live on the marker rather than in one shared failures
-- table because their lifetime is the marker's lifetime. Cleared marker, gone
-- row; the same DELETE that ends the work ends the record of the failures
-- getting there. A second table would need its own cleanup at every point a
-- marker is removed, and a missed one is an orphan that reads as a permanently
-- stuck queue.
--
-- next_attempt_at is NULL-able and has no default: SQLite refuses to add a
-- column whose default is not a constant, and "due now" is what NULL already
-- means. Every read of it is written with the NULL case spelled out, because
-- NULL does not compare: `julianday(NULL) <= julianday('now')` is NULL, not
-- true, and a filter relying on it silently skips every row that has never
-- failed.
--
-- ---------------------------------------------------------------------------
-- Why the delay has a ceiling rather than a terminal state
-- ---------------------------------------------------------------------------
-- A row that has been refused is retried forever, at a widening interval
-- (1s, 5s, 15s, 1m, then every 5m). It is never written off. There is no
-- operator action anywhere in this build that revives a written-off row, so a
-- terminal "failed" state would trade one silent failure -- a blocked queue --
-- for a different silent one: rows that stopped being delivered and can only be
-- cleared by editing a database by hand. A slow retry has neither problem: the
-- cause is usually a downward sync that has not landed yet, and it fixes
-- itself.
--
-- The cost is stated rather than hidden: the ceiling means a row whose cause is
-- repaired can take up to five minutes to travel. That is the deliberate
-- trade, and it is why the retry state is counted and reported (an operator
-- sees how many rows are waiting) instead of being invisible.

ALTER TABLE dhcp_lease_dirty     ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dhcp_lease_dirty     ADD COLUMN next_attempt_at DATETIME;
ALTER TABLE dhcp_lease_dirty     ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE dhcp_dns_event_dirty ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dhcp_dns_event_dirty ADD COLUMN next_attempt_at DATETIME;
ALTER TABLE dhcp_dns_event_dirty ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE dhcp_log_dirty       ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dhcp_log_dirty       ADD COLUMN next_attempt_at DATETIME;
ALTER TABLE dhcp_log_dirty       ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE dns_record_dirty     ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dns_record_dirty     ADD COLUMN next_attempt_at DATETIME;
ALTER TABLE dns_record_dirty     ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE dns_zone_serial_dirty ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dns_zone_serial_dirty ADD COLUMN next_attempt_at DATETIME;
ALTER TABLE dns_zone_serial_dirty ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE audit_log_dirty      ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audit_log_dirty      ADD COLUMN next_attempt_at DATETIME;
ALTER TABLE audit_log_dirty      ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE audit_log_dirty      DROP COLUMN last_error;
ALTER TABLE audit_log_dirty      DROP COLUMN next_attempt_at;
ALTER TABLE audit_log_dirty      DROP COLUMN attempts;

ALTER TABLE dns_zone_serial_dirty DROP COLUMN last_error;
ALTER TABLE dns_zone_serial_dirty DROP COLUMN next_attempt_at;
ALTER TABLE dns_zone_serial_dirty DROP COLUMN attempts;

ALTER TABLE dns_record_dirty     DROP COLUMN last_error;
ALTER TABLE dns_record_dirty     DROP COLUMN next_attempt_at;
ALTER TABLE dns_record_dirty     DROP COLUMN attempts;

ALTER TABLE dhcp_log_dirty       DROP COLUMN last_error;
ALTER TABLE dhcp_log_dirty       DROP COLUMN next_attempt_at;
ALTER TABLE dhcp_log_dirty       DROP COLUMN attempts;

ALTER TABLE dhcp_dns_event_dirty DROP COLUMN last_error;
ALTER TABLE dhcp_dns_event_dirty DROP COLUMN next_attempt_at;
ALTER TABLE dhcp_dns_event_dirty DROP COLUMN attempts;

ALTER TABLE dhcp_lease_dirty     DROP COLUMN last_error;
ALTER TABLE dhcp_lease_dirty     DROP COLUMN next_attempt_at;
ALTER TABLE dhcp_lease_dirty     DROP COLUMN attempts;
