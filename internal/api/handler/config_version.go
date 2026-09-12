package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/configver"
	"github.com/jasonwa/goddi/internal/rbac"
)

// ConfigVersionService is the process-wide publishing service. It is set once
// at startup; a nil value means the API is running without configuration
// publishing wired up and every endpoint reports that plainly rather than
// panicking.
var ConfigVersionService *configver.Service

var configVersionOnce sync.Once

// InitConfigVersionService installs the publishing service for the API layer.
func InitConfigVersionService(svc *configver.Service) {
	configVersionOnce.Do(func() { ConfigVersionService = svc })
}

// requireConfigService resolves the service or writes an error response.
func requireConfigService(w http.ResponseWriter) *configver.Service {
	if ConfigVersionService == nil {
		response.ServiceUnavailable(w, "配置发布服务未初始化", nil)
		return nil
	}
	return ConfigVersionService
}

// publishConfigRequest is the body of a publish call.
//
// content is a raw object rather than a typed struct: the shape depends on
// resource_type, and the adapter is the only thing that knows it. Keeping it
// raw also preserves the caller's byte-for-byte intent for the revision
// snapshot (the service canonicalises it).
type publishConfigRequest struct {
	ResourceType     string          `json:"resource_type"`
	ResourceID       string          `json:"resource_id"`
	Content          json.RawMessage `json:"content"`
	ExpectedRevision int64           `json:"expected_revision"`
	IdempotencyKey   string          `json:"idempotency_key,omitempty"`
	Note             string          `json:"note,omitempty"`
	DryRun           bool            `json:"dry_run,omitempty"`
}

