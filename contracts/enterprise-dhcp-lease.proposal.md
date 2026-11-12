# Enterprise DHCP 租约快照参考 schema（**已冻结**）

状态：**FROZEN（round 256–258 冻结；授权：用户指示「从合理角度评估决策直接实施」）**。本文件曾是提案（PROPOSAL，待评审冻结）；6 个开放决策项已按下列决定冻结，实现据此进行（`internal/enterprise/dhcplease`）。本文件不是上游 DDL、不是已执行迁移、不是生产验收。

## 1. 两类现有租约模型

- 旧 upstream `dhcp_leases`（`migrations/003` + `007` + `017`）：`id TEXT PK`、`scope_id TEXT`、`ip_address TEXT`、`mac_address TEXT`、`hostname`、`client_id TEXT`、`lease_start/lease_end DATETIME`、`status TEXT`、`last_seen DATETIME`；部分唯一索引 `(scope_id, ip_address)` 覆盖 `active`/`offered`，`conflict` 隔离不参与分配。身份用 **MAC**。
- `dhcptx` 合成夹具（`internal/enterprise/dhcptx/tx_test.go`）：`dhcp_lease(tenant, space, lease_id, client_key BLOB, address TEXT, kind, offer_xid INTEGER, issued_ms INTEGER, expires_ms INTEGER, revision)`，`PK(tenant,space,address)`、`UNIQUE(tenant,space,client_key)`、`UNIQUE(tenant,space,lease_id)`；`dhcp_outbox(cluster_id, epoch, seq, hash, payload)`，`PK(cluster_id,epoch,seq)`。结构化、Option61 BLOB、毫秒整数、revision。

**不能直接复用旧模型**：TEXT 主键 / DATETIME 与候选快照规范编码（INTEGER 十进制字符串、BLOB canonical base64url、按真实主键排序）不一致；MAC 身份与 `docs/02` 要求的规范 Option 61 身份不一致；缺 tenant/space 隔离与 revision；状态词表（active/offered/conflict）与候选状态机不一致。

## 2. 建议冻结候选（需决策项已标注）

建议列（在 dhcptx 合成基础上补齐 family 与地址表示决策）：

- `tenant` TEXT NOT NULL
- `space` TEXT NOT NULL
- `address` — **决策项 A**：`BLOB(16)`（与 IPAM 前缀/占用同族字节编码，便于跨表比对）或沿用 dhcptx 的 `TEXT`（点分/冒号文本）。
- `family` INTEGER CHECK IN (4,6)（若 address 用 16 字节 BLOB 则必需）
- `lease_id` TEXT NOT NULL（稳定 ID，不复用）
- `client_key` BLOB NOT NULL（规范 Option 61 原始字节，长度有界）
- `kind` TEXT CHECK IN ('OFFER','BOUND','RELEASED','EXPIRED','DECLINED')（**决策项 B**：是否与旧 active/offered/conflict 映射）
- `offer_xid` INTEGER CHECK >= 0
- `issued_ms` INTEGER CHECK >= 0
- `expires_ms` INTEGER（HOLD/OFFER 需 > issued_ms；永久保留的表示需决策——不得用导入机时钟推断）
- `revision` INTEGER CHECK > 0
- `PRIMARY KEY (tenant, space, address)`
- `UNIQUE (tenant, space, lease_id)`
- `UNIQUE (tenant, space, client_key)`（同一客户端同作用域唯一活跃租约；是否按 kind 分部分唯一需决策）
- `CHECK (expires_ms >= issued_ms)`

## 3. 快照行编码候选

- INTEGER 用规范非负十进制字符串；BLOB（client_key，及若 decision A 选 16 字节则 address）用 canonical 无填充 base64url；TEXT 为有效 UTF-8 无 NUL、有界长度。
- 行顺序按真实 SQL 主键 `(tenant, space, address)` 的 SQLite BINARY 顺序（address 若为 BLOB 则按原始 16 字节比较，不按 base64 文本）。
- **恢复不得按导入机当前时钟重新过期**：`expires_ms` 原样保留；是否清理由独立保留策略与可信时间决定，不由快照加载动作改写。
- 行上限与 chunk 上限沿用 `snapshotrows`/`snapshotverify` 的既有预算；租约行较大时应单独评估 1 MiB/2 MiB 行界。

## 4. 跨表不变量（与 IPAM/池/hold）

- 每条活跃 `OFFER`/`BOUND` 租约地址必须落在其 `subnet`/DHCP 池闭区间内且同族。
- 活动租约阻止删除其所属池（docs/02 IPAM-04）；释放后新资源 ID 不同（IPAM-06）。
- `DECLINED`/`conflict` 隔离表与活动租约的地址唯一性需双向核对（旧 `017` 用部分唯一索引；候选需明确隔离表）。
- `dhcp_outbox` 与 `replica_event` 的 `(cluster_id,epoch,seq,hash,payload)` 必须逐字节一致（发件箱与事件同事务）。
- DHCP 固定保留（RESERVATION）与 IPAM 占用一一对应，不重复建第二占用。

## 5. 不能由候选自证的点

租约“当前仍有效/未被撤销”、地址真实可分配、源库历史完整性、可信时钟、epoch/holder/role/fencing、发件箱与事件链的真实同源，都不能由行形状或本候选自证；需可信源快照 + 独立角色/epoch/锚点证据 + 同事务一致性。

## 6. 开放问题 → 已冻结的决定（round 256–258）

1. **地址表示**：`address BLOB(16)`（16 字节网络序）+ `family INTEGER CHECK IN (4,6)`。理由：与 IPAM 前缀/占用同族字节编码，跨表比对（dhcpholds 双向守卫）无需文本转换。
2. **状态枚举映射**：冻结为 `('OFFER','BOUND','RELEASED','EXPIRED','DECLINED')`；旧词表映射：`active→BOUND`、`offered→OFFER`、`conflict→DECLINED`（隔离语义入独立表）；`RELEASED`/`EXPIRED` 为终态，保留审计轨迹。
3. **永久租约与不重算**：`expires_ms` 允许 NULL（=永久保留，RESERVATION 绑定）；CHECK 为 `(expires_ms IS NULL OR expires_ms >= issued_ms)`。导入**原样保留** `expires_ms`，是否清理由独立保留策略与可信时间决定，不由快照加载改写。
4. **部分唯一索引与隔离守卫**：活跃唯一改为部分唯一索引 `CREATE UNIQUE INDEX ... ON dhcp_lease(tenant,space,client_key) WHERE kind IN ('OFFER','BOUND')`；隔离表 `dhcp_declined(tenant,space,address BLOB(16),reason,declined_ms,revision)` 与活跃租约地址**双向互斥**（应用层同事务双查 + 快照校验断言）。
5. **并入八表快照**：租约/隔离/发件箱/收据表作为**追加表集并入快照**，快照 schema 升为 **2**（表集变化必须升版本）；理由：租约地址∈池、RESERVATION↔占用一一对应等跨表不变量需在同一校验事务内与 IPAM 表共同核对，独立版本化块会割裂这些核对。
6. **总体版本号**：快照 schema=2（八表候选=1）；行编码沿用既有规则（INTEGER 规范十进制、BLOB canonical base64url、UTF-8 有界），行序按主键 BINARY 序（address BLOB 按原始 16 字节比较）。发件箱与事件逐字节一致不变量保留。

原「冻结前不实现 dhcplease 代码」的禁令随冻结解除：实现见 `internal/enterprise/dhcplease`（round 256–258 起）。
