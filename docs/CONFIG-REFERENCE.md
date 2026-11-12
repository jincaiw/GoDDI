# 配置键参考（补齐 `config.yaml` 未列出的可选项）

> `config.yaml` 是**最小可运行样例**，只列出常用键；`internal/config` 中另有 36 个 `yaml` 键通过默认值生效或通过环境变量覆盖。本文把这些键按段落列全，并标注哪些是**保留占位（当前不生效 / 501 契约）**，避免运维误以为配置后即可用。
> 校验：`config.yaml` 中的所有键都有对应 `yaml` 标签（0 个未知键，`yaml.UnmarshalStrict` 不会因样例配置报错）；本文覆盖全部 36 个未列入样例的标签，由 `test/browser/docs.cjs` 静态钉住。

## 1. 生效中的可选项（可直接使用）

| 键路径 | 类型 / 默认 | 环境变量 | 说明 |
|---|---|---|---|
| `dns.allow_private_upstream` | bool / `true` | `GODDI_DNS_ALLOW_PRIVATE_UPSTREAM` | 允许上游转发器与诊断查询指向 loopback/私网/链路本地；面向公网部署应设 `false` 以关闭剩余 SSRF 面 |
| `dns.rate_limit.enabled` | bool / `false` | `GODDI_DNS_RATE_LIMIT_ENABLED` | DNS 查询/响应限速总开关 |
| `dns.rate_limit.client_qps` | int / `0` | `GODDI_DNS_RATE_LIMIT_CLIENT_QPS` | 单客户端持续查询上限，`0` = 不限 |
| `dns.rate_limit.client_burst` | int / `0` | — | 单客户端令牌桶容量，`0` 时取 `client_qps` |
| `dns.rate_limit.rrl_threshold` | int / `0` | `GODDI_DNS_RATE_LIMIT_RRL_THRESHOLD` | 全客户端相同响应每秒阈值，`0` = 关闭响应限速 |
| `dns.dynamic_update.enabled` | bool / `false` | — | RFC 2136 动态更新开关 |
| `dns.dynamic_update.tsig_keys` | map[string]string / `{}` | — | TSIG 密钥名 → base64 共享密钥；仅接受已登记密钥签名的 UPDATE，未配置则 fail-closed |
| `security.login_rate_window_seconds` | int / `60` | — | 登录锁定窗口（秒），`0` 时用默认 900s |
| `server.tls.client_ca_file` | string / 空 | — | 开启 mTLS：要求客户端证书由该 CA 签发；浏览器部署无客户端证书时启用会锁死所有运维，默认关闭。设置时要求 `server.tls.enabled=true` |
| `dns.listeners.*.address` / `enabled` / `cert_file` / `key_file` | — | — | DoT/DoH/DoQ 监听器配置，样例中已有部分；省略时使用默认值 |
| `dataplane.lease_dsn` / `zone_dsn` | string / 空 | — | 空时落在 `<server.data_dir>/leases.db` 与 `dnsdata.db` |
| `dhcp.lease_store` | `sqlite`/`wal`，默认 **`wal`** | `GODDI_DHCP_LEASE_STORE` | DHCP 租约热路径存储：`wal`（默认）用事件溯源 `WALLeaseStore`（WAL 持久 + 异步 SQLite 投影 + facts 由 WAL 派生）；设 `sqlite` 回退同步 `*lease.Manager`。HA 下应保持 primary=wal、standby 随 `standsAside` 不装配 |
| `dhcp.lease_wal_path` | string / 空（=`<server.data_dir>/leases.wal`） | `GODDI_DHCP_LEASE_WAL_PATH` | `lease_store=wal` 时的 WAL 文件路径 |
| `dhcp.require_ipam_projection` | bool / `false` | `GODDI_DHCP_REQUIRE_IPAM_PROJECTION` | D-09：为 true 时 DHCP 分配前要求 scope 子网在本地 `ipam_subnets` 投影中存在，否则拒绝（fail-closed）；默认 false 仅以缺口检测/就绪信号提示 |
| `dhcp.facts_hash_chain_enabled` | bool / `false` | `GODDI_DHCP_FACTS_HASH_CHAIN_ENABLED` | D-03：为 true 时 DHCP 事实 outbox 在生产侧打哈希链戳（epoch/prev/payload/tip）；默认 false（写入不带链字段，行为不变） |
| `server.expose_openapi` | bool / `false` | `GODDI_SERVER_EXPOSE_OPENAPI` | 是否公开 `/api/v1/openapi.json` |
| `metrics.enabled` / `metrics.path` | bool / string | — | Prometheus 端点开关与路径 |
| `env` | string / 空（=development） | — | 仅精确值 `production` 会收紧行为（CORS loopback 放宽、从 JWT 派生加密键等变为启动错误） |
| `edition` | string | — | 版本/发行版标识，由 `ValidateEdition` 校验 |

