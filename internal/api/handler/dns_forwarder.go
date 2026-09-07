package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dns/cache"
	"github.com/jasonwa/goddi/internal/dns/client"
	"github.com/jasonwa/goddi/internal/dns/filter"
	"github.com/jasonwa/goddi/internal/dns/forwarder"
	dnsserver "github.com/jasonwa/goddi/internal/dns/server"
	"github.com/jasonwa/goddi/internal/dns/zone"
)

// validProtocols contains the set of allowed DNS forwarder protocols.
var validProtocols = map[string]bool{
	"udp": true,
	"tcp": true,
	"dot": true,
	"doh": true,
	"doq": true,
}

// DNSServices holds references to DNS server components for API handlers.
var DNSServices *DNSServiceContainer

// dnsInitOnce ensures DNSServices is initialized exactly once.
var dnsInitOnce sync.Once

// DNSServiceContainer holds references to all DNS service components.
type DNSServiceContainer struct {
	DB               *sql.DB
	Cache            *cache.Cache
	Filter           *filter.FilterEngine
	Forwarder        *forwarder.ForwarderGroup
	Conditional      *forwarder.ConditionalForwarderManager
	DNSClient        *client.DNSClient
	ZoneStore        *zone.Store
	BlockListFetcher *filter.BlockListFetcher
	DNSServer        *dnsserver.Server
	JWTSecret        string
	// Config exposes the effective runtime configuration to handlers that
	// need to consult feature gates (e.g. the experimental DNSSEC switch).
	Config *config.Config
}

// InitDNSServices initializes the DNS service container for API handlers.
func InitDNSServices(svc *DNSServiceContainer) {
	dnsInitOnce.Do(func() {
		DNSServices = svc
	})
}

// --- Forwarder Handlers ---

// ListDNSForwarders lists all DNS forwarders.
func ListDNSForwarders(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Forwarder == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	forwarders := DNSServices.Forwarder.GetForwarders()
	response.OK(w, forwarders)
}

