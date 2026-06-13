-- +goose Up
-- GoDDI System Tables: settings, audit logs, backup jobs, and backup files.

CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    description TEXT,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    user_id TEXT,
    username TEXT,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    detail TEXT,
    source_ip TEXT,
    user_agent TEXT,
    success BOOLEAN DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS backup_jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    status TEXT NOT NULL,
    file_path TEXT,
    size_bytes INTEGER,
    started_at DATETIME,
    completed_at DATETIME,
    error TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS backup_files (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES backup_jobs(id) ON DELETE CASCADE,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    size_bytes INTEGER,
    checksum TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Indexes for system tables.
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_resource_type ON audit_logs(resource_type);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX idx_audit_logs_resource_type_id ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_backup_jobs_status ON backup_jobs(status);
CREATE INDEX idx_backup_jobs_created_at ON backup_jobs(created_at);
CREATE INDEX idx_backup_files_job_id ON backup_files(job_id);

-- +goose Down
DROP TABLE IF EXISTS backup_files;
DROP TABLE IF EXISTS backup_jobs;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS system_settings;
