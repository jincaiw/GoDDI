-- +goose Up
-- The data-plane store.
--
-- A DNS or DHCP process serves from this file, not from the control database.
-- That is what keeps resolution and lease renewal working while the management
-- process is restarting, being upgraded, or is simply down.
--
-- One schema covers both roles: a store that belongs to the DNS role simply
-- never populates the DHCP tables and vice versa. Keeping a single migration
-- history means there is one thing to review, one thing to back up and one
-- integrity check, instead of two that can drift apart.
--
-- ---------------------------------------------------------------------------
-- Deliberately no foreign keys anywhere in this file.
-- ---------------------------------------------------------------------------
-- Replica rows are replaced wholesale from the control database. A foreign key
-- with ON DELETE CASCADE from a replaced parent would take the children with
-- it: replacing dhcp_scopes would delete every lease in the store, and
-- replacing dns_zones would delete every record. Referential integrity is
-- enforced where the rows are authored (the control database) plus the rule
-- that a replica is only ever applied as a whole, parents before children.
-- The one relationship that mattered -- "deleting a scope removes its leases" --
-- is re-implemented explicitly in the sync, where it can report what it did
-- instead of deleting rows silently.

-- ---------------------------------------------------------------------------
-- Replicas of control-plane configuration. Read-only from the data plane's
-- point of view; only the sync writes here.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS dhcp_scopes (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    interface          TEXT,
    subnet             TEXT NOT NULL,
    start_ip           TEXT NOT NULL,
    end_ip             TEXT NOT NULL,
    subnet_mask        TEXT,
    router             TEXT,
    dns_servers        TEXT,
    ntp_servers        TEXT,
    domain_name        TEXT,
    lease_time         INTEGER DEFAULT 86400,
    max_lease_time     INTEGER,
    enabled            BOOLEAN DEFAULT TRUE,
    ping_check_enabled BOOLEAN DEFAULT TRUE,
    dns_updates        BOOLEAN DEFAULT FALSE,
    comment            TEXT,
    created_at         DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at         DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dhcp_scopes_enabled ON dhcp_scopes(enabled);
CREATE INDEX IF NOT EXISTS idx_dhcp_scopes_subnet ON dhcp_scopes(subnet);

CREATE TABLE IF NOT EXISTS dhcp_reservations (
    id          TEXT PRIMARY KEY,
    scope_id    TEXT NOT NULL,
    ip_address  TEXT NOT NULL,
    mac_address TEXT NOT NULL UNIQUE,
    hostname    TEXT,
    description TEXT,
    enabled     BOOLEAN DEFAULT TRUE,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dhcp_reservations_scope_id ON dhcp_reservations(scope_id);
CREATE INDEX IF NOT EXISTS idx_dhcp_reservations_mac_address ON dhcp_reservations(mac_address);

CREATE TABLE IF NOT EXISTS dhcp_options (
    id             TEXT PRIMARY KEY,
    scope_id       TEXT NOT NULL,
    reservation_id TEXT,
    code           INTEGER NOT NULL,
    value          TEXT NOT NULL,
    priority       TEXT NOT NULL DEFAULT 'scope',
    created_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at     DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dhcp_options_scope_id ON dhcp_options(scope_id);
CREATE INDEX IF NOT EXISTS idx_dhcp_options_reservation_id ON dhcp_options(reservation_id);

CREATE TABLE IF NOT EXISTS dns_zones (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    type                TEXT NOT NULL,
    enabled             BOOLEAN DEFAULT TRUE,
    dnssec_enabled      BOOLEAN DEFAULT FALSE,
    default_ttl         INTEGER DEFAULT 3600,
    soa_mname           TEXT NOT NULL,
    soa_rname           TEXT NOT NULL,
    serial              INTEGER NOT NULL,
    refresh             INTEGER DEFAULT 3600,
    retry               INTEGER DEFAULT 600,
    expire              INTEGER DEFAULT 86400,
    minimum             INTEGER DEFAULT 300,
    transfer_policy     TEXT,
    update_policy       TEXT,
    acl                 TEXT,
    catalog             TEXT NOT NULL DEFAULT '',
    nsec3_iterations    INTEGER NOT NULL DEFAULT 0,
    nsec3_salt          TEXT NOT NULL DEFAULT '',
    nsec3_optout        BOOLEAN NOT NULL DEFAULT 0,
    last_sync_at        DATETIME,
    last_sync_attempt_at DATETIME,
    sync_failure_count  INTEGER NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at          DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dns_zones_enabled ON dns_zones(enabled);
CREATE INDEX IF NOT EXISTS idx_dns_zones_name ON dns_zones(name);

CREATE TABLE IF NOT EXISTS dns_records (
    id              TEXT PRIMARY KEY,
    zone_id         TEXT NOT NULL,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL,
    value           TEXT NOT NULL,
    ttl             INTEGER DEFAULT 300,
    priority        INTEGER,
    weight          INTEGER,
    port            INTEGER,
    enabled         BOOLEAN DEFAULT TRUE,
    comment         TEXT,
    tags            TEXT,
    tag             TEXT,
    flag            INTEGER DEFAULT 0,
    owner           TEXT,
    owner_ref       TEXT,
    owner_generation INTEGER NOT NULL DEFAULT 0,
    expires_at      DATETIME,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dns_records_name ON dns_records(name);
CREATE INDEX IF NOT EXISTS idx_dns_records_name_type ON dns_records(name, type);
CREATE INDEX IF NOT EXISTS idx_dns_records_zone_id ON dns_records(zone_id);
CREATE INDEX IF NOT EXISTS idx_dns_records_type ON dns_records(type);
CREATE INDEX IF NOT EXISTS idx_dns_records_owner_ref ON dns_records(owner_ref);

-- ---------------------------------------------------------------------------
-- Authoritative local state. These rows are written here and nowhere else; the
-- control database holds a read-only replica for the console, the dependency
-- checks and backups.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS dhcp_leases (
    id          TEXT PRIMARY KEY,
    scope_id    TEXT NOT NULL,
    ip_address  TEXT NOT NULL,
    mac_address TEXT NOT NULL,
    hostname    TEXT,
    client_id   TEXT,
    lease_start DATETIME NOT NULL,
    lease_end   DATETIME NOT NULL,
    status      TEXT NOT NULL,
    last_seen   DATETIME NOT NULL,
    generation  INTEGER NOT NULL DEFAULT 1
);

-- Covers 'active' and 'offered': both mean a client is holding the address.
-- 'conflict' stays out for the same reason as in the control database -- a
-- quarantined row may legitimately sit next to a later binding for the same
-- address, and the uniqueness is about allocation, not about quarantine.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dp_dhcp_leases_scope_ip_held
    ON dhcp_leases(scope_id, ip_address) WHERE status IN ('active', 'offered');
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_leases_scope_id ON dhcp_leases(scope_id);
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_leases_ip_address ON dhcp_leases(ip_address);
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_leases_mac_address ON dhcp_leases(mac_address);
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_leases_status ON dhcp_leases(status);
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_leases_lease_end ON dhcp_leases(lease_end);

-- The durable DNS outbox. It lives here, next to the lease it describes, so
-- "the binding is committed and the update is owed" is one transaction.
CREATE TABLE IF NOT EXISTS dhcp_dns_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    lease_id        TEXT    NOT NULL,
    generation      INTEGER NOT NULL,
    action          TEXT    NOT NULL,
    scope_id        TEXT    NOT NULL DEFAULT '',
    ip_address      TEXT    NOT NULL DEFAULT '',
    mac_address     TEXT    NOT NULL DEFAULT '',
    hostname        TEXT    NOT NULL DEFAULT '',
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_error      TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'pending',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dp_dhcp_dns_events_pending
    ON dhcp_dns_events(status, next_attempt_at, id);
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_dns_events_lease ON dhcp_dns_events(lease_id);

-- The event log is written from the request path, so it belongs on the request
-- path's database: a control database that is locked or unreachable must not be
-- able to slow a client's DISCOVER down.
CREATE TABLE IF NOT EXISTS dhcp_logs (
    id          TEXT PRIMARY KEY,
    scope_id    TEXT,
    mac_address TEXT,
    ip_address  TEXT,
    event_type  TEXT NOT NULL,
    message     TEXT,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_dp_dhcp_logs_scope_id ON dhcp_logs(scope_id);
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_logs_mac_address ON dhcp_logs(mac_address);
CREATE INDEX IF NOT EXISTS idx_dp_dhcp_logs_created_at ON dhcp_logs(created_at);

-- ---------------------------------------------------------------------------
-- Lease changes owed to the control-plane replica.
--
-- A trigger records the id of every lease row this process creates, changes or
-- deletes. The pusher copies exactly those rows up and clears the marker, so
-- the cost of keeping the replica current is proportional to how fast leases
-- actually change rather than to how many leases exist.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS dhcp_lease_dirty (
    lease_id  TEXT PRIMARY KEY,
    deleted   INTEGER NOT NULL DEFAULT 0,
    queued_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Trigger bodies contain semicolons, so goose has to be told to hand the whole
-- definition to SQLite as one statement.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_lease_dirty_insert
AFTER INSERT ON dhcp_leases
BEGIN
    INSERT INTO dhcp_lease_dirty (lease_id, deleted, queued_at)
    VALUES (NEW.id, 0, datetime('now'))
    ON CONFLICT(lease_id) DO UPDATE SET deleted = 0, queued_at = datetime('now');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_lease_dirty_update
AFTER UPDATE ON dhcp_leases
BEGIN
    INSERT INTO dhcp_lease_dirty (lease_id, deleted, queued_at)
    VALUES (NEW.id, 0, datetime('now'))
    ON CONFLICT(lease_id) DO UPDATE SET deleted = 0, queued_at = datetime('now');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_lease_dirty_delete
AFTER DELETE ON dhcp_leases
BEGIN
    INSERT INTO dhcp_lease_dirty (lease_id, deleted, queued_at)
    VALUES (OLD.id, 1, datetime('now'))
    ON CONFLICT(lease_id) DO UPDATE SET deleted = 1, queued_at = datetime('now');
END;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Bookkeeping.
-- ---------------------------------------------------------------------------

-- One row per replication direction. source_revision is the control database's
-- revision the local replica was built from; a mismatch is what tells the data
-- plane it has work to do.
CREATE TABLE IF NOT EXISTS dataplane_sync_state (
    domain          TEXT PRIMARY KEY,
    source_revision INTEGER NOT NULL DEFAULT 0,
    synced_at       DATETIME,
    last_error      TEXT NOT NULL DEFAULT ''
);

-- Single-occurrence facts about this store.
--
--   leases_taken_over_at — when the control database's lease rows were copied
--                          in on the first start of the split build. Its
--                          presence is what stops that copy from running again
--                          and resurrecting leases an operator has since
--                          removed.
CREATE TABLE IF NOT EXISTS dataplane_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_lease_dirty_insert;
DROP TRIGGER IF EXISTS trg_dp_lease_dirty_update;
DROP TRIGGER IF EXISTS trg_dp_lease_dirty_delete;
DROP TABLE IF EXISTS dataplane_meta;
DROP TABLE IF EXISTS dataplane_sync_state;
DROP TABLE IF EXISTS dhcp_lease_dirty;
DROP TABLE IF EXISTS dhcp_logs;
DROP TABLE IF EXISTS dhcp_dns_events;
DROP TABLE IF EXISTS dhcp_leases;
DROP TABLE IF EXISTS dns_records;
DROP TABLE IF EXISTS dns_zones;
DROP TABLE IF EXISTS dhcp_options;
DROP TABLE IF EXISTS dhcp_reservations;
DROP TABLE IF EXISTS dhcp_scopes;
