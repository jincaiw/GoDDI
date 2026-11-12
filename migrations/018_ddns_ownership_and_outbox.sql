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
ALTER TABLE dns_records ADD COLUMN owner_ref TEXT;
ALTER TABLE dns_records ADD COLUMN owner_generation INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_dns_records_owner_ref ON dns_records(owner_ref);

-- Existing leases start a fresh generation series. Every write path that
-- (re)establishes a binding increments this value, so generation 0 is never a
-- valid generation for a live binding.
ALTER TABLE dhcp_leases ADD COLUMN generation INTEGER NOT NULL DEFAULT 1;

-- +goose Down
DROP INDEX IF EXISTS idx_dns_records_owner_ref;
ALTER TABLE dhcp_leases DROP COLUMN generation;
ALTER TABLE dns_records DROP COLUMN owner_generation;
ALTER TABLE dns_records DROP COLUMN owner_ref;
