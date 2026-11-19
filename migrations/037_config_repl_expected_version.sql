-- +goose Up
-- expected_version is the fingerprint of the author snapshot the follower
-- last observed. Keeping it beside applied_version lets the status API show
-- whether the follower is behind even when the pull was a no-op.
ALTER TABLE config_repl_state ADD COLUMN expected_version INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot DROP COLUMN on every supported version; the table is
-- recreated by 036 when migrating down past this point.
