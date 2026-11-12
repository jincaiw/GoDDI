-- +goose Up
-- The unified facts domain: the control-side inbox's rows (written by the
-- data-plane relay) drive the DNS plane's facts replica. Every relayed fact
-- bumps the facts revision so the DNS sync notices without polling the whole
-- table; the consumer's own completion writes do not bump it (they are local
-- state, not new facts).
INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('facts', 0);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS trg_dp_rev_facts_inbox_ins
AFTER INSERT ON dhcp_ipam_observation_events
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1, updated_at = datetime('now') WHERE domain = 'facts';
END;
-- +goose StatementEnd
-- +goose Down
DROP TRIGGER IF EXISTS trg_dp_rev_facts_inbox_ins;
DELETE FROM dataplane_revision WHERE domain = 'facts';
