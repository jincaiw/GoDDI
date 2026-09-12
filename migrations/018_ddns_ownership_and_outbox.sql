-- +goose Up
-- DHCP-driven DNS records need three things the previous schema could not
-- express.
--
-- 1. Ownership by lease, not by the string 'dhcp'.
--    Deletion matched on (zone, name, type, owner='dhcp'). Two clients that
--    announce the same hostname therefore share one name, and when the older
--    binding is torn down its cleanup removes the *newer* client's record,
--    leaving a live host unresolvable. owner_ref pins the record to the exact
--    lease that wrote it, so a teardown can only ever remove its own row.
--
-- 2. A generation counter, so a teardown decided before a renewal cannot
--    remove the record of the renewed binding ("a late renew must not be
--    resurrected, and a stale delete must not undo it"). dhcp_leases.generation
--    is bumped on every transition into a held state; a delete event carries
--    the generation it observed and only matches rows at or below it.
--
-- 3. A durable event log. Dispatching DNS updates from a goroutine meant a
--    crash between the ACK and the write lost the update silently, with no
--    record that it was ever owed. The event row is written before the DHCP
--    reply is sent and replayed by a consumer until it succeeds, so the work
--    survives a restart instead of depending on one in-flight goroutine.
ALTER TABLE dns_records ADD COLUMN owner_ref TEXT;
ALTER TABLE dns_records ADD COLUMN owner_generation INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_dns_records_owner_ref ON dns_records(owner_ref);

-- Existing leases start a fresh generation series. Every write path that
-- (re)establishes a binding increments this value, so generation 0 is never a
-- valid generation for a live binding.
ALTER TABLE dhcp_leases ADD COLUMN generation INTEGER NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS dhcp_dns_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    lease_id        TEXT    NOT NULL,
    generation      INTEGER NOT NULL,
    action          TEXT    NOT NULL,
    scope_id        TEXT    NOT NULL DEFAULT '',
    ip_address      TEXT    NOT NULL DEFAULT '',
    mac_address     TEXT    NOT NULL DEFAULT '',
    hostname        TEXT    NOT NULL DEFAULT '',
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_error      TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'pending',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- The consumer drains in id order so a create is never applied after the
-- delete that superseded it.
CREATE INDEX IF NOT EXISTS idx_dhcp_dns_events_pending
    ON dhcp_dns_events(status, next_attempt_at, id);
CREATE INDEX IF NOT EXISTS idx_dhcp_dns_events_lease
    ON dhcp_dns_events(lease_id);

-- +goose Down
DROP TABLE IF EXISTS dhcp_dns_events;
DROP INDEX IF EXISTS idx_dns_records_owner_ref;
ALTER TABLE dhcp_leases DROP COLUMN generation;
ALTER TABLE dns_records DROP COLUMN owner_generation;
ALTER TABLE dns_records DROP COLUMN owner_ref;
