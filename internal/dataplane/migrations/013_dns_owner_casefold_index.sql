-- +goose Up
-- Dynamic updates and DNS invariants compare owner names case-insensitively
-- and without a trailing root dot. Keep that lookup indexed on the data plane.
CREATE INDEX IF NOT EXISTS idx_dns_records_zone_owner_canonical
    ON dns_records(zone_id, LOWER(RTRIM(name, '.')));

-- +goose Down
DROP INDEX IF EXISTS idx_dns_records_zone_owner_canonical;
