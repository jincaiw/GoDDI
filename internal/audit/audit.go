package audit

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id,omitempty"`
	Username     string    `json:"username,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	SourceIP     string    `json:"source_ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	Success      bool      `json:"success"`
	CreatedAt    time.Time `json:"created_at"`
}

// LogEntry is used to create a new audit log entry.
type LogEntry struct {
	UserID       string
	Username     string
	Action       string
	ResourceType string
	ResourceID   string
	Detail       string
	SourceIP     string
	UserAgent    string
	Success      bool
}

// AuditFilter is used to filter audit logs.
type AuditFilter struct {
	UserID       string
	ResourceType string
	Action       string
	StartTime    *time.Time
	EndTime      *time.Time
	Page         int
	PageSize     int
}

// AuditManager manages audit logging.
type AuditManager struct {
	db *sql.DB
}

// NewAuditManager creates a new AuditManager.
func NewAuditManager(db *sql.DB) *AuditManager {
	return &AuditManager{db: db}
}

// Log creates a new audit log entry.
func (am *AuditManager) Log(entry LogEntry) error {
	id := uuid.New().String()
	_, err := am.db.Exec(`
		INSERT INTO audit_logs (id, user_id, username, action, resource_type, resource_id,
			detail, source_ip, user_agent, success, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, entry.UserID, entry.Username, entry.Action, entry.ResourceType,
		entry.ResourceID, entry.Detail, entry.SourceIP, entry.UserAgent,
		entry.Success, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting audit log: %w", err)
	}
	return nil
}

// QueryLogs queries audit logs with filtering and pagination.
func (am *AuditManager) QueryLogs(filter AuditFilter) ([]AuditLog, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	if filter.Page > 1000 {
		filter.Page = 1000
	}

	// Build the WHERE clause
	where := "WHERE 1=1"
	args := []interface{}{}

	if filter.UserID != "" {
		where += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	if filter.ResourceType != "" {
		where += " AND resource_type = ?"
		args = append(args, filter.ResourceType)
	}
	if filter.Action != "" {
		where += " AND action = ?"
		args = append(args, filter.Action)
	}
	if filter.StartTime != nil {
		where += " AND created_at >= ?"
		args = append(args, filter.StartTime.Format(time.RFC3339))
	}
	if filter.EndTime != nil {
		where += " AND created_at <= ?"
		args = append(args, filter.EndTime.Format(time.RFC3339))
	}

	// Count total
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", where)
	err := am.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting audit logs: %w", err)
	}

	// Query with pagination
	offset := (filter.Page - 1) * filter.PageSize
	query := fmt.Sprintf(`
		SELECT id, user_id, username, action, resource_type, resource_id,
			detail, source_ip, user_agent, success, created_at
		FROM audit_logs %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, where)

	queryArgs := append(args, filter.PageSize, offset)
	rows, err := am.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		var createdAt string
		var userID, username, resourceID, detail, sourceIP, userAgent sql.NullString

		if err := rows.Scan(&l.ID, &userID, &username, &l.Action, &l.ResourceType,
			&resourceID, &detail, &sourceIP, &userAgent, &l.Success, &createdAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning audit log: %w", err)
		}

		if userID.Valid {
			l.UserID = userID.String
		}
		if username.Valid {
			l.Username = username.String
		}
		if resourceID.Valid {
			l.ResourceID = resourceID.String
		}
		if detail.Valid {
			l.Detail = detail.String
		}
		if sourceIP.Valid {
			l.SourceIP = sourceIP.String
		}
		if userAgent.Valid {
			l.UserAgent = userAgent.String
		}
		createdAtTime, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			// Schema guarantees RFC3339, but a corrupted row should not
			// silently disappear from the result set. Log and zero the
			// timestamp so the operator can spot the bad row in the DB.
			slog.Warn("audit: invalid created_at format, using zero time",
				"row_id", l.ID, "raw", createdAt, "error", err)
		}
		l.CreatedAt = createdAtTime

		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating audit logs: %w", err)
	}

	return logs, total, nil
}

// LogLogin records a login attempt in the login_history table.
func (am *AuditManager) LogLogin(userID, username, authType, sourceIP, userAgent string, success bool, reason string) error {
	id := uuid.New().String()
	var nullableUserID interface{}
	if userID != "" {
		nullableUserID = userID
	}
	_, err := am.db.Exec(`
		INSERT INTO login_history (id, user_id, username, auth_type, source_ip, user_agent, success, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, nullableUserID, username, authType, sourceIP, userAgent, success, reason,
		time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting login history: %w", err)
	}
	return nil
}

// QueryLoginHistory queries login history with pagination.
func (am *AuditManager) QueryLoginHistory(userID string, page, pageSize int) ([]map[string]interface{}, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	// Cap the page number itself so a malicious caller cannot trigger an
	// extremely large OFFSET (which on SQLite is O(n) and may DoS the DB).
	if page > 1000 {
		page = 1000
	}

	where := "WHERE 1=1"
	args := []interface{}{}

	if userID != "" {
		where += " AND user_id = ?"
		args = append(args, userID)
	}

	// Count total
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM login_history %s", where)
	err := am.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting login history: %w", err)
	}

	// Query with pagination
	offset := (page - 1) * pageSize
	query := fmt.Sprintf(`
		SELECT id, user_id, username, auth_type, source_ip, user_agent, success, reason, created_at
		FROM login_history %s ORDER BY created_at DESC LIMIT ? OFFSET ?`, where)

	queryArgs := append(args, pageSize, offset)
	rows, err := am.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying login history: %w", err)
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var id, username, authType, sourceIP, userAgent, reason, createdAt string
		var userID sql.NullString
		var success bool

		if err := rows.Scan(&id, &userID, &username, &authType, &sourceIP, &userAgent, &success, &reason, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scanning login history: %w", err)
		}

		entry := map[string]interface{}{
			"id":         id,
			"username":   username,
			"auth_type":  authType,
			"source_ip":  sourceIP,
			"user_agent": userAgent,
			"success":    success,
			"reason":     reason,
			"created_at": createdAt,
		}
		if userID.Valid {
			entry["user_id"] = userID.String
		}

		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating login history: %w", err)
	}

	return history, total, nil
}
