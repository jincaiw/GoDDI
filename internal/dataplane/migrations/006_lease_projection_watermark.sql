-- +goose Up
-- Lease projection watermark is distinct from dataplane_revision and HA
-- replication watermarks. It advances only with contiguous WAL projection.
CREATE TABLE IF NOT EXISTS lease_projection_watermark (
    domain      TEXT PRIMARY KEY,
    applied_seq INTEGER NOT NULL DEFAULT 0 CHECK (applied_seq >= 0)
);

INSERT OR IGNORE INTO lease_projection_watermark (domain, applied_seq)
VALUES ('dhcp_lease_projection', 0);

-- +goose Down
DROP TABLE IF EXISTS lease_projection_watermark;
