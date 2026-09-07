package handler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dhcp/reservation"
)

// ListDHCPReservations lists DHCP reservations with pagination and filtering.
func ListDHCPReservations(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ReservMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := reservation.ReservationFilter{
		ScopeID:    r.URL.Query().Get("scope_id"),
		MACAddress: r.URL.Query().Get("mac"),
		IPAddress:  r.URL.Query().Get("ip"),
		Hostname:   r.URL.Query().Get("hostname"),
		Page:       page,
		PageSize:   pageSize,
	}

	reservations, total, err := DHCPServices.ReservMgr.ListReservations(filter)
	if err != nil {
		response.InternalErrorWithLog(w, "列表查询失败", err)
		return
	}

	if reservations == nil {
		reservations = []reservation.Reservation{}
	}

	response.OKPaginated(w, reservations, total, page, pageSize)
}

// CreateDHCPReservation creates a new DHCP reservation.
func CreateDHCPReservation(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ReservMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	var req struct {
		ScopeID     string `json:"scope_id"`
		IPAddress   string `json:"ip_address"`
		MACAddress  string `json:"mac_address"`
		Hostname    string `json:"hostname"`
		Description string `json:"description"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.IPAddress == "" {
		response.BadRequest(w, "缺少IP地址")
		return
	}
	if net.ParseIP(req.IPAddress) == nil {
		response.BadRequest(w, "IP地址格式无效")
		return
	}
	if req.MACAddress == "" {
		response.BadRequest(w, "缺少MAC地址")
		return
	}
	if _, err := net.ParseMAC(req.MACAddress); err != nil {
		response.BadRequest(w, "MAC地址格式无效")
		return
	}
	if req.ScopeID == "" {
		response.BadRequest(w, "缺少作用域ID")
		return
	}

	opts := reservation.ReservationOptions{
		IPAddress:   req.IPAddress,
		MACAddress:  req.MACAddress,
		Hostname:    req.Hostname,
		Description: req.Description,
		Enabled:     req.Enabled,
	}

	res, err := DHCPServices.ReservMgr.CreateReservation(req.ScopeID, req.IPAddress, req.MACAddress, req.Hostname, opts)
	if err != nil {
		switch {
		case errors.Is(err, reservation.ErrDuplicate):
			response.Conflict(w, "保留冲突: "+err.Error())
		case errors.Is(err, reservation.ErrOutOfRange), errors.Is(err, reservation.ErrInvalidData):
			response.BadRequest(w, err.Error())
		default:
			response.InternalErrorWithLog(w, "创建保留失败", err)
		}
		return
	}

	response.Created(w, res)
}

// GetDHCPReservation retrieves a DHCP reservation by ID.
func GetDHCPReservation(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ReservMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少保留ID")
		return
	}

	res, err := DHCPServices.ReservMgr.GetReservation(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}

	response.OK(w, res)
}

// UpdateDHCPReservation updates a DHCP reservation.
func UpdateDHCPReservation(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ReservMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少保留ID")
		return
	}

	var opts reservation.ReservationOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	res, err := DHCPServices.ReservMgr.UpdateReservation(id, opts)
	if err != nil {
		switch {
		case errors.Is(err, reservation.ErrDuplicate):
			response.Conflict(w, "保留冲突: "+err.Error())
		case errors.Is(err, reservation.ErrOutOfRange), errors.Is(err, reservation.ErrInvalidData):
			response.BadRequest(w, err.Error())
		default:
			response.InternalErrorWithLog(w, "更新保留失败", err)
		}
		return
	}

	response.OK(w, res)
}

// DeleteDHCPReservation deletes a DHCP reservation.
func DeleteDHCPReservation(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.ReservMgr == nil {
		response.InternalError(w, "DHCP服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少保留ID")
		return
	}

	if err := DHCPServices.ReservMgr.DeleteReservation(id); err != nil {
		response.InternalErrorWithLog(w, "删除保留失败", err)
		return
	}

	response.OK(w, map[string]string{"id": id})
}
