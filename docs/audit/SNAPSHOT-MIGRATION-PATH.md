# 旧四/五/六表快照到八表候选的迁移路径（候选定义，未冻结）

> 状态：**候选定义，待独立评审冻结**。本文只把「迁移路径」从「尚未定义」推进到「有可评审的定义」，**不实现任何转换代码、不发明旧格式、不提升任何门禁、不授予角色/epoch/fencing/写权限、不宣称向后兼容或生产可用**。对应 `contracts/version-compatibility.md` 的原缺口条目。

## 1. 当前已经强制的形状（本工作树实测）

| 层 | 强制内容 | 代码 / 测试 |
|---|---|---|
| 冻结表集与顺序 | 恰好 8 张表，顺序固定：`ipam_tenants`、`ipam_spaces`、`ipam_prefixes`、`ipam_pools`、`ipam_occupancies`、`ipam_resource_ids`、`static_occupancy_audit`、`idempotency_receipt` | `internal/enterprise/snapshotipam/validate.go` 的 `order` 与 `definitions`；`format_pin_test.go` |
| manifest 表集 | `len(schema) == len(m.Tables)`，逐项名称相等且顺序一致，白名单只从本地 schema 派生 | `internal/enterprise/snapshotpipeline/pipeline.go` 的 `New`；`TestNewRejectsManifestTableSetAndOrder` 钉住 missing / duplicate / unknown / reordered |
| 传输版本 | manifest `schema` 必须等于 `limits.Schema`，编码必须是 `EncodingNDJSONV1` | `snapshotverify.New` |
| 行与主键 | 每表精确行数、严格 SQLite BINARY 主键顺序、列形状逐字段校验 | `snapshotrows.New` / `snapshotipam.Validate`；`snapshotipamcheck`、`snapshotauditcheck` |

结论：**表集不完整（4/5/6 张）的 manifest 在 pipeline 入口即被 `ErrInvalid` 拒绝**，不是被当作「缺表 = 空表」接受。`docs/audit/IMPLEMENTATION-STATUS.md` §1v 补了针对 4/5/6 表集的专项回归。

## 2. 为什么不能就地转换

- 旧的四/五/六表格式是**彼此隔离的实验候选**（`snapshotipam/FORMAT.md`），各自有不同的表子集、行形状与主键顺序；它们**都标作 `schema=1`**，因此版本号本身无法区分是哪一种。`contracts/version-compatibility.md` 第 4 条规则明确禁止「数值相同即可读」的推断。
- 缺的表不是「行数 0」：它们改变账本覆盖、主键顺序与跨表不变量。补零行会**凭空发明历史**，而历史完整性正是快照最不能自证的东西（`enterprise-static-audit-v1.md`、`enterprise-receipt-v1.md`）。
- 旧格式的行形状已不在代码中，任何「按旧形状解析」的实现都等于**重新发明一份未冻结的合同**，这正是提案禁止的。

## 3. 定义（候选，逐步、全部 fail-closed）

1. **检测而非猜测**：manifest 的表集 ≠ 冻结 8 表有序集合时，以**明确的、可区分的**拒绝/处置回应，绝不静默补表、绝不降级到默认集合。当前 `snapshotpipeline.New` 已返回 `ErrInvalid`；正式协议还应在握手层给出独立处置码，而不是只让解析失败。
2. **显式版本协商**：对端必须声明一个**独立于 `schema=1` 的快照表集/格式版本标识**。`internal/enterprise/compat` 的候选矩阵需先经评审增加该轴（或等价的 format id），`Negotiate` 取交集且空集即拒绝；旧格式不得因「也是 1」而被接受。
3. **从可信源重新抽取**：唯一受支持的迁移是**由权威源生成一份全新的八表快照**，包含全部 8 张表（含零行表）、按冻结顺序、自带完整性与可信锚点证据。**不支持把一份旧快照逐行转换成八表** —— 那要求旧形状、历史与来源全部已被独立验证，而本仓不具备。
4. **独立只读校验**：依次经过 `snapshotverify`（传输/预算）、`snapshotrows` + `snapshotipam`（行形状与主键顺序）、`snapshotauditcheck` / `snapshotipamcheck`（审计、收据、IPAM 不变量）、`snapshotstage` 的一次性夹具演练。任一失败即丢弃，不得部分接受。
5. **不安装**：`snapshotstage.Import/Install`、`generationpointer.Import/Install/Activate/Switch` 恒 `ErrNoAuthority`。生成或校验快照**不**授予角色、epoch、fencing、可信锚点或写权限；`prodgates` 回归保持 10 个哨兵关闭。
6. **迁移事件本身要可审计**：若将来允许任何形式的迁移，迁移动作必须作为独立事件进入审计，并携带源/目标格式标识与操作者授权证据；不得由数据库错误或解析失败**推断**出「需要迁移」。

## 4. 冻结前必须由独立评审回答的问题

1. 旧四/五/六表格式的**精确行形状与主键顺序**究竟是什么（当前代码无定义）—— 在冻结前不得写解析器。
2. 是否新增独立版本轴；它与事件 `schema_version`、快照 `encoding`、`SnapshotManifest.schema_version`、RPC `goddi.replica.v1`、本机 DDL 版本如何互不替代（六轴规则）。
3. 历史一致性与来源证明：如何证明重新抽取的快照覆盖原快照的全部业务事实，且不引入或丢失账本行。
4. 迁移事件的审计、授权与对账要求；跨主机分区/旧主回归时的行为。
5. 与 DHCP 租约/发件箱/完整审计 schema 冻结（`contracts/enterprise-dhcp-lease.proposal.md`）的先后顺序。

## 5. 明确不宣称的内容

- **没有**实现 `internal/enterprise/snapshotmigrate`（或任何等价代码）。
- **没有**把任何行形状宣称为「旧格式参考 schema」。
- **没有**把候选兼容矩阵或本路径冻结为生产协议。
- **没有**把仓内单机/离线只读证据升级为 B8.5 外部验收证据。
- 本文不替代 `docs/B8.5-外部验收Runbook.md` §10 的原始证据归档。
