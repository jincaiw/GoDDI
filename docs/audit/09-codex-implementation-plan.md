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

**E-2 mTLS 复制通道 + 防重放（D-01）**
1. `internal/dhcp/ha` 用 `internal/tlsutil.ServerConfig`（`RequireAndVerifyClientCert`）启 mTLS；节点证书 CN=NodeID，根 CA 校验。
2. 帧头加 nonce + 时间戳 + 单调序号，接收端拒绝过期/重复 nonce。
3. 证书轮换/吊销流程（短期证书 + 根 CA 轮换 + 吊销名单）。
4. 测试：无/过期/错误证书拒绝、重放帧拒绝、轮换不断链。
5. 验收：抓包无明文租约；无有效证书不可连。

**E-3 quorum/witness（自动 degraded 前置，D-05/ADR-0005）**
1. 引入 leader lease/epoch 与 quorum 或 witness（ADR-0005），明确"2 节点 + witness"共识。
2. 自动 degraded 继续仅当：epoch 最新 + quorum/witness 证明无更高 epoch 新主。
3. 测试：分区无双主；quorum 丢失 fail-closed；恢复后自动回 primary 或 blocked。
4. 验收：仅在满足 DEC-006 (a) 条件时自动继续。

## 阶段 1（H2 可信身份 + 围栏，最高优先）
- **T1-1 mTLS 复制通道**（D-01，细化见 E-2）：`internal/dhcp/ha` 用 `internal/tlsutil` 启 mTLS；帧加 nonce/时间戳/序号防重放；证书轮换/吊销流程。验收：无证书拒绝、重放拒绝、轮换不断链。依赖：无。
- **T1-2 Owner Epoch 写栅栏**（D-02，细化见 E-1，**已完成基础实现 2026-11-12**）：`dataplane_meta` 增 `owner_epoch`；Takeover 单调+1；写事务校验 epoch；旧主读到更高 epoch 即 fenced。验收：旧 epoch 写 100% 拒绝+审计。依赖：T1-1。
- **T1-3 facts 哈希链**（D-03）：Envelope 增 epoch/prev_hash/payload_hash/tip_hash；生产者链式、消费者校验、断链阻断（DEC-007）。验收：篡改/断链检测并阻断。依赖：T1-2。

## 阶段 2（H2 一致性/恢复）
- **T2-1 阻断状态机**（DEC-007）：新增 `blocked/inconsistent` 状态，任何链/水位/身份不一致进入阻断，需人工按 T2-2 清理。验收：不一致不自动恢复。
- **T2-2 人工恢复/接管流程硬化**：takeover/fence/rejoin 绑定 RBAC + 近期 MFA + 理由 + 审计 + 证据（当前 CLI 有 confirm，补 MFA/审计/证据校验）；区分"镜像重新验证"与"Owner 接管"两状态机。验收：无 MFA/审批无法接管。
- **T2-3 镜像回执/覆盖证明**：以 ACK 序号集合 + 哈希链证明备覆盖全部已承诺租约；增加对账工具。验收：可证明无缺口。

## 阶段 3（多节点配置发布）
- **T3-1 实现 ADR-0005 发布协调**（D-05，细化见 E-3）：leader epoch/fencing/quorum 或 witness；两阶段 Prepare/CommitDecision/Apply；暴露三态（已持久化/节点已应用/集群全部应用）。验收：中断不产生两个已提交版本；三态可查。依赖：T1-1/T1-2。
- **T3-2 DNS LKG/原子快照**（D-14）：显式 LKG 版本戳 + 回滚；`replaceWhole` 多表同事务。验收：中断回滚到 LKG。

## 阶段 4（IPAM/DDNS 与持久化加固）
- **T4-1 IPAM freshness 栅栏**（D-09）：DHCP 分配前校验 `ipam_subnets` generation/updated_at 或发布屏障。验收：陈旧配置不分配。
- **T4-2 Outbox 故障注入 + DLQ**（D-10/D-11）：relay 中断、repeat、consumer crash 测试；可选 DLQ。验收：无丢无重、可重放。
- **T4-3 SQLite 磁盘满/fsync 错误处理**（D-08）：错误注入；写失败不 ACK。验收：ENOSPC/fsync 错误被正确传播且不半提交。
- **T4-4 备份/升级恢复演练**（D-12）：自动化 restore drill + 版本升级回滚。验收：可恢复。

## 阶段 5（运维/性能/清理）
- **T5-1 性能基准/soak**（D-18）：并发 DORA、高频续租、72h 长跑。
- **T5-2 遗留路径收敛**（D-13）：shadow/indexed lease store 按灰度计划清理。
- **T5-3 TSIG 热加载 + 授权 UI**（D-06 后续）：动态 `TsigProvider` + Vue 授权页。
- **T5-4 文档/编号统一**（D-16）：DEC/ADR 登记表。

## 风险控制
- 每阶段独立可回滚；H2 变更先在双容器演练（`scripts/d2_*`、`dns_primary_secondary_container_drill.sh`）验证，再谈真机 B8.5。
- 不得用重试/延时掩盖一致性；阻断优先于自动恢复（DEC-007）。
