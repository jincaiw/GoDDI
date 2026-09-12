package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/configver"
	"github.com/jasonwa/goddi/internal/ipam"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/space"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
	"github.com/jasonwa/goddi/internal/rbac"
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
	// Linkage is the bridge to DHCP and DNS. Without it the 360° view has
	// nothing to compare IPAM against.
	Linkage *ipam.Linkage
}

// InitIPAMServices initializes the IPAM service container for API handlers.
func InitIPAMServices(svc *IPAMServiceContainer) {
	ipamInitOnce.Do(func() {
		IPAMServices = svc
	})
}

// ipamActor returns the authenticated caller for the audit trail.
func ipamActor(r *http.Request) string {
	if u := rbac.GetUsername(r.Context()); u != "" {
		return u
	}
	return address.SourceAPI
}

// respondIPAMError maps the package's sentinel errors onto HTTP statuses.
//
// Matching on the error rather than on the message matters here: the difference
// between "the address is taken" (409, retry with another address) and "the
// request is malformed" (400, fix the request) is the difference between an
// operator retrying and an operator investigating.
func respondIPAMError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case err == nil:
		return
	case errors.Is(err, address.ErrAddressTaken),
		errors.Is(err, address.ErrIllegalTransition),
		errors.Is(err, subnet.ErrSubnetHasDependencies):
		response.Conflict(w, err.Error())
	case errors.Is(err, address.ErrInvalidIP),
		errors.Is(err, address.ErrInvalidStatus),
		errors.Is(err, address.ErrOutOfSubnet),
		errors.Is(err, subnet.ErrDHCPv6Unsupported):
		response.BadRequest(w, err.Error())
	case errors.Is(err, address.ErrAddressNotFound),
		errors.Is(err, address.ErrSubnetNotFound):
		response.NotFound(w, err.Error())
	case errors.Is(err, address.ErrPoolExhausted):
		// A full pool is a state of the system, not a bug in the request.
		response.Conflict(w, err.Error())
	default:
		response.InternalErrorWithLog(w, fallback, err)
	}
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
		response.InternalErrorWithLog(w, "列表查询失败", err)
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
		response.InternalErrorWithLog(w, "创建失败", err)
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
		response.InternalErrorWithLog(w, "更新失败", err)
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
		if errors.Is(err, space.ErrSpaceInUse) {
			response.Conflict(w, "删除失败: "+err.Error())
			return
		}
		response.InternalErrorWithLog(w, "删除失败", err)
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
		response.InternalErrorWithLog(w, "列表查询失败", err)
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
		response.InternalErrorWithLog(w, "创建子网失败", err)
		return
	}

	// The creation becomes revision 1, so the subnet's history starts at the
	// values it was made with rather than at the first edit.
	recordResourceCreation(r, configver.ResourceIPAMSubnet, s.ID, configver.IPAMSubnetContentFromSubnet(s))

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
		response.InternalErrorWithLog(w, "更新子网失败", err)
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
		// A refusal carries the dependency list, so the caller can act on it
		// instead of only learning that something is in the way.
		var depErr *subnet.DependencyError
		if errors.As(err, &depErr) {
			response.Conflict(w, depErr.Error())
			return
		}
		respondIPAMError(w, r, err, "删除子网失败")
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
		response.InternalErrorWithLog(w, "生成DHCP作用域失败", err)
		return
	}

	response.OK(w, scopeOpts)
}

// PlanDHCPScope previews the DHCP scope that would be created from a subnet.
//
// It computes and never writes. The plan carries a fingerprint of the state it
// was derived from; the create handler takes that fingerprint back and refuses
// to apply a plan whose world has moved. The draft is composed here, from the
// same manager that has always generated drafts, so the preview and the
// created scope cannot disagree about defaults.
func PlanDHCPScope(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.Linkage == nil || IPAMServices.SubnetMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	plan, err := IPAMServices.Linkage.PlanDHCPScope(id)
	if err != nil {
		respondIPAMError(w, r, err, "生成作用域计划失败")
		return
	}

	draft, err := IPAMServices.SubnetMgr.GenerateDHCPScope(id)
	if err != nil {
		respondIPAMError(w, r, err, "生成作用域草稿失败")
		return
	}

	// The plan knows the router; the generated draft has never filled one in,
	// which is how a scope ends up handing clients no default route.
	if draft.Router == nil && plan.Router != "" {
		router := plan.Router
		draft.Router = &router
	}

	response.OK(w, map[string]interface{}{"plan": plan, "draft": draft})
}

