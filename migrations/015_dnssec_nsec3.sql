-- Migration 015: NSEC3 parameters per zone (Technitium parity).
-- NSEC3 denial-of-existence parameters (RFC 5155). These are persisted per
-- zone and reported through the DNSSEC status; they take effect once real
-- zone signing lands.

-- +goose Up
ALTER TABLE dns_zones ADD COLUMN nsec3_iterations INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dns_zones ADD COLUMN nsec3_salt TEXT NOT NULL DEFAULT '';
ALTER TABLE dns_zones ADD COLUMN nsec3_optout BOOLEAN NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE dns_zones DROP COLUMN nsec3_optout;
ALTER TABLE dns_zones DROP COLUMN nsec3_salt;
ALTER TABLE dns_zones DROP COLUMN nsec3_iterations;
