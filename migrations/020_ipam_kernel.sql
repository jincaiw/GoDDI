-- +goose Up
-- IPAM kernel: address identity, observation vs allocation, DNS links.
--
-- Four changes, each fixing a concrete way the old schema lost information:
--
-- 1. Address identity. The unique key was (subnet_id, ip_address) and
--    ip_address stored whatever string the caller supplied. "192.168.001.001"
--    and "192.168.1.1" were two different addresses, so the unique index did
--    not stop the same host being allocated twice under two spellings.
--    space_id is now part of the identity and every write path normalises the
--    IP through net.ParseIP().String() before it reaches this table.
--
-- 2. Allocation state vs observation. `status` used to be written by both the
--    administrator and the DHCP sync, so a DHCP renewal silently overwrote an
--    administrative "reserved". The observed_* columns carry what the data
--    plane saw and never overwrite the allocation intent.
--
-- 3. One IP, many names. dns_record_id held a single record id, so a host with
--    both an A and a PTR -- or two A records in different zones -- could only
--    track one of them, and the second write erased the first.
--
-- 4. History provenance. ipam_history recorded who changed a status but not
--    why or from which subsystem, which made a DHCP-driven change
--    indistinguishable from an operator's in the audit trail.

ALTER TABLE ipam_addresses ADD COLUMN space_id TEXT NOT NULL DEFAULT '';

-- Denormalised from ipam_subnets. The foreign key guarantees the parent row
-- exists, so this backfill always resolves; an empty value here would mean a
-- row whose subnet was deleted out from under it.
UPDATE ipam_addresses
SET space_id = (SELECT s.space_id FROM ipam_subnets s WHERE s.id = ipam_addresses.subnet_id)
WHERE space_id = '';

ALTER TABLE ipam_addresses ADD COLUMN observed_state TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE ipam_addresses ADD COLUMN observed_at DATETIME;
ALTER TABLE ipam_addresses ADD COLUMN observed_source TEXT NOT NULL DEFAULT '';
ALTER TABLE ipam_addresses ADD COLUMN observed_mac TEXT;
ALTER TABLE ipam_addresses ADD COLUMN observed_hostname TEXT;
ALTER TABLE ipam_addresses ADD COLUMN allocated_at DATETIME;
ALTER TABLE ipam_addresses ADD COLUMN allocated_by TEXT NOT NULL DEFAULT '';

-- The identity index. If this fails on an upgraded database, the schema now
-- disagrees with the data: two subnets in one space claim the same address.
-- Either checkOverlap was bypassed when one of them was created, or a subnet
-- was resized afterwards so that it started covering the other's addresses.
-- Find the offenders before re-running:
--
--   SELECT space_id, ip_address, COUNT(*) AS n, GROUP_CONCAT(subnet_id)
--   FROM ipam_addresses GROUP BY space_id, ip_address HAVING n > 1;
--
-- space_id is in the output on purpose: without it the operator knows two rows
-- collide but not which space to look in, and the repair is per space.
--
-- Failing here is deliberate. A silently accepted duplicate address is an
-- outage waiting for the second host to come online.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ipam_addresses_space_ip_unique
ON ipam_addresses(space_id, ip_address);

CREATE INDEX IF NOT EXISTS idx_ipam_addresses_observed_state
ON ipam_addresses(observed_state);

CREATE INDEX IF NOT EXISTS idx_ipam_addresses_space_id
ON ipam_addresses(space_id);

