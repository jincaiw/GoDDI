package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/task"
)

// --- Task API Handlers ---

// ListTasksHandler handles GET /api/v1/tasks
func ListTasksHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.TaskMgr == nil {
		response.InternalError(w, "任务服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := task.TaskFilter{
		Status:   r.URL.Query().Get("status"),
		Page:     page,
		PageSize: pageSize,
	}

	tasks, total, err := SystemServices.TaskMgr.ListTasks(filter)
	if err != nil {
		response.InternalErrorWithLog(w, "查询任务列表失败", err)
		return
	}

	response.OKPaginated(w, tasks, total, page, pageSize)
}

// GetTaskHandler handles GET /api/v1/tasks/{id}
func GetTaskHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.TaskMgr == nil {
		response.InternalError(w, "任务服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少任务ID")
		return
	}

	status, err := SystemServices.TaskMgr.GetTaskStatus(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, status)
}

// CancelTaskHandler handles POST /api/v1/tasks/{id}/cancel
func CancelTaskHandler(w http.ResponseWriter, r *http.Request) {
	if SystemServices == nil || SystemServices.TaskMgr == nil {
		response.InternalError(w, "任务服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少任务ID")
		return
	}

	if err := SystemServices.TaskMgr.CancelTask(id); err != nil {
		response.BadRequest(w, "取消任务失败: "+err.Error())
		return
	}

	response.OKWithMessage(w, "任务已取消", map[string]string{"id": id})
}
