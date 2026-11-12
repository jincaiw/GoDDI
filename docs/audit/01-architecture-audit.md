# 01 · 总体架构审计（GoDDI 企业级 / S1 + H2）

> **审计性质**：只读、以真实源码为依据，不修改任何业务/安全决策。
> **审计基线**：Git commit `8e769d7963d4553f95cd7ebe10e8032e1c1e3360`（工作树在该提交上干净）。
> **工具链**：`go version go1.27.1 darwin/arm64`，`go.mod` 声明 `go 1.26.6`，直接依赖 51 项。
> **证据命令（本轮已执行，真实结果）**：
> - `go build -mod=readonly ./...` → exit 0
> - `go vet ./...` → exit 0
> - `go test -mod=readonly ./...` → exit 0，**102 个包 `ok`，0 `FAIL`**
> 备注：单元测试通过**不等于**分布式一致性成立；故障注入项见文档 07，多数为 NOT_VERIFIED。

## 0. 审计基线冲突（重要）
- 任务书要求"检查 DEC-001～DEC-019"。**全仓库未找到任何 `DEC-0` 标识**（`grep -rli "DEC-0"` 无结果）。实际生效的决策记录是 `docs/adr/0001–0009`。**结论：DEC-001~019 属"文档缺失/未入库"，不得作为已批准基线引用**；本审计以 `docs/adr/*` 为准并标注。
- `docs/adr/0005-multi-node-config-publish-coordination.md` 明确状态为"**设计完成，生产多节点协调未实现**"，且 `internal/cluster` 仍是 `ErrNotImplemented`。因此"多节点配置发布协调"属**设计阶段**，不是已实现功能。
- 任务书 DEC-006（复制中断时原 Owner 继续服务）与 `docs/adr/0003`（"接受无围栏时暂停新租约"）**存在直接冲突**，实现选择后者（详见 04）。该冲突必须由 ADR 决议，本轮不改。

## 1. Go 模块划分与依赖
`internal/` 实际模块（`ls internal/`）：
api · apps(runtime/sdk) · audit · auditlog · auth · backup · capability · cluster · config · configver · database · dataplane · dhcp · dns · enterprise · facts · gen · ipam · log · metrics · rbac · secretbox · sqlitepolicy · system · task · tlsutil。

分层判断：
- **控制面**：`api`、`auth`、`rbac`、`configver`、`system`、`backup`、`audit*`。
- **数据面**：`dns/*`（权威/缓存/转发/传送）、`dhcp/*`（server/lease）、`ipam/*`、`dataplane/*`（跨角色复制）。
- **横切**：`facts`（统一事实信封/序列/水位/复制）、`secretbox`（密钥封装）、`sqlitepolicy`（连接级 PRAGMA）、`tlsutil`。
- **HA/集群**：`dhcp/ha`（DHCP 单写热备已实现）；`cluster`（仅协议模型 + 501 边界，`internal/cluster/cluster.go:14` `ErrNotImplemented`）。

循环依赖：`go build ./...` 通过即 Go 层无编译期循环；`internal/cluster/purity_test.go` 守卫 cluster 只能保留协议模型、不得引入 server 标记（`purity_test.go:35-60`）。

## 2. 数据面与控制面隔离（真实证据）
- DHCP HA 复制走**对端直连 TCP**，不经控制库（`docs/adr/0003` 决策1；实现 `internal/dhcp/ha/replicator.go`/`mirror.go`，`net.Listen`/`net.Dial`）。**但传输为明文 TCP + 共享 `peer_token`，无 mTLS**（`internal/dhcp/ha/config.go` `PeerToken`；ha 包无 `tls.` 引用）——这是**安全隔离缺口**（见 02/08，P0）。
- DNS/DHCP 数据面从**本地 SQLite 副本**应答，不每次查控制库：`internal/dataplane/sync.go:101-124` 定义 `dnsTables`（`dns_zones` replaceWhole、`dns_records` replaceControlAuthored、`dns_zone_transfer` replaceWhole、`dns_dnssec_keys`）；DNS 服务读取本地 `dnsdata`（`cmd/goddi/main.go` `dnsRunner`/`dnsDataDB`）。
- 判断：**读路径隔离成立（本地副本）**；**写一致性依赖控制库 + 周期复制**，非强同步（见 03）。

## 3. 组件实现合理性
| 组件 | 实现状态 | 证据 |
|---|---|---|
| Controller(API/RBAC/MFA/审计) | **PASS（单机）** | `internal/api`、`internal/auth/totp.go`、`internal/rbac`；`go test` 绿；RBAC/auth HTTP 流程测试存在 |
| DNS Engine | **PASS（单机权威/缓存/转发/DNSSEC/传送）** | `internal/dns/*`；AXFR/IXFR/NOTIFY/TSIG 传送（`internal/dns/transfer/*`） |
| DHCP Engine | **PARTIAL（单机 + HA 主备可用，见 04）** | `internal/dhcp/server/*`、`internal/dhcp/lease/*`、`internal/dhcp/ha/*` |
| IPAM | **PARTIAL（配置权威 + 事实消费者实现；强一致未验证）** | `internal/ipam/*`、`internal/ipam/facts_consumer.go:85-126` |
| DDNS | **PASS（经统一事实链，非旧 ddns outbox）** | `internal/dhcp/dns_facts_consumer.go`；migration `034_retire_legacy_ddns.sql` |
| Cluster（多节点协调） | **NOT_IMPLEMENTED** | `internal/cluster/cluster.go:14`；`/cluster` 501（`internal/api/handler/extension.go:114-124`） |
| License | **NOT_IMPLEMENTED**（未发现 license 执行代码） | `grep -rli licen internal cmd` 仅命中测试文件 |
| Appliance Agent | **PARTIAL/NOT_VERIFIED** | `internal/apps/runtime`、`internal/apps/sdk` 存在，未在故障/运行时验证 |

