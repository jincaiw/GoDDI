-- +goose Up
-- GoDDI DHCP Tables: scopes, leases, reservations, options, and logs.

CREATE TABLE IF NOT EXISTS dhcp_scopes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    interface TEXT,
    subnet TEXT NOT NULL,
    start_ip TEXT NOT NULL,
    end_ip TEXT NOT NULL,
    subnet_mask TEXT,
    router TEXT,
    dns_servers TEXT,
    ntp_servers TEXT,
    domain_name TEXT,
    lease_time INTEGER DEFAULT 86400,
    max_lease_time INTEGER,
    enabled BOOLEAN DEFAULT TRUE,
    ping_check_enabled BOOLEAN DEFAULT TRUE,
    dns_updates BOOLEAN DEFAULT FALSE,
    comment TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dhcp_leases (
    id TEXT PRIMARY KEY,
    scope_id TEXT NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
    ip_address TEXT NOT NULL,
    mac_address TEXT NOT NULL,
    hostname TEXT,
    client_id TEXT,
    lease_start DATETIME NOT NULL,
    lease_end DATETIME NOT NULL,
    status TEXT NOT NULL,
    last_seen DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS dhcp_reservations (
    id TEXT PRIMARY KEY,
    scope_id TEXT NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
    ip_address TEXT NOT NULL,
    mac_address TEXT NOT NULL UNIQUE,
    hostname TEXT,
    description TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dhcp_options (
    id TEXT PRIMARY KEY,
    scope_id TEXT NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
    reservation_id TEXT REFERENCES dhcp_reservations(id) ON DELETE CASCADE,
    code INTEGER NOT NULL,
    value TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT 'scope',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dhcp_logs (
    id TEXT PRIMARY KEY,
    scope_id TEXT,
    mac_address TEXT,
    ip_address TEXT,
    event_type TEXT NOT NULL,
    message TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Indexes for DHCP tables.
CREATE INDEX idx_dhcp_scopes_enabled ON dhcp_scopes(enabled);
CREATE INDEX idx_dhcp_scopes_subnet ON dhcp_scopes(subnet);
CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_leases_scope_ip_active
ON dhcp_leases(scope_id, ip_address) WHERE status = 'active';
CREATE INDEX idx_dhcp_leases_scope_id ON dhcp_leases(scope_id);
CREATE INDEX idx_dhcp_leases_ip_address ON dhcp_leases(ip_address);
CREATE INDEX idx_dhcp_leases_mac_address ON dhcp_leases(mac_address);
CREATE INDEX idx_dhcp_leases_status ON dhcp_leases(status);
CREATE INDEX idx_dhcp_leases_lease_end ON dhcp_leases(lease_end);
CREATE INDEX idx_dhcp_reservations_scope_id ON dhcp_reservations(scope_id);
CREATE INDEX idx_dhcp_reservations_mac_address ON dhcp_reservations(mac_address);
CREATE INDEX idx_dhcp_options_scope_id ON dhcp_options(scope_id);
CREATE INDEX idx_dhcp_options_reservation_id ON dhcp_options(reservation_id);
CREATE INDEX idx_dhcp_logs_scope_id ON dhcp_logs(scope_id);
CREATE INDEX idx_dhcp_logs_mac_address ON dhcp_logs(mac_address);
CREATE INDEX idx_dhcp_logs_created_at ON dhcp_logs(created_at);

-- +goose Down
DROP TABLE IF EXISTS dhcp_logs;
DROP TABLE IF EXISTS dhcp_options;
DROP TABLE IF EXISTS dhcp_reservations;
DROP TABLE IF EXISTS dhcp_leases;
DROP TABLE IF EXISTS dhcp_scopes;
