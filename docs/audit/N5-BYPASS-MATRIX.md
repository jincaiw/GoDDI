# N5 默认旁路与 durable ACK 调用链矩阵

> 目的：把每个**改动租约状态**或**发送 DHCP ACK**的入口，逐一钉住它的 owner、事务边界、facts/DNS 覆盖、ACK durable 边界与失败语义。任何绕过权威写入者的路径都必须显式列出并给出理由；没有就不允许存在。默认 DHCP 仍为 SQLite（`*lease.Manager` + `FactsMutationWriter` seam），WAL 仍 opt-in，cluster 保持 501。
>
> 本文件由 `internal/dhcp/server/lease_write_boundary_test.go` 静态守卫，改动下列文件或新增 `dhcp_leases` 直写会使其失败。

## 1. DHCP 报文路径（权威写入者）

| 入口 | 调用链 | Owner | 事务边界 | Facts/DNS | ACK durable 边界 | 失败语义 |
|---|---|---|---|---|---|---|
| DISCOVER → OFFER | `handler.handleDiscover` → `s.reserveAddress` → `s.leaseMgr.ReserveAddress` (`lease.Manager.createLeaseWithStatus`, status=`offered`, generation=0) | `lease.Manager` | 单条 `INSERT dhcp_leases` | **无**（offer 不是 binding，不产生 facts/DNS） | 无 ACK（DISCOVER 回 OFFER） | 冲突/满池→换候选重试；不写 facts |
| REQUEST → 新绑定 | `handler.handleRequest` → `factsCfg.Caller.BindLease`（`lease.FactsMutationWriter.BindLease`） | `FactsMutationWriter` | 单事务：lease 写 + sequence 分配 + facts outbox 入队 | `MutationBind` → ipam/dns facts 链 | `RequireDurable(s.durableGate, bound)` 在回 ACK 前（当前 gate=nil ⇒ SQLite 事务提交即持久边界） | 任一步失败整体回滚（lease+sequence+outbox），不回 ACK |
| REQUEST → 激活 offer | `handler.handleRequest` → `factsCfg.Caller.ActivateLease` | `FactsMutationWriter` | 同上 | `MutationActivate` | `RequireDurable` | 回滚、不 ACK |
| REQUEST → 续租 | `handler.handleRequest` → `factsCfg.Caller.RenewLease` | `FactsMutationWriter` | 同上 | `MutationRenew` | `RequireDurable` | 回滚、不 ACK |
| RELEASE | `handler.handleRelease` → `factsCfg.Caller.ReleaseLease` | `FactsMutationWriter` | 同上 | `MutationRelease` | 无 ACK（RELEASE 不回复） | 回滚 |
| DECLINE | `handler.handleDecline` → `QuarantineIP` + `factsCfg.Caller.DeclineLease` | `FactsMutationWriter` | 同上 | `MutationDecline` | 无 ACK | 回滚 |
| 过期清扫 | `server.ExpireLeasesFromDataPlane` → `ExpireFactsBatch`（`lease/facts_expiry_batch.go`） | `FactsMutationWriter` | 单事务批量 | `MutationExpire`/每条 | 不在 ACK 路径（定时器） | 事务回滚；失败保留租约待下轮 |

## 2. 管理面路径

| 入口 | 调用链 | Owner | 事务边界 | Facts/DNS | 说明 |
|---|---|---|---|---|---|
| 管理面删除单条租约 | `api/handler/dhcp_lease.go` → `DHCPServices.MutationOwner.ReleaseLeaseFromManagement(id)` → `server.ReleaseLease` → `Caller.ReleaseLease` | `MutationOwner`（= `*server.Server`） | 同 RELEASE 的 facts 事务 | `MutationRelease` | 管理面**不得**直接调 `LeaseMgr.ReleaseLease`，必须经 owner seam（守卫禁止） |
| 管理面删除作用域 | `api/handler/dhcp_scope.go` → `ScopeMgr.DeleteScope` → `scope/scope.go` 级联 `DELETE FROM dhcp_leases WHERE scope_id=?` | `scope.Manager` | 单事务（先 `SELECT COUNT` 存活绑定，>0 直接拒绝） | **无（受控旁路）** | 仅删除**非活跃历史行**（active 存活绑定存在时拒绝删除）；删除历史不产生新 facts，理由见 §3 |

## 3. 复制 / 投影路径（非本地权威，不产生本地 facts）

| 文件 | 作用 | 为什么可以直写 `dhcp_leases` |
|---|---|---|
| `internal/dataplane/sync.go` / `transfer.go` | 租约库与控制库之间的投影上下行复制（含 stale-holder supersede） | 复数/投影副本，不是权威写入；上行由 dirty marker 驱动 |
| `internal/dhcp/ha/mirror.go` | standby 应用 primary 的快照（`DELETE`/`INSERT dhcp_leases`） | 镜像副本，只从 primary 快照重建；不本地决策、不产生本地 facts |
| `internal/dhcp/lease/wal_projection.go` | opt-in WAL 投影 applier 把 WAL 事件落到 SQLite | WAL opt-in 路径，默认进程不装配（守卫 `TestDefaultProcessDoesNotAssembleWALPipeline`） |
| `internal/dhcp/scope/scope.go` | 管理面删除作用域的级联清理 | **受控旁路**：仅删非活跃历史行；唯一非投影文件，理由见 §4 |

## 4. 只读路径（不写租约）

- `internal/metrics/*`：作用域利用率 provider 只读 `dhcp_leases`。
- `internal/backup/*`：整库文件级备份/恢复，不按行写租约；恢复是库级动作，另有闸门。
- `api/handler/dhcp_scope.go` 的 `LeaseMgr` 是 `dhcpserver.LeaseReader`（只读接口），不是 `*lease.Manager`（守卫强制）。

## 5. 已知受控旁路与缺口结论

- **作用域删除**（`scope.DeleteScope`）：先在同一事务内 `SELECT COUNT(*) ... status='active' AND 未到期`，>0 则拒绝；仅删除非活跃历史（expired/released/offered/conflict）行。删除历史不会撤销任何仍然有效的 DNS 记录（active 绑定被拒绝删除），故不发布 facts 是正确的。owner=`scope.Manager`，事务=单 tx，失败=整体回滚。
- **无其它未记录旁路**：`TestLeaseWritesAreConfinedToTheDocumentedOwners` 强制 `dhcp_leases` 直写只出现在上表列出的文件；新增文件若直写会失败，必须在本矩阵登记并说明 owner/事务/facts。
- **durable ACK 缺口**：默认 `durableGate=nil`，ACK 的持久性由 SQLite 事务提交承担；WAL durable gate 尚未接管默认装配（计划 D2 门禁，需单独确认）。这是**已登记的门禁**，不是遗漏。

## 6. 允许直写 `dhcp_leases` 的文件清单（守卫白名单）

- `internal/dhcp/lease/lease.go` — 默认 SQLite 权威写入者
- `internal/dhcp/lease/facts_mutation.go` — facts mutation writer（权威 seam）
- `internal/dhcp/lease/facts_expiry_batch.go` — facts 过期批量清扫
- `internal/dhcp/lease/wal_projection.go` — opt-in WAL 投影
- `internal/dhcp/ha/mirror.go` — standby 快照应用
- `internal/dhcp/scope/scope.go` — 受控作用域删除级联（仅非活跃历史）
- `internal/dataplane/sync.go` — 控制库↔租约库投影复制
- `internal/dataplane/transfer.go` — 控制库↔租约库投影复制

守卫 `TestLeaseWritesAreConfinedToTheDocumentedOwners` 会比对实际直写文件集合与该白名单；越界即失败。
