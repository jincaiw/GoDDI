# GoDDI 产品需求文档最终稿

**文档版本：V0.01**  
**产品名称：GoDDI**  
**产品类型：DNS + DHCP + IPAM 一体化 DDI 平台**  
**开发语言：Go**  
**开源协议：MIT License**  
**默认数据库：SQLite**  
**v0.1.0 数据库：SQLite（MySQL / PostgreSQL 为后续规划）**  
**前端技术栈：Vue 3 + TypeScript + Naive UI**  
**后端技术栈：Go + REST API + sqlc + goose**  
**部署方式：单文件、systemd、Docker、Docker Compose**

---

## 0. 文档说明

本文档是 GoDDI V0.01 的产品需求文档最终稿，用于指导产品设计、系统架构、后端开发、前端开发、测试验收和后续迭代。

GoDDI 是一个独立开发的 DDI 平台，功能目标覆盖企业级 DNS、DHCP、IPAM、Web 管理、API、日志、审计、安全策略、插件扩展、集群管理和监控指标等能力。

本文档不引用任何外部产品的专有接口、专有配置格式和专有数据结构。文中出现的通用开源组件名称仅用于技术选型说明，不构成对外部产品实现的复用或兼容承诺。所有功能均以通用 DNS / DHCP / IPAM 行业能力为依据，采用 Go 语言独立设计和实现。

---

## 1. 产品定位

GoDDI 是一个使用 Go 语言开发的轻量企业级 DDI 平台。

DDI 指：

```text
DNS + DHCP + IPAM
```

GoDDI 的目标是为企业、园区、学校、数据中心、分支机构、家庭实验室、私有云和边缘节点提供统一的网络基础服务管理能力。

GoDDI 不是单纯的 DNS 转发器，也不是单纯的 DHCP 服务，而是一个统一管理以下能力的基础设施产品：

- 权威 DNS
- 递归 DNS
- DNS 转发
- DNS 缓存
- DNSSEC
- 加密 DNS
- DNS 安全过滤
- DNS 查询审计
- DHCPv4
- IP 地址管理
- Web 管理控制台
- REST API
- 多用户权限
- API Token
- 2FA
- OIDC SSO
- 插件扩展
- 集群管理
- 备份恢复
- Prometheus 监控指标

---

## 2. 开源协议

### 2.1 协议选择

GoDDI 使用：

```text
MIT License
```

### 2.2 MIT 协议目标

使用 MIT 协议后，GoDDI 允许：

- 个人免费使用
- 企业免费使用
- 商业产品集成
- 私有化部署
- 二次开发
- 修改源码
- 闭源分发修改版本
- SaaS 化部署
- 集成到其他商业系统
- 作为网络基础设施产品内置组件使用

### 2.3 合规要求

GoDDI 必须坚持独立实现：

- 不复制任何不兼容协议项目的代码
- 不改写任何不兼容协议项目的代码
- 不复用任何不兼容协议项目的 UI、接口、配置格式和数据结构
- 不将 GPL / AGPL 依赖引入核心代码
- 优先使用 MIT、BSD、Apache-2.0、ISC 类兼容依赖
- 仓库根目录必须包含 `LICENSE`
- 每个主要源码文件建议保留 MIT License 头部声明
- README 明确说明 GoDDI 是独立产品

### 2.4 依赖协议约束

| 协议 | 是否允许 | 说明 |
|---|---|---|
| MIT | 允许 | 推荐 |
| BSD-2-Clause | 允许 | 推荐 |
| BSD-3-Clause | 允许 | 推荐 |
| Apache-2.0 | 允许 | 推荐 |
| ISC | 允许 | 推荐 |
| MPL-2.0 | 谨慎允许 | 需要审查边界 |
| LGPL | 原则上避免 | 避免核心依赖 |
| GPL | 禁止用于核心代码 | 避免传染性合规风险 |
| AGPL | 禁止用于核心代码 | 避免网络服务开源义务风险 |

---

## 3. 产品目标

### 3.1 总体目标

GoDDI 要实现一个独立、轻量、稳定、安全、可视化、可自动化的 DDI 平台。

核心目标：

以下目标描述的是产品完整能力方向，V0.01 首版交付范围以第 36 章为准，首版优先单机和小规模试点能力。

1. 提供高性能 DNS 服务。
2. 同时支持权威 DNS 和递归 DNS。
3. 支持 DNS UDP、TCP，预留 DoT、DoH、DoQ。
4. 支持 DNS over PROXY protocol。
5. 支持企业内部 Zone 管理。
6. 支持 DNSSEC 验证和 Zone 签名。
7. 支持 DNS 缓存、预取、持久化缓存和 Serve Stale。
8. 支持广告、恶意域名、钓鱼域名、跟踪域名阻断。
9. 支持 DNS Rebinding 防护。
10. 支持 CNAME Cloaking 防护。
11. 支持客户端、子网、用户组级策略。
12. 支持 DHCPv4 多网络地址分配。
13. 支持 DHCP 与 DNS 自动联动。
14. 支持 IPAM 地址资源管理。
15. 支持 Web 控制台。
16. 支持完整 REST API。
17. 支持多用户、角色权限、2FA，预留 OIDC SSO。
18. 支持长期 API Token 和单次 API Token。
19. 支持系统日志、查询日志、DHCP 日志、审计日志。
20. 支持 Prometheus 监控指标。
21. 支持 Docker、systemd 部署，预留容器编排扩展。
22. 支持单节点运行与配置管理，预留集群管理与配置同步。
23. 预留插件/App 扩展机制并定义接入边界。
24. 使用 MIT 协议发布，便于商业使用和二次开发。

### 3.2 产品愿景

GoDDI 最终应成为一个：

```text
可单机运行、可集群扩展、可 API 自动化、可企业私有化部署的开源 DDI 平台
```

---

## 4. 产品边界

### 4.1 必须包含

GoDDI 完整产品范围必须包含：

V0.01 的实际交付边界以第 36 章为准，下列条目表示完整产品蓝图。

- DNS Server
- DNS Client
- DHCP Server
- IPAM
- Web Console
- REST API
- Plugin / App 扩展机制
- Cluster
- Metrics
- Logs
- Backup
- RBAC
- 2FA
- OIDC SSO
- Docker 部署
- systemd 部署
- 配置文件管理
- 数据导入导出
- 安全策略管理

### 4.2 不包含

GoDDI 不做以下内容：

- 不做完整云管平台
- 不做资产管理 CMDB
- 不做网络设备配置管理
- 不做 SDN 控制器
- 不做域名注册商系统
- 不做公网权威 DNS SaaS 平台
- 不做复杂工单审批系统
- 不做商业闭源授权模块
- 不兼容任何外部产品的专有配置格式
- 不承诺迁移任何外部产品的数据文件

---

## 5. 目标用户

| 用户类型 | 典型需求 |
|---|---|
| 企业网络管理员 | 管理内网 DNS、DHCP、IP 地址、解析审计 |
| 中小企业 IT 运维 | 替代路由器、Windows DHCP/DNS 或零散脚本 |
| 数据中心运维 | 管理多业务 Zone、内网解析、反向解析、保留地址 |
| 学校/园区网络 | 统一管理终端 DHCP、DNS 策略和访问控制 |
| MSP / 集成商 | 为客户部署本地 DNS/DHCP/IPAM 平台 |
| 家庭实验室用户 | 自建安全 DNS、广告阻断、DoH/DoT 转发 |
| 私有云平台团队 | 管理云内域名、动态地址和自动化 API |
| 安全团队 | 进行 DNS 查询审计、阻断恶意域名和分析异常访问 |

---

## 6. 总体架构

```text
+------------------------------------------------------+
|                    GoDDI Web Console                 |
|              Vue 3 + TypeScript + Naive UI           |
+----------------------------+-------------------------+
                             |
                             | REST API / WebSocket
                             |
+------------------------------------------------------+
|                    GoDDI API Server                  |
| Auth / RBAC / Audit / Settings / Backup / Metrics    |
+--------+-------------+-------------+-----------------+
         |             |             |
         |             |             |
+--------v---+   +-----v-----+   +---v-----------------+
| DNS Engine |   | DHCP      |   | IPAM                |
| Recursive  |   | Server    |   | Address Space       |
| Authority  |   | Scope     |   | Subnet              |
| Forwarder  |   | Lease     |   | Address             |
| Cache      |   | Option    |   | Reservation         |
| DNSSEC     |   | DNS Link  |   | Conflict Detection  |
| Filter     |   +-----------+   +---------------------+
| DNS Client |
+--------+---+
         |
+--------v---------------------------------------------+
| Plugin / App Runtime                                |
| Blocking / Forwarding / DNS64 / Split Horizon / Geo |
+------------------------------------------------------+
         |
+------------------------------------------------------+
| Storage Layer                                       |
| SQLite / File Storage / Cache                       |
+------------------------------------------------------+
         |
+------------------------------------------------------+
| Network Listeners                                   |
| DNS UDP/TCP / DHCP UDP / HTTP API / 加密 DNS 预留   |
+------------------------------------------------------+
```

