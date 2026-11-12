# Enterprise 候选实现评审材料（独立评审输入）

> 状态：**候选 / 原语，不是生产验收**。**一页交接总览见 `docs/STATUS.md`。**`docs/audit/P0-G1.md` 的进度总表所有行均为隔离候选或原语；受保护写、复制、快照安装入口在 `internal/enterprise/prodgates` 回归下恒 `ErrNoAuthority`。本文件是给独立评审人的索引，**不授予角色、epoch、fencing、写权限，也不声称生产就绪**。

## 1. 范围与量化

| 项 | 数量 | 证据 |
|---|---|---|
| `internal/enterprise/` 包 | 53 | 本文件 §3 分组清单 |
| Go 测试与 Fuzz/Property 入口 | 613 | `grep -rh '^func Test\|^func Fuzz' internal/enterprise/ --include=*_test.go \| wc -l` |
| 含 Fuzz / 属性 / 篡改测试的域 | 14 | `dnswire`, `dhcpwire`, `auditwire`, `receiptwire`, `snapshotrows`, `ipamevent`, `hasign`, `hapermit`, `rolelog`, `replicabatch`, `staticbundle`, `httpcontract`, `timewater`, `snapshotstage` |
| 生产写 / 激活 / 网络 / 迁移门 | 10 | §2，`prodgates/gates_test.go` 的 `TestProductionWriteGatesRemainClosed` 固定 |
| 版本轴 | 6 条互不替代 | `contracts/version-compatibility.md` + `internal/enterprise/compat` |
| 可复现验证入口 | 6 | `make verify` / `verify-race` / `verify-browser` / `verify-e2e` / `verify-tidy` / `build`（见 §5） |

## 2. 门禁清单（10 个 `ErrNoAuthority` 哨兵）

| 包 | 被保护的入口 | 门禁理由 |
|---|---|---|
| `snapshotstage` | `Import` / `Install` | 快照安装需可信源与独立角色/epoch/锚点证据 |
| `generationpointer` | `Import` / `Install` / `Activate` / `Switch` | 代次指针切换需安装授权 |
| `eventlogguard` | `Migrate` | 本机 DDL 加固参考结构的生产升级需独立评审与授权 |
| `eventlog` | `Begin` / `RecordApplied` 等受保护写 | 事件日志写需本机有效 role/epoch/generation/fencing |
| `replicaapply` | `Apply` / `Batch` | 复制应用需经认证的权威 |
| `replicaread` | `Read` | 复制读服务需授权 |
| `replicarpc` | `Start` | gRPC 服务启动需授权 |
| `dnstransport` | `Start` | 生产 DNS 服务需授权 |
| `dhcptx` | `Begin` / `Commit` 等写事务 | DHCP 写事务需授权 |
| `haregistry` | `Issue` / `Cancel` / `Complete` | 接管许可（takeover permit）需外部授权 |

`internal/enterprise/prodgates` **本身没有生产逻辑**，只持有 3 组回归测试 `TestProductionWriteGatesRemainClosed`（21 项）、`TestAuthoritySentinelsAreDistinct`、`TestADatabaseAuthorityRowAloneDoesNotGrantWriteAuthority`，保证上述 10 个哨兵互不混淆、恒返回 `ErrNoAuthority`，且完整 `writer_authority` 行也不授予写权限。任何提升门禁的改动都会使该测试失败。

## 3. 分组清单（每组：证明了什么 / 不证明什么）

### 3.1 事件链、读与复制协议候选
`eventlog`(72 tests)、`eventlogguard`(44)、`replicabatch`(7)、`replicaapply`(15)、`replicaread`(9)、`replicarpc`(23)、`replicaguard`、`replicaschema`(8)、`pullbudget`、`pullresponse`(22)、`staticbundle`、`auditwire`、`receiptwire`、`idempotency`、`ipamevent`、`soalineage`、`soaserial`、`prodgates`(3)、`compat`(7)。

- **证明**：事件哈希框架（SHA-256 over `"goddi.replica.v1" || ...`）、链/快照只读一致性、加固/旧版参考结构识别、离线 Pull 控制候选（4 种处置）、protobuf 预算、mTLS 回环夹具（证书/pin/同 SAN 异钥拒绝）、版本轴独立性与 fail-closed 协商。
- **不证明**：权威尾、角色/epoch/fencing 权威、可信时间、真实 TLS 通道装配、撤权后网络排空、生产 Start、旧库升级、跨主机分叉。
- `pullresponse` 的 `SNAPSHOT_REQUIRED` / `DIVERGED` 只是**离线编码器**，要求调用方显式提供 `Earliest` / `Conflict` 事件证据；**禁止**从数据库错误推断分叉（包内 `README.md`）。

