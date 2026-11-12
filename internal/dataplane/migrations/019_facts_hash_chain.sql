-- +goose Up
ALTER TABLE dhcp_ipam_observation_events ADD COLUMN epoch INTEGER NOT NULL DEFAULT 0;
ALTER TABLE dhcp_ipam_observation_events ADD COLUMN prev_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE dhcp_ipam_observation_events ADD COLUMN payload_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE dhcp_ipam_observation_events ADD COLUMN tip_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE dhcp_ipam_observation_events DROP COLUMN tip_hash;
ALTER TABLE dhcp_ipam_observation_events DROP COLUMN payload_hash;
ALTER TABLE dhcp_ipam_observation_events DROP COLUMN prev_hash;
ALTER TABLE dhcp_ipam_observation_events DROP COLUMN epoch;
