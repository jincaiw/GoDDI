package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/configver"
	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/dhcp/option"
	"github.com/jasonwa/goddi/internal/dhcp/reservation"
	"github.com/jasonwa/goddi/internal/dhcp/scope"
	"github.com/jasonwa/goddi/internal/ipam"
)

// DHCPServices holds references to DHCP server components for API handlers.
var DHCPServices *DHCPServiceContainer

// dhcpInitOnce ensures DHCPServices is initialized exactly once.
var dhcpInitOnce sync.Once

// DHCPServiceContainer holds references to all DHCP service components.
type DHCPServiceContainer struct {
	DB          *sql.DB
	ScopeMgr    *scope.Manager
	LeaseMgr    *lease.Manager
	ReservMgr   *reservation.Manager
	OptionMgr   *option.Manager
	EventLogger *dhcpinternal.EventLogger

	// LeasesAreReplica marks the lease view as a copy.
	//
	// When the data plane runs in its own process, the rows the console reads
	// were pushed up by the process that owns them. Reading them is useful;
	// writing them is not, because the next push replaces them and the operator
	// sees an action that reported success and then quietly undid itself. With
	// this set, lease mutations are refused with an explanation instead.
	LeasesAreReplica bool
}

// InitDHCPServices initializes the DHCP service container for API handlers.
func InitDHCPServices(svc *DHCPServiceContainer) {
	dhcpInitOnce.Do(func() {
		DHCPServices = svc
	})
}

// --- Scope Handlers ---

// ListDHCPScopes lists DHCP scopes with pagination.
func ListDHCPScopes(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ScopeMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := scope.ScopeFilter{
		Name:     r.URL.Query().Get("name"),
		Subnet:   r.URL.Query().Get("subnet"),
		Page:     page,
		PageSize: pageSize,
	}
	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}

	scopes, total, err := DHCPServices.ScopeMgr.ListScopes(filter)
	if err != nil {
		response.InternalErrorWithLog(w, "列表查询失败", err)
		return
	}

	if scopes == nil {
		scopes = []scope.Scope{}
	}

	response.OKPaginated(w, scopes, total, page, pageSize)
}

// scopePlanRef is an optimistic-concurrency reference to a previewed scope
// plan. A create that carries one is only applied if the state the plan was
// built from is still the state on disk.
type scopePlanRef struct {
	SubnetID    string `json:"subnet_id"`
	Fingerprint string `json:"fingerprint"`
}

var (
	errPlanRefIncomplete    = errors.New("计划引用必须同时包含 subnet_id 与 fingerprint")
	errPlanCheckUnavailable = errors.New("无法校验作用域计划：IPAM 侧不可用")
)

// checkScopePlan refuses a create whose approved preview no longer describes
// the current state.
//
// A reference that cannot be checked is refused rather than ignored. A
// half-filled one means the caller changed their mind about what to send; one
// presented while the IPAM side is unreachable means the caller believes their
// preview was verified and it was not. Both are worse than a refusal, because
// the operator walks away thinking a check happened.
func checkScopePlan(ref *scopePlanRef) error {
	if ref.SubnetID == "" || ref.Fingerprint == "" {
		return errPlanRefIncomplete
	}
	if IPAMServices == nil || IPAMServices.Linkage == nil {
		return errPlanCheckUnavailable
	}
	return IPAMServices.Linkage.CheckDHCPScopePlan(ref.SubnetID, ref.Fingerprint)
}