## 2. `dhcp_ha.*`（**生效中，但仅隔离验证**）

`dhcp_ha` 在 round 125 的隔离双进程安全模式验证中使用；**真实跨主机 HA / 分区 / 断电验收仍为 B8.5 外部项**。

| 键 | 类型 / 默认 | 环境变量 | 说明 |
|---|---|---|---|
| `dhcp_ha.enabled` | bool / `false` | `GODDI_DHCP_HA_ENABLED` | HA 总开关 |
| `dhcp_ha.node_id` | string / 空 | `GODDI_DHCP_HA_NODE_ID` | 节点标识，成对节点必须不同 |
| `dhcp_ha.role` | `primary` / `standby`，默认 `primary` | `GODDI_DHCP_HA_ROLE` | standby 纯镜像、不服务客户端；不自动 promote |
| `dhcp_ha.listen_addr` | string / `0.0.0.0:647` | `GODDI_DHCP_HA_LISTEN_ADDR` | primary 监听地址；standby 不绑定 |
| `dhcp_ha.peer_address` | string / 空 | `GODDI_DHCP_HA_PEER_ADDRESS` | standby 侧填 primary 地址；必须含主机名 |
| `dhcp_ha.peer_token` | string / 空（≥16 字符） | `GODDI_DHCP_HA_PEER_TOKEN` | 共享密钥，建议只从环境注入 |
| `dhcp_ha.confirm_timeout` | duration / `2s` | `GODDI_DHCP_HA_CONFIRM_TIMEOUT` | 绑定等待第二副本确认的上限 |
| `dhcp_ha.heartbeat_interval` | duration / `1s` | `GODDI_DHCP_HA_HEARTBEAT_INTERVAL` | 空闲时的心跳间隔 |
| `dhcp_ha.peer_stale_after` | duration / `5s` | `GODDI_DHCP_HA_PEER_STALE_AFTER` | 静默多久后节点退出可承诺状态；必须大于 `confirm_timeout` 与 `heartbeat_interval` |
| `dhcp_ha.tls_cert_file` | string / 空 | `GODDI_DHCP_HA_TLS_CERT_FILE` | 设置后复制通道启用 mTLS：本节点证书（CN=node_id）。三项 TLS 文件必须同时提供 |
| `dhcp_ha.tls_key_file` | string / 空 | `GODDI_DHCP_HA_TLS_KEY_FILE` | 证书私钥；仅从受限文件/环境注入 |
| `dhcp_ha.tls_ca_file` | string / 空 | `GODDI_DHCP_HA_TLS_CA_FILE` | 校验对端证书的根 CA |
| `dhcp_ha.tls_server_name` | string / 空 | `GODDI_DHCP_HA_TLS_SERVER_NAME` | dial 侧校验的对端名称；空则取 peer_address 主机名 |

## 3. 保留占位（**配置可被解析，但功能未实现 / 501**）

以下段落对应的管理 API 仍返回 `501`（`/sso`、`/cluster`、`/apps`），`internal/cluster.ErrNotImplemented`、`internal/apps/*/ErrNotImplemented` 保持。**写入这些键不会启用相应能力**；按 v0.9.0 计划保留至真实 transport、持久化状态与外部验收完成。

| 段落 | 键 | 默认 | 说明 |
|---|---|---|---|
| `sso` | `enabled`, `provider`, `authority`, `client_id`, `client_secret`, `discovery_url`, `scopes`, `auto_register`, `group_mapping` | `enabled=false`, `provider=oidc`, `scopes=openid,profile,email`, `auto_register=false` | OIDC/SSO 保留结构 |
| `cluster` | `enabled`, `node_name`, `node_role`, `bind_addr`, `peers`, `sync_interval` | `enabled=false`, `node_role=primary`, `bind_addr=:7946`, `sync_interval=30` | 多节点协调保留结构；`internal/cluster.ProtocolModel` 仅进程内模型 |
| `plugin` | `enabled`, `registry`, `auto_update` | 全 `false` / 空 | App 市场保留结构 |
| `dns.recursion.*` 之外的 `scopes` / `authority` 等 | 见上 | — | 同上，均属保留段 |

