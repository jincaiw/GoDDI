# 加固参考结构的同键 UPSERT 写路径兼容（候选定义，未冻结）

> 状态：**候选定义，待独立评审冻结**。本文只把「同键 UPSERT 语义变更」从「尚未集成/未定义」推进到「行为已实测钉住 + 写路径要求有可评审定义」，**不实现任何生产写路径、不提升 `eventlogguard.Migrate` 或任何受保护写门禁、不授予角色/epoch/fencing/写权限、不宣称生产升级已完成**。对应 `contracts/version-compatibility.md` 的缺口条目与 `docs/audit/P0-G1.md` 的「新增逻辑键守卫会拒绝同键 UPSERT」。

## 1. 已实测的行为（本工作树）

在 `ipamstore.Migrate` 安装的加固（v2）参考库上，逐项实测（`internal/enterprise/ipamstore/upsert_semantics_test.go`）：

| 写法 | 结果 | 触发原因 |
|---|---|---|
| `INSERT OR REPLACE ... VALUES('old',...)` 同逻辑键 | 拒绝 `IPAM_ID_ALREADY_USED` | 逻辑键守卫：`ipam_resource_ids` 中该 ID 已存在，禁止复用/替换 |
| `INSERT ... ON CONFLICT(id) DO UPDATE ...` 同逻辑键 | 拒绝 `IPAM_ID_ALREADY_USED` | 同上；SQLite 的 `BEFORE INSERT` 触发器先于冲突处理求值 |
| `UPDATE ipam_tenants SET slug=... WHERE id='old'` | 拒绝 `IPAM_REVISION_INVALID` | 修订守卫：更新必须推进 `revision` |
| `UPDATE ... SET slug=..., revision=revision+1 WHERE id='old'` | **成功** | 受支持的写形式 |
| `INSERT OR REPLACE ... VALUES('fresh',...)` 新键 | **成功** | 无冲突，守卫只拒绝「替换」 |

原行在两种被拒的 UPSERT 后**保持不变**（测试逐个断言）。另由既有测试钉住：`INSERT OR REPLACE ... SELECT rowid, ...` 被 `IPAM_ROWID_CONFLICT` 拒绝、`UPDATE ... SET rowid=...` 被 `IPAM_ROWID_IMMUTABLE` 拒绝、`OR REPLACE` 不得驱逐他人占用（`IPAM_ADDRESS_OCCUPIED`）。

设计意图：**替换一行等于替换历史**。加固结构用逻辑键 + rowid + revision 三重守卫把「静默改写」变成显式失败，这正是它相对旧版（v1）的语义变更，不是缺陷。

## 2. 版本差异必须显式协商，不能推断

- v1（legacy）参考结构**没有**这些守卫；v2（hardened）有。`eventlogguard.InspectTx` 可在同一只读事务里区分 `LegacyReference` / `HardenedReference`（round 122 已集成到 `replicaschema.Verify`）。
- 两条结构都标 `schema=1`，但**物理 DDL 版本与事件/快照/RPC 版本互不替代**（`contracts/version-compatibility.md` 六轴规则）。写路径**不得**只因版本号相同就假定某一种守卫存在或不存在。
- 因此任何集成都必须先做**只读结构分类 + 显式版本协商**，再选择写形式；`compat.Negotiate` 候选矩阵需经评审扩展相应轴，空集即拒绝，绝不降级到默认。

## 3. 写路径要求（候选，全部 fail-closed）

1. **禁止 UPSERT 作为更新手段**：生产写路径不得使用 `INSERT OR REPLACE` / `ON CONFLICT DO UPDATE` 表达「更新同一逻辑键」。原因：在 v2 会失败，在 v1 会静默替换历史 —— 同一段代码在两版上语义相反。
2. **显式区分插入与更新**：调用方必须在**同一事务内**先判定该 ID 是否已存在，再二选一：
   - 新 ID → `INSERT`（正常列，不带显式 `rowid`，不复用已退役 ID）；
   - 已存在 ID → `UPDATE ... SET ..., revision = revision + 1`，且不得改动 `rowid` / 主键。
   判定与写入必须在同一事务、同一连接、同一读视图内完成，避免 TOCTOU。
3. **同事务事实与审计**：任何变更必须与事件/outbox/审计/收据写入处于同一事务；`dhcptx`、`idempotency`、`auditwire` 等候选的现有不变式继续适用。不得用「先删后插」绕过守卫。
4. **失败即丢弃**：任一守卫拒绝、修订冲突、跨作用域引用或地址占用错误，都必须整体回滚并保留失败证据，不得部分提交、不得把错误推断成「需要迁移」或「需要快照」。
5. **版本感知的装配**：默认进程仍使用现有 SQLite 热路径（v0.9.0 四条主线的边界未动）；加固参考结构只在隔离一次性夹具中出现。把 v2 接入默认写路径前，必须完成下面的评审项。
6. **不安装、不授权**：`eventlogguard.Migrate`、`snapshotstage.Import/Install`、`generationpointer.*`、`replicaapply.*`、`dhcptx.Begin/Commit` 仍恒 `ErrNoAuthority`；`prodgates` 的 10 个哨兵保持关闭。本文不产生任何写权限。

## 4. 冻结前必须由独立评审回答的问题

1. v1 写路径中现存的所有 UPSERT / REPLACE 调用点清单与逐项替换方案（本文**未**扫描现存生产写路径，也未改任何一行）。
2. 逻辑键 ID 的生命周期：「释放后新资源 ID 不同」如何在不使用 REPLACE 的前提下保证；隔离表与 `ipam_resource_ids` 的单事务删除守卫。
3. revision 与可信时间/幂等收据/事件 seq 的一致性：谁分配 revision、跨主机分区与旧主回归时如何对账。
4. 版本协商轴的正式取值集合、升级顺序（先接收方兼容两版、再发送方发新格式）与回滚预案。
5. 真实遗留库升级、历史数据对账、物理断电/未知提交证据（B8.5），以及 ACK 语义迁移评审（G1–G5）。

## 5. 明确不宣称的内容

- **没有**把 v2 加固结构接入任何生产或默认写路径。
- **没有**修改 `001_ipam.sql` / `002_rowid_guards.sql` 或任何守卫定义。
- **没有**扫描或改造现存生产写路径中的 UPSERT 调用（那需要上面第 4 节的评审结论）。
- **没有**冻结兼容矩阵、没有宣称升级完成、没有把仓内离线夹具证据升级为外部验收。
- 本文不替代 `docs/B8.5-外部验收Runbook.md` §10 的原始证据归档。
