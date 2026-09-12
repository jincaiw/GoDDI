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
		Search:     r.URL.Query().Get("search"),
		Page:       page,
		PageSize:   pageSize,
	}

	leases, total, err := DHCPServices.LeaseMgr.ListLeases(filter)
	if err != nil {
		response.InternalErrorWithLog(w, "列表查询失败", err)
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
//
// A lease record lives on the data plane, which owns it and overwrites the
// console's copy on every replication pass. Releasing it here would report
// success and then be undone without a trace, so when the console is reading a
// replica the write is refused and the operator is told where the lease is.
func DeleteDHCPLease(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.LeaseMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}
	if DHCPServices.LeasesAreReplica {
		response.Conflict(w,
			"租约由 DHCP 数据面持有，控制台读到的是副本；请在数据面所在进程上释放，"+
				"否则下一次同步会覆盖此处所做的修改")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少租约ID")
		return
	}

	if err := DHCPServices.LeaseMgr.ReleaseLease(id); err != nil {
		response.InternalErrorWithLog(w, "释放租约失败", err)
		return
	}

	response.OK(w, map[string]string{"id": id})
}
