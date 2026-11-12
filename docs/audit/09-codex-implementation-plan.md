# 09 · 按依赖排序的修复任务（Codex 实施计划）

> 目标：以最低风险把 PARTIAL/FAIL 收敛到可生产的企业级 S1/H2。**先决策后编码**；破坏性改动分批、可回滚。每项含验收。

## 阶段 0（决策，无代码）
- **T0-1 修订决策登记（已完成 2026-11-12）**：**ADR-0010 已批准**（默认 paused；自动 degraded 仅 epoch+quorum+mTLS 后）；ADR-0003 标注被修订；建立 `docs/decisions/`（DEC-006/007 正式条目）。详见下方"决策落地任务"。

### 决策落地任务（ADR-0010 已批准 2026-11-12）

**E-1 Owner Epoch 写栅栏（D-02，最高优先）— 状态：已实现（2026-11-12，单测通过）**
1. 存储：`dataplane_meta` 增 `owner_epoch`（单调递增，持久于租约数据面库，与 `ha_role` 同库）。
2. 生成：`goddi ha takeover/fence/rejoin` 成功时事务内 `owner_epoch+1` 并写审计；状态展示读回。
3. 校验：所有权威写（DHCP 绑定/续租/释放/facts 投影）事务内校验 `req.epoch == current_epoch`，落后即拒绝（`ErrStaleEpoch`）+ 审计；旧主启动/重连读到更高 epoch 立即 fenced。
4. 复制：epoch 随握手交换；低 epoch 节点不得作为 primary 写复制。
5. 测试：旧 epoch 写拒绝、epoch 回退拒绝、并发 takeover 仅一个成功、旧主回归只读。
6. 验收：落后 epoch 写 100% 拒绝并审计；分区恢复后旧主 fenced。

**E-2 mTLS 复制通道 + 防重放（D-01）— 状态：已实现基础（2026-11-12，单测通过）**
1. `internal/dhcp/ha` 用 `internal/tlsutil.ServerConfig`（`RequireAndVerifyClientCert`）启 mTLS；节点证书 CN=NodeID，根 CA 校验。
2. 帧头加 nonce + 时间戳 + 单调序号，接收端拒绝过期/重复 nonce。
3. 证书轮换/吊销流程（短期证书 + 根 CA 轮换 + 吊销名单）。
4. 测试：无/过期/错误证书拒绝、重放帧拒绝、轮换不断链。
5. 验收：抓包无明文租约；无有效证书不可连。

**E-3 quorum/witness（自动 degraded 前置，D-05/ADR-0005）— 状态：BLOCKED（外部阻断，2026-11-12 评估）**

> 合理性评估：真实 quorum/witness 需第三个仲裁节点或外部见证，本沙箱无三节点/见证环境，无法实现并**验证**；伪造内存 witness 会给出虚假安全感（违反"不得伪造通过记录"）。因此**不实现自动 degraded**，默认 `paused` 保持不变（符合 ADR-0010）；E-3 标记 `external-validation-required / BLOCKED`，待现场提供见证节点后按如下设计实施。

1. 引入 leader lease/epoch 与 quorum 或 witness（ADR-0005），明确"2 节点 + witness"共识。
2. 自动 degraded 继续仅当：epoch 最新 + quorum/witness 证明无更高 epoch 新主。
3. 测试：分区无双主；quorum 丢失 fail-closed；恢复后自动回 primary 或 blocked。
4. 验收：仅在满足 DEC-006 (a) 条件时自动继续。

## 阶段 1（H2 可信身份 + 围栏，最高优先）
- **T1-1 mTLS 复制通道**（D-01，细化见 E-2，**已完成基础实现 2026-11-12**）：`internal/dhcp/ha` 用 `internal/tlsutil` 启 mTLS；帧加 nonce/时间戳/序号防重放；证书轮换/吊销流程。验收：无证书拒绝、重放拒绝、轮换不断链。依赖：无。
- **T1-2 Owner Epoch 写栅栏**（D-02，细化见 E-1，**已完成基础实现 2026-11-12**）：`dataplane_meta` 增 `owner_epoch`；Takeover 单调+1；写事务校验 epoch；旧主读到更高 epoch 即 fenced。验收：旧 epoch 写 100% 拒绝+审计。依赖：T1-1。
- **T1-3 facts 哈希链**（D-03，**阶段一完成；阶段二第 1 步完成（双库增列+outbox/replica 透传，默认空）2026-11-12**；第 2 步完成（2026-11-12：`facts_hash_chain_enabled` 开关 + 生产 outbox StampTx 打戳，默认关闭）；阶段一/二（含 IPAM+DNS 消费者校验）已完成（2026-11-12，flag 开启端到端）：Envelope 增 epoch/prev_hash/payload_hash/tip_hash；生产者链式、消费者校验、断链阻断（DEC-007）。验收：篡改/断链检测并阻断。依赖：T1-2。

