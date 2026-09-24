-- +goose Up
-- Durable producer-to-control delivery markers for the unified fact stream.
CREATE TABLE IF NOT EXISTS dhcp_ipam_observation_event_dirty (
    event_id TEXT PRIMARY KEY,
    queued_at DATETIME NOT NULL DEFAULT (datetime('now')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at DATETIME,
    last_error TEXT NOT NULL DEFAULT ''
);

-- Events are immutable once enqueued. Delivery status changes must not enqueue
-- them again, so only insertion creates an upward marker.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_ipam_fact_dirty_insert
AFTER INSERT ON dhcp_ipam_observation_events
BEGIN
    INSERT INTO dhcp_ipam_observation_event_dirty(event_id, queued_at)
    VALUES (NEW.event_id, datetime('now'))
    ON CONFLICT(event_id) DO UPDATE SET queued_at = datetime('now');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_ipam_fact_dirty_insert;
DROP TABLE IF EXISTS dhcp_ipam_observation_event_dirty;
