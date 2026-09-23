-- +goose Up
ALTER TABLE dns_zone_changes ADD COLUMN flag INTEGER;
ALTER TABLE dns_zone_changes ADD COLUMN tag TEXT;

-- +goose Down
ALTER TABLE dns_zone_changes DROP COLUMN tag;
ALTER TABLE dns_zone_changes DROP COLUMN flag;
