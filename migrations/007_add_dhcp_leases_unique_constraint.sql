-- +goose Up
-- Add UNIQUE constraint on (scope_id, ip_address) for active DHCP leases.
-- This prevents duplicate active leases for the same IP within a scope and is
-- relied upon by lease.FindAvailableIP / CreateLease to enforce atomicity when
-- multiple DISCOVER requests race for the same IP.

CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_leases_scope_ip_active
ON dhcp_leases(scope_id, ip_address) WHERE status = 'active';

-- +goose Down
DROP INDEX IF EXISTS idx_dhcp_leases_scope_ip_active;
