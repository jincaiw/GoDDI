package handler

import (
	"net/http"

	"github.com/jasonwa/goddi/internal/api/response"
	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
)

// ListDHCPLogs queries DHCP logs with pagination and filtering.
func ListDHCPLogs(w http.ResponseWriter, r *http.Request) {
	if DHCPServices == nil || DHCPServices.DB == nil {
		response.InternalError(w, "DHCP services not initialized")
		return
	}

	page, pageSize := response.ParsePagination(r)

	filter := dhcpinternal.DHCPLogFilter{
		ScopeID:    r.URL.Query().Get("scope_id"),
		EventType:  dhcpinternal.DHCPEventType(r.URL.Query().Get("event_type")),
		MACAddress: r.URL.Query().Get("mac"),
		IPAddress:  r.URL.Query().Get("ip"),
		StartTime:  r.URL.Query().Get("start_time"),
		EndTime:    r.URL.Query().Get("end_time"),
		Page:       page,
		PageSize:   pageSize,
	}

	entries, total, err := dhcpinternal.QueryDHCPLogs(DHCPServices.DB, filter)
	if err != nil {
		response.InternalError(w, "failed to query DHCP logs: "+err.Error())
		return
	}

	if entries == nil {
		entries = []dhcpinternal.DHCPLogEntry{}
	}

	response.OKPaginated(w, entries, total, page, pageSize)
}
