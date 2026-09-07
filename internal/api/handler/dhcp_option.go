package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dhcp/option"
)

// ListDHCPOptions lists DHCP options, optionally filtered by scope_id.
func ListDHCPOptions(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.OptionMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	scopeID := r.URL.Query().Get("scope_id")

	options, err := DHCPServices.OptionMgr.ListOptions(scopeID)
	if err != nil {
		response.InternalErrorWithLog(w, "列表查询失败", err)
		return
	}

	if options == nil {
		options = []option.Option{}
	}

	response.OK(w, options)
}

// CreateDHCPOption creates a new DHCP option.
func CreateDHCPOption(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.OptionMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	var opts option.OptionOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if opts.Code == 0 {
		response.BadRequest(w, "缺少选项代码")
		return
	}
	if opts.Code < 1 || opts.Code > 254 {
		response.BadRequest(w, "选项代码必须在1-254之间")
		return
	}
	if opts.Value == "" {
		response.BadRequest(w, "缺少选项值")
		return
	}

	o, err := DHCPServices.OptionMgr.CreateOption(opts.ScopeID, opts)
	if err != nil {
		response.InternalErrorWithLog(w, "创建选项失败", err)
		return
	}

	response.Created(w, o)
}

// UpdateDHCPOption updates a DHCP option.
func UpdateDHCPOption(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.OptionMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少选项ID")
		return
	}

	var opts option.OptionOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	o, err := DHCPServices.OptionMgr.UpdateOption(id, opts)
	if err != nil {
		response.InternalErrorWithLog(w, "更新选项失败", err)
		return
	}

	response.OK(w, o)
}

// DeleteDHCPOption deletes a DHCP option.
func DeleteDHCPOption(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.OptionMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少选项ID")
		return
	}

	if err := DHCPServices.OptionMgr.DeleteOption(id); err != nil {
		response.InternalErrorWithLog(w, "删除选项失败", err)
		return
	}

	response.OK(w, map[string]string{"id": id})
}
