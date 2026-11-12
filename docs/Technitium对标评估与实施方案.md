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
| Zones | ✅ dns_zones + zone-detail | **大部分已补齐（round 220–221 复核修正）**：启用/禁用、per-zone 权限、Zone History 均已上线；Zone Options 完整面板仍部分（allow-query/transfer/update/notify 已有） |
| Cache | ✅ dns_cache | 基本对齐 |
| Allowed / Blocked | ✅ dns_security 单页合并 | **低（round 222 复核修正）**：导入/导出/清空在 allowed 页已有（batch5 e2e）、blocklists flush 已有（API + blocked 页）——「按 T 拆分入口」的布局差异保留 |
| Apps | ⚠️ `/api/apps` 为 **501 预留契约**（v0.9.0 计划，`TestReservedExtensionEndpointsAnswer501` 钉住）| 中：前端无 Apps 管理页——按计划有意保留 |
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
| 启用/禁用 Zone | ✅ 独立 API + 状态字段 | **已补齐（round 221 复核修正）**：`POST /dns/zones/{id}/enable|disable`（OpenAPI 已登记） | 低 |
| Import/Export | ✅ + overwrite/overwriteZone/overwriteSoaSerial 参数 | ✅ 基础版 | 低 |
| Clone / Convert | ✅ | ✅ | — |
| Resync（从主同步） | ✅ | ✅（POST /sync） | — |
| Zone Options | ✅ 查询 ACL 6 种 + 传送 + notify + 动态更新策略 | 部分：zone-detail 有 allow-query/transfer/update、notify | 中 |
| per-zone 权限 | ✅ permissions get/set（用户/组 × View/Modify/Delete） | **已补齐（round 221 复核修正）**：`GET/PUT /dns/zones/{id}/permissions` + zone-detail 页权限编辑（`RequireZonePermission` 路由中间件按 View/Modify/Delete 分级） | 低 |
| Zone History | ✅ zone history（基于变更日志回放） | **已补齐（round 220 复核修正）**：`GET /dns/zones/{id}/history`（OpenAPI 已登记，`internal/api/handler/dns_zone_extra.go`）+ zone-detail 页历史表（`dns/zone-detail/[id].vue`）——原「无查询 API/页面」为快照时点状态 | 低 |
| 记录类型 | 标准 15 + 专有 ANAME/FWD/APP + 未知类型 rdata | 28 种标准类型（record.go L15-20，含 SVCB/HTTPS/TLSA/SSHFP/DNAME/CAA 等），**无 FWD/APP/ANAME** | 低 |
| 记录 expiryTtl / overwrite | ✅ | **已补齐（round 222 复核修正）**：`expiry_ttl` 字段在 API 与 zone-detail 记录表单均已暴露 | 低 |
| DNSSEC | ✅ 14 API（NSEC/NSEC3、KSK/ZSK 生命周期、DS Info） | 评估时：实验性且未生成 RRSIG/DNSKEY。**现已在批次 F 增量 1–5 补齐后端签名链路（含 NSEC/NSEC3 否定证明）与 DS/参数/密钥管理**；仍缺父区 DS 链、生产级轮换，功能默认关闭 | 高（已收敛） |

### 3.3 统计与日志对比

| 能力 | Technitium | GoDDI | 差距 |
|---|---|---|---|
| stats API | LastHour~LastYear + Custom 区间 + 3 Top 榜 | stats.go + dashboard 概览 | 中：缺时间范围参数与 Top 榜 API |
| 统计页面 | Dashboard 11 卡 + 趋势图 + 饼图 | dashboard 单页概览 | 中 |
| Prometheus | ✅ /api/dashboard/metrics/text | ✅ `/metrics` 与示例告警规则存在；真实 Prometheus/`promtool` 接入与告警触发尚未作为本仓证据闭合 | 中 |
| 查询日志 | ✅ 分页查询日志 | ✅ logs_dns（含 cached/blocked 标记） | — |

### 3.4 认证/权限/集群

| 能力 | Technitium | GoDDI |
|---|---|---|
| API Token | 不过期 token + single-use token | ✅ tokens 含 **single-use**（`is_single_use`，第二次使用 401——api.spec 实测；batch4 实测一次性值仅展示一次） |
| 2FA | ✅ TOTP init/enable/disable | ✅ auth/totp |
| RBAC | 分区权限（Dashboard:View 等 6 分区 ×3 级） | ✅ dns/dhcp/ipam/… × read/write/delete（颗粒度不同，不强求对齐） |
| 集群 | 多节点聚合 + node 参数 | ⚠️ `/cluster` 为 **501 预留契约**（有意保留，`TestReservedExtensionEndpointsAnswer501` 钉住；协议模型与候选原语在 `internal/cluster` + `internal/enterprise/*`） |
| SSO OIDC | ✅ | ⚠️ `/sso` 为 **501 预留契约**（有意保留，同上钉住）——原「✅」不准确 |

