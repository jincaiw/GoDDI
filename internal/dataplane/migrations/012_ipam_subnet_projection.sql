-- +goose Up
-- Minimal local copy used to resolve DHCP fact space IDs without a control DB
-- read in the packet path. No foreign key: replicas are replaced atomically.
CREATE TABLE IF NOT EXISTS ipam_subnets (
    id       TEXT PRIMARY KEY,
    space_id TEXT NOT NULL,
    cidr     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dp_ipam_subnets_space ON ipam_subnets(space_id);

-- +goose Down
DROP INDEX IF EXISTS idx_dp_ipam_subnets_space;
DROP TABLE IF EXISTS ipam_subnets;
