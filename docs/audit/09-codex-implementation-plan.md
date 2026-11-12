# 09 · 按依赖排序的修复任务（Codex 实施计划）

> 目标：以最低风险把 PARTIAL/FAIL 收敛到可生产的企业级 S1/H2。**先决策后编码**；破坏性改动分批、可回滚。每项含验收。

## 阶段 0（决策，无代码）
- **T0-1 修订决策登记**（D-04/D-16）：把 DEC-001~019 与 ADR-0001~0009 对齐；对"复制中断默认行为"二选一并写 ADR（暂停 vs 自动 degraded 继续）。产出：ADR 变更稿。验收：单一权威决策文本，无冲突。

## 阶段 1（H2 可信身份 + 围栏，最高优先）
- **T1-1 mTLS 复制通道**（D-01）：`internal/dhcp/ha` 用 `internal/tlsutil` 启 mTLS；帧加 nonce/时间戳/序号防重放；证书轮换/吊销流程。验收：无证书拒绝、重放拒绝、轮换不断链。依赖：无。
- **T1-2 Owner Epoch 写栅栏**（D-02）：`dataplane_meta` 增 `owner_epoch`；Takeover 单调+1；写事务校验 epoch；旧主读到更高 epoch 即 fenced。验收：旧 epoch 写 100% 拒绝+审计。依赖：T1-1。
- **T1-3 facts 哈希链**（D-03）：Envelope 增 epoch/prev_hash/payload_hash/tip_hash；生产者链式、消费者校验、断链阻断（DEC-007）。验收：篡改/断链检测并阻断。依赖：T1-2。

## 阶段 2（H2 一致性/恢复）
- **T2-1 阻断状态机**（DEC-007）：新增 `blocked/inconsistent` 状态，任何链/水位/身份不一致进入阻断，需人工按 T2-2 清理。验收：不一致不自动恢复。
- **T2-2 人工恢复/接管流程硬化**：takeover/fence/rejoin 绑定 RBAC + 近期 MFA + 理由 + 审计 + 证据（当前 CLI 有 confirm，补 MFA/审计/证据校验）；区分"镜像重新验证"与"Owner 接管"两状态机。验收：无 MFA/审批无法接管。
- **T2-3 镜像回执/覆盖证明**：以 ACK 序号集合 + 哈希链证明备覆盖全部已承诺租约；增加对账工具。验收：可证明无缺口。

## 阶段 3（多节点配置发布）
- **T3-1 实现 ADR-0005 发布协调**（D-05）：leader epoch/fencing/quorum 或 witness；两阶段 Prepare/CommitDecision/Apply；暴露三态（已持久化/节点已应用/集群全部应用）。验收：中断不产生两个已提交版本；三态可查。依赖：T1-1/T1-2。
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
