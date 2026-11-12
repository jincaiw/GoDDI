-- +goose Up
-- A DNS data plane caches DNSSEC keys in its own store. Generating, enabling,
-- disabling or deleting a key must invalidate that copy, otherwise the
-- responder would keep signing with a retired key (or fail to sign at all)
-- until some unrelated change moved the revision.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_dnssec_keys_ins
AFTER INSERT ON dns_dnssec_keys
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_dnssec_keys_upd
AFTER UPDATE ON dns_dnssec_keys
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_dnssec_keys_del
AFTER DELETE ON dns_dnssec_keys
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_rev_dns_dnssec_keys_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_dnssec_keys_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_dnssec_keys_ins;
