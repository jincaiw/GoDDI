-- +goose Up
-- DNSSEC private keys are configuration, not zone data, but a DNS data plane
-- has to be able to mint RRSIGs without opening the control database. The
-- signing responder reads dns_zones + dns_dnssec_keys from the store it serves
-- from, so both tables have to be replicated here.
--
-- The private key is stored exactly as it is in the control database: already
-- encrypted at rest. No foreign key is declared on purpose, matching the
-- dns_zone_transfer projection: dns_zones is replaced wholesale on every sync
-- and a cascade would delete the key rows between the clear and the reload.
CREATE TABLE IF NOT EXISTS dns_dnssec_keys (
    id                     TEXT PRIMARY KEY,
    zone_id                TEXT NOT NULL,
    key_type               TEXT NOT NULL,
    algorithm              TEXT NOT NULL,
    public_key             TEXT NOT NULL,
    private_key_ciphertext TEXT NOT NULL,
    key_tag                INTEGER,
    enabled                BOOLEAN DEFAULT TRUE,
    created_at             DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dns_dnssec_keys_zone_id ON dns_dnssec_keys(zone_id);

-- +goose Down
DROP INDEX IF EXISTS idx_dns_dnssec_keys_zone_id;
DROP TABLE IF EXISTS dns_dnssec_keys;
