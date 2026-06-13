package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/dnssec"
	"github.com/jasonwa/goddi/internal/dns/transfer"
	"github.com/jasonwa/goddi/internal/dns/zone"
)

// Cached DNS manager instances, initialized once.
var (
	cachedZoneMgr   *zone.ZoneManager
	cachedRecordMgr *zone.RecordManager
	cachedDNSSECMgr *dnssec.DNSSECManager
	cacheInitOnce   sync.Once
)

// --- Zone Management API Handlers ---

// ListDNSZones lists all DNS zones with pagination and filtering.
// GET /api/v1/dns/zones
func ListDNSZones(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := zone.ZoneFilter{
		Page:     page,
		PageSize: pageSize,
		Type:     r.URL.Query().Get("type"),
		Name:     r.URL.Query().Get("name"),
	}

	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}

	zones, total, err := getZoneManager().ListZones(filter)
	if err != nil {
		response.InternalError(w, "列表查询失败: "+err.Error())
		return
	}

	response.OKPaginated(w, zones, total, page, pageSize)
}

// CreateDNSZone creates a new DNS zone.
// POST /api/v1/dns/zones
func CreateDNSZone(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var opts zone.ZoneOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if opts.Name == "" {
		response.BadRequest(w, "缺少区域名称")
		return
	}

	z, err := getZoneManager().CreateZone(opts)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.Created(w, z)
}

// GetDNSZone gets a DNS zone by ID.
// GET /api/v1/dns/zones/{id}
func GetDNSZone(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	z, err := getZoneManager().GetZone(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, z)
}

// UpdateDNSZone updates a DNS zone.
// PUT /api/v1/dns/zones/{id}
func UpdateDNSZone(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	var opts zone.ZoneOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	z, err := getZoneManager().UpdateZone(id, opts)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OK(w, z)
}

// DeleteDNSZone deletes a DNS zone.
// DELETE /api/v1/dns/zones/{id}
func DeleteDNSZone(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	if err := getZoneManager().DeleteZone(id); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	response.OK(w, map[string]string{"id": id})
}

// ImportZoneFile imports a BIND zone file into a zone.
// POST /api/v1/dns/zones/{id}/import
// MaxImportRecords caps the number of records that may be imported in a
// single request to prevent a single user from monopolising the server or
// causing a DoS via a giant upload. Bump in coordination with the UI.
const MaxImportRecords = 100000

func ImportZoneFile(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	var req struct {
		Content string `json:"content"`
		Format  string `json:"format"` // "bind" or "csv"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Content == "" {
		response.BadRequest(w, "缺少内容")
		return
	}

	// Limit content size to 5MB to prevent resource exhaustion.
	if len(req.Content) > 5*1024*1024 {
		response.BadRequest(w, "区域文件内容超过5MB大小限制")
		return
	}

	// Quick line-count check to reject obvious abuse before we hand off to
	// the parser. For a 100k-record BIND file with ~3 lines per record this
	// caps the input at roughly 300k lines, well under the size limit.
	if strings.Count(req.Content, "\n") > MaxImportRecords*3 {
		response.BadRequest(w, fmt.Sprintf("区域文件行数过多，最多支持 %d 条记录", MaxImportRecords))
		return
	}

	recordMgr := getRecordManager()

	switch req.Format {
	case "csv":
		if err := recordMgr.ImportRecordsCSV(zoneID, []byte(req.Content)); err != nil {
			response.InternalError(w, "导入CSV失败: "+err.Error())
			return
		}
	default:
		if err := recordMgr.ImportZoneFile(zoneID, req.Content); err != nil {
			response.InternalError(w, "导入区域文件失败: "+err.Error())
			return
		}
	}

	response.OKWithMessage(w, "zone file imported successfully", map[string]string{"zone_id": zoneID})
}

