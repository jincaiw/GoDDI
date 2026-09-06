-- +goose Up
-- GoDDI v0.1.6: zone-level ACL (query/transfer/update sources + NOTIFY targets).

ALTER TABLE dns_zones ADD COLUMN acl TEXT;

-- +goose Down
ALTER TABLE dns_zones DROP COLUMN acl;
