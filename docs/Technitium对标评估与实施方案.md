# GoDDI × Technitium DNS Server 对标评估与实施方案

> 版本：v0.1（待确认）
> 日期：2026-09-07
> 对标基线：TechnitiumSoftware/DnsServer v15.4（源码浅克隆核对，非仅凭文档）

---

## 1. 对标方法与信息来源

| 优先级 | 来源 | 位置 | 用途 |
|---|---|---|---|
| 主 | GitHub 源码 | `/tmp/technitium-src`（浅克隆） | 菜单结构、Zone 类型枚举、内核能力 |
| 主 | 源码：`DnsServerCore/www/index.html` L148-158 | SPA 单页 | 确认 Web UI 顶级菜单 |
| 主 | 源码：`DnsServerCore/Dns/Zones/AuthZoneInfo.cs` L34-43 | 枚举定义 | 确认 Zone 类型 |
| 辅 | `APIDOCS.md`（8306 行） | 仓库根目录 | API action 清单与参数 |
| 辅 | `DnsServerCore/WebService*.cs` × 10 | API 实现层 | API 分组印证 |

GoDDI 现状来源：`internal/api/router.go`、`internal/dns/**`、`web-admin/src/router/elegant/routes.ts`、`web-admin/src/views/dns/**` 实地核对。

---

## 2. Technitium 能力基线（源码确认）

### 2.1 Web UI 顶级菜单（index.html，共 11 项）

| # | 菜单 | 关键子功能 |
|---|---|---|
| 1 | Dashboard | 11 项统计卡（NoError/ServFail/NXDomain/Refused/AuthHit/Recursive/CacheHit/Blocked/Dropped/Clients/TotalQueries）、主趋势图 + 3 饼图、Top Clients / Top Domains / Top Blocked Domains 表、Zones / CachedEntries / AllowedZones / BlockedZones / Allow-List / Block-List 计数卡、Blocking 快捷开关下拉 |
| 2 | Zones | 权威区域列表（7 类型筛选 + 通配符过滤 + 分页）、创建/导入/克隆/转换/启用/禁用/删除/Resync、Zone Options、Zone Permissions、DNSSEC 签名与密钥管理 |
| 3 | Cache | 缓存区域列表、删除单区域缓存、Flush 全部缓存 |
| 4 | Allowed | 允许区域：列表/添加/删除/Flush/导入/导出 |
| 5 | Blocked | 阻止区域：同上 6 项 + 阻止列表 URL 订阅与临时禁用 |
| 6 | Apps | 应用商店、安装/更新/卸载、应用配置 |
| 7 | DNS Client | 内置查询工具（Resolve Query / Health Check），结果可导入本地区域 |
| 8 | Settings | DNS 设置（常规/转发/ECS/代理/缓存/安全）、TSIG 密钥名、备份/恢复设置 |
| 9 | DHCP | 作用域/租约/保留（若启用） |
| 10 | Administration | 会话、API Token、用户/组/权限明细、SSO、集群 |
| 11 | Logs | 系统日志 / 查询日志（分页、按节点） |

### 2.2 Zone 类型（AuthZoneType 枚举，8 值）

`Primary / Secondary / Stub / Forwarder / SecondaryForwarder / Catalog / SecondaryCatalog / Unknown`

源码佐证：`Zones/` 目录含 `CatalogZone.cs`、`SecondaryCatalogZone.cs`、`CatalogSubDomainZone.cs`、`SecondaryForwarderZone.cs` 等独立实现。

### 2.3 HTTP API 分组（约 140+ action）

