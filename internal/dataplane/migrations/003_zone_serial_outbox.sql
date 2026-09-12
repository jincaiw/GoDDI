-- +goose Up
-- The zone serials this store moved, owed upward.
--
-- ---------------------------------------------------------------------------
-- Why a serial bump has to travel at all
-- ---------------------------------------------------------------------------
-- The zone row is a replica: the control plane authors the zone -- its name,
-- its SOA, its policies -- and this store copies it in. But the serial is not
-- only configuration. It is the number this store advertises in the SOA of
-- every answer and every transfer, and it is what a secondary compares to
-- decide whether it needs a transfer at all.
--
-- Two writers move it here:
--
--   * an RFC 2136 dynamic update (a client adding its own name), and
--   * an inbound secondary transfer, which adopts the primary's serial.
--
-- The control database does not see either. It computes the next serial from
-- the largest serial *it* holds, and the downward sync replaces the zone row
-- wholesale. So a bump that is not pushed up is overwritten by a value the
-- console computed without it.
--
-- That is not a stale number on a screen. The console's next record edit would
-- reuse a serial this store has already published, every secondary would
-- conclude the zone had not changed, and the edit would never be transferred.
-- The counter moves; the zone does not.
--
-- ---------------------------------------------------------------------------
-- Why the marker carries the serial
-- ---------------------------------------------------------------------------
-- The push has to be monotonic at both ends, and the value to publish is not
-- the one in dns_zones -- the sync may already have overwritten it. The marker
-- holding the serial is what makes the push immune to that: it is written by
-- the bump itself, and it survives the sync replacing the row.
--
-- The sync then re-applies it (see the DNS branch of Replicator.apply), so a
-- downward replace can never move this store's advertised serial backwards
-- either.

CREATE TABLE IF NOT EXISTS dns_zone_serial_dirty (
    zone_id   TEXT PRIMARY KEY,
    serial    INTEGER NOT NULL,
    queued_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Only an increase is owed upward. A serial that moved backwards is the sync
-- replacing the row, not a bump, and queueing it would send the control plane
-- a value it is already ahead of -- and, worse, would mark the zone as having
-- local work when it does not.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_zone_serial_dirty_update
AFTER UPDATE OF serial ON dns_zones
WHEN NEW.serial > OLD.serial
BEGIN
    INSERT INTO dns_zone_serial_dirty (zone_id, serial, queued_at)
    VALUES (NEW.id, NEW.serial, datetime('now'))
    ON CONFLICT(zone_id) DO UPDATE SET
        serial    = MAX(dns_zone_serial_dirty.serial, excluded.serial),
        queued_at = excluded.queued_at;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_zone_serial_dirty_update;
DROP TABLE IF EXISTS dns_zone_serial_dirty;
