-- +goose Up
-- GoDDI DNS Tables: zones, records, forwarders, block lists, and related tables.

CREATE TABLE IF NOT EXISTS dns_zones (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    dnssec_enabled BOOLEAN DEFAULT FALSE,
    default_ttl INTEGER DEFAULT 3600,
    soa_mname TEXT NOT NULL,
    soa_rname TEXT NOT NULL,
    serial INTEGER NOT NULL,
    refresh INTEGER DEFAULT 3600,
    retry INTEGER DEFAULT 600,
    expire INTEGER DEFAULT 86400,
    minimum INTEGER DEFAULT 300,
    transfer_policy TEXT,
    update_policy TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_records (
    id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    value TEXT NOT NULL,
    ttl INTEGER DEFAULT 300,
    priority INTEGER,
    weight INTEGER,
    port INTEGER,
    enabled BOOLEAN DEFAULT TRUE,
    comment TEXT,
    tags TEXT,
    tag TEXT,
    flag INTEGER DEFAULT 0,
    owner TEXT,
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_zone_transfer (
    id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    allowed_cidr TEXT NOT NULL,
    tsig_key_name TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_dynamic_update_policies (
    id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    pattern TEXT,
    action TEXT NOT NULL,
    tsig_key_name TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_dnssec_keys (
    id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    key_type TEXT NOT NULL,
    algorithm TEXT NOT NULL,
    public_key TEXT NOT NULL,
    private_key_ciphertext TEXT NOT NULL,
    key_tag INTEGER,
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_cache (
    id TEXT PRIMARY KEY,
    query_name TEXT NOT NULL,
    query_type TEXT NOT NULL,
    response_data TEXT NOT NULL,
    ttl INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_forwarders (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'udp',
    address TEXT NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    priority INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_conditional_forwarders (
    id TEXT PRIMARY KEY,
    domain TEXT NOT NULL UNIQUE,
    forwarder_ids TEXT NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_client_history (
    id TEXT PRIMARY KEY,
    client_ip TEXT NOT NULL,
    query_name TEXT,
    query_type TEXT,
    last_seen DATETIME NOT NULL,
    query_count INTEGER DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_block_lists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    url TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    last_updated DATETIME,
    entry_count INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_block_rules (
    id TEXT PRIMARY KEY,
    list_id TEXT NOT NULL REFERENCES dns_block_lists(id) ON DELETE CASCADE,
    pattern TEXT NOT NULL,
    match_type TEXT NOT NULL,
    response_type TEXT NOT NULL DEFAULT 'NXDOMAIN',
    response_data TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_allow_rules (
    id TEXT PRIMARY KEY,
    pattern TEXT NOT NULL,
    match_type TEXT NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_client_policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    source_cidr TEXT,
    action TEXT NOT NULL,
    block_list_ids TEXT,
    allow_rule_ids TEXT,
    priority INTEGER DEFAULT 0,
    enabled BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dns_query_logs (
    id TEXT PRIMARY KEY,
    client_ip TEXT NOT NULL,
    client_port INTEGER,
    protocol TEXT,
    query_name TEXT NOT NULL,
    query_type TEXT NOT NULL,
    response_code TEXT,
    response_time_ms REAL,
    upstream TEXT,
    cached BOOLEAN DEFAULT FALSE,
    blocked BOOLEAN DEFAULT FALSE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Indexes for DNS tables.
CREATE INDEX idx_dns_zones_name ON dns_zones(name);
CREATE INDEX idx_dns_zones_enabled ON dns_zones(enabled);
CREATE INDEX idx_dns_records_zone_id ON dns_records(zone_id);
CREATE INDEX idx_dns_records_name ON dns_records(name);
CREATE INDEX idx_dns_records_type ON dns_records(type);
CREATE INDEX idx_dns_records_name_type ON dns_records(name, type);
CREATE INDEX idx_dns_zone_transfer_zone_id ON dns_zone_transfer(zone_id);
CREATE INDEX idx_dns_dynamic_update_policies_zone_id ON dns_dynamic_update_policies(zone_id);
CREATE INDEX idx_dns_dnssec_keys_zone_id ON dns_dnssec_keys(zone_id);
CREATE INDEX idx_dns_cache_query ON dns_cache(query_name, query_type);
CREATE INDEX idx_dns_cache_expires_at ON dns_cache(expires_at);
CREATE INDEX idx_dns_forwarders_enabled ON dns_forwarders(enabled);
CREATE INDEX idx_dns_conditional_forwarders_domain ON dns_conditional_forwarders(domain);
CREATE INDEX idx_dns_client_history_client_ip ON dns_client_history(client_ip);
CREATE INDEX idx_dns_client_history_last_seen ON dns_client_history(last_seen);
CREATE INDEX idx_dns_block_rules_list_id ON dns_block_rules(list_id);
CREATE INDEX idx_dns_client_policies_enabled ON dns_client_policies(enabled);
CREATE INDEX idx_dns_client_policies_priority ON dns_client_policies(priority);
CREATE INDEX idx_dns_query_logs_client_ip ON dns_query_logs(client_ip);
CREATE INDEX idx_dns_query_logs_query_name ON dns_query_logs(query_name);
CREATE INDEX idx_dns_query_logs_created_at ON dns_query_logs(created_at);
CREATE INDEX idx_dns_query_logs_blocked ON dns_query_logs(blocked);

-- +goose Down
DROP TABLE IF EXISTS dns_query_logs;
DROP TABLE IF EXISTS dns_client_policies;
DROP TABLE IF EXISTS dns_allow_rules;
DROP TABLE IF EXISTS dns_block_rules;
DROP TABLE IF EXISTS dns_block_lists;
DROP TABLE IF EXISTS dns_client_history;
DROP TABLE IF EXISTS dns_conditional_forwarders;
DROP TABLE IF EXISTS dns_forwarders;
DROP TABLE IF EXISTS dns_cache;
DROP TABLE IF EXISTS dns_dnssec_keys;
DROP TABLE IF EXISTS dns_dynamic_update_policies;
DROP TABLE IF EXISTS dns_zone_transfer;
DROP TABLE IF EXISTS dns_records;
DROP TABLE IF EXISTS dns_zones;
