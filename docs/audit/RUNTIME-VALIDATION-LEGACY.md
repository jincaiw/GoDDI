# 旧版 GoDDI 运行验证与缺陷记录（隔离/单机·双机）

> 本记录只覆盖在隔离环境下对**旧版 GoDDI 运行时**的实跑验证与缺陷修复，不涉及 Enterprise 候选包（见 `P0-G1.md`）。
> 全部运行均在 `/tmp/goddi-run` 隔离目录、独立 SQLite 库、回环/非生产网卡（`utun7`/`lo0`、`198.18.0.1`）上进行；**未触碰仓库生产数据**，运行结束进程均已停止。
> 单机：单进程 `role: all/control/dns`。双机：同主机两个进程 + 两套 DB/端口，或用 `dhcp_ha` 主备。

## 一、已修复缺陷（含回归测试）
1. **DHCP OFFER/ACK 缺少 RFC 2131 选项 54（Server Identifier）** — `internal/dhcp/server/handler.go`；`WithServerIP` 仅设 siaddr。修复：`applyServerIdentifier` 在 OFFER/ACK/INFORM ACK 补 option 54。测试：`internal/dhcp/server/server_identifier_test.go`。
2. **REQUEST 可将基础设施地址（服务器自身 IP / 网关）作为租约** — RENEWING 分支跳过池范围校验。修复：`isInfrastructureAddress` 拒绝服务器/网关地址。测试：`internal/dhcp/server/infra_address_test.go`。
3. **DHCP 租约过期的批量事实写入在单连接库上自死锁** — `internal/dhcp/lease/facts_expiry_batch.go`：事务持有唯一连接时再调 `ResolveSpaceID` 查同库。修复：事务**外**解析 space、事务内按行条件过期。测试：`internal/dhcp/lease/facts_expiry_deadlock_test.go`；并经端到端复验（5s 租约到期后回收、DHCP 保持响应）。

## 二、已验证可运行（实测）
- 构建 `go build ./...`、`goddi migrate`（29 迁移）、`goddi version`。**【round 190 现状更新】迁移数已随后续轮次增长**：`goddi migrate` 现适用 **32** 个控制面迁移（`migrations/`），另有 **17** 个数据面迁移（`internal/dataplane/migrations/`）——按 round 163 澄清的边界，后者由首次 `goddi serve` 的「装配」创建并迁移，`goddi migrate` 不创建数据面库（`cmd/goddi/migrate_dataplane_test.go` 已钉住）。
- 单节点：控制面 `/health`/`/ready` ok、优雅关闭；DNS 数据面绑定应答。
- **控制→DNS 数据面复制**：API 建区+记录 → DNS 权威应答；**DDNS**：绑定发布 A 记录、释放/过期撤销（含过期拆除）。
- 双机：两控制面进程并存；`dhcp_ha` 主备镜像（primary 服务/standby 不服务）；`ha degrade/fence/rejoin/takeover` 状态机与故障转移。
- 运维：`migrate --check/--status`、`backup`（运行中 VACUUM INTO）、`restore --verify/--into/--in-place(--yes)` 往返。
- DNS：多记录类型权威应答、NODATA/NXDOMAIN、RFC 2308（authority 带 SOA）、UDP 截断(TC)+TCP 回退、正向/负向缓存命中、转发/递归（含 RFC6761 特殊域名本地拦截）、区导入/导出、**区级查询 ACL**。
- DHCP：作用域复制、真实四次握手（选项 54 齐全）、续租、静态保留（MAC→IP）、DECLINE 冲突隔离、过期回收、DDNS 拆除、选项下发（mask/router/dns/domain/lease）、INFORM（仅回选项不下发地址）、**中继 giaddr 选域**。
- IPAM：空间/子网 CRUD、地址自动分配/释放、CIDR 规范化、地址状态机（合法/非法转移+原因必填+统计联动）、**DHCP↔IPAM 观测联动**（绑定→地址 `dhcp/in_use` + 租约ID）。
- API/安全：鉴权(401)、登录限速(5/分钟后 429)、**CSRF**(缺/错 token 401)、**RBAC**(无权限角色 403，admin 200)、配置 fail-closed（edition/secret/url）。

