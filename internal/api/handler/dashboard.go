package handler

import (
	"database/sql"
	"net/http"
	"sync"

	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/backup"
	"github.com/jasonwa/goddi/internal/metrics"
	"github.com/jasonwa/goddi/internal/system"
	"github.com/jasonwa/goddi/internal/task"
)

// SystemServices holds references to all system-level service components.
var SystemServices *SystemServiceContainer

// systemInitOnce ensures SystemServices is initialized exactly once.
var systemInitOnce sync.Once

// SystemServiceContainer holds references to system service components.
type SystemServiceContainer struct {
	DB          *sql.DB
	SettingsMgr *system.Manager
	BackupMgr   *backup.Manager
	TaskMgr     *task.Manager
	Version     string
}

// InitSystemServices initializes the system service container for API handlers.
func InitSystemServices(svc *SystemServiceContainer) {
	systemInitOnce.Do(func() {
		SystemServices = svc
	})
}

// --- Dashboard Handlers ---

// GetDashboard handles GET /api/v1/dashboard
func GetDashboard(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.DB == nil {
		response.InternalError(w, "系统服务未初始化")
		return
	}

	handler := metrics.DashboardDataHandler(SystemServices.DB, SystemServices.Version)
	handler(w, r)
}