---

## 7. 技术选型

### 7.1 后端技术栈

| 项目 | 选型 |
|---|---|
| 语言 | Go 1.26+ |
| Web 框架 | Chi，备选 Gin |
| DNS 协议库 | miekg/dns |
| DHCP 协议库 | insomniacslk/dhcp 或自研封装 |
| 配置格式 | YAML + 环境变量 |
| 数据访问 | sqlc |
| 数据库迁移 | goose |
| 默认数据库 | SQLite |
| 后续数据库规划 | MySQL / PostgreSQL |
| 日志 | slog / zerolog |
| 指标 | Prometheus |
| 认证 | Session + JWT + API Token |
| 密码哈希 | Argon2id |
| CLI | cobra |
| 参数校验 | go-playground/validator |
| 后台任务 | 内置任务调度器 |
| 插件隔离 | 进程隔离 / WASM 预留 |

### 7.2 前端技术栈

| 项目 | 选型 |
|---|---|
| 框架 | Vue 3 |
| 语言 | TypeScript |
| UI 组件 | Naive UI |
| 状态管理 | Pinia |
| 构建工具 | Vite |
| 图表 | ECharts |
| API 客户端 | Axios |
| 路由 | Vue Router |
| 代码规范 | ESLint + Prettier |
| 国际化 | vue-i18n |

### 7.3 部署技术

| 场景 | 方式 |
|---|---|
| 单机部署 | 二进制文件 + config.yaml |
| Linux 服务 | systemd |
| 容器部署 | Docker |
| 编排部署 | Docker Compose |
| 容器编排预留 | 编排接口与目录结构预留 |
| DHCP 容器部署 | host network |
| 监控接入 | Prometheus scrape |
| 日志接入 | 文件日志 / stdout / syslog 预留 |

V0.01 仅交付二进制、systemd、Docker 和 Docker Compose，其它编排场景仅做配置与目录结构预留。

---

## 8. 功能模块总览

| 模块 | 子功能 |
|---|---|
| Dashboard | 实时状态、统计图、Top 查询、Top 客户端、系统健康 |
| DNS Server | UDP、TCP，预留 DoT、DoH、DoQ、PROXY protocol |
| Recursive DNS | 递归解析、转发、缓存、QNAME 最小化、DNSSEC 验证 |
| Authoritative DNS | Zone、Record、DNSSEC 签名、Zone Transfer |
| DNS Cache | 正向缓存、负向缓存、Serve Stale、Prefetch、持久化 |
| DNS Forwarder | UDP、TCP，预留 DoT、DoH、DoQ、HTTP/SOCKS5 代理 |
| DNS Client | 在线解析测试、导入解析结果、本地调试 |
| DNS Security | Rebinding 防护、Rate Limit、CNAME Cloaking、DNSSEC |
| DNS Filter | 黑名单、白名单、阻断列表、正则规则、客户端策略 |
| DNS Apps | 插件框架、扩展记录、阻断扩展、DNS64、Split Horizon |
| Zone Transfer | AXFR、IXFR、NOTIFY、TSIG、XFR-over-TLS、XFR-over-QUIC |
| Catalog Zone | 自动分发 Secondary Zone |
| Dynamic DNS | 动态 DNS 更新、安全策略 |
| DHCP Server | Scope、Lease、Reservation、Option、DNS 联动 |
| IPAM | 地址空间、子网、IP 地址、冲突检测、使用率 |
| User & RBAC | 用户、角色、权限、组、会话 |
| API Token | 长期 Token、单次 Token、权限范围 |
| SSO | OIDC 登录预留 |
| 2FA | TOTP 二次认证 |
| Logs | 系统日志、查询日志、DHCP 日志、审计日志 |
| Metrics | JSON 指标、Prometheus 文本指标 |
| Cluster | 节点、心跳、统一管理、配置同步预留 |
| Backup | 备份、恢复、导入、导出 |
| Settings | 系统配置、DNS 配置、证书、网络监听、代理 |

---

# 9. DNS Server 需求

## 9.1 DNS 网络协议

GoDDI 必须支持以下 DNS 监听协议：

| 协议 | 默认端口 | 交付阶段 |
|---|---:|---|
| DNS over UDP | 53 | 必须实现 |
| DNS over TCP | 53 | 必须实现 |
| DNS-over-TLS | 853 | 预留设计，后续实现 |
| DNS-over-HTTPS HTTP/1.1 | 443 / 8053 | 预留设计，后续实现 |
| DNS-over-HTTPS HTTP/2 | 443 / 8053 | 预留设计，后续实现 |
| DNS-over-HTTPS HTTP/3 | 443 / 8053 | 预留设计，后续实现 |
| DNS-over-QUIC | 853 / 8853 | 预留设计，后续实现 |
| DNS over PROXY protocol v1 | 可配置 | 预留设计，后续实现 |
| DNS over PROXY protocol v2 | 可配置 | 预留设计，后续实现 |

## 9.2 DNS 监听配置

```yaml
dns:
  enabled: true
  listeners:
    udp:
      enabled: true
      address: "0.0.0.0:53"
    tcp:
      enabled: true
      address: "0.0.0.0:53"
    dot:
      enabled: false
      address: "0.0.0.0:853"
      cert_file: "/etc/goddi/certs/server.crt"
      key_file: "/etc/goddi/certs/server.key"
    doh:
      enabled: false
      address: "0.0.0.0:8053"
      path: "/dns-query"
      http1: true
      http2: true
      http3: false
    doq:
      enabled: false
      address: "0.0.0.0:8853"
    proxy_protocol:
      enabled: false
      versions: ["v1", "v2"]
```

## 9.3 DNS 请求处理流程

```text
客户端请求
  ↓
协议解析 UDP/TCP（DoT/DoH/DoQ 预留）
  ↓
PROXY protocol 解析，可选
  ↓
访问控制检查
  ↓
客户端策略匹配
  ↓
本地 Zone 查询
  ↓
白名单检查
  ↓
黑名单/阻断列表/安全规则检查
  ↓
缓存查询
  ↓
递归解析或上游转发
  ↓
DNSSEC 验证
  ↓
响应生成
  ↓
查询日志异步写入
  ↓
指标统计更新
```

## 9.4 DNS 服务运行要求

- DNS 服务必须独立于 Web 服务运行。
- Web 控制台重启不应影响 DNS 查询。
- 配置变更应支持热加载。
- DNS 监听端口异常时必须给出明确日志。
- 配置错误不得导致进程反复崩溃。
- DNS 查询日志必须异步写入。
- DNS 热路径不得依赖数据库同步查询。
- 权威 Zone 与缓存数据必须有内存索引。
- 支持 IPv4 和 IPv6 双栈监听。
- 支持限制递归访问来源，默认只允许内网。

---

# 10. 递归 DNS 需求

## 10.1 递归解析模式

GoDDI 支持两种递归解析模式：

| 模式 | 说明 |
|---|---|
| Full Recursive | 从根服务器开始完整递归解析 |
| Forwarder | 转发到上游 DNS 服务器 |

## 10.2 基础递归能力

GoDDI 必须支持：

- 从根服务器开始递归解析
- 使用上游 DNS 转发解析
- 条件转发
- 多上游并发解析
- 基于延迟选择最快上游
- 上游健康检查
- 失败重试
- 超时控制
- IPv4 和 IPv6
- EDNS(0)
- EDNS Client Subnet
- Extended DNS Errors
- EDNS EXPIRE
- DNSSEC 验证
- QNAME Minimization
- QNAME Case Randomization
- Serve Stale
- Negative Cache
- DNS64
- Root hints 管理
- 本地根模式预留
- out-of-order DNS-over-TCP 处理
- out-of-order DNS-over-TLS 处理

## 10.3 上游 DNS 类型

| 类型 | 示例 |
|---|---|
| UDP | 8.8.8.8:53 |
| TCP | 8.8.8.8:53 |
| DoT | dns.example.com:853 |
| DoH | https://dns.example.com/dns-query |
| DoQ | dns.example.com:853 |
| 域名 + 固定 IP | dns.example.com (1.1.1.1) |
| HTTP Proxy | HTTP 代理转发 |
| SOCKS5 Proxy | SOCKS5 代理转发 |

## 10.4 上游选择策略

| 策略 | 说明 |
|---|---|
| sequential | 按顺序使用 |
| round_robin | 轮询 |
| random | 随机 |
| parallel_fastest | 并发请求，采用最快结果 |
| latency_best | 基于历史延迟选择 |
| health_aware | 自动跳过异常上游 |

## 10.5 条件转发

条件转发用于将特定域名后缀转发到指定 DNS 服务器。

功能要求：

- 支持域名后缀匹配
- 支持多个上游
- 支持 UDP/TCP/DoT/DoH/DoQ 上游
- 支持优先级
- 支持健康检查
- 支持 ECS 透传
- 支持日志记录
- 支持静态 Stub 解析
- 支持批量条件转发
- 支持插件增强转发策略

示例：

