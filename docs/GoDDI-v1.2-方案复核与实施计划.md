# GoDDI 企业级 DDI 方案复核与实施计划

版本：v1.2
日期：2026-09-24
用途：在 v1.1 评审基础上，对照当前代码收敛实施次序，并记录已启动的工作。

## 结论

v1.1 的总体架构判断仍然成立：继续演进 GoDDI；先保证 DHCP 租约、DNS 写入、IPAM 地址所有权和灾备恢复正确；以持久化、单写边界和故障行为作为 GA 门槛。它对双节点分区、备份完整性、ACK 前持久化和“拆池不等于 HA”的限定尤其重要，建议保留。

v1.1 不宜原样作为当前版本的实施清单。它的代码证据锁定在 `d5b5fe3 / v0.5.2`。本次复核起点为最新已发布基线 `v0.22.0 / 29e0e6c`；当前实现分支 `codex/ddi-review-implementation-20260923` 已推送多轮 W01/W04 改动，包含 W04 非 HA 事实生产、控制端消费与跨库端到端回归。该分支没有发布新版本，代码存在不等于生产验收完成。

## 合理之处

1. 用数据安全硬门槛代替平均评分，正确区分“已有基础”和“可生产验收”。
2. 保留 REQUEST 持久化成功后再 ACK 的语义，不以异步落库换取表面吞吐。
3. 将规划分配、DHCP 租约和 DNS 记录分开管理，并要求租约事件可持久重放、按版本去重和对账。
4. 不把同进程快照、双 DNS 地址或 80/20 拆池夸大为完整故障隔离或 DHCP 高可用。
5. 将安全、迁移、备份恢复、可观测性和故障注入纳入产品验收，而不是只做正常路径演示。

## 需要修订的地方

### 1. 重新建立代码证据基线

v1.1 的行号、包覆盖和“尚未接通”判断只适用于 `d5b5fe3`。最新版已出现租约内存索引与重建、提交后索引更新失败时降级、可控重试失败 outbox、outbox 顺序恢复、IPAM 必须命中目标地址等能力。它们与 v1.1 的 W03、W04、W06、W07、W11 有交集，不能再按旧工作包重复设计。

执行要求：每个工作包开始前先做一次基线差异表，逐条记录旧缺口的当前状态、代码证据、未覆盖的故障情形和剩余工作。将已有能力标为“代码存在、待验证”，只有通过对应恢复/故障验收后才标“已达成”。

### 2. 把开发分支实现纳入审查，但不提前算作交付

当前实现分支已有已提交并推送的事实事件投递、消费与启动边界改动，但仍未合入发布分支。审查事件顺序、缺口检测、失败事件阻塞、重复投递、消费水位事务、进程重启及数据面 readiness；不能只因增加了表、接口或测试就关闭 W04/W06/W11。

### 3. 将产品决策与可独立开发的修复拆开

原方案把首发网络画像、分区暂停、单副本降级和 SLO 留作待决项。现已按用户授权形成默认工程决策：本轮 IPv4 DHCP 首发；HA 沿用 ADR-0003 的严格双副本、无围栏不自动接管、单副本需显式批准；无真实场景数据时不承诺统一容量/RTO/RPO 数值。后续可依据明确客户画像修订这些决定，但当前安全修复无需等待。

### 4. 把 W14 的验证工作前移

容量画像、协议反例、SQLite 持久化语义和故障注入是架构设计的输入，不应等 W10—W13 结束才开始。早期先建立可复现的小型正确性门槛；双节点真实网络、长期稳定性和容量认证放在相应基础能力完成后执行。这样可以尽早发现设计错误，同时不把未测场景包装成 GA 证据。

### 5. 重新定义版本退出条件

v1.1 的 v0.6—v1.0 阶段次序可作骨架，但应以能力与证据退出，不按发布日期或工作包数量退出。实现、恢复验证和运维验收分开记录。所有“RPO=0”“60 秒接管”“2 秒内应用”等数字必须绑定故障模型、硬件、负载和测量方法，否则仅作为待验证目标。

## 复核状态矩阵

