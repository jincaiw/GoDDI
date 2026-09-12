-- +goose Up
-- Who authored a DNS record, and the two counters the second data plane needs.
--
-- ---------------------------------------------------------------------------
-- 1. Authorship
-- ---------------------------------------------------------------------------
-- A record in dns_records now has one of two authors, and they must not be
-- allowed to overwrite each other:
--
--   authored_locally = 0  the control plane wrote it (the console, the API, a
--                         zone import). It is configuration, and a DNS data
--                         plane copies it downward.
--   authored_locally = 1  a data plane wrote it: the DHCP-to-DNS linkage
--                         publishing a confirmed binding, an RFC 2136 dynamic
--                         update from an authorised client, or an inbound zone
--                         transfer. The data plane owns these rows.
--
-- This column exists because no existing column can answer the question.
-- owner_ref identifies a DHCP binding, but a dynamic update writes neither
-- owner nor owner_ref, so its rows are indistinguishable from a hand-typed
-- record. And 'owner' is submitted by the caller -- an operator may legitimately
-- set it to any string, including 'dhcp', which would then be misread as
-- data-plane authorship.
--
-- ---------------------------------------------------------------------------
-- 2. The revision counters, and a loop they would otherwise create
-- ---------------------------------------------------------------------------
-- From here on the DNS data plane both reads its records from the control
-- database and writes to it, so a counter that moves on *every* write to
-- dns_records would make the data plane fetch back the row it has just pushed
-- up. That is the same trap that was already avoided for leases, and the fix is
-- the same: only the author's writes may move the counter.
--
--   * a console write (authored_locally = 0) bumps 'dns' -- the data plane has
--     configuration to pick up;
--   * a row a data plane pushed up (authored_locally = 1) does not, because the
--     control database's copy of it is a reporting artifact, not news.
--
-- The two new counters cover the material the second data plane needs and the
-- first one does not:
--
--   'ddns'    the outbox rows a DHCP process owes the DNS side. It moves on
--             insert only: the consumer orders by id and keeps its own retry
--             state, so a row's status changing is not something anyone has to
--             be told about.
--   'leases'  the lease replica, which the DNS side needs to answer "does the
--             binding behind this record still exist?". It is separate from
--             'dhcp' because lease churn is constant and scope configuration is
--             not; sharing one counter would make every lease renewal look like
--             a configuration change.
--
-- Note that the previous migration's comment on dns_records said the counter had
-- to move for DHCP publications and RFC 2136 updates because those were changes
-- "a DNS data plane most needs to see promptly". That was true while the data
-- plane only read records. Now that it authors them, bumping on its own writes
-- is the loop described above, and the trigger is narrowed accordingly.

ALTER TABLE dns_records ADD COLUMN authored_locally INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_dns_records_authored_locally ON dns_records(authored_locally);

-- The counters first: a trigger body is resolved against the schema when it is
-- created, so the rows it updates have to exist already.
INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('ddns', 0);
INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('leases', 0);

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_ins;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_dp_rev_dns_records_ins
AFTER INSERT ON dns_records
WHEN NEW.authored_locally = 0
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_upd;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_dp_rev_dns_records_upd
AFTER UPDATE ON dns_records
WHEN NEW.authored_locally = 0
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_del;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_dp_rev_dns_records_del
AFTER DELETE ON dns_records
WHEN OLD.authored_locally = 0
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_ddns_events_ins
AFTER INSERT ON dhcp_dns_events
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'ddns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_leases_ins
AFTER INSERT ON dhcp_leases
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'leases';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_leases_upd
AFTER UPDATE ON dhcp_leases
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'leases';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_leases_del
AFTER DELETE ON dhcp_leases
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'leases';
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_rev_leases_del;
DROP TRIGGER IF EXISTS trg_dp_rev_leases_upd;
DROP TRIGGER IF EXISTS trg_dp_rev_leases_ins;
DROP TRIGGER IF EXISTS trg_dp_rev_ddns_events_ins;

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_ins;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_upd;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_dp_rev_dns_records_del;
-- +goose StatementEnd

-- Restore the original triggers, which moved the counter on every write.
-- +goose StatementBegin
CREATE TRIGGER trg_dp_rev_dns_records_ins
AFTER INSERT ON dns_records
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_dp_rev_dns_records_upd
AFTER UPDATE ON dns_records
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_dp_rev_dns_records_del
AFTER DELETE ON dns_records
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'dns';
END;
-- +goose StatementEnd

DELETE FROM dataplane_revision WHERE domain IN ('ddns', 'leases');
DROP INDEX IF EXISTS idx_dns_records_authored_locally;
ALTER TABLE dns_records DROP COLUMN authored_locally;
