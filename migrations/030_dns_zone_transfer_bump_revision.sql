-- +goose Up
-- A DNS data plane caches the zone-transfer ACL in its own store. Adding,
-- changing or removing an allow-CIDR must invalidate that copy, otherwise
-- Sync would see an unchanged revision and keep answering AXFR with the old
-- policy indefinitely.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_zone_transfer_ins
AFTER INSERT ON dns_zone_transfer
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_zone_transfer_upd
AFTER UPDATE ON dns_zone_transfer
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_zone_transfer_del
AFTER DELETE ON dns_zone_transfer
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_rev_dns_zone_transfer_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_zone_transfer_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_zone_transfer_ins;