// CreateDHCPScope creates a new DHCP scope.
func CreateDHCPScope(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ScopeMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	// The plan reference is decoded alongside the scope rather than as part of
	// it: it describes the conversation, not the scope, and it must not end up
	// stored.
	var req struct {
		scope.ScopeOptions
		Plan *scopePlanRef `json:"plan,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}
	opts := req.ScopeOptions

	if opts.Name == "" {
		response.BadRequest(w, "缺少名称")
		return
	}
	if opts.Subnet == "" {
		response.BadRequest(w, "缺少子网")
		return
	}
	if opts.StartIP == "" {
		response.BadRequest(w, "缺少起始IP")
		return
	}
	if opts.EndIP == "" {
		response.BadRequest(w, "缺少结束IP")
		return
	}

	// Validate IP address formats.
	startIP := net.ParseIP(opts.StartIP)
	if startIP == nil {
		response.BadRequest(w, "起始IP地址格式无效")
		return
	}
	endIP := net.ParseIP(opts.EndIP)
	if endIP == nil {
		response.BadRequest(w, "结束IP地址格式无效")
		return
	}

	// Validate that StartIP < EndIP.
	if bytes.Compare(startIP, endIP) >= 0 {
		response.BadRequest(w, "起始IP必须小于结束IP")
		return
	}

	// Validate subnet CIDR and range membership here so that user input
	// errors are reported as 400 instead of bubbling up as a 500 from the
	// storage layer.
	if opts.Subnet != "" {
		_, ipNet, err := net.ParseCIDR(opts.Subnet)
		if err != nil {
			response.BadRequest(w, "子网格式无效，应为CIDR格式如 192.168.1.0/24")
			return
		}
		if !ipNet.Contains(startIP) || !ipNet.Contains(endIP) {
			response.BadRequest(w, "起始/结束IP必须位于子网范围内")
			return
		}
	}

	if req.Plan != nil {
		switch err := checkScopePlan(req.Plan); {
		case err == nil:
		case errors.Is(err, errPlanRefIncomplete):
			response.BadRequest(w, err.Error())
			return
		case errors.Is(err, errPlanCheckUnavailable):
			response.ServiceUnavailable(w, err.Error(), nil)
			return
		case errors.Is(err, ipam.ErrDHCPScopePlanStale):
			response.Conflict(w, err.Error())
			return
		default:
			response.InternalErrorWithLog(w, "校验作用域计划失败", err)
			return
		}
	}

	sc, err := DHCPServices.ScopeMgr.CreateScope(opts)
	if err != nil {
		response.InternalErrorWithLog(w, "创建失败", err)
		return
	}

	// The creation becomes revision 1, so the scope's history starts at the
	// pool it was made with rather than at the first edit.
	recordResourceCreation(r, configver.ResourceDHCPScope, sc.ID, configver.DHCPScopeContentFromScope(sc))

	response.Created(w, sc)
}

// GetDHCPScope retrieves a DHCP scope by ID.
func GetDHCPScope(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ScopeMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少作用域ID")
		return
	}

	sc, err := DHCPServices.ScopeMgr.GetScope(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, sc)
}

// UpdateDHCPScope updates a DHCP scope.
func UpdateDHCPScope(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ScopeMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少作用域ID")
		return
	}

	var opts scope.ScopeOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Validate subnet CIDR and range membership so user input errors are
	// reported as 400 instead of a 500 from the storage layer.
	if opts.Subnet != "" {
		_, ipNet, err := net.ParseCIDR(opts.Subnet)
		if err != nil {
			response.BadRequest(w, "子网格式无效，应为CIDR格式如 192.168.1.0/24")
			return
		}
		if opts.StartIP != "" && opts.EndIP != "" {
			startIP := net.ParseIP(opts.StartIP)
			endIP := net.ParseIP(opts.EndIP)
			if startIP == nil || endIP == nil {
				response.BadRequest(w, "起始/结束IP地址格式无效")
				return
			}
			if !ipNet.Contains(startIP) || !ipNet.Contains(endIP) {
				response.BadRequest(w, "起始/结束IP必须位于子网范围内")
				return
			}
		}
	}

	sc, err := DHCPServices.ScopeMgr.UpdateScope(id, opts)
	if err != nil {
		response.InternalErrorWithLog(w, "更新失败", err)
		return
	}

	response.OK(w, sc)
}

// DeleteDHCPScope deletes a DHCP scope.
func DeleteDHCPScope(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ScopeMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少作用域ID")
		return
	}

	if err := DHCPServices.ScopeMgr.DeleteScope(id); err != nil {
		response.InternalErrorWithLog(w, "删除失败", err)
		return
	}

	response.OK(w, map[string]string{"id": id})
}
