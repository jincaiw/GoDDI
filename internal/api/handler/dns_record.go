package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/rbac"
	"github.com/jasonwa/goddi/pkg/dnsutil"
)

// --- Record Management API Handlers ---

// ListDNSRecords lists DNS records with pagination and filtering.
// GET /api/v1/dns/zones/{zoneId}/records
func ListDNSRecords(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "zoneId")
	page, pageSize := response.ParsePagination(r)

	filter := zone.RecordFilter{
		Page:     page,
		PageSize: pageSize,
		ZoneID:   zoneID,
		Name:     r.URL.Query().Get("name"),
		Type:     r.URL.Query().Get("type"),
	}

	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}

	records, total, err := getRecordManager().ListRecords(filter)
	if err != nil {
		response.InternalErrorWithLog(w, "列表查询失败", err)
		return
	}

	response.OKPaginated(w, records, total, page, pageSize)
}

// CreateDNSRecord creates a new DNS record.
// POST /api/v1/dns/zones/{zoneId}/records
func CreateDNSRecord(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "zoneId")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	var opts zone.RecordOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if opts.Name == "" {
		response.BadRequest(w, "缺少记录名称")
		return
	}
	if opts.Type == "" {
		response.BadRequest(w, "缺少记录类型")
		return
	}
	if opts.Value == "" {
		response.BadRequest(w, "缺少记录值")
		return
	}
	if err := dnsutil.ValidateRecordName(opts.Name); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	record, err := getRecordManager().CreateRecord(zoneID, opts)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.Created(w, record)
}

// GetDNSRecord gets a DNS record by ID.
// GET /api/v1/dns/zones/{zoneId}/records/{id}
func GetDNSRecord(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少记录ID")
		return
	}

	record, err := getRecordManager().GetRecord(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, record)
}

// UpdateDNSRecord updates a DNS record.
// PUT /api/v1/dns/zones/{zoneId}/records/{id}
func UpdateDNSRecord(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少记录ID")
		return
	}

	var opts zone.RecordOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if opts.Name != "" {
		if err := dnsutil.ValidateRecordName(opts.Name); err != nil {
			response.BadRequest(w, err.Error())
			return
		}
	}

	record, err := getRecordManager().UpdateRecord(id, opts)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OK(w, record)
}

// DeleteDNSRecord deletes a DNS record.
// DELETE /api/v1/dns/zones/{zoneId}/records/{id}
func DeleteDNSRecord(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少记录ID")
		return
	}

	if err := getRecordManager().DeleteRecord(id); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OK(w, map[string]string{"id": id})
}

// BatchCreateRecords creates multiple DNS records.
// POST /api/v1/dns/records/batch
func BatchCreateRecords(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		ZoneID  string               `json:"zone_id"`
		Records []zone.RecordOptions `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.ZoneID == "" {
		response.BadRequest(w, "缺少zone_id")
		return
	}
	if len(req.Records) == 0 {
		response.BadRequest(w, "缺少记录列表")
		return
	}
	if len(req.Records) > 500 {
		response.BadRequest(w, "批量操作不能超过500条")
		return
	}

	// Per-zone permission intersection (Technitium parity).
	if allowed, err := getZoneManager().ZonePermissionAllows(req.ZoneID, rbac.GetUserID(r.Context()), rbac.GetRoleIDs(r.Context()), "modify"); err == nil && !allowed {
		response.Forbidden(w, "区域权限不足")
		return
	}

	records, err := getRecordManager().BatchCreateRecords(req.ZoneID, req.Records)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.Created(w, records)
}

// BatchDeleteRecords deletes multiple DNS records.
// DELETE /api/v1/dns/records/batch
func BatchDeleteRecords(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if len(req.IDs) == 0 {
		response.BadRequest(w, "缺少ID列表")
		return
	}
	if len(req.IDs) > 500 {
		response.BadRequest(w, "批量操作不能超过500条")
		return
	}

	// Per-zone permission intersection: resolve each record's zone and
	// reject records inside zones the caller may not delete from.
	zm := getZoneManager()
	denied := 0
	for _, id := range req.IDs {
		if rec, err := getRecordManager().GetRecord(id); err == nil {
			if allowed, err := zm.ZonePermissionAllows(rec.ZoneID, rbac.GetUserID(r.Context()), rbac.GetRoleIDs(r.Context()), "delete"); err == nil && !allowed {
				denied++
			}
		}
	}
	if denied > 0 {
		response.Forbidden(w, "区域权限不足")
		return
	}

	if err := getRecordManager().BatchDeleteRecords(req.IDs); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OK(w, map[string]interface{}{"deleted": len(req.IDs)})
}
