# ADR-0009：DHCP HA 的事实事件复制与接管连续性

- 状态：已接受设计；实现未完成
- 日期：2026-09-24
- 相关：ADR-0001、ADR-0003、ADR-0007、`internal/dhcp/ha`、`internal/facts`

## 背景

非 HA DHCP 已在 authoritative lease transaction 中写入统一 facts outbox；data-plane Runner 异步将事件投递到 control inbox，control consumer 再原子更新 IPAM 投影和消费水位。DHCP ACK 不等待 control 投影。

当前 HA 协议只复制租约行，并使用独立的 HA lease sequence。HA 部署因此继续使用兼容 IPAM observer，不装配统一 facts producer。若只启用 producer 而不复制 envelope 与 allocator，primary 故障后 standby 的下一个 event sequence 可能回退或跳号；若只复制 envelope 而不将其纳入 durable ACK，primary 可能确认客户端租约，而 standby 尚未持有该事实。

## 决策

HA facts 在下列协议与验收条件全部实现前保持关闭。当前观测路径是安全降级边界，不得删除或改为 best-effort facts 写入。

### 1. 双水位是两个独立契约

- `lease_seq` 表示镜像已持有的租约状态变化。
- `facts_seq` 表示镜像已持有的连续 facts envelope 和对应 allocator 水位。
- 不得从其中一个水位推断另一个水位。无 facts 的租约操作可以推进 `lease_seq` 而不推进 `facts_seq`。
- `/ready`、HA 状态、接管结果与指标必须分别报告这两个水位及差值。

### 2. ACK 的复制边界包含 facts

- 绑定或续租的 authoritative transaction 原子提交 lease、facts envelope、facts sequence allocator 与兼容副作用。
- 严格双副本模式只有在 standby 持久提交对应 lease mutation 和 facts envelope 后才能 ACK 客户端。
- ACK 不等待 control database inbox 或 IPAM consumer；control DB 故障时，事件保留在两侧的本地数据面，等待后续至少一次投递。
- OFFER 等非承诺操作继续遵循 ADR-0003；其复制失败不得被误报成已确认的绑定。

### 3. Standby 原子应用

每批复制事务必须原子写入 lease rows、facts envelopes、facts allocator 的 `last_sequence` 与相应 lease/facts applied watermark。任意一项失败时整批回滚，两个水位均不得前进。重复事件仅在 event ID、sequence 与完整 envelope 均一致时幂等接受；序号冲突或缺口必须 fail closed。

### 4. Snapshot 可分块但只能整体确认

重连快照必须覆盖同一逻辑切点上的 lease rows、facts outbox 历史及 allocator 水位。事实历史可能大于单个 HA frame 上限，故传输必须分块并有总数/完整性校验；standby 先写入不可见 staging 状态，所有分块和校验通过后才在单一事务中切换快照并发布新水位。中断或校验失败只留下可丢弃的 staging 数据，不得覆盖当前已确认副本。

### 5. 接管与协议兼容

- 接管前必须验证 `facts_seq` 等于 primary 声明的最后 facts sequence，且 outbox sequence 连续；事实水位落后时拒绝正常接管，除非运维执行明确记录数据损失的降级流程。
- promoted primary 从镜像 allocator 水位继续分配 sequence；待投递事件保留原 event ID 并可重放。
- wire format 增加两个水位与 facts 分块能力时升级协议版本。旧版本 peer 必须拒绝建立冗余状态；禁止静默降级成只复制租约。
- 控制库 inbox 已提交的事件允许 producer 重放，消费端依靠 event ID/envelope 校验与 sequence watermark 幂等处理。

## 实施顺序

1. 为 facts 包增加可校验、可分块的 replica snapshot 编解码与完整性检查，不改 HA 服务装配。代码已提供 `ObservationOutbox.ReadReplicaPageTx` 与 `StreamReplicaSnapshotTx`：调用方持有单一只读事务时，按 allocator 水位有界读取 envelope 和 producer delivery state，并对 sequence 缺口 fail closed；每个 chunk 经回调持久接收后，才继续流式读取，全部成功后返回 manifest。`ReplicaSnapshotAccumulator` 校验跨页游标、固定高水位、chunk 序号与首尾边界，并对按序事件生成 SHA-256 manifest；接收侧可核对 manifest，且摘要不受传输分块大小影响。单页最多 1000 个事件且 JSON 编码总量不超过 16 MiB。定向测试覆盖状态保真、缺口拒绝、高水位一致、分块边界与不完整快照拒绝。HA wire 和主备服务接入尚未实现。
2. 升级 HA wire protocol 与 handshake 水位，接入已实现的 `StageReplicaChunkTx`/`ApplyStagedReplicaSnapshotTx`：standby 分块持久化不可见 staging 数据，校验 manifest 后在 lease/facts 同一事务中原子应用，并覆盖故障中断恢复。当前仅有存储原语，尚未接入 HA 服务或 wire protocol。
3. 将 DHCP facts mutation identity/sequence 返回至 HA replicator；把事实复制确认纳入 REQUEST/续租 ACK gate，并覆盖 release/decline/expiry。
4. 将 takeover/rejoin/fence/readiness 与双水位绑定；确保 promoted primary 装配 facts producer 和 control inbox delivery。
5. 完成断开、进程崩溃、部分 snapshot、重复 envelope、sequence 冲突/缺口、control DB 长时间不可用及 takeover/replay 的自动化故障矩阵。
6. 在真实双主机网络环境验证 fencing、分区、旧主回归、control DB 恢复与 IPAM 对账后，再考虑关闭兼容 observer 或发布 HA facts 能力。

## 结果与边界

该决策定义了实现边界，不代表协议已实现或通过部署验收。W04/W09 继续标记未完成；现有 HA 不宣称统一 facts durability，正式版本不得将本 ADR 或未来单机模型测试描述为 HA facts GA 证据。
