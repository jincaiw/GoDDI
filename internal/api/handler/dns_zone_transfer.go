package handler

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/api/response"
)

// Zone transfer authorization (dns_zone_transfer) is what makes AXFR/IXFR
// usable: the transfer handler both checks the client CIDR (CheckTransferACL)
// and requires the request's TSIG key name to be bound to the zone
// (HandleAXFR JOIN dns_zone_transfer.tsig_key_name). Without a management path
// for these rows a pure UI/API deployment could never authorize a transfer.
// These endpoints provide that path; rows are written to the control DB and
// replicated to the DNS data plane like the rest of dns_zone_transfer.

type zoneTransferEntry struct {
	ID          string `json:"id"`
	ZoneID      string `json:"zone_id"`
	AllowedCIDR string `json:"allowed_cidr"`
	TSIGKeyName string `json:"tsig_key_name"`
	CreatedAt   string `json:"created_at"`
}

type createZoneTransferRequest struct {
	AllowedCIDR string `json:"allowed_cidr"`
	TSIGKeyName string `json:"tsig_key_name"`
}

func zoneIDParam(r *http.Request) string {
	if id := chi.URLParam(r, "zoneId"); id != "" {
		return id
	}
	return chi.URLParam(r, "id")
}

func zoneExists(db *sql.DB, zoneID string) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM dns_zones WHERE id = ?`, zoneID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListZoneTransfers lists transfer authorizations for a zone.
// GET /api/v1/dns/zones/{id}/transfers
func ListZoneTransfers(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	zoneID := zoneIDParam(r)
	ok, err := zoneExists(DNSServices.DB, zoneID)
	if err != nil {
		response.InternalErrorWithLog(w, "查询区域失败", err)
		return
	}
	if !ok {
		response.NotFound(w, "区域不存在")
		return
	}
	rows, err := DNSServices.DB.Query(`SELECT id, zone_id, allowed_cidr, COALESCE(tsig_key_name,''), COALESCE(created_at,'')
		FROM dns_zone_transfer WHERE zone_id = ? ORDER BY created_at`, zoneID)
	if err != nil {
		response.InternalErrorWithLog(w, "查询传送授权失败", err)
		return
	}
	defer rows.Close()
	out := make([]zoneTransferEntry, 0)
	for rows.Next() {
		var e zoneTransferEntry
		if err := rows.Scan(&e.ID, &e.ZoneID, &e.AllowedCIDR, &e.TSIGKeyName, &e.CreatedAt); err != nil {
			response.InternalErrorWithLog(w, "读取传送授权失败", err)
			return
		}
		out = append(out, e)
	}
	response.OK(w, out)
}

// CreateZoneTransfer authorizes a client CIDR (optionally with a named TSIG
// key) to transfer a zone.
// POST /api/v1/dns/zones/{id}/transfers
func CreateZoneTransfer(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	zoneID := zoneIDParam(r)
	ok, err := zoneExists(DNSServices.DB, zoneID)
	if err != nil {
		response.InternalErrorWithLog(w, "查询区域失败", err)
		return
	}
	if !ok {
		response.NotFound(w, "区域不存在")
		return
	}
	var req createZoneTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "请求格式错误")
		return
	}
	cidr := strings.TrimSpace(req.AllowedCIDR)
	if cidr == "" {
		response.BadRequest(w, "allowed_cidr 必填")
		return
	}
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		// Accept a bare address as a single host CIDR for convenience.
		if ip := net.ParseIP(cidr); ip != nil {
			if ip.To4() != nil {
				cidr += "/32"
			} else {
				cidr += "/128"
			}
		} else {
			response.BadRequest(w, "allowed_cidr 不是合法 CIDR")
			return
		}
	}
	tsigName := strings.TrimSpace(req.TSIGKeyName)
	if tsigName != "" {
		var n int
		if err := DNSServices.DB.QueryRow(`SELECT count(*) FROM dns_tsig_keys WHERE name = ?`, tsigName).Scan(&n); err != nil {
			response.InternalErrorWithLog(w, "查询 TSIG 密钥失败", err)
			return
		}
		if n == 0 {
			response.BadRequest(w, "tsig_key_name 不存在")
			return
		}
	}
	e := zoneTransferEntry{ID: uuid.NewString(), ZoneID: zoneID, AllowedCIDR: cidr, TSIGKeyName: tsigName}
	if _, err := DNSServices.DB.Exec(`INSERT INTO dns_zone_transfer (id, zone_id, allowed_cidr, tsig_key_name, created_at)
		VALUES (?, ?, ?, ?, datetime('now'))`, e.ID, e.ZoneID, e.AllowedCIDR, e.TSIGKeyName); err != nil {
		response.InternalErrorWithLog(w, "创建传送授权失败", err)
		return
	}
	response.Created(w, e)
}

// DeleteZoneTransfer removes a transfer authorization.
// DELETE /api/v1/dns/zones/{id}/transfers/{transferId}
func DeleteZoneTransfer(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}
	zoneID := zoneIDParam(r)
	transferID := chi.URLParam(r, "transferId")
	res, err := DNSServices.DB.Exec(`DELETE FROM dns_zone_transfer WHERE id = ? AND zone_id = ?`, transferID, zoneID)
	if err != nil {
		response.InternalErrorWithLog(w, "删除传送授权失败", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		response.NotFound(w, "传送授权不存在")
		return
	}
	response.OK(w, map[string]any{"deleted": transferID})
}
