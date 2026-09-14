-- +goose Up
-- N6 migration-stage durable fact storage. This table is an additive bridge:
-- existing dhcp_dns_events remains the compatibility DNS outbox until consumers
-- are migrated and the unified sequence is produced by the authoritative path.
CREATE TABLE IF NOT EXISTS dhcp_ipam_observation_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id        TEXT    NOT NULL UNIQUE,
    version         INTEGER NOT NULL CHECK (version > 0),
    entity          TEXT    NOT NULL,
    action          TEXT    NOT NULL,
    generation      INTEGER NOT NULL CHECK (generation >= 0),
    sequence        INTEGER NOT NULL UNIQUE CHECK (sequence > 0),
    source          TEXT    NOT NULL,
    occurred_at     DATETIME NOT NULL,
    payload_version INTEGER NOT NULL CHECK (payload_version > 0),
    payload         TEXT    NOT NULL CHECK (json_valid(payload)),
    attempts        INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_error      TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'done', 'failed')),
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dp_ipam_fact_pending
    ON dhcp_ipam_observation_events(status, next_attempt_at, sequence);
CREATE INDEX IF NOT EXISTS idx_dp_ipam_fact_event_id
    ON dhcp_ipam_observation_events(event_id);

-- One watermark per consumer domain. A consumer must advance it in the same
-- transaction as its projection write; gaps are not silently skipped.
CREATE TABLE IF NOT EXISTS facts_projection_watermark (
    domain      TEXT PRIMARY KEY,
    applied_seq INTEGER NOT NULL DEFAULT 0 CHECK (applied_seq >= 0)
);

-- Canonical sequence allocator for the unified DHCP-DNS-IPAM fact stream.
-- Allocation must happen in the same transaction as the authoritative mutation
-- and the corresponding outbox insert.
CREATE TABLE IF NOT EXISTS facts_sequence_allocator (
    domain        TEXT PRIMARY KEY,
    last_sequence INTEGER NOT NULL DEFAULT 0 CHECK (last_sequence >= 0)
);

-- +goose Down
DROP TABLE IF EXISTS facts_sequence_allocator;
DROP TABLE IF EXISTS facts_projection_watermark;
DROP INDEX IF EXISTS idx_dp_ipam_fact_event_id;
DROP INDEX IF EXISTS idx_dp_ipam_fact_pending;
DROP TABLE IF EXISTS dhcp_ipam_observation_events;
