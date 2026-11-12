# N6 统一事实链灰度装配评审

> 目的：把统一事实链在默认进程中的**实际装配图**、opt-in/回滚开关、失败 consumer 的 Stop→重建→Start 恢复闭环、以及 readiness 的 lag/gap 观测逐项钉住，并把每条验收判据映射到代码与既有测试。默认 DHCP 仍 SQLite、事实链的 stage 组件（WAL store / durable gate）仍 opt-in；本文件是装配评审，不宣称生产 HA/验收。

## 1. 装配图（单机 all-in-one；split 按同一形状拆分到各进程）

```
生产者（lease 库，dhcpStore.DB）
  facts.SequenceAllocator(dhcpStore.DB)
  facts.ObservationOutbox(dhcpStore.DB)            ← 生产者 outbox（+ dirty marker）
  lease.FactsMutationWriter(manager, allocator, outbox)
    ├─ SetLeaseFactsMutation(主路径：Bind/Activate/Renew/Decline)
    └─ SetReleaseFactsMutation(RELEASE)
  （仅 primary 或非 HA 装配；standby 只接收 primary 的 facts）
        │  PostCommitWake → dhcpRunner.Wake
        ▼
relay（controlPlane）
  facts.NewRelay(dhcpFactsProducerOutbox, controlInbox=ObservationOutbox(db.DB), db.DB)
        ▼
消费侧（控制库 db.DB）
  ipam.NewFactsPipeline(ipamLinkage, controlInbox, {Enabled:true, Relay})
    ├─ FactsConsumer → IPAM 投影 + facts_projection_watermark(domain=ipam)
    └─ 内置 relay 循环（若 Relay 传入）
DNS 面（servesDNS）
  facts.NewObservationOutbox(dnsStore.DB)            ← DNS inbox
  dhcpinternal.NewDNSFactsConsumer(dnsLink, dnsInbox)
  facts.NewRelay(dhcpFactsProducerOutbox, dnsInbox, dnsStore.DB)   ← 单进程直连
  1s Replay(batch=100) → DNSLink.ApplyOne + ReloadZones
  split：DNS 进程经 DomainFacts 复制从控制库 inbox 取 facts（无 lease store）
```

## 2. Opt-in / 回滚开关

| 组件 | 开关 | 关闭时行为 |
|---|---|---|
| `ipam.FactsPipeline` | `FactsPipelineOptions.Enabled` | `NewFactsPipeline` 构造为 no-op，不校验/不触碰 consumer 依赖；Start/Stop/Status 安全 no-op；`Status.Enabled=false` 显式「未配置」而非伪装健康 |
| 生产者 mutation seam | 仅在 `!haEnabled || haRole==primary` 时 `SetLeaseFactsMutation`/`SetReleaseFactsMutation` | standby 不写 facts（只接收 primary 流） |
| WAL lease store（`WALPipeline`）/ durable gate | 默认**不装配**（`TestDefaultProcessDoesNotAssembleWALPipeline` + `durableGate==nil`） | 默认 SQLite 事务提交即持久边界 |
| DNS facts consumer | `servesDNS` 分支才有 | 不服务 DNS 的进程不装配 |

解析：事实链的 **consumer/pipeline** 在控制面已是默认装配（round 273–280）；仍 opt-in 的是 **stage 组件**（WAL store / durable gate）。回滚 = 关闭 pipeline 的 `Enabled`（或进程不装配该分支），不产生半装配。

## 3. 失败 consumer 恢复闭环（Stop → 重建 → Start）

- `FactsPipeline.Stop`：先停 relay、再 `consumer.Stop(ctx)` 等 in-flight 投影事务。**超时/取消返回错误，绝不被转换成「排空成功」**（`facts_pipeline.go` Stop 注释与实现）。
- 若 `consumer.Stop` 返回 `ErrFactsConsumerFailed`：清 wrapper 的 `started=false`（只清包装状态，不动已退出的 runner），下一个 `Start` 会 `NewFactsConsumerWithOptions` **重建全新 consumer** 后启动；超时/取消则保留当前 runner 归属，不误判。
- 未投影的 facts 是 durable 的（outbox + watermark），重建后从 watermark 继续 replay，不丢。
- 既有测试：`TestFactsPipelineRestartsAfterConsumerFailure`、`TestFactsPipelineStartAndStopAreIdempotentAtWrapperBoundary`、`TestFactsPipelineConcurrentStopIsSafe`。

## 4. Readiness 的 lag / gap 观测

- `/ready` 的 `ipam_facts` / `dhcp_facts` / `dns` plane 报告 lifecycle、applied watermark、next expected、durable head、pending/failed backlog、lag、gap。
- failed 事件保留在 outbox，**不把未知/缺口伪装成健康**：failed→Failing、backlog→Degraded。
- 既有测试：`TestFactsPipelineReportsLagAsDegradedWhileRunning`、`TestFactsPipelineReportsGapAndFailedBacklogAsFailing`。

## 5. 验收判据映射

| P4 判据 | 结论 | 证据 |
|---|---|---|
| 只允许显式 opt-in | ✅ stage 组件默认不装配；pipeline 显式 Enabled | `TestFactsPipelineDisabledIsSafeNoOp`、`TestDefaultProcessDoesNotAssembleWALPipeline`、源码守卫 `TestDefaultFactsChainAssemblyMatchesGrayReview` |
| 默认（now facts-only）边界守卫继续通过 | ✅ | `TestDefaultProcessUsesFactsOnlyDNSConsumer`、`TestDefaultMutationPathsRetainExplicitMigrationBoundary`、`TestLeaseWritesAreConfinedToTheDocumentedOwners` |
| 失败事实可重放且不丢失 | ✅ | outbox durable + watermark replay；`TestFactsPipelineRestartsAfterConsumerFailure`、`TestRecoverSQLiteProjectionReplaysAfterPersistedWatermark` |
| 停止超时不误报回滚完成 | ✅ | `FactsPipeline.Stop` 超时返回错误；`TestFactsPipelineConcurrentStopIsSafe` |

## 6. 结论

灰度装配的形态、开关、恢复闭环与 readiness 均已在代码中实现并有测试；本文件把「装配图 + 回滚语义 + 判据→测试」固化，并由 `cmd/goddi` 源码守卫 `TestDefaultFactsChainAssemblyMatchesGrayReview` 保证文档描述的装配标记与 `main.go` 实际一致。**默认生产接入（durable gate 接管 / WAL 热路径）仍未做**，属计划 D2 门禁。