| v1.1 主题 | 最新代码观察 | 当前评审状态 | 接下来要确认 |
|---|---|---|---|
| IXFR 单连接下嵌套查询与历史差异 | primary 写入者现同事务写旧/新 SOA 分界及 RR delta；transfer 按 rowid 验证连续历史链，覆盖 serial 回绕，缺口/坏数据回退 AXFR | IXFR 增量路径已接通，协议包测试与单连接测试通过；未做真实 secondary 互操作 | 验证 IXFR 多消息 TCP framing、删除/新增顺序、journal pruning 边界，并在真实 secondary 上演练；任何不完整链必须继续回退 AXFR |
| DHCP REQUEST/OFFER/DECLINE/relay | 最新已发布代码与当前工作计划记录 REQUEST 分类、server-id、OFFER/DECLINE 隔离、Option 82 allowlist、有界队列和 HA 状态机已有仓内实现与负向用例 | 已有实现，外部互操作待验收 | 真实 relay、不同 client-id/MAC 客户端、多网卡和真实 ACK 仍需网络环境证据；不重复开发已覆盖状态机 |
| 配置 revision 与发布 | 最新工作计划记录 expected revision、幂等变更、发布 outbox、rollback-as-new-revision、首条 diff 和显式 pruning 已完成 | 单节点实现已具备 | 多节点发布协调仍属实验/未实现；发布中断和逐节点 applied 水位需要真实执行器与验收 |
| DHCP→DNS durable outbox | 最新工作计划记录 generation、consumer、retry/reconciliation 和进程级追平演练已经存在 | 范围内已实现，统一事实源仍未完成 | DHCP-DNS-IPAM 统一 sequence、producer 真实接入、跨库 replay 和完整 lag/gap 监测继续单独跟踪 |
| DHCP 内存选址和租约索引 | 当前代码已有快照重建、租约索引和索引更新失败后的 readiness 降级路径 | 代码已存在，异常恢复与真实运行态待验收 | 启动重建失败、写入后索引失败、冲突并发和池变化下不误发 OFFER/ACK；必须与持久租约源对照 |
| DHCP ACK 持久性 | DHCP 独立 lease data-plane store 单连接运行；`internal/dataplane/store.go` 启动设置并读回 WAL、`synchronous=FULL`；REQUEST 路径先提交 lease，再做 HA 确认，最后构建 ACK | 代码边界存在，物理耐久性未验收 | 在目标驱动、文件系统与硬件上分别验收进程崩溃、主机断电、复制链路分区；`FULL` 是应用请求的同步边界，不替代介质掉电演练 |
| DHCP→IPAM 持久事件 | 非 HA 默认路径已将 REQUEST/续租/释放/管理释放/拒绝/过期写入本地事实 outbox，经幂等上行复制到 control inbox，由默认控制端 consumer 原子投影 IPAM；分离数据库端到端回归通过 | 非 HA 实现已接通，未发布；HA 仍走兼容观察路径 | HA 接管的事件/序列连续性、首事件丢失与重放、端到端对账及真实运行态恢复验收；未完成前不发布 W04 版本 |
| DNS DDNS 与 IPAM 统一所有权 | v1.1 识别的生命周期、generation、DNS serial 和所有权风险依然需要核对 | 需按最新代码重新验收 | 旧 RELEASE/过期事件不能删除新代记录；A/PTR、zone 发布和 serial 的失败恢复保持一致 |
| DNS secondary | 最新基线已有周期扫描、健康状态及 EXPIRE 相关实现，优于 v1.1 描述的初始缺口 | 代码存在，待验证 | 完整检查启动接线、运行态发布、恢复失败、EXPIRE 临界点和重新同步行为 |
| IPAM 分配与跨 scope 唯一性 | 最新工作树的事实消费接口已强调目标缺失不得当作成功；不能据此推定规划分配 CAS 和跨池唯一性全部完成 | 部分解决 | 比较最新迁移、地址状态迁移、space 作用域唯一索引、导入和并发分配路径 |
| 进程隔离与控制快照 | 代码已有数据面 lease snapshot 重建和 readiness gate；事实投递控制端消费者也已接入默认非 HA 服务 | 实现已接线，故障隔离与恢复未验收 | kill 控制进程、坏快照、旧 schema、磁盘满和重启时数据面实际行为；不要把同主机多进程当作主机级 HA |
| 配置/租约 HA | 最新已有协议模型、租约 fence/takeover/rejoin、watermark 和 `/ready` 仓内边界；生产配置协调、真实 fencing 与跨主机网络测试未闭合 | 实验/不完整 | 保持 cluster API 501；需要 quorum/witness/fence 决策和真实跨主机测试，单机模型不能关闭 HA 门槛 |
| 灾备与密钥恢复 | 未见足够证据证明 v1.1 所列全状态备份、离线恢复和密钥恢复已闭环 | 未关闭 | 新路径恢复演练、校验、权限/TSIG/密钥覆盖、旧快照不得覆盖新状态 |

## W01—W14 当前任务状态（2026-09-24）

| 工作包 | 仓内状态 | 尚未关闭的退出条件 |
|---|---|---|
| W01 IXFR | primary serial 作者写入者均记录完整 SOA 分界；连续 journal 链服务已实现，缺链回退 AXFR | 真实 secondary、多消息 TCP framing、pruning 临界点与恢复互通 |
| W02 架构边界 | 单写、双副本确认、无见证时暂停、显式降级/接管操作已有实现与 ADR | 生产围栏设备、跨主机分区、主备恢复演练；不得据此启用自动接管 |
| W03 DHCP 协议 | REQUEST 分类、server-id、relay allowlist、有界队列及负向用例已有代码 | 真实 relay、多种客户端/多网卡的现场交互和 ACK 行为 |
| W04 DHCP→IPAM facts | 非 HA 默认 producer/consumer、跨库幂等传送/水位和 readiness 已接通 | HA 复制事件与 sequence、接管后首事件连续性、控制库故障重放和实网演练 |
| W05 配置发布 | revision、CAS、幂等与 release outbox 已有实现 | 多节点 applied 确认、部分节点失败时保留 LKG 的部署级协调验收 |
| W06 DNS/IPAM 不变量 | CNAME owner 规范化、catalog 自动 PTR 与 zone rename 冲突已修复；CSV 与 BIND 导入均支持 dry-run | BIND 目前返回首个数据冲突而非全部冲突；多 DNS 关系源及并发地址分配仍需完整验收 |
| W07 进程隔离/恢复 | 数据面快照重建和 readiness gate 已接线 | 坏快照、旧 schema、磁盘满、控制进程退出与重启故障注入 |
| W08 DNS HA | secondary 周期刷新、EXPIRE 与健康状态已有代码 | 双 DNS 地址/策略同步、客户端切换和真实节点断连恢复 |
| W09 DHCP HA | 单写复制水位、显式 takeover/fence/rejoin 机制存在；当前不自动 promote | HA facts continuity、生产 fencing 和网络分区验收；未完成前不能宣称 HA GA |
| W10 安全/备份 | 加密备份、restore 版本/schema 门禁和全状态归档已有实现 | 密钥异机保存/找回、真实权限/TSIG 恢复、硬件断电持久性 |
| W11 可观测性 | facts consumer gap/readiness、producer backlog/失败/水位、HA replication 水位和备份年龄指标已接线 | 正式 Prometheus 抓取、告警规则/阈值、恢复动作与告警演练 |
| W12 升级/灾备 | 在线 WAL 原地 restore 与空白异路径整机恢复 smoke 均通过 | 逐节点升级、expand-contract 兼容性、现场密钥及介质恢复 |
| W13 产品体验 | 尚无本轮完成的 UI/API 工作包实现 | 地址详情、子网到池、dry-run 导入、API/Web 权限契约 |
| W14 容量认证 | 当前无固定硬件/混合负载容量结论 | 固定硬件、负载、故障矩阵和长稳实测；未测数字不对外承诺 |

最新工作树 `go test ./...` 与 `go vet ./...` 均通过；W12-a/W12-c 隔离式 restore 演练通过。上述表中仍标为未关闭的现场/部署退出条件没有被仓内测试代替。当前分支只作为草稿 PR 评审，未满足正式发布门槛。

## 调整后的实施计划

### 第 0 阶段：最新版基线复核与安全收敛