## 4. 使用约定

- **严格解析**：`LoadFromYAML` 使用 `yaml.UnmarshalStrict`，`config.yaml` 中出现任何没有对应 `yaml` 标签的键都会加载失败（已有回归 `TestLoadFromYAML_RejectsUnknownFields`）。新增配置键时务必同步 `internal/config` 结构体、样例或本文。
- **环境变量优先**：`GODDI_*` 覆盖 YAML；布尔/整数解析失败会在启动时报错，不静默回退。
- **密钥不入文件**：`security.jwt_secret`、`security.encryption_key`、`dhcp_ha.peer_token`、`sso.client_secret`、`dns.dynamic_update.tsig_keys` 建议只经环境变量或密钥管理注入；`production` 环境会强制要求独立 `encryption_key`。
- **不授予权限**：以上任何配置项都不授予角色、epoch、fencing 或受保护写权限；`internal/enterprise` 的 10 个 `ErrNoAuthority` 哨兵恒关闭。

## 5. 静态校验

`test/browser/docs.cjs` 会：
1. 收集 `internal/config/*.go` 的全部 `yaml` 标签；
2. 确认 `config.yaml` 中的每个键都有对应标签（防止样例配置漂移）；
3. 确认每个**未列入 `config.yaml`** 的标签都在本文中被提及（防止新增可选键无人文档化）。

新增配置键而未更新本文时，该守卫会 FAIL。

## 6. 环境变量覆盖全表（49 项）

> `internal/config/config.go` 的 `applyEnvOverrides` 通过 `setEnvString/setEnvBool/setEnvInt` 读取以下变量，**环境变量优先于 YAML**。布尔/整数解析失败会在启动时报错，不静默回退。本表由 `test/browser/docs.cjs` 静态钉住：新增 `GODDI_*` 覆盖而未更新本表即 FAIL。