| 分组 | action 数（约） | 要点 |
|---|---|---|
| User | 17 | login/createToken(不过期)/createSingleUseToken/2FA(init·enable·disable)/profile/session |
| Dashboard | 5 | stats/get（LastHour~LastYear+Custom）、stats/getTop（3 类 Top 榜）、metrics JSON/Prometheus、stats/deleteAll |
| Zones | 30+ | CRUD + import/export/clone/convert/enable/disable/resync + options/get·set（queryAccess ACL 6 种、zoneTransfer、notify、RFC2136 update 安全策略）+ permissions/get·set |
| DNSSEC（Zones 子组） | 14 | sign/unsign/viewDS/properties/NSEC↔NSEC3/DNSKEY TTL/私钥增删/rollover/retire/publish |
| Records | 4 | add/get/update/delete，支持 `listZone=true` 全区域列出、expiryTtl、overwrite |
| Cache | 3 | List Cached Zones / Delete Cached Zone / Flush |
| Allowed / Blocked | 6 + 6 | List/Allow(Block)/Delete/Flush/Import/Export |
| Apps | 9 | 商店列表/安装/更新/卸载/配置 |
| DNS Client | 2 | Resolve Query / Health Check |
| Settings | 9 | Get/Set DNS Settings、TSIG names、Force Update BlockLists、Temp Disable、Backup/Restore |
| DHCP | 12 | scopes/leases/reserved CRUD + enable/disable |
| Administration | 26+ | sessions/users/groups/permissions(明细到分区权限)/SSO/Cluster(init/join/leave/config) |

通用约定：`Authorization: Bearer <token>`；响应统一 `status: ok|error|invalid-token|2fa-required`；多数接口支持集群 `node` 参数。

---

## 3. GoDDI 现状矩阵

### 3.1 Web UI 菜单对比

| Technitium | GoDDI 现状 | 差距 |
|---|---|---|
| Dashboard | ✅ dashboard | 部分：缺 6 类细分统计卡与 Top 榜（见 3.3） |
| Zones | ✅ dns_zones + zone-detail | 部分：缺启用/禁用、Zone Options 完整面板、per-zone 权限、Zone History |
| Cache | ✅ dns_cache | 基本对齐 |
| Allowed / Blocked | ✅ dns_security 单页合并 | 中：未按 T 拆分为两个独立入口；缺导入/导出/Flush 语义对齐（待核实已有程度） |
| Apps | ⚠️ 有 `/api/apps` 路由 + internal/apps | 中：前端无 Apps 管理页 |
| DNS Client | ✅ tools_client | 基本对齐（缺"结果导入区域"） |
| Settings | ✅ settings_system | 部分：DNS 设置项颗粒度不足 |
| DHCP | ✅ dhcp ×3 | 基本对齐 |
| Administration | ✅ admin ×5 | 基本对齐（权限模型不同，见 3.4） |
| Logs | ✅ logs_dns / logs_dhcp / logs_audit | 基本对齐 |
| —（GoDDI 特有） | ipam ×3、backup | Technitium 无，保留即可 |

### 3.2 Zone 管理对比

| 能力 | Technitium | GoDDI | 差距等级 |
|---|---|---|---|
| Zone 类型 | 7 类（含 Catalog×2、SecondaryForwarder） | 6 类（primary/secondary/stub/forward/reverse/allowed/blocked），**无 Catalog、无 SecondaryForwarder** | 高 |
| Zone CRUD | ✅ | ✅（router.go L168-188） | — |
| 启用/禁用 Zone | ✅ 独立 API + 状态字段 | ❌ 记录级有 enabled，Zone 级无 disabled 字段 | 中 |
| Import/Export | ✅ + overwrite/overwriteZone/overwriteSoaSerial 参数 | ✅ 基础版 | 低 |
| Clone / Convert | ✅ | ✅ | — |
| Resync（从主同步） | ✅ | ✅（POST /sync） | — |
| Zone Options | ✅ 查询 ACL 6 种 + 传送 + notify + 动态更新策略 | 部分：zone-detail 有 allow-query/transfer/update、notify | 中 |
| per-zone 权限 | ✅ permissions get/set（用户/组 × View/Modify/Delete） | ❌ 仅全局 RBAC | 中 |
| Zone History | ✅ zone history（基于变更日志回放） | ⚠️ 有 logChange 写库（record.go L196），无查询 API/页面 | 低 |
| 记录类型 | 标准 15 + 专有 ANAME/FWD/APP + 未知类型 rdata | 28 种标准类型（record.go L15-20，含 SVCB/HTTPS/TLSA/SSHFP/DNAME/CAA 等），**无 FWD/APP/ANAME** | 低 |
| 记录 expiryTtl / overwrite | ✅ | ❌ expires_at 字段已有（L185），缺 expiryTtl 语义暴露 | 低 |
| DNSSEC | ✅ 14 API（NSEC/NSEC3、KSK/ZSK 生命周期、DS Info） | 部分：签名、NSEC、轮换已有；无 NSEC3 参数管理、无 DS Info、无密钥生命周期 UI（嵌在 zone 页） | 中 |