// GenerateReverseZone describes where a subnet's addresses would be published
// in reverse DNS: the subnet's CIDR, the most specific reverse zone name, and
// every less specific alternative.
//
// It computes and does not write. The zone itself is created by POST /dns/zones,
// which requires a different permission, so a console cannot treat this call as
// the action -- reporting success here would report a write that never
// happened. The response carries the alternatives so the operator can pick one
// instead of being handed a decision the server made for them.
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

	plan, err := IPAMServices.SubnetMgr.PlanReverseZone(id)
	if err != nil {
		// A subnet that is not there and a subnet whose CIDR yields no reverse
		// zone are different answers: the first is a stale row in the console,
		// the second is a data problem the operator has to fix.
		if errors.Is(err, subnet.ErrSubnetNotFound) {
			response.NotFound(w, "子网不存在")
			return
		}
		response.BadRequest(w, "无法为此子网生成反向区域: "+err.Error())
		return
	}

	response.OK(w, plan)
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
		SubnetID:      r.URL.Query().Get("subnet_id"),
		SpaceID:       r.URL.Query().Get("space_id"),
		Status:        address.Status(r.URL.Query().Get("status")),
		ObservedState: address.ObservedState(r.URL.Query().Get("observed_state")),
		IPAddress:     r.URL.Query().Get("ip"),
		MACAddress:    r.URL.Query().Get("mac"),
		Hostname:      r.URL.Query().Get("hostname"),
		Owner:         r.URL.Query().Get("owner"),
		Search:        r.URL.Query().Get("search"),
		Page:          page,
		PageSize:      pageSize,
	}

	addresses, total, err := IPAMServices.AddressMgr.ListAddresses(filter)
	if err != nil {
		respondIPAMError(w, r, err, "列表查询失败")
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
	opts.Actor = ipamActor(r)
	opts.Source = address.SourceAdmin
	if opts.Reason == "" {
		opts.Reason = "updated through the API"
	}

	a, err := IPAMServices.AddressMgr.UpdateAddress(id, opts)
	if err != nil {
		respondIPAMError(w, r, err, "更新地址失败")
		return
	}

	response.OK(w, a)
}

// AllocateIP allocates an IP address.
//
// When no address is named, selection and claiming happen in one call. The
// previous split -- find a free address, then allocate that address -- had a
// window in which another request could take it, and the loser was told the
// address it never chose was unavailable.
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
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.SubnetID == "" {
		response.BadRequest(w, "缺少subnet_id")
		return
	}

	a, err := IPAMServices.AddressMgr.AllocateIP(address.AllocateRequest{
		SubnetID:    req.SubnetID,
		IPAddress:   req.IPAddress,
		Status:      address.Status(req.Status),
		MACAddress:  req.MACAddress,
		Hostname:    req.Hostname,
		Owner:       req.Owner,
		Device:      req.Device,
		Location:    req.Location,
		Description: req.Description,
		Actor:       ipamActor(r),
		Source:      address.SourceAPI,
		Reason:      req.Reason,
	})
	if err != nil {
		respondIPAMError(w, r, err, "分配IP失败")
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
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.ID == "" {
		response.BadRequest(w, "缺少ID")
		return
	}
	if req.Reason == "" {
		req.Reason = "released through the API"
	}

	if _, err := IPAMServices.AddressMgr.ReleaseAddress(
		req.ID, ipamActor(r), req.Reason, address.SourceAPI); err != nil {
		respondIPAMError(w, r, err, "释放IP失败")
		return
	}

	response.OK(w, map[string]string{"id": req.ID})
}

