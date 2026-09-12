# ADR-0005：多节点配置发布协调（设计契约，未实现）

- 状态：**设计完成，生产多节点协调未实现**（2026-09-12）
- 范围：B8.7
- 基线：已发布 `v0.8.0` / `28ed5aa`
- 现状：`internal/configver.Service` 仍是单节点 revision/outbox；`internal/cluster` 仍返回 `ErrNotImplemented`

## 背景

当前配置版本服务能在单节点内完成 revision、`ExpectedRevision`、`IdempotencyKey`、rollback 和 release outbox，但这些机制不能证明多个进程/节点对同一资源只有一个作者。尤其不能用 SQLite 写锁、单机多进程或已有 DHCP lease HA 复制替代通用配置发布协调。

本 ADR 固化未来实现必须满足的协议边界，供后续原型和真实多节点验收使用；它不改变当前运行行为。

## 决策

### 1. 发布只能由已授权 leader 发起

- 集群视图包含 `cluster_epoch`、`leader_id`、leader lease/任期和成员状态。
- 节点只有持有当前 epoch 的 leader 授权令牌，才能创建 `staged` 或 `applied` 发布。
- 任期变化后旧令牌立即失效；发布请求必须携带 `epoch` 与唯一 `publish_id`。
- 现有单节点 `ExpectedRevision` 继续保护资源版本，但不代替 leader fencing。

### 2. fencing 优先于 promote 或重新加入

- 节点失联不能自行成为 leader；心跳超时只能进入 `suspect/paused`。
- 新 leader 产生前必须获得 quorum/witness 的确认，或由明确的外部运维动作完成旧 leader fencing。
- 旧 leader 的每次写入都必须在存储/发布日志中验证 epoch；旧 epoch 写入拒绝并记录审计。
- 被 fence 的节点不得继续对外提供写发布能力，重新加入前必须清理未确认的本地发布状态。

### 3. quorum/witness 是发布安全闸门

默认建议三票：两个数据节点加一个轻量 witness；若部署只能提供双节点，则不自动选主，改为显式 fencing + operator takeover。不能把 SQLite 单机锁误称为跨主机 quorum。

quorum 需要证明：

- 当前 epoch 只有一个可写 leader；
- leader 的发布日志至少被 quorum 接受，或明确进入 `degraded` 且禁止新的高风险发布；
- 网络分区时少数侧拒绝发布，而不是双方各自成功。

### 4. watermark 与 gap 是恢复事实

每个节点维护：

```text
applied_watermark
acked_watermark
peer_watermark
cluster_epoch
```

- 发布事件按单调 `seq` 排序；节点只能应用 `seq == applied + 1`。
- `seq <= applied` 是幂等重放；`seq > applied + 1` 是 gap，必须暂停应用并请求 snapshot/补齐。
- rollback 不是删除历史，而是生成新的发布事件和新的 revision。
- 节点 rejoin 前必须报告本地 watermark，leader 决定发增量还是全量快照；rejoin 期间节点不得接受写请求。

### 5. 发布幂等和 outbox 事务边界

同一 `publish_id` 在重试、连接断开和 leader 重选后只能产生一个逻辑 revision。以下记录必须在同一 durable 事务中形成：

- resource revision；
- publish event / release outbox；
- idempotency result；
- 本地 applied watermark。

跨节点确认不能只表示“报文已收到”，必须表示对端已持久化并应用到声明的 watermark。超时则返回未完成/未知结果，客户端可用同一 `publish_id` 查询，不得盲目生成第二个 revision。

### 6. split-brain 拒绝语义

发生以下任一情况时，写发布必须 fail-closed：

- epoch 不匹配或 token 已过期；
- leader 身份无法由 quorum/witness 或显式 operator 证明；
- 本地 watermark 与 leader 宣告之间存在未解释 gap；
- 节点被 fence、处于 rejoin、或只拥有少数分区票数；
- 应用配置失败但发布状态未能原子记录。

读请求可以按一致性等级返回本地快照，但必须标出 stale/unknown，不得静默当作最新配置。

## 状态机

```text
follower -> candidate -> leader
    |          |           |
    |          |           +--> publishing -> committed
    |          +--------------> paused
    +--> fenced <------------ split-brain / stale epoch
    fenced -> rejoin -> snapshot/catch-up -> follower
```

- `candidate` 不能直接写资源；只有获得 quorum 后才进入 `leader`。
- `publishing` 期间节点故障，重试使用同一 `publish_id`；结果未知时先查询状态。
- `fenced` 和 `paused` 都拒绝新的写发布，差别是前者需要显式 rejoin，后者可在一致性恢复后自动回 follower。

## 与现有代码的边界

- `internal/configver.Service` 当前仍是单节点服务；不新增伪造的 leader 或 quorum 字段来制造“已实现”假象。
- `internal/cluster.ErrNotImplemented` 必须保留，直到有真实跨进程协议、网络分区负向测试、旧 leader fencing 和 rejoin 证据。
- DHCP HA 的 `seq/acked/applied/fenced` 模式可作为实现参考，但租约事实复制与通用配置发布的作者、事务和权限边界不同，不能直接复用为完成证明。

## 最小原型与验收

后续原型至少要验证：

1. 同一 `publish_id` 重试只产生一个 revision；
2. 旧 epoch 写入被拒绝；
3. 少数分区不能提交发布；
4. 节点 watermark 出现 gap 时暂停，不跳过；
5. rollback 生成新 revision；
6. fenced 节点清理本地未确认状态后才能 rejoin；
7. leader 故障后，未确认结果可查询且不会重复发布。

这些是模型/进程级契约测试；只有两台以上真实主机、真实网络分区和实际部署身份系统完成后，才能形成外部 HA 验收结论。
