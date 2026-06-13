-- +goose Up
ALTER TABLE backup_jobs ADD COLUMN description TEXT;

-- +goose Down
ALTER TABLE backup_jobs DROP COLUMN description;
