package backup

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/metrics"
)

// ErrNotFound is returned when a backup job does not exist. Callers use it to
// distinguish "no such backup" (404) from real failures (500).
var ErrNotFound = errors.New("backup job not found")

// BackupOptions specifies options for creating a backup.
type BackupOptions struct {
	Type        string `json:"type"` // full, dns, dhcp, ipam, security, config
	Description string `json:"description"`
}

// BackupJob represents a backup job record.
type BackupJob struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"` // pending, running, completed, failed
	FilePath    string `json:"file_path,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// BackupFilter specifies filter criteria for listing backups.
type BackupFilter struct {
	Type     string `json:"type"`
	Status   string `json:"status"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// BackupFile is the structure of a backup file on disk.
type BackupFile struct {
	Metadata BackupMetadata `json:"metadata"`
	Data     BackupData     `json:"data"`
}

// BackupMetadata contains metadata about the backup.
type BackupMetadata struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
}

// BackupData contains the actual backup data sections.
type BackupData struct {
	DNS      json.RawMessage `json:"dns,omitempty"`
	DHCP     json.RawMessage `json:"dhcp,omitempty"`
	IPAM     json.RawMessage `json:"ipam,omitempty"`
	Security json.RawMessage `json:"security,omitempty"`
	Config   json.RawMessage `json:"config,omitempty"`
}

// Manager manages backup operations.
type Manager struct {
	db      *sql.DB
	dataDir string
	version string
	// runMu serialises backup job creation. Two concurrent CreateBackup
	// calls would otherwise both INSERT a "pending" job, both try to grab
	// the same tempdir / sqlite snapshot lock, and may corrupt the output
	// file. A single global mutex is fine because backups are infrequent
	// and intentionally slow operations.
	runMu sync.Mutex
}

// NewManager creates a new backup manager.
func NewManager(db *sql.DB, dataDir string, version string) *Manager {
	return &Manager{
		db:      db,
		dataDir: dataDir,
		version: version,
	}
}