## 阶段 2（H2 一致性/恢复）
- **T2-1 阻断状态机**（DEC-007）：新增 `blocked/inconsistent` 状态，任何链/水位/身份不一致进入阻断，需人工按 T2-2 清理。验收：不一致不自动恢复。
- **T2-2 人工恢复/接管流程硬化**：takeover/fence/rejoin 绑定 RBAC + 近期 MFA + 理由 + 审计 + 证据（当前 CLI 有 confirm，补 MFA/审计/证据校验）；区分"镜像重新验证"与"Owner 接管"两状态机。验收：无 MFA/审批无法接管。
- **T2-3 镜像回执/覆盖证明**：以 ACK 序号集合 + 哈希链证明备覆盖全部已承诺租约；增加对账工具。验收：可证明无缺口。

## 阶段 3（多节点配置发布）
- **T3-1 实现 ADR-0005 发布协调**（D-05，细化见 E-3）：leader epoch/fencing/quorum 或 witness；两阶段 Prepare/CommitDecision/Apply；暴露三态（已持久化/节点已应用/集群全部应用）。验收：中断不产生两个已提交版本；三态可查。依赖：T1-1/T1-2。
- **T3-2 DNS LKG/原子快照**（D-14，**基础已完成 2026-11-12**）：LKG 版本戳同事务写入（`dataplane.LKG`）+ `apply` 单事务原子；测试 `lkg_test.go`。剩余：显式回滚到指定 LKG revision；"集群全部应用"三态仍 NOT_IMPLEMENTED（D-05）。

## 阶段 4（IPAM/DDNS 与持久化加固）
- **T4-1 IPAM freshness 栅栏**（D-09，**基础已完成 2026-11-12**）：投影缺口检测 `ipam.ProjectionGaps` + 可选严格栅栏 `ipam.NewProjectionGuard`（`dhcp.require_ipam_projection`，Server `SetScopeProjectionGuard`，测试 `projection_test.go`）。剩余：跨面水位（E-3/ADR-0005）实现"相对控制面滞后"硬判据。
- **T4-2 Outbox 故障注入 + DLQ**（D-10/D-11，**基础已完成 2026-11-12**）：relay 中断/重启续传/重复投递幂等由 `relay_test.go` 覆盖；D-11 幂等由 `idempotency_test.go` 覆盖。剩余：显式 DLQ；consumer apply/MarkDone 间崩溃演练。
- **T4-3 SQLite 磁盘满/fsync 错误处理**（D-08，**基础已完成 2026-11-12**）：`sqlitepolicy.ClassifyError` 映射 FULL/IOERR/BUSY/ENOSPC/EIO；lease 写路径接入，失败即不 ACK。剩余：真实填盘/fsync 注入（NOT_VERIFIED）。
- **T4-4 备份/升级恢复演练**（D-12）：自动化 restore drill + 版本升级回滚。验收：可恢复。

## 阶段 5（运维/性能/清理）
- **T5-1 性能基准/soak**（D-18，**基准工具已加 2026-11-12**：`scripts/bench.sh`/`make bench`）：并发 DORA、高频续租、72h 长跑仍待执行（NOT_VERIFIED）。
- **T5-2 遗留路径收敛**（D-13）：shadow/indexed lease store 按灰度计划清理。
- **T5-3 TSIG 热加载 + 授权 UI**（D-06 后续）：动态 `TsigProvider` + Vue 授权页。
- **T5-4 文档/编号统一**（D-16）：DEC/ADR 登记表。

## 风险控制
- 每阶段独立可回滚；H2 变更先在双容器演练（`scripts/d2_*`、`dns_primary_secondary_container_drill.sh`）验证，再谈真机 B8.5。
- 不得用重试/延时掩盖一致性；阻断优先于自动恢复（DEC-007）。
