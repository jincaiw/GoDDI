-- +goose Up
-- N6 migration-stage placement contract. Unified facts are produced beside
-- authoritative DHCP lease mutations in the lease data-plane store. The IPAM
-- consumer projects into the control-plane database in its own transaction;
-- SQLite does not provide a cross-file atomic commit here.
CREATE TABLE IF NOT EXISTS facts_storage_placement (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    producer_store  TEXT NOT NULL CHECK (producer_store = 'leases'),
    consumer_store  TEXT NOT NULL CHECK (consumer_store = 'control'),
    atomicity       TEXT NOT NULL CHECK (atomicity = 'per_store_only'),
    status          TEXT NOT NULL DEFAULT 'opt_in'
                    CHECK (status IN ('opt_in', 'production')),
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO facts_storage_placement
    (id, producer_store, consumer_store, atomicity, status)
VALUES
    (1, 'leases', 'control', 'per_store_only', 'opt_in');

-- +goose Down
DROP TABLE IF EXISTS facts_storage_placement;
