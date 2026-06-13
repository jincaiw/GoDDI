package dhcp

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// DHCPEventType represents a DHCP event type.
type DHCPEventType string

const (
	EventDiscover DHCPEventType = "discover"
	EventOffer    DHCPEventType = "offer"
	EventRequest  DHCPEventType = "request"
	EventAck      DHCPEventType = "ack"
	EventNak      DHCPEventType = "nak"
	EventDecline  DHCPEventType = "decline"
	EventRelease  DHCPEventType = "release"
	EventInform   DHCPEventType = "inform"
	EventConflict DHCPEventType = "conflict"
)

// DHCPLogEntry represents a DHCP log entry.
type DHCPLogEntry struct {
	ID         string        `json:"id"`
	ScopeID    string        `json:"scope_id,omitempty"`
	MACAddress string        `json:"mac_address,omitempty"`
	IPAddress  string        `json:"ip_address,omitempty"`
	EventType  DHCPEventType `json:"event_type"`
	Message    string        `json:"message,omitempty"`
	CreatedAt  string        `json:"created_at"`
}

// DHCPLogFilter holds filter parameters for DHCP log queries.
type DHCPLogFilter struct {
	ScopeID    string        `json:"scope_id,omitempty"`
	EventType  DHCPEventType `json:"event_type,omitempty"`
	MACAddress string        `json:"mac_address,omitempty"`
	IPAddress  string        `json:"ip_address,omitempty"`
	StartTime  string        `json:"start_time,omitempty"`
	EndTime    string        `json:"end_time,omitempty"`
	Page       int           `json:"page,omitempty"`
	PageSize   int           `json:"page_size,omitempty"`
}

// EventLogger handles async DHCP event logging.
type EventLogger struct {
	db            *sql.DB
	entries       chan DHCPLogEntry
	quit          chan struct{}
	done          chan struct{}
	batchSize     int
	flushInterval time.Duration

	// dropped counts entries that were discarded because the channel was full.
	// It is exposed through Dropped() so operators can detect back-pressure.
	dropped uint64
}

// NewEventLogger creates a new async DHCP event logger.
func NewEventLogger(db *sql.DB) *EventLogger {
	el := &EventLogger{
		db:            db,
		entries:       make(chan DHCPLogEntry, 5000),
		quit:          make(chan struct{}),
		done:          make(chan struct{}),
		batchSize:     50,
		flushInterval: 2 * time.Second,
	}

	go el.processLoop()

	return el
}

// LogDHCPEvent queues a DHCP event for async writing.
func (el *EventLogger) LogDHCPEvent(eventType DHCPEventType, scopeID, mac, ip, message string) error {
	entry := DHCPLogEntry{
		ID:         uuid.New().String(),
		ScopeID:    scopeID,
		MACAddress: mac,
		IPAddress:  ip,
		EventType:  eventType,
		Message:    message,
	}

	select {
	case el.entries <- entry:
		return nil
	default:
		// Track drops so the operator can detect back-pressure or stalled
		// flushes. We use atomic-friendly access (the value is only ever
		// incremented) and report it via Dropped() / the periodic log line
		// emitted by processLoop.
		atomic.AddUint64(&el.dropped, 1)
		slog.Warn("dhcp_log: dropped entry (channel full)",
			"event_type", eventType,
			"mac", mac,
			"total_dropped", atomic.LoadUint64(&el.dropped),
		)
		return fmt.Errorf("log channel full, entry dropped")
	}
}

// Dropped returns the number of log entries that were dropped because the
// internal channel was full. The counter is monotonic for the lifetime of the
// EventLogger.
func (el *EventLogger) Dropped() uint64 {
	return atomic.LoadUint64(&el.dropped)
}

