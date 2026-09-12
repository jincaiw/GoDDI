package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// T10: OpenAPI 3.0 documentation. The spec below is maintained as a
// structured route table and serialized on request; keeping it in code
// (rather than a static JSON file) lets route registrations and docs be
// reviewed in the same change.

// apiRouteDoc describes one documented API operation.
type apiRouteDoc struct {
	Path    string   `json:"-"`
	Method  string   `json:"-"`
	Tag     string   `json:"tag"`
	Summary string   `json:"summary"`
	Secure  bool     `json:"secure"`
	Params  []string `json:"params,omitempty"`
}

// apiQueryParam describes a query parameter precisely enough for generated
// clients to distinguish it from a required URL path component.
type apiQueryParam struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// apiOperationContract captures the high-value request and response details
// that cannot be inferred from a legacy route row alone.
type apiOperationContract struct {
	RequestContentType  string
	RequestDescription  string
	ResponseContentType string
	ResponseSchema      map[string]interface{}
	SuccessStatus       string
	SuccessDescription  string
}

// documentedQueryParams defines the high-traffic operations whose contracts
// need a precise query schema. Other legacy rows retain their existing path
// parameter declaration until their DTOs are documented.
var documentedOperationContracts = map[string]apiOperationContract{
	"POST /dns/zones/{id}/import": {
		RequestContentType: "multipart/form-data",
		RequestDescription: "Zone file upload. The file field contains BIND-compatible zone text.",
	},
	"GET /dns/zones/{id}/export": {
		ResponseContentType: "application/dns",
		ResponseSchema:      map[string]interface{}{"type": "string", "format": "binary"},
	},
	"GET /backup/{id}/download": {
		ResponseContentType: "application/octet-stream",
		ResponseSchema:      map[string]interface{}{"type": "string", "format": "binary"},
	},
	"POST /dns/zones/batch-delete": {
		RequestContentType: "application/json",
		RequestDescription: "JSON body containing the zone IDs to delete.",
	},
	"POST /dns/records/batch": {
		RequestContentType: "application/json",
		RequestDescription: "JSON body containing records to create.",
	},
	"DELETE /dns/records/batch": {
		RequestContentType: "application/json",
		RequestDescription: "JSON body containing record IDs to delete.",
	},
	"PUT /dns/forwarders/{id}": {
		RequestContentType: "application/json",
		RequestDescription: "Forwarder configuration update.",
	},
	"DELETE /dns/forwarders/{id}": {SuccessStatus: "204"},
	"PUT /dns/conditional-forwarders/{id}": {
		RequestContentType: "application/json",
		RequestDescription: "Conditional forwarder configuration update.",
	},
	"DELETE /dns/conditional-forwarders/{id}": {SuccessStatus: "204"},
	"GET /sso": {
		SuccessStatus:      "501",
		SuccessDescription: "预留扩展：当前版本不提供 SSO 配置",
	},
	"PUT /sso": {
		SuccessStatus:      "501",
		SuccessDescription: "预留扩展：当前版本不提供 SSO 配置",
	},
	"GET /cluster": {
		SuccessStatus:      "501",
		SuccessDescription: "预留扩展：当前版本不提供多节点集群协调",
	},
	"POST /cluster": {
		SuccessStatus:      "501",
		SuccessDescription: "预留扩展：当前版本不提供多节点集群协调",
	},
	"GET /apps": {
		SuccessStatus:      "501",
		SuccessDescription: "预留扩展：当前版本不提供应用市场运行时",
	},
	"POST /apps/{id}/install": {
		SuccessStatus:      "501",
		SuccessDescription: "预留扩展：当前版本不提供应用市场运行时",
	},
	"GET /dhcp/ha": {
		SuccessStatus:      "501",
		SuccessDescription: "预留扩展：当前版本不提供该配置 API",
	},
	"PUT /dns/listeners/dot": {
		RequestContentType: "application/json",
		RequestDescription: "DoT listener configuration.",
	},
	"PUT /dns/listeners/doh": {
		RequestContentType: "application/json",
		RequestDescription: "DoH listener configuration.",
	},
	"PUT /dns/listeners/doq": {
		RequestContentType: "application/json",
		RequestDescription: "DoQ listener configuration.",
	},
}

