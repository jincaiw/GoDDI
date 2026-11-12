# N6 默认 Mutation 接入前置：覆盖、身份、存储归属与重放

> 目的：在把统一事实链接入**默认** DHCP mutation 之前，把每一类租约 mutation 的 facts 覆盖、EventID 身份来源、producer/consumer 存储归属、跨库一致性与重放语义写清楚，并用测试钉住关键契约。默认 DHCP 仍 SQLite、事实链仍 opt-in；本文件不宣称默认已接入。

## 1. Mutation 覆盖矩阵（7 类）

| Mutation | 契约 `To` | DNS | IPAM | Generation | RequiresDurable | Facts writer 方法 | 默认路径是否发事实 |
|---|---|---|---|---|---|---|---|
| `MutationOffer` (discover_offer) | offered | none | none | keep | **false** | **无（有意不发）** | 否——offer 不是 binding |
| `MutationBind` (request_bind) | active | upsert | observe | start(=1) | true | `BindLease` / `BindLeaseTx` | 是（REQUEST 新绑定） |
| `MutationActivate` (request_activate) | active | upsert | observe | increment | true | `ActivateLease` / `ActivateLeaseTx` | 是 |
| `MutationRenew` (request_renew) | active | upsert | observe | increment | true | `RenewLease` / `RenewLeaseTx` | 是 |
| `MutationRelease` (release) | released | delete | observe | keep | true | `ReleaseLease` / `ReleaseLeaseTx` | 是 |
| `MutationDecline` (decline) | conflict | delete | observe | keep | true | `DeclineLease` / `DeclineLeaseTx`、`DeclineTombstone` / `DeclineTombstoneTx` | 是（含未知地址 tombstone） |
| `MutationExpire` (expire) | expired | delete | observe | keep | true | `ExpireFactsBatch`（`facts_expiry_batch.go`） | 是（批量清扫） |

**OFFER 的定位**：`ReserveAddress` 只代表 OFFER，不涉及 binding；契约里 DNS/IPAM 都是 `none`、`RequiresDurable=false`，因此默认 `ReserveAddress` 路径**不发 facts**是正确的。若未来确需为 OFFER 留痕，身份获取方式已在下方定义，但会改变可观测语义，需单独评审（当前不发）。

## 2. EventID 身份来源

`NewMutationFactIdentity(source, spaceID, kind, leaseID, generation)` → `dhcp-fact-<sha256>`，键为 `source | spaceID | kind | leaseID | generation`（各段做转义）。同一 (source, space, kind, lease, generation) 重试得到**同一** EventID（重试稳定），跨 mutation / 跨 generation 自动区分（避免把一次更新当成另一次）。

- **source**：由调用方显式提供（默认进程用配置的 DHCP 节点 source），非从库行推导。
- **spaceID**：由 `resolveLeaseFactSpace`（scope→IPAM space）在 mutation 前解析，空则 fail-closed。
- **leaseID / generation**：取 mutation 后的 After 快照；OFFER/Bind 起始 generation 见契约（offer=0、bind=start 1）。
- **未知地址 DECLINE tombstone**：`tombstoneID` 由调用方提供（重试复用同一 ID），generation=0；因此重试幂等，不同 tombstone 不互相覆盖。
- **批量 EXPIRE**：每条 lease 一条事实，身份各自为 (source, space, expire, leaseID, generation)。

## 3. Producer / Consumer 存储归属（不宣称跨库单事务）

```
DHCP 数据面（lease 库 / 单机 all-in-one）
  FactsMutationWriter
    └─ 同一 SQLite 事务：权威 lease 写 + SequenceAllocator.NextTx + ObservationOutbox.EnqueueTx
         └─ 生产者 outbox：dhcp_ipam_observation_events + dhcp_ipam_observation_event_dirty（marker）
                     │
                     │ data-plane relay（PushFacts，按 dirty marker 上行，幂等）
                     ▼
控制库
  dhcp_ipam_observation_events（inbox 副本，append-only）
     ├─ ipam facts pipeline   → 投影 + facts_projection_watermark(domain=ipam)
     └─ DNS facts consumer    → DNSLink.ApplyOne + facts_projection_watermark(domain=dns)
```

- **存储归属**：生产者 outbox 在租约库；relay 把它复制到控制库 inbox；consumer 在控制库侧按 watermark 投影。DNS split 进程再经 `DomainFacts` 复制拿到 facts 副本。
- **跨库一致性**：**不宣称单事务**。生产侧（lease 写+fsequence+outbox）是一个本地事务；上行是异步、幂等的复制；消费侧（投影+watermark）各自本地事务。整体是最终一致，缺口用 gap/lag 观测暴露，不伪装成单库事务。

## 4. 重放语义与既有测试

| 场景 | 语义 | 既有测试 |
|---|---|---|
| 重复 event_id | outbox 幂等：同信封重复入队不新增；信封冲突则报错 | `internal/facts`（outbox/relay）；本轮新增 `TestDuplicateFactEnqueueIsIdempotentAcrossRestart` |
| 乱序 / 缺口 | consumer 以 sequence+watermark 严格 +1 推进；缺口 fail-closed，不前进水位、不写投影 | `TestWatermarkIsIdempotentAndRejectsGaps`、`TestApplyWALEventTxGapDoesNotWriteLeaseOrWatermark`、`TestNewSQLiteProjectionApplierRejectsGapAndStops`、`TestFactsPipelineReportsGapAndFailedBacklogAsFailing` |
| 重启 replay | 从持久化 watermark 之后重放；未投影 durable 事件不丢 | `TestRecoverSQLiteProjectionReplaysAfterPersistedWatermark`、`TestFactsPipelineRestartsAfterConsumerFailure`、`TestRelayReplaysIdempotentlyAfterAPartialTick` |

## 5. 结论与门禁

- 6 类需持久 mutation 均有 facts writer 且契约 `RequiresDurable=true`；OFFER 有意不发（`RequiresDurable=false`、DNS/IPAM none）。契约覆盖由 `internal/dhcp/lease/mutation_coverage_test.go` 守卫。
- producer/consumer 存储归属明确、跨库不宣称单事务、重放/缺口/重复有测试。
- **默认接入仍未完成**：默认 handler 的 OFFER 故意无 facts，durable gate 仍 nil（计划 D2）；本文件把前置条件定义清楚但**不**把事实链标记为默认生产化。