- 固定对照基线：最新已发布版本 `v0.22.0 / 29e0e6c`；实现分支 `codex/ddi-review-implementation-20260923` 的后续提交单独列示，不混称为已发布版本。
- 建立 W01—W14 的当前状态台账，更新文件、迁移、接口和验收证据。
- 复核现有事实 outbox 开发：持久顺序、水位原子性、失败阻塞、重试权限、启动 readiness 和重复投递。
- 修复发现的“部分数据仍返回成功”类协议问题。AXFR fail-closed 并在单一 SQLite 读快照中读取记录和 SOA；primary 侧 Record API、导入、catalog、DHCP DDNS、动态更新及配置/元数据发布现均同事务写 SOA 分界与 journal。IXFR 只在找到从客户端 serial 到当前 serial 的完整连续链时返回差异，历史缺失或不合法时返回 AXFR。
- 本阶段代码变更需附带针对性回归用例；当前实现分支 `go test ./...` 已全量通过，但网络、掉电、长稳与容量场景仍未验收。

退出条件：基线台账逐项指向最新版代码；安全正确性缺陷有修复或明确阻断记录；对正在开发的事实投递代码完成审查。当前分支 Go 全量测试已通过；物理断电、跨主机网络和真实客户端验收仍未完成。

### 第 1 阶段：冻结安全边界并补齐核心协议

- W02 以已接受的 ADR-0001/0003 为约束复核最新版装配；保持 primary 单写、未围栏不自动接管、双副本 durable ACK、显式单副本降级。
- W01 完成 primary 写入分界后接通完整 journal 链传送和 RFC 1982 serial 回绕处理；继续验证 TCP 分帧、历史清理边界及真实 secondary 互操作。缺历史时始终回退 AXFR。
- W03 不重复实现已有 REQUEST 分类、server identifier、OFFER/DECLINE、Option 82 allowlist 和有界队列；补齐最新版差异对账与真实 relay/客户端外部验收。
- W04 复核租约 generation 与持久事实事件，闭合 A/PTR 生命周期。DHCP ACK 不等待控制库投影，但投影失败可见且可重放。
- W10 提前处理安全默认、关键审计和“全状态备份能否在新路径恢复”的最小演练。

退出条件：协议反例无错误 ACK/NAK；重复分配、误删记录及半成功 DNS 更新被回归用例拦截；恢复流程可重复执行。

### 第 2 阶段：配置发布与地址所有权统一

- W05 复核最新版已有的 revision、expected version、幂等和发布 outbox；仅补仍缺失的多节点 applied 确认和失败保留 LKG。
- W06 逐项核对 space 唯一性、CAS、导入预检与多 DNS 关联，按代码/回归证据补缺，不重建已有能力。
- W07 审查三角色和启动恢复实现；验证坏快照、旧 schema、磁盘故障、控制进程退出及 DHCP 本地租约库恢复。
- W11 对账现有 metrics/ready，再补 facts producer outstanding、HA 复制确认/peer applied 水位和真实告警阈值；consumer gap/失败、备份年龄已有对应系列，仍需核对部署抓取与告警规则。

退出条件：并发分配唯一；配置失败保留最后有效版本；控制面退出不改变健康数据面状态；租约库异常时不发送无持久保护的新成功 ACK。

### 第 3 阶段：按决策启用高可用

- W08 验收双 DNS 地址、权威区/策略同步、secondary EXPIRE 和客户端切换。
- W09 在已批准的围栏、见证和降级策略下实现 DHCP 单写、复制确认水位、接管和旧主隔离。
- W12 执行逐节点升级、expand-contract 迁移和离线恢复演练。

退出条件：主宕机、单向/双向分区、备份落后、旧主回归及重复接管均不产生双写或地址重复授权；RTO/RPO 仅报告已实测场景。

### 第 4 阶段：体验、容量与 GA 门禁

- W13 在稳定服务层之上统一 IP 详情、子网到池流程、导入 dry-run 和 API/Web 权限契约。
- W14 按固定硬件、租约/DNS 混合负载、故障矩阵和长稳时长完成容量认证。
- GA 仅在所有 P0 证据闭环后评审；仍有未决网络画像或围栏依赖时，发布能力范围和禁用条件。

## 已定工程默认决策

| 决策 | 本轮默认 | 实施约束 |
|---|---|---|
| 首发网络画像 | IPv4 DHCP 首发；IPv6 DHCP/IPv6-only 不纳入本轮 GA 保证 | 每项 DNS/IPAM IPv6 能力单独据实声明 |
| 双节点分区 | 沿用 ADR-0003：无围栏时不自动接管；严格双副本确认后 ACK | 网络分区期间暂停新承诺，不宣称持续发租约 |
| 单副本降级 | 默认严格双副本；单副本模式必须显式运维批准并持续告警 | 复制追平后才恢复完整保护状态 |
| IXFR | 完整连续 journal 链可增量传送；历史缺口、无效 RDATA、serial 歧义回退 AXFR | 仍需验证 TCP 分帧、历史清理边界和真实 secondary 互操作 |
| 管理与密钥恢复 | 完整列出证书、TSIG、JWT、TOTP 等备份和恢复责任 | 数据库文件备份不等于灾备完成 |
| 容量与 SLO | 固定真实硬件和站点负载后测量 | 不采用通用 QPS、利用率或切换秒数作为发布承诺 |

## 已启动工作

