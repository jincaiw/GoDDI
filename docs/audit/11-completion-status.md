# 11 · 实施完成度与剩余缺口总表（滚动更新）

> 基线 commit 于本轮更新；本表随实施滚动维护。状态：PASS / PARTIAL / FAIL / NOT_IMPLEMENTED / NOT_VERIFIED / BLOCKED（外部）。

## A. 缺陷实施状态（对应 08）
| 编号 | 标题 | 状态 | 证据/产物 |
|---|---|---|---|
| D-01 | HA 复制通道 mTLS+防重放 | PASS（基础）；证书轮换编排 BLOCKED | `ha.LoadTLS/serverTLS/clientTLS`、hello nonce/sentAt、`tls_test.go`；生产须配 `dhcp_ha.tls_*` |
| D-02 | Owner Epoch 写栅栏 | PASS（基础）；quorum 自动 degraded BLOCKED | `ha_owner_epoch`、Takeover +1、握手交换、`epoch_test.go` |
| D-03 | facts 哈希链 | PARTIAL：阶段一 PASS；**阶段二第 1 步 PASS（2026-11-12：双库迁移增列 + outbox/replica 透传，默认空/行为不变）**；第 2 步 PASS（开关+生产打戳+幂等）；**第 3 步 PASS（IPAM 消费者 VerifyEnvelope + blocked，2026-11-12）**；DNS 消费者校验待接线；默认关闭 | `facts/chain.go`、`migrations/035`、`dataplane/migrations/019`、`facts/outbox.go`、`replica.go` |
| D-04 | DEC-006 vs ADR-0003 冲突 | 已解决 | ADR-0010 Accepted、DEC-006/007 建立、08/10 更新 |
| D-05 | 多节点配置发布协调/quorum | NOT_IMPLEMENTED / BLOCKED（ADR-0005 外部仲裁） | `internal/cluster` ErrNotImplemented、`/cluster` 501 |
| D-06 | AXFR 授权链路 + TSIG 热加载 | PASS（API+测试+实机）；TSIG 运行期热加载 NOT implemented（需重启）；UI 未做 | `dns_zone_transfer.go`、`primary_secondary_e2e_test.go`、容器 dig 演练 |
| D-08 | SQLite FULL/IO/BUSY 错误分类 | PASS（分类+传播+测试）；真实填盘注入 NOT_VERIFIED | `sqlitepolicy/errors.go`、`errors_test.go` |
| D-09 | IPAM 投影缺口 + 可选严格栅栏 | PASS（检测+可选 guard+测试）；跨面滞后判据 BLOCKED（需 E-3 水位） | `ipam/projection.go`、`projection_test.go`、`dhcp.require_ipam_projection` |
| D-10 | outbox/relay 故障注入 | PASS（relay 中断/重启/重复幂等）；DLQ NOT implemented；consumer apply/MarkDone 崩溃同事务未单独注入 | `facts/relay_test.go`、既有 outbox/replica/consumer 测试 |
| D-11 | DHCP 租约幂等/并发 | PASS（测试 + 真实修复 UNIQUE→ErrAddressTaken） | `lease/idempotency_test.go`、`lease.go` |
| D-12 | 备份/升级恢复演练 | PARTIAL：full 备份删后恢复 DNS/DHCP/IPAM 集成测试通过；**升级路径回滚、大库/停机演练 NOT_VERIFIED** | `internal/backup/restore_full_test.go` |
| D-13 | 遗留 shadow/indexed lease store 清理 | NOT done（高风险，延后） | `server/lease_store_shadow.go`、`lease_store_indexed.go` |
| D-14 | dataplane LKG 版本戳 + 原子快照 | PASS（同事务戳+读取+测试）；显式回滚到 LKG NOT implemented | `dataplane/lkg.go`、`lkg_test.go`、启动日志 |
| D-16 | 文档/决策编号 | PARTIAL：ADR-0010 + DEC-002/006/007/008 正式化；DEC-001/003/004/005/009~019 未建档（未批准） | `docs/adr/0010`、`docs/decisions/*` |
| D-17 | Appliance agent 运行验证 | NOT_VERIFIED | `internal/apps/{runtime,sdk}` |
| D-18 | 性能/长跑基准 | PARTIAL：`scripts/bench.sh`+`make bench`（烟测通过）；soak/并发压测 NOT_VERIFIED | `scripts/bench.sh` |
| D-19 | ClusterID/节点唯一仲裁 | NOT_IMPLEMENTED（501） | — |
| D-20 | 可观测性（LKG/投影缺口指标） | PARTIAL：新增 Prometheus gauge `goddi_dataplane_lkg_revision{domain}`、`goddi_dhcp_ipam_projection_gaps`（启动置值 + **30s 周期刷新**）；抓取即时采集/更多就绪子项待做 | `metrics/metrics.go`、`main.go` |

## B. 外部阻断（BLOCKED，沙箱不可完成）
- B8.5 物理真机验收（真实主机/断电/网络隔离/fencing/真实 relay/真实监控时间线）。
- 真实 quorum/witness（无第三仲裁节点）→ 自动 degraded（E-3）。
- 真实填盘 ENOSPC / fsync 故障注入（需隔离盘）。
- 证书轮换/吊销编排（需现场 PKI）。
- 多节点配置发布协调（ADR-0005）。

## C. 结论口径
- **可实施且低风险的 P0/P1 项**（D-01/D-02 基础、D-03 阶段一、D-04、D-06、D-08、D-09 基础、D-10 relay、D-11、D-14）**已完成并有测试证据**。
- **高风险/需迁移窗口项**（D-03 阶段二、D-13 清理）与**外部项**（D-05、B8.5、quorum、真实填盘、证书轮换）仍开放。
- S1：可试运行/接近就绪（待真机与长跑）；H2：核心安全要素（epoch/mTLS/不自动提升/ACK 前双端持久化）到位，**自动 degraded 与 quorum 未开放（BLOCKED）**，**D-03 端到端哈希链未接**，故仍为 PARTIAL。