## 三、已定位问题（第 4、6、7 项已修复；第 5 项已做成可配置项，默认语义不变）
4. ~~**DNS 区传送（AXFR/IXFR）在服务角色下不可用**~~ → **已于 round 111 修复**。采纳「ACL 表纳入数据面存储并复制同步」方案（不改由控制库读取，以保留控制面宕机时数据面独立服务的能力）：
   - `internal/dataplane/migrations/015_zone_transfer_acl.sql`：数据面 zone 存储新增 `dns_zone_transfer` 只读投影表（**故意不建外键**，避免 `dns_zones` 整表替换时的级联删除）。
   - `internal/dataplane/sync.go`：`dnsTables` 增加 `dns_zone_transfer`（`replaceWhole`，列 `id/zone_id/allowed_cidr/tsig_key_name/created_at`）。
   - `migrations/030_dns_zone_transfer_bump_revision.sql`：控制库新增 3 个触发器，ACL 增/改/删均 bump `domain=dns` 的 `dataplane_revision`，否则 `Sync` 会认为修订未变而永远保留旧策略。
   - 新增 `internal/dataplane/zone_transfer_acl_test.go`：3 个测试覆盖 ACL 复制到数据面存储、`transfer.NewAXFRHandler(store.DB).CheckTransferACL` 的允许/拒绝/默认拒绝、ACL 变更与删除后的失效与重同步、投影列与控制库一致性。
   - **隔离单机实跑 A/B 对照**：控制库插入 `127.0.0.0/8` 授权 → 1s 同步后数据面存储出现该行 → 真实 `dig AXFR` 越过 ACL 检查到达预期的 TSIG 校验阶段（日志 `zone transfer requires TSIG authentication`）；删掉授权 → 同步后日志变为 `zone transfer denied by ACL`；恢复授权 → 又回到 TSIG 阶段。修复前第二步也会是「denied by ACL」（查表失败）。
   - 注：AXFR 本身按设计强制 TSIG（`HandleAXFR`），未改动该安全语义；本次只修 ACL 表缺失。
5. **RENEWING 池外地址认领策略** → **已于 round 118 做成可配置项，语义决定交给运维/产品，未擅自改变默认行为**。
   - 原状：`HandleRequest` 的 RENEWING/REBINDING 跳过池边界检查（设计上允许静态主机续租），因此任何客户端可用 RENEWING 认领子网内、池外、无既有租约的空闲地址并**新建**一条绑定。基础设施地址（服务器 IP / 网关）已在 round 102 被 `isInfrastructureAddress` 堵住。
   - 实现：`config.DHCPConfig.RenewOutOfPoolPolicy`（`yaml: renew_out_of_pool_policy`，`oneof=allow existing-binding-only`，默认 `allow`），env `GODDI_DHCP_RENEW_OUT_OF_POOL_POLICY`；`config.yaml` 示例已注释说明。`dhcp/server.Server` 新增 `SetRenewOutOfPoolPolicy`（未知值 fail-safe 回退 `allow` 并告警）+ `strictOutOfPoolRenewal()`；`HandleRequest` 仅在 `state==RENEWING && held==nil && 无预留 && 地址在池外` 时按严格策略 NAK，且**不写租约**。`main.go` 在 `Start` 之前设置（与 relay allowlist 同理）。
   - **不变的部分**：默认 `allow` 完全保留历史行为；池内续租、基础设施地址拒绝、HA/ durable ACK 门禁、所有非 RENEWING 状态均不受影响。有既有租约（`held`）或运维创建的预留（`reservID`）时严格模式也照常 ACK。
   - 测试：`renew_out_of_pool_test.go` 7 组全 PASS（默认静态主机 ACK、严格模式 NAK 且不写租约、既有绑定 ACK、预留 ACK、池内不受影响、未知值回退、原 `TestHandleRequest_RenewingOutsidePoolInsideSubnetIsAcked` 不回归）；`config_test.go` 新增默认值/严格值通过/越界值被 `Validate` 拒绝/空值允许。
   - **隔离单机实跑**：`GODDI_DHCP_RENEW_OUT_OF_POOL_POLICY=existing-binding-only` 启动 → `/health` 正常；`=bogus` 启动被拒并报 `oneof` 校验错误。
   - 因此「是否收紧」不再需要改代码，只是配置选择；**产品默认值仍维持现状**，未替用户做决定。
