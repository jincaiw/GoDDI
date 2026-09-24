-- +goose Up
-- Durable, invisible staging for the HA facts snapshot. Rows become active
-- only when a verified snapshot is atomically applied by the standby.
CREATE TABLE IF NOT EXISTS facts_replica_snapshot_chunks (
    snapshot_id   TEXT    NOT NULL,
    chunk_index   INTEGER NOT NULL CHECK (chunk_index >= 0),
    first_sequence INTEGER NOT NULL CHECK (first_sequence > 0),
    last_sequence INTEGER NOT NULL CHECK (last_sequence >= first_sequence),
    event_count   INTEGER NOT NULL CHECK (event_count > 0),
    chunk_json    TEXT    NOT NULL CHECK (json_valid(chunk_json)),
    PRIMARY KEY (snapshot_id, chunk_index)
);

-- +goose Down
DROP TABLE IF EXISTS facts_replica_snapshot_chunks;
