-- +goose Up
-- config_repl_state records, per source node, the snapshot version a follower
-- has applied. It is the follower's durable watermark for ADR-0013
-- configuration replication: without it a restart would re-apply the whole
-- snapshot (harmless but wasteful) and an operator could not tell whether the
-- follower is current.
CREATE TABLE IF NOT EXISTS config_repl_state (
    source_node     TEXT PRIMARY KEY,
    applied_version INTEGER NOT NULL DEFAULT 0,
    applied_rows    INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    synced_at       DATETIME
);

-- +goose Down
DROP TABLE IF EXISTS config_repl_state;
