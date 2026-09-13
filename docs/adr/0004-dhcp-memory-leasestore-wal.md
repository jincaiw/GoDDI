# ADR-0004：DHCP 内存 LeaseStore 与异步 WAL（设计与 spike）

- 状态：**分阶段契约完成，默认生产实现未切换**（2026-09-13）
- 本轮边界守卫：默认 `dhcpserver.New` 仍使用 SQLite-backed `lease.Manager`；`MemoryIndex.ClaimAvailable` 不满足 DHCP `LeaseStore` 接口，未接入 `HandleDiscover`；`durableGate` 默认为空，WAL durable ACK 仍为可选 hook。
- 范围：B8.6 / N3 / N4
- 基线：已发布 `v0.8.2` / `ccf9eff`；历史设计基线为 `v0.8.0` / `28ed5aa`
- 相关：ADR 0001、ADR 0003、`internal/dhcp/lease/lease.go`、`internal/dhcp/lease/durability_test.go`

## 背景与现状

当前 `lease.Manager` 直接持有 `*sql.DB`。`CreateLease`、`ReserveAddress`、`ActivateLease`、`RenewLease`、`ReleaseLease`、`ExpireLeases` 和可用地址查询仍以 SQLite 为事实来源。现有 WAL/FULL、崩溃恢复和 HA 镜像证据证明的是当前数据库路径的正确性，不等于已经拥有内存 LeaseStore。

B8.6 的目标是先固定设计边界并用最小 spike 验证 WAL 事件的顺序、幂等重放、缺口拒绝和有界回压；本 ADR **不宣称已替换生产 Manager，也不宣称已完成容量或掉电验收**。

## N3.1 已落地的生产切入边界

- `internal/dhcp/server.LeaseStore` 现在只覆盖 DHCP 数据面实际调用的租约操作；控制台分页、利用率统计和 SQL 维护不被强行塞入热路径接口。
- `*lease.Manager` 仍是默认且唯一的生产实现，已通过编译期断言满足该接口；`Server.New` 的现有签名和 SQLite 行为保持不变。
- 新增 `Server.NewWithLeaseStore` 与装配测试，为后续 shadow、内存读路径和 WAL 实现提供注入点；本批没有改变 ACK、持久化或故障语义。
- 因此本批只证明“接口化和安全切入点已建立”，不证明内存 LeaseStore、WAL durable boundary、异步 SQLite apply 或生产 ACK gate 已完成。

N3.2 已新增 `ReadShadowLeaseStore`：主 Store 始终权威，候选 Store 仅异步比较 `GetLease`、`GetLeaseByMAC`、`GetHeldLeaseByIP` 和 `FindAvailableIP`；比较队列有界，队列满时丢弃比较任务并计数，候选错误不会泄漏到 DHCP 请求或 ACK。N3.2 不改变任何写入和持久化语义。

N3.3 已新增 `lease.MemoryIndex` 与 `server.IndexedLeaseStore`：索引支持快照原子替换、按 ID/MAC/IP 读取、held 状态、enabled reservation、确定性首个空闲地址和写后同步更新；未 `MarkReady` 前强制回退到 SQLite。该步骤仍是 SQLite 写权威，不是生产内存事实源。

N3.4 已新增版本化 WAL 记录、顺序重放、重复幂等、gap/corruption fail-closed、显式 `Sync` durable boundary，以及独立 `DurableLeaseGate` 契约。当前生产 Server 仍未装配 WAL gate；因此不宣称 ACK 已由 WAL durable 保护，异步 SQLite apply 和真实掉电仍未完成。

N3.5 已新增 `internal/dhcp/lease/wal_file.go` 文件级生命周期组件：以 owner-only `0600` 创建/打开带 magic/version header 的 WAL，启动时先校验 header 并完整 replay 校验已有记录，再恢复追加 sequence；追加与 `Sync` 分离，关闭后的操作 fail-closed，临时文件重启、gap、损坏、权限和关闭语义均有真实文件测试。该组件仍是可注入的文件级基础设施，不改变默认 SQLite `Manager`，不接入生产 ACK 路径，也不等同真实掉电耐久性。

N3.6 已新增 `internal/dhcp/lease/async_apply.go` 有界异步投影组件：单 worker 按 sequence 顺序消费已 durable 的 WAL 事件，容量固定、提交非阻塞，队列满返回 `ErrApplyQueueFull`；apply 错误、sequence gap 和关闭后提交均 fail-closed，并暴露 applied watermark、队列深度和容量。`Wait` 现在同时确认队列为空且没有 in-flight apply，避免仅按队列长度提前返回。apply 回调必须由上层在同一事务中完成 SQLite 租约/DNS outbox/applied watermark 写入；本组件不改变默认 SQLite `Manager`，不接入生产 ACK 路径。

N3.7 已将 `DurableLeaseGate` 作为显式可选装配点接入 `Server.HandleRequest` 的 ACK 构建前：配置 gate 时 durable 失败保持静默不 ACK，未配置时旧 SQLite 路径保持原行为；集成测试覆盖成功 ACK、失败静默和默认路径兼容。该接线仍属于 staged contract，不代表默认 DHCP 已迁移到内存事实源，也不代表真实 WAL 已由生产装配提供。

N4.1 已新增 `RecoverMemoryIndex`：先在临时 map 上装载 SQLite/快照基线，再按 sequence replay WAL，全部成功后才原子 `Replace` 目标内存索引；baseline 缺失 ID、WAL gap、损坏或 apply 错误均保持目标索引不变并返回错误。该组件固定了启动恢复的 fail-closed 边界，但尚未接入进程启动、控制面快照读取或默认 DHCP 装配。

N4.2 将恢复编排接入 `WALFile` 生命周期：`WALFile.RecoverMemoryIndex` 复用已打开文件的 header/replay 校验，不重复打开或绕过文件锁；真实临时 WAL 测试验证 append、sync、恢复和目标索引发布。N4.2 仍只提供装配适配器，尚未改变 `New` 的默认 SQLite 路径。

N4.3 新增 `WALDurableGate`：由 ACK 前置 gate 显式执行事件构造、WAL append 和 `Sync`，上下文取消、nil 依赖和文件错误均 fail-closed；不自动提交 SQLite applier，不把 WAL durable 误写成 SQLite applied，也不改变默认 Server 构造。本轮补充 `WALFile.AppendDurable`，将 sequence 分配、append 和 `Sync` 置于同一文件临界区，避免多 worker 并发 gate 读取同一旧 sequence 后发生重复序号竞态，并补充文件级自动序列与关闭后 fail-closed 测试。

N4.4 新增 `MemoryWriteBridge`：按 memory update -> WAL durable -> async projection submit 的顺序协调一次租约状态变更；WAL durable 失败时恢复旧内存值且不提交 projection，projection queue 满或 apply 失败时保留已 durable 的内存值并返回错误，后续由 replay 追平。该桥是 staged contract，不替换 `lease.Manager`，也不自动接管 DHCP 热路径。

后续必须完成真实内存写事实源、启动 replay/apply 装配、崩溃/掉电/容量验收，并保留 SQLite fallback 和独立负向测试。

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

必须拒绝：版本未知、`seq <= 0`、空租约 ID、未知操作、JSON 损坏、跨事件序号缺口。当前实现对尾部未完成记录与中间损坏统一 fail-closed：`ReplayWAL` 遇到无法完整解码的行直接返回 `ErrWALCorrupt`，不截断、不静默忽略；恢复前必须保留原始 WAL 供取证。若未来改为允许截断尾部半条记录，必须单独变更 ADR、增加告警和回归测试，不能隐式改变语义。

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