// CreateBackup creates a new backup job and executes it.
func (m *Manager) CreateBackup(opts BackupOptions) (*BackupJob, error) {
	// Serialise the whole operation: only one backup may be in flight at a
	// time. Holding the lock for the duration of the backup is acceptable
	// because backups are expected to be infrequent and we want a strict
	// "no two concurrent backups" guarantee.
	m.runMu.Lock()
	defer m.runMu.Unlock()

	id := uuid.New().String()
	now := time.Now().Format(time.RFC3339)

	job := &BackupJob{
		ID:          id,
		Type:        opts.Type,
		Status:      "pending",
		Description: opts.Description,
		CreatedAt:   now,
	}

	// Insert job record.
	_, err := m.db.Exec(
		`INSERT INTO backup_jobs (id, type, status, description, started_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, opts.Type, "pending", opts.Description, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("creating backup job: %w", err)
	}

	// Update status to running.
	job.Status = "running"
	if _, err := m.db.Exec(`UPDATE backup_jobs SET status = ? WHERE id = ?`, "running", id); err != nil {
		slog.Error("failed to update backup job status to running", "id", id, "error", err)
	}

	// Execute backup.
	backupFile, err := m.executeBackup(job)
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		m.db.Exec(`UPDATE backup_jobs SET status = ?, error = ? WHERE id = ?`, "failed", err.Error(), id)
		metrics.RecordBackupJob("failed")
		return job, nil
	}

	// Update job as completed.
	completedAt := time.Now().Format(time.RFC3339)
	job.Status = "completed"
	job.FilePath = backupFile
	job.CompletedAt = completedAt

	_, err = m.db.Exec(
		`UPDATE backup_jobs SET status = ?, file_path = ?, completed_at = ? WHERE id = ?`,
		"completed", backupFile, completedAt, id,
	)
	if err != nil {
		slog.Error("updating backup job status", "error", err)
	}

	metrics.RecordBackupJob("completed")
	return job, nil
}

// executeBackup performs the actual backup operation.
func (m *Manager) executeBackup(job *BackupJob) (string, error) {
	// Ensure backup directory exists.
	backupDir := filepath.Join(m.dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	// Build backup data.
	backupData := BackupData{}
	var err error

	switch job.Type {
	case "full":
		backupData.DNS, err = ExportDNS(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting DNS data: %w", err)
		}
		backupData.DHCP, err = ExportDHCP(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting DHCP data: %w", err)
		}
		backupData.IPAM, err = ExportIPAM(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting IPAM data: %w", err)
		}
		backupData.Security, err = ExportSecurity(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting security data: %w", err)
		}
		backupData.Config, err = ExportConfig(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting config data: %w", err)
		}
	case "dns":
		backupData.DNS, err = ExportDNS(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting DNS data: %w", err)
		}
	case "dhcp":
		backupData.DHCP, err = ExportDHCP(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting DHCP data: %w", err)
		}
	case "ipam":
		backupData.IPAM, err = ExportIPAM(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting IPAM data: %w", err)
		}
	case "security":
		backupData.Security, err = ExportSecurity(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting security data: %w", err)
		}
	case "config":
		backupData.Config, err = ExportConfig(m.db)
		if err != nil {
			return "", fmt.Errorf("exporting config data: %w", err)
		}
	default:
		return "", fmt.Errorf("unknown backup type: %s", job.Type)
	}

	// Build backup file.
	backupFile := BackupFile{
		Metadata: BackupMetadata{
			ID:          job.ID,
			Type:        job.Type,
			CreatedAt:   time.Now(),
			Version:     m.version,
			Description: job.Description,
		},
		Data: backupData,
	}

	// Serialize to JSON.
	data, err := json.MarshalIndent(backupFile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling backup data: %w", err)
	}

	// Write to file. Include a UUID suffix to guarantee uniqueness even when
	// two backups complete within the same second.
	fileName := fmt.Sprintf("%s_%s_%s_%s.json", job.Type, time.Now().Format("20060102_150405"), job.ID[:8], uuid.New().String()[:8])
	filePath := filepath.Join(backupDir, fileName)
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return "", fmt.Errorf("writing backup file: %w", err)
	}

	// Get file size.
	fileInfo, _ := os.Stat(filePath)
	var sizeBytes int64
	if fileInfo != nil {
		sizeBytes = fileInfo.Size()
	}

	// Update size in database.
	m.db.Exec(`UPDATE backup_jobs SET file_path = ?, size_bytes = ? WHERE id = ?`, filePath, sizeBytes, job.ID)
	job.SizeBytes = sizeBytes

	slog.Info("backup created", "id", job.ID, "type", job.Type, "size", sizeBytes)
	return filePath, nil
}

// RestoreBackup restores data from a backup.
func (m *Manager) RestoreBackup(jobID string) error {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	// No second confirmation / MFA prompt is performed here. Authorization
	// is the responsibility of the HTTP layer (RBAC + audit log entry). The
	// backup payload itself is treated as trusted because it is produced
	// by this very process and stored locally; an attacker who can call
	// this endpoint has already been authenticated, so adding a "are you
	// sure?" prompt would just push the same trust assumption onto the UI
	// without raising the security bar in any meaningful way.
	job, err := m.GetBackup(jobID)
	if err != nil {
		return fmt.Errorf("getting backup job: %w", err)
	}

	if job.Status != "completed" {
		return fmt.Errorf("backup job %s is not completed (status: %s)", jobID, job.Status)
	}

	if job.FilePath == "" {
		return fmt.Errorf("backup file path is empty for job %s", jobID)
	}

	// Resolve and verify the on-disk path before reading.
	safePath, err := m.safeBackupPath(job.FilePath)
	if err != nil {
		return err
	}

	// Read backup file.
	data, err := os.ReadFile(safePath)
	if err != nil {
		return fmt.Errorf("reading backup file: %w", err)
	}

	var backupFile BackupFile
	if err := json.Unmarshal(data, &backupFile); err != nil {
		return fmt.Errorf("parsing backup file: %w", err)
	}

	// Each backup section is restored in its own transaction (R3-1).
	// A single mega-transaction over the whole restore would monopolise
	// the single SQLite write connection for the entire run (with
	// SetMaxOpenConns(1) every other query queues behind it) and risks
	// hitting busy_timeout on large datasets. Per-section transactions
	// keep each section atomic (delete + insert of one domain) while
	// releasing the connection between sections, so concurrent API
	// traffic can interleave.
	var errs []string
	var restored []string

	if err := m.restoreSection("DNS", backupFile.Data.DNS, restoreDNS); err != nil {
		slog.Error("failed to restore DNS data", "error", err)
		errs = append(errs, fmt.Sprintf("DNS: %v", err))
	} else if len(backupFile.Data.DNS) > 0 {
		restored = append(restored, "DNS")
	}
	if err := m.restoreSection("DHCP", backupFile.Data.DHCP, restoreDHCP); err != nil {
		slog.Error("failed to restore DHCP data", "error", err)
		errs = append(errs, fmt.Sprintf("DHCP: %v", err))
	} else if len(backupFile.Data.DHCP) > 0 {
		restored = append(restored, "DHCP")
	}
	if err := m.restoreSection("IPAM", backupFile.Data.IPAM, restoreIPAM); err != nil {
		slog.Error("failed to restore IPAM data", "error", err)
		errs = append(errs, fmt.Sprintf("IPAM: %v", err))
	} else if len(backupFile.Data.IPAM) > 0 {
		restored = append(restored, "IPAM")
	}
	if err := m.restoreSection("Security", backupFile.Data.Security, restoreSecurity); err != nil {
		slog.Error("failed to restore security data", "error", err)
		errs = append(errs, fmt.Sprintf("Security: %v", err))
	} else if len(backupFile.Data.Security) > 0 {
		restored = append(restored, "Security")
	}
	if err := m.restoreSection("Config", backupFile.Data.Config, restoreConfig); err != nil {
		slog.Error("failed to restore config data", "error", err)
		errs = append(errs, fmt.Sprintf("Config: %v", err))
	} else if len(backupFile.Data.Config) > 0 {
		restored = append(restored, "Config")
	}

	if len(errs) > 0 {
		return fmt.Errorf("backup restore partially failed (restored: %s): %s",
			strings.Join(restored, ","), strings.Join(errs, "; "))
	}

	slog.Info("backup restored", "id", jobID, "type", backupFile.Metadata.Type)
	return nil
}

// ListBackups lists backup jobs with filtering and pagination.
func (m *Manager) ListBackups(filter BackupFilter) ([]BackupJob, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}

	// Build query.
	query := "SELECT id, type, status, file_path, size_bytes, description, error, started_at, completed_at, created_at FROM backup_jobs WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM backup_jobs WHERE 1=1"
	args := []interface{}{}

	if filter.Type != "" {
		query += " AND type = ?"
		countQuery += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		countQuery += " AND status = ?"
		args = append(args, filter.Status)
	}

	// Count total.
	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := m.db.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting backups: %w", err)
	}

	// Add ordering and pagination.
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing backups: %w", err)
	}
	defer rows.Close()

	jobs := make([]BackupJob, 0)
	for rows.Next() {
		var j BackupJob
		var filePath, description, errMsg, startedAt, completedAt sql.NullString
		var sizeBytes sql.NullInt64

		if err := rows.Scan(&j.ID, &j.Type, &j.Status, &filePath, &sizeBytes, &description, &errMsg, &startedAt, &completedAt, &j.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning backup job: %w", err)
		}

		if filePath.Valid {
			j.FilePath = filePath.String
		}
		if sizeBytes.Valid {
			j.SizeBytes = sizeBytes.Int64
		}
		if description.Valid {
			j.Description = description.String
		}
		if errMsg.Valid {
			j.Error = errMsg.String
		}
		if startedAt.Valid {
			j.StartedAt = startedAt.String
		}
		if completedAt.Valid {
			j.CompletedAt = completedAt.String
		}

		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating backup jobs: %w", err)
	}

	return jobs, total, nil
}

// GetBackup returns a single backup job by ID.
func (m *Manager) GetBackup(id string) (*BackupJob, error) {
	var j BackupJob
	var filePath, description, errMsg, startedAt, completedAt sql.NullString
	var sizeBytes sql.NullInt64

	err := m.db.QueryRow(
		`SELECT id, type, status, file_path, size_bytes, description, error, started_at, completed_at, created_at FROM backup_jobs WHERE id = ?`,
		id,
	).Scan(&j.ID, &j.Type, &j.Status, &filePath, &sizeBytes, &description, &errMsg, &startedAt, &completedAt, &j.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backup job %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("querying backup job: %w", err)
	}

	if filePath.Valid {
		j.FilePath = filePath.String
	}
	if sizeBytes.Valid {
		j.SizeBytes = sizeBytes.Int64
	}
	if description.Valid {
		j.Description = description.String
	}
	if errMsg.Valid {
		j.Error = errMsg.String
	}
	if startedAt.Valid {
		j.StartedAt = startedAt.String
	}
	if completedAt.Valid {
		j.CompletedAt = completedAt.String
	}

	return &j, nil
}

// DeleteBackup deletes a backup job and its file.
func (m *Manager) DeleteBackup(id string) error {
	job, err := m.GetBackup(id)
	if err != nil {
		return err
	}

	// Remove backup file. Use safeBackupPath to ensure path traversal protection
	// is applied even when the on-disk path was tampered with.
	if job.FilePath != "" {
		safePath, pathErr := m.safeBackupPath(job.FilePath)
		if pathErr != nil {
			slog.Warn("refusing to remove backup file with unsafe path", "path", job.FilePath, "error", pathErr)
		} else if err := os.Remove(safePath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove backup file", "path", safePath, "error", err)
		}
	}

	// Delete from database.
	_, err = m.db.Exec(`DELETE FROM backup_jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting backup job: %w", err)
	}

	return nil
}