var documentedQueryParams = map[string][]apiQueryParam{
	"GET /logs/dns": {
		{Name: "client_ip", Type: "string", Description: "精确匹配客户端 IP"},
		{Name: "domain", Type: "string", Description: "查询域名模糊匹配"},
		{Name: "query_type", Type: "string"},
		{Name: "response_code", Type: "string"},
		{Name: "blocked", Type: "boolean"},
		{Name: "start_time", Type: "string", Description: "RFC3339 起始时间"},
		{Name: "end_time", Type: "string", Description: "RFC3339 结束时间"},
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /logs/dns/export": {
		{Name: "client_ip", Type: "string"},
		{Name: "domain", Type: "string"},
		{Name: "query_type", Type: "string"},
		{Name: "response_code", Type: "string"},
		{Name: "blocked", Type: "boolean"},
		{Name: "start_time", Type: "string", Description: "RFC3339 起始时间"},
		{Name: "end_time", Type: "string", Description: "RFC3339 结束时间"},
	},
	"GET /stats": {
		{Name: "range", Type: "string", Description: "hour、day、week、month、year 或 custom"},
		{Name: "start", Type: "string", Description: "custom 范围的 RFC3339 起始时间"},
		{Name: "end", Type: "string", Description: "custom 范围的 RFC3339 结束时间"},
	},
	"GET /stats/top": {
		{Name: "type", Type: "string", Required: true, Description: "clients、domains 或 blocked"},
		{Name: "range", Type: "string"},
		{Name: "start", Type: "string"},
		{Name: "end", Type: "string"},
		{Name: "limit", Type: "integer"},
	},
	"GET /dashboard/top": {
		{Name: "range", Type: "string"},
		{Name: "limit", Type: "integer"},
	},
	"GET /dns/zones": {
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
		{Name: "type", Type: "string", Description: "forward 或 reverse"},
		{Name: "name", Type: "string"},
	},
	"GET /dns/zones/{id}/history": {
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /dns/zones/{zoneId}/records": {
		{Name: "name", Type: "string"},
		{Name: "type", Type: "string"},
		{Name: "enabled", Type: "boolean"},
	},
	"GET /dns/cache/entries": {
		{Name: "qname", Type: "string"},
		{Name: "qtype", Type: "string"},
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /logs/audit": {
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /logs/dhcp": {
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /ipam/spaces": {
		{Name: "name", Type: "string"},
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /ipam/subnets": {
		{Name: "space_id", Type: "string"},
		{Name: "name", Type: "string"},
		{Name: "cidr", Type: "string"},
		{Name: "location", Type: "string"},
		{Name: "vlan_id", Type: "integer"},
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /ipam/addresses": {
		{Name: "subnet_id", Type: "string"},
		{Name: "space_id", Type: "string"},
		{Name: "status", Type: "string"},
		{Name: "observed_state", Type: "string"},
		{Name: "ip", Type: "string"},
		{Name: "mac", Type: "string"},
		{Name: "hostname", Type: "string"},
		{Name: "owner", Type: "string"},
		{Name: "search", Type: "string"},
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	// The 360 view is addressed by the pair that identifies an address, not by
	// an id: the caller usually has an IP and a space, and the row may not
	// exist yet.
	"GET /ipam/addresses/view": {
		{Name: "space_id", Type: "string", Required: true},
		{Name: "ip", Type: "string", Required: true},
	},
	"GET /ipam/export": {
		{Name: "type", Type: "string", Description: "addresses 或 subnets"},
		{Name: "format", Type: "string", Description: "csv（默认）或 json"},
		{Name: "parent_id", Type: "string"},
	},
	"GET /config/revisions": {
		{Name: "resource_type", Type: "string", Description: "dns_zone、dhcp_scope、ipam_subnet 或 dns_records"},
		{Name: "resource_id", Type: "string"},
		{Name: "status", Type: "string", Description: "persisted、staged、applied 或 failed"},
		{Name: "page", Type: "integer"},
		{Name: "page_size", Type: "integer"},
	},
	"GET /config/revisions/{id}": {
		{Name: "with_changes", Type: "boolean", Description: "true 时附带相对基准修订的字段级变更"},
	},
	"GET /config/diff": {
		{Name: "resource_type", Type: "string", Required: true},
		{Name: "resource_id", Type: "string", Required: true},
		{Name: "from", Type: "integer", Required: true},
		{Name: "to", Type: "integer", Required: true},
	},
	"GET /config/releases": {
		{Name: "limit", Type: "integer"},
	},
}

// routeDocs enumerates the public API surface of /api/v1.
var routeDocs = []apiRouteDoc{
	{"/auth/init", "POST", "Auth", "初始化管理员", false, nil},
	{"/auth/login", "POST", "Auth", "登录并获取会话令牌", false, nil},
	{"/auth/refresh", "POST", "Auth", "刷新访问令牌", false, nil},
	{"/auth/logout", "POST", "Auth", "登出", true, nil},
	{"/auth/me", "GET", "Auth", "当前用户信息", true, nil},
	{"/auth/change-password", "POST", "Auth", "修改密码", true, nil},
	{"/auth/sessions", "GET", "Auth", "会话列表", true, nil},
	{"/auth/sessions/{id}", "DELETE", "Auth", "删除会话", true, []string{"id"}},
	{"/auth/lockouts", "GET", "Auth", "登录锁定列表", true, nil},
	{"/auth/unlock", "POST", "Auth", "解锁被限速锁定的账号", true, nil},

	{"/users", "GET", "Users", "用户列表", true, nil},
	{"/users", "POST", "Users", "创建用户", true, nil},
	{"/users/{id}", "GET", "Users", "用户详情", true, []string{"id"}},
	{"/users/{id}", "PUT", "Users", "更新用户", true, []string{"id"}},
	{"/users/{id}", "DELETE", "Users", "删除用户", true, []string{"id"}},

	{"/roles", "GET", "Roles", "角色列表", true, nil},
	{"/roles", "POST", "Roles", "创建角色", true, nil},
	{"/roles/{id}", "PUT", "Roles", "更新角色", true, []string{"id"}},
	{"/roles/{id}", "DELETE", "Roles", "删除角色", true, []string{"id"}},

	{"/tokens", "GET", "Tokens", "API 令牌列表", true, nil},
	{"/tokens", "POST", "Tokens", "创建 API 令牌", true, nil},
	{"/tokens/{id}", "DELETE", "Tokens", "删除 API 令牌", true, []string{"id"}},

	{"/dashboard", "GET", "Dashboard", "仪表盘总览", true, nil},
	{"/dashboard/top", "GET", "Dashboard", "Top 统计（客户端/域名/阻断）", true, nil},
	{"/stats", "GET", "Dashboard", "查询统计（range: day/week/month/year）", true, nil},
	{"/stats/top", "GET", "Dashboard", "Top 统计（type: clients/domains/blocked）", true, nil},

	{"/dns/zones", "GET", "DNS Zones", "区域列表", true, nil},
	{"/dns/zones", "POST", "DNS Zones", "创建区域", true, nil},
	{"/dns/zones/batch-delete", "POST", "DNS Zones", "批量删除区域", true, nil},
	{"/dns/zones/{id}", "GET", "DNS Zones", "区域详情", true, []string{"id"}},
	{"/dns/zones/{id}", "PUT", "DNS Zones", "更新区域", true, []string{"id"}},
	{"/dns/zones/{id}", "DELETE", "DNS Zones", "删除区域", true, []string{"id"}},
	{"/dns/zones/{id}/import", "POST", "DNS Zones", "导入 Zone File", true, []string{"id"}},
	{"/dns/zones/{id}/export", "GET", "DNS Zones", "导出 Zone File", true, []string{"id"}},
	{"/dns/zones/{id}/sync", "POST", "DNS Zones", "Secondary 立即同步", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec", "GET", "DNSSEC", "DNSSEC 状态", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/enable", "POST", "DNSSEC", "启用 DNSSEC 签名", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/disable", "POST", "DNSSEC", "禁用 DNSSEC 签名", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/rotate", "POST", "DNSSEC", "轮换 DNSSEC 密钥", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/ds", "GET", "DNSSEC", "DS 记录（父区/注册商委托数据）", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/keys", "POST", "DNSSEC", "生成 KSK/ZSK 密钥", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/keys/promote", "POST", "DNSSEC", "晋升备用密钥（轮换第二阶段）", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/keys/{keyId}", "PUT", "DNSSEC", "启用/禁用单个密钥", true, []string{"id", "keyId"}},
	{"/dns/zones/{id}/dnssec/keys/{keyId}", "DELETE", "DNSSEC", "删除单个密钥", true, []string{"id", "keyId"}},
	{"/dns/zones/{id}/dnssec/nsec3", "GET", "DNSSEC", "NSEC3 参数查询", true, []string{"id"}},
	{"/dns/zones/{id}/dnssec/nsec3", "PUT", "DNSSEC", "NSEC3 参数设置", true, []string{"id"}},
	{"/dns/zones/{id}/enable", "POST", "DNS Zones", "启用区域", true, []string{"id"}},
	{"/dns/zones/{id}/disable", "POST", "DNS Zones", "禁用区域", true, []string{"id"}},
	{"/dns/zones/{id}/history", "GET", "DNS Zones", "区域变更历史", true, []string{"id"}},
	{"/dns/zones/{id}/permissions", "GET", "DNS Zones", "区域权限列表", true, []string{"id"}},
	{"/dns/zones/{id}/permissions", "PUT", "DNS Zones", "设置区域权限", true, []string{"id"}},
	{"/dns/zones/{id}/catalog/members", "GET", "DNS Zones", "Catalog 区域成员列表", true, []string{"id"}},
	{"/dns/security/allowlists/import", "POST", "DNS Security", "批量导入白名单规则（text/plain，每行 pattern [match_type]）", true, nil},
	{"/dns/security/allowlists/export", "GET", "DNS Security", "导出白名单规则", true, nil},
	{"/dns/security/allowlists/flush", "POST", "DNS Security", "清空白名单", true, nil},
	{"/dns/security/blocklists/flush", "POST", "DNS Security", "清空全部黑名单", true, nil},

	{"/dns/zones/{zoneId}/records", "GET", "DNS Records", "记录列表", true, []string{"zoneId"}},
	{"/dns/zones/{zoneId}/records", "POST", "DNS Records", "创建记录", true, []string{"zoneId"}},
	{"/dns/zones/{zoneId}/records/{id}", "GET", "DNS Records", "记录详情", true, []string{"zoneId", "id"}},
	{"/dns/zones/{zoneId}/records/{id}", "PUT", "DNS Records", "更新记录", true, []string{"zoneId", "id"}},
	{"/dns/zones/{zoneId}/records/{id}", "DELETE", "DNS Records", "删除记录", true, []string{"zoneId", "id"}},
	{"/dns/records/batch", "POST", "DNS Records", "批量创建记录", true, nil},
	{"/dns/records/batch", "DELETE", "DNS Records", "批量删除记录", true, nil},

	{"/dns/forwarders", "GET", "DNS Forwarders", "转发器列表", true, nil},
	{"/dns/forwarders", "POST", "DNS Forwarders", "创建转发器", true, nil},
	{"/dns/forwarders/{id}", "PUT", "DNS Forwarders", "更新转发器", true, []string{"id"}},
	{"/dns/forwarders/{id}", "DELETE", "DNS Forwarders", "删除转发器", true, []string{"id"}},
	{"/dns/conditional-forwarders", "GET", "DNS Forwarders", "条件转发器列表", true, nil},
	{"/dns/conditional-forwarders", "POST", "DNS Forwarders", "创建条件转发器", true, nil},
	{"/dns/conditional-forwarders/{id}", "PUT", "DNS Forwarders", "更新条件转发器", true, []string{"id"}},
	{"/dns/conditional-forwarders/{id}", "DELETE", "DNS Forwarders", "删除条件转发器", true, []string{"id"}},
	{"/dns/listeners/dot", "PUT", "DNS Listeners", "配置 DoT 监听器", true, nil},
	{"/dns/listeners/doh", "PUT", "DNS Listeners", "配置 DoH 监听器", true, nil},
	{"/dns/listeners/doq", "PUT", "DNS Listeners", "配置 DoQ 监听器", true, nil},

	{"/dns/cache", "GET", "DNS Cache", "缓存统计", true, nil},
	{"/dns/cache/entries", "GET", "DNS Cache", "缓存条目列表", true, nil},
	{"/dns/cache", "DELETE", "DNS Cache", "清空缓存", true, nil},
	{"/dns/cache/{name}/{type}", "DELETE", "DNS Cache", "刷新单条缓存", true, []string{"name", "type"}},

	{"/dns/client", "POST", "DNS Client", "DNS 诊断查询", true, nil},

	{"/dns/security/blocklists", "GET", "DNS Security", "阻断列表", true, nil},
	{"/dns/security/blocklists", "POST", "DNS Security", "创建阻断列表", true, nil},
	{"/dns/security/blocklists/{id}/refresh", "POST", "DNS Security", "立即刷新外部阻断列表", true, []string{"id"}},
	{"/dns/security/blocklists/{id}/rules", "GET", "DNS Security", "阻断规则列表", true, []string{"id"}},
	{"/dns/security/allowlists", "GET", "DNS Security", "白名单列表", true, nil},
	{"/dns/security/allowlists", "POST", "DNS Security", "添加白名单规则", true, nil},
	{"/dns/security/policies", "GET", "DNS Security", "客户端策略列表", true, nil},
	{"/dns/security/policies", "POST", "DNS Security", "创建客户端策略", true, nil},
	{"/dns/security/temporary-disable", "POST", "DNS Security", "临时禁用阻断", true, nil},
	{"/dns/security/blocking-status", "GET", "DNS Security", "阻断状态", true, nil},

	{"/logs/dns", "GET", "Logs", "DNS 查询日志", true, nil},
	{"/logs/dns/export", "GET", "Logs", "导出 DNS 查询日志 CSV", true, nil},
	{"/logs/audit", "GET", "Logs", "审计日志", true, nil},
	{"/logs/dhcp", "GET", "Logs", "DHCP 日志", true, nil},

	{"/dhcp/scopes", "GET", "DHCP", "Scope 列表", true, nil},
	{"/dhcp/leases", "GET", "DHCP", "租约列表", true, nil},
	{"/dhcp/reservations", "GET", "DHCP", "保留地址列表", true, nil},
	{"/dhcp/options", "GET", "DHCP", "选项列表", true, nil},

	// IPAM Spaces.
	{"/ipam/spaces", "GET", "IPAM", "地址空间列表", true, nil},
	{"/ipam/spaces", "POST", "IPAM", "创建地址空间", true, nil},
	{"/ipam/spaces/{id}", "GET", "IPAM", "地址空间详情", true, []string{"id"}},
	{"/ipam/spaces/{id}", "PUT", "IPAM", "更新地址空间", true, []string{"id"}},
	{"/ipam/spaces/{id}", "DELETE", "IPAM", "删除地址空间", true, []string{"id"}},

	// IPAM Subnets.
	{"/ipam/subnets", "GET", "IPAM", "子网列表", true, nil},
	{"/ipam/subnets", "POST", "IPAM", "创建子网", true, nil},
	{"/ipam/subnets/{id}", "GET", "IPAM", "子网详情", true, []string{"id"}},
	{"/ipam/subnets/{id}", "PUT", "IPAM", "更新子网", true, []string{"id"}},
	{"/ipam/subnets/{id}", "DELETE", "IPAM", "删除子网（有已分配地址或依赖时拒绝）", true, []string{"id"}},
	{"/ipam/subnets/{id}/stats", "GET", "IPAM", "子网用量统计", true, []string{"id"}},
	{"/ipam/subnets/{id}/dependencies", "GET", "IPAM", "子网依赖（阻塞删除的原因）", true, []string{"id"}},
	// A preview of a write, so it is guarded by write permission: the
	// fingerprint it returns is what the create call is checked against.
	{"/ipam/subnets/{id}/dhcp-scope-plan", "POST", "IPAM", "建池计划（只读，返回指纹）", true, []string{"id"}},
	{"/ipam/subnets/{id}/generate-dhcp-scope", "POST", "IPAM", "按计划创建 DHCP 作用域（需携带指纹）", true, []string{"id"}},
	{"/ipam/subnets/{id}/generate-reverse-zone", "POST", "IPAM", "创建反向区域", true, []string{"id"}},

	// IPAM Addresses.
	{"/ipam/addresses", "GET", "IPAM", "地址列表", true, nil},
	{"/ipam/addresses/view", "GET", "IPAM", "统一 IP 详情（地址 + DNS + DHCP + 占位）", true, nil},
	{"/ipam/addresses/{id}", "GET", "IPAM", "地址详情", true, []string{"id"}},
	{"/ipam/addresses/{id}", "PUT", "IPAM", "更新地址", true, []string{"id"}},
	{"/ipam/addresses/{id}/transition", "POST", "IPAM", "变更地址状态", true, []string{"id"}},
	{"/ipam/addresses/{id}/dns-links", "GET", "IPAM", "地址当前被哪些名字发布", true, []string{"id"}},
	{"/ipam/addresses/allocate", "POST", "IPAM", "分配 IP", true, nil},
	{"/ipam/addresses/release", "POST", "IPAM", "释放 IP", true, nil},

	// IPAM Import / Export.
	{"/ipam/import", "POST", "IPAM", "批量导入地址或子网", true, nil},
	{"/ipam/import/preview", "POST", "IPAM", "导入预演（只读，不写任何东西）", true, nil},
	{"/ipam/export", "GET", "IPAM", "导出地址或子网", true, nil},
	{"/ipam/integrity", "GET", "IPAM", "IPAM 一致性检查", true, nil},

	{"/settings", "GET", "Settings", "系统设置列表", true, nil},
	{"/settings", "PUT", "Settings", "批量更新设置（热生效）", true, nil},
	{"/settings/{key}", "PUT", "Settings", "更新单个设置（热生效）", true, []string{"key"}},

	// Reserved extension points. They are intentionally documented with their
	// actual 501 response so generated clients do not mistake them for live
	// enterprise capabilities.
	{"/sso", "GET", "Extensions", "SSO 配置（当前版本未实现）", true, nil},
	{"/sso", "PUT", "Extensions", "SSO 配置（当前版本未实现）", true, nil},
	{"/cluster", "GET", "Extensions", "多节点集群（当前版本未实现）", true, nil},
	{"/cluster", "POST", "Extensions", "多节点集群（当前版本未实现）", true, nil},
	{"/apps", "GET", "Extensions", "应用市场（当前版本未实现）", true, nil},
	{"/apps/{id}/install", "POST", "Extensions", "安装应用（当前版本未实现）", true, []string{"id"}},
	{"/dhcp/ha", "GET", "Extensions", "DHCP HA 配置 API（当前版本未实现）", true, nil},

	// Configuration publishing: revision history, diff, rollback and the
	// release queue. Guarded by the settings permission rather than a
	// domain-specific one, because it can publish DNS, DHCP and IPAM
	// configuration in the same call.
	{"/config/types", "GET", "Config Versions", "可发布配置的资源类型", true, nil},
	{"/config/revisions", "GET", "Config Versions", "配置修订列表", true, nil},
	{"/config/revisions/{id}", "GET", "Config Versions", "单个修订（可选带字段级变更）", true, []string{"id"}},
	{"/config/diff", "GET", "Config Versions", "两个修订之间的字段级差异", true, nil},
	{"/config/publish", "POST", "Config Versions", "发布配置（产生新修订并排队下发）", true, nil},
	{"/config/rollback", "POST", "Config Versions", "回滚到历史修订（作为新修订发布）", true, nil},
	{"/config/releases", "GET", "Config Versions", "数据面发布队列", true, nil},
	{"/config/releases/retry", "POST", "Config Versions", "重置失败的发布项以便重试", true, nil},

	{"/backup", "GET", "Backup", "备份列表", true, nil},
	{"/backup", "POST", "Backup", "创建备份", true, nil},
	{"/backup/{id}", "GET", "Backup", "备份详情", true, []string{"id"}},
	{"/backup/{id}", "DELETE", "Backup", "删除备份", true, []string{"id"}},
	{"/backup/{id}/download", "GET", "Backup", "下载备份文件", true, []string{"id"}},
	{"/backup/{id}/restore", "POST", "Backup", "恢复备份", true, []string{"id"}},
}

// OpenAPIHandler serves the generated OpenAPI 3.0 specification.
func OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	paths := map[string]interface{}{}
	for _, rd := range routeDocs {
		p, ok := paths[rd.Path].(map[string]interface{})
		if !ok {
			p = map[string]interface{}{}
			paths[rd.Path] = p
		}

		paramObjs := make([]interface{}, 0, len(rd.Params))
		for _, name := range rd.Params {
			paramObjs = append(paramObjs, map[string]interface{}{
				"name":     name,
				"in":       "path",
				"required": true,
				"schema":   map[string]string{"type": "string"},
			})
		}
		key := rd.Method + " " + rd.Path
		for _, qp := range documentedQueryParams[key] {
			obj := map[string]interface{}{
				"name":     qp.Name,
				"in":       "query",
				"required": qp.Required,
				"schema":   map[string]string{"type": qp.Type},
			}
			if qp.Description != "" {
				obj["description"] = qp.Description
			}
			paramObjs = append(paramObjs, obj)
		}

		contentType := "application/json"
		contentSchema := map[string]interface{}{"$ref": "#/components/schemas/ApiResponse"}
		if key == "GET /logs/dns/export" {
			contentType = "text/csv"
			contentSchema = map[string]interface{}{"type": "string", "format": "binary"}
		}
		contract := documentedOperationContracts[key]
		if contract.ResponseContentType != "" {
			contentType = contract.ResponseContentType
		}
		if contract.ResponseSchema != nil {
			contentSchema = contract.ResponseSchema
		}
		successStatus := contract.SuccessStatus
		if successStatus == "" {
			successStatus = "200"
		}
		successDescription := contract.SuccessDescription
		if successDescription == "" {
			successDescription = "成功"
		}
		responses := map[string]interface{}{
			successStatus: map[string]interface{}{
				"description": successDescription,
				"content": map[string]interface{}{
					contentType: map[string]interface{}{"schema": contentSchema},
				},
			},
			"400": map[string]interface{}{"description": "请求参数无效"},
			"500": map[string]interface{}{"description": "服务端错误"},
		}
		if successStatus == "204" {
			responses[successStatus] = map[string]interface{}{"description": successDescription}
		}
		op := map[string]interface{}{
			"tags":        []string{rd.Tag},
			"summary":     rd.Summary,
			"operationId": operationID(rd.Method, rd.Path),
			"responses":   responses,
		}
		if contract.RequestContentType != "" {
			schema := map[string]interface{}{"type": "object"}
			if contract.RequestContentType == "multipart/form-data" {
				schema = map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file": map[string]interface{}{"type": "string", "format": "binary"},
					},
					"required": []string{"file"},
				}
			}
			op["requestBody"] = map[string]interface{}{
				"required":    true,
				"description": contract.RequestDescription,
				"content": map[string]interface{}{
					contract.RequestContentType: map[string]interface{}{"schema": schema},
				},
			}
		}
		if len(paramObjs) > 0 {
			op["parameters"] = paramObjs
		}
		if rd.Secure {
			op["security"] = []interface{}{map[string]interface{}{"bearerAuth": []interface{}{}}}
		}

		p[lower(rd.Method)] = op
	}

	spec := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "GoDDI API",
			"description": "GoDDI DDI 平台 REST API（DNS + DHCP + IPAM）。控制台所有操作均通过本 API 完成。",
			"version":     "1.0.0",
		},
		"servers": []map[string]string{
			{"url": "/api/v1"},
		},
		"paths": paths,
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
					"description":  "登录获取的访问令牌，或长期 API Token。",
				},
			},
			"schemas": map[string]interface{}{
				"ApiResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"code":    map[string]string{"type": "integer", "example": "0"},
						"message": map[string]string{"type": "string", "example": "ok"},
						"data":    map[string]string{"type": "object"},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(spec)
}

// operationID converts an HTTP method and URI template into a stable,
// generator-safe OpenAPI identifier.
func operationID(method, path string) string {
	name := strings.ToLower(method) + "_" + strings.Trim(path, "/")
	name = strings.NewReplacer("/", "_", "{", "by_", "}", "", "-", "_").Replace(name)
	return strings.Trim(name, "_")
}

func lower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}
