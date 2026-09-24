# ADR-0009：DHCP HA 的事实事件复制与接管连续性

- 状态：协议与代码路径已实现；部署验收未完成
- 日期：2026-09-24
- 相关：ADR-0001、ADR-0003、ADR-0007、`internal/dhcp/ha`、`internal/facts`

## 背景

非 HA DHCP 已在 authoritative lease transaction 中写入统一 facts outbox；data-plane Runner 异步将事件投递到 control inbox，control consumer 再原子更新 IPAM 投影和消费水位。DHCP ACK 不等待 control 投影。

HA 以独立 lease sequence 复制租约状态。已实现 facts 全量重连快照与运行期增量 facts 帧；primary 将 mapped DHCP mutations 与 lease/outbox 原子提交，并在 REQUEST/续租 ACK 前等待 standby 同时确认 lease 和 facts 水位。facts sequence gap 会阻止 takeover。HA facts 功能仍需经过自动故障矩阵和双主机实网验收，未通过前不宣称 HA GA。

## 决策

HA facts 写入与复制已启用；HA GA 与关闭兼容 observer 的边界仍由下列验收条件控制。未映射到本地 IPAM space 的 lease mutations 保留兼容 observer 路径，不得删除或改为 best-effort facts 写入。

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

1. **全量重连快照已接通**：`ObservationOutbox.ReadReplicaPageTx`/`StreamReplicaSnapshotTx` 在调用方单一只读事务中有界读取 envelope 与 producer delivery state，并按 allocator 水位对 sequence 缺口 fail closed；HA protocol v3 同时快照 lease rows、facts outbox 与 allocator。standby 分块 staging、校验 SHA-256 manifest 后在单一事务联合应用，并返回双水位确认。定向回归验证坏 manifest 不会替换既有租约。
2. **运行期增量和 ACK gate 已接通**：primary 从 facts acknowledged watermark 读取稳定 outbox suffix，以有界 chunk 和 manifest 发送；standby 验证 base watermark 与连续 sequence 后，在事务中追加 outbox、marker、allocator 和 facts applied watermark。DHCP primary 的 mapped REQUEST/续租 mutation 原子写入 facts；HA ACK 同时等待租约与 facts durable ACK。HA facts gap 硬拒绝 takeover；sequence snapshot 要求连续，所以不提供跳过缺口的 override。
3. **剩余实现与验收**：覆盖重复/乱序帧、断开和进程崩溃、部分 delta staging、sequence conflict/gap、控制库长期不可用与 takeover/replay 的故障矩阵；验证 promoted primary 上行重放、旧主回归与恢复对账。
4. 在真实双主机网络环境验证 fencing、分区、旧主回归、control DB 恢复与 IPAM 对账后，再考虑 HA facts GA 和关闭兼容 observer。

## 结果与边界

该 ADR 不代表通过部署验收。W04/W09 的协议实现已完成阶段性接线，但自动故障矩阵与双主机现场验收仍未完成；正式版本不得仅凭单机模型测试宣称 HA facts GA。
