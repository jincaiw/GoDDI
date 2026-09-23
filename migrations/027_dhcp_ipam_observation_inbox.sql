-- +goose Up
-- Control-side inbox. The data-plane copy is the immutable producer outbox;
-- this copy owns projection retry and completion state.
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

CREATE INDEX IF NOT EXISTS idx_ctrl_ipam_fact_pending
    ON dhcp_ipam_observation_events(status, next_attempt_at, sequence);
CREATE INDEX IF NOT EXISTS idx_ctrl_ipam_fact_event_id
    ON dhcp_ipam_observation_events(event_id);

CREATE TABLE IF NOT EXISTS facts_projection_watermark (
    domain      TEXT PRIMARY KEY,
    applied_seq INTEGER NOT NULL DEFAULT 0 CHECK (applied_seq >= 0)
);

-- +goose Down
DROP TABLE IF EXISTS facts_projection_watermark;
DROP INDEX IF EXISTS idx_ctrl_ipam_fact_event_id;
DROP INDEX IF EXISTS idx_ctrl_ipam_fact_pending;
DROP TABLE IF EXISTS dhcp_ipam_observation_events;
