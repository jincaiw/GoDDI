-- Migration 025: index the DNS record value.
--
-- The 360° address view asks "which names publish this address" and answers it
-- by reading `dns_records` directly, matching A/AAAA records on `value`. There
-- was no index on that column: the four existing ones all lead with zone_id,
-- name or type, none of which an equality on value can use. Without this the
-- question is a full scan of the record table on every detail-view request.
--
-- Only `value` is indexed, not (type, value). The lookup is always "the A or
-- AAAA record whose value is this address", so a leading `type` column would
-- not narrow an equality predicate on the second column, and the type filter is
-- applied as a cheap residual either way. The reverse direction -- a PTR whose
-- *name* is the reverse name of this address -- is already served by
-- idx_dns_records_name and needs nothing here.
--
-- Expand only: an index adds no column and no constraint, so a binary that
-- predates this migration reads the database exactly as before.

-- +goose Up
CREATE INDEX IF NOT EXISTS idx_dns_records_value ON dns_records(value);

-- +goose Down
DROP INDEX IF EXISTS idx_dns_records_value;
