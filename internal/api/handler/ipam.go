package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/ipam"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/space"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

// IPAMServices holds references to IPAM server components for API handlers.
var IPAMServices *IPAMServiceContainer

// ipamInitOnce ensures IPAMServices is initialized exactly once.
var ipamInitOnce sync.Once

// IPAMServiceContainer holds references to all IPAM service components.
type IPAMServiceContainer struct {
	DB         *sql.DB
	SpaceMgr   *space.Manager
	SubnetMgr  *subnet.Manager
	AddressMgr *address.Manager
}

// InitIPAMServices initializes the IPAM service container for API handlers.
func InitIPAMServices(svc *IPAMServiceContainer) {
	ipamInitOnce.Do(func() {
		IPAMServices = svc
	})
}

// --- Space Handlers ---

// ListIPAMSpaces lists IPAM spaces with pagination.
func ListIPAMSpaces(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SpaceMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := space.SpaceFilter{
		Name:     r.URL.Query().Get("name"),
		Page:     page,
		PageSize: pageSize,
	}

	spaces, total, err := IPAMServices.SpaceMgr.ListSpaces(filter)
	if err != nil {
		response.InternalError(w, "列表查询失败: "+err.Error())
		return
	}

	if spaces == nil {
		spaces = []space.Space{}
	}

	response.OKPaginated(w, spaces, total, page, pageSize)
}

// CreateIPAMSpace creates a new IPAM space.
func CreateIPAMSpace(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SpaceMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "缺少名称")
		return
	}

	s, err := IPAMServices.SpaceMgr.CreateSpace(req.Name, req.Description)
	if err != nil {
		response.InternalError(w, "创建失败: "+err.Error())
		return
	}

	response.Created(w, s)
}

// GetIPAMSpace retrieves an IPAM space by ID.
func GetIPAMSpace(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SpaceMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少空间ID")
		return
	}

	s, err := IPAMServices.SpaceMgr.GetSpace(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, s)
}

// UpdateIPAMSpace updates an IPAM space.
func UpdateIPAMSpace(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SpaceMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少空间ID")
		return
	}

	var opts space.SpaceOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	s, err := IPAMServices.SpaceMgr.UpdateSpace(id, opts)
	if err != nil {
		response.InternalError(w, "更新失败: "+err.Error())
		return
	}

	response.OK(w, s)
}

// DeleteIPAMSpace deletes an IPAM space.
func DeleteIPAMSpace(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SpaceMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少空间ID")
		return
	}

	if err := IPAMServices.SpaceMgr.DeleteSpace(id); err != nil {
		response.InternalError(w, "删除失败: "+err.Error())
		return
	}

	response.OK(w, map[string]string{"id": id})
}

// --- Subnet Handlers ---

// ListIPAMSubnets lists IPAM subnets with pagination and filtering.
func ListIPAMSubnets(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := subnet.SubnetFilter{
		SpaceID:  r.URL.Query().Get("space_id"),
		Name:     r.URL.Query().Get("name"),
		CIDR:     r.URL.Query().Get("cidr"),
		Location: r.URL.Query().Get("location"),
		Page:     page,
		PageSize: pageSize,
	}
	if vlanStr := r.URL.Query().Get("vlan_id"); vlanStr != "" {
		if v, err := strconv.Atoi(vlanStr); err == nil {
			filter.VLANID = &v
		}
	}

	subnets, total, err := IPAMServices.SubnetMgr.ListSubnets(filter)
	if err != nil {
		response.InternalError(w, "列表查询失败: "+err.Error())
		return
	}

	if subnets == nil {
		subnets = []subnet.Subnet{}
	}

	response.OKPaginated(w, subnets, total, page, pageSize)
}