| 环境变量 | 类型 | 对应配置路径 |
|---|---|---|
| `GODDI_CACHE_ENABLED` | 布尔 | `cache.enabled` |
| `GODDI_CACHE_MAX_ENTRIES` | 整数 | `cache.max_entries` |
| `GODDI_DATABASE_DRIVER` | 字符串 | `database.driver` |
| `GODDI_DATABASE_DSN` | 字符串 | `database.dsn` |
| `GODDI_DHCP_ENABLED` | 布尔 | `dhcp.enabled` |
| `GODDI_DHCP_HA_CONFIRM_TIMEOUT` | 字符串 | `dhcp_ha.confirm_timeout` |
| `GODDI_DHCP_HA_ENABLED` | 布尔 | `dhcp_ha.enabled` |
| `GODDI_DHCP_HA_HEARTBEAT_INTERVAL` | 字符串 | `dhcp_ha.heartbeat_interval` |
| `GODDI_DHCP_HA_LISTEN_ADDR` | 字符串 | `dhcp_ha.listen_addr` |
| `GODDI_DHCP_HA_NODE_ID` | 字符串 | `dhcp_ha.node_id` |
| `GODDI_DHCP_HA_PEER_ADDRESS` | 字符串 | `dhcp_ha.peer_address` |
| `GODDI_DHCP_HA_PEER_STALE_AFTER` | 字符串 | `dhcp_ha.peer_stale_after` |
| `GODDI_DHCP_HA_PEER_TOKEN` | 字符串 | `dhcp_ha.peer_token` |
| `GODDI_DHCP_HA_ROLE` | 字符串 | `dhcp_ha.role` |
| `GODDI_DHCP_RENEW_OUT_OF_POOL_POLICY` | 字符串 | `dhcp.renew_out_of_pool_policy` |
| `GODDI_DNS_ALLOW_PRIVATE_UPSTREAM` | 布尔 | `dns.allow_private_upstream` |
| `GODDI_DNS_DEFAULT_TTL` | 整数 | `dns.default_ttl` |
| `GODDI_DNS_DNSSEC_ENABLED` | 布尔 | `dns.dnssec.enabled` |
| `GODDI_DNS_DOMAIN` | 字符串 | `dns.domain` |
| `GODDI_DNS_ENABLED` | 布尔 | `dns.enabled` |
| `GODDI_DNS_LISTENERS_TCP_ADDR` | 字符串 | `dns.listeners.tcp.address` |
| `GODDI_DNS_LISTENERS_UDP_ADDR` | 字符串 | `dns.listeners.udp.address` |
| `GODDI_DNS_RATE_LIMIT_CLIENT_QPS` | 整数 | `dns.rate_limit.client_qps` |
| `GODDI_DNS_RATE_LIMIT_ENABLED` | 布尔 | `dns.rate_limit.enabled` |
| `GODDI_DNS_RATE_LIMIT_RRL_THRESHOLD` | 整数 | `dns.rate_limit.rrl_threshold` |
| `GODDI_ENV` | 字符串 | `env` |
| `GODDI_LOG_LEVEL` | 字符串 | `log.level` |
| `GODDI_LOG_QUERY_LOG_ENABLED` | 布尔 | `log.query_log_enabled` |
| `GODDI_LOG_RETENTION_DAYS` | 整数 | `log.retention_days` |
| `GODDI_METRICS_ENABLED` | 布尔 | `metrics.enabled` |
| `GODDI_METRICS_PATH` | 字符串 | `metrics.path` |
| `GODDI_PROXY_ADDRESS` | 字符串 | `proxy.address` |
| `GODDI_PROXY_ENABLED` | 布尔 | `proxy.enabled` |
| `GODDI_PROXY_TYPE` | 字符串 | `proxy.type` |
| `GODDI_SECURITY_ENCRYPTION_KEY` | 字符串 | `security.encryption_key` |
| `GODDI_SECURITY_JWT_SECRET` | 字符串 | `security.jwt_secret` |
| `GODDI_SECURITY_LOGIN_RATE_LIMIT` | 整数 | `security.login_rate_limit` |
| `GODDI_SECURITY_REBINDING_PROTECTION` | 布尔 | `security.rebinding_protection` |
| `GODDI_SECURITY_TOTP_ENABLED` | 布尔 | `security.totp_enabled` |
| `GODDI_SERVER_DARK_MODE` | 布尔 | `server.dark_mode` |
| `GODDI_SERVER_DATA_DIR` | 字符串 | `server.data_dir` |
| `GODDI_SERVER_EXPOSE_OPENAPI` | 布尔 | `server.expose_openapi` |
| `GODDI_SERVER_HTTP_ADDR` | 字符串 | `server.http_addr` |
| `GODDI_SERVER_LANGUAGE` | 字符串 | `server.language` |
| `GODDI_SERVER_NAME` | 字符串 | `server.name` |
| `GODDI_SERVER_PUBLIC_URL` | 字符串 | `server.public_url` |
| `GODDI_SERVER_TLS_CERT_FILE` | 字符串 | `server.tls.cert_file` |
| `GODDI_SERVER_TLS_ENABLED` | 布尔 | `server.tls.enabled` |
| `GODDI_SERVER_TLS_KEY_FILE` | 字符串 | `server.tls.key_file` |

### 密钥类（建议只从环境注入，不写入 YAML）

- `GODDI_SECURITY_JWT_SECRET`、`GODDI_SECURITY_ENCRYPTION_KEY`、`GODDI_DHCP_HA_PEER_TOKEN`。
- `production` 环境强制要求独立 `encryption_key`；`jwt_secret` 为默认值时启动会被拒绝（除非显式设置 `GODDI_ALLOW_DEFAULT_SECRET=true` 用于开发）。

### 保留占位相关

- `GODDI_DNS_DNSSEC_ENABLED`：DNSSEC 仍为实验功能，默认关闭，开启后正向/否定应答已签名但**无父区 DS 链式信任**。
- `GODDI_DHCP_HA_*`：内部已有隔离双进程验证；真实跨主机 HA / fencing / 断电仍属 B8.5 外部验收。

> 环境变量不授予角色、epoch、fencing 或受保护写权限；`internal/enterprise` 的 10 个 `ErrNoAuthority` 哨兵恒关闭。

