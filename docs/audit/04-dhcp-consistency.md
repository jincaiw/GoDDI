# 04 · DHCP 租约与防脑裂审计

> 基线 commit `8e769d7`。证据：`internal/dhcp/server/handler.go`、`internal/dhcp/lease/*`、`internal/dhcp/ha/*`。本轮 `go test ./internal/dhcp/...` 全绿；D2 演练见 07。

## 1. DORA / 续租 / 释放 / DECLINE / 重传 路径（真实代码）
- 状态判定：`handler.go:286-346`（`requestSelecting/requestRenewing/...`）、`:399-443`（续租越池策略 `strictOutOfPoolRenewal`）。
- ACK 前置：`handler.go:491` `MayBind` 检查 → `:497-568` 提交租约 + 复制确认（`Confirm`/`ConfirmFacts`）→ `:577-583` `RequireDurable` → `:617-643` 构建 ACK。**"ACK 在权威持久化之后"：PASS**。
- RELEASE：`handler.go:688-746` `HandleRelease` → `ReleaseLease` + `replicateLeaseStateAfterRelease`。
- DECLINE：经 `internal/dhcp/lease`（MANAGER/WAL）处理冲突地址（`internal/ipam/linkage.go:30-33` `heldLeaseStatuses` 含 `conflict`）。**PARTIAL（存在处理，报文层去重/幂等 NOT_VERIFIED）**。
- 重传/重复 REQUEST：按 IP/MAC 查询已有租约后激活/续租（`handler.go:513-539`），**天然幂等**；无显式 RequestID 去重（依赖 DHCP 语义）。**PARTIAL**。

## 2. 租约权威存储与事务边界
- 两套实现并存：同步 `*lease.Manager`（SQLite）与事件溯源 `WALLeaseStore`（WAL 持久 + 异步投影 + facts 同源）。平价/事实/崩溃测试：`internal/dhcp/lease/wal_store_parity_test.go`、`wal_store_facts_test.go`、`wal_store_drill_test.go`。**PASS（测试证据）**。
- 默认热路径：`config.yaml` `dhcp.lease_store: wal`（本轮之前为 sqlite）。S1 默认走 WAL store；**S1 长期稳定性 NOT_VERIFIED**。
- 遗留：`internal/dhcp/server/lease_store_shadow.go`、`lease_store_indexed.go` 为灰度遗留路径；`startup_gate.go`/`durable_gate.go` 为注入点。**PARTIAL（复杂度/清理待办）**。

## 3. ACK 后崩溃丢租约？
- 代码满足"S1：提交(fsync)→ACK"；HA：主提交→备持久确认→ACK。SIGKILL 演练：`scripts/d2_wal_canary_drill.sh`（9/0）、`scripts/d2_ha_wal_canary_drill.sh`（17/0）证明 **SIGKILL 重启后 WAL replay 保留租约**。
- **真实断电（电源/磁盘）NOT_VERIFIED**（B8.5 BLOCKED）。**PASS（代码+SIGKILL）；真实断电 NOT_VERIFIED**。

## 4. 备用节点"真正持久化后才回执"
- `internal/dhcp/ha/mirror.go:344-347`（`applyOps` 提交事务后 `confirm`）、`:430`；`applySnapshot` 提交后才确认。**PASS**。

## 5. 复制失败时的降级（**关键冲突**）
- 默认：无对端→`StatePaused`→`MayBind=false`→ REQUEST **静默不 ACK**（`state.go`；`handler.go:491`）。需 `goddi ha degrade --confirm` 才进入 `primary-degraded` 继续服务（`main.go:273-291`；`replicator.go:684-697`）。
- **与任务书 DEC-006 冲突**：DEC-006 要求"复制中断时原合法 Owner 在本地持久化后**继续服务**；不得把复制失败解释为必须停止全部 DHCP 服务"。实现选择**默认停止新承诺**。`docs/adr/0003` 明确"接受无围栏时暂停新租约"。→ **P0 文档/决策冲突，需 ADR 决议**（不得由审计擅改）。
- 降级标记持久化：`dataplane_meta` `ha_degraded/ha_degraded_reason`（`replicator.go:631-707`）。**PASS**。

## 6. 防脑裂 / Epoch / 旧主回归
- 无 `Owner Epoch` 写入栅栏（见 02 §3.1）。防双写依赖：单写者 + `paused` 时 `MayBind=false` + Confirm 必须成功。
- 旧主分区：Confirm 失败/对端 stale → paused，不 ACK（fail-closed）。分区恢复：两 primary 均无对端→均 paused，需人工 `fence`/`rejoin`（`role.go:584/626`）。**无自动接管（DEC-002 PASS）**；但**可用性依赖人工**，且**无 epoch 证明旧主绝不写**（NOT_VERIFIED 并发双写注入）。
- 决策方向：建议新增 `owner_epoch`（单调、持久、写前校验），旧主写入因 epoch 落后被拒并审计（见 08/09）。

## 7. 幂等 / 重放
- facts `Watermark.AdvanceTx` 幂等、拒绝 gap（`facts/event.go:70+`；`ipam/facts_consumer.go:110`）。
- HA 镜像按 primary 顺序应用并校验水位，非盲目覆盖（`replica_stage.go:126-161`）。
- DHCP 报文层无显式事务号去重（依赖 address/MAC 语义）。**PARTIAL**。

## 8. 测试证据（真实）
- 单元：`internal/dhcp/server/*_test.go`（`ack_gate_test.go`、`durable_gate_test.go`、`startup_gate_test.go`、`wal_lease_store_integration_test.go`）、`internal/dhcp/lease/*_test.go`。
- 演练：`scripts/d2_wal_canary_drill.sh`（9/0）、`scripts/d2_ha_wal_canary_drill.sh`（17/0，含 SIGKILL、fail-closed、同卷回滚）。
- **未做**：真实断电、磁盘满、网络分区恢复、并发高频续租压测 → NOT_VERIFIED（见 07）。

## 9. 结论
| 项 | 状态 |
|---|---|
| ACK 前置持久化（S1 fsync / HA 双端） | PASS（代码+单测+SIGKILL） |
| 单写热备、备不自动升主（DEC-002） | PASS |
| 复制失败默认暂停（vs DEC-006 继续） | **FAIL（决策冲突）** |
| Owner Epoch 写入栅栏 | NOT_IMPLEMENTED |
| facts 哈希链/篡改证明 | FAIL（见 02） |
| 真实断电/磁盘满/分区压测 | NOT_VERIFIED |