// CreateDNSForwarder creates a new DNS forwarder.
func CreateDNSForwarder(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Protocol string `json:"protocol"`
		Address  string `json:"address"`
		Enabled  bool   `json:"enabled"`
		Priority int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" || req.Address == "" {
		response.BadRequest(w, "缺少名称和地址")
		return
	}

	// Validate address format (host:port).
	if _, _, err := net.SplitHostPort(req.Address); err != nil {
		response.BadRequest(w, "地址格式无效，应为 host:port 格式")
		return
	}

	// SSRF guard: unless dns.allow_private_upstream is enabled, upstreams may
	// not target loopback/private/link-local addresses.
	if !dnsAllowPrivateUpstream() {
		if err := validateUpstreamAddress(req.Address); err != nil {
			response.BadRequest(w, "无效的上游地址: "+err.Error())
			return
		}
	}

	if req.Protocol == "" {
		req.Protocol = "udp"
	}

	// Validate protocol.
	if !validProtocols[req.Protocol] {
		response.BadRequest(w, "无效的协议类型，支持: udp, tcp, dot, doh, doq")
		return
	}

	id := uuid.New().String()
	_, err := DNSServices.DB.Exec(`
		INSERT INTO dns_forwarders (id, name, protocol, address, enabled, priority)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, req.Name, req.Protocol, req.Address, req.Enabled, req.Priority)
	if err != nil {
		response.InternalErrorWithLog(w, "创建转发器失败", err)
		return
	}

	// Add to in-memory forwarder group.
	fwd := &forwarder.Forwarder{
		ID:       id,
		Name:     req.Name,
		Protocol: req.Protocol,
		Address:  req.Address,
		Enabled:  req.Enabled,
		Priority: req.Priority,
	}
	fwd.SetHealthy(true)
	if err := DNSServices.Forwarder.AddForwarder(fwd); err != nil {
		// Roll back DB insert so the persistent state matches the in-memory state.
		_, _ = DNSServices.DB.Exec("DELETE FROM dns_forwarders WHERE id = ?", id)
		// Don't leak parser/resolver internals to the client; log them in
		// detail so the operator can diagnose, but only return a generic
		// message.
		slog.Warn("create forwarder: invalid upstream address",
			"id", id, "name", req.Name, "address", req.Address, "error", err)
		response.BadRequest(w, "无效的上游地址")
		return
	}

	response.Created(w, map[string]string{"id": id, "name": req.Name})
}

// UpdateDNSForwarder updates a DNS forwarder.
func UpdateDNSForwarder(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少转发器ID")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Protocol string `json:"protocol"`
		Address  string `json:"address"`
		Enabled  bool   `json:"enabled"`
		Priority int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Validate address format (host:port).
	if req.Address != "" {
		if _, _, err := net.SplitHostPort(req.Address); err != nil {
			response.BadRequest(w, "地址格式无效，应为 host:port 格式")
			return
		}
		if !dnsAllowPrivateUpstream() {
			if err := validateUpstreamAddress(req.Address); err != nil {
				response.BadRequest(w, "无效的上游地址: "+err.Error())
				return
			}
		}
	}

	// Validate protocol.
	if req.Protocol != "" && !validProtocols[req.Protocol] {
		response.BadRequest(w, "无效的协议类型，支持: udp, tcp, dot, doh, doq")
		return
	}

	_, err := DNSServices.DB.Exec(`
		UPDATE dns_forwarders SET name=?, protocol=?, address=?, enabled=?, priority=?, updated_at=datetime('now')
		WHERE id=?
	`, req.Name, req.Protocol, req.Address, req.Enabled, req.Priority, id)
	if err != nil {
		response.InternalErrorWithLog(w, "更新转发器失败", err)
		return
	}

	// Update in-memory forwarder.
	DNSServices.Forwarder.RemoveForwarder(id)
	fwd := &forwarder.Forwarder{
		ID:       id,
		Name:     req.Name,
		Protocol: req.Protocol,
		Address:  req.Address,
		Enabled:  req.Enabled,
		Priority: req.Priority,
	}
	fwd.SetHealthy(true)
	if err := DNSServices.Forwarder.AddForwarder(fwd); err != nil {
		// Roll back the DB update so the persistent state matches the
		// in-memory state we just restored by RemoveForwarder.
		// We can't perfectly undo the change without knowing the prior
		// values, so we fall back to disabling the row as a safer state.
		slog.Warn("update forwarder: in-memory update failed; marking DB row disabled",
			"id", id, "name", req.Name, "address", req.Address, "error", err)
		if _, dbErr := DNSServices.DB.Exec(
			"UPDATE dns_forwarders SET enabled = 0, updated_at = datetime('now') WHERE id = ?",
			id,
		); dbErr != nil {
			slog.Error("update forwarder: failed to disable row in DB after rollback",
				"id", id, "error", dbErr)
		}
		slog.Warn("update forwarder: invalid upstream address",
			"id", id, "name", req.Name, "address", req.Address, "error", err)
		response.BadRequest(w, "无效的上游地址")
		return
	}

	response.OK(w, map[string]string{"id": id})
}

// DeleteDNSForwarder deletes a DNS forwarder.
func DeleteDNSForwarder(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少转发器ID")
		return
	}

	// Use a transaction so the reference check and the deletion observe a
	// consistent view of the database.
	tx, err := DNSServices.DB.Begin()
	if err != nil {
		response.InternalErrorWithLog(w, "删除转发器失败", err)
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Check if any conditional forwarder references this forwarder using a
	// JSON array query (json_each) so that we look for exact ID matches
	// instead of doing a naive LIKE substring search.
	var refCount int
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM dns_conditional_forwarders
		WHERE EXISTS (SELECT 1 FROM json_each(forwarder_ids) WHERE value = ?)
	`, id).Scan(&refCount)
	if err != nil {
		response.InternalErrorWithLog(w, "检查转发器引用失败", err)
		return
	}
	if refCount > 0 {
		response.BadRequest(w, fmt.Sprintf("该转发器仍被 %d 个条件转发器引用，无法删除", refCount))
		return
	}

	if _, err = tx.Exec("DELETE FROM dns_forwarders WHERE id=?", id); err != nil {
		response.InternalErrorWithLog(w, "删除转发器失败", err)
		return
	}

	if err := tx.Commit(); err != nil {
		response.InternalErrorWithLog(w, "删除转发器失败", err)
		return
	}

	DNSServices.Forwarder.RemoveForwarder(id)
	response.OK(w, map[string]string{"id": id})
}

