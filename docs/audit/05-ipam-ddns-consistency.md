# 05 · IPAM / DDNS 一致性审计

> 基线 commit `8e769d7`。证据：`internal/ipam/*`、`internal/dhcp/dns_facts_consumer.go`、`internal/facts/*`、`migrations/027_dhcp_ipam_observation_inbox.sql`、`internal/dataplane/sync.go`。

## 1. IPAM 权威配置 vs DHCP 租约投影（分离）
- IPAM 配置权威（space/subnet/address/reservation）在控制库，复制到 DHCP 数据面：`dataplane/sync.go` `ipam_subnets`(`replaceWhole`) 等；DHCP 从本地 `ipam_subnets` 副本分配（避免 ACK 时查控制库）。
- DHCP 租约权威在 `dhcp/lease`（`lease.Manager`/`WALLeaseStore`）；IPAM 的"占用"是租约的**投影**：`internal/ipam/linkage.go`（`heldLeaseStatuses` `:30-33`，`detectConflicts` `:781+`）。**分离：PASS**。
- IPAM 不把探测结果当权威租约：`linkage.go:122` 注释"known-conflicting address 记为 free 会把冲突地址发出去"——有防护。**PASS（证据在代码注释/逻辑）**。

## 2. 唯一性 / 重叠规则（VRF/VLAN/Prefix/Subnet/Address/Reservation）
- 包结构：`internal/ipam/{space,subnet,address}`；导入计划 `import_plan.go`/`subnet_import_plan.go`。
- 重叠处理：`linkage.go:243` 提及"overlapping address space 的 unambiguous 形式"；`detectConflicts` 比较 IPAM/发布记录/scope/leases/reservations。
- **判断：PARTIAL**——存在冲突检测与导入计划，但**未证明 VRF/VLAN/Prefix/Subnet 全维重叠拒绝的运行级测试**（NOT_VERIFIED）。

## 3. Transactional Outbox（DHCP → IPAM → DDNS）
- 生产者：DHCP WAL 投影同事务写 `ObservationOutbox`（`internal/facts/outbox.go`、`wal_store.go applyEvent`）。
- 中继/入站：`facts/relay.go` → 控制库 inbox `dhcp_ipam_observation_events`（`migrations/027...:2-30`）→ 消费者。
- IPAM 消费：`internal/ipam/facts_consumer.go:85-126`（投影+watermark+`MarkDoneTx` 同事务）。
- DNS 消费：`internal/dhcp/dns_facts_consumer.go`（幂等 `:14-17`，apply+inbox 完成同库 `:52-54,93-125`）。
- **判断：PASS（单节点实现+单测）**；**跨库非单事务**（lease 库 outbox vs 控制库 inbox），属最终一致设计（N6 文档已述），正确但要按 PARTIAL/最终一致表述。

## 4. Outbox 丢失 / 重复 / 乱序
- 重复：`Watermark` 幂等、`MarkDoneTx`；DNS consumer "idempotent, generation-tolerant"（`:14-17`）。
- 乱序/缺口：`Watermark` 拒绝未来 sequence（`facts/event.go:70+`）。
- 丢失：relay 重试 + inbox `next_attempt_at`（`027` 索引 `:24-26`）；DLQ 未见（失败留在 inbox 重放）。
- **判断：PARTIAL**——机制齐全（单测），**无故障注入运行证据**（relay 中断、consumer 崩溃、重复投递 NOT_VERIFIED）。

## 5. DNS A/AAAA/PTR 幂等/重试/补偿/发布回执
- `dns_facts_consumer.go`：bind/renew/activate 发布名字，release/expire/decline 撤销（`toDNSEvent` `:66-69`）；apply 幂等、失败留 inbox 重放（`:93-125`）。**PTR 更新**：由 `DNSLink.ApplyOne` 处理（原 legacy queue 同实现，注释 `:14-15`）。**PARTIAL**（PTR 具体逻辑需在 `DNSLink` 核对；本轮未逐行验证）。
- 发布回执：DNS 消费"apply 与 inbox 完成同库同事务"（`:52-54`），但**没有向控制面回传"DNS 已发布"的跨机回执**（单机库内完成）。**PARTIAL**。

## 6. IPAM 滞后时 DHCP 仍正确分配？
- DHCP 从本地 `ipam_subnets` 副本分配；若 IPAM 配置更新滞后（复制未到），DHCP 可能按**旧网段/旧池**分配 → **潜在错误分配风险**。
- 缓解：scope/pool 变更属配置发布；但**无"配置 staleness 拒绝分配"的显式栅栏**（未发现 generation 校验）。**PARTIAL/风险（NOT_VERIFIED）**。建议：DHCP 分配前校验 `ipam_subnets` 的 `generation`/`updated_at` 新鲜度或依赖显式发布屏障（ADR-0005）。

## 7. 两节点配置更新冲突/部分提交
- 多节点配置发布协调 **NOT_IMPLEMENTED**（`cluster` 501；ADR-0005）。两节点各自从控制库复制，无 leader fencing → 若存在多个控制面写者将冲突。当前部署单控制面（单写）规避，但不满足 H2 多节点写协调。**FAIL（H2 目标）**。

## 8. 结论
| 项 | 状态 |
|---|---|
| IPAM 配置权威 vs 租约投影分离 | PASS |
| DHCP→IPAM→DDNS 事务性 Outbox（单节点） | PASS |
| Outbox 幂等/重放机制 | PARTIAL（缺故障注入证据） |
| DNS A/AAAA/PTR 幂等/失败留箱 | PARTIAL |
| IPAM 滞后时 DHCP 分配安全 | PARTIAL/风险（无 freshness 栅栏） |
| 两节点配置写协调 | NOT_IMPLEMENTED |
| VRF/VLAN/Prefix 重叠运行级验证 | NOT_VERIFIED |