### 3.3 统计与日志对比

| 能力 | Technitium | GoDDI | 差距 |
|---|---|---|---|
| stats API | LastHour~LastYear + Custom 区间 + 3 Top 榜 | stats.go + dashboard 概览 | 中：缺时间范围参数与 Top 榜 API |
| 统计页面 | Dashboard 11 卡 + 趋势图 + 饼图 | dashboard 单页概览 | 中 |
| Prometheus | ✅ /api/dashboard/metrics/text | ✅ docs/prometheus-alerts.yml 表明已有 metrics | — |
| 查询日志 | ✅ 分页查询日志 | ✅ logs_dns（含 cached/blocked 标记） | — |

### 3.4 认证/权限/集群

| 能力 | Technitium | GoDDI |
|---|---|---|
| API Token | 不过期 token + single-use token | ✅ tokens（缺 single-use） |
| 2FA | ✅ TOTP init/enable/disable | ✅ auth/totp |
| RBAC | 分区权限（Dashboard:View 等 6 分区 ×3 级） | ✅ dns/dhcp/ipam/… × read/write/delete（颗粒度不同，不强求对齐） |
| 集群 | 多节点聚合 + node 参数 | ⚠️ internal/cluster + /cluster 路由已有（成熟度待核实） |
| SSO OIDC | ✅ | ✅ /sso |

---

## 4. 实施方案（按批次）

> 原则：**保持 GoDDI 现有 RESTful API 风格与 RBAC 模型，对标能力而非照搬接口形态**；每批次独立可交付、可回归。

### 批次 A：Zone 管理补全（P0，管理面核心缺口）

| # | 任务 | 涉及文件（预估改动） | 预估 |
|---|---|---|---|
| A1 | Zone 级启用/禁用：Zone struct 加 `disabled`、API `POST /dns/zones/{id}/enable|disable`、解析路径短路 | `internal/dns/zone/zone.go`、`store.go`、`handler/dns_zone.go`、migration | 后端 ~150 行 |
| A2 | Zone Options 扩展：queryAccess ACL（Deny/Allow/OnlyPrivateNetworks/NetworkACL）、notifyNameServers、update 安全策略入库与下发 | `zone.go`、`authoritative.go`、`handler/dns_zone.go` | 后端 ~300 行 |
| A3 | per-zone 权限：zone_members（user/group × view/modify/delete）+ API get/set + Handler 校验 | migration、`internal/rbac`、`handler/dns_zone.go` | 后端 ~250 行 |
| A4 | Zone History：基于现有 logChange 表暴露 `GET /dns/zones/{id}/history` + zone-detail 新增 History 标签页 | `handler/dns_zone.go`、前端 zone-detail | 前+后 ~200 行 |
| A5 | 记录 expiryTtl 语义 + overwrite 参数 + `listZone` 全区域列出参数对齐 | `record.go`、`handler/dns_record.go` | 后端 ~80 行 |
| A6 | 前端 Zone 详情页对标：拆 Options / Permissions / History / DNSSEC 标签页 | `web-admin/src/views/dns/zone-detail/[id].vue` 拆分 | 前端 ~500 行 |

### 批次 B：统计对标（P0）

| # | 任务 | 涉及文件 | 预估 |
|---|---|---|---|
| B1 | stats API 扩展：type=LastHour/Day/Week/Month/Year/Custom(start·end) | `internal/dns/stats.go`、`handler/` | 后端 ~200 行 |
| B2 | Top 榜 API：TopClients / TopDomains / TopBlockedDomains（limit、noReverseLookup） | 同上 | 后端 ~150 行 |
| B3 | Dashboard 页对标：6 细分统计卡 + 3 Top 表 + Blocking 快捷开关 | `web-admin/src/views/dashboard` | 前端 ~400 行 |