### 3.2 快照导出 / 校验 / 暂存候选
`snapshotverify`(5)、`snapshotpipeline`(11)、`snapshotrows`(12)、`snapshotipam`(29)、`snapshotipamcheck`(16)、`snapshotauditcheck`(5)、`snapshotstage`(31)、`dhcplease`(29)、`dnssnapshot`(8)、`dnsanswer`(10)、`dnswire`(7)、`dnstransport`(7)、`dnsanswer/edns`。

- **证明**：八表冻结行/主键顺序、NDJSON 结构、manifest/预算校验、审计与收据行形状、隔离八表暂存夹具（含 rowid/逻辑键守卫）、只读 DNS 应答原型（A/AAAA/NS/SOA、EDNS 预算、简单名 fail-closed）。**round 258–262（提案冻结后，用户授权决策）：`dhcplease` schema=2 租约表集（FROZEN DDL、行校验/暂存/导出/feed 接线、guarded 12 表 staging、导出/校验闭环，§1cb 系列）**。
- **不证明**：快照安装/激活、真实 DNS 监听、委派/通配符/CNAME/传送/递归、快照来源可信、历史完整性。DHCP 租约/hold/offer 运行面见 §3.3 与旧版 DHCP HA。
- `contracts/enterprise-dhcp-lease.proposal.md` 状态 **FROZEN**（round 258，用户授权决策记录 6 项决定，§6）；`dnswire` 是冻结线格式（`len(Type) ≤ 4`，连 `CNAME` 都容不下），扩展需版本协商与评审。

### 3.3 DHCP 数据面原型
`dhcpwire`(7)、`dhcpingress`(9)、`dhcpreply`(8)、`dhcptx`(17)、`dhcpholds`(12)、`dhcpdeadline`(2)、`ipamrequest`、`ipamresponse`、`ipamaddr`(9)、`ipamgap`(4)、`ipamstore`(20)。

- **证明**：有界 DHCPv4 报文解码、精确 scope 映射、离线应答编码、同事务业务占用/幂等/审计/outbox 原型、Option 61 三元绑定候选、IPAM 地址编码/缺口、参考 DDL 002 加固与旧版 rowid 替换风险复现。
- **不证明**：实际 UDP 发包、可信时间、生产集成、默认 DHCP 热路径迁移、统一事实链默认装配、跨库 replay、完整 lag/gap 观测。

### 3.4 角色 / epoch / fencing / 接管原语
`rolelog`(14)、`rolebarrier`(4)、`hapermit`(10)、`hasign`(7)、`haregistry`(8)、`timeguard`(5)、`timewater`(5)、`trustedtime`(8)、`peeridentity`(8)、`generationpointer`(9)、`httpcontract`(4)。

- **证明**：本地角色记录与比较、进程内 drain、接管许可签名、可信时间本地高水位、TLS 对等身份绑定（含 TLS 1.3 会话恢复后重新检查 pin 与时间）。
- **不证明**：真实授权/fencing/撤销、可信时钟来源、跨主机分区、证书不等于角色或可信时间、生产接管。

### 3.5 只读结构校验（round 122 集成）
`replicaschema` 现通过 `eventlogguard.InspectTx` 分类并接受 legacy / hardened 两种冻结参考；`eventlogguard.ReferenceDDLs` / `ReferenceMarkerStatements` 是统一的 DDL/marker 真源。这是**只读识别**，不是版本协商、生产升级或同键 UPSERT 写路径兼容。

## 4. 版本轴（互不替代）

`contracts/version-compatibility.md` 定义 6 条独立轴：事件线格式 schema、快照编码、快照行/表 schema、RPC 消息、本机物理 DDL、IPAM 参考迁移。多处同为 `1` 但控制对象完全不同；**任何「数值相同即可读」的推断都被禁止**。`internal/enterprise/compat` 提供**候选**矩阵与 `Negotiate` / `ValidateReadableSchemas` fail-closed 原语，标注「尚未冻结」，需独立评审与对端显式协商。

## 5. 可复现验证