| 域名 | 上游 DNS |
|---|---|
| corp.local | 10.0.0.10 |
| branch.local | 10.10.0.10 |
| example.internal | 192.168.1.1 |

## 10.6 DNS64

GoDDI 在 V0.01 需为 DNS64 做预留设计：

- 预留为 IPv6-only 客户端合成 AAAA 记录的处理链路
- 预留 NAT64 前缀配置
- 预留按客户端策略启用
- 预留按域名排除
- 预留 DNS64 响应日志标记
- 支持后续以插件形式实现

配置示例：

```yaml
dns64:
  enabled: false
  prefix: "64:ff9b::/96"
  apply_to_clients:
    - "2001:db8::/32"
  exclude_domains:
    - "internal.example"
```

---

# 11. 权威 DNS 需求

## 11.1 Zone 类型

GoDDI 必须支持：

| Zone 类型 | V0.01 要求 |
|---|---|
| Primary Zone | 必须实现 |
| Secondary Zone | 必须实现 |
| Stub Zone | 必须实现 |
| Conditional Forwarder Zone | 必须实现 |
| Reverse Zone | 必须实现 |
| Catalog Zone | 预留设计，后续实现 |
| Static Stub Zone | 必须实现 |
| Split Horizon Zone | 预留设计，插件实现 |
| Geo Response Zone | 预留设计，插件实现 |

## 11.2 Zone 功能

- 创建 Zone
- 删除 Zone
- 编辑 Zone
- 启用 Zone
- 禁用 Zone
- 设置默认 TTL
- 设置 SOA
- 设置 NS
- 自动递增 Serial
- 支持日期格式 Serial
- 导入 Zone File
- 导出 Zone File
- 导入 JSON
- 导出 JSON
- 记录老化
- 记录过期自动删除
- Zone 验证
- Zone 签名
- Zone Transfer
- 动态更新
- Wildcard 子域名
- Zone 模板
- Zone 批量创建
- Zone 搜索
- Zone 权限控制

## 11.3 Zone 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string | Zone ID |
| name | string | Zone 名称 |
| type | enum | primary/secondary/stub/forward/reverse/catalog |
| enabled | bool | 是否启用 |
| default_ttl | int | 默认 TTL |
| soa_mname | string | 主 NS |
| soa_rname | string | 管理邮箱 |
| serial | uint32 | 序列号 |
| refresh | int | SOA refresh |
| retry | int | SOA retry |
| expire | int | SOA expire |
| minimum | int | SOA minimum |
| dnssec_enabled | bool | 是否启用 DNSSEC |
| transfer_policy | json | 传送策略 |
| update_policy | json | 动态更新策略 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

## 11.4 Reverse Zone

GoDDI 必须支持反向解析 Zone：

- IPv4 in-addr.arpa
- IPv6 ip6.arpa
- 从 IPAM 子网一键生成 Reverse Zone
- 从 DHCP 租约自动生成 PTR
- 从静态 IP 记录自动生成 PTR
- 支持 PTR 批量导入导出

---

# 12. DNS Record 需求

## 12.1 必须支持的记录类型

GoDDI 必须支持：

- A
- AAAA
- NS
- SOA
- CNAME
- DNAME
- MX
- TXT
- PTR
- SRV
- CAA
- TLSA
- SVCB
- HTTPS
- URI
- SSHFP
- NAPTR
- DS
- DNSKEY
- RRSIG
- NSEC
- NSEC3
- NSEC3PARAM
- APL
- CNAME Flattening Record
- APP 类记录
- Unknown RR Type

## 12.2 扩展记录

| 类型 | 说明 |
|---|---|
| ANAME 类记录 | Zone Apex 类 CNAME 能力 |
| APP 类记录 | 交由插件处理 |
| Unknown RR | 通过 RFC 3597 十六进制格式保存 |
| Flattened CNAME | 将 CNAME 结果转换为 A/AAAA 响应 |

## 12.3 记录功能

- 新增记录
- 编辑记录
- 删除记录
- 批量删除记录
- 启用记录
- 禁用记录
- 复制记录
- 搜索记录
- 按类型过滤
- 按值过滤
- TTL 快速修改
- 记录过期时间
- 记录自动老化
- 记录冲突检查
- Wildcard 子域名
- 导入解析结果到 Zone
- 支持 CNAME Cloaking 检测
- 支持自动生成 TLSA Hash
- 支持 SVCB/HTTPS 参数化编辑
- 支持批量导入 CSV
- 支持批量导入 Zone File
- 支持批量导出 CSV
- 支持记录注释
- 支持记录标签
- 支持记录责任人

## 12.4 记录字段

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string | 记录 ID |
| zone_id | string | 所属 Zone |
| name | string | 主机名 |
| type | string | 记录类型 |
| value | json/text | 记录值 |
| ttl | int | TTL |
| priority | int | MX/SRV/SVCB 优先级 |
| weight | int | SRV 权重 |
| port | int | SRV 端口 |
| enabled | bool | 是否启用 |
| expires_at | datetime | 过期时间 |
| comment | string | 备注 |
| tags | json | 标签 |
| owner | string | 负责人 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

---

# 13. DNSSEC 需求

## 13.1 DNSSEC 验证

GoDDI 必须支持：

- 递归解析 DNSSEC 验证
- 转发模式 DNSSEC 验证
- 条件转发 DNSSEC 验证
- NSEC
- NSEC3
- Trust Anchor
- DS 验证
- DNSKEY 验证
- RRSIG 验证
- 验证失败返回 SERVFAIL
- 验证失败日志
- 按域名关闭验证
- 按客户端策略关闭验证
- AD/CD 标志处理
- 验证结果在查询日志中标记

## 13.2 DNSSEC Zone 签名

GoDDI 必须支持：

- Primary Zone 签名
- KSK
- ZSK
- CSK，后续支持
- 自动生成 DNSKEY
- 自动生成 DS
- 自动生成 RRSIG
- NSEC
- NSEC3
- 自动续签
- 手动轮换密钥
- 密钥导入
- 密钥导出
- 签名状态检查
- DNSSEC 策略模板
- 密钥生命周期管理
- 过期签名告警

## 13.3 DNSSEC 算法

| 算法 | 要求 |
|---|---|
| RSASHA256 | 必须 |
| RSASHA512 | 必须 |
| ECDSAP256SHA256 | 必须 |
| ECDSAP384SHA384 | 必须 |
| ED25519 | 必须 |
| ED448 | 后续 |
| HMAC-SHA TSIG 系列 | 必须 |

---

# 14. Zone Transfer 与同步

## 14.1 基础能力

GoDDI 必须支持：

- AXFR
- IXFR
- DNS NOTIFY
- TSIG
- Secondary 自动同步
- Zone Transfer ACL
- 同步状态
- 同步失败日志
- 手动触发同步
- SOA Serial 检查

## 14.2 高级能力

| 能力 | 要求 |
|---|---|
| XFR-over-TLS | 预留设计，后续实现 |
| XFR-over-QUIC | 预留设计，后续实现 |
| ZONEMD | 预留设计，后续实现 |
| Catalog Zone | 预留设计，后续实现 |

## 14.3 传送策略

```yaml
zone_transfer:
  enabled: true
  allow:
    - "10.0.0.10"
    - "10.0.0.11"
  tsig_required: true
  tls_required: false
  quic_required: false
```

## 14.4 Catalog Zone

Catalog Zone 用于自动向多个从节点分发 Zone。

需求：

- 支持 Catalog Zone 数据模型
- 支持成员 Zone 管理
- 支持节点订阅 Catalog Zone
- 支持自动创建 Secondary Zone
- 支持同步状态查看
- 支持失败重试
- 支持权限控制

---

# 15. Dynamic DNS Update 需求

GoDDI 必须支持动态 DNS 更新。

## 15.1 功能

- 添加记录
- 删除记录
- 替换记录
- 条件更新
- TSIG 鉴权
- 客户端 IP ACL
- 按 Zone 控制是否允许更新
- 按记录类型控制
- DHCP 自动更新 DNS
- 更新日志
- 更新审计
- 更新失败告警

## 15.2 安全策略

| 策略 | 说明 |
|---|---|
| disabled | 禁止动态更新 |
| allow_acl | 指定 IP 可更新 |
| tsig_required | 必须 TSIG |
| dhcp_only | 仅 DHCP 模块可更新 |
| record_owner | 仅记录所有者可更新 |

---

# 16. DNS 缓存需求

## 16.1 缓存能力

GoDDI 必须支持：

- 正向缓存
- 负向缓存
- NXDOMAIN 缓存
- NODATA 缓存
- Serve Stale
- Prefetch
- Auto Prefetch
- 持久化缓存
- 缓存清理
- 单域名缓存清理
- 按类型缓存清理
- 缓存导出
- 缓存统计
- 缓存命中率
- 缓存内存限制
- 缓存条目限制
- 缓存热加载
- 缓存过期清理
- 缓存污染防护

## 16.2 缓存配置