// processLoop runs the batch insert loop.
func (el *EventLogger) processLoop() {
	defer close(el.done)

	batch := make([]DHCPLogEntry, 0, el.batchSize)
	ticker := time.NewTicker(el.flushInterval)
	defer ticker.Stop()

	// drain reads and flushes every entry that is already sitting in the
	// channel, plus anything that arrives while we are draining. Using a
	// bounded wait avoids the previous "default-case" path that could exit
	// the loop while dozens of entries were still buffered and never
	// persisted, particularly on shutdown.
	drain := func() {
		// First, opportunistically grab whatever is buffered right now.
		for {
			select {
			case entry := <-el.entries:
				batch = append(batch, entry)
				if len(batch) >= el.batchSize {
					if el.flush(batch) {
						batch = batch[:0]
					}
				}
			default:
				if len(batch) > 0 {
					if !el.flush(batch) {
						// We keep the batch and retry on the next tick /
						// drain cycle so transient DB errors don't lose
						// entries.
						return
					}
					batch = batch[:0]
				}
				return
			}
		}
	}

	for {
		select {
		case entry := <-el.entries:
			batch = append(batch, entry)
			if len(batch) >= el.batchSize {
				if el.flush(batch) {
					batch = batch[:0]
				}
			}

		case <-ticker.C:
			if len(batch) > 0 {
				if el.flush(batch) {
					batch = batch[:0]
				}
			}

		case <-el.quit:
			// Stop the ticker-based path: we are shutting down.
			ticker.Stop()

			// Drain in two phases:
			//   1. flush whatever is in the in-memory batch,
			//   2. consume everything currently in the channel, batching
			//      along the way.
			if len(batch) > 0 {
				if el.flush(batch) {
					batch = batch[:0]
				}
			}
			drain()

			// After the first opportunistic pass, wait briefly for any
			// final producers (e.g. a deferred LogDHCPEvent) to push
			// remaining entries. The bounded wait guarantees we never
			// hang Close() forever, while still capturing tail data.
			deadline := time.Now().Add(500 * time.Millisecond)
			for time.Now().Before(deadline) {
				select {
				case entry := <-el.entries:
					batch = append(batch, entry)
					if len(batch) >= el.batchSize {
						if el.flush(batch) {
							batch = batch[:0]
						}
					}
				case <-time.After(50 * time.Millisecond):
					// No new entry; flush what we have and exit if empty.
					if len(batch) > 0 {
						if el.flush(batch) {
							batch = batch[:0]
						}
						continue
					}
					return
				}
			}
			if len(batch) > 0 {
				el.flush(batch)
			}
			return
		}
	}
}

// flush performs a batch insert of DHCP log entries.
// Returns true on success, false on failure (caller should not clear the batch on failure).
func (el *EventLogger) flush(entries []DHCPLogEntry) bool {
	if el.db == nil || len(entries) == 0 {
		return true
	}

	tx, err := el.db.Begin()
	if err != nil {
		slog.Error("dhcp_log: failed to begin transaction", "error", err)
		return false
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO dhcp_logs (id, scope_id, mac_address, ip_address, event_type, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`)
	if err != nil {
		slog.Error("dhcp_log: failed to prepare statement", "error", err)
		return false
	}
	defer stmt.Close()

	for _, e := range entries {
		_, err := stmt.Exec(e.ID, e.ScopeID, e.MACAddress, e.IPAddress, string(e.EventType), e.Message)
		if err != nil {
			slog.Error("dhcp_log: failed to insert entry", "error", err)
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("dhcp_log: failed to commit transaction", "error", err)
		return false
	}

	slog.Debug("dhcp_log: flushed entries", "count", len(entries))
	return true
}

// Close stops the event logger and flushes remaining entries.
func (el *EventLogger) Close() {
	close(el.quit)
	<-el.done
}

// QueryDHCPLogs retrieves DHCP logs from the database with filtering and pagination.
func QueryDHCPLogs(db *sql.DB, filter DHCPLogFilter) ([]DHCPLogEntry, int64, error) {
	var conditions []string
	var args []interface{}

	if filter.ScopeID != "" {
		conditions = append(conditions, "scope_id = ?")
		args = append(args, filter.ScopeID)
	}
	if filter.EventType != "" {
		conditions = append(conditions, "event_type = ?")
		args = append(args, string(filter.EventType))
	}
	if filter.MACAddress != "" {
		conditions = append(conditions, "mac_address = ?")
		args = append(args, filter.MACAddress)
	}
	if filter.IPAddress != "" {
		conditions = append(conditions, "ip_address = ?")
		args = append(args, filter.IPAddress)
	}
	if filter.StartTime != "" {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, filter.StartTime)
	}
	if filter.EndTime != "" {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, filter.EndTime)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM dhcp_logs " + whereClause
	if err := db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := filter.Page
	pageSize := filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	querySQL := `SELECT id, scope_id, mac_address, ip_address, event_type, message, created_at
		FROM dhcp_logs ` + whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []DHCPLogEntry
	for rows.Next() {
		var e DHCPLogEntry
		var scopeID, macAddress, ipAddress, message sql.NullString
		if err := rows.Scan(&e.ID, &scopeID, &macAddress, &ipAddress, &e.EventType, &message, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		e.ScopeID = scopeID.String
		e.MACAddress = macAddress.String
		e.IPAddress = ipAddress.String
		e.Message = message.String
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating DHCP log entries: %w", err)
	}

	return entries, total, nil
}