- 工作树：`/Volumes/My-Data/jason.wa/WorkBuddy/Worktrees/GoDDI/ddi-review-implementation-20260923`
- 基线：`d3ddb16 / v0.10.0`，独立分支 `codex/ddi-review-implementation-20260923`。
- 修改：`internal/dns/transfer/transfer.go` 初期暂停使用尚不完整的 IXFR journal，非当前 serial 请求回退 AXFR；该阶段状态已由下方最新 W01 记录更新。AXFR 遇到扫描失败、无效/不支持 RR 或缺少 SOA 时 fail-closed。
- AXFR 的记录和 SOA 现在在同一数据库读快照中取得；IXFR 的“客户端已最新”判断也从同一 SOA 快照读取 serial，避免版本检查本身读到互不一致的数据。
- 修改：`internal/dns/zone/record.go` 将 Create/Update/Delete、BatchCreate/BatchDelete 和过期清理的记录变更、SOA serial 与 journal 写入纳入同一 SQL 事务；serial 或 history 写入失败会回滚记录变更。
- 修改：`internal/dns/zone/import_export.go` 将 CSV/zone-file 导入记录、serial 和 journal 纳入同一事务。
- 修改：`internal/dns/zone/catalog.go` 和 `zone.go` 将 catalog 成员 PTR、zone 的 catalog 属性、新建/删除成员及相关 serial/history 合入各自单一数据库事务。删除 catalog 时也在删除事务内清空成员关联。
- 修改：`internal/dns/zone/record.go` 在启用 CreatePTR 的 A/AAAA 创建路径中，先解析反向区，再将正向记录、反向 PTR、涉及区域的 serial 和 journal 纳入同一数据库事务；PTR 插入或 serial/history 失败时两区一起回滚。未配置匹配反向区时保留原行为，仅创建正向记录。
- 修改：`internal/dhcp/dns_link.go` 将 DHCP A/PTR 投影按可见 RRset 差异推进本地 serial/history；generation 或 expiry 等不可见元数据的幂等更新不产生 serial 变更。
- DHCP DNS 投影在 RRset、serial 和 history 提交成功后，对受影响的 primary 区域异步发送 NOTIFY；无可见 RRset 变化时不发通知。
- 修改：`internal/dns/dynamic_update/update.go` 将已提交且实际改变 RRset 的 RFC 2136 更新同步刷新内存 Store，并在提交后通知 primary 区域的 secondaries；prerequisite-only 和 RRset no-op 不推进通知。动态更新回归尚未执行。
- RFC 2136 动态更新对 TXT 的新增和按 RDATA 删除会检查存储后能否重建原 character-string 边界；当前 schema 不能无损表达时拒绝整条更新，单个空 TXT 字符串仍可精确表示。
- 自动 PTR 创建现与正向 A/AAAA 创建共用同一 SQL 事务；提交后分别通知发生变更的 primary 区域，避免只写入一侧或出现孤立 PTR。
- 修改：`internal/dataplane/sync.go`、`runner.go` 与 `cmd/goddi/main.go` 在数据面配置同步事务内比较同步前后的 primary SOA/可见 RRset，计算实际变化区域；事务完成后刷新内存 Store，并只对变化的 primary 区域发送 NOTIFY。删除区域已无法从当前配置查询并发送通知，secondary 将按既有刷新/EXPIRE 机制处理，此边界仍需运行态验收。
- 修改：`internal/dns/zone/zone.go` 将管理 API 修改 SOA 相关参数和 zone 类型转换与 serial 推进纳入同一事务。
- 修改：`internal/configver/adapters_records.go` 在配置发布改变应答 RRset 时，同事务推进 zone serial 并记录实际删除/新增记录；`adapters_dns.go`、`ZoneManager.UpdateZone`、`IncrementSerial` 和 zone type conversion 对 serial/SOA 变化同事务记录旧/新 SOA RDATA。初期 journal 覆盖不足的问题已由后续 W01 工作收敛；仍需真实 secondary 互操作验收。
- 修改：`internal/dns/transfer/secondary.go` 校验 AXFR 首尾 SOA、SOA 数据一致性、IN 类和 owner 区域范围；缺少闭合 SOA、存储模型不支持/不能无损表示的 RR（包括无法保留字符串边界的 TXT）或任一插入失败均不提交新快照。
- AXFR 校验进一步要求 SOA 位于响应第一条和最后一条，类别为 IN，并比较首尾 TTL 与其余 SOA 数据；仅“响应中恰好出现两个 SOA”不再视为完整帧。
- 修改：`internal/dns/zone/store.go` 和 `internal/dns/transfer/transfer.go` 保留显式 TTL=0，不再改写为 3600；RecordManager 限制 TTL 在 DNS 有效范围内，非法数据库记录会阻止不完整的内存快照替换，AXFR 则失败关闭。
- 新增数据面迁移 `internal/dataplane/migrations/009_zone_change_history.sql`，在权威区数据所在 SQLite 中建立 IXFR journal 与索引，确保拆分部署下记录/serial/history 可以同事务提交；既有控制库迁移保持向后兼容。
- 修改：`internal/dns/zone/store.go` 在同一 SQLite 只读事务中读取 zone 元数据和 records；任何扫描、遍历或数据校验错误都会放弃整份新快照并保留旧快照。管理接口同时校验默认 TTL、SOA 定时器和 minimum 的无符号范围。
- Store 会为当前快照中的最近一条未来 expiry 安排重载，到期后不必等管理清理周期就从权威应答中移除；AXFR 也过滤已到期记录。后台持久删除、serial/history 和 NOTIFY 清理周期缩短为一分钟，因此 secondary 的到期记录清理仍有最多约一分钟的后台传播延迟，需在运行态验收。
- `zone.Store.Close()` 会停止 debounce/expiry 定时器并等待正在执行的数据库快照读取；主服务在关闭数据面数据库前关闭 Store，避免退出后定时回调继续触碰连接。
- 内存 Store 合成 SOA 的 TTL 已与 AXFR/zone metadata 统一使用 `default_ttl`，避免同一区域的查询与传送视图不一致。
- 修改：`internal/dns/zone/zone.go` 改名区域时，在同一事务中将区内 owner 从旧 apex 迁到新 apex，并写入 delete/add history；`catalog.go` 同事务更新成员 PTR RDATA 及 catalog serial/history。
- 以上 secondary 和改名修复仍需协议回归与运行态验收。控制/数据面变更的跨库原子传播需按工作包继续验收。DNSSEC 当前明确未实现签名与 DNSKEY/RRSIG/NSEC 应答，因此现有开关和 NSEC3 参数不改变权威 RRset；真正接入签名时必须将签名材料发布和 serial 更新作为同一变更审查。
- 2026-09-23 验证：`go test ./...`、`go build ./...`、`go vet ./...` 及 DHCP/DNS/dataplane/configver 关键包 `go test -race` 通过；前端 typecheck 和生产构建通过。单独重跑了拆分数据面 DHCP→DNS 发布与 ACK 到可解析延迟回归。前端 lint 被仓库已有的 14 条规则错误阻断（未改动对应业务源码）。首次全量 Go 测试揭示旧测试 schema 缺列/缺表及同步回调签名过期，已修复夹具；另发现 zone plane 缺少 journal 迁移，已补迁移并复跑通过。
- 当日早期 W01 仍为局部收敛的状态记录，已被本文件后续 `W01 IXFR journal chain` 进展更新；外部 secondary 互操作仍属于未验收项目。
- 仓内验证通过不代表完成真实现场验收：断电持久性、relay/客户端互操作、跨主机分区/围栏、目标硬件容量与长期稳定性仍是后续门槛。

