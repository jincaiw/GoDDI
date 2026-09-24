-- +goose Up
-- The zone data plane owns the SOA serial and the journal used to serve IXFR.
-- Keep the history beside the records so split deployments can journal a
-- record mutation in the same SQLite transaction as the zone update.
CREATE TABLE IF NOT EXISTS dns_zone_changes (
    id          TEXT PRIMARY KEY,
    zone_id     TEXT NOT NULL,
    serial      INTEGER NOT NULL,
    change_type TEXT NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    value       TEXT NOT NULL,
    ttl         INTEGER DEFAULT 300,
    priority    INTEGER,
    weight      INTEGER,
    port        INTEGER,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dns_zone_changes_zone_serial
    ON dns_zone_changes(zone_id, serial);

-- +goose Down
DROP INDEX IF EXISTS idx_dns_zone_changes_zone_serial;
DROP TABLE IF EXISTS dns_zone_changes;