6. ~~**`config.yaml` 的 `cache:` 段被 `system_settings` 硬编码默认值遮蔽**~~ → **已于 round 112 修复（采用最小、不改变运行时优先级语义的方案）**：
   - 根因：`dnsCache` 由 `cfg.Cache` 构造，随后 `applyPersistedSettings()` 把 `system_settings` 每行重放到 cache 上；而 `EnsureDefaults()` 在全新库里播种的是 `defaultSettings` 的硬编码常量（`stale_ttl=86400`、`prefetch=false`、`min_ttl=60`、`max_ttl=86400`、`serve_stale=true`），这些值与 config 默认值本身就不一致，于是显式 `cache:` 配置被静默覆盖。
   - 修复：`system.Manager.EnsureDefaults(overrides ...map[string]string)` 支持按 key 覆盖播种值（仅写缺失行，已存在的行一律不动）；`cmd/goddi/main.go` 新增 `cacheSettingOverrides(cfg.Cache)`，把 5 个 `dns_cache_*` key 从 config 传入。
   - **语义**：仅决定「全新库首次播种时用 config 值」，不改变既有优先级 —— 运维在 UI 里改过的值仍持久化并在重启后优先于 config；`EnsureDefaults` 从不改写已有行。其余未覆盖的 setting key 仍按原硬编码默认值播种，改动面最小。
   - 测试：`internal/system/settings_test.go` 新增 4 个测试（config 覆盖生效、既有行不被覆盖、无覆盖时保持文档默认值、多 map 按序覆盖），全部 PASS。
   - **隔离实跑**：新建空库 + 特意与硬编码默认值全部相反的 5 个 cache 值（`min_ttl=123/max_ttl=9999/serve_stale=false/stale_ttl=777/prefetch=true`），启动后查询 `system_settings`，5 行全部等于 config 值（5/5 PASS）；服务端 `/health` 正常。
   - 注：`applySetting` 里对缺失行的硬编码 fallback 未改（正常路径下 `EnsureDefaults` 已播种，不会走到）。
7. ~~**RFC 2136 动态更新（未定论）**：实测无 TSIG 的 UPDATE 返回 NOTIMP(4)~~ → **已于 round 117 定位并修复，是真实缺陷（整条路径此前完全不可用）**。用 `nsupdate -v -d` 复现后依次发现三层问题：
   - **(a) 库层拦截**：`miekg/dns` 的 `DefaultMsgAcceptFunc` 在调用 handler **之前**就把 QUERY/NOTIFY 之外的 opcode 以 `RcodeNotImplemented` 拒绝，并且拒绝 `Nscount>1` 的报文 —— 所以 UPDATE 从未到达 `DNSHandler.ServeDNS`。修复：`internal/dns/server/server.go` 新增 `msgAcceptFunc`，对 `OpcodeUpdate` 放行并限制 `Qdcount=1`、`Ancount/Nscount≤2000`、`Arcount≤16`，其余 opcode 仍委托 `dns.DefaultMsgAcceptFunc`；UDP/TCP/DoT 三个 `dns.Server` 均设置 `MsgAcceptFunc`。新增 `update_accept_test.go` 3 组测试。
   - **(b) 策略表未复制**：`dynamic_update.checkUpdatePolicy` 从 `dnsDataDB`（数据面）读 `dns_dynamic_update_policies`，而该表只在控制库 `migrations/002` 存在 → 查询失败 → 所有签名 UPDATE 被拒。修复：`internal/dataplane/migrations/017_update_policies.sql` 投影表 + `sync.go` 的 `dnsTables` 加入 `replaceWhole` + `migrations/032_update_policies_bump_revision.sql` 三个触发器；新增 `update_policies_test.go` 2 组测试（复制/变更吊销/列一致性）。
   - **(c) TSIG 重新打包破坏 MAC**：`verifyTSIG` 用 `msg.Pack()` 重打包后再 `dns.TsigVerify`，但真实客户端（nsupdate）发送的是**压缩**报文，重打包后字节不同 → `dns: bad signature`。已用独立探针证实：压缩 wire 重新打包校验失败、未压缩则通过。修复：`DNSHandler` 把库层基于**原始 wire** 的 `w.TsigStatus()` 传给 `HandleUpdateFrom`（新增可选可变参数），取代重打包校验；`main.go` 把 `cfg.DNS.DynamicUpdate.TSIGKeys` 合并进监听器的 TSIG 密钥表，并给 UDP 监听器也设置 `TsigSecret`；`Server.hasTSIGSecrets()` 保证「有 TSIG RR 但监听器无密钥」时 fail-closed。新增 `wire_tsig_test.go` 3 组测试。
   - **隔离单机实跑（`nsupdate`）**：未签名 UPDATE → `REFUSED`（带 ZONE 回显，证明 handler 已执行，不再是库层 NOTIMP）；未知区 UPDATE → `NOTZONE`；**正确密钥的 hmac-sha256 签名 UPDATE → `NOERROR`，记录 `probe-signed.example.test. A 192.0.2.99` 写入数据面存储且可被 `dig` 解析**；错误密钥签名 → `REFUSED`。三种失败路径均未改动数据。

## 四、未完成 / 阻断（需真实环境或评审）
- 真实跨主机双节点、物理断电/未知提交、真实遗留生产库迁移实测——本沙箱不具备，且用户要求不得触碰生产。
- Enterprise 版 schema（DHCP 租约/发件箱/完整审计等）冻结与独立专家评审；G2–G5。
- 目标 Linux 平台、真实 DHCP 报文发送与时序门禁的最终网络传输验证。

## 五、分离角色实跑（round 124 补充）

