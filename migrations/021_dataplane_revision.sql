-- +goose Up
-- The control plane's change counter for the data planes.
--
-- A DNS or DHCP process copies its configuration out of this database rather
-- than reading it per request, so it has to know when its copy is stale.
-- Polling for changes by comparing rows would mean reading the tables it is
-- trying to avoid depending on, and comparing timestamps would miss any write
-- that does not touch the column being compared.
--
-- Triggers are used instead of bumping the counter from the API handlers: a
-- counter maintained by hand is only correct as long as every future write path
-- remembers to bump it, and the failure mode is a data plane serving
-- configuration that was changed hours ago with nothing anywhere to show it.
-- A trigger cannot be forgotten.
--
-- The counter is per domain: a change to a DHCP scope has no bearing on a DNS
-- node, and vice versa. dns_records is included because it is written outside
-- the console too -- a DHCP binding publishing its name, and an RFC 2136 update
-- from an authorised client, both land there. Those are exactly the changes a
-- DNS data plane most needs to see promptly; leaving them out would mean a
-- record that appears in the console but never resolves.
--
-- Every trigger below is wrapped in StatementBegin/StatementEnd because goose
-- splits on semicolons and a trigger body contains one.

-- The counter table comes first: a trigger body is resolved against the schema
-- when it is created, so the table it updates has to exist already.
CREATE TABLE IF NOT EXISTS dataplane_revision (
    domain     TEXT PRIMARY KEY,
    revision   INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('dhcp', 0);
INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('dns', 0);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_scopes_ins
AFTER INSERT ON dhcp_scopes
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_scopes_upd
AFTER UPDATE ON dhcp_scopes
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_scopes_del
AFTER DELETE ON dhcp_scopes
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_reservations_ins
AFTER INSERT ON dhcp_reservations
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_reservations_upd
AFTER UPDATE ON dhcp_reservations
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_reservations_del
AFTER DELETE ON dhcp_reservations
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_options_ins
AFTER INSERT ON dhcp_options
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_options_upd
AFTER UPDATE ON dhcp_options
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_options_del
AFTER DELETE ON dhcp_options
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dhcp';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_zones_ins
AFTER INSERT ON dns_zones
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_zones_upd
AFTER UPDATE ON dns_zones
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_zones_del
AFTER DELETE ON dns_zones
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_records_ins
AFTER INSERT ON dns_records
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_records_upd
AFTER UPDATE ON dns_records
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dns_records_del
AFTER DELETE ON dns_records
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_ins;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_zones_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_zones_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dns_zones_ins;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_options_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_options_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_options_ins;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_reservations_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_reservations_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_reservations_ins;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_scopes_del;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_scopes_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_dhcp_scopes_ins;
DROP TABLE IF EXISTS dataplane_revision;