type rollbackConfigRequest struct {
	ResourceType     string `json:"resource_type"`
	ResourceID       string `json:"resource_id"`
	ToRevision       int64  `json:"to_revision"`
	ExpectedRevision int64  `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
	Note             string `json:"note,omitempty"`
}

// writeConfigError maps the service's sentinel errors onto HTTP status codes.
//
// Getting this wrong is not cosmetic: a lost concurrency race reported as 400
// tells a client to fix its content, when the correct response is to reload and
// re-apply. A 500 would be worse still -- it hides a normal, expected outcome
// inside "the server is broken".
func writeConfigError(w http.ResponseWriter, err error) {
	var conflict *configver.ConflictError
	var validation *configver.ValidationError

	switch {
	case errors.As(err, &conflict):
		response.Conflict(w, conflict.Error())
	case errors.As(err, &validation):
		response.BadRequest(w, validation.Error())
	case errors.Is(err, configver.ErrResourceNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, configver.ErrNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, configver.ErrNoAdapter), errors.Is(err, configver.ErrInvalidRevision):
		response.BadRequest(w, err.Error())
	default:
		response.InternalErrorWithLog(w, "配置发布失败", err)
	}
}

// ListConfigTypes reports which resource types this instance can publish.
func ListConfigTypes(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}
	response.OK(w, map[string]any{"resource_types": svc.RegisteredTypes()})
}

// ListConfigRevisions lists revisions, newest first.
func ListConfigRevisions(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}

	page, pageSize := response.ParsePagination(r)
	filter := configver.ListFilter{
		ResourceType: configver.ResourceType(r.URL.Query().Get("resource_type")),
		ResourceID:   r.URL.Query().Get("resource_id"),
		Status:       configver.Status(r.URL.Query().Get("status")),
		Page:         page,
		PageSize:     pageSize,
	}

	revisions, total, err := svc.List(filter)
	if err != nil {
		response.InternalErrorWithLog(w, "查询配置版本失败", err)
		return
	}
	if revisions == nil {
		revisions = []configver.Revision{}
	}
	response.OKPaginated(w, revisions, total, page, pageSize)
}

// GetConfigRevision returns one revision, optionally with the diff against the
// revision it was based on.
func GetConfigRevision(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}

	rev, err := svc.Get(chi.URLParam(r, "id"))
	if err != nil {
		writeConfigError(w, err)
		return
	}

	out := map[string]any{"revision": rev}
	if r.URL.Query().Get("with_changes") == "true" {
		changes, err := svc.DiffRevisions(rev.ResourceType, rev.ResourceID, rev.BaseRevision, rev.Revision)
		switch {
		case err == nil:
			out["changes"] = changes
		default:
			response.InternalErrorWithLog(w, "计算差异失败", err)
			return
		}
	}
	response.OK(w, out)
}

// DiffConfigRevisions compares two revisions of one resource.
func DiffConfigRevisions(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}

	rt := configver.ResourceType(r.URL.Query().Get("resource_type"))
	id := r.URL.Query().Get("resource_id")
	from, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	if err != nil {
		response.BadRequest(w, "缺少或无效的 from 版本号")
		return
	}
	to, err := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err != nil {
		response.BadRequest(w, "缺少或无效的 to 版本号")
		return
	}
	if id == "" {
		response.BadRequest(w, "缺少资源 ID")
		return
	}

	changes, err := svc.DiffRevisions(rt, id, from, to)
	if err != nil {
		writeConfigError(w, err)
		return
	}
	response.OK(w, map[string]any{"from": from, "to": to, "changes": changes})
}

// PublishConfigRevision validates and publishes a configuration change.
//
// The endpoints that mutate DNS/DHCP/IPAM resources still exist and still work;
// this is the governed path that records a revision, checks for concurrent
// modification and queues the data-plane release. Callers that need an audit
// trail and a rollback path should use this one.
func PublishConfigRevision(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}

	var req publishConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.ResourceType == "" {
		response.BadRequest(w, "缺少 resource_type")
		return
	}
	if req.ResourceID == "" {
		response.BadRequest(w, "缺少 resource_id")
		return
	}

	result, err := svc.Publish(configver.PublishRequest{
		ResourceType:     configver.ResourceType(req.ResourceType),
		ResourceID:       req.ResourceID,
		Content:          req.Content,
		ExpectedRevision: req.ExpectedRevision,
		IdempotencyKey:   req.IdempotencyKey,
		Actor:            rbac.GetUsername(r.Context()),
		Note:             req.Note,
		DryRun:           req.DryRun,
	})
	if err != nil {
		writeConfigError(w, err)
		return
	}

	if result.DryRun {
		response.OK(w, result)
		return
	}
	response.Created(w, result)
}

// RollbackConfigRevision republishes the content of an earlier revision.
func RollbackConfigRevision(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}

	var req rollbackConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.ResourceType == "" || req.ResourceID == "" {
		response.BadRequest(w, "缺少 resource_type 或 resource_id")
		return
	}

	result, err := svc.Rollback(configver.RollbackRequest{
		ResourceType:     configver.ResourceType(req.ResourceType),
		ResourceID:       req.ResourceID,
		ToRevision:       req.ToRevision,
		ExpectedRevision: req.ExpectedRevision,
		IdempotencyKey:   req.IdempotencyKey,
		Actor:            rbac.GetUsername(r.Context()),
		Note:             req.Note,
	})
	if err != nil {
		writeConfigError(w, err)
		return
	}
	response.Created(w, result)
}

// ListConfigReleases lists queued data-plane releases.
//
// This is the endpoint that answers "is a configuration stuck?". A failed row
// here means the configuration is recorded but the data plane never confirmed
// it, which is exactly the state that used to be invisible.
func ListConfigReleases(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}

	releases, err := svc.ListReleases(r.URL.Query().Get("status"), limit)
	if err != nil {
		response.InternalErrorWithLog(w, "查询发布队列失败", err)
		return
	}
	if releases == nil {
		releases = []configver.Release{}
	}

	pending, failed, err := svc.OutboxStats()
	if err != nil {
		response.InternalErrorWithLog(w, "查询发布队列统计失败", err)
		return
	}

	response.OK(w, map[string]any{
		"releases": releases,
		"stats":    map[string]int64{"pending": pending, "failed": failed},
	})
}

// RetryConfigReleases resets failed releases so the worker picks them up again.
func RetryConfigReleases(w http.ResponseWriter, r *http.Request) {
	svc := requireConfigService(w)
	if svc == nil {
		return
	}

	var req struct {
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id"`
	}
	if r.Body != nil {
		// An empty body means "retry everything", so a decode failure on an
		// empty body is not an error worth rejecting.
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	n, err := svc.RetryFailed(configver.ResourceType(req.ResourceType), req.ResourceID)
	if err != nil {
		response.InternalErrorWithLog(w, "重试发布失败", err)
		return
	}
	response.OK(w, map[string]any{"reset": n})
}
