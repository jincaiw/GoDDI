package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jincaiw/GoDDI/internal/api/response"
	"github.com/jincaiw/GoDDI/internal/configrepl"
	"github.com/jincaiw/GoDDI/internal/dataplane"
)

// ConfigReplicationInfo is the deployment's ADR-0013 role for this node. It is
// passed as plain values so the handler never depends on the config package or
// exposes the replication token.
type ConfigReplicationInfo struct {
	Enabled         bool
	IsAuthor        bool
	Source          string
	IntervalSeconds int
	NodeID          string
}

// configReplicationStatus is the read-only view the console renders. A
// follower that has never applied a snapshot, has a last_error, or has not
// synced recently is reported as healthy=false: an unknown replica is never
// shown as green.
type configReplicationStatus struct {
	Enabled         bool   `json:"enabled"`
	Role            string `json:"role"` // disabled | author | follower
	Source          string `json:"source,omitempty"`
	ExpectedVersion int64  `json:"expected_version"`
	AppliedVersion  int64  `json:"applied_version"`
	AppliedRows     int64  `json:"applied_rows"`
	LastError       string `json:"last_error"`
	SyncedAt        string `json:"synced_at"`
	Healthy         bool   `json:"healthy"`
}

// ConfigReplicationStatusHandler serves GET /api/v1/cluster/config-replication.
// The author and standalone nodes report their role without touching the
// database; a follower reads its durable watermark from config_repl_state.
// Nothing here mutates state and the shared replication secret is not returned.
func ConfigReplicationStatusHandler(controlDB *sql.DB, info ConfigReplicationInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := configReplicationStatus{Enabled: info.Enabled}
		switch {
		case info.IsAuthor:
			status.Role = "author"
			// The author advertises the fingerprint of its current config so an
			// operator can compare it with a follower's applied_version. If the
			// expected version cannot be read, do not report a healthy author.
			if controlDB == nil {
				status.LastError = "期望版本不可读：控制数据库不可用"
			} else if snap, err := configrepl.ReadSnapshot(r.Context(), controlDB, info.NodeID, dataplane.ConfigTableSpecs()); err != nil {
				slog.Warn("config replication status: cannot read author version", "error", err)
				status.LastError = "期望版本不可读，作者配置状态未知"
			} else {
				status.ExpectedVersion = snap.Version
				status.Healthy = true
			}
		case !info.Enabled:
			status.Role = "disabled"
			status.Healthy = true
		default:
			status.Role = "follower"
			status.Source = info.Source
			readFollowerReplState(r, controlDB, info, &status)
		}
		response.OK(w, status)
	}
}

func readFollowerReplState(r *http.Request, controlDB *sql.DB, info ConfigReplicationInfo, status *configReplicationStatus) {
	staleSeconds := info.IntervalSeconds * 3
	if staleSeconds < 30 {
		staleSeconds = 30
	}
	if controlDB == nil {
		status.LastError = "复制状态未知：控制数据库不可用"
		return
	}
	var appliedVersion, appliedRows, expectedVersion int64
	var lastError, syncedAt sql.NullString
	err := controlDB.QueryRowContext(r.Context(), `
		SELECT applied_version, applied_rows, last_error, synced_at, expected_version
		FROM config_repl_state WHERE source_node = ?`, info.Source).
		Scan(&appliedVersion, &appliedRows, &lastError, &syncedAt, &expectedVersion)
	if err != nil {
		// Do not leak SQL text to the console; the operator only needs to know
		// that the replica state is unknown, which is a fail-closed condition.
		slog.Warn("config replication status: cannot read repl state", "error", err)
		status.LastError = "复制状态未知：尚未应用任何快照或状态不可读"
		return
	}
	status.AppliedVersion = appliedVersion
	status.AppliedRows = appliedRows
	status.ExpectedVersion = expectedVersion
	status.LastError = lastError.String
	status.SyncedAt = syncedAt.String

	healthy := lastError.String == "" && syncedAt.Valid && syncedAt.String != ""
	if healthy {
		t, parseErr := parseReplSyncedAt(syncedAt.String)
		switch {
		case parseErr != nil:
			healthy = false
			status.LastError = "复制时间戳无法解析，状态未知"
		case t.After(time.Now().Add(replClockSkewTolerance)):
			// A timestamp ahead of this node's clock is not evidence of a
			// fresh replica: it usually means clock skew, and would otherwise
			// keep a stale follower green forever.
			healthy = false
			status.LastError = "复制时间戳超前于本机时钟，状态未知"
		case time.Since(t) > time.Duration(staleSeconds)*time.Second:
			healthy = false
			status.LastError = "复制状态过期：超过 " + strconv.Itoa(staleSeconds) + " 秒未同步"
		}
	}
	if !healthy && status.LastError == "" {
		status.LastError = "复制尚未成功应用任何快照"
	}
	status.Healthy = healthy
}

// replClockSkewTolerance bounds how far a reported synced_at may be ahead of
// this node's clock before the status is treated as unknown rather than fresh.
const replClockSkewTolerance = 30 * time.Second

// parseReplSyncedAt accepts SQLite datetime('now') (UTC) and RFC3339 forms.
func parseReplSyncedAt(v string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, sql.ErrNoRows
}
