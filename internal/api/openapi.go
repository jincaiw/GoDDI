package api

import (
	"encoding/json"
	"net/http"
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
	{"/dashboard/top", "GET", "Dashboard", "Top 统计（客户端/域名/阻断）", true, []string{"range", "limit"}},

	{"/dns/zones", "GET", "DNS Zones", "区域列表", true, []string{"page", "page_size", "type", "name"}},
	{"/dns/zones", "POST", "DNS Zones", "创建区域", true, nil},
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

	{"/dns/zones/{zoneId}/records", "GET", "DNS Records", "记录列表", true, []string{"zoneId", "name", "type", "enabled"}},
	{"/dns/zones/{zoneId}/records", "POST", "DNS Records", "创建记录", true, []string{"zoneId"}},
	{"/dns/zones/{zoneId}/records/{id}", "GET", "DNS Records", "记录详情", true, []string{"zoneId", "id"}},
	{"/dns/zones/{zoneId}/records/{id}", "PUT", "DNS Records", "更新记录", true, []string{"zoneId", "id"}},
	{"/dns/zones/{zoneId}/records/{id}", "DELETE", "DNS Records", "删除记录", true, []string{"zoneId", "id"}},
	{"/dns/records/batch", "POST", "DNS Records", "批量创建记录", true, nil},
	{"/dns/records/batch", "DELETE", "DNS Records", "批量删除记录", true, nil},

	{"/dns/forwarders", "GET", "DNS Forwarders", "转发器列表", true, nil},
	{"/dns/forwarders", "POST", "DNS Forwarders", "创建转发器", true, nil},
	{"/dns/conditional-forwarders", "GET", "DNS Forwarders", "条件转发器列表", true, nil},
	{"/dns/conditional-forwarders", "POST", "DNS Forwarders", "创建条件转发器", true, nil},

	{"/dns/cache", "GET", "DNS Cache", "缓存统计", true, nil},
	{"/dns/cache/entries", "GET", "DNS Cache", "缓存条目列表", true, []string{"qname", "qtype", "page", "page_size"}},
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

	{"/logs/dns", "GET", "Logs", "DNS 查询日志", true, []string{"client_ip", "domain", "blocked", "page"}},
	{"/logs/audit", "GET", "Logs", "审计日志", true, []string{"page"}},
	{"/logs/dhcp", "GET", "Logs", "DHCP 日志", true, []string{"page"}},

	{"/dhcp/scopes", "GET", "DHCP", "Scope 列表", true, nil},
	{"/dhcp/leases", "GET", "DHCP", "租约列表", true, nil},
	{"/dhcp/reservations", "GET", "DHCP", "保留地址列表", true, nil},
	{"/dhcp/options", "GET", "DHCP", "选项列表", true, nil},

	{"/ipam/spaces", "GET", "IPAM", "地址空间列表", true, nil},
	{"/ipam/subnets", "GET", "IPAM", "子网列表", true, nil},
	{"/ipam/addresses", "GET", "IPAM", "地址列表", true, nil},

	{"/settings", "GET", "Settings", "系统设置列表", true, nil},
	{"/settings", "PUT", "Settings", "批量更新设置（热生效）", true, nil},
	{"/settings/{key}", "PUT", "Settings", "更新单个设置（热生效）", true, []string{"key"}},

	{"/backup", "GET", "Backup", "备份列表", true, nil},
	{"/backup", "POST", "Backup", "创建备份", true, nil},
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

		op := map[string]interface{}{
			"tags":        []string{rd.Tag},
			"summary":     rd.Summary,
			"operationId": rd.Method + "_" + rd.Path,
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "成功",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{"$ref": "#/components/schemas/ApiResponse"},
						},
					},
				},
			},
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
