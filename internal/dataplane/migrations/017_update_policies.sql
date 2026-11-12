-- +goose Up
-- The RFC 2136 update handler runs on the DNS data plane and reads its
-- update policies from dnsDataDB. Without this projection the query fails
-- and every signed UPDATE is refused, even though the control database has
-- the grants.
--
-- No foreign key on purpose: dns_zones is replaced wholesale on each sync
-- and a cascade would clear the policies between the delete and the reload.
CREATE TABLE IF NOT EXISTS dns_dynamic_update_policies (
    id            TEXT PRIMARY KEY,
    zone_id       TEXT NOT NULL,
    name          TEXT NOT NULL,
    pattern       TEXT,
    action        TEXT NOT NULL,
    tsig_key_name TEXT,
    enabled       BOOLEAN DEFAULT TRUE,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dns_dynamic_update_policies_zone_id ON dns_dynamic_update_policies(zone_id);

-- +goose Down
DROP INDEX IF EXISTS idx_dns_dynamic_update_policies_zone_id;
DROP TABLE IF EXISTS dns_dynamic_update_policies;
