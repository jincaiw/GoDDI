-- +goose Up
-- Add UNIQUE constraint on (subnet_id, ip_address) for IPAM addresses.
-- This prevents two IPAM rows from claiming the same IP in the same subnet
-- and is the last line of defense against the TOCTOU race in AllocateIP.

CREATE UNIQUE INDEX IF NOT EXISTS idx_ipam_addresses_subnet_ip_unique
ON ipam_addresses(subnet_id, ip_address);

-- +goose Down
DROP INDEX IF EXISTS idx_ipam_addresses_subnet_ip_unique;
