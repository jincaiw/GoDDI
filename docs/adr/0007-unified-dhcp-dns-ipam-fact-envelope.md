# ADR-0007：DHCP-DNS-IPAM 统一事实事件边界（迁移期契约）

- 状态：**迁移期协议契约已建立，生产事实源尚未统一**（2026-09-13）
- 范围：N6
- 基线：`v0.8.3` / `41e724a`
- 相关：ADR-0004、ADR-0005、`internal/facts`、`dhcp_dns_events`、IPAM observation

## 背景

当前 DHCP-DNS 已有 durable outbox，IPAM 已有同步 observation 和 reconciliation，但三者仍由不同局部事实源驱动。现阶段不能把这些局部机制称作统一、全局可重放的事实链，也不能用单机 SQLite 事务推导跨进程最终一致性。

本 ADR 只建立兼容迁移期需要的共同事件 envelope、顺序水位和 fail-closed 语义。它不替换现有 DHCP-DNS outbox，不改变默认 DHCP SQLite 热路径，也不接管 IPAM 写入。

## 决策

### 1. 统一 envelope

`internal/facts.Envelope` 包含：

- `event_id`：稳定事件身份，用于幂等关联；
- `version`：envelope 版本；
- `entity` / `action`：实体和状态动作；
- `generation`：同一实体的业务代次；
- `sequence`：事实流单调序列；
- `source`：事实产生方；
- `occurred_at`：产生时间；
- `payload_version` / `payload`：版本化不透明载荷。

编码前必须拒绝未知 envelope 版本、非正 sequence、空身份、空来源、零时间、无效 JSON 和空载荷。消费者不得在校验失败时静默跳过事件。

### 2. watermark 与 gap

`internal/facts.Watermark` 规定：

- `sequence <= applied`：幂等重放，不重复应用；
- `sequence == applied + 1`：允许应用并推进水位；
- `sequence > applied + 1`：拒绝并报告 gap，等待补齐或快照；
- 任何失败不得推进水位。

生产消费者最终必须将投影变更与 applied watermark 放在同一 durable 事务中；当前包只提供内存协议测试，不提供该事务实现。

### 3. 迁移期兼容边界

在统一事实链正式接管前：

- `dhcp_dns_events` 继续作为现有 DNS durable outbox；
- IPAM `ObserveLease` / `Reconcile` 继续按现有接口运行；
- 不删除旧表，不改变既有 generation 语义；
- 不把 envelope 测试标记为三者最终一致；
- DHCP、DNS、IPAM 的默认跨重启 replay、默认共享 durable watermark 和默认 lag/gap 指标仍未完成；facts consumer 的 opt-in replay、watermark、backlog/readiness 与低基数指标不改变该边界。

## 验证

当前仓内测试覆盖：

1. envelope JSON round-trip；
2. 非法 payload 拒绝；
3. 重复事件幂等；
4. sequence gap 拒绝且不推进水位。

这些测试证明协议边界，不证明跨进程 transport、统一数据库事务、真实多主机部署或外部最终一致验收。
