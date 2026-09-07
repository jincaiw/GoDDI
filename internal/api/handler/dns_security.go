package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/api/response"
	"github.com/jasonwa/goddi/internal/dns/filter"
)

// validBlockListTypes contains the set of allowed block list type values.
var validBlockListTypes = map[string]bool{
	"custom":   true,
	"external": true,
}

// --- Block List Handlers ---

// validMatchTypes contains the set of allowed match_type values.
var validMatchTypes = map[string]bool{
	"exact":    true,
	"suffix":   true,
	"wildcard": true,
	"regex":    true,
}

// validResponseTypes contains the set of allowed response_type values.
var validResponseTypes = map[string]bool{
	"NXDOMAIN":  true,
	"NODATA":    true,
	"REFUSED":   true,
	"CUSTOM_IP": true,
	"DROP":      true,
}

// validActions contains the set of allowed client policy action values.
var validActions = map[string]bool{
	"allow":       true,
	"block":       true,
	"apply_lists": true,
}

// maxBlockPatternLength caps the length of patterns (including regexes) to
// mitigate ReDoS and memory amplification attacks.
const maxBlockPatternLength = 256

// ListBlockLists lists all DNS block lists.
func ListBlockLists(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	lists := DNSServices.Filter.BlockListMgr.ListLists()
	response.OK(w, lists)
}

// CreateBlockList creates a new DNS block list.
func CreateBlockList(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		Name string `json:"name"`
		Type string `json:"type"` // custom, external
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "缺少名称")
		return
	}

	if req.Type != "" && !validBlockListTypes[req.Type] {
		response.BadRequest(w, "无效的黑名单类型，支持: custom, external")
		return
	}

	// Validate any supplied URL, not only for type=external: the URL is
	// fetched by the server on refresh, so an internal address would be an
	// SSRF primitive regardless of how the list is labelled.
	if req.URL != "" {
		if err := validateExternalURL(req.URL); err != nil {
			response.BadRequest(w, "无效的URL: "+err.Error())
			return
		}
	}

	id := uuid.New().String()
	_, err := DNSServices.DB.Exec(`
		INSERT INTO dns_block_lists (id, name, type, url, enabled)
		VALUES (?, ?, ?, ?, ?)
	`, id, req.Name, req.Type, req.URL, true)
	if err != nil {
		response.InternalErrorWithLog(w, "创建黑名单失败", err)
		return
	}

	DNSServices.Filter.BlockListMgr.AddList(&filter.BlockList{
		ID:      id,
		Name:    req.Name,
		Type:    req.Type,
		URL:     req.URL,
		Enabled: true,
	})

	response.Created(w, map[string]string{"id": id, "name": req.Name})
}

// validateExternalURL validates the URL of an external block list, ensuring it
// is a syntactically valid http or https URL that does not point at internal
// address space (SSRF).
func validateExternalURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errInvalidURLScheme
	}
	if u.Host == "" {
		return errInvalidURLHost
	}
	return filter.AssertPublicURL(rawURL)
}

var (
	errInvalidURLScheme = &urlError{message: "URL scheme must be http or https"}
	errInvalidURLHost   = &urlError{message: "URL host is required"}
)

type urlError struct{ message string }

func (e *urlError) Error() string { return e.message }

// GetBlockList gets a DNS block list by ID.
func GetBlockList(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	list, ok := DNSServices.Filter.BlockListMgr.GetList(id)
	if !ok {
		response.NotFound(w, "黑名单未找到")
		return
	}

	rules := DNSServices.Filter.BlockListMgr.GetRules(id)
	response.OK(w, map[string]interface{}{
		"list":  list,
		"rules": rules,
	})
}

