# 版本与兼容矩阵（P0-G1 候选）

本文件冻结当前工作树中**相互独立**的几个“版本”轴，说明它们为什么不能互相替代。任何一处的数值相等都不代表另一处兼容；这些数值不是生产授权、迁移许可或就绪信号。

## 彼此独立的版本轴

| 轴 | 别名/常量 | 当前值 | 作用域 | 谁定义 | 能否用于生产协商 |
|---|---|---|---|---|---|
| 事件/线格式 schema | `eventlog.SchemaVersionV1` | `1` | 单个 `Event.Schema`、`replica_head.schema_version`、`PullRequest.readable_schema_versions` | 本仓库事件合同 | 仅候选，需与对端协商矩阵冻结 |
| 快照编码 | `snapshotverify.EncodingNDJSONV1` | `1` | `SnapshotManifest.encoding` | 快照合同 | 仅候选 |
| 快照行/表 schema | `SnapshotManifest.schema_version` / `Limits.Schema` | `1` | 八表 NDJSON 候选行格式 | `snapshotipam/FORMAT.md` | 仅候选；四/五/六/八表实验式互不兼容 |
| RPC 消息/服务版本 | proto `package goddi.replica.v1` | `v1` | gRPC 服务与消息 | `contracts/replication.proto` | 是线协议版本，非本机 DDL 版本 |
| 本机物理 DDL 结构 | `eventlogguard.Version` | 枚举 `Unknown`/`LegacyReference`/`HardenedReference` | 单库 `main` schema 对象与 marker | `eventlogguard` | 只读识别，非角色或 wire 版本 |
| IPAM 参考迁移集 | `ipamstore.MigrationPaths()` | `001` + `002` | 隔离参考 IPAM DDL | `ipamstore` | 非上游升级证据 |

## 关键不可推断规则

1. **同号不同义**：上表中多个轴都取 `1`，但它们控制的对象完全不同。事件 `schema_version` 决定事件类型解码范围；快照 `schema` 决定导出/导入的行与表形状；`goddi.replica.v1` 是 RPC 服务/消息版本；`eventlogguard.Version` 是本机 SQLite 物理 DDL 结构。任一处为 `1` 都不能推出另一处可读、可恢复或可写。
2. **快照候选互不兼容**：四表、五表、六表、八表实验候选即使都写作 `schema=1` 也不能互相读取。正式协议必须先冻结行/表形状，再做显式版本协商与迁移；不得静默降级。
3. **物理 DDL 版本与事件 schema 分离**：`eventlogguard.Inspect`/`InspectTx` 只报告本机参考结构（`LegacyReference` 或 `HardenedReference`），不代表事件 `schema_version`。加固参考结构比旧参考多 28 条守卫，旧版校验器会拒绝加固库，反之亦然。
4. **升级顺序**：docs/04 要求先让接收方支持新旧事件 `schema_version`，再让发送方发新格式；快照导入需 `readable_schema_versions` 明确交集。任何“数值相同即可读”的推断都被禁止。

## 候选生产兼容矩阵（round 123，**尚未冻结**）

`internal/enterprise/compat` 提供一个**候选**矩阵与协商原语，用代码把上表的六条轴钉住并说明各自控制什么；测试断言常量与 `eventlog.SchemaVersionV1` / `snapshotverify.EncodingNDJSONV1` 保持相等，且多条轴同为 `1` 时仍**互不替代**（`Axes()` 的 `Controls` 两两不同）。

| 轴 | 本构建可发出 | 本构建可接受 | 协商规则 |
|---|---|---|---|
| 事件/线格式 schema | `1` | `1` | `Negotiate(offered, accepted)` 取交集；空集 / 0 / 重复 / 未知版本一律拒绝，**不静默降级** |
| 快照编码 | `1` (NDJSON) | `1` | 只接受 NDJSON；不因数值同为 1 而用事件 schema 推断 |
| 快照行/表 schema | `1` (八表) | `1` | 四/五/六/八表实验式互不兼容，即使都写作 1 |
| RPC 消息 | `goddi.replica.v1` | `v1` | 线协议版本，非本机 DDL 版本 |
| 本机物理 DDL | legacy / hardened | legacy / hardened | 只读结构识别，不授予角色或写权限 |
| IPAM 参考迁移 | `001+002` | `001+002` | 隔离参考集，非上游升级证据 |

`ValidateReadableSchemas` 对 pull request 的 `readable_schema_versions` 做同样的 fail-closed 校验。**该矩阵仍是候选**：需独立评审与对端显式协商后才能冻结并用于生产；此包不授予角色、epoch、fencing、可信锚点或写权限，也未接入任何生产 RPC / 快照安装 / 复制读路径。

## 尚未完成

- 生产兼容矩阵（各轴对端可接受的版本集合及其映射）已在上节以**候选**形式给出并附测试，但仍**未冻结**：需独立评审与对端显式协商。
- 旧四/五/六表快照到八表候选的迁移路径**已于 round 145 给出候选定义，但仍未冻结**：见 `docs/audit/SNAPSHOT-MIGRATION-PATH.md`。定义明确「检测而非猜测 → 独立格式版本协商 → 从可信源重新抽取全新八表快照 → 独立只读校验 → 不安装」，**不支持逐行就地转换、不补零表、不静默降级**；`snapshotpipeline` 的 4/5/6 表集拒绝已有专项回归。冻结前需独立评审旧格式精确行形状、新版本轴、历史一致性与迁移事件审计。
- 本机 DDL 加固参考结构的版本迁移与同键 UPSERT 语义变更**仍未集成**（公开 `eventlogguard.Migrate` / 快照安装恒 `ErrNoAuthority`）；但 **round 146 已把语义变更实测钉住并给出候选写路径定义**：`docs/audit/UPSERT-WRITE-PATH-COMPAT.md` + `internal/enterprise/ipamstore/upsert_semantics_test.go`。v2 守卫拒绝同键 `INSERT OR REPLACE` / `ON CONFLICT DO UPDATE`（`IPAM_ID_ALREADY_USED`）与不推进 revision 的 `UPDATE`（`IPAM_REVISION_INVALID`），只支持新键 `INSERT` 与 `UPDATE ... revision=revision+1`。定义要求显式插入/更新二分、同事务事实与审计、禁止降级、版本感知装配；**冻结前需独立评审现存 UPSERT 调用点、ID 生命周期、revision 对账与真实升级证据**。
- **`replicaschema` 对新结构的接受已于 round 122 完成**（离线只读校验层面）：`replicaschema.Verify` 现在通过 `eventlogguard.InspectTx` 分类并接受 legacy / hardened 两种冻结参考结构；`eventlogguard.ReferenceDDLs` / `ReferenceMarkerStatements` 提供统一的 DDL 与 marker 描述。这**只是只读结构识别**，不是版本协商、生产升级或写路径兼容的证据。
- 以上所有条目都不授予角色、fencing 或写权限；受保护写入口与复制/快照安装入口保持默认拒绝。