// DownloadBackup returns a streaming reader for the backup file content along
// with the file name. The caller is responsible for closing the returned
// ReadCloser.
func (m *Manager) DownloadBackup(id string) (io.ReadCloser, string, error) {
	job, err := m.GetBackup(id)
	if err != nil {
		return nil, "", err
	}

	if job.FilePath == "" {
		return nil, "", fmt.Errorf("backup file not available")
	}

	// Security: verify the file path is within the backup directory to prevent path traversal.
	safePath, err := m.safeBackupPath(job.FilePath)
	if err != nil {
		return nil, "", err
	}

	f, err := os.Open(safePath)
	if err != nil {
		return nil, "", fmt.Errorf("opening backup file: %w", err)
	}

	fileName := filepath.Base(safePath)
	return f, fileName, nil
}

// safeBackupPath resolves the given file path and ensures it resides within the
// configured backup directory. It returns the cleaned absolute path or an
// error if the path is invalid or escapes the backup directory.
func (m *Manager) safeBackupPath(filePath string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("backup file path is empty")
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("resolving backup file path: %w", err)
	}
	backupDir := filepath.Join(m.dataDir, "backups")
	absBackupDir, err := filepath.Abs(backupDir)
	if err != nil {
		return "", fmt.Errorf("resolving backup directory: %w", err)
	}
	if !strings.HasPrefix(absPath, absBackupDir+string(filepath.Separator)) {
		return "", fmt.Errorf("backup file path is outside the backup directory")
	}
	return absPath, nil
}