// CreateIPAMSubnet creates a new IPAM subnet.
func CreateIPAMSubnet(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	var req struct {
		SpaceID     string `json:"space_id"`
		Name        string `json:"name"`
		CIDR        string `json:"cidr"`
		VLANID      *int   `json:"vlan_id"`
		Location    string `json:"location"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "缺少名称")
		return
	}
	if req.CIDR == "" {
		response.BadRequest(w, "缺少CIDR")
		return
	}
	if req.SpaceID == "" {
		response.BadRequest(w, "缺少space_id")
		return
	}

	opts := subnet.SubnetOptions{
		Name:        req.Name,
		CIDR:        req.CIDR,
		VLANID:      req.VLANID,
		Location:    req.Location,
		Description: req.Description,
	}

	s, err := IPAMServices.SubnetMgr.CreateSubnet(req.SpaceID, req.Name, req.CIDR, opts)
	if err != nil {
		response.InternalError(w, "创建子网失败: "+err.Error())
		return
	}

	response.Created(w, s)
}

// GetIPAMSubnet retrieves an IPAM subnet by ID.
func GetIPAMSubnet(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	s, err := IPAMServices.SubnetMgr.GetSubnet(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, s)
}

// UpdateIPAMSubnet updates an IPAM subnet.
func UpdateIPAMSubnet(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	var opts subnet.SubnetOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	s, err := IPAMServices.SubnetMgr.UpdateSubnet(id, opts)
	if err != nil {
		response.InternalError(w, "更新子网失败: "+err.Error())
		return
	}

	response.OK(w, s)
}

// DeleteIPAMSubnet deletes an IPAM subnet.
func DeleteIPAMSubnet(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	if err := IPAMServices.SubnetMgr.DeleteSubnet(id); err != nil {
		response.InternalError(w, "删除子网失败: "+err.Error())
		return
	}

	response.OK(w, map[string]string{"id": id})
}

// GetIPAMSubnetStats returns usage statistics for a subnet.
func GetIPAMSubnetStats(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	stats, err := IPAMServices.SubnetMgr.GetUsageStats(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, stats)
}

// GenerateDHCPScope generates DHCP scope options from a subnet.
func GenerateDHCPScope(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	scopeOpts, err := IPAMServices.SubnetMgr.GenerateDHCPScope(id)
	if err != nil {
		response.InternalError(w, "生成DHCP作用域失败: "+err.Error())
		return
	}

	response.OK(w, scopeOpts)
}

// GenerateReverseZone generates reverse zone name from a subnet.
func GenerateReverseZone(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	zoneName, err := IPAMServices.SubnetMgr.GenerateReverseZone(id)
	if err != nil {
		response.InternalError(w, "生成反向区域失败: "+err.Error())
		return
	}

	response.OK(w, map[string]string{"zone_name": zoneName})
}

// --- Address Handlers ---

// ListIPAMAddresses lists IPAM addresses with pagination and filtering.
func ListIPAMAddresses(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.AddressMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := address.AddressFilter{
		SubnetID:   r.URL.Query().Get("subnet_id"),
		Status:     address.IPAddressStatus(r.URL.Query().Get("status")),
		IPAddress:  r.URL.Query().Get("ip"),
		MACAddress: r.URL.Query().Get("mac"),
		Hostname:   r.URL.Query().Get("hostname"),
		Owner:      r.URL.Query().Get("owner"),
		Page:       page,
		PageSize:   pageSize,
	}

	addresses, total, err := IPAMServices.AddressMgr.ListAddresses(filter)
	if err != nil {
		response.InternalError(w, "列表查询失败: "+err.Error())
		return
	}

	if addresses == nil {
		addresses = []address.Address{}
	}

	response.OKPaginated(w, addresses, total, page, pageSize)
}

// GetIPAMAddress retrieves an IPAM address by ID.
func GetIPAMAddress(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.AddressMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少地址ID")
		return
	}

	a, err := IPAMServices.AddressMgr.GetAddress(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, a)
}

// UpdateIPAMAddress updates an IPAM address.
func UpdateIPAMAddress(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.AddressMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少地址ID")
		return
	}

	var opts address.AddressOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	a, err := IPAMServices.AddressMgr.UpdateAddress(id, opts)
	if err != nil {
		response.InternalError(w, "更新地址失败: "+err.Error())
		return
	}

	response.OK(w, a)
}

// AllocateIP allocates an IP address.
func AllocateIP(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.AddressMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	var req struct {
		SubnetID    string `json:"subnet_id"`
		IPAddress   string `json:"ip_address"`
		Owner       string `json:"owner"`
		MACAddress  string `json:"mac_address"`
		Hostname    string `json:"hostname"`
		Device      string `json:"device"`
		Location    string `json:"location"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.SubnetID == "" {
		response.BadRequest(w, "缺少subnet_id")
		return
	}

	// Auto-assign an IP if not specified.
	if req.IPAddress == "" {
		autoIP, err := IPAMServices.AddressMgr.AutoAssignIP(req.SubnetID)
		if err != nil {
			response.InternalError(w, "自动分配IP失败: "+err.Error())
			return
		}
		req.IPAddress = autoIP
	}

	opts := address.AddressOptions{
		MACAddress:  req.MACAddress,
		Hostname:    req.Hostname,
		Owner:       req.Owner,
		Device:      req.Device,
		Location:    req.Location,
		Description: req.Description,
	}
	if req.Status != "" {
		s := address.IPAddressStatus(req.Status)
		opts.Status = &s
	}

	a, err := IPAMServices.AddressMgr.AllocateIP(req.SubnetID, req.IPAddress, req.Owner, opts)
	if err != nil {
		response.InternalError(w, "分配IP失败: "+err.Error())
		return
	}

	response.Created(w, a)
}

// ReleaseIP releases an IP address.
func ReleaseIP(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.AddressMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.ID == "" {
		response.BadRequest(w, "缺少ID")
		return
	}

	if err := IPAMServices.AddressMgr.ReleaseIP(req.ID); err != nil {
		response.InternalError(w, "释放IP失败: "+err.Error())
		return
	}

	response.OK(w, map[string]string{"id": req.ID})
}

// --- Import/Export Handlers ---

// MaxImportIPAMRecords caps the number of rows an IPAM import may contain
// per request, mirroring the DNS import limit to keep resource use bounded.
const MaxImportIPAMRecords = 100000

// ImportIPAMData imports IPAM data (CSV/JSON).
func ImportIPAMData(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.DB == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	var req struct {
		Type     string `json:"type"`      // "addresses" or "subnets"
		Format   string `json:"format"`    // "csv" or "json"
		ParentID string `json:"parent_id"` // subnet_id or space_id
		Data     string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Type == "" {
		response.BadRequest(w, "缺少类型（addresses或subnets）")
		return
	}
	if req.Format == "" {
		response.BadRequest(w, "缺少格式（csv或json）")
		return
	}
	if req.ParentID == "" {
		response.BadRequest(w, "缺少parent_id")
		return
	}
	if req.Data == "" {
		response.BadRequest(w, "缺少数据")
		return
	}

	// Limit data size to 5MB to prevent resource exhaustion.
	if len(req.Data) > 5*1024*1024 {
		response.BadRequest(w, "导入数据超过5MB大小限制")
		return
	}

	// Enforce a maximum row count (one record per line) to mirror the DNS
	// import limit. This catches both malicious and accidental large uploads
	// before the CSV parser is invoked.
	if strings.Count(req.Data, "\n") > MaxImportIPAMRecords {
		response.BadRequest(w, fmt.Sprintf("导入数据行数过多，最多支持 %d 条", MaxImportIPAMRecords))
		return
	}

	ie := ipam.NewImportExport(IPAMServices.DB)

	switch req.Type {
	case "addresses":
		if req.Format == "csv" {
			if err := ie.ImportAddressesCSV(req.ParentID, []byte(req.Data)); err != nil {
				response.InternalError(w, "导入地址失败: "+err.Error())
				return
			}
		} else {
			response.BadRequest(w, "地址的JSON导入暂不支持，请使用CSV")
			return
		}
	case "subnets":
		if req.Format == "csv" {
			if err := ie.ImportSubnetsCSV(req.ParentID, []byte(req.Data)); err != nil {
				response.InternalError(w, "导入子网失败: "+err.Error())
				return
			}
		} else {
			response.BadRequest(w, "子网的JSON导入暂不支持，请使用CSV")
			return
		}
	default:
		response.BadRequest(w, "无效的类型，必须为'addresses'或'subnets'")
		return
	}

	response.OKWithMessage(w, "import completed", nil)
}

// ExportIPAMData exports IPAM data (CSV/JSON).
func ExportIPAMData(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.DB == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	exportType := r.URL.Query().Get("type")
	format := r.URL.Query().Get("format")
	parentID := r.URL.Query().Get("parent_id")

	if exportType == "" {
		exportType = "addresses"
	}
	if format == "" {
		format = "csv"
	}
	if parentID == "" {
		response.BadRequest(w, "缺少parent_id")
		return
	}

	ie := ipam.NewImportExport(IPAMServices.DB)

	switch exportType {
	case "addresses":
		if format == "csv" {
			data, err := ie.ExportAddressesCSV(parentID)
			if err != nil {
				response.InternalError(w, "导出地址失败: "+err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=addresses.csv")
			w.Write(data)
			return
		}
	case "subnets":
		if format == "csv" {
			data, err := ie.ExportSubnetsCSV(parentID)
			if err != nil {
				response.InternalError(w, "导出子网失败: "+err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=subnets.csv")
			w.Write(data)
			return
		}
	}

	response.BadRequest(w, "不支持的导出类型或格式")
}
