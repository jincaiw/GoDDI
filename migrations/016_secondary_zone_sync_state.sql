-- +goose Up
-- Track secondary-zone transfer state so refresh / retry / expire can be
-- enforced per RFC 1035 §3.3.13 and §6.3.
--
-- `updated_at` must NOT be used as the "last successful sync" clock: it is
-- bumped by any edit to the zone row (rename, policy change, toggle enabled).
-- Using it would reset the expiry clock on an unrelated edit and let a
-- secondary keep answering authoritatively from data the primary has already
-- superseded — the exact failure EXPIRE exists to prevent.

ALTER TABLE dns_zones ADD COLUMN last_sync_at DATETIME;
ALTER TABLE dns_zones ADD COLUMN last_sync_attempt_at DATETIME;
ALTER TABLE dns_zones ADD COLUMN sync_failure_count INTEGER NOT NULL DEFAULT 0;

-- Expired zones are looked up on every sweep, so index the clock.
CREATE INDEX IF NOT EXISTS idx_dns_zones_type_last_sync ON dns_zones(type, last_sync_at);

-- +goose Down
DROP INDEX IF EXISTS idx_dns_zones_type_last_sync;
ALTER TABLE dns_zones DROP COLUMN sync_failure_count;
ALTER TABLE dns_zones DROP COLUMN last_sync_attempt_at;
ALTER TABLE dns_zones DROP COLUMN last_sync_at;