---

## 4. 实施方案（按批次）

> 原则：**保持 GoDDI 现有 RESTful API 风格与 RBAC 模型，对标能力而非照搬接口形态**；每批次独立可交付、可回归。

### 批次 A：Zone 管理补全（P0，管理面核心缺口）

> **状态（round 224 复核）**：A1（启用/禁用）、A3（per-zone 权限）、A4（Zone History）、A5（expiryTtl）**已实现**（见 §3.2 修正行）；A2（Zone Options 扩展）与 A6（zone-detail 拆分标签页）**部分实现**（allow-query/transfer/update/notify 已有，History/权限/加密标签已在 zone-detail 页）。本节其余内容保留为原方案文字。

| # | 任务 | 涉及文件（预估改动） | 预估 |
|---|---|---|---|
| A1 | Zone 级启用/禁用：Zone struct 加 `disabled`、API `POST /dns/zones/{id}/enable|disable`、解析路径短路 | `internal/dns/zone/zone.go`、`store.go`、`handler/dns_zone.go`、migration | 后端 ~150 行 |
| A2 | Zone Options 扩展：queryAccess ACL（Deny/Allow/OnlyPrivateNetworks/NetworkACL）、notifyNameServers、update 安全策略入库与下发 | `zone.go`、`authoritative.go`、`handler/dns_zone.go` | 后端 ~300 行 |
| A3 | per-zone 权限：zone_members（user/group × view/modify/delete）+ API get/set + Handler 校验 | migration、`internal/rbac`、`handler/dns_zone.go` | 后端 ~250 行 |
| A4 | Zone History：基于现有 logChange 表暴露 `GET /dns/zones/{id}/history` + zone-detail 新增 History 标签页 | `handler/dns_zone.go`、前端 zone-detail | 前+后 ~200 行 |
| A5 | 记录 expiryTtl 语义 + overwrite 参数 + `listZone` 全区域列出参数对齐 | `record.go`、`handler/dns_record.go` | 后端 ~80 行 |
| A6 | 前端 Zone 详情页对标：拆 Options / Permissions / History / DNSSEC 标签页 | `web-admin/src/views/dns/zone-detail/[id].vue` 拆分 | 前端 ~500 行 |

### 批次 B：统计对标（P0）

> **状态（round 225 复核）**：**未实现**——stats API 仍缺时间范围参数与 Top 榜（§3.3 行准确；dashboard 单页概览已有）。本节内容保留为原方案文字。

| # | 任务 | 涉及文件 | 预估 |
|---|---|---|---|
| B1 | stats API 扩展：type=LastHour/Day/Week/Month/Year/Custom(start·end) | `internal/dns/stats.go`、`handler/` | 后端 ~200 行 |
| B2 | Top 榜 API：TopClients / TopDomains / TopBlockedDomains（limit、noReverseLookup） | 同上 | 后端 ~150 行 |
| B3 | Dashboard 页对标：6 细分统计卡 + 3 Top 表 + Blocking 快捷开关 | `web-admin/src/views/dashboard` | 前端 ~400 行 |

### 批次 C：菜单与安全页对标（P0，纯前端为主）

> **状态（round 225 复核）**：**未实现**——菜单仍合并、Apps 无管理页（与 501 契约一致）；DNS Client 页缺「结果导入区域」（§3.2 行准确）。本节内容保留为原方案文字。

| # | 任务 | 说明 | 预估 |
|---|---|---|---|
| C1 | Allowed / Blocked 拆分为两个独立菜单页（对标 T 菜单结构），各含 Flush/导入/导出 | 从 dns_security 拆分 | 前端 ~300 行 |
| C2 | Apps 管理页：对接已有 /api/apps（列表/启停/配置） | 新页面 | 前端 ~250 行 |
| C3 | DNS Client 页补"结果导入区域"动作 | tools_client 增强 | 前端 ~80 行 |

### 批次 D：加密上游转发（P1，内核大项）

