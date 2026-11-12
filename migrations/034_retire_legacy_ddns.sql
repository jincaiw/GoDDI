-- +goose Up
-- migration-contract: retire-allowed
-- Round 287: the new architecture has no legacy DNS outbox. Existing
-- databases (created before this migration) still carry the table, its
-- indexes, the revision counter and the insert trigger; drop them all so the
-- schema is identical to a fresh install. This is intentionally irreversible:
-- the unified facts chain is the only DNS producer and there is nothing to
-- migrate back to.
DROP TRIGGER IF EXISTS trg_dp_rev_ddns_events_ins;
DROP INDEX IF EXISTS idx_dhcp_dns_events_pending;
DROP INDEX IF EXISTS idx_dhcp_dns_events_lease;
DROP TABLE IF EXISTS dhcp_dns_events;
DELETE FROM dataplane_revision WHERE domain = 'ddns';
-- +goose Down
-- Irreversible by design: the legacy queue is not part of the new schema.
SELECT 1;