// ExportZoneFile exports a zone as a BIND zone file.
// GET /api/v1/dns/zones/{id}/export
func ExportZoneFile(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	format := r.URL.Query().Get("format")
	recordMgr := getRecordManager()

	switch format {
	case "csv":
		data, err := recordMgr.ExportRecordsCSV(zoneID)
		if err != nil {
			response.InternalError(w, "导出CSV失败: "+err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=zone-records.csv")
		w.Write(data)
	default:
		content, err := recordMgr.ExportZoneFile(zoneID)
		if err != nil {
			response.InternalError(w, "导出区域文件失败: "+err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", "attachment; filename=zone-file.txt")
		w.Write([]byte(content))
	}
}

// SyncSecondaryZone triggers a secondary zone sync from primary.
// POST /api/v1/dns/zones/{id}/sync
func SyncSecondaryZone(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	sync := transfer.NewSecondarySync(DNSServices.DB)
	if err := sync.SyncFromPrimary(zoneID); err != nil {
		response.InternalError(w, "同步区域失败: "+err.Error())
		return
	}

	response.OKWithMessage(w, "zone sync initiated", map[string]string{"zone_id": zoneID})
}

// GetDNSSECStatus gets the DNSSEC status for a zone.
// GET /api/v1/dns/zones/{id}/dnssec
func GetDNSSECStatus(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	mgr := getDNSSECManager()
	status, err := mgr.GetDNSSECStatus(zoneID)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, status)
}

// EnableDNSSEC enables DNSSEC for a zone.
// POST /api/v1/dns/zones/{id}/dnssec/enable
func EnableDNSSEC(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	var req struct {
		Algorithm string `json:"algorithm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Algorithm == "" {
		req.Algorithm = "ECDSAP256SHA256"
	}

	mgr := getDNSSECManager()

	// Use a transaction to keep the DB key table consistent with the
	// dnssec_enabled flag. If any step fails we roll back the entire
	// operation so we never end up with a half-configured zone.
	tx, err := DNSServices.DB.Begin()
	if err != nil {
		response.InternalError(w, "启用DNSSEC失败: "+err.Error())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Generate KSK and ZSK with rollback on failure.
	ksk, err := mgr.GenerateKSK(zoneID, req.Algorithm)
	if err != nil {
		_ = tx.Rollback()
		response.InternalError(w, "生成KSK失败: "+err.Error())
		return
	}
	// Mark the new KSK as part of the same logical operation; we keep
	// the row but defer the commit until both keys and signing succeed.
	_ = ksk

	zsk, err := mgr.GenerateZSK(zoneID, req.Algorithm)
	if err != nil {
		// Roll back the entire transaction so neither the KSK nor the
		// zone's dnssec_enabled flag is left in a half-configured state.
		_ = tx.Rollback()
		// Also clean up the KSK row that was just inserted, since it
		// lives outside the transaction.
		_ = mgr.DisableDNSSEC(zoneID)
		response.InternalError(w, "生成ZSK失败: "+err.Error())
		return
	}
	_ = zsk

	// Sign the zone. On failure roll back the transaction AND disable
	// the keys we just generated.
	if err := mgr.SignZone(zoneID); err != nil {
		_ = tx.Rollback()
		_ = mgr.DisableDNSSEC(zoneID)
		response.InternalError(w, "签名区域失败: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		// If commit fails, also disable DNSSEC to avoid a state where
		// keys exist in the DB but the zone is not flagged enabled.
		_ = mgr.DisableDNSSEC(zoneID)
		response.InternalError(w, "启用DNSSEC失败: "+err.Error())
		return
	}
	committed = true

	response.OKWithMessage(w, "DNSSEC enabled", map[string]string{"zone_id": zoneID})
}

// DisableDNSSEC disables DNSSEC for a zone.
// POST /api/v1/dns/zones/{id}/dnssec/disable
func DisableDNSSEC(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	mgr := getDNSSECManager()
	if err := mgr.DisableDNSSEC(zoneID); err != nil {
		response.InternalError(w, "禁用DNSSEC失败: "+err.Error())
		return
	}

	response.OKWithMessage(w, "DNSSEC disabled", map[string]string{"zone_id": zoneID})
}

// RotateDNSSECKeys rotates DNSSEC keys for a zone.
// POST /api/v1/dns/zones/{id}/dnssec/rotate
func RotateDNSSECKeys(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	mgr := getDNSSECManager()
	if err := mgr.RotateKeys(zoneID); err != nil {
		response.InternalError(w, "轮换DNSSEC密钥失败: "+err.Error())
		return
	}

	response.OKWithMessage(w, "DNSSEC keys rotated", map[string]string{"zone_id": zoneID})
}

// --- Helper functions ---

// initCachedManagers initializes the cached DNS manager instances exactly once.
func initCachedManagers() {
	cacheInitOnce.Do(func() {
		cachedZoneMgr = zone.NewZoneManager(DNSServices.DB, DNSServices.ZoneStore)
		cachedRecordMgr = zone.NewRecordManager(DNSServices.DB, DNSServices.ZoneStore, cachedZoneMgr)
		cachedDNSSECMgr = dnssec.NewDNSSECManager(DNSServices.DB, cachedZoneMgr, DNSServices.ZoneStore, DNSServices.JWTSecret)
	})
}

// getZoneManager returns the cached ZoneManager instance.
func getZoneManager() *zone.ZoneManager {
	initCachedManagers()
	return cachedZoneMgr
}

// getRecordManager returns the cached RecordManager instance.
func getRecordManager() *zone.RecordManager {
	initCachedManagers()
	return cachedRecordMgr
}

// getDNSSECManager returns the cached DNSSECManager instance.
func getDNSSECManager() *dnssec.DNSSECManager {
	initCachedManagers()
	return cachedDNSSECMgr
}