// UpdateBlockList updates a DNS block list.
func UpdateBlockList(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		URL     string `json:"url"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Validate the type and URL fields to prevent storing arbitrary data
	// that other components (e.g. a fetcher) might blindly consume.
	if req.Type != "" && !validBlockListTypes[req.Type] {
		response.BadRequest(w, "无效的黑名单类型，支持: custom, external")
		return
	}
	if req.URL != "" {
		if err := validateExternalURL(req.URL); err != nil {
			response.BadRequest(w, "无效的URL: "+err.Error())
			return
		}
	}

	_, err := DNSServices.DB.Exec(`
		UPDATE dns_block_lists SET name=?, type=?, url=?, enabled=?, updated_at=datetime('now')
		WHERE id=?
	`, req.Name, req.Type, req.URL, req.Enabled, id)
	if err != nil {
		response.InternalErrorWithLog(w, "更新黑名单失败", err)
		return
	}

	DNSServices.Filter.BlockListMgr.UpdateList(&filter.BlockList{
		ID:      id,
		Name:    req.Name,
		Type:    req.Type,
		URL:     req.URL,
		Enabled: req.Enabled,
	})

	response.OK(w, map[string]string{"id": id})
}

// DeleteBlockList deletes a DNS block list.
func DeleteBlockList(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")

	tx, err := DNSServices.DB.Begin()
	if err != nil {
		response.InternalErrorWithLog(w, "删除黑名单失败", err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM dns_block_rules WHERE list_id=?", id); err != nil {
		response.InternalErrorWithLog(w, "删除黑名单规则失败", err)
		return
	}
	if _, err := tx.Exec("DELETE FROM dns_block_lists WHERE id=?", id); err != nil {
		response.InternalErrorWithLog(w, "删除黑名单失败", err)
		return
	}

	if err := tx.Commit(); err != nil {
		response.InternalErrorWithLog(w, "删除黑名单失败", err)
		return
	}

	DNSServices.Filter.BlockListMgr.RemoveList(id)
	response.OK(w, map[string]string{"id": id})
}

// --- Block Rule Handlers ---

// ListBlockRules lists rules in a block list.
func ListBlockRules(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	listID := chi.URLParam(r, "id")
	rules := DNSServices.Filter.BlockListMgr.GetRules(listID)
	response.OK(w, rules)
}

// AddBlockRule adds a rule to a block list.
func AddBlockRule(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	listID := chi.URLParam(r, "id")
	var req struct {
		Pattern      string `json:"pattern"`
		MatchType    string `json:"match_type"`    // exact, suffix, wildcard, regex
		ResponseType string `json:"response_type"` // NXDOMAIN, NODATA, REFUSED, CUSTOM_IP, DROP
		ResponseData string `json:"response_data"`
		Enabled      bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Pattern == "" {
		response.BadRequest(w, "缺少匹配模式")
		return
	}

	// Cap the pattern length to mitigate ReDoS and memory amplification.
	if len(req.Pattern) > maxBlockPatternLength {
		response.BadRequest(w, "匹配模式过长，最大允许 256 字节")
		return
	}

	if req.MatchType == "" {
		req.MatchType = "suffix"
	}
	if req.ResponseType == "" {
		req.ResponseType = "NXDOMAIN"
	}

	// Validate match_type.
	if !validMatchTypes[req.MatchType] {
		response.BadRequest(w, "无效的匹配类型")
		return
	}

	// Validate regex pattern if match_type is regex. We use MustCompile (which
	// panics only on programmer error) only as a syntactic check; the actual
	// matching is done with MatchString, which does not expose backtracking
	// semantics to the caller.
	if req.MatchType == "regex" {
		re, err := regexp.Compile(req.Pattern)
		if err != nil {
			response.BadRequest(w, "无效的正则表达式: "+err.Error())
			return
		}
		// A no-op use of the compiled expression to keep the compiler honest
		// and to ensure we do not discard the compile result. The actual
		// matching happens later inside the filter engine.
		_ = re
	}

	// Validate response_type.
	if !validResponseTypes[req.ResponseType] {
		response.BadRequest(w, "无效的响应类型")
		return
	}

	id := uuid.New().String()
	_, err := DNSServices.DB.Exec(`
		INSERT INTO dns_block_rules (id, list_id, pattern, match_type, response_type, response_data, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, listID, req.Pattern, req.MatchType, req.ResponseType, req.ResponseData, req.Enabled)
	if err != nil {
		response.InternalErrorWithLog(w, "添加黑名单规则失败", err)
		return
	}

	rule := filter.MatchRule{
		ID:           id,
		ListID:       listID,
		Pattern:      req.Pattern,
		MatchType:    req.MatchType,
		ResponseType: req.ResponseType,
		ResponseData: req.ResponseData,
		Enabled:      req.Enabled,
	}
	DNSServices.Filter.BlockListMgr.AddRule(listID, rule)

	response.Created(w, map[string]string{"id": id, "pattern": req.Pattern})
}

