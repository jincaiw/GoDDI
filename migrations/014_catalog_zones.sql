-- Migration 014: Catalog Zones (RFC 9432, Technitium parity).
-- Each member zone stores the name of its catalog zone. Membership is
-- materialised as RFC 9432 PTR records ("<hash>.members.<catalog>") inside
-- the catalog zone so AXFR and the in-memory store work unchanged.

-- +goose Up
ALTER TABLE dns_zones ADD COLUMN catalog TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE dns_zones DROP COLUMN catalog;
