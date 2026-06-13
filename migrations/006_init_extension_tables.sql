-- +goose Up
-- GoDDI Extension Tables: apps, cluster, SSO.

CREATE TABLE IF NOT EXISTS apps (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    version TEXT,
    description TEXT,
    enabled BOOLEAN DEFAULT FALSE,
    installed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS app_settings (
    id TEXT PRIMARY KEY,
    app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS app_logs (
    id TEXT PRIMARY KEY,
    app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS cluster_nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'offline',
    last_heartbeat DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS cluster_sync_tasks (
    id TEXT PRIMARY KEY,
    source_node_id TEXT NOT NULL REFERENCES cluster_nodes(id) ON DELETE CASCADE,
    target_node_id TEXT NOT NULL REFERENCES cluster_nodes(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    started_at DATETIME,
    completed_at DATETIME,
    error TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sso_settings (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL UNIQUE,
    authority TEXT,
    client_id TEXT,
    client_secret_ciphertext TEXT,
    scopes TEXT,
    discovery_url TEXT,
    auto_register BOOLEAN DEFAULT FALSE,
    group_mapping TEXT,
    enabled BOOLEAN DEFAULT FALSE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oidc_user_links (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    email TEXT,
    groups_snapshot TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(issuer, subject)
);

-- Indexes for extension tables.
CREATE INDEX idx_app_settings_app_id ON app_settings(app_id);
CREATE INDEX idx_app_logs_app_id ON app_logs(app_id);
CREATE INDEX idx_app_logs_level ON app_logs(level);
CREATE INDEX idx_app_logs_created_at ON app_logs(created_at);
CREATE INDEX idx_cluster_nodes_name ON cluster_nodes(name);
CREATE INDEX idx_cluster_nodes_status ON cluster_nodes(status);
CREATE INDEX idx_cluster_sync_tasks_source_node_id ON cluster_sync_tasks(source_node_id);
CREATE INDEX idx_cluster_sync_tasks_target_node_id ON cluster_sync_tasks(target_node_id);
CREATE INDEX idx_cluster_sync_tasks_status ON cluster_sync_tasks(status);
CREATE INDEX idx_sso_settings_provider ON sso_settings(provider);
CREATE INDEX idx_oidc_user_links_user_id ON oidc_user_links(user_id);
CREATE INDEX idx_oidc_user_links_issuer_subject ON oidc_user_links(issuer, subject);

-- +goose Down
DROP TABLE IF EXISTS oidc_user_links;
DROP TABLE IF EXISTS sso_settings;
DROP TABLE IF EXISTS cluster_sync_tasks;
DROP TABLE IF EXISTS cluster_nodes;
DROP TABLE IF EXISTS app_logs;
DROP TABLE IF EXISTS app_settings;
DROP TABLE IF EXISTS apps;