### v0.12.0 后续修复（2026-09-23）

- 复核发现 CAA 的 `tag` 在记录写入和读取路径中遗漏，zonefile/CSV 导入也会丢弃 CAA `flag/tag`；这会令管理 API 接受的记录与权威 DNS 应答不一致。
- 修复单条/批量记录创建、读取、列表、局部更新、覆写 journal、zonefile 和 CSV 往返中的 CAA 元数据保留；更新 CAA value 时继承未显式修改的 tag/flag，并拒绝超出 8 位 wire flag 的值。
- 控制库迁移 `026_dns_zone_change_caa_fields.sql` 和 zone 数据面迁移 `010_zone_change_caa_fields.sql` 为历史记录加上 `flag/tag`；迁移可回滚并可重放。区变更历史 API 同步返回字段。
- 回归用例覆盖创建、局部更新、authoritative CAA answer、journal/API history、CSV 与 zonefile 导入，以及 flag 边界。
- `go test ./...`、`go vet ./...`、`go build ./...` 及 zone/transfer/dynamic update/dataplane 关键包 race 检查通过。此修复补足了 journal 元数据的一类缺口，但不等于所有 RR 类型都已具备完整 delta history；IXFR 继续回退 AXFR。

### v0.13.0 后续修复（2026-09-23）

- 导入路径此前会忽略 CSV 语法错误、列不足、无效数字及不支持的 RR 类型；zonefile 导入会跳过无法表示的 RR，并且 NAPTR 只保留 replacement 字段。这些情况会让调用方得到成功响应但区域数据不完整。
- CSV 导入改为对解析错误、字段数量、数值字段和不支持的类型明确报错；所有记录先完成校验，再进入单一事务，不会部分写入。
- zonefile 导入遇到未知 RR 类型、无法编码的 RDATA 或当前 schema 无法无损表示的 NAPTR 时明确失败；此前已能保留元数据的 CAA 路径保持正常。
- 回归覆盖 CSV 混合有效/无效行整体拒绝、CSV 语法错误和 NAPTR zonefile 拒绝，确认失败后记录数为零。全量 Go 测试、关键 DNS/dataplane 竞态检查、`go vet` 与构建通过。
- W06 的 dry-run、导入预检及更多 RR 类型的无损表示仍未完成；本修复只消除静默部分成功，不代表导入能力验收完成。

### v0.14.0 后续修复（2026-09-23）

- 将 NAPTR 的 `flags/service/regexp/replacement` 完整保存在 value presentation 中，order/preference 使用已有 priority/weight 字段；创建、读取、权威应答、zonefile 和 CSV 均沿用这套表示。
- CSV 导出 owner 改为相对区域名，导入按目标区域解析，支持跨区域迁移并避免 owner 留在源区域。CSV 和 zonefile 导入都拒绝越界 owner，校验完成后才开启事务。
- 添加 NAPTR API/权威响应/CSV/zonefile 全字段往返，以及越界 owner 拒绝和失败无部分写入回归。
- W06 的 dry-run 和完整预检流程仍未实现；其他未具备无损表示的记录类型继续逐项核实，IXFR 继续回退 AXFR。

### v0.15.0 后续修复（2026-09-23）

- 在区域导入 endpoint 增加 CSV `dry_run`，只执行正式导入相同的解析和字段校验，不启动写事务。
- 成功预览返回 `record_count` 与 `record_types`；输入错误会拒绝请求，实际导入路径保持同一 parser/validator。
- 回归验证 API 预览不写入记录、报告计数准确，并通过全量测试、关键包竞态检查、vet 和构建。
- 更完整的导入预检仍待做：现有预览不报告与区域现有 RRset 的冲突或相同 RR 重复策略；IXFR 保持回退 AXFR。

### v0.16.0 后续修复（2026-09-23）

- CSV 预览与正式导入在同一数据库快照内分类新建与未变化记录，额外返回 `creates` 和 `unchanged`。
- 完全相同的输入 RR 与区域已有 RR 按未变化处理；同文件重复 RR 只写一次；重复 RDATA 的 TTL 不同则拒绝，避免静默忽略差异。
- 导入前检测同 owner 的 CNAME 与其他类型冲突，包括同一 CSV 内的冲突；错误在写入前返回，不留下半成品。
- 回归覆盖重复 CSV 幂等、二次预览计数、TTL/类型边界及 CNAME 冲突整体拒绝。剩余预检工作是逐类核对其它 RRset 不变量及更友好的冲突明细；IXFR 继续回退 AXFR。

### v0.17.0 后续修复（2026-09-23）

- CSV 预览与导入检查同一 owner/type RRset 的 TTL 一致性；既检查目标区已有记录，也检查导入文件中的多条记录。
- TTL 冲突在写入事务提交前失败，预览与正式导入采用相同规则；已有 RR 保持不变，不会写入该文件的部分记录。
- 新增文件内冲突和已有 RRset 冲突的拒绝及无部分写入回归。该约束目前在 CSV 预检路径闭合，其他记录创建/更新和非 CSV 导入路径仍需逐入口统一核对；IXFR 继续回退 AXFR。

### v0.18.0 后续修复（2026-09-23）

- 将 RRset TTL 一致性从 CSV 预检扩展到单条创建、批量创建、记录更新和 zonefile 导入；这些路径与 CSV 相同，忽略禁用记录，只约束当前启用并参与权威应答的 RRset。
- 校验在各写路径的同一事务中执行；批量和 zonefile 发生冲突时整体回滚。更新只在改变 owner/type/TTL 或启用状态时检查，普通备注等变更不被旧 RRset 问题阻断。
- 新增 API 创建/更新/批量操作及 zonefile 文件内 TTL 冲突回归。跨 catalog/DHCP 投影等其它写入入口仍需确认是否绕过此规则；W06 其余 RRset 约束与冲突诊断继续待办，IXFR 继续回退 AXFR。

### v0.19.0 后续修复（2026-09-23）