// DeleteBlockRule deletes a block rule.
func DeleteBlockRule(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	listID := chi.URLParam(r, "listId")
	ruleID := chi.URLParam(r, "ruleId")

	_, err := DNSServices.DB.Exec("DELETE FROM dns_block_rules WHERE id=? AND list_id=?", ruleID, listID)
	if err != nil {
		response.InternalErrorWithLog(w, "删除黑名单规则失败", err)
		return
	}

	DNSServices.Filter.BlockListMgr.RemoveRule(listID, ruleID)
	response.OK(w, map[string]string{"id": ruleID})
}

// --- Allow List Handlers ---

// ListAllowRules lists all allow rules.
func ListAllowRules(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	rules := DNSServices.Filter.AllowListMgr.GetRules()
	response.OK(w, rules)
}

// AddAllowRule adds an allow rule.
func AddAllowRule(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		Pattern   string `json:"pattern"`
		MatchType string `json:"match_type"` // exact, suffix, wildcard, regex
		Enabled   bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Pattern == "" {
		response.BadRequest(w, "缺少匹配模式")
		return
	}

	// Cap pattern length to mitigate ReDoS / amplification.
	if len(req.Pattern) > maxBlockPatternLength {
		response.BadRequest(w, "匹配模式过长，最大允许 256 字节")
		return
	}

	if req.MatchType == "" {
		req.MatchType = "exact"
	}

	if !validMatchTypes[req.MatchType] {
		response.BadRequest(w, "无效的匹配类型")
		return
	}

	if req.MatchType == "regex" {
		re, err := regexp.Compile(req.Pattern)
		if err != nil {
			response.BadRequest(w, "无效的正则表达式: "+err.Error())
			return
		}
		_ = re
	}

	id := uuid.New().String()
	_, err := DNSServices.DB.Exec(`
		INSERT INTO dns_allow_rules (id, pattern, match_type, enabled)
		VALUES (?, ?, ?, ?)
	`, id, req.Pattern, req.MatchType, req.Enabled)
	if err != nil {
		response.InternalErrorWithLog(w, "添加白名单规则失败", err)
		return
	}

	rule := filter.MatchRule{
		ID:        id,
		Pattern:   req.Pattern,
		MatchType: req.MatchType,
		Enabled:   req.Enabled,
	}
	DNSServices.Filter.AllowListMgr.AddRule(rule)

	response.Created(w, map[string]string{"id": id, "pattern": req.Pattern})
}

// DeleteAllowRule deletes an allow rule.
func DeleteAllowRule(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	_, err := DNSServices.DB.Exec("DELETE FROM dns_allow_rules WHERE id=?", id)
	if err != nil {
		response.InternalErrorWithLog(w, "删除白名单规则失败", err)
		return
	}

	DNSServices.Filter.AllowListMgr.RemoveRule(id)
	response.OK(w, map[string]string{"id": id})
}

// --- Client Policy Handlers ---

// ListClientPolicies lists all DNS client policies.
func ListClientPolicies(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.Filter == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	policies := DNSServices.Filter.PolicyMgr.GetPolicies()
	response.OK(w, policies)
}

