# 02 · H2 双节点一致性专项审计

> 基线 commit `8e769d7`。以 `internal/dhcp/ha/*`、`internal/facts/*`、`internal/ipam/*` 真实代码为依据。已执行 `go test ./internal/dhcp/ha/ ./internal/facts/ ./internal/dhcp/lease/` 全绿。

## 3.1 集群与所有权
| 检查项 | 状态 | 证据与结论 |
|---|---|---|
| Node ID 持久化/唯一 | PARTIAL | `internal/dhcp/ha/config.go` `NodeID`；来自配置/`GODDI_DHCP_HA_NODE_ID`（`internal/config/config.go:413`）。两组必须不同（配置校验），但**无集群级唯一性仲裁**（cluster 501）。 |
| Cluster ID | NOT_IMPLEMENTED | 无 cluster id 概念；`internal/cluster` 仅协议模型（`cluster.go:14`）。 |
| Owner Epoch（单调围栏令牌） | PARTIAL→**基础已实现（E-1，2026-11-12）** | `dataplane_meta.ha_owner_epoch` 持久化；`Takeover` 单调 +1；`Watermarks.Epoch` 握手交换；见更高 epoch → `StateFenced`/`MayBind=false`（不 ACK）。剩余：quorum 证明自动继续（E-3）与写路径显式 epoch 参数。 |
| 双节点身份认证/mTLS/证书轮换/防重放 | **FAIL** | 复制通道为**明文 TCP + 共享 `peer_token`**（`internal/dhcp/ha/config.go` `PeerToken`，`protocol.go` Hello 携带 token）；ha 包无 `tls.`；无证书轮换；防重放仅靠水位(sequence)而非加密认证。管理面另有 mTLS（`tlsutil`）但**未用于数据面**。 |
| 每 Scope Group 唯一 Owner | PARTIAL | 单写者模型：`EffectiveRole` 以库中 `ha_role` 为准（`role.go:115`），`standsAside` 非 primary 不构造 DHCP（`cmd/goddi/main.go` ~927）。但"Scope Group"粒度所有权未实现（所有权是全节点级，非每 scope group）。 |
| 并发双写/旧主回归/脑裂 | PARTIAL | 无 epoch 写栅栏；旧主在分区时进入 `paused`（`state.go` `MayBind=false`）从而不 ACK，网络恢复后两 primary 无对端→均 paused（fail-closed，需人工 fence/rejoin）。**可用性依赖人工，但写入脑裂被 paused 抑制**。NOT_VERIFIED 运行时双写测试。 |
| 心跳/复制/控制面链路区分 | PASS | 心跳=复制通道应用回执(`Applied`)，非独立心跳；控制库不在复制通道（ADR-0003 决策1，`replicator.go`/`mirror.go` 直连 TCP）。 |
| 是否依赖 SQLite 文件复制/共享 WAL | PASS（未这样做） | 复制通过协议帧重放租约行（`LeaseRow`，`protocol.go`），非文件复制。 |
| 未经验证的配置 Writer 变更 | NOT_IMPLEMENTED | 多节点配置发布协调未实现（ADR-0005）。 |

**DEC-002（禁止自动接管）**：PASS（设计层）。`state.go` 明确"Nothing in this package promotes a node ... on its own"；`internal/dhcp/ha/ha_test.go`/`operator_test.go` 覆盖不自动提升；`Replicator.State()` 对无对端返回 `paused`（`replicator.go:182-196`）。

## 3.2 DHCP 强一致性
| 检查项 | 状态 | 证据 |
|---|---|---|
| ACK 在权威持久化之后 | PASS | `internal/dhcp/server/handler.go:497-580,617`：提交→Confirm→RequireDurable→构建 ACK。 |
| Lease/Facts/Outbox/水位事务边界 | PARTIAL | WAL 投影同事务写 lease+watermark+facts outbox（`internal/dhcp/lease/wal_store.go applyEvent`）；但 HA 镜像行(facts)应用在 standby 的 SQLite 事务（`mirror.applyOps`）与 primary 的 WAL 事件是**跨机最终一致**，非分布式单事务（正确，但需按 PARTIAL 表述）。 |
| WAL/synchronous=FULL/fsync | PASS | 见 06。 |
| ACK 后崩溃丢租约 | PARTIAL（代码满足；真实断电 NOT_VERIFIED） | 见 06 §4；SIGKILL 演练已验证 replay。 |
| 备用真正落盘后才回执 | PASS | `internal/dhcp/ha/mirror.go:344-347` `applyOps`（提交事务）→ `confirm(conn, seq)`；`applySnapshot` 提交后才确认。 |
| 复制失败→本地持续服务 的降级持久化 | **FAIL（与 DEC-006 冲突）** | 复制失败默认 `paused`（`state.go` `MayBind=false`）→**不 ACK**，需 `goddi ha degrade` 显式批准（`main.go:273-291`，`replicator.go:684-697`）。**任务书 DEC-006 要求"原 Owner 本地持久化后继续服务"**；ADR-0003 却"接受暂停新租约"。→ **文档冲突 P0，待 ADR 决议**。降级标记持久化到 `dataplane_meta`（`replicator.go:631-707`）本身 PASS。 |
| 并发 ACK/提交/Epoch 竞态 | PARTIAL | 无 epoch 写栅栏；`requreConfirm` 与 paused 状态转换存在窗口，NOT_VERIFIED 并发压测。 |
| 重试/重复/重放幂等 | PARTIAL | facts `Watermark.AdvanceTx` 幂等（`facts/event.go:70+`、`ipam/facts_consumer.go:110`）；WAL 以 `Version/Seq` 递增（`wal.go`）。DHCP 报文层重复 REQUEST 幂等依赖租约按 IP/MAC 查询，未做显式报文去重（NOT_VERIFIED）。 |