// --- Restore helpers ---

// restoreSection restores one backup section (DNS, DHCP, ...) inside its
// own transaction. Empty sections are skipped. On failure the section
// transaction is rolled back, leaving that section untouched.
func (m *Manager) restoreSection(name string, data json.RawMessage, fn func(execOrQuery, json.RawMessage) error) error {
	if len(data) == 0 {
		return nil
	}
	start := time.Now()
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning %s restore transaction: %w", name, err)
	}
	if err := fn(tx, data); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing %s restore transaction: %w", name, err)
	}
	slog.Info("backup section restored", "section", name, "duration", time.Since(start).String())
	return nil
}

// execOrQuery is the minimum set of methods shared by *sql.DB and *sql.Tx, so
// that restore helpers can run either inside a transaction or directly on the
// database connection.
type execOrQuery interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func restoreDNS(db execOrQuery, data json.RawMessage) error {
	var d struct {
		Zones                 []map[string]interface{} `json:"zones"`
		Records               []map[string]interface{} `json:"records"`
		Forwarders            []map[string]interface{} `json:"forwarders"`
		ConditionalForwarders []map[string]interface{} `json:"conditional_forwarders"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("parsing DNS backup: %w", err)
	}
	for _, table := range []string{"dns_records", "dns_conditional_forwarders", "dns_forwarders", "dns_zones"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}
	if err := restoreRows(db, "dns_zones", d.Zones,
		[]string{"id", "name", "type", "enabled", "dnssec_enabled", "default_ttl", "soa_mname", "soa_rname", "serial", "refresh", "retry", "expire", "minimum", "transfer_policy", "update_policy", "created_at", "updated_at"},
		map[string]interface{}{"enabled": true, "dnssec_enabled": false, "default_ttl": 3600, "soa_mname": "ns1.example.com", "soa_rname": "admin.example.com", "serial": 1, "refresh": 3600, "retry": 600, "expire": 86400, "minimum": 300}); err != nil {
		return err
	}
	if err := restoreRows(db, "dns_records", d.Records,
		[]string{"id", "zone_id", "name", "type", "value", "ttl", "priority", "weight", "port", "enabled", "comment", "tags", "tag", "flag", "owner", "expires_at", "created_at", "updated_at"},
		map[string]interface{}{"ttl": 300, "enabled": true, "flag": 0}); err != nil {
		return err
	}
	if err := restoreRows(db, "dns_forwarders", d.Forwarders,
		[]string{"id", "name", "protocol", "address", "enabled", "priority", "created_at", "updated_at"},
		map[string]interface{}{"protocol": "udp", "enabled": true, "priority": 0}); err != nil {
		return err
	}
	return restoreRows(db, "dns_conditional_forwarders", d.ConditionalForwarders,
		[]string{"id", "domain", "forwarder_ids", "enabled", "created_at", "updated_at"},
		map[string]interface{}{"enabled": true})
}

func restoreDHCP(db execOrQuery, data json.RawMessage) error {
	var d struct {
		Scopes       []map[string]interface{} `json:"scopes"`
		Options      []map[string]interface{} `json:"options"`
		Reservations []map[string]interface{} `json:"reservations"`
		Leases       []map[string]interface{} `json:"leases"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("parsing DHCP backup: %w", err)
	}
	for _, table := range []string{"dhcp_options", "dhcp_reservations", "dhcp_leases", "dhcp_scopes"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}
	if err := restoreRows(db, "dhcp_scopes", d.Scopes,
		[]string{"id", "name", "interface", "subnet", "start_ip", "end_ip", "subnet_mask", "router", "dns_servers", "ntp_servers", "domain_name", "lease_time", "max_lease_time", "enabled", "ping_check_enabled", "dns_updates", "comment", "created_at", "updated_at"},
		map[string]interface{}{"lease_time": 86400, "enabled": true, "ping_check_enabled": true, "dns_updates": false}); err != nil {
		return err
	}
	if err := restoreRows(db, "dhcp_reservations", d.Reservations,
		[]string{"id", "scope_id", "ip_address", "mac_address", "hostname", "description", "enabled", "created_at", "updated_at"},
		map[string]interface{}{"enabled": true}); err != nil {
		return err
	}
	if err := restoreRows(db, "dhcp_options", d.Options,
		[]string{"id", "scope_id", "reservation_id", "code", "value", "priority", "created_at", "updated_at"},
		map[string]interface{}{"priority": "scope"}); err != nil {
		return err
	}
	return restoreRows(db, "dhcp_leases", d.Leases,
		[]string{"id", "scope_id", "ip_address", "mac_address", "hostname", "client_id", "lease_start", "lease_end", "status", "last_seen"}, nil)
}

