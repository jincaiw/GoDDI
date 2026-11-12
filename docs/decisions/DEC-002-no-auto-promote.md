# DEC-002：禁止自动接管（备不得凭心跳超时升主）

- 状态：**已确认（2026-11-12 正式化；源自 ADR-0003 决策 4/7）**
- 权威来源：`docs/adr/0003-dhcp-ha-contract.md`、`docs/adr/0010-replication-loss-service-policy.md`

## 决策
- 备用节点**不得**仅凭心跳超时/对端失联自行提升为 primary；两节点无法区分"对端宕机"与"网络分区"，只有运维能判断。
- 心跳超时只能把节点带入 `paused`（不承诺新/延长租约），**不是**提升理由。
- 提升（takeover）必须是**显式运维动作**，且要求：操作者确认、旧主已无法写（fenced）、缺口被理解（accept-gap）——见 `goddi ha takeover` 的多个 `--confirm` 标志（`cmd/goddi/main.go`，`internal/dhcp/ha/role.go` `Takeover`）。
- 被 fence 的节点重启后 `EffectiveRole` 以存储中 `ha_role` 为准，不得以旧配置恢复为 primary（`internal/dhcp/ha/role.go:115`）。

## 状态/证据
- 实现：`internal/dhcp/ha/state.go`（无自动提升路径）、`replicator.go` `State()` 无对端→`paused`、`ha_test.go`/`operator_test.go` 覆盖不自动提升。
- 验收：断开对端后节点进入 paused；无任何代码路径在无操作者输入时改 `ha_role=primary`。
