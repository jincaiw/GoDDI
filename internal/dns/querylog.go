package dns

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// QueryLogEntry represents a DNS query log entry.
type QueryLogEntry struct {
	ID             string  `json:"id"`
	ClientIP       string  `json:"client_ip"`
	ClientPort     int     `json:"client_port"`
	Protocol       string  `json:"protocol"`
	QueryName      string  `json:"query_name"`
	QueryType      string  `json:"query_type"`
	ResponseCode   string  `json:"response_code"`
	ResponseTimeMs float64 `json:"response_time_ms"`
	Upstream       string  `json:"upstream"`
	Cached         bool    `json:"cached"`
	Blocked        bool    `json:"blocked"`
	CreatedAt      string  `json:"created_at"`
}

// QueryLogger handles async DNS query logging.
type QueryLogger struct {
	db      *sql.DB
	entries chan QueryLogEntry
	quit    chan struct{}
	done    chan struct{}

	bufferSize    int
	batchSize     int
	flushInterval time.Duration
	retentionDays int

	droppedCount atomic.Int64
}

// NewQueryLogger creates a new async query logger.
func NewQueryLogger(db *sql.DB, retentionDays int) *QueryLogger {
	ql := &QueryLogger{
		db:            db,
		entries:       make(chan QueryLogEntry, 10000),
		quit:          make(chan struct{}),
		done:          make(chan struct{}),
		bufferSize:    10000,
		batchSize:     100,
		flushInterval: 1 * time.Second,
		retentionDays: retentionDays,
	}

	go ql.processLoop()

	return ql
}

// Log queues a query log entry for async writing.
// Non-blocking: if the channel is full, the entry is dropped.
func (ql *QueryLogger) Log(entry QueryLogEntry) {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	select {
	case ql.entries <- entry:
		// Entry queued successfully.
	default:
		// Channel full, drop the entry. Never block DNS.
		ql.droppedCount.Add(1)
		slog.Debug("querylog: dropped entry (channel full)", "query_name", entry.QueryName)
	}
}

// processLoop runs the batch insert loop.
func (ql *QueryLogger) processLoop() {
	defer close(ql.done)

	batch := make([]QueryLogEntry, 0, ql.batchSize)
	ticker := time.NewTicker(ql.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry := <-ql.entries:
			batch = append(batch, entry)
			if len(batch) >= ql.batchSize {
				ql.flush(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				ql.flush(batch)
				batch = batch[:0]
			}
			// Periodic cleanup of old entries.
			ql.cleanup()

		case <-ql.quit:
			// Drain remaining entries.
			if len(batch) > 0 {
				ql.flush(batch)
			}
			// Drain channel.
			for {
				select {
				case entry := <-ql.entries:
					batch = append(batch, entry)
					if len(batch) >= ql.batchSize {
						ql.flush(batch)
						batch = batch[:0]
					}
				default:
					if len(batch) > 0 {
						ql.flush(batch)
					}
					return
				}
			}
		}
	}
}

// flush performs a batch insert of query log entries.
func (ql *QueryLogger) flush(entries []QueryLogEntry) {
	if ql.db == nil || len(entries) == 0 {
		return
	}

	tx, err := ql.db.Begin()
	if err != nil {
		slog.Error("querylog: failed to begin transaction", "error", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO dns_query_logs
			(id, client_ip, client_port, protocol, query_name, query_type,
			 response_code, response_time_ms, upstream, cached, blocked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		slog.Error("querylog: failed to prepare statement", "error", err)
		return
	}
	defer stmt.Close()

	for _, e := range entries {
		_, err := stmt.Exec(
			e.ID, e.ClientIP, e.ClientPort, e.Protocol,
			e.QueryName, e.QueryType, e.ResponseCode,
			e.ResponseTimeMs, e.Upstream, e.Cached, e.Blocked,
		)
		if err != nil {
			slog.Error("querylog: failed to insert entry", "error", err)
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("querylog: failed to commit transaction", "error", err)
		return
	}

	slog.Debug("querylog: flushed entries", "count", len(entries))
}

// cleanup removes old query log entries based on retention policy.
func (ql *QueryLogger) cleanup() {
	if ql.db == nil || ql.retentionDays <= 0 {
		return
	}

	result, err := ql.db.Exec(
		"DELETE FROM dns_query_logs WHERE created_at < datetime('now', ?)",
		fmt.Sprintf("-%d days", ql.retentionDays),
	)
	if err != nil {
		slog.Error("querylog: failed to cleanup old entries", "error", err)
		return
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("querylog: cleaned up old entries", "deleted", rows)
	}
}

// Close stops the query logger and flushes remaining entries.
func (ql *QueryLogger) Close() {
	close(ql.quit)
	<-ql.done // Wait for processLoop to finish.
}

// DroppedCount returns the total number of dropped log entries.
func (ql *QueryLogger) DroppedCount() int64 {
	return ql.droppedCount.Load()
}

// QueryLogs retrieves query logs from the database with filtering and pagination.
func QueryLogs(db *sql.DB, filters QueryLogFilters, page, pageSize int) ([]QueryLogEntry, int64, error) {
	var conditions []string
	var args []interface{}

	if filters.ClientIP != "" {
		conditions = append(conditions, "client_ip = ?")
		args = append(args, filters.ClientIP)
	}
	if filters.Domain != "" {
		conditions = append(conditions, "query_name LIKE ?")
		args = append(args, "%"+filters.Domain+"%")
	}
	if filters.QueryType != "" {
		conditions = append(conditions, "query_type = ?")
		args = append(args, strings.ToUpper(filters.QueryType))
	}
	if filters.ResponseCode != "" {
		conditions = append(conditions, "response_code = ?")
		args = append(args, filters.ResponseCode)
	}
	if filters.Blocked != nil {
		conditions = append(conditions, "blocked = ?")
		args = append(args, *filters.Blocked)
	}
	if filters.StartTime != "" {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, filters.StartTime)
	}
	if filters.EndTime != "" {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, filters.EndTime)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total.
	var total int64
	countSQL := "SELECT COUNT(*) FROM dns_query_logs " + whereClause
	err := db.QueryRow(countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch page.
	offset := (page - 1) * pageSize
	querySQL := "SELECT id, client_ip, client_port, protocol, query_name, query_type, " +
		"response_code, response_time_ms, upstream, cached, blocked, created_at " +
		"FROM dns_query_logs " + whereClause +
		" ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []QueryLogEntry
	for rows.Next() {
		var e QueryLogEntry
		if err := rows.Scan(
			&e.ID, &e.ClientIP, &e.ClientPort, &e.Protocol,
			&e.QueryName, &e.QueryType, &e.ResponseCode,
			&e.ResponseTimeMs, &e.Upstream, &e.Cached, &e.Blocked, &e.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating query log entries: %w", err)
	}

	return entries, total, nil
}

// QueryLogFilters holds filter parameters for query log queries.
type QueryLogFilters struct {
	ClientIP     string `json:"client_ip,omitempty"`
	Domain       string `json:"domain,omitempty"`
	QueryType    string `json:"query_type,omitempty"`
	ResponseCode string `json:"response_code,omitempty"`
	Blocked      *bool  `json:"blocked,omitempty"`
	StartTime    string `json:"start_time,omitempty"`
	EndTime      string `json:"end_time,omitempty"`
}