## 4. 数据所有权、事务与事件
- 权威写：控制库由单进程写；DHCP 租约由 Owner（primary）唯一写（ADR-0003 决策2）。
- 统一事实链：`facts.Envelope`（`internal/facts/event.go:21-37`）携带 `EventID/Version/Entity/Action/Generation/Sequence/Source/OccurredAt/Payload`；序列分配器 `internal/facts/sequence.go`（`facts_sequence_allocator`，`NextTx` 要求与 mutation/outbox 同事务）。
- **缺口**：`Envelope` **无 PrevHash / PayloadHash / TipHash / Epoch**（见 §3.1/§3.3 与 02/08）——只有 `Watermark` 的序列缺口检测（`event.go:70+`），无哈希链、无篡改证明。

## 5. SQLite / 并发 / WAL
`internal/sqlitepolicy/connection.go:83-96`：连接必须 `busy_timeout=5000`、`foreign_keys=ON`、`journal_mode=WAL`、`synchronous=FULL`；并有测试断言（`sqlite_connection_test.go:23-24`）。详细见 06。

## 6. 多进程/崩溃/systemd/监控/备份/升级
- systemd 单元存在且加固：`deployments/systemd/goddi.service`（`ProtectSystem=strict`、`NoNewPrivileges`、`ReadWritePaths=/var/lib/goddi /var/log/goddi`、`Restart=on-failure`）。
- 崩溃恢复：DHCP WAL 重放（`internal/dhcp/lease/wal_file.go:83+`、`wal_pipeline.go` Replay）；本轮演练实测 SIGKILL 后 WAL replay 保租约（`scripts/d2_*_drill.sh`，见 07）。
- 备份：`internal/backup/*`（snapshot/export/加密），**未做运行时恢复演练 → NOT_VERIFIED**。
- 监控：`internal/metrics`；Prometheus 规则/告警外部验收 BLOCKED（B8.5）。

## 7. API/RBAC/MFA/审计与底层一致性
- 管理面 TLS 可选（`internal/tlsutil/tlsutil.go:58-89`，支持 `RequireAndVerifyClientCert` mTLS）。
- MFA：TOTP 实现（`internal/auth/totp.go`），登录强制/恢复码流程有 HTTP 测试。
- 审计：`internal/audit`/`auditlog`。
- **不一致点**：HA 数据面通道**不复用** mTLS/证书体系（明文 token），管理面与数据面安全等级不一致（P0）。

## 8. S1 与 H2 架构冲突
- 现状：S1（单节点）默认 `dhcp.lease_store=wal`（`config.yaml`），H2（双节点 HA）主节点跑 `wal`、备节点为 `sqlite` 镜像（standby `standsAside`）。两套存储语义并存（WAL 事件溯源 vs 同步 Manager）。
- 冲突/风险：S1 与 H2 的持久化路径不同（WAL vs SQLite Manager 镜像），恢复/取证/回滚需分别验证；默认 `wal` 是较新路径，S1 生产长期稳定性证据不足（NOT_VERIFIED）。
- 结论：**架构可运行但存在两套一致性实现并存的复杂度**；建议在 ADR 中明确 S1 默认存储与 H2 组合矩阵。

## 9. 不必要的复杂度 / 单点 / 性能
- 单点故障：控制库（`goddi.db`）是单写单库；控制面宕机时 DNS/DHCP 读路径可继续（本地副本），但**写/配置发布、审计、人工接管**依赖控制库。
- 重复实现：旧的 `lease_store_shadow.go`/`lease_store_indexed.go` 与 WAL store 并存（灰度遗留）；旧 ddns outbox 已由 `034_retire_legacy_ddns.sql` 退役。
- 性能：SQLite `synchronous=FULL` + 每次写入 fsync；HA 每条绑定等待对端确认（`confirm_timeout` 默认 1s）→ 高并发 DORA 时吞吐受对端持久化与 fsync 约束（**NOT_VERIFIED 压测**；建议基准测试）。

## 10. 总体判断
- **单机 S1**：功能面较完整、单元测试充分；**未经过长期运行/破坏性故障验收**（B8.5 BLOCKED）。
- **双节点 H2**：DHCP 单写热备的"ACK 前双端持久化"骨架**真实实现且有演练证据**；但**传输无 mTLS、无 Owner Epoch 写入栅栏、facts 无哈希链、多节点配置发布未实现**，尚不满足"企业级 H2"的强一致与可审计要求。
- 总体评级：**S1 = PARTIAL（可试运行，不宜宣称生产就绪）**；**H2 = PARTIAL（一致性骨架在，但安全与仲裁要素缺失）**。详见 10-final-audit-report.md。
