-- Migration 013: Per-zone permissions (Technitium Zone Permissions parity).
-- A zone with at least one permission row restricts access to the listed
-- users/groups (intersected with global RBAC); zones without any rows keep
-- the global-RBAC-only behavior.

-- +goose Up
CREATE TABLE IF NOT EXISTS dns_zone_permissions (
    id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
    principal_type TEXT NOT NULL CHECK (principal_type IN ('user','group')),
    principal_id TEXT NOT NULL,
    can_view INTEGER NOT NULL DEFAULT 1,
    can_modify INTEGER NOT NULL DEFAULT 0,
    can_delete INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE (zone_id, principal_type, principal_id)
);

CREATE INDEX IF NOT EXISTS idx_dns_zone_permissions_zone ON dns_zone_permissions(zone_id);

-- +goose Down
DROP TABLE IF EXISTS dns_zone_permissions;
