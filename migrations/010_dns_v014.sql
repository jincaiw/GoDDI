-- +goose Up
-- GoDDI v0.1.4: block list fetch status, zone change history (IXFR).

-- Block list fetch status columns (URL subscription refresh).
ALTER TABLE dns_block_lists ADD COLUMN last_fetch_at DATETIME;
ALTER TABLE dns_block_lists ADD COLUMN last_fetch_status TEXT;
ALTER TABLE dns_block_lists ADD COLUMN last_fetch_error TEXT;

-- Zone change history for true IXFR (RFC 1995) incremental transfer.
CREATE TABLE IF NOT EXISTS dns_zone_changes (
    id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    serial INTEGER NOT NULL,
    change_type TEXT NOT NULL, -- 'add' or 'delete'
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    value TEXT NOT NULL,
    ttl INTEGER DEFAULT 300,
    priority INTEGER,
    weight INTEGER,
    port INTEGER,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_dns_zone_changes_zone_serial ON dns_zone_changes(zone_id, serial);

-- +goose Down
DROP TABLE IF EXISTS dns_zone_changes;
