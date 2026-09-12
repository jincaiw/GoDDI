-- +goose Up
-- The DISCOVER/REQUEST handshake needs a third durable lease state: an address
-- that has been offered to a client but not yet confirmed by a REQUEST.
--
-- Previously only 'active' was covered, so a DISCOVER had to create a fully
-- active lease with the full lease time. That let a client which never sent
-- REQUEST (or a scanner walking the pool) hold addresses for the whole lease
-- duration and exhaust the scope, and it made the server report a binding that
-- the client had never accepted.
--
-- 'conflict' is deliberately left out of the index: quarantined addresses are
-- excluded from allocation by the availability query in lease.FindAvailableIP,
-- whereas including them here would require reconciling legacy rows where a
-- conflict entry co-exists with a later active lease for the same address
-- (which the old code produced by re-allocating a declined address).

DROP INDEX IF EXISTS idx_dhcp_leases_scope_ip_active;

CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_leases_scope_ip_held
ON dhcp_leases(scope_id, ip_address) WHERE status IN ('active', 'offered');

-- +goose Down
DROP INDEX IF EXISTS idx_dhcp_leases_scope_ip_held;

CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_leases_scope_ip_active
ON dhcp_leases(scope_id, ip_address) WHERE status = 'active';