此前矩阵均为 `role=all` 单机进程。本轮在隔离环境用两个进程验证 `role=control`（18082）+ `role=dns`（15355）共享控制库、各自数据面存储：`lsof` 确认端口隔离（control 不绑 DNS、dns 不绑 HTTP）；control API 创建区与记录后约 4s 由 dns 进程权威应答；**杀掉 control 后 dns 仍从 `dnsdata.db` 应答**，重启 control 后两者恢复。0 ERROR。详见 `docs/audit/IMPLEMENTATION-STATUS.md` §1b。这是对「双机/多进程架构预留」的运行期证据，不是真实跨主机 HA、fencing 或断电验收。

## 六、DHCP HA 隔离安全模式（round 125 补充）

两进程 `role=dhcp` + `dhcp_ha.enabled=true`（primary `utun7` / standby `lo0`，dev token，无生产网络）：清空早期 takeover 遗留的持久化角色后，primary 与 standby 正确建立镜像链路、standby 明确不服务；杀掉 standby 后 primary 日志进入 `state=paused`（无第二副本不再承诺绑定），重启后重连恢复；错误 token 无法建立会话；`goddi ha status` 正常，`degrade` 无 `--confirm` 与 primary 上 `takeover` 均按 `ErrNoConfirmation` / `ErrWrongRole` 拒绝。0 ERROR。详见 `docs/audit/IMPLEMENTATION-STATUS.md` §1c。**不构成**真实跨主机 / 分区 / 断电 / fencing 验收。

## 七、生产配置 fail-closed 运行期复核（round 162）

此前运行期验证均用 `env: development` 的隔离配置；本轮用**重新构建的二进制**直接验证 `internal/config` 的生产安全闸门在真实启动路径上生效（配置放 `/tmp/goddi-run/prod162/`，独立 DB 与端口 18086，不触碰主隔离库）：

| 场景 | 结果 |
|---|---|
| `env: production` + `encryption_key: ""` | 启动被拒，`invalid config: security.encryption_key must be set when env is production...`，`EXIT=1` |
| `env: production` + 独立 `encryption_key`（32 字符） | 启动成功，`/health` `{"status":"ok"}`、`/ready` `{"status":"ok"}`、`/login` 200，日志 0 ERROR |
| `encryption_key == jwt_secret` | 启动被拒 `must differ from security.jwt_secret`，`EXIT=1` |
| `encryption_key` 仅 5 字符 | 启动被拒 `must be at least 16 characters long (current: 5)`，`EXIT=1` |

对应单测 `internal/config/secrets_test.go` 已覆盖 4 条规则（`TestProductionRefusesToRunWithoutADedicatedEncryptionKey` / `TestProductionAcceptsADedicatedEncryptionKey` / `TestTheEncryptionKeyMustDifferFromTheJWTSecret` / `TestAShortEncryptionKeyIsRejected`）；本轮补充的是**二进制级端到端证据**，确认配置校验确实在 `serve` 启动路径上先于监听与数据库打开执行。仍不构成真实生产环境验收。

## 八、加密备份 / 恢复往返运行期复核（round 165）

用**随仓库 `config.yaml` + round 163 已 `serve` 过一次的三库实例**（独立 `/tmp/goddi-run/cfg163/`，专用 `GODDI_SECURITY_ENCRYPTION_KEY`）复跑灾备链路，补齐 B8.5-C 相邻的仓内证据：

| 步骤 | 结果 |
|---|---|
| `goddi backup -o backup.goddi` | `BACKUP_EXIT=0`，归档 57,653 字节；含 control(schema 32/62 表)、dnsdata(17/31)、leases(17/31)、`config/config.yaml`、密钥盘点（TOTP 0 条、TSIG 0 条）与两份密钥指纹 |
| `restore --verify` | `VERIFY_EXIT=0`：「归档完整，且与本机密钥材料一致。未写入任何文件」，三库 schema/表数与备份一致 |
| `restore --into unpacked/` | `INTO_EXIT=0`，解出 `config/config.yaml` 与 3 个 `db/*.db`；提示「还差三步」（指向解包目录、启动服务补 schema、确认密钥材料） |
| 解包库 `PRAGMA integrity_check` | control / dnsdata / leases 全部 `ok`（62 / 31 / 31 表） |
| **篡改归档 1 字节** | `restore --verify` `EXIT=1`：`chunk 0 does not authenticate ... message authentication failed`（AEAD 拒绝） |
| **错误 `encryption_key`** | `restore --verify` `EXIT=1`：同一认证失败，不泄露归档内容 |

全部为隔离单机证据，`--in-place` 未执行，未触碰主隔离库与生产；**不构成**真实断电/持久盘/恢复前后校验的 B8.5-C 外部验收。
