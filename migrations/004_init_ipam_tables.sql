-- +goose Up
-- GoDDI IPAM Tables: spaces, subnets, addresses, and history.

CREATE TABLE IF NOT EXISTS ipam_spaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS ipam_subnets (
    id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL REFERENCES ipam_spaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    cidr TEXT NOT NULL,
    vlan_id INTEGER,
    location TEXT,
    description TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS ipam_addresses (
    id TEXT PRIMARY KEY,
    subnet_id TEXT NOT NULL REFERENCES ipam_subnets(id) ON DELETE CASCADE,
    ip_address TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'available',
    mac_address TEXT,
    hostname TEXT,
    dns_record_id TEXT,
    dhcp_lease_id TEXT,
    owner TEXT,
    device TEXT,
    location TEXT,
    description TEXT,
    last_seen DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS ipam_history (
    id TEXT PRIMARY KEY,
    subnet_id TEXT NOT NULL REFERENCES ipam_subnets(id) ON DELETE CASCADE,
    ip_address TEXT NOT NULL,
    action TEXT NOT NULL,
    old_status TEXT,
    new_status TEXT,
    changed_by TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Indexes for IPAM tables.
CREATE INDEX idx_ipam_subnets_space_id ON ipam_subnets(space_id);
CREATE INDEX idx_ipam_subnets_cidr ON ipam_subnets(cidr);
CREATE INDEX idx_ipam_addresses_subnet_id ON ipam_addresses(subnet_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ipam_addresses_subnet_ip_unique
ON ipam_addresses(subnet_id, ip_address);
CREATE INDEX idx_ipam_addresses_ip_address ON ipam_addresses(ip_address);
CREATE INDEX idx_ipam_addresses_status ON ipam_addresses(status);
CREATE INDEX idx_ipam_addresses_mac_address ON ipam_addresses(mac_address);
CREATE INDEX idx_ipam_addresses_hostname ON ipam_addresses(hostname);
CREATE INDEX idx_ipam_addresses_subnet_status ON ipam_addresses(subnet_id, status);
CREATE INDEX idx_ipam_history_subnet_id ON ipam_history(subnet_id);
CREATE INDEX idx_ipam_history_created_at ON ipam_history(created_at);

-- +goose Down
DROP TABLE IF EXISTS ipam_history;
DROP TABLE IF EXISTS ipam_addresses;
DROP TABLE IF EXISTS ipam_subnets;
DROP TABLE IF EXISTS ipam_spaces;
