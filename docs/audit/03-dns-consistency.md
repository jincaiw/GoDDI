# 03 · DNS 数据一致性与双节点同步审计

> 基线 commit `8e769d7`。证据：`internal/configver/*`、`internal/dataplane/sync.go`、`internal/dns/*`、`cmd/goddi/main.go`。本轮 `go test ./internal/dns/... ./internal/configver/` 全绿。

## 1. 配置版本一致性（Zone/Record/View/ACL/Forwarder/策略）
- **单写者 + 不可变 Revision**：`internal/configver/service.go:111` `Publish`，`config_revisions`/`config_release_outbox`（`migrations/019_config_revisions.sql:73-100`），含 `revision`、`resource_applied`、`applied_generation`、`last_error`。**PASS（单节点）**。
- **数据面复制**：`internal/dataplane/sync.go:101-124` `dnsTables`——`dns_zones`(`replaceWhole`)、`dns_records`(`replaceControlAuthored`)、`dns_zone_transfer`(`replaceWhole`)、`dns_dnssec_keys`(`replaceWhole`)。DNS 引擎从本地 `dnsdata` 应答（`main.go` `dnsRunner`/`dnsDataDB`）。**PASS（读隔离）**。
- **View/ACL/Forwarder**：ACL 存 `dns_zones.acl`（`internal/dns/zone/zone.go` `ZoneACL:52-61`，`ACLAllows`）；Forwarder/条件转发为本地配置（`internal/dns/forwarder`）。版本随 zone revision 复制。**PARTIAL**（View 概念以 ACL/zone 表达，无独立 View revision）。

## 2. Prepare / Commit Decision / Apply 机制
- **单节点**：`configver.Publish` 写 revision + `config_release_outbox`（同事务），数据面 runner 消费 outbox 应用到本地副本并记录 `applied_generation`（`internal/dataplane/*`）。**PASS（单节点）**。
- **多节点协调（Prepare→CommitDecision→Apply，全集群）**：**NOT_IMPLEMENTED**。`docs/adr/0005-multi-node-config-publish-coordination.md` 状态"设计完成，生产多节点协调未实现"；`internal/cluster/cluster.go:14` `ErrNotImplemented`。**FAIL（§4.3/4.4/4.5 目标未达成）**。

## 3. 双节点发布中断 → 两个有效版本？
- 因为是"控制库单写 + 每节点各自复制应用"（非两阶段），在复制进行中/中断时，**节点 A 与节点 B 可能短暂持有不同 revision**（A 已应用，B 未）。系统**没有"集群全部应用"的裁决点**，也**没有把某节点成功当作集群成功的显式上报**（管理 API 无集群状态字段）。
- **判断：PARTIAL/风险**——不会产生"两个都被宣布为已提交"的假象（无集群提交概念），但**无法证明两节点一致**；需引入协调协议（ADR-0005）或至少暴露每节点 `applied_revision` 并定义"集群一致"判据。

## 4. 三种状态区分（已持久化 / 节点已应用 / 集群全部应用）
- 已持久化：`config_revisions`（控制库）。
- 节点已应用：`config_release_outbox.applied_generation` / `resource_applied`（**单节点**）。
- **集群全部应用：NOT_IMPLEMENTED**（无集群视图）。
- **结论：FAIL（第三种状态缺失）**——必须明确并实现，否则运维会把单节点成功误当集群成功。

## 5. LKG（管理面宕机时 DNS 仍服务）
- DNS 数据面从本地 `dnsdata` 副本应答，控制面宕机不影响已复制数据 → **PASS（读可用）**。`configver/outbox.go:27` 注释"last applied revision stays LKG"。
- 缺口：本地副本是"最后一次复制成功的状态"，**没有显式 LKG 版本戳/回滚到指定 LKG 的机制**（PARTIAL）。复制若中途 `replaceWhole` 中断，需确认原子性（数据面按事务替换，见 06 的 SQLite 事务；跨表一致性 NOT_VERIFIED）。

## 6. AXFR / IXFR / NOTIFY / SOA Serial
- 实现：`internal/dns/transfer/transfer.go`（`HandleAXFR`/`HandleIXFR`，IXFR journal）、`notify.go`（NOTIFY）、`secondary.go`（`SecondarySync`）。SOA serial 由 zone 写路径 bump（`migrations/030_dns_zone_transfer_bump_revision.sql` 等）。
- **本轮修复**：AXFR 请求/响应的 TSIG 签名此前缺失，方案 A 实际不可用；已修复（commit `df23b05`：`internal/dns/transfer/secondary.go` `SetTsig`；`internal/dns/server/handler.go` `serveZoneTransfer` 响应签名 + timers-only）。
- 验证：`go test ./internal/dns/transfer ./internal/dns/server` 绿；loopback 演练 `DRILL: OK`；两容器 + dig 演练 `PASS=5 FAIL=0`（`scripts/dns_primary_secondary_container_drill.sh`）。**PASS（有运行证据）**。
- 授权链路：`dns_zone_transfer` 管理 API 本轮补齐（`internal/api/handler/dns_zone_transfer.go`）；此前无应用路径写该表（**历史 FAIL，已修**）。TSIG 新 key 需重启加载（PARTIAL，见 08）。

## 7. DNS 缓存 / 查询日志是否被强制同步
- 缓存 `internal/dns/cache`、查询日志 `internal/dns/querylog.go` 均为**节点本地**，未纳入 `dnsTables` 复制。**PASS（未被强制同步，符合预期）**。

## 8. 结论
| 项 | 状态 |
|---|---|
| 单节点配置 revision/outbox/应用 | PASS |
| DNS 读路径本地副本（控制面宕机可服务） | PASS |
| AXFR/IXFR/NOTIFY/TSIG（本轮修复+演练） | PASS |
| 多节点 Prepare/Commit/Apply 协调 | NOT_IMPLEMENTED |
| "集群全部应用"状态 | NOT_IMPLEMENTED / FAIL |
| 双节点版本一致性证明 | NOT_VERIFIED / 风险 |
| DNS 缓存/查询日志不强制同步 | PASS |
