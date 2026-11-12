-- +goose Up
-- A DNS data plane caches its RFC 2136 update policies. Adding, changing or
-- removing a grant must invalidate that copy, otherwise a revoked key could
-- keep writing records until some unrelated change moved the revision.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_update_policies_ins
AFTER INSERT ON dns_dynamic_update_policies
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_update_policies_upd
AFTER UPDATE ON dns_dynamic_update_policies
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_update_policies_del
AFTER DELETE ON dns_dynamic_update_policies
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_rev_dns_update_policies_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_update_policies_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_update_policies_ins;