> **状态（round 224 复核）**：D1–D4 **全部已实现**——`internal/dns/forwarder/upstream_encrypted.go`（DoT `tcp-tls`、DoH RFC 8484 POST、DoQ quic-go/RFC 9250）+ 转发器 `Priority` 排序；STATUS §1 已列「DoT/DoH/DoQ 上游装配」。本节其余内容保留为原方案文字。

| # | 任务 | 说明 | 预估 |
|---|---|---|---|
| D1 | DoT 上游客户端（miek + crypto/tls） | `internal/dns/forwarder/` | 后端 ~250 行 |
| D2 | DoH 上游客户端（net/http + RFC 8484） | 同上 | 后端 ~250 行 |
| D3 | 转发器地址模型扩展（URL 形式 + 优先级 + 延迟选择）+ 前端 forwarders 页适配 | 前后端 | ~300 行 |
| D4 | DoQ 上游（quic-go） | 建议最后/可裁剪 | ~300 行 |

### 批次 E：Catalog Zone（P1，RFC 9432）

> **状态（round 224 复核）**：**未实现**——Zone 类型仍无 Catalog/SecondaryCatalog（§3.2 行「无 Catalog」准确）。本节内容保留为原方案文字。

| # | 任务 | 说明 | 预估 |
|---|---|---|---|
| E1 | ZoneType 增加 Catalog/SecondaryCatalog + 成员 zone 关联 catalog 字段 | zone 类型扩展 | 后端 ~350 行 |
| E2 | Catalog 成员记录自动管理（成员 zone 增删 → catalog 记录同步）+ AXFR 支持 | transfer 联动 | 后端 ~250 行 |
| E3 | UI：catalog 创建/成员视图 | 前端 | ~150 行 |

### 批次 F：DNSSEC 完善（P1~P2）

**增量 1（round 113）：真实签名核心已完成并通过密码学验证，尚未接入应答路径。**

针对表中「当前不生成 RRSIG/DNSKEY，真实签名链路未完成」的高优先级缺口，新增 `internal/dns/dnssec/responder.go`：
- `Responder.DNSKEYRRset(zoneName)`：发布该区所有启用的 DNSKEY（KSK+ZSK），并用活跃 KSK 对整个 DNSKEY RRset 签出 RRSIG。
- `Responder.SignRRsets(zoneName, rrs)`：按 (owner, type, class) 分组 RRset，每组用活跃 ZSK 签出独立 RRSIG；一个混合应答不会被错误地用单一签名覆盖。
- `Responder.SignedZone(zoneName)`：只有 `dnssec_enabled=1` 且同时具备活跃 KSK/ZSK 才视为已签名。
- **fail-closed**：无密钥、密钥解密失败（错误的 JWT secret / 损坏密文）、签名出错时一律返回**原始未签名应答**，绝不返回部分签名 —— 部分签名会让校验方把整个区判为 Bogus，比不签名更糟。
- 签名有效期：inception 回拨 1h、有效期 30 天；已发布 DNSKEY TTL 3600。

配套重构（行为不变）：`generateKey` 改为直接从 `dns_zones` 读取区名，不再依赖 `ZoneManager`（签名回应器没有 ZoneManager）；抽出包级 `parsePrivateKey`，让管理器 API 与回应器共用同一份私钥编码解析；抽出 `buildDNSKEYRecord`。

验证：`responder_test.go` 6 组测试全部 PASS，含 **真实 `dns.RRSIG.Verify` 往返**（DNSKEY RRset 用 KSK 验、A/TXT RRset 用 ZSK 验）、篡改 RRset 后验证必须失败、未签名/无密钥/密钥不可解密时保持未签名、以及 ECDSA P256/P384、ED25519、RSA-SHA256/512 **5 种算法的逐一签名验证**。

**增量 2（round 114）：DNSSEC 密钥已复制到数据面存储，签名不再依赖控制库。**
- `internal/dataplane/migrations/016_dnssec_keys.sql`：zone 存储新增 `dns_dnssec_keys` 只读投影（9 列，私钥保持加密态；**故意不建外键**，理由同 `dns_zone_transfer`）。
- `internal/dataplane/sync.go`：`dnsTables` 加入 `dns_dnssec_keys`（`replaceWhole`）。已确认 `TablesFor` 只被下行 `Sync` 消费，不会被上行 push 回写。
- `migrations/031_dnssec_keys_bump_revision.sql`：控制库 3 个触发器，密钥增/改/删均 bump `domain=dns` 修订号，否则密钥轮换后数据面会继续用已退役密钥签名。
- `internal/dataplane/dnssec_keys_test.go` 3 个测试全 PASS：密钥复制后 **仅用 `store.DB` 构造 `dnssec.Responder` 即可产出 `dns.RRSIG.Verify` 通过的 DNSKEY/RRSIG**（证明数据面独立性）、禁用/删除密钥后的失效与重同步、投影列一致性。
- **隔离单机实跑**：启用 `dns.dnssec.enabled` 后经 API 生成 ED25519 KSK(tag 43318)+ZSK(tag 55514)，1s 同步后 `dnsdata.db` 出现完全相同的两行；手动禁用 ZSK → 控制库修订 40→41 → 数据面存储该行 `enabled=0`。两个新迁移（控制 031 / 数据面 016）在真实启动中均成功应用。