```yaml
cache:
  enabled: true
  max_entries: 1000000
  min_ttl: 30
  max_ttl: 86400
  negative_ttl: 300
  serve_stale: true
  stale_ttl: 3600
  prefetch: true
  auto_prefetch: true
  persistent: true
  persistent_file: "/var/lib/goddi/cache.db"
```

---

# 17. DNS 安全过滤需求

## 17.1 阻断列表

GoDDI 必须支持：

- 一个或多个阻断列表 URL
- 本地阻断列表
- 手工阻断域名
- 批量导入域名
- 自动更新阻断列表
- 更新失败告警
- 正则阻断规则
- 通配符阻断规则
- 后缀匹配
- 精确匹配
- 客户端分组阻断策略
- 子网级阻断策略
- DNSBL/RBL 托管
- TXT 阻断报告，配置可选

## 17.2 阻断响应

| 响应方式 | 说明 |
|---|---|
| NXDOMAIN | 返回不存在 |
| NODATA | 返回空结果 |
| 0.0.0.0 | 返回空 IPv4 |
| :: | 返回空 IPv6 |
| REFUSED | 拒绝 |
| Custom IP | 返回指定 IP |
| Drop | 直接丢弃 |
| TXT Report | 返回阻断报告 |

## 17.3 白名单

- 白名单优先级高于黑名单
- 支持域名白名单
- 支持客户端白名单
- 支持子网白名单
- 支持临时白名单
- 支持白名单命中日志
- 支持白名单导入导出

## 17.4 CNAME Cloaking 防护

GoDDI 必须支持：

- 递归检查 CNAME 链
- 若最终 CNAME 命中阻断规则，则阻断原始查询
- 支持最大 CNAME 递归深度
- 支持日志标记
- 支持按策略启用或禁用

## 17.5 DNS Rebinding 防护

GoDDI 必须支持：

- 阻止公网域名解析到私有地址
- 支持例外域名
- 支持例外客户端
- 支持例外 IP 段
- 支持日志记录
- 支持按客户端策略启用
- 支持 IPv4 与 IPv6 私有地址段

## 17.6 DNS 放大攻击防护

GoDDI 必须支持：

- ANY 查询限制
- UDP 响应大小限制
- EDNS 响应大小限制
- Response Rate Limiting
- 客户端 QPS 限制
- 域名 QPS 限制
- 失败响应限速
- 异常请求丢弃
- 递归访问 ACL
- 默认不允许公网开放递归

---

# 18. DNS Apps / 插件系统需求

## 18.1 插件定位

GoDDI 需要设计插件系统，用于扩展 DNS 请求处理逻辑。

插件不应影响核心 DNS 服务稳定性。插件异常必须隔离，不得导致主进程崩溃。

V0.01 仅要求定义运行时边界、权限模型和配置结构，不强制交付完整插件运行时。

## 18.2 插件能力

插件可实现：

- 高级阻断
- 高级转发
- DNS64
- Split Horizon
- Geo Response
- 自定义记录响应
- APP 类记录处理
- DNS 请求审计增强
- 特殊业务逻辑解析
- DNSBL/RBL 服务
- 自定义策略
- 威胁情报订阅
- 企业内网特殊解析逻辑

## 18.3 插件生命周期

- 安装插件
- 卸载插件
- 启用插件
- 禁用插件
- 配置插件
- 查看插件日志
- 插件版本管理
- 插件升级
- 插件权限声明
- 插件资源限制
- 插件健康检查
- 插件异常自动熔断

## 18.4 插件安全

- 插件默认沙箱运行
- 插件不可直接访问数据库
- 插件通过受控 SDK 访问上下文
- 插件必须声明权限
- 插件异常不能导致 DNS 主进程崩溃
- 插件执行必须有超时限制
- 插件支持本地安装，不强依赖外部市场
- 插件必须支持签名校验，后续实现
- 插件必须有资源限制，包括 CPU、内存、并发数

---

# 19. DNS Client 需求

GoDDI 必须提供内置 DNS Client，用于管理端调试。

## 19.1 功能

- 输入域名查询
- 选择记录类型
- 选择查询协议
- 选择指定上游
- 支持 UDP/TCP/DoT/DoH/DoQ
- 显示完整 DNS 响应
- 显示响应时间
- 显示权威标记
- 显示 DNSSEC AD/CD 位
- 显示 EDNS 信息
- 支持原始响应查看
- 支持将查询结果导入本地 Zone
- 支持批量测试
- 支持历史记录
- 支持复制结果
- 支持导出结果
- 支持查询链路诊断

---

# 20. DHCP Server 需求

## 20.1 DHCPv4 基础能力

GoDDI 必须支持 DHCPv4：

- DISCOVER
- OFFER
- REQUEST
- ACK
- NAK
- DECLINE
- RELEASE
- INFORM
- 多网络
- 多 Scope
- 多网卡监听
- 地址池
- 租约
- 保留地址
- 排除地址
- 冲突检测
- Ping 检测
- DNS 联动
- DHCP 日志
- DHCP 服务状态监控

## 20.2 Scope 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string | Scope ID |
| name | string | 名称 |
| interface | string | 监听网卡 |
| subnet | cidr | 子网 |
| start_ip | ip | 起始地址 |
| end_ip | ip | 结束地址 |
| subnet_mask | ip | 子网掩码 |
| router | ip | 默认网关 |
| lease_time | int | 默认租期 |
| max_lease_time | int | 最大租期 |
| domain_name | string | 域名 |
| dns_servers | array | DNS 服务器 |
| ntp_servers | array | NTP 服务器 |
| enabled | bool | 是否启用 |
| ping_check_enabled | bool | 是否启用冲突检测 |
| dns_updates | bool | 是否自动更新 DNS |
| comment | string | 备注 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

## 20.3 DHCP Option

必须支持：

| Option | 名称 |
|---:|---|
| 1 | Subnet Mask |
| 3 | Router |
| 6 | DNS Servers |
| 12 | Host Name |
| 15 | Domain Name |
| 42 | NTP Servers |
| 44 | WINS/NBNS Servers |
| 51 | Lease Time |
| 54 | DHCP Server Identifier |
| 58 | Renewal Time |
| 59 | Rebinding Time |
| 66 | TFTP Server |
| 67 | Bootfile Name |
| 81 | Client FQDN |
| 119 | Domain Search |
| 121 | Classless Static Route |
| 138 | CAPWAP AC |
| 150 | TFTP Server Address |

## 20.4 Option 优先级

```text
Reservation Option > Client Class Option > Scope Option > Global Option
```

## 20.5 DHCP 与 DNS 联动

GoDDI 必须支持：

- DHCP 分配地址后自动创建 A 记录
- DHCP 分配地址后自动创建 PTR 记录
- 租约释放后自动删除动态记录
- 保留地址记录永久保留
- 支持覆盖动态租约 DNS 记录
- 支持设置 DNS TTL
- 支持按 Scope 配置是否联动 DNS
- 支持 DHCP Client FQDN
- 支持反向解析 Zone 自动匹配
- 支持更新失败重试
- 支持更新失败告警
- 支持 DHCP 记录所有权标记

## 20.6 Lease 管理

- 查看活跃租约
- 查看过期租约
- 查看释放租约
- 手动释放租约
- 租约转保留
- 按 IP 搜索
- 按 MAC 搜索
- 按 Hostname 搜索
- 导出租约
- 查看历史租约
- 查看租约变更记录
- 查看租约对应 DNS 记录
- 查看租约对应 IPAM 地址

## 20.7 Reservation 管理

功能：

- 创建保留地址
- 修改保留地址
- 删除保留地址
- 启用/禁用保留地址
- 从租约转为保留地址
- 保留地址联动 DNS
- 保留地址联动 IPAM
- 支持备注、标签、负责人

---

# 21. IPAM 需求

## 21.1 地址空间

GoDDI 必须支持：

- IPv4 地址空间
- IPv6 地址空间
- 地址空间分层
- 子网划分
- 子网合并
- 子网重叠检测
- VLAN 标记
- 位置标记
- 用途标记
- 负责人
- 标签
- 备注
- 导入导出

## 21.2 子网管理

功能：

- 创建子网
- 编辑子网
- 删除子网
- 查看使用率
- 查看地址矩阵
- 生成 DHCP Scope
- 生成反向解析 Zone
- 导入子网
- 导出子网
- 地址冲突检测
- 子网重叠检测
- 子网容量统计
- 子网预留地址管理
- 子网标签管理

## 21.3 IP 地址状态

| 状态 | 说明 |
|---|---|
| available | 可用 |
| used | 已使用 |
| reserved | 保留 |
| dhcp | DHCP 动态分配 |
| static | 静态分配 |
| gateway | 网关 |
| excluded | 排除 |
| conflict | 冲突 |
| unknown | 未知 |