- RRset TTL 检查不再将已过期记录视作正在应答的成员；CSV 预检同样只用未过期且启用的记录比较重复项、CNAME 和 TTL。配置版本发布保留的数据面 authored RR 与新配置 RR 一并校验，避免跨作者拼出不同 TTL 的集合。
- secondary 在替换最后已知可用区之前检查整个 AXFR 的 owner/type TTL 一致；来自上游的异常快照被拒绝，不会替换当前服务副本。
- 自动 PTR 与 DHCP forward A/reverse PTR 投影在各自 SQLite 写事务中拒绝会产生 TTL 不一致的更新；自动 PTR 与主记录共享事务，因此冲突整体回滚。DHCP forward/reverse 仍分开提交，RRset 分别受保护但整对更新尚非原子。
- 修复自动 PTR 把相对 owner 直接写入数据库的问题，现在存储区域内规范的 FQDN；这让创建的 PTR 能按反向区域 owner 正确应答。
- 回归覆盖过期记录解除 TTL 阻塞、自动 PTR 冲突整体回滚/成功后 FQDN 应答 owner、配置发布混合 authored TTL 冲突、AXFR 不一致 TTL 拒绝，以及 DHCP A 冲突失败关闭。catalog 投影与动态更新中多条 RRset 的 TTL 归一策略仍需评审；DHCP A/PTR 跨区原子性继续列为 W04 未完成项。

### v0.20.0 后续修复（2026-09-23）

- RFC 2136 动态 UPDATE 添加 RR 时，采用该次更新指定的 TTL，并在同一事务内调整该 owner/type 的全部旧成员；随后添加或替换的记录不会留下混合 TTL 的 RRset。
- TTL 重写为每个旧成员写 delete/add journal 行，新 RR 写 add 行；相同 RDATA、相同 TTL 仍保持幂等，不推进 serial。
- 修复上行复制首次拒绝的重试截止时间精度，避免 SQLite 秒级时间戳截断后让 1 秒退避过早到期；清理 catalog 实现中重复且未调用的旧包装方法以恢复 lint。
- 回归覆盖向已有 RRset 添加不同 TTL 的记录，以及以不同 TTL 重新添加相同 RDATA；其它元数据完整表达与 IXFR 重新启用仍未验收。

### v0.21.0 后续修复（2026-09-23）

- DHCP 正向 A 与反向 PTR 的创建/接管改用同一 DNS 事务；任一 RRset 校验、记录写入或 journal 失败都会整体回滚，journal 对同一受影响区域只推进一次 serial。
- 新增反向 PTR TTL 冲突回归，确保失败时既不留下 A，也不推进任一区域 serial。W04 的跨库 durable event/重放依赖与真实网络客户端验收仍未完成。

### v0.22.0 后续修复（2026-09-24）

- DNS CSV dry-run 在同一 SQLite 快照内汇总所有目标区域及文件内部的 RRset TTL、CNAME 独占性和多 CNAME 冲突，返回 CSV 行号、owner、记录类型、稳定冲突码与说明；有效行仍统计新增和未变化数量，冲突行不计作未变化。
- CSV 正式导入复用相同冲突检查；发现任一冲突时整批拒绝，保持记录、serial 与 journal 不变。预览仍不写入，并以 `valid=false` 和冲突列表表示可修复的数据问题；语法/字段校验错误继续作为请求错误返回。
- 该改动收敛 W06 导入预检诊断的一部分；其他记录入口的完整不变量核对、多 DNS 关联以及 IXFR 解禁条件仍未完成。
- BIND zonefile 导入现检查 TTL/RR 值并在同一事务中执行 CNAME 独占性验证，覆盖既有记录及本批此前已插入记录；发现冲突时整批回滚。回归确认同 owner 的 CNAME+A 不会留下部分写入。`go test ./internal/dns/zone ./internal/dns/transfer` 通过。其他记录入口的完整不变量、多 DNS 关联和 IXFR 解禁条件仍未完成。
- Record API 单条/批量创建、更新、自动 PTR、配置发布和 DHCP DDNS 共用 DNS 大小写无关且忽略过期/禁用 RR 的 CNAME 独占性事务校验；拒绝同一 owner 下不同 CNAME 目标以及 CNAME 与其他类型共存。批量路径过去会吞掉 CNAME 检查的 SQL 错误，现已移除。更新路径排除当前记录。回归覆盖大小写不同的冲突创建/改名、批次原子回滚、zonefile 整批回滚、DHCP A 与现有 CNAME 冲突、自动 PTR 冲突、配置发布与 data-plane CNAME 冲突。相关包测试通过。
- IPAM 的 DNS 辅助关联曾对跨 space 重叠 IP 使用 `LIMIT 2` 后任取第一条，可能关联错地址对象。现无 scope 的入口在存在多个 space 候选时 fail-closed，并提供显式 `space_id` 的关联方法；回归确认歧义不写入关联、显式 space 关联命中目标对象。
- SOA 配置/serial 变更（管理 API、配置发布、显式 serial increment 与 zone type conversion）现将旧/新 synthesized SOA RDATA 写入同 serial 的 journal，并与元数据和 serial 更新同事务提交；回归核对旧、新序列及 timer/name 字段。当前 primary 侧已发现的 serial/RR journal 写入者均已加 SOA 分界，IXFR 仍需完成协议和历史序列端到端验收后再考虑启用。
- RFC 2136 动态 UPDATE 现在在变更事务内读取旧 synthesized SOA，按旧 SOA 删除、RR 删除/新增、更新后 SOA 添加的顺序写 journal；新 SOA serial、RR、serial 更新与 history 同事务提交。回归验证了四行顺序及更新后 SOA serial，动态 UPDATE、zone 与 transfer 包测试通过。该入口已补齐 SOA 分界，但其它 RR 写入者的分界、journal 读取排序和完整 IXFR 协议测试仍未闭合。
- DHCP DDNS 正向与反向区域写入也在其共享 DNS 事务中记录旧 SOA、RR 多重集差异及新 SOA；每个有变化的区域各推进一次 serial。回归确认两区各包含 SOA 删除/添加和对应 A/PTR 差异，DHCP、dataplane、dynamic_update、zone 与 transfer 包测试通过。其它 RR 写入入口和协议级 IXFR 仍未完成。
- Record API 单条创建/更新/删除、批量创建/删除、自动 PTR 和过期记录清理均通过事务 serial helper 写入旧 SOA 分界，并在该 serial 的 RR journal 完成后写入新 SOA；serial、RR 数据和 history 同事务提交。zone 元数据更新和类型转换也使用事务开始前捕获的旧状态，并只写一次每个 SOA 分界。
- CSV/zonefile 导入、catalog 成员新增/删除/改名和 DNS 配置版本记录发布已接上旧 SOA 删除与新 SOA 添加；配置发布仍按服务 RR 多重集写出真实删除/新增。primary 写入清单的代码审计已覆盖 Record API、动态 UPDATE、DHCP DDNS、配置发布、区域元数据/serial 操作、catalog 和两种导入路径；secondary 载入及 dataplane 同步的 serial 写入属于复制侧。全量 Go 测试通过。IXFR 仍保持回退 AXFR，待增加完整 RFC 1995/1982 传输用例、验证历史范围/排序与缺口行为后再重新评估启用。

