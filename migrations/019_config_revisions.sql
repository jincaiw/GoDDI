-- +goose Up
-- Configuration publishing previously had no history. Every write went
-- straight at the resource row, so there was no way to answer "what changed,
-- when, by whom", no way to diff two states, and no way back: a mistyped
-- SOA contact or a wrong pool range could only be fixed by hand.
--
-- Two tables fix that.
--
-- config_revisions is an append-only log of published configurations. Each row
-- carries the exact content that was published, so any earlier state can be
-- read back, diffed, and rolled forward again. Revisions are numbered per
-- (resource_type, resource_id) and are never rewritten: a rollback is a NEW
-- revision that happens to carry old content. That is what keeps the log
-- auditable -- if rollback rewrote history there would be no record that the
-- bad configuration was ever live.
--
-- config_release_outbox separates "the configuration is durably recorded" from
-- "the data plane is actually serving it". Reloading an in-memory zone store
-- cannot join a SQLite transaction, so the release event is written in the same
-- transaction as the revision. A crash after commit therefore leaves a durable
-- record that a reload is owed, and the worker replays it, instead of leaving a
-- configuration in the database that the resolver never learned about.
--
-- Ownership/generation semantics deliberately mirror dhcp_dns_events
-- (migration 018): ordered drain, bounded retry with backoff, and terminal
-- failures kept visible rather than deleted.

CREATE TABLE IF NOT EXISTS config_revisions (
    id                 TEXT PRIMARY KEY,
    resource_type      TEXT    NOT NULL,
    resource_id        TEXT    NOT NULL,
    revision           INTEGER NOT NULL,
    -- persisted | staged | applied | failed. There is no stored 'draft' or
    -- 'validated': a preview is validated and reported without being written,
    -- so that previewing a change never consumes a revision number.
    status             TEXT    NOT NULL DEFAULT 'staged',
    content            TEXT    NOT NULL,
    content_hash       TEXT    NOT NULL,
    -- Revision this one was based on. A concurrent writer that still holds the
    -- old base loses the race instead of silently overwriting.
    base_revision      INTEGER NOT NULL DEFAULT 0,
    idempotency_key    TEXT,
    actor              TEXT    NOT NULL DEFAULT '',
    note               TEXT    NOT NULL DEFAULT '',
    error              TEXT    NOT NULL DEFAULT '',
    -- Monotonic counter the data plane reported back for this revision. 0 until
    -- the release worker has confirmed the data plane picked it up.
    applied_generation INTEGER NOT NULL DEFAULT 0,
    created_at         DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at         DATETIME NOT NULL DEFAULT (datetime('now')),
    applied_at         DATETIME
);

-- A given resource can never have two rows with the same revision number.
-- This is the constraint that turns "read max, then insert max+1" into a real
-- optimistic-concurrency check rather than a hopeful one.
CREATE UNIQUE INDEX IF NOT EXISTS idx_config_revisions_version
    ON config_revisions(resource_type, resource_id, revision);

-- Retrying a publish with the same idempotency key must return the original
-- result rather than create a second revision. NULL is allowed (and unlimited)
-- for callers that do not supply a key.
CREATE UNIQUE INDEX IF NOT EXISTS idx_config_revisions_idempotency
    ON config_revisions(idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_config_revisions_resource
    ON config_revisions(resource_type, resource_id, revision DESC);

CREATE INDEX IF NOT EXISTS idx_config_revisions_status
    ON config_revisions(status);

CREATE TABLE IF NOT EXISTS config_release_outbox (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    revision_id        TEXT    NOT NULL,
    resource_type      TEXT    NOT NULL,
    resource_id        TEXT    NOT NULL,
    revision           INTEGER NOT NULL,
    status             TEXT    NOT NULL DEFAULT 'pending',
    attempts           INTEGER NOT NULL DEFAULT 0,
    -- Set in the same transaction as the resource write. It distinguishes
    -- "the configuration never reached the database" from "it is written but
    -- the data plane was never told", which are different failures needing
    -- different operator responses.
    resource_applied   INTEGER NOT NULL DEFAULT 0,
    applied_generation INTEGER NOT NULL DEFAULT 0,
    last_error         TEXT    NOT NULL DEFAULT '',
    next_attempt_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    created_at         DATETIME NOT NULL DEFAULT (datetime('now')),
    applied_at         DATETIME
);

-- The worker drains in id order so an older release is never applied after the
-- newer one that superseded it.
CREATE INDEX IF NOT EXISTS idx_config_release_outbox_pending
    ON config_release_outbox(status, next_attempt_at, id);

CREATE INDEX IF NOT EXISTS idx_config_release_outbox_resource
    ON config_release_outbox(resource_type, resource_id, revision DESC);

-- +goose Down
DROP TABLE IF EXISTS config_release_outbox;
DROP TABLE IF EXISTS config_revisions;