## 21.4 IP 地址字段

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string | IP ID |
| subnet_id | string | 所属子网 |
| ip_address | string | IP 地址 |
| status | enum | 地址状态 |
| mac_address | string | MAC 地址 |
| hostname | string | 主机名 |
| dns_record_id | string | DNS 记录 |
| dhcp_lease_id | string | DHCP 租约 |
| owner | string | 负责人 |
| device | string | 设备名称 |
| location | string | 位置 |
| description | string | 备注 |
| last_seen | datetime | 最后发现 |
| created_at | datetime | 创建时间 |
| updated_at | datetime | 更新时间 |

## 21.5 IPAM 联动

IPAM 必须与 DNS、DHCP 联动：

- DHCP 租约自动同步为 IP 状态
- DHCP 保留地址同步为 reserved
- DNS A/AAAA/PTR 记录可关联 IP
- IP 状态变更可提示是否创建 DNS 记录
- 子网可一键生成 DHCP Scope
- 子网可一键生成 Reverse Zone
- 删除子网时检查 DHCP 和 DNS 依赖
- IP 冲突时生成告警
- IP 使用率过高时生成告警

---

# 22. 用户、权限与认证

## 22.1 用户功能

GoDDI 必须支持：

- 本地用户
- 用户组
- 角色
- 权限
- 用户禁用
- 修改密码
- 重置密码
- 登录历史
- 会话列表
- 删除会话
- Session 超时
- API Token
- 单次使用 Token
- TOTP 2FA
- OIDC SSO（预留）

## 22.2 内置角色

| 角色 | 权限 |
|---|---|
| Super Admin | 全部权限 |
| DNS Admin | DNS 管理 |
| DHCP Admin | DHCP 管理 |
| IPAM Admin | IPAM 管理 |
| Security Admin | 安全策略管理 |
| App Admin | 插件管理 |
| Auditor | 日志与审计只读 |
| Operator | 日常运维 |
| Viewer | 只读 |

## 22.3 权限模块

权限必须覆盖：

- Dashboard
- Zones
- Records
- Cache
- Allowed
- Blocked
- Apps
- DNS Client
- Settings
- DHCP Server
- IPAM
- Administration
- Logs
- Backup
- Cluster
- Metrics
- Security
- Tokens
- Sessions

## 22.4 权限动作

每个模块至少支持：

- can_view
- can_create
- can_update
- can_delete
- can_execute
- can_export
- can_import

## 22.5 API Token

支持两类 Token：

| 类型 | 说明 |
|---|---|
| Long-lived Token | 长期自动化使用 |
| Single-use Token | 使用一次后立即失效 |

Token 要求：

- 创建后只显示一次
- 存储哈希值
- 支持名称
- 支持过期时间
- 支持权限范围
- 支持禁用
- 支持删除
- 支持最后使用时间
- 支持来源 IP 记录
- 支持审计日志
- 支持 IP 限制
- 支持只读 Token
- 支持自动轮换提醒

## 22.6 OIDC SSO

GoDDI 必须预留并逐步实现：

- OIDC Authority
- Client ID
- Client Secret
- Discovery URL
- Scope 配置
- 自动注册开关
- 只允许映射用户注册
- 远程组到本地组映射
- 登录后同步组
- SSO 用户禁用
- SSO 登录审计

---

# 23. Web Console 需求

## 23.1 页面结构

```text
GoDDI
├── Dashboard
├── DNS
│   ├── Zones
│   ├── Records
│   ├── Recursive Resolver
│   ├── Forwarders
│   ├── Conditional Forwarders
│   ├── Cache
│   ├── DNSSEC
│   ├── Zone Transfer
│   ├── Dynamic Updates
│   ├── DNS Client
│   └── Query Logs
├── Security
│   ├── Allowed
│   ├── Blocked
│   ├── Block Lists
│   ├── Client Policies
│   ├── Rebinding Protection
│   ├── CNAME Cloaking
│   └── Rate Limit
├── Apps
│   ├── Installed Apps
│   ├── App Settings
│   └── App Logs
├── DHCP
│   ├── Scopes
│   ├── Leases
│   ├── Reservations
│   ├── Options
│   └── DHCP Logs
├── IPAM
│   ├── Address Spaces
│   ├── Subnets
│   ├── IP Addresses
│   └── Usage
├── Cluster
│   ├── Nodes
│   ├── Sync Status
│   └── Cluster Settings
├── Administration
│   ├── Users
│   ├── Groups
│   ├── Roles
│   ├── Sessions
│   ├── API Tokens
│   ├── SSO
│   └── 2FA
├── Logs
│   ├── System Logs
│   ├── Query Logs
│   ├── Audit Logs
│   └── DHCP Logs
├── Settings
│   ├── DNS Settings
│   ├── Web Settings
│   ├── Certificate
│   ├── Proxy
│   ├── Backup
│   └── Update
```

## 23.2 UI 要求

- 支持浅色模式
- 支持深色模式
- 支持中文
- 支持英文
- 支持响应式布局
- 支持表格分页
- 支持批量操作
- 支持导入导出
- 所有危险操作二次确认
- 所有写操作显示成功/失败结果
- 所有表单必须前后端双重校验
- 支持顶部全局搜索
- 支持页面级权限控制
- 支持操作完成后自动刷新数据
- 支持长任务进度条
- 支持系统通知中心

---

# 24. REST API 需求

## 24.1 API 原则

- REST 风格
- JSON 请求
- JSON 响应
- Bearer Token 鉴权
- 支持分页
- 支持过滤
- 支持排序
- 支持 OpenAPI 文档
- 支持 API Version
- 支持错误码
- 支持审计日志
- Web Console 所有操作必须通过 API 完成
- 上传文件使用 multipart/form-data
- 批量操作使用异步任务

## 24.2 API 分组

未纳入 V0.01 必做范围的分组，可先保留接口预留定义，不默认开放。

| 模块 | API 前缀 |
|---|---|
| Auth | /api/v1/auth |
| SSO | /api/v1/sso |
| User | /api/v1/users |
| Group | /api/v1/groups |
| Role | /api/v1/roles |
| Session | /api/v1/sessions |
| Token | /api/v1/tokens |
| Dashboard | /api/v1/dashboard |
| Metrics JSON | /api/v1/metrics/json |
| Metrics Prometheus | /metrics |
| DNS Zone | /api/v1/dns/zones |
| DNS Record | /api/v1/dns/records |
| DNS Cache | /api/v1/dns/cache |
| DNS Client | /api/v1/dns/client |
| DNS Forwarder | /api/v1/dns/forwarders |
| DNS Security | /api/v1/dns/security |
| DNS Apps | /api/v1/apps |
| DHCP Scope | /api/v1/dhcp/scopes |
| DHCP Lease | /api/v1/dhcp/leases |
| DHCP Reservation | /api/v1/dhcp/reservations |
| DHCP Option | /api/v1/dhcp/options |
| IPAM | /api/v1/ipam |
| Cluster | /api/v1/cluster |
| Logs | /api/v1/logs |
| Settings | /api/v1/settings |
| Backup | /api/v1/backup |

## 24.3 API 响应格式

成功：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

失败：

```json
{
  "code": 40001,
  "message": "invalid request",
  "detail": "zone name is required"
}
```