// --- Conditional Forwarder Handlers ---

// ListConditionalForwarders lists all conditional forwarders.
func ListConditionalForwarders(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Conditional == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	conditionals := DNSServices.Conditional.GetConditionals()
	response.OK(w, conditionals)
}

// CreateConditionalForwarder creates a new conditional forwarder.
func CreateConditionalForwarder(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		Domain       string   `json:"domain"`
		ForwarderIDs []string `json:"forwarder_ids"`
		Enabled      bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Domain == "" || len(req.ForwarderIDs) == 0 {
		response.BadRequest(w, "缺少域名和转发器ID")
		return
	}

	// Validate that all forwarder_ids reference existing forwarders.
	for _, fid := range req.ForwarderIDs {
		var exists string
		err := DNSServices.DB.QueryRow(`SELECT id FROM dns_forwarders WHERE id = ?`, fid).Scan(&exists)
		if err != nil {
			response.BadRequest(w, fmt.Sprintf("转发器ID %s 不存在", fid))
			return
		}
	}

	id := uuid.New().String()
	idsJSON, _ := json.Marshal(req.ForwarderIDs)
	_, err := DNSServices.DB.Exec(`
		INSERT INTO dns_conditional_forwarders (id, domain, forwarder_ids, enabled)
		VALUES (?, ?, ?, ?)
	`, id, req.Domain, string(idsJSON), req.Enabled)
	if err != nil {
		response.InternalErrorWithLog(w, "创建条件转发器失败", err)
		return
	}

	// Add to in-memory manager.
	cf := &forwarder.ConditionalForwarder{
		ID:           id,
		Domain:       req.Domain,
		ForwarderIDs: req.ForwarderIDs,
		Enabled:      req.Enabled,
	}
	DNSServices.Conditional.AddConditional(cf)

	// Rebuild forwarder groups.
	allFwds := DNSServices.Forwarder.GetForwarders()
	DNSServices.Conditional.RebuildGroups(allFwds)

	response.Created(w, map[string]string{"id": id, "domain": req.Domain})
}

// UpdateConditionalForwarder updates a conditional forwarder.
func UpdateConditionalForwarder(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少条件转发器ID")
		return
	}

	var req struct {
		Domain       string   `json:"domain"`
		ForwarderIDs []string `json:"forwarder_ids"`
		Enabled      bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	idsJSON, _ := json.Marshal(req.ForwarderIDs)
	_, err := DNSServices.DB.Exec(`
		UPDATE dns_conditional_forwarders SET domain=?, forwarder_ids=?, enabled=?, updated_at=datetime('now')
		WHERE id=?
	`, req.Domain, string(idsJSON), req.Enabled, id)
	if err != nil {
		response.InternalErrorWithLog(w, "更新条件转发器失败", err)
		return
	}

	// Update in-memory.
	DNSServices.Conditional.RemoveConditional(id)
	cf := &forwarder.ConditionalForwarder{
		ID:           id,
		Domain:       req.Domain,
		ForwarderIDs: req.ForwarderIDs,
		Enabled:      req.Enabled,
	}
	DNSServices.Conditional.AddConditional(cf)

	allFwds := DNSServices.Forwarder.GetForwarders()
	DNSServices.Conditional.RebuildGroups(allFwds)

	response.OK(w, map[string]string{"id": id})
}

// DeleteConditionalForwarder deletes a conditional forwarder.
func DeleteConditionalForwarder(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, "缺少条件转发器ID")
		return
	}

	_, err := DNSServices.DB.Exec("DELETE FROM dns_conditional_forwarders WHERE id=?", id)
	if err != nil {
		response.InternalErrorWithLog(w, "删除条件转发器失败", err)
		return
	}

	DNSServices.Conditional.RemoveConditional(id)
	response.OK(w, map[string]string{"id": id})
}
