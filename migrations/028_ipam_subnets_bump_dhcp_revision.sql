-- +goose Up
-- DHCP data planes carry the IPAM subnet-to-space projection used by durable
-- lease facts. Any mapping change must invalidate their DHCP configuration copy.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_ipam_subnets_ins
AFTER INSERT ON ipam_subnets
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_ipam_subnets_upd
AFTER UPDATE OF space_id, cidr ON ipam_subnets
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_ipam_subnets_del
AFTER DELETE ON ipam_subnets
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_rev_ipam_subnets_del;
DROP TRIGGER IF EXISTS trg_dp_rev_ipam_subnets_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_ipam_subnets_ins;
