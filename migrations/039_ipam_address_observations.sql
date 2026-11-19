-- +goose Up
-- BLK-02: separate authoritative allocation intent from local observations.
--
-- ipam_addresses.status is operator/config intent. observed_* columns are a
-- local data-plane fact: DHCP and reconcile write them, configuration
-- replication must not carry them, and retiring a parent must preserve them
-- alongside the address row. A separate table makes that boundary explicit and
-- gives each observation its own timestamp and provenance.
CREATE TABLE IF NOT EXISTS ipam_address_observations (
    address_id       TEXT PRIMARY KEY REFERENCES ipam_addresses(id) ON DELETE CASCADE,
    state            TEXT NOT NULL DEFAULT 'unknown',
    observed_at      DATETIME,
    source           TEXT NOT NULL DEFAULT '',
    mac_address      TEXT,
    hostname         TEXT,
    lease_id         TEXT,
    created_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at       DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Backfill from the legacy columns. Rows that never received an observation
-- still get a row so a single upsert can maintain the whole observation.
INSERT OR IGNORE INTO ipam_address_observations
    (address_id, state, observed_at, source, mac_address, hostname, lease_id, created_at, updated_at)
SELECT id, observed_state, observed_at, observed_source, observed_mac,
       observed_hostname, dhcp_lease_id, datetime('now'), datetime('now')
FROM ipam_addresses;

-- Keep existing writers compatible: any legacy observed_* write mirrors into
-- the authoritative split table in the same statement, so a caller cannot
-- update one representation and leave the other stale.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS ipam_addresses_observation_insert
AFTER INSERT ON ipam_addresses
BEGIN
  INSERT INTO ipam_address_observations
    (address_id, state, observed_at, source, mac_address, hostname, lease_id, created_at, updated_at)
  VALUES (NEW.id, NEW.observed_state, NEW.observed_at, NEW.observed_source,
          NEW.observed_mac, NEW.observed_hostname, NEW.dhcp_lease_id,
          datetime('now'), datetime('now'))
  ON CONFLICT(address_id) DO UPDATE SET
    state = excluded.state, observed_at = excluded.observed_at,
    source = excluded.source, mac_address = excluded.mac_address,
    hostname = excluded.hostname, lease_id = excluded.lease_id,
    updated_at = datetime('now');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS ipam_addresses_observation_update
AFTER UPDATE OF observed_state, observed_at, observed_source, observed_mac, observed_hostname, dhcp_lease_id ON ipam_addresses
BEGIN
  INSERT INTO ipam_address_observations
    (address_id, state, observed_at, source, mac_address, hostname, lease_id, created_at, updated_at)
  VALUES (NEW.id, NEW.observed_state, NEW.observed_at, NEW.observed_source,
          NEW.observed_mac, NEW.observed_hostname, NEW.dhcp_lease_id,
          datetime('now'), datetime('now'))
  ON CONFLICT(address_id) DO UPDATE SET
    state = excluded.state, observed_at = excluded.observed_at,
    source = excluded.source, mac_address = excluded.mac_address,
    hostname = excluded.hostname, lease_id = excluded.lease_id,
    updated_at = datetime('now');
END;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_ipam_address_observations_state
    ON ipam_address_observations(state);
CREATE INDEX IF NOT EXISTS idx_ipam_address_observations_source
    ON ipam_address_observations(source);

-- +goose Down
DROP TRIGGER IF EXISTS ipam_addresses_observation_update;
DROP TRIGGER IF EXISTS ipam_addresses_observation_insert;
DROP INDEX IF EXISTS idx_ipam_address_observations_source;
DROP INDEX IF EXISTS idx_ipam_address_observations_state;
DROP TABLE IF EXISTS ipam_address_observations;