func restoreIPAM(db execOrQuery, data json.RawMessage) error {
	var d struct {
		Spaces    []map[string]interface{} `json:"spaces"`
		Subnets   []map[string]interface{} `json:"subnets"`
		Addresses []map[string]interface{} `json:"addresses"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("parsing IPAM backup: %w", err)
	}
	for _, table := range []string{"ipam_addresses", "ipam_subnets", "ipam_spaces"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}
	if err := restoreRows(db, "ipam_spaces", d.Spaces,
		[]string{"id", "name", "description", "created_at", "updated_at"}, nil); err != nil {
		return err
	}
	if err := restoreRows(db, "ipam_subnets", d.Subnets,
		[]string{"id", "space_id", "name", "cidr", "vlan_id", "location", "description", "created_at", "updated_at"}, nil); err != nil {
		return err
	}
	return restoreRows(db, "ipam_addresses", d.Addresses,
		[]string{"id", "subnet_id", "ip_address", "status", "mac_address", "hostname", "dns_record_id", "dhcp_lease_id", "owner", "device", "location", "description", "last_seen", "created_at", "updated_at"},
		map[string]interface{}{"status": "available"})
}

func restoreSecurity(db execOrQuery, data json.RawMessage) error {
	var d struct {
		BlockLists   []map[string]interface{} `json:"block_lists"`
		BlockRules   []map[string]interface{} `json:"block_rules"`
		AllowRules   []map[string]interface{} `json:"allow_rules"`
		ClientPolicy []map[string]interface{} `json:"client_policies"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("parsing security backup: %w", err)
	}
	for _, table := range []string{"dns_block_rules", "dns_allow_rules", "dns_client_policies", "dns_block_lists"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}
	if err := restoreRows(db, "dns_block_lists", d.BlockLists,
		[]string{"id", "name", "type", "url", "enabled", "last_updated", "entry_count", "created_at", "updated_at"},
		map[string]interface{}{"type": "manual", "enabled": true, "entry_count": 0}); err != nil {
		return err
	}
	if err := restoreRows(db, "dns_block_rules", d.BlockRules,
		[]string{"id", "list_id", "pattern", "match_type", "response_type", "response_data", "enabled", "created_at"},
		map[string]interface{}{"match_type": "exact", "response_type": "NXDOMAIN", "enabled": true}); err != nil {
		return err
	}
	if err := restoreRows(db, "dns_allow_rules", d.AllowRules,
		[]string{"id", "pattern", "match_type", "enabled", "created_at"},
		map[string]interface{}{"match_type": "exact", "enabled": true}); err != nil {
		return err
	}
	return restoreRows(db, "dns_client_policies", d.ClientPolicy,
		[]string{"id", "name", "source_cidr", "action", "block_list_ids", "allow_rule_ids", "priority", "enabled", "created_at", "updated_at"},
		map[string]interface{}{"action": "filter", "priority": 0, "enabled": true})
}

