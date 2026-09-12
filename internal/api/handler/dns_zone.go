package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/configver"
	"github.com/jasonwa/goddi/internal/dns/dnssec"
	"github.com/jasonwa/goddi/internal/dns/transfer"
	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/jasonwa/goddi/internal/rbac"
	"github.com/jasonwa/goddi/pkg/dnsutil"
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
		response.InternalErrorWithLog(w, "列表查询失败", err)
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

	if err := dnsutil.ValidateZoneName(opts.Name); err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	z, err := getZoneManager().CreateZone(opts)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}

	// The creation becomes revision 1, and so does the record set the zone was
	// born with -- usually empty, which is still the right starting point for
	// the record history. Without these the first revision of either resource
	// would be its first edit.
	recordResourceCreation(r, configver.ResourceDNSZone, z.ID, configver.DNSZoneContentFromZone(z))
	recordRecordSetCreation(r, DNSServices.DB, z.ID)

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

	if opts.Name != "" {
		if err := dnsutil.ValidateZoneName(opts.Name); err != nil {
			response.BadRequest(w, err.Error())
			return
		}
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
			response.InternalErrorWithLog(w, "导入CSV失败", err)
			return
		}
	default:
		if err := recordMgr.ImportZoneFile(zoneID, req.Content); err != nil {
			response.InternalErrorWithLog(w, "导入区域文件失败", err)
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
			response.InternalErrorWithLog(w, "导出CSV失败", err)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=zone-records.csv")
		w.Write(data)
	default:
		content, err := recordMgr.ExportZoneFile(zoneID)
		if err != nil {
			response.InternalErrorWithLog(w, "导出区域文件失败", err)
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
		response.InternalErrorWithLog(w, "同步区域失败", err)
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

// dnssecExperimentalNote explains what enabling DNSSEC currently does and,
// more importantly, what it does not do. It is surfaced in API responses and
// in the UI so operators are never left with a false sense of security.
const dnssecExperimentalNote = "DNSSEC 为实验功能：当前仅标记区域并生成密钥，权威应答尚未携带 RRSIG/DNSKEY，校验方会判定为 Bogus"

// dnssecFeatureEnabled reports whether the experimental DNSSEC feature gate is
// on. It is disabled by default (see config.DNSSECConfig) because signing is
// not implemented yet.
func dnssecFeatureEnabled() bool {
	return DNSServices != nil && DNSServices.Config != nil && DNSServices.Config.DNS.DNSSEC.Enabled
}

// EnableDNSSEC enables DNSSEC for a zone.
// POST /api/v1/dns/zones/{id}/dnssec/enable
//
// EXPERIMENTAL: this marks the zone as DNSSEC-enabled and generates KSK/ZSK,
// but the zone is not actually signed yet — no RRSIG or NSEC records are
// produced, so validating resolvers will treat answers as Bogus. The endpoint
// is gated behind dns.dnssec.enabled until real signing lands.
func EnableDNSSEC(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	if !dnssecFeatureEnabled() {
		response.Forbidden(w, "DNSSEC 实验功能未启用；如需体验请在配置中设置 dns.dnssec.enabled=true。"+dnssecExperimentalNote)
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

	// NOTE: no surrounding transaction here on purpose. The pool is
	// configured with SetMaxOpenConns(1) (see internal/database), and
	// DNSSECManager calls use the pool directly rather than a tx handle;
	// opening a transaction would hold that single connection and every
	// subsequent query would block forever. Failure paths below still
	// roll back the zone state via DisableDNSSEC.

	// Generate KSK and ZSK with rollback on failure.
	ksk, err := mgr.GenerateKSK(zoneID, req.Algorithm)
	if err != nil {
		response.InternalErrorWithLog(w, "生成KSK失败", err)
		return
	}
	// Mark the new KSK as part of the same logical operation; we keep
	// the row but defer the commit until both keys and signing succeed.
	_ = ksk

	zsk, err := mgr.GenerateZSK(zoneID, req.Algorithm)
	if err != nil {
		// Clean up the KSK row that was just inserted so the zone is not
		// left half-configured.
		_ = mgr.DisableDNSSEC(zoneID)
		response.InternalErrorWithLog(w, "生成ZSK失败", err)
		return
	}
	_ = zsk

	// Sign the zone. On failure roll back the transaction AND disable
	// the keys we just generated.
	if err := mgr.SignZone(zoneID); err != nil {
		_ = mgr.DisableDNSSEC(zoneID)
		response.InternalErrorWithLog(w, "签名区域失败", err)
		return
	}

	response.OKWithMessage(w, "DNSSEC 已启用（实验功能）", map[string]any{
		"zone_id": zoneID,
		"warning": dnssecExperimentalNote,
	})
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
		response.InternalErrorWithLog(w, "禁用DNSSEC失败", err)
		return
	}

	response.OKWithMessage(w, "DNSSEC disabled", map[string]string{"zone_id": zoneID})
}

// RotateDNSSECKeys rotates DNSSEC keys for a zone.
// POST /api/v1/dns/zones/{id}/dnssec/rotate
//
// EXPERIMENTAL: gated by dns.dnssec.enabled, see EnableDNSSEC.
func RotateDNSSECKeys(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	if !dnssecFeatureEnabled() {
		response.Forbidden(w, "DNSSEC 实验功能未启用；如需体验请在配置中设置 dns.dnssec.enabled=true。"+dnssecExperimentalNote)
		return
	}

	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		response.BadRequest(w, "缺少区域ID")
		return
	}

	mgr := getDNSSECManager()
	if err := mgr.RotateKeys(zoneID); err != nil {
		response.InternalErrorWithLog(w, "轮换DNSSEC密钥失败", err)
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
		// Make the record manager reachable from ZoneManager.CloneZone.
		zone.RegisterSharedRecordManager(cachedRecordMgr)
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

// --- Zone Clone / Convert / Batch Delete (Technitium v13.5/v15.3 parity) ---

// CloneDNSZone handles POST /api/v1/dns/zones/{id}/clone
func CloneDNSZone(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.Name == "" {
		response.BadRequest(w, "缺少新区域名称")
		return
	}

	clone, err := getZoneManager().CloneZone(id, req.Name)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	response.Created(w, clone)
}

// ConvertDNSZone handles POST /api/v1/dns/zones/{id}/convert
func ConvertDNSZone(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.Type == "" {
		response.BadRequest(w, "缺少目标区域类型")
		return
	}

	z, err := getZoneManager().ConvertZoneType(id, req.Type)
	if err != nil {
		response.BadRequest(w, err.Error())
		return
	}
	response.OK(w, z)
}

// BatchDeleteDNSZones handles POST /api/v1/dns/zones/batch-delete
func BatchDeleteDNSZones(w http.ResponseWriter, r *http.Request) {
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
		response.BadRequest(w, "缺少区域ID列表")
		return
	}
	if len(req.IDs) > 100 {
		response.BadRequest(w, "单次最多删除 100 个区域")
		return
	}

	mgr := getZoneManager()
	deleted, failed := 0, make([]string, 0)
	for _, id := range req.IDs {
		// Per-zone permission intersection (Technitium parity).
		if allowed, err := mgr.ZonePermissionAllows(id, rbac.GetUserID(r.Context()), rbac.GetRoleIDs(r.Context()), "delete"); err == nil && !allowed {
			failed = append(failed, id)
			continue
		}
		if err := mgr.DeleteZone(id); err != nil {
			failed = append(failed, id)
			continue
		}
		deleted++
	}

	response.OK(w, map[string]interface{}{
		"deleted": deleted,
		"failed":  failed,
	})
}
