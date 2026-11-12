-- +goose Up
-- Zone-transfer ACLs are authorisation data, not zone data, but a DNS data
-- plane answers AXFR/IXFR without ever opening the control database. The
-- handler reads this table from the store it serves from, so the ACL rows have
-- to be replicated here alongside dns_zones and dns_records.
--
-- No foreign key is declared on purpose: the table is a read-only projection.
-- dns_zones is replaced wholesale on every sync, and a cascade would delete
-- the ACL rows out from under the handler between the clear and the reload.
CREATE TABLE IF NOT EXISTS dns_zone_transfer (
    id              TEXT PRIMARY KEY,
    zone_id         TEXT NOT NULL,
    allowed_cidr    TEXT NOT NULL,
    tsig_key_name   TEXT,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dns_zone_transfer_zone_id ON dns_zone_transfer(zone_id);

-- +goose Down
DROP INDEX IF EXISTS idx_dns_zone_transfer_zone_id;
DROP TABLE IF EXISTS dns_zone_transfer;