func restoreConfig(db execOrQuery, data json.RawMessage) error {
	var d struct {
		Settings []map[string]interface{} `json:"settings"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("parsing config backup: %w", err)
	}
	if _, err := db.Exec(`DELETE FROM system_settings`); err != nil {
		return fmt.Errorf("clearing system_settings: %w", err)
	}
	return restoreRows(db, "system_settings", d.Settings,
		[]string{"key", "value", "description", "updated_at"}, nil)
}

func restoreRows(db execOrQuery, table string, rows []map[string]interface{}, columns []string, defaults map[string]interface{}) error {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(columns, ","), placeholders)
	for i, row := range rows {
		args := make([]interface{}, len(columns))
		for j, column := range columns {
			value, ok := row[column]
			// Older backups encoded SQL NULL as an empty string, including numbers.
			if value == "" && ((table == "dns_records" && (column == "priority" || column == "weight" || column == "port" || column == "flag")) || (table == "ipam_subnets" && column == "vlan_id") || (table == "dhcp_scopes" && column == "max_lease_time")) {
				value = nil
			}
			if !ok || value == nil || value == "" && (column == "created_at" || column == "updated_at") {
				if fallback, exists := defaults[column]; exists {
					value = fallback
				} else if column == "created_at" || column == "updated_at" {
					value = time.Now().Format(time.RFC3339)
				}
			}
			args[j] = value
		}
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("restoring %s row %d: %w", table, i, err)
		}
	}
	return nil
}