### 批次 C：菜单与安全页对标（P0，纯前端为主）

| # | 任务 | 说明 | 预估 |
|---|---|---|---|
| C1 | Allowed / Blocked 拆分为两个独立菜单页（对标 T 菜单结构），各含 Flush/导入/导出 | 从 dns_security 拆分 | 前端 ~300 行 |
| C2 | Apps 管理页：对接已有 /api/apps（列表/启停/配置） | 新页面 | 前端 ~250 行 |
| C3 | DNS Client 页补"结果导入区域"动作 | tools_client 增强 | 前端 ~80 行 |

### 批次 D：加密上游转发（P1，内核大项）

| # | 任务 | 说明 | 预估 |
|---|---|---|---|
| D1 | DoT 上游客户端（miek + crypto/tls） | `internal/dns/forwarder/` | 后端 ~250 行 |
| D2 | DoH 上游客户端（net/http + RFC 8484） | 同上 | 后端 ~250 行 |
| D3 | 转发器地址模型扩展（URL 形式 + 优先级 + 延迟选择）+ 前端 forwarders 页适配 | 前后端 | ~300 行 |
| D4 | DoQ 上游（quic-go） | 建议最后/可裁剪 | ~300 行 |

### 批次 E：Catalog Zone（P1，RFC 9432）

| # | 任务 | 说明 | 预估 |
|---|---|---|---|
| E1 | ZoneType 增加 Catalog/SecondaryCatalog + 成员 zone 关联 catalog 字段 | zone 类型扩展 | 后端 ~350 行 |
| E2 | Catalog 成员记录自动管理（成员 zone 增删 → catalog 记录同步）+ AXFR 支持 | transfer 联动 | 后端 ~250 行 |
| E3 | UI：catalog 创建/成员视图 | 前端 | ~150 行 |

### 批次 F：DNSSEC 完善（P1~P2）

NSEC3 参数管理、DS Info 查看、KSK/ZSK 密钥生命周期 UI（add/rollover/retire/publish）独立页面。预估后端 ~400 行、前端 ~350 行。

### 建议暂不做（差异说明）

- **DNS Apps 商店/APP 记录**：GoDDI internal/apps 已有框架，但 App Store 生态为 T 专有，对标价值低；
- **ANAME 专有记录**：可用"zone apex CNAME 展开"替代方案讨论后定；
- **XFR-over-TLS/QUIC、PROXY protocol、DNS64**：使用面窄，列为 backlog。

---

## 5. 工作量汇总

| 批次 | 内容 | 预估规模 |
|---|---|---|
| A | Zone 管理补全 | 后端 ~980 行 + 前端 ~700 行 |
| B | 统计对标 | 后端 ~350 行 + 前端 ~400 行 |
| C | 菜单/安全页对标 | 前端 ~630 行 |
| D | 加密上游 | 后端 ~1100 行 + 前端 ~300 行 |
| E | Catalog Zone | 后端 ~600 行 + 前端 ~150 行 |
| F | DNSSEC 完善 | 后端 ~400 行 + 前端 ~350 行 |

P0（A+B+C）合计：后端 ~1330 行 + 前端 ~1730 行，约 6 个可独立回归的交付单元。

---

## 6. 待确认决策点

1. **实施范围**：仅 P0（A/B/C）/ P0 + P1（D/E）/ 全部（A~F）？
2. **API 风格**：确认保持 GoDDI RESTful（推荐），不对齐 Technitium 的 action 风格？
3. **per-zone 权限模型**（A3）：新增 zone 级成员权限，会与全局 RBAC 叠加（取交集），确认该设计？
4. **DoQ**（D4）：依赖 quic-go，体积与维护成本高，是否延后？
5. **Catalog Zone**（E）：多权威服务器场景才有价值，是否纳入本期？