**增量 3（round 115）：签名已接入权威应答路径，真实 `dig +dnssec` 验证通过。**
- `internal/dns/server/server.go`：`Server` 新增 `dnssecResponder` 字段与 `SetDNSSECResponder`；`lookupAuthoritative` 增加 `dnssecOK`（EDNS DO 位）参数。仅当区已签名（`dnssec_enabled=1` + 活跃 KSK/ZSK）且 DO 置位时：
  - `DNSKEY` 查询返回区密钥集 + KSK 签出的 RRSIG（未置 DO 时仍返回 DNSKEY RR，但不带签名）。
  - 正向应答按 RRset 追加 ZSK 签出的 RRSIG。
  - 任何错误都保留原始未签名应答（fail-closed），未签名区/未启用门控时行为完全不变。
- `internal/dns/server/handler.go`：从 `req.IsEdns0().Do()` 取 DO 位传入。
- `cmd/goddi/main.go`：仅当 `cfg.DNS.DNSSEC.Enabled` 时用 `dnsDataDB`（已含复制的密钥）+ `cfg.Security.JWTSecret` 构造回应器；默认关闭。
- `internal/dns/server/dnssec_answer_test.go` 6 个测试全 PASS：DO 位下 A 应答的 RRSIG 用 ZSK `Verify` 通过、无 DO 位不发签名、DNSKEY 应答用 KSK `Verify` 通过、未签名区/错误 secret/未挂回应器均保持未签名。
- **隔离单机实跑（`dig +tcp +dnssec`）**：`DNSKEY` 应答含 2 个 DNSKEY（flags 256/257）+ `RRSIG DNSKEY ... 54415`（KSK tag）；`www.signed-e2e.test. A` 应答含 `A 192.0.2.55` + `RRSIG A ... 12818`（ZSK tag）；**不带 `+dnssec` 的普通查询只返回 A，无 RRSIG**。
- 同步修正已过时注释：`dns_zone.go` 实验性提示、`SignZone` 文档、`GetDNSSECStatus().signed=false` 的说明改为「缺 NSEC 否定证明」，不再声称「从不产生 RRSIG」。

**增量 4（round 116）：NSEC 否定证明完成，真实 `dig +dnssec` 验证通过。**
- 新增 `internal/dns/dnssec/denial.go`：
  - `Responder.SignNegative` 给权威否定应答追加 **SOA 的 RRSIG** 与 **NSEC + RRSIG**，由活跃 ZSK 签发；全程 fail-closed（缺密钥/解密失败/签名失败 → 原样返回未签名的 SOA）。
  - `zoneNameMap` 从数据面 `dns_records` 读取区内所有 owner 名与类型，apex 恒定补 SOA/DNSKEY，所有名补 NSEC/RRSIG；`canonicalLess` 实现 RFC 4034 §6.1 的**从右到左标签、字节序、短名在前**规范顺序。
  - NXDOMAIN：取覆盖查询名的 NSEC（owner < qname < NextDomain，链在 apex 回绕），并按 RFC 4035 §3.1.3.2 追加**最近封闭者通配符 `*.closestEncloser` 的覆盖 NSEC**（与前者 owner 相同时去重）；NODATA：取查询名自身的 NSEC 且位图剔除 qtype。
  - `nsec3Params`：区配置了 `nsec3_*` 非默认参数时改走 NSEC3 路径（增量 5）；参数不可读/非法时保持未签名否定应答（fail-closed）。