-- One IP, many names. A record is linked by its own id, so two records for the
-- same address coexist instead of overwriting each other.
CREATE TABLE IF NOT EXISTS ipam_dns_links (
    id          TEXT PRIMARY KEY,
    address_id  TEXT NOT NULL REFERENCES ipam_addresses(id) ON DELETE CASCADE,
    record_id   TEXT NOT NULL,
    zone_id     TEXT,
    record_name TEXT NOT NULL,
    record_type TEXT NOT NULL,
    value       TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'manual',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ipam_dns_links_address_record
ON ipam_dns_links(address_id, record_id);
CREATE INDEX IF NOT EXISTS idx_ipam_dns_links_record ON ipam_dns_links(record_id);
CREATE INDEX IF NOT EXISTS idx_ipam_dns_links_address ON ipam_dns_links(address_id);

-- Carry over the handful of rows the old single-column link managed to record,
-- so an upgrade does not silently drop an existing association. Nothing in the
-- codebase ever populated dns_record_id, so in practice this is a no-op; it
-- exists so that "we migrated existing links" is true rather than assumed.
INSERT OR IGNORE INTO ipam_dns_links
    (id, address_id, record_id, zone_id, record_name, record_type, value, source)
SELECT
    lower(hex(randomblob(16))),
    a.id,
    a.dns_record_id,
    r.zone_id,
    COALESCE(r.name, ''),
    COALESCE(r.type, ''),
    COALESCE(r.value, a.ip_address),
    'dns_record_id-backfill'
FROM ipam_addresses a
LEFT JOIN dns_records r ON r.id = a.dns_record_id
WHERE a.dns_record_id IS NOT NULL AND a.dns_record_id <> '';

ALTER TABLE ipam_history ADD COLUMN reason TEXT NOT NULL DEFAULT '';
ALTER TABLE ipam_history ADD COLUMN source TEXT NOT NULL DEFAULT 'admin';

-- History is looked up by (space, IP) rather than by subnet, because several
-- subnets in one space may cover the same address across a resize and the
-- history must survive that. Denormalising the space keeps the lookup from
-- needing a join per row.
--
-- The COALESCE is not decoration. A scalar subquery that matches no row yields
-- NULL, and this column is NOT NULL, so a history row whose subnet is gone
-- would abort the migration and block the upgrade of the whole database. Such
-- a row is reachable: the ON DELETE CASCADE on ipam_history.subnet_id only
-- runs when the foreign_keys pragma is on, which is per connection and off by
-- default -- any delete issued by the sqlite3 CLI, or on a connection that
-- never had the pragma applied, leaves the row behind.
--
-- Those rows keep the empty space_id they already have, which means the
-- address's history is not reachable by space. That is the honest outcome:
-- there is no space to attribute them to, and inventing one would put another
-- space's changes in this space's history. The alternative -- refusing to
-- migrate -- turns a few invisible history rows into a server that will not
-- start.
ALTER TABLE ipam_history ADD COLUMN space_id TEXT NOT NULL DEFAULT '';
UPDATE ipam_history
SET space_id = COALESCE(
        (SELECT s.space_id FROM ipam_subnets s WHERE s.id = ipam_history.subnet_id),
        '')
WHERE space_id = '';

CREATE INDEX IF NOT EXISTS idx_ipam_history_ip ON ipam_history(space_id, ip_address, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_ipam_history_ip;
ALTER TABLE ipam_history DROP COLUMN space_id;
ALTER TABLE ipam_history DROP COLUMN source;
ALTER TABLE ipam_history DROP COLUMN reason;

DROP TABLE IF EXISTS ipam_dns_links;

DROP INDEX IF EXISTS idx_ipam_addresses_space_id;
DROP INDEX IF EXISTS idx_ipam_addresses_observed_state;
DROP INDEX IF EXISTS idx_ipam_addresses_space_ip_unique;

ALTER TABLE ipam_addresses DROP COLUMN allocated_by;
ALTER TABLE ipam_addresses DROP COLUMN allocated_at;
ALTER TABLE ipam_addresses DROP COLUMN observed_hostname;
ALTER TABLE ipam_addresses DROP COLUMN observed_mac;
ALTER TABLE ipam_addresses DROP COLUMN observed_source;
ALTER TABLE ipam_addresses DROP COLUMN observed_at;
ALTER TABLE ipam_addresses DROP COLUMN observed_state;
ALTER TABLE ipam_addresses DROP COLUMN space_id;
