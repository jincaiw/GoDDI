# DEC-006：复制中断时的服务策略（正式条目）

- 状态：**已确认（2026-11-12，随 ADR-0010 批准）**
- 权威来源：`docs/adr/0010-replication-loss-service-policy.md`
- 关联：DEC-002（禁自动接管）、DEC-007（不一致阻断）

## 决策
复制链路中断或对端不可达时，原合法 Owner **默认进入 `paused`**：
- **不承诺**新的或延长的租约（客户端 REQUEST 静默不 ACK；DISCOVER 仍可得到 OFFER，预约非承诺）；
- **持续**在本地权威存储持久化已发生的事实，并告警；
- 撤销型操作（RELEASE / DECLINE）照常处理。

**仅当**满足以下之一，方可在本地权威持久化后对客户端 ACK：
- (a) 具备 **Owner Epoch 写栅栏**且本节点 epoch 仍为最新（证明仍是唯一合法 Owner）；或
- (b) 运维**显式** `goddi ha degrade --confirm`（持久化授权标记 + 审计，可撤销）。

**备用节点在任何情况下都不得自行接管**。禁止以单纯重试或超时自动解除暂停；仅"链路中断"可 `paused`，检出**不一致**（哈希链断裂/分叉/水位异常）必须进入 `blocked`（见 DEC-007），不得自动恢复。

## 验收
- 断链→`paused`、新 REQUEST 不 ACK、`ha degrade`→`primary-degraded` 并 ACK、全程可审计；
- 引入 epoch 后旧 epoch 写 100% 拒绝；
- 分区/旧主回归/双主场景无重复 IP。