// CreateClientPolicy creates a new DNS client policy.
func CreateClientPolicy(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	var req struct {
		Name         string   `json:"name"`
		SourceCIDR   string   `json:"source_cidr"`
		Action       string   `json:"action"` // allow, block, apply_lists
		BlockListIDs []string `json:"block_list_ids"`
		AllowRuleIDs []string `json:"allow_rule_ids"`
		Priority     int      `json:"priority"`
		Enabled      bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	if req.Name == "" || req.SourceCIDR == "" || req.Action == "" {
		response.BadRequest(w, "缺少名称、源CIDR和操作")
		return
	}

	// Validate action.
	if !validActions[req.Action] {
		response.BadRequest(w, "无效的操作类型，支持: allow, block, apply_lists")
		return
	}

	// Validate source_cidr.
	if _, _, err := net.ParseCIDR(req.SourceCIDR); err != nil {
		response.BadRequest(w, "无效的CIDR格式")
		return
	}

	// Validate that all referenced block list IDs exist.
	for _, bid := range req.BlockListIDs {
		var exists int
		err := DNSServices.DB.QueryRow(`SELECT COUNT(*) FROM dns_block_lists WHERE id = ?`, bid).Scan(&exists)
		if err != nil {
			response.InternalErrorWithLog(w, "校验黑名单ID失败", err)
			return
		}
		if exists == 0 {
			response.BadRequest(w, "引用的黑名单ID不存在: "+bid)
			return
		}
	}
	// Validate that all referenced allow rule IDs exist.
	for _, aid := range req.AllowRuleIDs {
		var exists int
		err := DNSServices.DB.QueryRow(`SELECT COUNT(*) FROM dns_allow_rules WHERE id = ?`, aid).Scan(&exists)
		if err != nil {
			response.InternalErrorWithLog(w, "校验白名单规则ID失败", err)
			return
		}
		if exists == 0 {
			response.BadRequest(w, "引用的白名单规则ID不存在: "+aid)
			return
		}
	}

	id := uuid.New().String()
	blockIDsJSON, _ := json.Marshal(req.BlockListIDs)
	allowIDsJSON, _ := json.Marshal(req.AllowRuleIDs)

	_, err := DNSServices.DB.Exec(`
		INSERT INTO dns_client_policies (id, name, source_cidr, action, block_list_ids, allow_rule_ids, priority, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.Name, req.SourceCIDR, req.Action, string(blockIDsJSON), string(allowIDsJSON), req.Priority, req.Enabled)
	if err != nil {
		response.InternalErrorWithLog(w, "创建客户端策略失败", err)
		return
	}

	policy := &filter.ClientPolicy{
		ID:           id,
		Name:         req.Name,
		SourceCIDR:   req.SourceCIDR,
		Action:       req.Action,
		BlockListIDs: req.BlockListIDs,
		AllowRuleIDs: req.AllowRuleIDs,
		Priority:     req.Priority,
		Enabled:      req.Enabled,
	}

	if err := DNSServices.Filter.PolicyMgr.AddPolicy(policy); err != nil {
		// Log a warning so operators know the in-memory state and DB state
		// have diverged; rollback the DB row to keep them aligned.
		slog.Warn("create_client_policy: in-memory add failed; rolling back DB", "policy_id", id, "error", err)
		if _, delErr := DNSServices.DB.Exec(`DELETE FROM dns_client_policies WHERE id = ?`, id); delErr != nil {
			slog.Error("create_client_policy: failed to roll back DB", "policy_id", id, "error", delErr)
		}
		response.BadRequest(w, "无效的CIDR: "+err.Error())
		return
	}

	response.Created(w, map[string]string{"id": id, "name": req.Name})
}

// UpdateClientPolicy updates a DNS client policy.
func UpdateClientPolicy(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Name         string   `json:"name"`
		SourceCIDR   string   `json:"source_cidr"`
		Action       string   `json:"action"`
		BlockListIDs []string `json:"block_list_ids"`
		AllowRuleIDs []string `json:"allow_rule_ids"`
		Priority     int      `json:"priority"`
		Enabled      bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "无效的请求数据")
		return
	}

	// Validate action.
	if req.Action != "" && !validActions[req.Action] {
		response.BadRequest(w, "无效的操作类型，支持: allow, block, apply_lists")
		return
	}

	// Validate source_cidr.
	if req.SourceCIDR != "" {
		if _, _, err := net.ParseCIDR(req.SourceCIDR); err != nil {
			response.BadRequest(w, "无效的CIDR格式")
			return
		}
	}

	blockIDsJSON, _ := json.Marshal(req.BlockListIDs)
	allowIDsJSON, _ := json.Marshal(req.AllowRuleIDs)

	_, err := DNSServices.DB.Exec(`
		UPDATE dns_client_policies SET name=?, source_cidr=?, action=?, block_list_ids=?, allow_rule_ids=?, priority=?, enabled=?, updated_at=datetime('now')
		WHERE id=?
	`, req.Name, req.SourceCIDR, req.Action, string(blockIDsJSON), string(allowIDsJSON), req.Priority, req.Enabled, id)
	if err != nil {
		response.InternalErrorWithLog(w, "更新客户端策略失败", err)
		return
	}

	policy := &filter.ClientPolicy{
		ID:           id,
		Name:         req.Name,
		SourceCIDR:   req.SourceCIDR,
		Action:       req.Action,
		BlockListIDs: req.BlockListIDs,
		AllowRuleIDs: req.AllowRuleIDs,
		Priority:     req.Priority,
		Enabled:      req.Enabled,
	}

	if err := DNSServices.Filter.PolicyMgr.UpdatePolicy(policy); err != nil {
		// We cannot fully roll back the DB update without first reading
		// the prior row, so we log a warning and disable the policy in
		// the DB to keep it from being served. The in-memory state will
		// be refreshed from the DB on the next service reload.
		slog.Warn("update_client_policy: in-memory update failed; marking DB row disabled",
			"policy_id", id, "error", err)
		if _, dbErr := DNSServices.DB.Exec(
			"UPDATE dns_client_policies SET enabled = 0, updated_at = datetime('now') WHERE id = ?",
			id,
		); dbErr != nil {
			slog.Error("update_client_policy: failed to disable row in DB after rollback",
				"policy_id", id, "error", dbErr)
		}
		response.BadRequest(w, "无效的CIDR: "+err.Error())
		return
	}

	response.OK(w, map[string]string{"id": id})
}

// DeleteClientPolicy deletes a DNS client policy.
func DeleteClientPolicy(w http.ResponseWriter, r *http.Request) {
	if DNSServices == nil || DNSServices.DB == nil {
		response.InternalError(w, "DNS服务未初始化")
		return
	}

	id := chi.URLParam(r, "id")
	_, err := DNSServices.DB.Exec("DELETE FROM dns_client_policies WHERE id=?", id)
	if err != nil {
		response.InternalErrorWithLog(w, "删除客户端策略失败", err)
		return
	}

	DNSServices.Filter.PolicyMgr.RemovePolicy(id)
	response.OK(w, map[string]string{"id": id})
}

// LoadBlockListsFromDB loads block lists and rules from the database into the filter engine.
func LoadBlockListsFromDB(db *sql.DB, filterEngine *filter.FilterEngine) error {
	// Load block lists (including URL subscription fetch status).
	rows, err := db.Query(`
		SELECT id, name, type, COALESCE(url, ''), enabled, entry_count,
			COALESCE(last_updated, ''), COALESCE(last_fetch_at, ''),
			COALESCE(last_fetch_status, ''), COALESCE(last_fetch_error, '')
		FROM dns_block_lists
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lists []*filter.BlockList
	rulesMap := make(map[string][]filter.MatchRule)

	for rows.Next() {
		var list filter.BlockList
		if err := rows.Scan(&list.ID, &list.Name, &list.Type, &list.URL, &list.Enabled, &list.EntryCount,
			&list.LastUpdated, &list.LastFetchAt, &list.LastFetchStatus, &list.LastFetchError); err != nil {
			slog.Error("load_block_lists: failed to scan block list row", "error", err)
			continue
		}
		lists = append(lists, &list)
	}
	if err := rows.Err(); err != nil {
		slog.Error("load_block_lists: failed to iterate block lists", "error", err)
		return err
	}

	// Load block rules.
	ruleRows, err := db.Query("SELECT id, list_id, pattern, match_type, response_type, response_data, enabled FROM dns_block_rules")
	if err != nil {
		return err
	}
	defer ruleRows.Close()

	for ruleRows.Next() {
		var rule filter.MatchRule
		var listID string
		if err := ruleRows.Scan(&rule.ID, &listID, &rule.Pattern, &rule.MatchType, &rule.ResponseType, &rule.ResponseData, &rule.Enabled); err != nil {
			slog.Error("load_block_lists: failed to scan block rule row", "error", err)
			continue
		}
		rulesMap[listID] = append(rulesMap[listID], rule)
	}
	if err := ruleRows.Err(); err != nil {
		slog.Error("load_block_lists: failed to iterate block rules", "error", err)
		return err
	}

	filterEngine.BlockListMgr.Reload(lists, rulesMap)
	return nil
}

// LoadAllowRulesFromDB loads allow rules from the database.
func LoadAllowRulesFromDB(db *sql.DB, filterEngine *filter.FilterEngine) error {
	rows, err := db.Query("SELECT id, pattern, match_type, enabled FROM dns_allow_rules")
	if err != nil {
		return err
	}
	defer rows.Close()

	var rules []filter.MatchRule
	for rows.Next() {
		var rule filter.MatchRule
		if err := rows.Scan(&rule.ID, &rule.Pattern, &rule.MatchType, &rule.Enabled); err != nil {
			slog.Error("load_allow_rules: failed to scan allow rule row", "error", err)
			continue
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		slog.Error("load_allow_rules: failed to iterate allow rules", "error", err)
		return err
	}

	filterEngine.AllowListMgr.Reload(rules)
	return nil
}

// LoadClientPoliciesFromDB loads client policies from the database.
func LoadClientPoliciesFromDB(db *sql.DB, filterEngine *filter.FilterEngine) error {
	rows, err := db.Query("SELECT id, name, source_cidr, action, block_list_ids, allow_rule_ids, priority, enabled FROM dns_client_policies")
	if err != nil {
		return err
	}
	defer rows.Close()

	var policies []*filter.ClientPolicy
	for rows.Next() {
		var p filter.ClientPolicy
		var blockIDsJSON, allowIDsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.SourceCIDR, &p.Action, &blockIDsJSON, &allowIDsJSON, &p.Priority, &p.Enabled); err != nil {
			slog.Error("load_client_policies: failed to scan client policy row", "error", err)
			continue
		}

		// Parse JSON arrays.
		if blockIDsJSON != "" && blockIDsJSON != "null" {
			if err := json.NewDecoder(strings.NewReader(blockIDsJSON)).Decode(&p.BlockListIDs); err != nil {
				slog.Error("load_client_policies: failed to decode block_list_ids", "policy_id", p.ID, "error", err)
			}
		}
		if allowIDsJSON != "" && allowIDsJSON != "null" {
			if err := json.NewDecoder(strings.NewReader(allowIDsJSON)).Decode(&p.AllowRuleIDs); err != nil {
				slog.Error("load_client_policies: failed to decode allow_rule_ids", "policy_id", p.ID, "error", err)
			}
		}

		policies = append(policies, &p)
	}
	if err := rows.Err(); err != nil {
		slog.Error("load_client_policies: failed to iterate client policies", "error", err)
		return err
	}

	filterEngine.PolicyMgr.Reload(policies)
	return nil
}