## 3.3 日志复制与恢复
| 检查项 | 状态 | 证据 |
|---|---|---|
| Lease Sequence 与 Facts Sequence 分离 | PASS | `dataplane_meta` 键 `ha_seq/ha_acked_seq/ha_applied_seq`（`mirror.go:23-33`）与 `ha_facts_applied_seq/ha_peer_facts_seq`（`mirror.go:32-34`）分开维护。 |
| Event ID / Epoch / PrevHash / PayloadHash / TipHash | **FAIL/PARTIAL** | `facts.Envelope`（`internal/facts/event.go:21-37`）**无 PrevHash/PayloadHash/TipHash/Epoch**；仅 `EventID+Sequence+Watermark`。无哈希链→**无篡改检测**、无分叉证明。 |
| 缺口/乱序/重复检测 | PARTIAL | `facts.Watermark` 拒绝未来 sequence（gap）与幂等重放（`event.go:70+`）；**无 prev-hash 分叉检测**。 |
| 重传/重启/快照/断点续传幂等 | PARTIAL | `facts.ReplicaStage`（`replica_stage.go:16-171`）分块+manifest+watermark，快照与增量有单测；无故障注入运行证据。 |
| 镜像回执在落盘后生成 | PASS | `mirror.go:344-347,430` 提交后 `confirm`。 |
| 崩溃在提交/回执/ACK 边界安全 | PARTIAL | 单测覆盖（`wal_store_drill_test.go`、`facts/replica_stage_test.go`）；无真实断电。 |
| 是否存在"最大 Sequence/时间戳直接覆盖对端" | PASS（未发现） | 镜像按 primary 顺序应用并校验水位（`replica_stage.go:126-161`），非盲目覆盖。 |
| 证明备用覆盖全部已承诺租约 | PARTIAL | ADR-0003 论证"备内容 ⊆ 主承诺"+按 ACK 序号恢复；**未形式化验证**，且 facts 无哈希链无法证明完整链。 |

**DEC-007（仅当链完整/身份正确/L/F 一致/双端持久化屏障成功才恢复镜像；不一致进入阻断）**：
- 现状：有 `degraded` 标记与 `paused`，但**无"不一致→阻断状态"专用状态机**（`state.go` 仅 solo/primary/paused/primary-degraded/standby/fenced）。`ha_accepted_gap` 记缺口（`role.go:53`），但**是否阻断自动恢复需代码走查确认**（NOT_VERIFIED）。→ **PARTIAL，建议补齐阻断状态**。

## 3.4 人工恢复与接管
| 检查项 | 状态 | 证据 |
|---|---|---|
| 人工恢复 RBAC/近期 MFA/理由/审计/证据 | PARTIAL | CLI `ha takeover/fence/rejoin/degrade`（`main.go:236-357`）要求多个 `--confirm` 标志与 `--reason`；是否强制 MFA/审计入库 NOT_VERIFIED（CLI 本地操作）。 |
| DEC-008 单管理员审批仅启动重新验证 | PARTIAL/NOT_VERIFIED | `TakeoverOptions`（`role.go`）与 `ha_accepted_gap`；是否"仅启动重新验证而非跳过一致性"需逐函数核对，当前标记未验证。 |
| 旧主 Fencing 证据可靠 | PARTIAL | `Fence` 写 `ha_fenced_at`（`role.go:584`）；无外部 fencing/witness（ADR-0003 明确不引入 etcd/见证，接受人工）。 |
| 未复制租约/未知 ACK/不确定地址隔离 | PARTIAL | `ha_accepted_gap` 记录缺口；隔离策略未完整实现/未验证。 |
| 旧主重新上线恢复过期写权限 | PARTIAL | 旧主重启 `EffectiveRole` 以库中 `ha_role` 为准（`role.go:115`），被 fence 则 `StateFenced` 不服务；但**运行中旧主无 epoch 感知**（若未重启且状态误判）存在风险。 |
| 管理 API 强制健康/覆盖日志/绕过审批 | **未发现危险入口（PASS）** | `/cluster` 501；无 force-overwrite API（`grep` 未见）。但需持续守卫。 |
| 镜像重新验证 vs DHCP Owner 接管状态机分离 | PARTIAL | 目前仅一套 HA 状态（role/degraded/fenced），未见独立的"镜像重新验证"状态机。 |

## 结论
- H2 的**"ACK 前双端持久化 + 单写热备 + 不自动接管"骨架真实存在且有演练**（`scripts/d2_ha_wal_canary_drill.sh` 17/0；见 07）。
- **P0 缺口**：(a) 数据面复制无 mTLS/证书轮换/防重放加密；(b) 无 Owner Epoch 写入栅栏；(c) facts 无哈希链（无可审计的完整性证明）；(d) DEC-006 与实现/DEC-007 冲突需 ADR 决议；(e) 多节点配置发布未实现（ADR-0005）。

> **后续（2026-11-12）**：DEC-006 与 ADR-0003 的冲突已由 **ADR-0010（已批准）** 收敛：默认 `paused`，自动 degraded 继续以 epoch+quorum+mTLS 为前提。本文 §3.2 的"FAIL（决策冲突）"按已裁定文本更新为"已裁定，待实现 E-1/E-2/E-3"。
