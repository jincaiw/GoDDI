package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/backup"
)

// backupNotFound reports whether err means "no such backup job", so handlers
// can answer 404 instead of 500 for unknown IDs.
func backupNotFound(err error) bool {
	return errors.Is(err, backup.ErrNotFound)
}

// --- Backup API Handlers ---

// ListBackupsHandler handles GET /api/v1/backup
func ListBackupsHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.BackupMgr == nil {
		response.InternalError(w, "备份服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := backup.BackupFilter{
		Type:     r.URL.Query().Get("type"),
		Status:   r.URL.Query().Get("status"),
		Page:     page,
		PageSize: pageSize,
	}

	jobs, total, err := SystemServices.BackupMgr.ListBackups(filter)
	if err != nil {
		response.InternalErrorWithLog(w, "查询备份列表失败", err)
		return
	}

	response.OKPaginated(w, jobs, total, page, pageSize)
}

// CreateBackupHandler handles POST /api/v1/backup
func CreateBackupHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.BackupMgr == nil {
		response.InternalError(w, "备份服务未初始化")
		return
	}

	var opts backup.BackupOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if opts.Type == "" {
		opts.Type = "full"
	}

	// Validate backup type.
	validTypes := map[string]bool{
		"full": true, "dns": true, "dhcp": true, "ipam": true, "security": true, "config": true,
	}
	if !validTypes[opts.Type] {
		response.BadRequest(w, "无效的备份类型，支持: full, dns, dhcp, ipam, security, config")
		return
	}

	job, err := SystemServices.BackupMgr.CreateBackup(opts)
	if err != nil {
		response.InternalErrorWithLog(w, "创建备份失败", err)
		return
	}

	response.Created(w, job)
}

// GetBackupHandler handles GET /api/v1/backup/{id}
func GetBackupHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.BackupMgr == nil {
		response.InternalError(w, "备份服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少备份ID")
		return
	}

	job, err := SystemServices.BackupMgr.GetBackup(id)
	if err != nil {
		if backupNotFound(err) {
			response.NotFound(w, "备份不存在")
			return
		}
		response.InternalErrorWithLog(w, "查询备份失败", err)
		return
	}

	response.OK(w, job)
}

// RestoreBackupHandler handles POST /api/v1/backup/{id}/restore
func RestoreBackupHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.BackupMgr == nil {
		response.InternalError(w, "备份服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少备份ID")
		return
	}

	if err := SystemServices.BackupMgr.RestoreBackup(id); err != nil {
		if backupNotFound(err) {
			response.NotFound(w, "备份不存在")
			return
		}
		response.InternalErrorWithLog(w, "恢复备份失败", err)
		return
	}

	if SystemServices.ReloadAfterRestore != nil {
		if err := SystemServices.ReloadAfterRestore(); err != nil {
			response.InternalError(w, "数据已恢复，但运行状态刷新失败，请重启服务")
			return
		}
	}
	response.OKWithMessage(w, "备份恢复成功；监听地址等启动配置需重启服务生效", map[string]string{"id": id})
}

// DeleteBackupHandler handles DELETE /api/v1/backup/{id}
func DeleteBackupHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.BackupMgr == nil {
		response.InternalError(w, "备份服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少备份ID")
		return
	}

	if err := SystemServices.BackupMgr.DeleteBackup(id); err != nil {
		if backupNotFound(err) {
			response.NotFound(w, "备份不存在")
			return
		}
		response.InternalErrorWithLog(w, "删除备份失败", err)
		return
	}

	response.OK(w, map[string]string{"id": id})
}

// DownloadBackupHandler handles GET /api/v1/backup/{id}/download
func DownloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.BackupMgr == nil {
		response.InternalError(w, "备份服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少备份ID")
		return
	}

	rc, fileName, err := SystemServices.BackupMgr.DownloadBackup(id)
	if err != nil {
		if backupNotFound(err) {
			response.NotFound(w, "备份不存在")
			return
		}
		response.InternalErrorWithLog(w, "下载备份失败", err)
		return
	}
	defer rc.Close()

	// Sanitize filename to prevent HTTP header injection.
	safeName := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, fileName)

	// RFC 5987 encoded filename parameter for non-ASCII safety. We always
	// emit a quoted filename for legacy clients and a UTF-8 version for
	// modern clients that understand the filename* parameter.
	disposition := "attachment; filename=" + strconv.Quote(safeName) +
		"; filename*=UTF-8''" + urlQueryEscape(safeName)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", disposition)
	if _, err := io.Copy(w, rc); err != nil {
		// Headers already sent; just log.
		_ = err
	}
}

// urlQueryEscape is a tiny wrapper around url.QueryEscape that avoids an extra
// import. It is only used for encoding the filename* parameter in
// Content-Disposition.
func urlQueryEscape(s string) string {
	// Use a minimal escape implementation to avoid pulling the net/url import
	// in this small handler file.
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9',
			c == '-' || c == '_' || c == '.' || c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
