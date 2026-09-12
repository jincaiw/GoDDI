# ADR-0004：DHCP 内存 LeaseStore 与异步 WAL（设计与 spike）

- 状态：**设计完成，未进入生产实现**（2026-09-12）
- 范围：B8.6
- 基线：已发布 `v0.8.0` / `28ed5aa`
- 相关：ADR 0001、ADR 0003、`internal/dhcp/lease/lease.go`、`internal/dhcp/lease/durability_test.go`

## 背景与现状

当前 `lease.Manager` 直接持有 `*sql.DB`。`CreateLease`、`ReserveAddress`、`ActivateLease`、`RenewLease`、`ReleaseLease`、`ExpireLeases` 和可用地址查询仍以 SQLite 为事实来源。现有 WAL/FULL、崩溃恢复和 HA 镜像证据证明的是当前数据库路径的正确性，不等于已经拥有内存 LeaseStore。

B8.6 的目标是先固定设计边界并用最小 spike 验证 WAL 事件的顺序、幂等重放、缺口拒绝和有界回压；本 ADR **不宣称已替换生产 Manager，也不宣称已完成容量或掉电验收**。

## 决策

### 1. 内存状态是数据面读路径，WAL 是顺序事实日志

生产目标结构为：

```text
DHCP request
  -> admission / worker
  -> memory LeaseStore（单写者或分片锁保护）
  -> append WAL event
  -> durable boundary
  -> ACK gate
  -> asynchronous SQLite apply / checkpoint
```

- 读路径（地址占用、租约状态、过期扫描）从内存快照读取，不能在每个请求中查询 SQLite。
- 每个状态变更先生成一个带单调 `seq` 的事件；事件追加顺序是 replay 顺序。
- SQLite 是恢复后的持久索引和控制面可读副本，不再是 DHCP 热路径的同步读源。
- DNS outbox 仍属于租约数据面事实的一部分，生产实现必须保证租约变更与应发 DNS 事件在同一可恢复事件序列中可重建。

### 2. 事件格式必须版本化、可校验、可重放

建议 WAL 使用 append-only、逐行编码的 JSONL 或等价长度前缀格式。最小事件契约：

```json
{
  "version": 1,
  "seq": 42,
  "op": "upsert",
  "lease": {
    "id": "...",
    "scope_id": "...",
    "ip_address": "192.0.2.10",
    "mac_address": "02:00:00:00:00:01",
    "status": "active",
    "lease_end": "2026-09-12T18:00:00Z",
    "generation": 3
  }
}
```

必须拒绝：版本未知、`seq <= 0`、空租约 ID、未知操作、JSON 损坏、跨事件序号缺口。尾部未完成的最后一条记录可以在启动恢复时被截断或忽略，但必须记录告警并保留原始证据；中间损坏不能静默跳过。

### 3. 三个状态边界必须分开

每条变更至少有三个可观测阶段：

1. **memory-applied**：内存状态已更新，后续请求能看到新状态；
2. **wal-appended**：事件已完整追加到 WAL，并通过 `fsync`/等价 durable flush；
3. **sqlite-applied**：异步落库事务已提交，控制面副本追平。

默认严格模式下，只有到达 `wal-appended` 才允许发送会让客户端依赖该绑定的 ACK；`sqlite-applied` 不得成为每请求的同步前提，否则又回到当前 DB 热路径。若 WAL append/flush 失败，必须回滚内存变更或进入不可继续授权的 fail-closed 状态，不能先 ACK 再补日志。

**明确窗口**：WAL 已 durable、SQLite 尚未 apply 时，机器崩溃后可通过 replay 恢复；控制面查询可能暂时落后，必须通过指标/readiness 暴露滞后。WAL 本身损坏、磁盘写满或无法 flush 时，不得继续签发新绑定。

### 4. 回压必须有界且 fail-closed

- WAL writer 与 SQLite applier 之间使用有界队列；队列满时不无限阻塞 receive loop。
- 队列满、WAL append 超时、checkpoint 滞后超过门槛时：新绑定和续租不 ACK；释放、DECLINE、过期清理可按独立安全策略继续，但不得掩盖失败。
- 入口丢弃、WAL 错误、SQLite apply 错误、队列深度和 replay lag 必须有独立指标。
- 不允许用“内存已写入”作为“客户端已获授权”的替代证据。

### 5. Replay 必须幂等且保持单调水位

SQLite apply 以 `seq` 或等价事件 ID 做幂等闸门：

- `seq == applied_seq` 的重复事件安全忽略；
- `seq < applied_seq` 的旧事件安全忽略并计数；
- `seq > applied_seq + 1` 必须拒绝并报告 gap，不能跳过；
- 应用租约行、DNS durable outbox 和 `applied_seq` 必须在同一事务中提交；
- checkpoint 成功后才能清理已覆盖的 WAL 前缀，且保留可审计的最后 checkpoint 水位。

### 6. 恢复顺序

1. 打开 WAL 并验证 header/version；
2. 从最后一个 checkpoint 读取内存快照；
3. 按 seq 连续 replay WAL；
4. 校验 `applied_seq`、内存 lease generation 和 DNS outbox 派生状态；
5. 恢复 admission；在 replay 未完成、发现 gap 或 WAL 不可读时保持 fail-closed。

当前 `durability_test.go` 的 SIGKILL 证据可复用于 SQLite apply 的崩溃一致性，但**不能**代替内存 LeaseStore WAL replay spike，也不能证明真实掉电。真实掉电仍须在目标硬件上切断电源验证。

## 未决实现选择

- 单写者 actor 与按 scope 分片锁的选择；
- WAL 文件轮转、压缩和 checkpoint 触发阈值；
- `fsync` 失败后的进程级处置（只降级、停止 DHCP 或退出）；
- 内存快照格式与版本迁移；
- DNS outbox 的事件内嵌还是从租约事件确定性派生；
- 真实客户端重试、ACK 延迟和目标硬件容量。

这些选择在生产代码改造前必须通过设计评审与目标环境压测，不能由本 spike 默认为已决定。

## 验收边界

本 ADR 配套的 `wal_spike_test.go` 只验证：事件编码/解码、连续序列、重复事件幂等、缺口拒绝和有界队列拒绝。它不替换 `Manager`，不改变 DHCP ACK 路径，不验证 20,000 租约容量、真实客户端 ACK、真实掉电、跨主机 HA 或 24 小时长稳。
