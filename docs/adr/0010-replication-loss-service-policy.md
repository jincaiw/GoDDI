# ADR-0010：复制中断时的服务策略（暂停新承诺 vs. 自动降级继续）

- 状态：**Proposed（待批准；不覆盖 ADR-0003，批准后由 ADR-0003 标注被本 ADR 修订）**
- 日期：2026-11-12
- 关联：ADR-0003（DHCP 单写热备，**已接受**）、任务书 DEC-002 / DEC-006 / DEC-007 / DEC-008
- 背景冲突：任务书 **DEC-006** 要求"复制中断时原合法 Owner 在本地持久化后继续服务"；**ADR-0003** 明确"接受无围栏时暂停新租约"。当前实现遵循 ADR-0003（默认 paused、不 ACK，需 `ha degrade` 才继续）。两份文本必须收敛为单一权威决策。

## 1. 问题
双节点、无第三方见证（quorum/witness）、无 Owner Epoch 写入栅栏。复制通道中断时，本节点**无法区分**：
- (a) 对端宕机/链路断，而自己仍是唯一合法 Owner；还是
- (b) 网络分区，对端已被运维提升为新 Owner。
在 (a) 下继续服务安全；在 (b) 下继续服务 = **双主并发、重复地址分配、同一地址两类 ACK**（脑裂写）。

## 2. 两个候选行为
### 选项 A：默认暂停新承诺（fail-closed，当前实现）
- 行为：无对端→`StatePaused`→`MayBind=false`→`REQUEST` 静默不 ACK（`internal/dhcp/ha/state.go`；`internal/dhcp/server/handler.go:491`）。`OFFER` 仍可（预约不是承诺）。
- 继续服务需运维显式 `goddi ha degrade --confirm`→`primary-degraded`（`cmd/goddi/main.go:273-291`；`internal/dhcp/ha/replicator.go:684-697`），降级标记持久化并审计。
### 选项 B：本地持久化后自动降级继续
- 行为：确认对端失败时自动进入 `primary-degraded`，对本地权威持久化后的请求继续 ACK。
- 好处：可用性/RTO 最好，满足 DEC-006 字面。
- 风险：**在无 epoch/无 quorum 时无法证明"仍是唯一 Owner"**，与"对端已被提升"叠加即脑裂。

## 3. 决策（建议）
1. **当前默认 = 选项 A（暂停新承诺）**。理由：两个节点无法自证唯一所有权；在缺少 epoch/quorum/mTLS 时，自动继续是"用可用性换正确性"，可能造成重复 IP（比短暂停机更严重）。这与 ADR-0003 一致，也是当前代码行为。
2. **不取消人工继续**：`ha degrade` 作为**显式、审计、可撤销**的运维动作保留（已是实现）。
3. **目标态 = 选项 B（自动降级继续），但必须同时具备以下前提**（缺一不可）：
   - Owner Epoch：持久、单调、写前强校验；旧 epoch 写一律拒绝并审计（D-02）。
   - 复制通道 mTLS + 防重放（D-01）。
   - 能够证明"无更高 epoch 的新 Owner"：三节点 quorum/witness，或至少外部 fencing/仲裁（ADR-0005 的 leader/quorum 机制）。
   满足后，自动 degraded 才是安全的，可回填 DEC-006 的可用性意图。
4. **续租的中间策略（可选，建议评估）**：即使暂停新绑定，是否允许"同一 IP、同一 MAC 的续租"自动继续以降低断链时的客户端掉线？结论：**在无 epoch 时也不建议自动放行续租**——若对端已把该 IP 重新分配，续租会造成冲突 ACK；仍应保守暂停或走人工 degrade。

## 4. 统一后的文本（供批准后落库）
**DEC-006（修订稿）**：
> 复制链路中断或对端不可达时，原 Owner **默认进入 paused：不承诺新的或延长的租约（可应答 OFFER），但持续在本地权威存储持久化已发生事实并告警**。只有在 (a) 具备 Owner Epoch 写入栅栏且本节点 epoch 仍为最新，或 (b) 运维显式 `ha degrade` 批准（持久化授权标记 + 审计）时，方可在本地权威持久化后继续对客户端 ACK。备用节点在任何情况下都不得自行接管。禁止以单纯重试/超时把暂停自动解除。

**DEC-007（对齐）**：区分"链路中断（可 `paused`，人工可降级）"与"已检出不一致（哈希链断裂/分叉/水位异常 → `blocked`，不得自动恢复）"。二者是不同状态机。

**ADR-0003 修订**：把决策 4 的"接受暂停新租约"标注为**当前默认**，并注明"待 ADR-0010 前提（epoch/quorum/mTLS）落地后升级为自动 degraded"。

## 5. 验收标准
- 断链后：`/ready` 与 `ha status` 显示 `paused`；新 REQUEST 不 ACK；`ha degrade` 后显示 `primary-degraded` 并 ACK；均可审计。
- 引入 epoch 后：旧 epoch 写被拒；自动 degraded 仅在 epoch 最新且 quorum/witness 证明后发生。
- 故障注入：分区/旧主回归/双主场景无重复 IP（见 07 矩阵 F-03/F-04/F-18）。

## 6. 后果
- 短期：断链时 DHCP 可用性受影响（fail-closed），但数据正确性优先；需配套告警与"一键 degrade"Runbook。
- 长期：补齐 epoch/quorum/mTLS 后切换为选项 B，恢复 DEC-006 的可用性目标且保持安全。