分页：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "page": 1,
    "page_size": 20,
    "total": 100
  }
}
```

---

# 25. Dashboard 与 Metrics

## 25.1 Dashboard 指标

- 总查询数
- NOERROR 数
- SERVFAIL 数
- NXDOMAIN 数
- REFUSED 数
- 权威查询数
- 递归查询数
- 缓存命中数
- 阻断数
- 丢弃数
- 客户端数量
- 当前 QPS
- 平均响应时间
- 上游延迟
- DHCP 活跃租约
- DHCP 地址池使用率
- IPAM 使用率
- 集群节点状态
- 系统运行时间
- 版本信息
- CPU / 内存 / 磁盘概览

## 25.2 Prometheus 指标

GoDDI 必须提供：

```text
/metrics
```

指标示例：

- goddi_uptime_seconds
- goddi_dns_queries_total
- goddi_dns_noerror_total
- goddi_dns_servfail_total
- goddi_dns_nxdomain_total
- goddi_dns_refused_total
- goddi_dns_authoritative_total
- goddi_dns_recursive_total
- goddi_dns_cached_total
- goddi_dns_blocked_total
- goddi_dns_dropped_total
- goddi_dns_clients_total
- goddi_dns_response_duration_seconds
- goddi_dhcp_leases_active
- goddi_dhcp_scope_usage_ratio
- goddi_cluster_nodes_total
- goddi_api_requests_total
- goddi_api_request_duration_seconds
- goddi_db_errors_total
- goddi_backup_jobs_total

---

# 26. 日志需求

## 26.1 系统日志

记录：

- 服务启动
- 服务停止
- 配置加载
- 配置修改
- DNS 服务异常
- DHCP 服务异常
- 数据库异常
- 插件异常
- 集群异常
- 备份恢复
- 证书加载
- 上游 DNS 健康变化
- 阻断列表更新

## 26.2 查询日志

字段：

| 字段 | 说明 |
|---|---|
| timestamp | 时间 |
| client_ip | 客户端 IP |
| client_mac | 客户端 MAC，可选 |
| protocol | UDP/TCP/DoT/DoH/DoQ |
| qname | 查询名称 |
| qtype | 查询类型 |
| rcode | 响应码 |
| answer | 响应摘要 |
| latency_ms | 延迟 |
| cache_hit | 是否缓存命中 |
| blocked | 是否阻断 |
| upstream | 上游 |
| policy | 策略 |
| dnssec | DNSSEC 状态 |

## 26.3 DHCP 日志

记录：

- DISCOVER
- OFFER
- REQUEST
- ACK
- NAK
- DECLINE
- RELEASE
- INFORM
- 地址分配
- 地址释放
- 租约过期
- 保留地址命中
- 地址冲突
- DNS 联动成功
- DNS 联动失败

## 26.4 审计日志

记录：

- 登录
- 登出
- 登录失败
- 修改密码
- 启用 2FA
- 禁用 2FA
- 创建用户
- 删除用户
- 修改权限
- 创建 Zone
- 删除 Zone
- 修改记录
- 修改 DHCP Scope
- 修改安全策略
- 安装插件
- 修改系统配置
- 备份恢复
- 创建 API Token
- 删除 API Token
- 使用单次 Token

---

# 27. 集群需求

## 27.1 节点管理

GoDDI 支持：

- 单节点运行
- 多节点注册
- 节点心跳
- 节点状态
- 节点版本
- 节点指标
- 节点日志
- 节点删除
- 节点标签
- 节点角色
- 节点健康检查

## 27.2 集群管理

- 一个控制台管理多个节点
- 聚合 Dashboard
- 聚合查询统计
- 聚合健康状态
- 配置同步
- Zone 同步
- 安全策略同步
- 阻断列表同步
- 用户权限同步
- 插件配置同步
- 节点状态告警
- 同步失败重试
- 同步任务历史

## 27.3 DHCP 高可用

V0.01 不强制实现 DHCP HA，但必须预留：

- 主备模式
- 租约同步
- 故障切换
- Split Scope
- 节点优先级
- 冲突避免机制

---

# 28. 备份恢复与导入导出

## 28.1 备份内容

- 系统配置
- DNS Zone
- DNS Record
- DNSSEC Key
- 阻断规则
- 允许规则
- DHCP Scope
- DHCP Lease
- DHCP Reservation
- IPAM 数据
- 用户
- 角色
- 权限
- 插件配置
- 审计日志，可选
- 查询日志，可选
- 证书文件，可选

## 28.2 备份功能

- 手动备份
- 定时备份
- 下载备份
- 上传恢复
- 恢复前校验
- 恢复前自动快照
- 分模块恢复
- 加密备份
- 备份保留策略
- 备份任务日志
- 备份失败告警

## 28.3 导入导出格式

| 数据类型 | 格式 |
|---|---|
| DNS Zone | BIND Zone File / JSON |
| DNS Record | CSV / JSON |
| DHCP Scope | CSV / JSON |
| DHCP Lease | CSV / JSON |
| DHCP Reservation | CSV / JSON |
| IPAM Subnet | CSV / JSON |
| IPAM Address | CSV / JSON |
| Block List | TXT / CSV |
| Allow List | TXT / CSV |
| Users | JSON，仅管理员可操作 |
| Settings | YAML / JSON |

---

# 29. 配置文件

## 29.1 config.yaml 示例

```yaml
server:
  name: "goddi-01"
  http_addr: "0.0.0.0:5380"
  public_url: "http://127.0.0.1:5380"
  data_dir: "/var/lib/goddi"
  language: "zh-CN"
  dark_mode: false

license:
  type: "MIT"

database:
  driver: "sqlite"
  dsn: "/var/lib/goddi/goddi.db"

dns:
  enabled: true
  domain: "server1.local"
  default_ttl: 3600
  recursion:
    enabled: true
    allow_nets:
      - "10.0.0.0/8"
      - "172.16.0.0/12"
      - "192.168.0.0/16"
  listeners:
    udp:
      enabled: true
      address: ":53"
    tcp:
      enabled: true
      address: ":53"
    dot:
      enabled: false
      address: ":853"
    doh:
      enabled: false
      address: ":8053"
      path: "/dns-query"
    doq:
      enabled: false
      address: ":8853"

cache:
  enabled: true
  max_entries: 1000000
  serve_stale: true
  prefetch: true
  persistent: true

forwarders:
  mode: "parallel_fastest"
  servers:
    - name: "default-udp"
      protocol: "udp"
      address: "8.8.8.8:53"

dhcp:
  enabled: false
  interfaces:
    - "eth0"

ipam:
  enabled: true

security:
  jwt_secret: "change-me"
  login_rate_limit: 5
  totp_enabled: true
  rebinding_protection: true

proxy:
  enabled: false
  type: "socks5"
  address: "127.0.0.1:9050"

metrics:
  enabled: true
  path: "/metrics"

log:
  level: "info"
  query_log_enabled: true
  retention_days: 30
```

## 29.2 环境变量

GoDDI Docker 部署必须支持通过环境变量初始化：

- GODDI_SERVER_NAME
- GODDI_ADMIN_USERNAME
- GODDI_ADMIN_PASSWORD
- GODDI_ADMIN_PASSWORD_FILE
- GODDI_HTTP_ADDR
- GODDI_ENABLE_HTTPS
- GODDI_TLS_CERT_FILE
- GODDI_TLS_KEY_FILE
- GODDI_DNS_ENABLE_UDP
- GODDI_DNS_ENABLE_TCP
- GODDI_DNS_ENABLE_DOT
- GODDI_DNS_ENABLE_DOH
- GODDI_RECURSION_MODE
- GODDI_RECURSION_ACL
- GODDI_ENABLE_BLOCKING
- GODDI_BLOCK_LIST_URLS
- GODDI_FORWARDERS
- GODDI_FORWARDER_PROTOCOL
- GODDI_LOG_LEVEL
- GODDI_LOG_RETENTION_DAYS
- GODDI_STATS_IN_MEMORY
- GODDI_SSO_ENABLED
- GODDI_SSO_AUTHORITY
- GODDI_SSO_CLIENT_ID
- GODDI_SSO_CLIENT_SECRET
- GODDI_SSO_SCOPES

---

# 30. 数据库设计

## 30.1 核心表

```text
users
groups
roles
permissions
role_permissions
user_groups
user_roles
group_roles
sessions
login_history
api_tokens
token_ip_restrictions
user_totp_secrets
user_recovery_codes
sso_settings
oidc_user_links

dns_zones
dns_records
dns_zone_transfer
dns_dynamic_update_policies
dns_dnssec_keys
dns_cache
dns_forwarders
dns_conditional_forwarders
dns_client_history
dns_block_lists
dns_block_rules
dns_allow_rules
dns_client_policies
dns_query_logs

apps
app_settings
app_logs

dhcp_scopes
dhcp_leases
dhcp_reservations
dhcp_options
dhcp_logs

ipam_spaces
ipam_subnets
ipam_addresses
ipam_history

cluster_nodes
cluster_sync_tasks