### W04 跨库事实传输实施中（2026-09-24，尚未发布）

- 控制库新增 observation inbox 与 projection watermark；lease data-plane store 增加事实事件 delivery marker。事件内容不可变地传输，控制库负责消费状态，producer 不会覆盖消费者状态。
- 数据面 Runner 在已有 lease 上行循环中异步投递事实。控制库提交成功后 producer 才确认本地事件和 marker；崩溃窗口可重放。event ID 重复按完整 envelope 校验，sequence 冲突留在有界退避队列中并计入拒绝数。
- 为避免 ACK 在控制库故障时查询 IPAM，DHCP 配置副本增加最小化的 subnet→space 投影；控制库 IPAM 子网新增/修改/删除会推进 DHCP 配置 revision，lease store 可按地址匹配最具体的本地 CIDR。
- IPAM consumer 明确使用控制库 inbox，使 inbox 完成、水位推进和 IPAM 投影在同一控制库事务内完成；传输与投影之间采用至少一次投递及幂等消费。
- 单节点 DHCP 默认初始化 sequence allocator 与 observation outbox，并把 REQUEST 绑定/激活/续租、释放、拒绝、管理界面释放及到期清理接到 FactsMutationWriter。租约、事实 envelope 和兼容 DNS outbox 同事务提交；scope 已关闭 DNS 更新时仍允许删除旧记录。facts 投递由 Runner 唤醒，但不阻塞 DHCP ACK。
- 地址无法映射到本地 IPAM space 时，DHCP 操作沿用现有 lease 路径，到期仍会清理租约；不会伪造 facts。映射数据库出现其他错误时，单次 facts mutation 回滚并返回错误。
- 非 HA 控制进程默认启动 IPAM facts consumer；投影、水位推进与 inbox 完成同事务，消费失败保留待处理事件并反映在 `/ready` 与 Prometheus。已映射地址不再同步双写 IPAM；无本地映射时继续使用原观察路径，避免丢掉既有的可见性。
- 定向回归覆盖首次传输、远端已消费后的崩溃重放、远端消费状态不被覆盖、sequence 冲突保留重试、IPAM 映射同步和最具体网段选择、事实绑定原子提交、REQUEST/RELEASE 路由、unmapped expiry，以及分离的 DHCP/控制库到 IPAM 投影端到端路径。当前 `go test ./...` 全量通过。
- HA 部署继续使用原 IPAM 观察路径，facts 生产者/消费者暂不接管：现有 HA 复制只镜像 lease rows，不复制 facts 序列和 outbox；启用后会在接管时造成序列断档。W04 仍需补齐 HA 故障转移时的事实复制语义、控制库与 DHCP 数据库分离时的故障恢复及真实网络端到端演练。本批接通非 HA 默认生产者和消费者，但不发布版本。

### W01 配置发布写路径补齐（2026-09-24，阶段记录）

- 配置版本发布替换 DNS 控制面记录后，对 before/after 的应答 RR 集合做多重集差分；删除旧 RR、添加新 RR 和推进 serial 后，将差异 journal 写入同一个数据库事务。
- journal 保留 wire 记录字段（owner、type、RDATA、TTL 及 priority/weight/port/CAA 字段），未变化记录不重复写入；同一内容的重复发布不推进 serial。
- 回归确认替换一个 A 记录并增加另一个时，当前 serial 下记录一条删除和两条新增。`go test ./internal/configver` 通过。
- 本节记录当时阶段状态；SOA 分界与完整 journal 链服务已由下方最新进展更新。

### W01 IXFR journal 链传送（2026-09-24，代码完成，现场验收待做）

- primary 侧已审计的作者写入者均在同一事务中写入旧 SOA 删除、RR 删除/新增及新 SOA 添加：Record API、批量/过期清理、自动 PTR、CSV/zonefile 导入、catalog 成员变更、DHCP DDNS、RFC 2136 动态 UPDATE、配置版本记录发布、区域元数据变更和显式 serial 操作。secondary 快照应用及 dataplane serial 同步属于复制侧，不作为新的 primary history 来源。
- `HandleIXFR` 先在单一 SQLite 读事务中读取当前 SOA 和 history；只在每个 delta 均有合法 SOA 分界、历史从请求 serial 连续到当前 serial、最终 SOA 与当前权威 SOA 一致时返回 IXFR。响应按 RFC 1995 组织为当前 SOA、旧 SOA/删除集、新 SOA/新增集，并以当前 SOA结束；同 serial 请求及 RFC 1982 判定为更新的客户端返回当前 SOA。
- 历史缺口、格式错误、不支持或无法解码的 RR、超大 history 扫描及 RFC 1982 半区间歧义均 fail-safe 回退完整 AXFR。旧的无 SOA delimiter history 无法被误当成可用 delta。
- 回归覆盖多 serial 差异顺序、缺口回退 AXFR、相同/更新 serial 请求、32 位 serial 回绕和单连接无嵌套查询死锁。`go test ./...`、`go vet ./...` 及 transfer/zone/DHCP/configver 关键包 race 检查通过。
- W01 仓内代码门槛已闭合；还需在真实 secondary 上验证 IXFR TCP 多消息 framing、历史 pruning 边界和互操作恢复。未完成现场验证前，任何缺链仍会返回 AXFR；本分支尚不构成整体 GA 发布。

