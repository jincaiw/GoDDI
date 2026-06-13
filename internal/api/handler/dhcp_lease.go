package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
)

// ListDHCPLeases lists DHCP leases with pagination and filtering.
func ListDHCPLeases(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.LeaseMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := lease.LeaseFilter{
		ScopeID:    r.URL.Query().Get("scope_id"),
		Status:     lease.LeaseStatus(r.URL.Query().Get("status")),
		MACAddress: r.URL.Query().Get("mac"),
		IPAddress:  r.URL.Query().Get("ip"),
		Hostname:   r.URL.Query().Get("hostname"),
		Page:       page,
		PageSize:   pageSize,
	}

	leases, total, err := DHCPServices.LeaseMgr.ListLeases(filter)
	if err != nil {
		response.InternalError(w, "列表查询失败: "+err.Error())
		return
	}

	if leases == nil {
		leases = []lease.Lease{}
	}

	response.OKPaginated(w, leases, total, page, pageSize)
}

// GetDHCPLease retrieves a DHCP lease by ID.
func GetDHCPLease(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.LeaseMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少租约ID")
		return
	}

	l, err := DHCPServices.LeaseMgr.GetLease(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, l)
}

// DeleteDHCPLease releases a DHCP lease.
func DeleteDHCPLease(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.LeaseMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少租约ID")
		return
	}

	if err := DHCPServices.LeaseMgr.ReleaseLease(id); err != nil {
		response.InternalError(w, "释放租约失败: "+err.Error())
		return
	}

	response.OK(w, map[string]string{"id": id})
}