- `internal/dns/server/server.go` 否定分支在 `signed && dnssecOK` 时调用 `SignNegative`；DO 位未置或区未签名时行为完全不变。
- 测试：`denial_test.go` 4 组（规范序、NXDOMAIN 双 NSEC 逐个 `Verify`、NODATA 位图剔除 qtype 并 `Verify`、非法参数/无 ZSK/错误 secret 三种 fail-closed）+ `dnssec_answer_test.go` 新增 4 组（服务层 NXDOMAIN/NODATA 的 RRSIG 验证、无 DO、未签名区）全部 PASS。
- **隔离单机实跑 `dig +tcp +dnssec`**：
  - `missing.nsec-e2e.test. A` → `NXDOMAIN`，authority 段为 `SOA + RRSIG(SOA) + NSEC(mail→www，覆盖 missing) + RRSIG(NSEC) + NSEC(apex→mail，覆盖通配符 *. ) + RRSIG(NSEC)`（**2 个 NSEC + 3 个 RRSIG**）。
  - `www.nsec-e2e.test. AAAA` → `NOERROR/NODATA`，`NSEC www→apex` 位图 `A RRSIG NSEC`（**不含 AAAA**）+ RRSIG。
  - 不带 `+dnssec` 的 NXDOMAIN 仅返回 SOA，无 NSEC/RRSIG。
- 同步修正过时注释（`main.go`、`dns_zone.go`、`SignZone`、`GetDNSSECStatus`），不再声称「否定应答无证明」。

**增量 5（round 120）：NSEC3 否定证明完成，真实 `dig +dnssec` 验证通过。**
- 新增 `internal/dns/dnssec/nsec3.go`：`nsec3Params` 读取并校验 `nsec3_iterations/nsec3_salt/nsec3_optout`（沿用 `SetNSEC3Params` 的边界）；`nsec3Chain` 用 `dns.HashName`（SHA-1 + salt + iterations，base32hex）把区名链按**哈希**排序；`exact`/`covering` 分别构造 NODATA 与覆盖证明；`nextCloserName` 实现 RFC 5155 §8.3 的最近封闭者路径。
- NXDOMAIN 按 RFC 5155 §8.9 给出**最近封闭者精确匹配 + 下一近名覆盖 + 通配符覆盖**三类 NSEC3（按 owner 去重），NODATA 给出 H(qname) 的精确 NSEC3 且位图剔除 qtype；SOA 与每个 NSEC3 RRset 均由 ZSK 签出 RRSIG。位图列出**原 owner 名**下的类型（剔除 NSEC 标记、apex 补 NSEC3PARAM），不是哈希名下的类型。
- 显式设置 `SaltLength = len(hex)/2`（空 salt 写 `-`）、`HashLength = 20`（SHA-1），与 `scan_rr.go` 的解析约定一致；`pack/unpack` 往返测试覆盖了带盐与空盐两种编码。
- 测试：`nsec3_test.go` 7 组全 PASS（参数识别/默认仍走 NSEC、NODATA 位图与 `RRSIG.Verify`、NXDOMAIN 三类证明与逐个 `Verify`、无效参数 fail-closed、`nextCloserName`、wire 往返、空盐 wire 往返）。
- **隔离单机实跑**：`PUT /dnssec/nsec3` 设 `iterations=5, salt=aabb` 并同步到数据面存储；`dig +dnssec` 的 NXDOMAIN 返回 `SOA + RRSIG(SOA) + 2×NSEC3 + 2×RRSIG(NSEC3)`，NODATA 返回 `SOA + RRSIG(SOA) + NSEC3 + RRSIG(NSEC3)`，NSEC3 线上字段 `1 0 5 AABB ...` 正确，且 **0 条 NSEC**（不再与已发布参数矛盾）。
- 同步修正所有过时注释（`main.go`、`dns_zone.go`、`SignZone`、`GetDNSSECStatus`），不再声称「NSEC3 不支持」。

**剩余（不算「未完成实施任务」，属明确实验性边界）**：父区 DS 链式信任不在本服务职责内、密钥轮换自动化与生产级 KSK 保管仍需设计、NSEC3 opt-out 的完整委派语义（`Flags=1` 已按参数写出，但未做未签名委派的链裁剪）。因此 `GetDNSSECStatus().signed=false` 与 `dns.dnssec.enabled` **默认关闭保持不变，不宣称 DNSSEC 生产可用**；**批次 F 的「UI 密钥生命周期独立页面」已于 round 133 补齐**（`web-admin/src/views/dns/dnssec/index.vue`，路由 `/dns/dnssec`、DNS 管理导航项、RBAC `dns:read`，含区域选择、启用/禁用/轮换/生成/晋升/启停/删除密钥、DS 记录、NSEC3 参数；`test/browser/batch15.cjs` 浏览器实测 PASS）。



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