### W12-c 灾备恢复演练（2026-09-24，仓内演练通过）

- 执行 `python3 scripts/w12c_restore_drill.py`：演练先创建包含 control、dnsdata、leases 三个数据库的实例并生成加密归档，随后删除源数据目录，在不同绝对路径、从未初始化的目标目录恢复。
- 三个恢复文件均与归档成员逐字节一致，SQLite integrity check 通过，恢复后无需迁移；目标配置未被归档配置覆盖，schema 版本与归档清单一致。
- 恢复实例实际启动，liveness/readiness 均返回 HTTP 200，数据面状态为 current，进程正常退出且无 panic。
- 这关闭了“空白主机、不同路径、源目录销毁”这一仓内恢复路径验收。演练明确没有验证密钥材料异机保管/找回、物理断电介质持久性、DHCP UDP/67 listener、生产拓扑及真实备份介质；这些仍是发布运行验收项，不据此宣称灾备 GA。

### W12-a 在线快照与原地恢复 smoke（2026-09-24，仓内演练通过）

- 执行 `python3 scripts/w12_snapshot_smoke_check.py`：覆盖 role=all 三库加密备份、`restore --verify` 无写入、活跃 SQLite 连接/WAL 存在时原地恢复、WAL/SHM/残留副本清理，以及安全副本通过 WAL 包含被替换前的状态。
- 同一演练验证 control-only/缺失 store 的归档清单、`migrate --status/--check` 不意外创建数据面库，以及拒绝新 schema/新版本归档、允许旧 schema 归档后再迁移。
- 所有断言通过。该演练验证文件/SQLite 路径和兼容性门禁，不证明底层介质断电持久性、加密密钥异机找回或真实生产恢复时间。

### W11 DHCP facts producer 可观测性（2026-09-24，代码完成）

- DHCP 非 HA 事实 outbox 增加低基数 Prometheus 样本：pending/failed 数、已分配最高 sequence、首个未完成 sequence。查询带 2 秒超时；读取失败只记告警，不伪造零水位。
- 指标能区分“控制端 consumer 已追平”与“DHCP 生产 outbox 尚未送达”，也能直接暴露失败事件阻塞点；事件 ID、错误文本不作为标签。
- `internal/metrics` 与 `cmd/goddi` 定向回归及 golangci-lint 通过。HA facts 生产/复制尚未接通；备份年龄已有系列，但抓取配置、告警阈值和现场告警动作仍需验收。
- 后续补充 primary HA replication gauges：本机已分配 sequence、mirror durable ACK sequence、最近观察到的 peer applied sequence，均只由 primary 采样并使用 node_id 标签。回归检查了 exposition；operator 可区分“发送序号”“备端确认”和“备端实际应用”水位。该观测不改变复制协议，也不代表 facts 已随 HA 镜像。

### W06 DNS owner 名规范化与 catalog 冲突修复（2026-09-24，代码完成）

- 盘点 catalog PTR 写入时发现 owner 以不带末尾点的相对存储形式写入；普通 DNS API 输入会正规化为带末尾点的 FQDN。原 CNAME 排他查询直接比较字符串，未将两者识别为同一 DNS owner，catalog 成员加入可与已有 CNAME 共存。
- CNAME 冲突验证改为大小写不敏感并忽略末尾点；catalog 加入前显式复用该验证，冲突时整个创建事务回滚。
- 新回归以合法 CNAME 与自动 catalog membership PTR 覆盖该冲突，并确认失败后没有残留 PTR。zone、DHCP、transfer、dynamic_update 定向回归及 zone lint 通过。
- 继续核对区域改名批量移动 owner 的写入，发现 owner 移入已有 RR 名称时同样可能产生 CNAME 共存。现改名后在原事务内检查最终 owner 集合；冲突时区域名、记录和 serial/journal 一并回滚。回归覆盖 CNAME 移入已有 A owner；zone、transfer、dynamic_update 测试和 zone lint 通过。
- 为闭合 BIND zonefile 的 dry-run，新增 `PreviewZoneFile` 与 API `dry_run=true` 路径：使用与真实导入相同的解析、RRset/CNAME 校验、插入、serial 和 journal 操作，成功后显式回滚；失败路径也由事务回滚。回归确认合法 A/MX 给出分类统计但无 records/journal 写入，已有 CNAME 冲突会拒绝且不改变记录、serial 或 history。
- BIND 导入改为使用共用的 CNAME owner 校验器；RRset TTL 与 CNAME 校验统一忽略 owner 的末尾点，防止 API FQDN 与导入相对名绕过冲突检查。zone 与 API handler 定向测试、lint 通过；仍可进一步补充 BIND 多冲突聚合诊断。

## 范围与限制

本计划按用户授权直接选择工程默认值，已记录在 ADR-0008；DHCP HA 默认沿用已接受的 ADR-0003。实施分支从 `v0.22.0 / 29e0e6c` 创建，后续提交已推送，但尚未合入发布分支或创建正式版本。目标客户的容量和真实拓扑仍需在发布验证中固定，未有证据前不作数值承诺。

原方案渲染为 17 页，但当前文档渲染环境未显示中文字符，导致页面与表格无法据此完成版式验收；本次内容评估以 DOCX 段落和表格完整文本抽取为依据，没有修改或重新导出原 DOCX。

## 规范依据

- [RFC 1995 IXFR](https://www.rfc-editor.org/rfc/rfc1995)：增量序列表示从客户端旧版本到服务器当前版本的完整差异；客户端应在全部差异处理成功后才替换旧区。服务端无法提供增量时可以返回完整区域。
- [RFC 1982 DNS serial arithmetic](https://www.rfc-editor.org/rfc/rfc1982)：SOA serial 是 32 位循环序列号，不能用普通无符号整数大小关系代表所有版本先后。
- [RFC 2131 DHCP](https://www.rfc-editor.org/rfc/rfc2131)：协议交互及 DHCPREQUEST 分类依据；持久租约承诺还需结合实际 SQLite、驱动和存储介质配置验证。
- [SQLite synchronous pragma](https://www.sqlite.org/pragma.html#pragma_synchronous) 与 [WAL 文档](https://www.sqlite.org/wal.html)：提交确认与断电持久性的边界取决于 journal mode、synchronous 配置和同步行为，不能只凭应用进程重启测试作结论。