system_settings
audit_logs
backup_jobs
backup_files
```

## 30.2 dns_zones

```sql
CREATE TABLE dns_zones (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    dnssec_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    default_ttl INTEGER NOT NULL DEFAULT 3600,
    soa_mname TEXT NOT NULL,
    soa_rname TEXT NOT NULL,
    serial INTEGER NOT NULL,
    refresh INTEGER NOT NULL DEFAULT 3600,
    retry INTEGER NOT NULL DEFAULT 600,
    expire INTEGER NOT NULL DEFAULT 86400,
    minimum INTEGER NOT NULL DEFAULT 300,
    transfer_policy TEXT,
    update_policy TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

## 30.3 dns_records

```sql
CREATE TABLE dns_records (
    id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    value TEXT NOT NULL,
    ttl INTEGER NOT NULL DEFAULT 300,
    priority INTEGER,
    weight INTEGER,
    port INTEGER,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    comment TEXT,
    tags TEXT,
    owner TEXT,
    expires_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY(zone_id) REFERENCES dns_zones(id)
);
```

## 30.4 dhcp_scopes

```sql
CREATE TABLE dhcp_scopes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    interface TEXT,
    subnet TEXT NOT NULL,
    start_ip TEXT NOT NULL,
    end_ip TEXT NOT NULL,
    subnet_mask TEXT,
    router TEXT,
    dns_servers TEXT,
    ntp_servers TEXT,
    domain_name TEXT,
    lease_time INTEGER NOT NULL DEFAULT 86400,
    max_lease_time INTEGER,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ping_check_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    dns_updates BOOLEAN NOT NULL DEFAULT FALSE,
    comment TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

## 30.5 dhcp_leases

```sql
CREATE TABLE dhcp_leases (
    id TEXT PRIMARY KEY,
    scope_id TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    mac_address TEXT NOT NULL,
    hostname TEXT,
    client_id TEXT,
    lease_start DATETIME NOT NULL,
    lease_end DATETIME NOT NULL,
    status TEXT NOT NULL,
    last_seen DATETIME NOT NULL,
    FOREIGN KEY(scope_id) REFERENCES dhcp_scopes(id)
);
```

## 30.6 ipam_addresses

```sql
CREATE TABLE ipam_addresses (
    id TEXT PRIMARY KEY,
    subnet_id TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    status TEXT NOT NULL,
    mac_address TEXT,
    hostname TEXT,
    dns_record_id TEXT,
    dhcp_lease_id TEXT,
    owner TEXT,
    device TEXT,
    location TEXT,
    description TEXT,
    last_seen DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

## 30.7 认证与权限补充表

```sql
CREATE TABLE user_roles (
    user_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (user_id, role_id),
    FOREIGN KEY(user_id) REFERENCES users(id),
    FOREIGN KEY(role_id) REFERENCES roles(id)
);

CREATE TABLE group_roles (
    group_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (group_id, role_id),
    FOREIGN KEY(group_id) REFERENCES groups(id),
    FOREIGN KEY(role_id) REFERENCES roles(id)
);

CREATE TABLE login_history (
    id TEXT PRIMARY KEY,
    user_id TEXT,
    username TEXT NOT NULL,
    auth_type TEXT NOT NULL,
    source_ip TEXT,
    user_agent TEXT,
    success BOOLEAN NOT NULL,
    reason TEXT,
    created_at DATETIME NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE user_totp_secrets (
    user_id TEXT PRIMARY KEY,
    secret_ciphertext TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE user_recovery_codes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    code_hash TEXT NOT NULL,
    used_at DATETIME,
    created_at DATETIME NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE token_ip_restrictions (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    cidr TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY(token_id) REFERENCES api_tokens(id)
);

CREATE TABLE oidc_user_links (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    email TEXT,
    groups_snapshot TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE(issuer, subject),
    FOREIGN KEY(user_id) REFERENCES users(id)
);
```

---

# 31. 后端工程结构

```text
cmd/
  goddi/
    main.go

internal/
  api/
    handler/
    middleware/
    router/
  auth/
  rbac/
  audit/
  config/
  database/
    migrations/
    queries/
  dns/
    server/
    resolver/
    authoritative/
    cache/
    forwarder/
    dnssec/
    filter/
    zone/
    client/
    transfer/
    dynamic_update/
  dhcp/
    server/
    lease/
    scope/
    option/
    reservation/
  ipam/
    space/
    subnet/
    address/
  apps/
    runtime/
    sdk/
  cluster/
  metrics/
  backup/
  log/
  system/
  task/

pkg/
  netutil/
  dnsutil/
  iputil/
  timeutil/
```

---

# 32. 部署需求

## 32.1 Linux 单文件部署

目录：

```text
/usr/local/bin/goddi
/etc/goddi/config.yaml
/var/lib/goddi
/var/log/goddi
```

systemd：

```ini
[Unit]
Description=GoDDI DDI Server
After=network.target

[Service]
ExecStart=/usr/local/bin/goddi serve --config /etc/goddi/config.yaml
Restart=always
User=goddi
Group=goddi
AmbientCapabilities=CAP_NET_BIND_SERVICE CAP_NET_RAW
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```

## 32.2 Docker Compose

```yaml
services:
  goddi:
    image: goddi/goddi:latest
    container_name: goddi
    network_mode: host
    volumes:
      - ./data:/var/lib/goddi
      - ./config.yaml:/etc/goddi/config.yaml
    environment:
      - GODDI_ADMIN_USERNAME=admin
      - GODDI_ADMIN_PASSWORD=change-me
      - GODDI_HTTP_ADDR=0.0.0.0:5380
    restart: unless-stopped
```

## 32.3 容器部署注意事项

- DNS 服务需要暴露 UDP/TCP 53。
- DHCP 服务需要二层广播，推荐 `network_mode: host`。
- 后续启用 DoT 时需要暴露 853。
- 后续启用 DoH 时需要暴露 443 或 8053。
- 数据目录必须持久化。
- 管理员密码必须支持环境变量或文件方式初始化。
- 生产环境必须修改默认密码。
- 容器日志默认输出到 stdout，同时支持文件日志。
- V0.01 不提供容器编排模板或编排控制器。

---

# 33. 性能要求

## 33.1 DNS 性能

| 指标 | 要求 |
|---|---|
| UDP DNS QPS | 单机 ≥ 50,000 |
| 缓存命中响应 | P95 < 5ms |
| 普通递归响应 | P95 < 100ms |
| TCP 并发连接 | ≥ 10,000 |
| 加密 DNS 并发连接（后续） | ≥ 5,000 |
| 缓存条目 | ≥ 1,000,000 |
| 查询日志写入 | 异步批量 |
| 配置热更新 | 不重启 DNS 服务 |

## 33.2 DHCP 性能

| 指标 | 要求 |
|---|---|
| Scope 数量 | ≥ 1,000 |
| 活跃租约 | ≥ 100,000 |
| 分配延迟 | P95 < 100ms |
| 租约查询 | 支持分页和索引 |
| 冲突检测 | 异步执行 |

## 33.3 Web/API 性能

| 指标 | 要求 |
|---|---|
| API P95 | < 300ms |
| Dashboard 刷新 | 5 秒 |
| 日志查询 | 百万级分页 |
| 导入导出 | 支持异步任务 |
| 长任务 | 支持进度查询 |

---

# 34. 安全要求

## 34.1 默认安全

- 默认禁止公网递归
- 默认要求首次登录修改密码
- 默认管理端不暴露到公网
- 默认启用登录失败限速
- 默认启用 CSRF 防护
- 默认启用安全 Cookie
- 默认禁用 Zone Transfer
- 默认禁用 Dynamic DNS Update
- 默认禁用不可信插件
- 默认不记录敏感明文 Token
- 默认 API Token 创建后只显示一次
- 默认危险操作需要二次确认

## 34.2 DNS 安全

- DNSSEC
- TSIG
- Rate Limit
- ANY 查询限制
- EDNS 响应大小限制
- Rebinding 防护
- CNAME Cloaking 防护
- DNS 放大攻击防护
- 客户端 ACL
- 递归访问控制
- Zone Transfer ACL

## 34.3 Web 安全

- Argon2id 密码哈希
- JWT 短期 Session
- API Token 哈希存储
- TOTP
- OIDC（预留）
- RBAC
- 审计日志
- HTTPS
- 安全响应头
- IP 访问限制
- 登录失败锁定
- 会话管理
- 密码复杂度策略

---

# 35. 告警需求

## 35.1 告警事件

- DNS 服务异常
- DHCP 服务异常
- 上游 DNS 不可用
- Zone 同步失败
- Zone 签名即将过期
- DHCP 地址池使用率超过阈值
- IP 地址冲突
- DNSSEC 验证异常
- 多节点启用后：集群节点离线
- 登录失败过多
- 阻断列表更新失败
- 数据库异常
- 证书即将过期
- 备份失败
- 插件异常
- 磁盘空间不足

## 35.2 告警方式

V0.01 必须支持：

- Web 控制台通知
- 系统日志
- Webhook

后续支持：

- 邮件
- 企业 IM
- Prometheus Alertmanager
- 短信网关

---

# 36. V0.01 开发范围

## 36.1 V0.01 必须实现

第一版必须完成：

说明：V0.01 以单机和小规模试点为目标，M1-M6 属于首版交付范围。

```text
DNS：
- UDP/TCP DNS
- 权威 DNS
- 递归转发
- 条件转发
- DNS 缓存
- Primary Zone
- Secondary Zone
- Stub Zone
- 常用记录管理
- AXFR/IXFR/NOTIFY
- TSIG
- DNSSEC 验证
- 查询日志
- 阻断列表
- 白名单
- 客户端策略
- DNS Client

DHCP：
- DHCPv4
- Scope
- Lease
- Reservation
- 常用 DHCP Option
- Ping 冲突检测
- DNS 自动更新

IPAM：
- 地址空间
- 子网
- IP 地址
- DHCP/IPAM 联动
- DNS/IPAM 联动

管理：
- Web Console
- REST API
- 用户
- 角色
- 权限
- API Token
- 单次 Token
- TOTP 2FA
- 会话管理
- 系统日志
- 查询日志
- DHCP 日志
- 审计日志
- Prometheus Metrics
- 备份恢复
- Docker
- systemd
- MIT License
```

## 36.2 V0.01 预留设计

以下功能必须在架构、数据库、API、配置中预留：

```text
- DoT
- DoH HTTP/1.1
- DoH HTTP/2
- DoQ
- DoH HTTP/3
- DNS over PROXY protocol
- XFR-over-TLS
- XFR-over-QUIC
- ZONEMD
- Catalog Zone
- DNS64
- DNSBL/RBL
- Plugin/App Runtime
- Split Horizon
- Geo Response
- OIDC SSO
- HTTP/SOCKS5 Proxy
- Cluster Node Management
- Cluster Sync
- Container Orchestration
- DHCP HA
```

## 36.3 后续增强实现

```text
- 完整插件市场
- 高级 DNS App SDK
- 多节点集群配置同步
- DHCP 主备高可用
- GeoDNS
- 多租户
- 容器编排控制器
- 高级报表
- 威胁情报订阅
- 外部 SIEM 集成
```

---

# 37. 功能覆盖核查清单

## 37.1 DNS 协议覆盖

| 功能 | 状态 |
|---|---|
| DNS UDP | 已纳入 |
| DNS TCP | 已纳入 |
| DoT | 已预留 |
| DoH HTTP/1.1 | 已预留 |
| DoH HTTP/2 | 已预留 |
| DoH HTTP/3 | 已预留 |
| DoQ | 已预留 |
| PROXY protocol v1/v2 | 已预留 |
| EDNS(0) | 已纳入 |
| ECS | 已纳入 |
| Extended DNS Errors | 已纳入 |
| EDNS EXPIRE | 已纳入 |
| QNAME Minimization | 已纳入 |
| QNAME Case Randomization | 已纳入 |
| Serve Stale | 已纳入 |
| DNS64 | 已预留 |
| DNSBL/RBL | 已预留 |

## 37.2 Zone 与记录覆盖

| 功能 | 状态 |
|---|---|
| Primary Zone | 已纳入 |
| Secondary Zone | 已纳入 |
| Stub Zone | 已纳入 |
| Conditional Forwarder Zone | 已纳入 |
| Static Stub Zone | 已纳入 |
| Reverse Zone | 已纳入 |
| Catalog Zone | 已预留 |
| AXFR | 已纳入 |
| IXFR | 已纳入 |
| DNS NOTIFY | 已纳入 |
| TSIG | 已纳入 |
| XFR-over-TLS | 已预留 |
| XFR-over-QUIC | 已预留 |
| ZONEMD | 已预留 |
| Dynamic DNS Update | 已纳入 |
| Record Aging | 已纳入 |
| Wildcard | 已纳入 |
| Unknown RR | 已纳入 |
| SVCB/HTTPS | 已纳入 |
| TLSA | 已纳入 |
| SSHFP | 已纳入 |
| URI | 已纳入 |
| DNAME | 已纳入 |
| NAPTR | 已纳入 |
| APL | 已纳入 |

## 37.3 管理与安全覆盖

| 功能 | 状态 |
|---|---|
| Web Console | 已纳入 |
| REST API | 已纳入 |
| 多用户 | 已纳入 |
| RBAC | 已纳入 |
| API Token | 已纳入 |
| 单次 Token | 已纳入 |
| TOTP 2FA | 已纳入 |
| OIDC SSO | 已预留 |
| Session 管理 | 已纳入 |
| 查询日志 | 已纳入 |
| 系统日志 | 已纳入 |
| 审计日志 | 已纳入 |
| Prometheus Metrics | 已纳入 |
| Dashboard | 已纳入 |
| Docker 环境变量初始化 | 已纳入 |
| 插件系统 | 已预留并定义 |
| 集群节点管理 | 已预留 |
| 配置同步 | 已预留 |
| 备份恢复 | 已纳入 |

## 37.4 DHCP 与 IPAM 覆盖

| 功能 | 状态 |
|---|---|
| DHCPv4 | 已纳入 |
| 多 Scope | 已纳入 |
| 多网络 | 已纳入 |
| Lease | 已纳入 |
| Reservation | 已纳入 |
| DHCP Option | 已纳入 |
| FQDN Option | 已纳入 |
| Domain Search Option | 已纳入 |
| Classless Static Route | 已纳入 |
| CAPWAP Option | 已纳入 |
| TFTP Option | 已纳入 |
| DHCP DNS 联动 | 已纳入 |
| DHCP IPAM 联动 | 已纳入 |
| 地址空间 | 已纳入 |
| 子网管理 | 已纳入 |
| IP 地址状态 | 已纳入 |
| 冲突检测 | 已纳入 |

---

# 38. 验收标准

## 38.1 DNS 验收

- 能通过 UDP/TCP 正常解析。
- 能创建本地 Zone。
- 能添加、编辑、删除 DNS 记录。
- 能作为递归 DNS 使用。
- 能作为权威 DNS 使用。
- 能配置上游 DNS。
- 能启用缓存。
- 能查看查询日志。
- 能使用阻断列表。
- 能启用白名单。
- 能执行 DNSSEC 验证。
- 能执行 AXFR/IXFR。
- 能使用 TSIG。
- 能通过 DNS Client 测试查询。
- 能导入查询结果到本地 Zone。
- 能导出 Zone File。
- 能执行条件转发。
- 能阻断 CNAME Cloaking 命中的域名。
- 能启用 DNS Rebinding 防护。

## 38.2 DHCP 验收

- 客户端能获取地址。
- 能创建 Scope。
- 能查看租约。
- 能释放租约。
- 能设置保留地址。
- 能下发 DNS、网关、域名、NTP。
- 能下发 Domain Search、Classless Static Route。
- 能自动更新 DNS A/PTR 记录。
- 能检测地址冲突。
- 能导出租约。
- 能将租约同步到 IPAM。
- 能将保留地址同步到 IPAM。

## 38.3 IPAM 验收

- 能创建地址空间。
- 能创建子网。
- 能查看 IP 使用率。
- 能手动分配 IP。
- 能同步 DHCP 租约。
- 能关联 DNS 记录。
- 能发现冲突。
- 能导入导出数据。
- 能从子网生成 DHCP Scope。
- 能从子网生成 Reverse Zone。

## 38.4 权限验收

- 未登录不能访问管理页面。
- 普通用户不能执行管理员操作。
- API Token 权限生效。
- 单次 Token 使用后失效。
- TOTP 生效。
- 会话可查看和删除。
- 所有写操作进入审计日志。
- 权限变更后立即生效。
- 被禁用用户不能登录。

## 38.5 部署验收

- Linux 二进制文件可运行。
- systemd 可管理服务。
- Docker 可运行。
- Docker Compose 可运行。
- 配置文件可加载。
- 数据目录可持久化。
- Prometheus 可抓取指标。
- 环境变量可初始化管理员账号。
- 日志可写入指定目录。
- 服务重启后配置和数据不丢失。

---

# 39. 开发里程碑

说明：M1-M6 属于 V0.01 首版范围，M7 为后续版本落地计划。

## 39.1 M1：基础框架

交付：

- Go 项目结构
- 配置加载
- 日志框架
- 数据库迁移
- 用户登录
- RBAC 基础
- Web Console 框架
- REST API 框架
- Docker/systemd 初版

## 39.2 M2：DNS 核心

交付：

- UDP/TCP DNS
- 递归转发
- 上游管理
- 条件转发
- DNS 缓存
- Query Log
- DNS Client
- 常用记录解析

## 39.3 M3：权威 DNS

交付：

- Zone 管理
- Record 管理
- Primary Zone
- Secondary Zone
- Stub Zone
- AXFR/IXFR
- TSIG
- Reverse Zone
- Dynamic Update

## 39.4 M4：安全与过滤

交付：

- 阻断列表
- 白名单
- 客户端策略
- DNS Rebinding 防护
- CNAME Cloaking 防护
- Rate Limit
- DNSSEC 验证

## 39.5 M5：DHCP 与 IPAM

交付：

- DHCPv4
- Scope
- Lease
- Reservation
- DHCP Option
- DHCP DNS 联动
- 地址空间
- 子网
- IP 地址状态
- DHCP/IPAM 联动

## 39.6 M6：管理增强

交付：

- API Token
- 单次 Token
- TOTP 2FA
- Session 管理
- 审计日志
- Prometheus Metrics
- Dashboard
- 备份恢复
- 导入导出

## 39.7 M7：后续能力落地

交付：

- DoT
- DoH
- 插件接口
- 集群节点管理
- 配置同步预览
- OIDC SSO 预览
- Proxy 配置预览

---

# 40. 最终结论

GoDDI V0.01 是一个使用 Go 语言开发、采用 MIT License 发布的独立 DDI 平台。

最终产品定位为：

```text
GoDDI = DNS Server + DHCP Server + IPAM + Web Console + REST API + 可扩展 Plugin / Cluster 架构
```

V0.01 第一版应优先交付可生产试点的核心能力，同时在架构、数据库、API 和配置层为完整企业级能力预留扩展点。

GoDDI 的第一目标不是做复杂，而是做到：

```text
轻量、稳定、安全、清晰、可部署、可自动化、可持续演进
```

本最终稿已完成以下优化：

- 明确 MIT License。
- 明确独立产品和独立实现。
- 删除对外部产品专有实现的引用。
- 补齐 DNS 协议高级能力。
- 补齐 DNSSEC、Zone Transfer、Dynamic Update。
- 补齐 DNS Client。
- 补齐插件系统预留设计。
- 补齐 DHCP 高级 Option。
- 补齐 IPAM 联动。
- 补齐 OIDC 预留、2FA、Token、Session。
- 补齐 Docker 环境变量初始化。
- 补齐日志、审计、Metrics、Dashboard。
- 补齐集群预留、备份、告警和验收标准。
