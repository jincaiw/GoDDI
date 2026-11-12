-- +goose Up
-- migration-contract: retire-allowed
-- Round 287: the new architecture has no legacy DNS outbox. Existing data
-- plane stores still carry the queue table, its marker table and the two
-- triggers; drop them all so the schema matches a fresh install. The unified
-- facts inbox (dhcp_ipam_observation_events) is the only DNS work queue.
DROP TRIGGER IF EXISTS trg_dp_dns_event_dirty_insert;
DROP TRIGGER IF EXISTS trg_dp_dns_event_dirty_update;
DROP TABLE IF EXISTS dhcp_dns_event_dirty;
DROP INDEX IF EXISTS idx_dp_dhcp_dns_events_pending;
DROP INDEX IF EXISTS idx_dp_dhcp_dns_events_lease;
DROP TABLE IF EXISTS dhcp_dns_events;
-- +goose Down
-- Irreversible by design: the legacy queue is not part of the new schema.
SELECT 1;
