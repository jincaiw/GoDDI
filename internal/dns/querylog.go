package dns

import (
	"context"
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

	droppedCount    atomic.Int64
	writtenCount    atomic.Int64
	beginFailures   atomic.Int64
	prepareFailures atomic.Int64
	execFailures    atomic.Int64
	commitFailures  atomic.Int64
	cleanupFailures atomic.Int64
}

// QueryLogStats is a point-in-time health snapshot for the asynchronous
// query-log pipeline. It distinguishes loss at the non-blocking ingress from
// failures in the SQLite writer so operators do not mistake partial logging
// for a healthy telemetry stream.
type QueryLogStats struct {
	QueueDepth      int
	QueueCapacity   int
	DroppedFull     int64
	Written         int64
	BeginFailures   int64
	PrepareFailures int64
	ExecFailures    int64
	CommitFailures  int64
	CleanupFailures int64
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

	// Retention cleanup runs a full-table DELETE, which is far too expensive
	// to repeat on every flush tick (especially with a single-writer SQLite
	// pool). Run it on its own hourly cadence instead.
	cleanupTicker := time.NewTicker(time.Hour)
	defer cleanupTicker.Stop()
	ql.cleanup()

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

		case <-cleanupTicker.C:
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
	if len(entries) == 0 {
		return
	}
	if ql.db == nil {
		ql.beginFailures.Add(int64(len(entries)))
		return
	}

	tx, err := ql.db.Begin()
	if err != nil {
		ql.beginFailures.Add(int64(len(entries)))
		slog.Error("querylog: failed to begin transaction", "error", err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT INTO dns_query_logs
			(id, client_ip, client_port, protocol, query_name, query_type,
			 response_code, response_time_ms, upstream, cached, blocked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		ql.prepareFailures.Add(int64(len(entries)))
		slog.Error("querylog: failed to prepare statement", "error", err)
		return
	}
	defer stmt.Close()

	written := int64(0)
	for _, e := range entries {
		_, err := stmt.Exec(
			e.ID, e.ClientIP, e.ClientPort, e.Protocol,
			e.QueryName, e.QueryType, e.ResponseCode,
			e.ResponseTimeMs, e.Upstream, e.Cached, e.Blocked,
		)
		if err != nil {
			ql.execFailures.Add(1)
			slog.Error("querylog: failed to insert entry", "error", err)
			continue
		}
		written++
	}

	if err := tx.Commit(); err != nil {
		// SQLite rolls back the entire transaction on a failed commit, so none
		// of the previously successful Exec calls are durable.
		ql.commitFailures.Add(int64(len(entries)))
		slog.Error("querylog: failed to commit transaction", "error", err)
		return
	}
	committed = true
	ql.writtenCount.Add(written)

	slog.Debug("querylog: flushed entries", "count", written)
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
		ql.cleanupFailures.Add(1)
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

// Stats returns a lock-free snapshot of queue pressure and SQLite writer
// outcomes. QueueDepth is intentionally sampled rather than synchronously
// emitted from Log so the DNS query path stays non-blocking.
func (ql *QueryLogger) Stats() QueryLogStats {
	return QueryLogStats{
		QueueDepth:      len(ql.entries),
		QueueCapacity:   cap(ql.entries),
		DroppedFull:     ql.droppedCount.Load(),
		Written:         ql.writtenCount.Load(),
		BeginFailures:   ql.beginFailures.Load(),
		PrepareFailures: ql.prepareFailures.Load(),
		ExecFailures:    ql.execFailures.Load(),
		CommitFailures:  ql.commitFailures.Load(),
		CleanupFailures: ql.cleanupFailures.Load(),
	}
}

// QueryLogs retrieves query logs from the database with filtering and pagination.
// It retains the legacy background context for non-HTTP callers; request
// handlers should use QueryLogsContext so a disconnected client releases the
// shared SQLite connection promptly.
func QueryLogs(db *sql.DB, filters QueryLogFilters, page, pageSize int) ([]QueryLogEntry, int64, error) {
	return QueryLogsContext(context.Background(), db, filters, page, pageSize)
}

// QueryLogsContext retrieves one paginated log page and its total under ctx.
func QueryLogsContext(ctx context.Context, db *sql.DB, filters QueryLogFilters, page, pageSize int) ([]QueryLogEntry, int64, error) {
	whereClause, args := queryLogWhere(filters)

	var total int64
	countSQL := "SELECT COUNT(*) FROM dns_query_logs " + whereClause
	if err := db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	entries, err := queryLogRows(ctx, db, whereClause, args, pageSize, (page-1)*pageSize)
	return entries, total, err
}

// StreamQueryLogsContext writes matching entries incrementally to consume.
// Unlike paginated listings it intentionally skips COUNT(*) and does not
// accumulate rows, making large CSV exports bounded by the database cursor and
// writer buffering rather than the result set size.
func StreamQueryLogsContext(ctx context.Context, db *sql.DB, filters QueryLogFilters, limit int, consume func(QueryLogEntry) error) error {
	if limit <= 0 {
		return nil
	}
	whereClause, args := queryLogWhere(filters)
	_, err := queryLogRows(ctx, db, whereClause, args, limit, 0, consume)
	return err
}

func queryLogWhere(filters QueryLogFilters) (string, []interface{}) {
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
	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func queryLogRows(ctx context.Context, db *sql.DB, whereClause string, args []interface{}, limit, offset int, consume ...func(QueryLogEntry) error) ([]QueryLogEntry, error) {
	querySQL := "SELECT id, client_ip, client_port, protocol, query_name, query_type, " +
		"response_code, response_time_ms, upstream, cached, blocked, created_at " +
		"FROM dns_query_logs " + whereClause +
		" ORDER BY created_at DESC LIMIT ? OFFSET ?"
	queryArgs := append(append([]interface{}(nil), args...), limit, offset)
	rows, err := db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []QueryLogEntry
	consumer := (func(QueryLogEntry) error)(nil)
	if len(consume) > 0 {
		consumer = consume[0]
	} else {
		entries = make([]QueryLogEntry, 0)
	}
	for rows.Next() {
		var e QueryLogEntry
		if err := rows.Scan(&e.ID, &e.ClientIP, &e.ClientPort, &e.Protocol, &e.QueryName, &e.QueryType, &e.ResponseCode, &e.ResponseTimeMs, &e.Upstream, &e.Cached, &e.Blocked, &e.CreatedAt); err != nil {
			return nil, err
		}
		if consumer != nil {
			if err := consumer(e); err != nil {
				return nil, err
			}
			continue
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating query log entries: %w", err)
	}
	return entries, nil
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
