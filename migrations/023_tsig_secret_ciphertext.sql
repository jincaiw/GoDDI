-- Migration 023: TSIG secrets are sealed at rest.
--
-- expand half of an expand/contract pair (see ADR 0002). The sealed column is
-- added here and backfilled in Go at startup — SQLite cannot encrypt — and the
-- old `secret` column is dropped in a later release, once no supported binary
-- reads it.
--
-- Unlike a textbook expand phase, the old column is NOT kept in sync for the
-- length of the window. The defect being fixed is that the secret exists in the
-- clear at all; a dual-write window would keep it there for as long as it
-- lasted. The cost is that rolling back to a binary that predates this
-- migration leaves nothing for it to read, so it refuses signed transfers
-- until the pre-upgrade snapshot is restored. That is fail-closed, and it is
-- recorded in the runbook rather than left to be discovered.
--
-- +goose Up
ALTER TABLE dns_tsig_keys ADD COLUMN secret_ciphertext TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE dns_tsig_keys DROP COLUMN secret_ciphertext;