// TransitionIPAMAddress moves an address to an explicit allocation status.
//
// This is the only path that can reach states the allocation endpoints refuse
// to reach, such as marking an address as the gateway or resolving a conflict.
// Each call is validated against the state machine and recorded in the history.
func TransitionIPAMAddress(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.AddressMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少地址ID")
		return
	}

	var req struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
		Actor  string `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	if req.Status == "" {
		response.BadRequest(w, "缺少status")
		return
	}
	if req.Reason == "" {
		response.BadRequest(w, "缺少reason：状态变更必须说明原因")
		return
	}

	a, err := IPAMServices.AddressMgr.TransitionAddress(
		id, address.Status(req.Status), ipamActor(r), req.Reason, address.SourceAPI)
	if err != nil {
		respondIPAMError(w, r, err, "状态变更失败")
		return
	}
	response.OK(w, a)
}

// GetIPAMAddressView returns the 360° view of an address: the IPAM record, the
// subnet and space it belongs to, the DNS records that publish it, the DHCP
// leases and reservations that claim it, its change history, and any place
// those disagree.
func GetIPAMAddressView(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.Linkage == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	ip := r.URL.Query().Get("ip")
	if spaceID == "" || ip == "" {
		response.BadRequest(w, "缺少space_id或ip")
		return
	}

	view, err := IPAMServices.Linkage.ViewAddress(spaceID, ip)
	if err != nil {
		respondIPAMError(w, r, err, "查询地址视图失败")
		return
	}
	response.OK(w, view)
}

// GetIPAMSubnetDependencies reports what would break if a subnet were deleted.
//
// It is a read-only preview of the refusal DeleteSubnet produces, so an
// operator can see the blockers before trying and read the same list either
// way.
func GetIPAMSubnetDependencies(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.Linkage == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少子网ID")
		return
	}

	deps, err := IPAMServices.Linkage.CheckDependencies(id)
	if err != nil {
		respondIPAMError(w, r, err, "查询依赖失败")
		return
	}
	response.OK(w, map[string]any{
		"subnet_id":    id,
		"dependencies": deps,
		"deletable":    len(deps) == 0,
	})
}

// GetIPAMAddressDNSLinks lists the DNS records publishing an address.
//
// Derived from `dns_records`, not read from `ipam_dns_links`: that table has no
// production writer, so reading it returned an empty list for every address no
// matter how many names pointed at it.
func GetIPAMAddressDNSLinks(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.Linkage == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少地址ID")
		return
	}

	links, err := IPAMServices.Linkage.PublishingRecordsForAddressID(id)
	if err != nil {
		respondIPAMError(w, r, err, "查询DNS关联失败")
		return
	}
	if links == nil {
		links = []ipam.PublishingRecord{}
	}
	response.OK(w, links)
}

// GetIPAMIntegrity reports rows that no lookup can find.
//
// Two things are worth surfacing rather than leaving in the database: address
// values that are not the canonical spelling of what they represent (so they
// sit outside the (space, IP) identity), and A/AAAA records whose value is not
// the canonical spelling of the address they name (so the 360° view, which
// matches that value as a string, does not see them).
//
// The orphaned-link check that used to be the second half is gone rather than
// empty. `ipam_dns_links` has no production writer, so reporting "no orphaned
// links" would have described the code, not the deployment -- the same class of
// false pass this endpoint exists to avoid.
func GetIPAMIntegrity(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.AddressMgr == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	nonCanonical, err := IPAMServices.AddressMgr.NonCanonicalAddresses(200)
	if err != nil {
		respondIPAMError(w, r, err, "检查地址规范性失败")
		return
	}
	nonCanonicalPublishers, err := IPAMServices.AddressMgr.NonCanonicalPublishingRecords(200)
	if err != nil {
		respondIPAMError(w, r, err, "检查DNS记录规范性失败")
		return
	}

	response.OK(w, map[string]any{
		"non_canonical_addresses":  nonCanonical,
		"non_canonical_publishers": nonCanonicalPublishers,
		"clean":                    len(nonCanonical) == 0 && len(nonCanonicalPublishers) == 0,
	})
}

// --- Import/Export Handlers ---

// MaxImportIPAMRecords caps the number of rows an IPAM import may contain
// per request, mirroring the DNS import limit to keep resource use bounded.
const MaxImportIPAMRecords = 100000

// ipamImportRequest is the body of both the import and the import preview.
type ipamImportRequest struct {
	Type     string `json:"type"`      // "addresses" or "subnets"
	Format   string `json:"format"`    // "csv" or "json"
	ParentID string `json:"parent_id"` // subnet_id or space_id
	Data     string `json:"data"`
}

// decodeImportRequest reads and bounds-checks an import body.
//
// The preview and the import share this, so a file the preview accepts cannot
// then be rejected by the import for a reason the preview never looked at --
// which is the only thing that makes a dry run worth reading.
func decodeImportRequest(w http.ResponseWriter, r *http.Request) (*ipamImportRequest, bool) {
	var req ipamImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return nil, false
	}

	if req.Type == "" {
		response.BadRequest(w, "缺少类型（addresses或subnets）")
		return nil, false
	}
	if req.Format == "" {
		response.BadRequest(w, "缺少格式（csv或json）")
		return nil, false
	}
	if req.ParentID == "" {
		response.BadRequest(w, "缺少parent_id")
		return nil, false
	}
	if req.Data == "" {
		response.BadRequest(w, "缺少数据")
		return nil, false
	}

	// Limit data size to 5MB to prevent resource exhaustion.
	if len(req.Data) > 5*1024*1024 {
		response.BadRequest(w, "导入数据超过5MB大小限制")
		return nil, false
	}

	// Enforce a maximum row count (one record per line) to mirror the DNS
	// import limit. This catches both malicious and accidental large uploads
	// before the CSV parser is invoked.
	if strings.Count(req.Data, "\n") > MaxImportIPAMRecords {
		response.BadRequest(w, fmt.Sprintf("导入数据行数过多，最多支持 %d 条", MaxImportIPAMRecords))
		return nil, false
	}

	return &req, true
}

// PreviewIPAMImport reports what an import would do, without writing.
//
// It asks only for `ipam.read`: nothing is created and every fact in the answer
// is derived from data the caller can already read. Requiring write access
// would push operators towards importing to find out, which is the habit the
// dry run exists to replace.
func PreviewIPAMImport(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.DB == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	req, ok := decodeImportRequest(w, r)
	if !ok {
		return
	}

	// The type is checked against the same closed set the import uses, so an
	// unknown type is a bad request here too. Only a type that exists and
	// cannot be previewed is "not implemented".
	switch req.Type {
	case "addresses":
		if req.Format != "csv" {
			response.BadRequest(w, "地址的JSON导入暂不支持，请使用CSV")
			return
		}
	case "subnets":
		if req.Format != "csv" {
			response.BadRequest(w, "子网的JSON导入暂不支持，请使用CSV")
			return
		}
		report, err := ipam.NewImportExport(IPAMServices.DB).
			PreviewSubnetsCSV(req.ParentID, []byte(req.Data))
		if err != nil {
			respondIPAMError(w, r, err, "子网导入预览失败")
			return
		}
		response.OK(w, report)
		return
	default:
		response.BadRequest(w, "无效的类型，必须为'addresses'或'subnets'")
		return
	}

	report, err := ipam.NewImportExport(IPAMServices.DB).
		PreviewAddressesCSV(req.ParentID, []byte(req.Data))
	if err != nil {
		respondIPAMError(w, r, err, "导入预览失败")
		return
	}

	response.OK(w, report)
}

// ImportIPAMData imports IPAM data (CSV/JSON).
func ImportIPAMData(w http.ResponseWriter, r *http.Request) {
	if IPAMServices == nil || IPAMServices.DB == nil {
		response.InternalError(w, "IPAM服务未初始化")
		return
	}

	req, ok := decodeImportRequest(w, r)
	if !ok {
		return
	}

	ie := ipam.NewImportExport(IPAMServices.DB)

	switch req.Type {
	case "addresses":
		if req.Format != "csv" {
			response.BadRequest(w, "地址的JSON导入暂不支持，请使用CSV")
			return
		}
		report, err := ie.ImportAddressesCSV(req.ParentID, []byte(req.Data))
		respondImportResult(w, r, report, err, "导入地址失败")
	case "subnets":
		if req.Format != "csv" {
			response.BadRequest(w, "子网的JSON导入暂不支持，请使用CSV")
			return
		}
		report, err := ie.ImportSubnetsCSVReport(req.ParentID, []byte(req.Data))
		respondSubnetImportResult(w, r, report, err)
	default:
		response.BadRequest(w, "无效的类型，必须为'addresses'或'subnets'")
	}
}

// respondImportResult turns an import outcome into a response.
//
// A file whose rows were refused is a bad request, not a server failure, and
// the per-row reasons are what the operator needs. Everything else that failed
// is a server failure and is answered as one.
func respondImportResult(w http.ResponseWriter, r *http.Request, report *ipam.ImportReport, err error, fallback string) {
	switch {
	case err == nil:
		response.OK(w, report)
	case errors.Is(err, ipam.ErrImportRejected):
		response.BadRequestWithData(w, "导入被拒绝，未写入任何行", report)
	default:
		respondIPAMError(w, r, err, fallback)
	}
}

func respondSubnetImportResult(w http.ResponseWriter, r *http.Request, report *ipam.SubnetImportReport, err error) {
	switch {
	case err == nil:
		response.OK(w, report)
	case errors.Is(err, ipam.ErrImportRejected):
		response.BadRequestWithData(w, "导入被拒绝，未写入任何行", report)
	default:
		respondIPAMError(w, r, err, "导入子网失败")
	}
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
				response.InternalErrorWithLog(w, "导出地址失败", err)
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
				response.InternalErrorWithLog(w, "导出子网失败", err)
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