```bash
make verify          # Go build/test/vet + gofmt + pnpm lint/typecheck/build + git diff --check
make verify-race     # 上述 + go test -race ./...
make verify-browser  # 运行中的隔离服务 + GODDI_ADMIN_PASS；20 项检查
make build           # 清空 dist 后从源码构建 ./dist/goddi 与 web/dist
make verify-tidy     # go mod tidy -diff：零副作用校验模块文件未漂移
make verify-e2e      # 一键跑完 CI 的两个 Playwright 套件（自建 plain + 种子实例）
```

浏览器回归共 **20 项检查**（`test/browser/run.sh` 或 `make verify-browser`）：17 个真实 Chromium 套件 + 3 个静态守卫（`docs` 交接/配置文档/部署产物、`contracts` 前后端契约、`i18n` 文案键）。需 `GODDI_ADMIN_PASS` 与运行中的隔离服务；流程与治理见 `test/browser/README.md` 与 `docs/audit/IMPLEMENTATION-STATUS.md` §1f/§1z/§1ag。分离角色与 DHCP HA 隔离双进程证据见 `docs/audit/IMPLEMENTATION-STATUS.md` §1b / §1c；B8.5 仓内准备进度见 `docs/B8.5-外部验收Runbook.md` §11，现场原始证据模板见同文件 §10。

## 6. 独立评审必须回答的问题（完成前不得关闭门禁）

1. **授权模型**：本机角色 / epoch / generation / fencing 如何独立验证？DB 行、config、TLS 身份都不能单独授予写权限；未知权威尾 ⇒ 无受保护写、无 DHCP 包。
2. **schema 冻结**：DHCP 租约 / 发件箱 / 完整业务审计参考 schema（含 6 个开放决策项）冻结为可评审合同，并完成独立专家评审。
3. **兼容矩阵**：`compat` 候选矩阵冻结；旧四/五/六表快照到八表候选的迁移路径**已有候选定义**（`docs/audit/SNAPSHOT-MIGRATION-PATH.md`，待冻结）；`dnswire` 是否扩展及版本协商顺序。
4. **老库升级**：真实遗留生产库升级、历史数据对账、加固 DDL 版本迁移与同键 UPSERT 写路径兼容（现为只读识别；语义变更与写路径要求已有候选定义 `docs/audit/UPSERT-WRITE-PATH-COMPAT.md`，待冻结）。
5. **G1–G5 与 ACK 语义迁移**：跨连接并发与故障注入、真实旧主回归对账、端到端 replay、HA ACK 语义变更的兼容评审。
6. **B8.5 外部验收**：真实双主机、物理断电/未知提交、可控网络分区与 fencing/witness、真实 relay/客户端互操作、真实 Prometheus + `promtool` + Alertmanager 告警触发原始时间线（见 `docs/B8.5-外部验收Runbook.md`，全部仍为 `未验收`/`BLOCKED`）。
7. **生产边界**：`/cluster`、`/apps`、`/sso` 共 6 个 `501` 预留端点（用户已明确不做物理真机验收与独立专家签字 → 决策为**有意保留为产品边界**，逐项理由与重新评估条件见 `docs/audit/RESERVED-EXTENSION-DECISION.md`）；**`GET /dhcp/ha` 已于 round 285 升级为只读状态 API（200）**；目标 Linux 平台、容量与长稳。

> **状态更新注记（2026-11-11，round 283）**：第 2 项的「DHCP 租约 schema 冻结」已由用户明确授权自行决策完成（round 256–258，6 项决定记入 `contracts/enterprise-dhcp-lease.proposal.md` §6，状态 FROZEN），`internal/enterprise/dhcplease` 已实现并通过 29 个测试（§1cb 系列）；该授权替代了「独立专家评审冻结」这一形式要求，但**不改变本文件第 1/3/4/5/6/7 项的开放状态**，也不提升任何受保护写门禁。第 3/4 项的兼容矩阵、快照迁移路径、UPSERT 写路径仍为候选、待独立评审冻结。

## 7. 本文件的作用与限制

本文件把散落在 `docs/audit/P0-G1.md`、`docs/audit/IMPLEMENTATION-STATUS.md`、`contracts/version-compatibility.md`、`contracts/enterprise-dhcp-lease.proposal.md`、`docs/B8.5-外部验收Runbook.md` 的候选状态与证据汇成单一评审索引，**不新增任何权限、不改变任何门禁、不替代任何原始证据**。评审结论与现场证据仍须按 `docs/B8.5-外部验收Runbook.md` §10 模板独立归档。
